package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/internal/dates"
)

// newServer runs the real plugins against a copy of examples/data.
func newServer(t *testing.T) (*httptest.Server, *http.Client, string) {
	t.Helper()
	root, _ := filepath.Abs("../..")
	data := t.TempDir()
	entries, _ := os.ReadDir(filepath.Join(root, "examples", "data"))
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(root, "examples", "data", e.Name()))
		os.WriteFile(filepath.Join(data, e.Name()), b, 0o644)
	}
	cfg := filepath.Join(t.TempDir(), "natlas.yml")
	os.WriteFile(cfg, []byte("data_dir: "+data+"\nplugins: [events, subscriptions, birthdays, health, inventory]\n"), 0o644)
	s := &config.Settings{Username: "u", Password: "p", SecretKey: strings.Repeat("k", 32),
		ConfigFile: cfg, PluginsDir: filepath.Join(root, "plugins")}
	app, err := config.Load(s)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Server{App: app, Auth: auth.New(s)}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return srv, &http.Client{Jar: jar, Timeout: 5 * time.Second}, data
}

func call(t *testing.T, c *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func find(list []any, id string) map[string]any {
	for _, r := range list {
		if m := r.(map[string]any); m["_id"] == id {
			return m
		}
	}
	return nil
}

func TestFlow(t *testing.T) {
	srv, c, data := newServer(t)
	u := srv.URL

	if code, _ := call(t, c, "GET", u+"/api/p/events/events", nil); code != 401 {
		t.Fatalf("unauthenticated list: %d", code)
	}
	if code, _ := call(t, c, "POST", u+"/api/login", map[string]string{"username": "u", "password": "nope"}); code != 401 {
		t.Fatal("bad password accepted")
	}
	if code, _ := call(t, c, "POST", u+"/api/login", map[string]string{"username": "u", "password": "p"}); code != 200 {
		t.Fatal("login failed")
	}

	_, list := call(t, c, "GET", u+"/api/p/events/events", nil)
	ev := find(list["records"].([]any), "call-bank")
	rev := ev["_rev"].(string)

	// one-tap Done
	code, res := call(t, c, "POST", u+"/api/p/events/events/call-bank/do/done", map[string]any{"rev": rev})
	if code != 200 || res["record"].(map[string]any)["status"] != "closed" {
		t.Fatalf("done: %d %v", code, res)
	}
	// the old revision is now stale -> 409, nothing overwritten
	if code, _ := call(t, c, "PATCH", u+"/api/p/events/events/call-bank", map[string]any{"rev": rev, "values": map[string]any{"status": "open"}}); code != 409 {
		t.Fatalf("stale edit must conflict, got %d", code)
	}
	// validation
	code, res = call(t, c, "PATCH", u+"/api/p/events/events/city-10k", map[string]any{"values": map[string]any{"priority": "urgent"}})
	if code != 422 || res["fields"].(map[string]any)["priority"] == nil {
		t.Fatalf("validation: %d %v", code, res)
	}
	// quick add with smart parsing
	code, res = call(t, c, "POST", u+"/api/p/events/events/quick", map[string]any{"text": "Pay rent tomorrow #bills !h"})
	rec := res["record"].(map[string]any)
	if code != 201 || rec["title"] != "Pay rent" || rec["tags"] != "bills" || rec["priority"] != "high" ||
		rec["date"] != dates.Format(dates.Today().AddDate(0, 0, 1), "") || rec["frequency"] != "none" || rec["status"] != "open" {
		t.Fatalf("quick add: %d %v", code, rec)
	}
	// read-only plugin
	if code, _ := call(t, c, "POST", u+"/api/p/birthdays/people", map[string]any{"values": map[string]any{"name": "x"}}); code != 403 {
		t.Fatalf("birthdays must be read-only, got %d", code)
	}
	// cross-site write blocked
	req, _ := http.NewRequest("POST", u+"/api/p/events/events/quick", strings.NewReader(`{"text":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	if r, _ := c.Do(req); r.StatusCode != 403 {
		t.Fatalf("cross-origin write: %d", r.StatusCode)
	}

	// health: move a planned run into recent with asked values
	_, planned := call(t, c, "GET", u+"/api/p/health/planned", nil)
	run := planned["records"].([]any)[0].(map[string]any)
	code, res = call(t, c, "POST", u+"/api/p/health/planned/"+run["_id"].(string)+"/do/complete",
		map[string]any{"rev": run["_rev"], "values": map[string]any{"time": "55:20", "heart": "Zone 4", "cost": "1200", "collectible": "medal"}})
	if code != 200 || res["collection"] != "recent" {
		t.Fatalf("complete run: %d %v", code, res)
	}
	b, _ := os.ReadFile(filepath.Join(data, "health.md"))
	if !strings.Contains(string(b), "time: '55:20'") || !strings.Contains(string(b), "cost: 1200") {
		t.Fatalf("health.md:\n%s", b)
	}

	// dashboard
	code, res = call(t, c, "GET", u+"/api/dashboard", nil)
	if code != 200 || len(res["stats"].([]any)) != 5 {
		t.Fatalf("dashboard: %d %v", code, res)
	}
}

func TestParseQuick(t *testing.T) {
	c := &config.Collection{QuickAdd: &config.QuickAdd{Title: "title", Date: "date", Tag: "tags", Priority: "priority"},
		Fields: []*config.Field{{Key: "priority", OptionList: []string{"high", "medium", "low"}}}}
	c.Reindex()
	today, _ := dates.Parse("2026-10-05", "") // a Monday
	cases := map[string][3]string{            // text -> title, date, priority
		"Call bank tomorrow #todo !high": {"Call bank", "2026-10-06", "high"},
		"Renew passport 12 oct":          {"Renew passport", "2026-10-12", ""},
		"Dentist jan 3":                  {"Dentist", "2027-01-03", ""},
		"Pay rent mon":                   {"Pay rent", "2026-10-12", ""},
		"Parcel +3 !l":                   {"Parcel", "2026-10-08", "low"},
		"Trip 25/12":                     {"Trip", "2026-12-25", ""},
		"Buy 2 apples":                   {"Buy 2 apples", "", ""},
	}
	for text, want := range cases {
		got := parseQuick(c, text, today)
		date, _ := got["date"].(string)
		prio, _ := got["priority"].(string)
		if got["title"] != want[0] || date != want[1] || prio != want[2] {
			t.Errorf("%q -> %v", text, got)
		}
	}
}

func TestPublicMessageHidesPaths(t *testing.T) {
	got := publicMessage(errors.New("open /home/someone/data/event.md: permission denied"))
	if got != "open event.md: permission denied" {
		t.Fatalf("got %q", got)
	}
}
