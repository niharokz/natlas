package auth

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"natlas/internal/config"
)

func settings() *config.Settings {
	return &config.Settings{Username: "u", Password: "correct horse", SecretKey: strings.Repeat("s", 32),
		CookieSecure: true, TrustProxy: true}
}

func login(a *Auth, user, pass, ip string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/login", bytes.NewBufferString(`{"username":"`+user+`","password":"`+pass+`"}`))
	r.Header.Set("CF-Connecting-IP", ip)
	w := httptest.NewRecorder()
	a.Login(w, r)
	return w
}

func withCookie(w *httptest.ResponseRecorder) *http.Request {
	r := httptest.NewRequest("GET", "/api/me", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func TestLoginSessionAndTamper(t *testing.T) {
	a := New(settings())
	w := login(a, "u", "correct horse", "1.1.1.1")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	c := w.Result().Cookies()[0]
	if c.Name != "__Host-natlas" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Fatalf("cookie attributes: %+v", c)
	}
	if a.User(withCookie(w)) != "u" {
		t.Fatal("valid session rejected")
	}
	// tampered signature
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value[:len(c.Value)-2] + "xx"})
	if a.User(r) != "" {
		t.Fatal("tampered cookie accepted")
	}
	// changing the password signs everyone out
	s := settings()
	s.Password = "new password!"
	if New(s).User(withCookie(w)) != "" {
		t.Fatal("old session survived a password change")
	}
}

func TestHashedPassword(t *testing.T) {
	h, err := HashPassword("pass phrase")
	if err != nil || ValidHash(h) != nil {
		t.Fatalf("hash: %v %v", h, err)
	}
	s := settings()
	s.Password, s.PasswordHash = "", h
	a := New(s)
	if login(a, "u", "pass phrase", "2.2.2.2").Code != 200 {
		t.Fatal("hash login failed")
	}
	if login(a, "u", "pass phrasE", "2.2.2.3").Code != 401 {
		t.Fatal("wrong password accepted")
	}
	if ValidHash("plain") == nil {
		t.Fatal("garbage accepted as hash")
	}
}

func TestBanPublicIPForDays(t *testing.T) {
	s := settings()
	s.StateDir = t.TempDir()
	a := New(s)
	for i := 0; i < 5; i++ {
		login(a, "u", "nope", "3.3.3.3")
	}
	if w := login(a, "u", "correct horse", "3.3.3.3"); w.Code != 403 {
		t.Fatalf("banned IP got %d", w.Code)
	}
	if until, ok := a.Bans().Banned("3.3.3.3"); !ok || time.Until(until) < 6*24*time.Hour {
		t.Fatalf("ban should last 7 days: %v %v", until, ok)
	}
	if w := login(a, "u", "correct horse", "4.4.4.4"); w.Code != 200 {
		t.Fatalf("other IP should still log in, got %d", w.Code)
	}
	// every request from the banned IP is refused, health checks still work
	h := a.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for path, want := range map[string]int{"/app": 403, "/healthz": 200} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("CF-Connecting-IP", "3.3.3.3")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s from banned IP: %d", path, w.Code)
		}
	}
	// a restart keeps the ban; the CLI unban lifts it
	b := New(s)
	if _, ok := b.Bans().Banned("3.3.3.3"); !ok {
		t.Fatal("ban lost on restart")
	}
	if n, err := OpenBans(filepath.Join(s.StateDir, "bans.yml"), 7, 5).Unban("3.3.3.3"); n != 1 || err != nil {
		t.Fatalf("unban: %d %v", n, err)
	}
	if w := login(b, "u", "correct horse", "3.3.3.3"); w.Code != 200 {
		t.Fatalf("after unban: %d %s", w.Code, w.Body)
	}
}

func TestHomeIPOnlyLockedOut(t *testing.T) {
	a := New(settings())
	for i := 0; i < 5; i++ {
		login(a, "u", "nope", "192.168.1.20")
	}
	if w := login(a, "u", "correct horse", "192.168.1.20"); w.Code != 429 {
		t.Fatalf("home IP should get the 30 min lockout, got %d", w.Code)
	}
	if _, ok := a.Bans().Banned("192.168.1.20"); ok {
		t.Fatal("home IP must never be banned")
	}
}

func TestDeviceKeySessions(t *testing.T) {
	s := settings()
	s.SessionHours = 12
	s.DeviceKey = strings.Repeat("k", 40)
	a := New(s)
	w := login(a, "u", "correct horse", "5.5.5.5")
	sess, ok := a.session(withCookie(w))
	if !ok || sess.device || time.Until(sess.exp) > 13*time.Hour {
		t.Fatalf("plain login must be short: %+v", sess)
	}
	device := func(key string) *httptest.ResponseRecorder {
		r := withCookie(w)
		r.Method = "POST"
		r.Body = io.NopCloser(bytes.NewBufferString(`{"key":"` + key + `"}`))
		r.Header.Set("CF-Connecting-IP", "5.5.5.5")
		rec := httptest.NewRecorder()
		a.Device(rec, r)
		return rec
	}
	if rec := device("wrong"); rec.Code != 401 {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	rec := device(strings.Repeat("k", 40))
	if rec.Code != 200 {
		t.Fatalf("right key: %d %s", rec.Code, rec.Body)
	}
	sess, ok = a.session(withCookie(rec))
	if !ok || !sess.device || time.Until(sess.exp) < 300*24*time.Hour {
		t.Fatalf("device session must last until logout: %+v", sess)
	}
	// after logout, logging in again on this browser is long again (device cookie kept)
	r := httptest.NewRequest("POST", "/api/login", bytes.NewBufferString(`{"username":"u","password":"correct horse"}`))
	for _, c := range rec.Result().Cookies() {
		if c.Name == "__Host-natlas-device" {
			r.AddCookie(c)
		}
	}
	w2 := httptest.NewRecorder()
	a.Login(w2, r)
	if sess, ok := a.session(withCookie(w2)); !ok || !sess.device {
		t.Fatal("login on a remembered browser should be long")
	}
	// a new device key signs out remembered browsers
	s2 := settings()
	s2.DeviceKey = strings.Repeat("n", 40)
	if New(s2).User(withCookie(rec)) != "" {
		t.Fatal("old device session survived a new device key")
	}
	// wrong device keys count towards the ban
	for i := 0; i < 5; i++ {
		device("wrong")
	}
	if _, ok := a.Bans().Banned("5.5.5.5"); !ok {
		t.Fatal("wrong device keys must lead to a ban")
	}
}

func TestClientIP(t *testing.T) {
	a := New(settings())
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.18.0.5:5555"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 192.168.1.20") // first entry forged by the client
	if got := a.ClientIP(r); got != "192.168.1.20" {
		t.Fatalf("want the proxy-added (last) entry, got %s", got)
	}
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if got := a.ClientIP(r); got != "203.0.113.9" {
		t.Fatalf("cloudflare header ignored: %s", got)
	}
	s := settings()
	s.TrustProxy = false
	if got := New(s).ClientIP(r); got != "172.18.0.5" {
		t.Fatalf("untrusted proxy headers used: %s", got)
	}
}
