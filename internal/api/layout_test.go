package api

import (
	"os"
	"strings"
	"testing"
)

func TestLayout(t *testing.T) {
	srv, c, _ := newServer(t)
	u := srv.URL
	call(t, c, "POST", u+"/api/login", map[string]string{"username": "u", "password": "p"})

	_, l := call(t, c, "GET", u+"/api/layout", nil)
	if menu := l["menu"].([]any); len(menu) != 8 || menu[0] != "events" || len(l["home"].([]any)) != 2 {
		t.Fatalf("default layout: %v", l)
	}
	code, l := call(t, c, "PUT", u+"/api/layout", map[string]any{
		"menu":   []string{"bookmarks", "nope", "events"},
		"hidden": []string{"inventory"},
		"home": []map[string]any{
			{"type": "list", "title": "Reading", "plugin": "bookmarks", "coll": "bookmarks", "search": "reading"},
			{"type": "agenda", "sections": []string{"overdue", "today"}},
			{"type": "note", "text": "Hello"},
		},
	})
	menu := l["menu"].([]any)
	if code != 200 || menu[0] != "bookmarks" || menu[1] != "events" || len(menu) != 8 || l["hidden"].([]any)[0] != "inventory" {
		t.Fatalf("save: %d %v", code, l)
	}
	if b := l["home"].([]any)[0].(map[string]any); b["limit"] != float64(6) {
		t.Fatalf("list block default limit: %v", b)
	}
	_, again := call(t, c, "GET", u+"/api/layout", nil)
	if again["menu"].([]any)[0] != "bookmarks" || len(again["home"].([]any)) != 3 {
		t.Fatalf("not kept: %v", again)
	}
	if code, _ := call(t, c, "PUT", u+"/api/layout", map[string]any{"home": []map[string]any{{"type": "list", "plugin": "events", "coll": "x"}}}); code != 400 {
		t.Fatalf("bad block accepted: %d", code)
	}
	if code, _ := call(t, c, "PUT", u+"/api/layout", map[string]any{"home": []map[string]any{}}); code != 200 {
		t.Fatal("empty home refused")
	}
	if _, l := call(t, c, "GET", u+"/api/layout", nil); len(l["home"].([]any)) != 0 {
		t.Fatalf("an empty Today screen is a choice, not a reset: %v", l)
	}
	if _, l := call(t, c, "PUT", u+"/api/layout", map[string]any{"reset": true}); len(l["home"].([]any)) != 2 || l["menu"].([]any)[0] != "events" {
		t.Fatalf("reset: %v", l)
	}
}

func TestReadOnlyDemo(t *testing.T) {
	srv, c, data := newServerRO(t, true)
	u := srv.URL
	call(t, c, "POST", u+"/api/login", map[string]string{"username": "u", "password": "p"})
	before, _ := os.ReadFile(data + "/event.md")
	if code, _ := call(t, c, "GET", u+"/api/p/events/events", nil); code != 200 {
		t.Fatal("reading must work")
	}
	for _, w := range [][2]string{{"POST", "/api/p/events/events"}, {"POST", "/api/p/events/events/quick"}, {"PUT", "/api/layout"}} {
		if code, out := call(t, c, w[0], u+w[1], map[string]any{"text": "x"}); code != 403 || !strings.Contains(out["error"].(string), "read-only") {
			t.Fatalf("%s %s: %d", w[0], w[1], code)
		}
	}
	if after, _ := os.ReadFile(data + "/event.md"); string(after) != string(before) {
		t.Fatal("demo changed a file")
	}
}

func TestQuickBookmark(t *testing.T) {
	srv, c, data := newServer(t)
	u := srv.URL
	call(t, c, "POST", u+"/api/login", map[string]string{"username": "u", "password": "p"})
	code, out := call(t, c, "POST", u+"/api/p/bookmarks/bookmarks/quick", map[string]any{"text": "https://www.example.com/x #a #b"})
	r, _ := out["record"].(map[string]any)
	if code != 201 || r["name"] != "example.com" || len(r["tags"].([]any)) != 2 {
		t.Fatalf("quick bookmark: %d %v", code, out)
	}
	b, _ := os.ReadFile(data + "/bookmark.md")
	if !strings.Contains(string(b), `  - { name: "example.com", url: "https://www.example.com/x", tags: ["a", "b"], added: '`) || strings.Contains(string(b), "id:") {
		t.Fatalf("bookmark file:\n%s", b)
	}
	_, list := call(t, c, "GET", u+"/api/p/bookmarks/bookmarks", nil)
	if opts := list["options"].(map[string]any)["tags"].([]any); len(opts) < 3 {
		t.Fatalf("tag options: %v", opts)
	}
}
