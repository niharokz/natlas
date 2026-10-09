package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"natlas/internal/config"
)

func TestRoutes(t *testing.T) {
	h, err := Handler("", &config.App{Title: "Natlas <x>"}, "2.1.0")
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}
	if code, body := get("/"); code != 200 || !strings.Contains(body, `href="/app"`) || !strings.Contains(body, "natlas 2.1.0") {
		t.Fatalf("landing: %d", code)
	}
	if code, body := get("/app"); code != 200 || !strings.Contains(body, `x-data="natlas"`) || !strings.Contains(body, "Natlas &lt;x&gt;") {
		t.Fatalf("app page: %d (title must be escaped)", code)
	}
	if code, _ := get("/app/"); code != 301 {
		t.Fatalf("/app/ should redirect, got %d", code)
	}
	for _, p := range []string{"/assets/vendor/", "/assets/web.go", "/nope"} {
		if code, _ := get(p); code != 404 {
			t.Errorf("%s: want 404, got %d", p, code)
		}
	}
	if code, body := get("/manifest.webmanifest"); code != 200 || !strings.Contains(body, `"start_url": "/app"`) {
		t.Fatalf("manifest: %d %s", code, body)
	}
	if code, body := get("/sw.js"); code != 200 || strings.Contains(body, "{{VERSION}}") {
		t.Fatal("service worker version not filled in")
	}
}
