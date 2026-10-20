package merge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"natlas/internal/config"
)

func setup(t *testing.T) (*config.App, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "natlas.yml")
	os.WriteFile(cfg, []byte("data_dir: "+dir+"\nplugins: [events, calendar, architecture]\n"), 0o644)
	app, err := config.Load(&config.Settings{ConfigFile: cfg, PluginsDir: "../../plugins"})
	if err != nil {
		t.Fatal(err)
	}
	return app, dir
}

const kept = `events:
  - id: a
    title: Alpha
    date: '2026-10-20'
    status: open
  - id: b
    title: Beta
    date: '2026-10-21'
    status: open
`

const other = `events:
  - id: a
    title: Alpha
    date: '2026-10-20'
    status: closed
  - id: b
    title: Beta
    date: '2026-10-21'
    status: open
  - id: c
    title: Gamma (added on the phone)
    date: '2026-10-22'
    status: open
    anchor_day: 31
`

func TestScanMergesAndQueues(t *testing.T) {
	app, dir := setup(t)
	os.WriteFile(filepath.Join(dir, "event.md"), []byte(kept), 0o644)
	cp := filepath.Join(dir, "event.sync-conflict-20261019-010203-ABCDEFG.md")
	os.WriteFile(cp, []byte(other), 0o644)
	os.WriteFile(filepath.Join(dir, "calendar.sync-conflict-20261019-010203-ABCDEFG.md"), []byte("x: [\n"), 0o644) // read-only file: ignored

	if n := len(Copies(app)); n != 1 {
		t.Fatalf("copies: %d", n)
	}
	rep, err := Scan(app, time.Date(2026, 10, 19, 1, 5, 0, 0, time.UTC))
	if err != nil || rep.Copies != 1 || rep.Added != 1 || rep.Pending != 1 {
		t.Fatalf("scan: %+v %v", rep, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "event.md"))
	if !strings.Contains(string(b), "Gamma") || !strings.Contains(string(b), "anchor_day: 31") || strings.Contains(string(b), "closed") {
		t.Fatalf("event.md:\n%s", b)
	}
	if _, err := os.Stat(cp); !os.IsNotExist(err) {
		t.Fatal("copy not removed")
	}
	list, _ := List(app)
	if len(list) != 1 || list[0].Record != "a" || list[0].Other["status"] != "closed" || list[0].Device != "ABCDEFG" {
		t.Fatalf("list: %+v", list)
	}
	// scanning again finds nothing new
	if rep, _ := Scan(app, time.Now()); rep.Copies != 0 {
		t.Fatalf("second scan: %+v", rep)
	}
	// take the other device's version
	if err := Resolve(app, list[0].ID, "other"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "event.md"))
	if !strings.Contains(string(b), "status: closed") {
		t.Fatalf("resolve other:\n%s", b)
	}
	if list, _ := List(app); len(list) != 0 {
		t.Fatalf("list after resolve: %+v", list)
	}
	if _, err := os.Stat(filepath.Join(dir, ".natlas", "conflicts.yml")); !os.IsNotExist(err) {
		t.Fatal("empty list file should be removed")
	}
}

func TestBrokenCopyIsKept(t *testing.T) {
	app, dir := setup(t)
	os.WriteFile(filepath.Join(dir, "event.md"), []byte(kept), 0o644)
	cp := filepath.Join(dir, "event.sync-conflict-20261019-010203-ABCDEFG.md")
	os.WriteFile(cp, []byte("events: [\n"), 0o644)
	if _, err := Scan(app, time.Now()); err == nil {
		t.Fatal("broken copy must report an error")
	}
	if _, err := os.Stat(cp); err != nil {
		t.Fatal("broken copy must stay")
	}
}

func TestMarkdownSections(t *testing.T) {
	app, dir := setup(t)
	os.WriteFile(filepath.Join(dir, "homelab.md"), []byte("## A\n\none\n\n## B\n\ntwo\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "homelab.sync-conflict-20261019-010203-ZZZZZZZ.md"), []byte("## A\n\nONE\n\n## B\n\ntwo\n\n## C\n\nthree\n"), 0o644)
	rep, err := Scan(app, time.Now())
	if err != nil || rep.Added != 1 || rep.Pending != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "homelab.md"))
	if !strings.Contains(string(b), "## C\n\nthree") || !strings.Contains(string(b), "\none\n") {
		t.Fatalf("homelab.md:\n%s", b)
	}
}
