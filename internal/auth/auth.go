// Package auth is single-user login: a signed session cookie (HMAC-SHA256,
// no server-side session store) plus a per-IP lockout after repeated
// failures.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"natlas/internal/config"
)

const (
	cookieName  = "natlas_session"
	sessionLife = 30 * 24 * time.Hour
	maxFailures = 5
	lockout     = 30 * time.Minute
)

// Auth checks credentials and sessions.
type Auth struct {
	s        *config.Settings
	mu       sync.Mutex
	failures map[string][]time.Time // ip -> recent failed attempts
}

// New creates the authenticator.
func New(s *config.Settings) *Auth {
	return &Auth{s: s, failures: map[string][]time.Time{}}
}

// User returns the logged-in username, or "" when the request has no valid session.
func (a *Auth) User(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	user, exp, ok := strings.Cut(string(raw), "|")
	expiry, err := strconv.ParseInt(exp, 10, 64)
	if !ok || err != nil || time.Now().Unix() > expiry || user != a.s.Username {
		return ""
	}
	return user
}

// Require wraps a handler so it only runs for logged-in users.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.User(r) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not logged in"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Login handles POST /api/login {username, password}.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	ip := a.clientIP(r)
	if wait := a.lockedFor(ip); wait > 0 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": fmt.Sprintf("Too many failed attempts. Try again in %d min.", int(wait.Minutes())+1)})
		return
	}
	var body struct{ Username, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Bad request"})
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(a.s.Username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(body.Password), []byte(a.s.Password)) == 1
	if !userOK || !passOK {
		left := a.fail(ip)
		msg := "Wrong username or password"
		if left <= 2 {
			msg += fmt.Sprintf(" (%d attempts left before a 30 min lockout)", left)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg})
		return
	}
	a.mu.Lock()
	delete(a.failures, ip)
	a.mu.Unlock()

	exp := time.Now().Add(sessionLife)
	payload := base64.RawURLEncoding.EncodeToString([]byte(a.s.Username + "|" + strconv.FormatInt(exp.Unix(), 10)))
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: payload + "." + a.sign(payload), Path: "/",
		Expires: exp, HttpOnly: true, Secure: a.s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": a.s.Username})
}

// Logout handles POST /api/logout.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.s.CookieSecure, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Me handles GET /api/me.
func (a *Auth) Me(w http.ResponseWriter, r *http.Request) {
	user := a.User(r)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": user != "", "username": user})
}

func (a *Auth) sign(payload string) string {
	m := hmac.New(sha256.New, []byte(a.s.SecretKey))
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// clientIP is the visitor's address. Behind Caddy the TCP peer is Caddy, so
// the first X-Forwarded-For entry is used when TrustProxy is on.
func (a *Auth) clientIP(r *http.Request) string {
	if a.s.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recent drops failures older than the lockout window. Caller holds mu.
func (a *Auth) recent(ip string) []time.Time {
	cutoff := time.Now().Add(-lockout)
	kept := a.failures[ip][:0]
	for _, t := range a.failures[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	a.failures[ip] = kept
	return kept
}

func (a *Auth) lockedFor(ip string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec := a.recent(ip)
	if len(rec) < maxFailures {
		return 0
	}
	return time.Until(rec[len(rec)-maxFailures].Add(lockout))
}

func (a *Auth) fail(ip string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[ip] = append(a.recent(ip), time.Now())
	return maxFailures - len(a.failures[ip])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
