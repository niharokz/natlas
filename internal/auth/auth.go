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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"natlas/internal/config"
)

const (
	deviceLife   = 400 * 24 * time.Hour   // browsers cap cookies at 400 days; renewed on use
	lockout      = 30 * time.Minute       // home (private) addresses: short lockout, never a ban
	failDelay    = 400 * time.Millisecond // slows down guessing even from many IPs
	maxTrackedIP = 10_000                 // memory cap for the failure table
)

// Auth checks credentials and sessions.
type Auth struct {
	s        *config.Settings
	key      []byte // signs short sessions
	dkey     []byte // signs "until logout" sessions and the device mark (needs the device key)
	cookie   string // session cookie name
	device   string // device cookie name
	bans     *Bans
	mu       sync.Mutex
	failures map[string][]time.Time // private ip -> recent failed attempts (30 min lockout)
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
	m.Write([]byte("natlas-session-v3\x00" + s.Username + "\x00" + credential))
	key := m.Sum(nil)
	d := hmac.New(sha256.New, key)
	d.Write([]byte("device\x00" + s.DeviceKey)) // a new device key signs out every remembered browser
	a := &Auth{s: s, key: key, dkey: d.Sum(nil), cookie: "natlas_session", device: "natlas_device", failures: map[string][]time.Time{}}
	if s.CookieSecure {
		a.cookie, a.device = "__Host-natlas", "__Host-natlas-device"
	}
	banFile := ""
	if !s.Local && s.StateDir != "" {
		banFile = filepath.Join(s.StateDir, "bans.yml")
	}
	max, days := s.MaxFailures, s.BanDays
	if max < 1 {
		max = 5
	}
	if days < 1 {
		days = 7
	}
	a.bans = OpenBans(banFile, days, max)
	return a
}

// Bans is the IP ban list (natlas bans / unban use it too).
func (a *Auth) Bans() *Bans { return a.bans }

// localCookie carries the phone app's launch token (NATLAS_MODE=local).
const localCookie = "natlas_local"

// session is a decoded, verified session cookie.
type session struct {
	user   string
	exp    time.Time
	device bool // signed in "until logout" (device key given on this browser)
}

func (a *Auth) session(r *http.Request) (session, bool) {
	c, err := r.Cookie(a.cookie)
	if err != nil {
		return session{}, false
	}
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return session{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return session{}, false
	}
	parts := strings.Split(string(raw), "|") // user|expiry|kind
	if len(parts) != 3 || (parts[2] != "s" && parts[2] != "d") {
		return session{}, false
	}
	key := a.key
	if parts[2] == "d" {
		if a.s.DeviceKey == "" {
			return session{}, false // device key removed from .env: only short sessions
		}
		key = a.dkey
	}
	if !hmac.Equal([]byte(sig), []byte(sign(key, payload))) {
		return session{}, false
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry || parts[0] != a.s.Username {
		return session{}, false
	}
	return session{user: parts[0], exp: time.Unix(expiry, 0), device: parts[2] == "d"}, true
}

// User returns the logged-in username, or "" when the request has no valid session.
func (a *Auth) User(r *http.Request) string {
	if a.s.Local { // phone app: the per-launch token is the session
		c, err := r.Cookie(localCookie)
		if err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(a.s.LocalToken)) == 1 {
			return "phone"
		}
		return ""
	}
	if s, ok := a.session(r); ok {
		return s.user
	}
	return ""
}

// Require wraps a handler so it only runs for logged-in users. A remembered
// browser's session is renewed once a day, so it never runs out while used.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.User(r) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not logged in"})
			return
		}
		if s, ok := a.session(r); ok && s.device && time.Until(s.exp) < deviceLife-24*time.Hour {
			a.setSession(w, true)
		}
		next.ServeHTTP(w, r)
	})
}

