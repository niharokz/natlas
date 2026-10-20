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
	os.WriteFile(cfg, []byte("data_dir: "+data+"\nplugins: [events, subscriptions, calendar, health, inventory, architecture]\n"), 0o644)
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
	// calendar: synced Google entries are read-only and come from their own file
	if code, _ := call(t, c, "POST", u+"/api/p/calendar/google", map[string]any{"values": map[string]any{"title": "x", "date": "2026-10-20"}}); code != 403 {
		t.Fatalf("synced calendar must be read-only, got %d", code)
	}
	_, g := call(t, c, "GET", u+"/api/p/calendar/google", nil)
	if n := len(g["records"].([]any)); n != 7 || g["note"] != "Synced from Google Calendar · last sync 2026-10-18" {
		t.Fatalf("google calendar: %d records, note %q", n, g["note"])
	}
	synced, _ := os.ReadFile(filepath.Join(data, "calendar.md"))
	// own entries: add (time normalised), edit, delete - written to calendar-local.md only
	code, res = call(t, c, "POST", u+"/api/p/calendar/mine", map[string]any{"values": map[string]any{"title": "Standup", "date": "2026-10-21", "time": "9:15am"}})
	rec = res["record"].(map[string]any)
	if code != 201 || rec["time"] != "09:15" || rec["category"] != "event" {
		t.Fatalf("calendar add: %d %v", code, res)
	}
	if code, res = call(t, c, "PATCH", u+"/api/p/calendar/mine/"+rec["_id"].(string), map[string]any{"rev": rec["_rev"], "values": map[string]any{"time": "25:00"}}); code != 422 {
		t.Fatalf("bad time must fail: %d %v", code, res)
	}
	code, res = call(t, c, "POST", u+"/api/p/calendar/mine/quick", map[string]any{"text": "Dinner at 8pm tomorrow"})
	rec = res["record"].(map[string]any)
	if code != 201 || rec["title"] != "Dinner" || rec["time"] != "20:00" || rec["date"] != dates.Format(dates.Today().AddDate(0, 0, 1), "") {
		t.Fatalf("calendar quick add: %d %v", code, rec)
	}
	if code, _ := call(t, c, "DELETE", u+"/api/p/calendar/mine/"+rec["_id"].(string)+"?rev="+rec["_rev"].(string), nil); code != 200 {
		t.Fatalf("calendar delete: %d", code)
	}
	local, _ := os.ReadFile(filepath.Join(data, "calendar-local.md"))
	if after, _ := os.ReadFile(filepath.Join(data, "calendar.md")); !bytes.Equal(after, synced) ||
		!strings.Contains(string(local), "time: 09:15") || strings.Contains(string(local), "Dinner") {
		t.Fatalf("calendar files:\n%s", local)
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
	if code != 200 || len(res["stats"].([]any)) != 6 {
		t.Fatalf("dashboard: %d %v", code, res)
	}
	for _, st := range res["stats"].([]any) {
		if e := st.(map[string]any)["error"]; e != nil {
			t.Fatalf("dashboard error: %v", e)
		}
	}
}

// The architecture note: sections and the front record edit only their own text.
func TestDocument(t *testing.T) {
	srv, c, data := newServer(t)
	u := srv.URL
	call(t, c, "POST", u+"/api/login", map[string]string{"username": "u", "password": "p"})
	_, res := call(t, c, "GET", u+"/api/p/architecture/sections", nil)
	recs := res["records"].([]any)
	if len(recs) != 3 || recs[1].(map[string]any)["_id"] != "hardware" {
		t.Fatalf("sections: %v", recs)
	}
	hw := recs[1].(map[string]any)
	before, _ := os.ReadFile(filepath.Join(data, "homelab.md"))
	code, res := call(t, c, "PATCH", u+"/api/p/architecture/sections/hardware", map[string]any{"rev": hw["_rev"], "values": map[string]any{"body": "| Item | Value |\n|---|---|\n| RAM | 32 GB |\n"}})
	if code != 200 {
		t.Fatalf("edit section: %d %v", code, res)
	}
	// add a section right after Overview (index 1)
	if code, res = call(t, c, "POST", u+"/api/p/architecture/sections", map[string]any{"values": map[string]any{"title": "Network", "body": "Two VLANs."}, "at": 1}); code != 201 {
		t.Fatalf("add section: %d %v", code, res)
	}
	if code, res = call(t, c, "PATCH", u+"/api/p/architecture/about/record", map[string]any{"values": map[string]any{"reviewed": "2026-10-19"}}); code != 200 {
		t.Fatalf("edit front: %d %v", code, res)
	}
	after, _ := os.ReadFile(filepath.Join(data, "homelab.md"))
	got := string(after)
	for _, want := range []string{"reviewed: '2026-10-19'", "| RAM | 32 GB |", "flowchart LR\n", "## Overview", "\n\n## Network\n\nTwo VLANs.\n\n## Hardware\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	// the untouched Backups section is byte for byte the same
	tail := func(b []byte) string { return string(b[bytes.Index(b, []byte("## Backups")):]) }
	if tail(after) != tail(before) {
		t.Fatalf("Backups changed:\n%s", tail(after))
	}
}

// A missing calendar-local.md is created on the first save, with the list in place.
func TestCalendarLocalFileIsCreated(t *testing.T) {
	srv, c, data := newServer(t)
	os.Remove(filepath.Join(data, "calendar-local.md"))
	call(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "u", "password": "p"})
	if code, res := call(t, c, "GET", srv.URL+"/api/p/calendar/mine", nil); code != 200 || len(res["records"].([]any)) != 0 {
		t.Fatalf("empty list: %d %v", code, res)
	}
	if code, res := call(t, c, "POST", srv.URL+"/api/p/calendar/mine", map[string]any{"values": map[string]any{"title": "First", "date": "2026-11-01"}}); code != 201 {
		t.Fatalf("create: %d %v", code, res)
	}
	b, _ := os.ReadFile(filepath.Join(data, "calendar-local.md"))
	if !strings.Contains(string(b), "events:") || !strings.Contains(string(b), "title: First") {
		t.Fatalf("calendar-local.md:\n%s", b)
	}
}

func TestParseQuick(t *testing.T) {
	c := &config.Collection{QuickAdd: &config.QuickAdd{Title: "title", Date: "date", Tag: "tags", Priority: "priority", Time: "time"},
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
		"Meet 12.10":                     {"Meet", "2026-10-12", ""},
	}
	for text, want := range cases {
		got := parseQuick(c, text, today)
		date, _ := got["date"].(string)
		prio, _ := got["priority"].(string)
		if got["title"] != want[0] || date != want[1] || prio != want[2] || got["time"] != nil {
			t.Errorf("%q -> %v", text, got)
		}
	}
	times := map[string][2]string{ // text -> title, time
		"Dinner fri 8pm":       {"Dinner", "20:00"},
		"Call at 9:30 am":      {"Call", "09:30"},
		"Standup 10:15 mon":    {"Standup", "10:15"},
		"Lunch 12pm":           {"Lunch", "12:00"},
		"Gym 6.30am":           {"Gym", "06:30"},
		"Midnight snack 12am":  {"Midnight snack", "00:00"},
		"Flight 13pm tomorrow": {"Flight 13pm", ""},
	}
	for text, want := range times {
		got := parseQuick(c, text, today)
		tm, _ := got["time"].(string)
		if got["title"] != want[0] || tm != want[1] {
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
