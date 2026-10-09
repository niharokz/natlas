package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestLockoutPerIP(t *testing.T) {
	a := New(settings())
	for i := 0; i < maxFailures; i++ {
		login(a, "u", "nope", "3.3.3.3")
	}
	if w := login(a, "u", "correct horse", "3.3.3.3"); w.Code != 429 {
		t.Fatalf("locked IP got %d", w.Code)
	}
	if w := login(a, "u", "correct horse", "4.4.4.4"); w.Code != 200 {
		t.Fatalf("other IP should still log in, got %d", w.Code)
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