// Guard answers 403 to banned addresses, before anything else runs.
func (a *Auth) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.s.Local && r.URL.Path != "/healthz" {
			if until, banned := a.bans.Banned(a.ClientIP(r)); banned {
				msg := "Blocked until " + until.Format("2 Jan 2006") + " after too many failed logins."
				if strings.HasPrefix(r.URL.Path, "/api/") {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
				} else {
					w.Header().Set("Cache-Control", "no-store")
					http.Error(w, msg, http.StatusForbidden)
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// setSession issues the session cookie: until logout with the device mark, else short.
func (a *Auth) setSession(w http.ResponseWriter, device bool) {
	kind, key, life := "s", a.key, time.Duration(a.s.SessionHours)*time.Hour
	if life <= 0 {
		life = 12 * time.Hour
	}
	if device {
		kind, key, life = "d", a.dkey, deviceLife
	}
	exp := time.Now().Add(life)
	payload := base64.RawURLEncoding.EncodeToString([]byte(a.s.Username + "|" + strconv.FormatInt(exp.Unix(), 10) + "|" + kind))
	http.SetCookie(w, &http.Cookie{
		Name: a.cookie, Value: payload + "." + sign(key, payload), Path: "/",
		Expires: exp, HttpOnly: true, Secure: a.s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

// deviceMark is the device cookie's value: proof that the device key was
// entered on this browser (the key itself never leaves the server).
func (a *Auth) deviceMark() string { return sign(a.dkey, "device-mark|"+a.s.Username) }

func (a *Auth) hasDevice(r *http.Request) bool {
	if a.s.DeviceKey == "" {
		return false
	}
	c, err := r.Cookie(a.device)
	return err == nil && hmac.Equal([]byte(c.Value), []byte(a.deviceMark()))
}

func (a *Auth) setDevice(w http.ResponseWriter, on bool) {
	c := &http.Cookie{Name: a.device, Path: "/", HttpOnly: true, Secure: a.s.CookieSecure, SameSite: http.SameSiteStrictMode}
	if on {
		c.Value, c.Expires = a.deviceMark(), time.Now().Add(deviceLife)
	} else {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

// blocked answers a banned or locked-out address; true when it did.
func (a *Auth) blocked(w http.ResponseWriter, ip string) bool {
	if until, banned := a.bans.Banned(ip); banned {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Blocked until " + until.Format("2 Jan 2006") + " after too many failed logins."})
		return true
	}
	if wait := a.lockedFor(ip); wait > 0 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": fmt.Sprintf("Too many failed attempts. Try again in %d min.", int(wait.Minutes())+1)})
		return true
	}
	return false
}

// failed records a wrong password or device key and builds the reply text.
func (a *Auth) failed(ip, what string) string {
	time.Sleep(failDelay)
	msg := "Wrong " + what
	if Bannable(ip) {
		left, banned := a.bans.Fail(ip, "wrong "+what)
		log.Printf("[auth] wrong %s from %s (%d attempts left)", what, ip, max(left, 0))
		switch {
		case banned:
			log.Printf("[auth] BANNED %s for %d days", ip, a.bans.days)
			return msg + ". This address is now blocked for " + strconv.Itoa(a.bans.days) + " days."
		case left <= 2:
			return msg + fmt.Sprintf(" (%d attempts left before this address is blocked for %d days)", left, a.bans.days)
		}
		return msg
	}
	left := a.fail(ip)
	log.Printf("[auth] wrong %s from %s (home network, %d attempts left)", what, ip, left)
	if left <= 2 {
		msg += fmt.Sprintf(" (%d attempts left before a 30 min lockout)", left)
	}
	return msg
}

// Login handles POST /api/login {username, password}. A browser that was
// given the device key stays signed in until logout; others for SessionHours.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	ip := a.ClientIP(r)
	if a.blocked(w, ip) {
		return
	}
	var body struct{ Username, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Bad request"})
		return
	}
	if !a.check(body.Username, body.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": a.failed(ip, "username or password")})
		return
	}
	a.mu.Lock()
	delete(a.failures, ip)
	a.mu.Unlock()
	a.bans.Clear(ip)
	device := a.hasDevice(r)
	a.setSession(w, device)
	writeJSON(w, http.StatusOK, map[string]any{"username": a.s.Username, "remembered": device})
}

// Device handles POST /api/device {key} (logged in): with the right
// NATLAS_DEVICE_KEY this browser stays signed in until logout, also after
// the next login. A wrong key counts as a failed login. DELETE forgets it.
func (a *Auth) Device(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		a.setDevice(w, false)
		a.setSession(w, false)
		writeJSON(w, http.StatusOK, map[string]bool{"remembered": false})
		return
	}
	ip := a.ClientIP(r)
	if a.blocked(w, ip) {
		return
	}
	if a.s.DeviceKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No device key on the server: set NATLAS_DEVICE_KEY in .env"})
		return
	}
	var body struct{ Key string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Bad request"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Key)), []byte(a.s.DeviceKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": a.failed(ip, "device key")})
		return
	}
	a.bans.Clear(ip)
	a.setDevice(w, true)
	a.setSession(w, true)
	writeJSON(w, http.StatusOK, map[string]bool{"remembered": true})
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
	sess, _ := a.session(r)
	out := map[string]any{"authenticated": user != "", "username": user, "local": a.s.Local,
		"remembered": sess.device, "device_key": a.s.DeviceKey != "", "session_hours": a.s.SessionHours}
	if user != "" && !a.s.Local {
		out["expires"] = sess.exp.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// LocalEntry handles GET /local?t=<token> in the phone app: it turns the
// launch token into a cookie and opens the app (only paths under /app).
func (a *Auth) LocalEntry(w http.ResponseWriter, r *http.Request) {
	t := r.URL.Query().Get("t")
	if !a.s.Local || subtle.ConstantTimeCompare([]byte(t), []byte(a.s.LocalToken)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: localCookie, Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	next := r.URL.Query().Get("next")
	if !strings.HasPrefix(next, "/app") || strings.HasPrefix(next, "//") {
		next = "/app"
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, next, http.StatusFound)
}

// Local reports whether this is the phone app.
func (a *Auth) Local() bool { return a.s.Local }

func sign(key []byte, payload string) string {
	m := hmac.New(sha256.New, key)
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
	if len(rec) < a.bans.max {
		return 0
	}
	return time.Until(rec[len(rec)-a.bans.max].Add(lockout))
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
	return a.bans.max - len(a.failures[ip])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
