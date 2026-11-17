package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// a write the browser marks as cross-site is refused, whatever its Origin says
func TestSameOriginHonoursSecFetchSite(t *testing.T) {
	ok := sameOrigin(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	cases := []struct {
		site, origin string
		want         int
	}{
		{"", "", 204}, // curl, the phone widgets
		{"same-origin", "https://example.com", 204}, // the app itself
		{"none", "", 204}, // typed address / bookmark
		{"cross-site", "", 403},
		{"same-site", "https://example.com", 403}, // a sibling subdomain is still foreign
		{"", "https://evil.test", 403},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "https://example.com/api/x", strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		if c.site != "" {
			r.Header.Set("Sec-Fetch-Site", c.site)
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		w := httptest.NewRecorder()
		ok(w, r)
		if w.Code != c.want {
			t.Errorf("site=%q origin=%q: got %d, want %d", c.site, c.origin, w.Code, c.want)
		}
	}
}
