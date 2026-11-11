package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"natlas/internal/config"
)

func TestRoutes(t *testing.T) {
	h, err := Handler("", &config.App{Title: "Natlas <x>"}, "1.0.0", Demo{User: "demo", Password: "d<e>mo"})
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}
	if code, body := get("/"); code != 200 || !strings.Contains(body, `href="/app"`) || !strings.Contains(body, "natlas 1.0.0") ||
		strings.Contains(body, "d&lt;e&gt;mo") || !strings.Contains(body, "/natlas.apk") {
		t.Fatalf("landing: %d (APK link expected, no demo login)", code)
	}
	if code, body := get("/app"); code != 200 || !strings.Contains(body, `x-data="natlas"`) || !strings.Contains(body, "Natlas &lt;x&gt;") ||
		!strings.Contains(body, `data-p="d&lt;e&gt;mo"`) {
		t.Fatalf("app page: %d (title escaped, demo login on the sign-in form)", code)
	}
	if _, body := get("/app"); strings.Contains(body, "{{>") || !strings.Contains(body, `class="cal"`) || !strings.Contains(body, `x-ref="sheet"`) {
		t.Fatal("app page: every views/ part must be included (calendar, sheet …)")
	}
	if _, body := get("/sw.js"); !strings.Contains(body, "/assets/js/") {
		t.Fatal("service worker must cache js/ and css/")
	}
	if code, _ := get("/app/"); code != 301 {
		t.Fatalf("/app/ should redirect, got %d", code)
	}
	for _, p := range []string{"/assets/vendor/", "/assets/web.go", "/assets/views/today.html", "/nope"} {
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
