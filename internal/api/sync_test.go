package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"natlas/internal/dates"
)

func TestHealthUpsert(t *testing.T) {
	s := newPhone(t)
	file := filepath.Join(s.App.DataDir, "health.md")
	before, _ := os.ReadFile(file)

	recs := []any{
		map[string]any{"date": "2026-10-20", "steps": 12034, "sleep_h": 7.2, "weight": 66.3, "body_fat": 24.9, "sleep_start": "23:10"},
		map[string]any{"date": "2026-10-19", "steps": 99999}, // existing day: only steps changes
		map[string]any{"date": "2026-07-01", "steps": 5000},  // older than everything: goes first
	}
	code, out := s.Exec(map[string]any{"op": "upsert", "plugin": "health", "coll": "daily", "records": recs})
	if code != 200 || out["added"] != 2 || out["changed"] != 1 {
		t.Fatalf("upsert: %d %v", code, out)
	}
	after, _ := os.ReadFile(file)
	text := string(after)
	if !strings.Contains(text, "weight: 66.3kg") || !strings.Contains(text, "body_fat: '24.9'") ||
		!strings.Contains(text, "last_updated: "+dates.Format(dates.Today(), "02 Jan 2006")) {
		t.Fatalf("profile not updated:\n%s", text[strings.Index(text, "meta:"):][:300])
	}
	i1, i2, i3 := strings.Index(text, "date: '2026-07-01'"), strings.Index(text, "date: '2026-07-22'"), strings.Index(text, "date: '2026-10-20'")
	if !(i1 >= 0 && i1 < i2 && i2 < i3) {
		t.Fatalf("days out of order: %d %d %d", i1, i2, i3)
	}
	if !strings.Contains(text, "steps: 99999") || !strings.Contains(text, "sleep_start: '23:10'") {
		t.Fatal("values not written")
	}
	// the rest of the file is untouched (races, gym)
	for _, keep := range []string{"name: CITY 10K", "registration: To Do", "goal_steps: 8000"} {
		if !strings.Contains(text, keep) {
			t.Fatalf("lost %q", keep)
		}
	}
	if len(after) <= len(before) {
		t.Fatal("file did not grow")
	}

	// same sync again: nothing changes, nothing written
	st, _ := os.Stat(file)
	time.Sleep(20 * time.Millisecond)
	code, out = s.Exec(map[string]any{"op": "upsert", "plugin": "health", "coll": "daily", "records": recs})
	st2, _ := os.Stat(file)
	if code != 200 || out["added"] != 0 || out["changed"] != 0 || out["latest"] != 0 || !st.ModTime().Equal(st2.ModTime()) {
		t.Fatalf("repeat sync must not write: %d %v", code, out)
	}

	// bad input
	if code, _ := s.Exec(map[string]any{"op": "upsert", "plugin": "health", "coll": "daily", "records": []any{map[string]any{"steps": 1}}}); code != 400 {
		t.Fatalf("missing date: %d", code)
	}
	if code, _ := s.Exec(map[string]any{"op": "upsert", "plugin": "health", "coll": "daily", "records": []any{map[string]any{"date": "2026-10-21", "steps": "many"}}}); code != 422 {
		t.Fatalf("bad number: %d", code)
	}
	if code, _ := s.Exec(map[string]any{"op": "upsert", "plugin": "events", "coll": "events", "records": []any{}}); code != 400 {
		t.Fatalf("no sync config: %d", code)
	}
}

func TestSnapshotCharts(t *testing.T) {
	s := newPhone(t)
	snap := s.Snapshot(time.Now())
	charts := snap["charts"].([]map[string]any)
	if len(charts) != 1 || charts[0]["coll"] != "daily" || charts[0]["sync"] != true {
		t.Fatalf("charts: %v", charts)
	}
	ch := charts[0]
	if g := ch["goals"].(map[string]any); g["steps"] != 8000.0 || g["sleep_h"] != 7.5 {
		t.Fatalf("goals: %v", g)
	}
	if ch["latest"].(map[string]any)["weight"] == nil || ch["last_day"] != "2026-10-19" {
		t.Fatalf("latest: %v %v", ch["latest"], ch["last_day"])
	}
	for _, l := range snap["lists"].([]map[string]any) {
		if l["coll"] == "daily" {
			t.Fatal("daily must not be a list widget source")
		}
	}
	for _, p := range snap["plugins"].([]map[string]any) {
		if p["id"] == "health" && p["collections"].([]map[string]any)[0]["sync"] != "health_connect" {
			t.Fatal("sync source missing")
		}
	}
}
