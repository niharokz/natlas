package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/internal/dates"
)

// newPhone loads the real plugins in phone (local) mode over a copy of examples/data.
func newPhone(t *testing.T) *Server {
	t.Helper()
	root, _ := filepath.Abs("../..")
	data := t.TempDir()
	entries, _ := os.ReadDir(filepath.Join(root, "examples", "data"))
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(root, "examples", "data", e.Name()))
		os.WriteFile(filepath.Join(data, e.Name()), b, 0o644)
	}
	s := &config.Settings{Local: true, LocalToken: strings.Repeat("t", 32), SecretKey: strings.Repeat("k", 32),
		DataDir: data, PluginList: "events,subscriptions,calendar,health,inventory,architecture",
		PluginsDir: filepath.Join(root, "plugins")}
	app, err := config.Load(s)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{App: app, Auth: auth.New(s)}
}

func TestSnapshot(t *testing.T) {
	s := newPhone(t)
	snap := s.Snapshot(time.Now())
	if len(snap["plugins"].([]map[string]any)) != 6 {
		t.Fatalf("plugins: %v", snap["plugins"])
	}
	var events map[string]any
	for _, l := range snap["lists"].([]map[string]any) {
		if l["plugin"] == "events" {
			events = l
		}
		if l["plugin"] == "architecture" || l["plugin"] == "calendar" {
			t.Fatalf("%v must not be a list widget source", l["plugin"])
		}
	}
	if events == nil || events["quick"] != true {
		t.Fatalf("events list: %v", events)
	}
	found := false
	for _, g := range events["groups"].([]map[string]any) {
		for _, it := range g["items"].([]map[string]any) {
			if it["id"] == "call-bank" {
				found = it["action"].(map[string]any)["id"] == "done"
			}
			if it["id"] == "parcel-headphones" {
				t.Fatal("closed items stay out of the widget")
			}
		}
	}
	if !found {
		t.Fatal("call-bank with a Done action missing")
	}
	if len(snap["calendar"].([]map[string]any)) == 0 || snap["today"] != dates.Format(dates.Today(), "") {
		t.Fatalf("calendar/today: %v %v", snap["calendar"], snap["today"])
	}
}

func TestExecTickUndoQuick(t *testing.T) {
	s := newPhone(t)
	_, list := s.call(s.list, "GET", map[string]string{"plugin": "events", "coll": "events"}, nil)
	rev := find(list["records"].([]any), "call-bank")["_rev"].(string)

	code, out := s.Exec(map[string]any{"op": "action", "plugin": "events", "coll": "events", "id": "call-bank", "action": "done", "rev": rev})
	if code != 200 || out["record"].(map[string]any)["status"] != "closed" {
		t.Fatalf("tick: %d %v", code, out)
	}
	closedRev := out["record"].(map[string]any)["_rev"]
	undo := out["undo"].(map[string]any)
	if undo["status"] != "open" {
		t.Fatalf("undo values: %v", undo)
	}
	code, out = s.Exec(map[string]any{"op": "patch", "plugin": "events", "coll": "events", "id": "call-bank",
		"rev": out["record"].(map[string]any)["_rev"], "values": undo})
	if code != 200 || out["record"].(map[string]any)["status"] != "open" {
		t.Fatalf("undo: %d %v", code, out)
	}
	// revs hash the content: after Undo the original rev is current again, the closed one is stale
	if code, _ := s.Exec(map[string]any{"op": "action", "plugin": "events", "coll": "events", "id": "call-bank", "action": "done", "rev": closedRev}); code != 409 {
		t.Fatalf("stale tick: %d", code)
	}
	code, out = s.Exec(map[string]any{"op": "quick", "plugin": "events", "coll": "events", "text": "Gym tomorrow"})
	rec, _ := out["record"].(map[string]any)
	if code != 201 || rec["title"] != "Gym" || rec["date"] != dates.Format(dates.Today().AddDate(0, 0, 1), "") {
		t.Fatalf("quick: %d %v", code, out)
	}
	code, out = s.Exec(map[string]any{"op": "quick", "plugin": "calendar", "coll": "mine", "text": "Dentist tomorrow 7am"})
	rec, _ = out["record"].(map[string]any)
	if code != 201 || rec["title"] != "Dentist" || rec["time"] != "07:00" {
		t.Fatalf("calendar quick: %d %v", code, out)
	}
	if code, _ := s.Exec(map[string]any{"op": "nope"}); code != 400 {
		t.Fatalf("unknown op: %d", code)
	}
	if code, out := s.Exec(map[string]any{"op": "merge"}); code != 200 || out["copies"] == nil {
		t.Fatalf("merge: %d %v", code, out)
	}
}

func TestFormat(t *testing.T) {
	if group(12345678, true) != "1,23,45,678" || group(12345678, false) != "12,345,678" {
		t.Fatal("grouping")
	}
	if fmtClock("20:30") != "8:30 pm" || fmtClock("00:05") != "12:05 am" {
		t.Fatal("clock")
	}
	if relDays(3) != "in 3 days" || relDays(-2) != "2 days ago" {
		t.Fatal("relDays")
	}
}
