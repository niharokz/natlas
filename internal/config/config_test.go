package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShippedPluginsAreValid(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "natlas.yml")
	os.WriteFile(cfg, []byte("plugins: [events, subscriptions, birthdays, health, inventory]\n"), 0o644)
	app, err := Load(&Settings{ConfigFile: cfg, PluginsDir: "../../plugins"})
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Loaded) != 5 || app.Plugin("health").Collection("profile").Kind != "record" {
		t.Fatal("plugins not loaded as expected")
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
