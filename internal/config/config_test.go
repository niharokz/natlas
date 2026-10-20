package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShippedPluginsAreValid(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "natlas.yml")
	os.WriteFile(cfg, []byte("plugins: [events, subscriptions, calendar, health, inventory, architecture]\n"), 0o644)
	app, err := Load(&Settings{ConfigFile: cfg, PluginsDir: "../../plugins"})
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Loaded) != 6 || app.Plugin("health").Collection("profile").Kind != "record" {
		t.Fatal("plugins not loaded as expected")
	}
	cal := app.Plugin("calendar")
	if cal.View != "calendar" || cal.FileOf(cal.Collection("google")) != filepath.Join("/data", "calendar.md") ||
		cal.FileOf(cal.Collection("mine")) != filepath.Join("/data", "calendar-local.md") || app.Plugin("events").View != "list" {
		t.Fatalf("calendar files/view not resolved: %+v", cal)
	}
}

func TestOwnFileOnlyForReadOnly(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cal"), 0o755)
	os.WriteFile(filepath.Join(dir, "cal", "plugin.yml"), []byte(`
file: a.md
view: calendar
collections:
  - id: mine
    path: events
    fields: [ { key: title } ]
  - id: synced
    file: b.md
    path: events
    fields: [ { key: title }, { key: date, type: date } ]
`), 0o644)
	cfg := filepath.Join(dir, "natlas.yml")
	os.WriteFile(cfg, []byte("plugins: [cal]\n"), 0o644)
	_, err := Load(&Settings{ConfigFile: cfg, PluginsDir: dir})
	for _, want := range []string{"only allowed on a readonly collection", `collection "mine": a calendar view needs a "date" field`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestMistakesAreReportedTogether(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bad"), 0o755)
	os.WriteFile(filepath.Join(dir, "bad", "plugin.yml"), []byte(`
file: bad.md
collections:
  - id: things
    path: things
    fields:
      - { key: name, type: txt }
      - { key: kind, type: select }
    list: { title: nam }
    actions:
      - { id: x, label: X, set: { colour: red } }
`), 0o644)
	cfg := filepath.Join(dir, "natlas.yml")
	os.WriteFile(cfg, []byte("plugins: [bad]\n"), 0o644)
	_, err := Load(&Settings{ConfigFile: cfg, PluginsDir: dir})
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{`unknown type "txt"`, "select fields need `options:`", `"nam" is not a field`, `set "colour" is not a field`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestTypoInKeyIsCaught(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "natlas.yml")
	os.WriteFile(cfg, []byte("plugin: [events]\n"), 0o644)
	if _, err := Load(&Settings{ConfigFile: cfg, PluginsDir: dir}); err == nil || !strings.Contains(err.Error(), "plugin") {
		t.Fatalf("unknown key must fail: %v", err)
	}
}

func TestMarkdownPluginRules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "doc"), 0o755)
	os.WriteFile(filepath.Join(dir, "doc", "plugin.yml"), []byte(`
file: note.md
view: document
collections:
  - id: s
    path: sections
    fields: [ { key: title }, { key: owner } ]
  - id: x
    path: other
    fields: [ { key: title } ]
`), 0o644)
	cfg := filepath.Join(dir, "natlas.yml")
	os.WriteFile(cfg, []byte("plugins: [doc]\n"), 0o644)
	_, err := Load(&Settings{ConfigFile: cfg, PluginsDir: dir})
	for _, want := range []string{"view: document needs format: markdown"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
	os.WriteFile(filepath.Join(dir, "doc", "plugin.yml"), []byte(`
file: note.md
view: document
format: markdown
collections:
  - id: s
    path: sections
    fields: [ { key: title }, { key: owner } ]
  - id: x
    path: other
    fields: [ { key: title } ]
`), 0o644)
	_, err = Load(&Settings{ConfigFile: cfg, PluginsDir: dir})
	for _, want := range []string{`only have ` + "`title` and `body`" + `, not "owner"`, `collection "x": in a Markdown note use`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}
