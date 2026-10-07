// Package auth is single-user login: a signed session cookie (HMAC-SHA256,
// no server-side session store) plus a per-IP lockout after repeated
// failures.
//
// Hardening notes:
//   - The cookie signing key is derived from NATLAS_SECRET_KEY *and* the
//     password, so changing the password logs out every existing session.
//   - Over HTTPS the cookie is named "__Host-natlas": browsers then refuse it
//     unless it is Secure, host-only and Path=/.
//   - The password may be stored as a PBKDF2 hash (NATLAS_PASSWORD_HASH).
//   - Behind a proxy the client IP comes from CF-Connecting-IP, X-Real-IP or
//     the *last* X-Forwarded-For entry (the one the proxy added), never the
//     first entry, which any client can forge.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"natlas/internal/config"
)

const (
	sessionLife  = 30 * 24 * time.Hour
	maxFailures  = 5
	lockout      = 30 * time.Minute
	failDelay    = 400 * time.Millisecond // slows down guessing even from many IPs
	maxTrackedIP = 10_000                 // memory cap for the failure table
)

// Auth checks credentials and sessions.
type Auth struct {
	s        *config.Settings
	key      []byte // cookie signing key
	cookie   string // cookie name
	mu       sync.Mutex
	failures map[string][]time.Time // ip -> recent failed attempts
}

// New creates the authenticator.
func New(s *config.Settings) *Auth {
	// Bind the signing key to the credential: a new password invalidates
	// every cookie signed with the old one.
	credential := s.PasswordHash
	if credential == "" {
		credential = s.Password
	}
	m := hmac.New(sha256.New, []byte(s.SecretKey))
	m.Write([]byte("natlas-session-v2\x00" + s.Username + "\x00" + credential))
	a := &Auth{s: s, key: m.Sum(nil), cookie: "natlas_session", failures: map[string][]time.Time{}}
	if s.CookieSecure {
		a.cookie = "__Host-natlas"
	}
	return a
}

// User returns the logged-in username, or "" when the request has no valid session.
func (a *Auth) User(r *http.Request) string {
	c, err := r.Cookie(a.cookie)
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
	ip := a.ClientIP(r)
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
	if !a.check(body.Username, body.Password) {
		left := a.fail(ip)
		log.Printf("[auth] failed login from %s (%d attempts left)", ip, left)
		time.Sleep(failDelay)
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
		Name: a.cookie, Value: payload + "." + a.sign(payload), Path: "/",
		Expires: exp, HttpOnly: true, Secure: a.s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": a.s.Username})
}

// check compares credentials without leaking which part was wrong or how
// much of it matched.
func (a *Auth) check(user, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.s.Username)) == 1
	var passOK bool
	if a.s.PasswordHash != "" {
		passOK = checkHash(a.s.PasswordHash, password)
	} else {
		passOK = subtle.ConstantTimeCompare([]byte(password), []byte(a.s.Password)) == 1
	}
	return userOK && passOK
}

// Logout handles POST /api/logout.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: a.cookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.s.CookieSecure, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Me handles GET /api/me.
func (a *Auth) Me(w http.ResponseWriter, r *http.Request) {
	user := a.User(r)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": user != "", "username": user})
}

func (a *Auth) sign(payload string) string {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// ClientIP is the visitor's address, used for the login lockout and logs.
//
// With NATLAS_TRUST_PROXY (the default, for use behind Caddy/Cloudflare) it
// reads, in order: CF-Connecting-IP (set by Cloudflare), X-Real-IP, then the
// last X-Forwarded-For entry. The last entry is the one your own proxy
// appended; earlier entries come from the client and can be forged.
func (a *Auth) ClientIP(r *http.Request) string {
	if a.s.TrustProxy {
		for _, h := range []string{"CF-Connecting-IP", "X-Real-IP"} {
			if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(h))); ip != nil {
				return ip.String()
			}
		}
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			parts := strings.Split(xff[len(xff)-1], ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
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
	if len(kept) == 0 {
		delete(a.failures, ip)
		return nil
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
	if len(a.failures) >= maxTrackedIP { // forget expired entries before growing further
		for k := range a.failures {
			a.recent(k)
		}
	}
	a.failures[ip] = append(a.recent(ip), time.Now())
	return maxFailures - len(a.failures[ip])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
