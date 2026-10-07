// Package config loads everything Natlas is told from outside the code:
//
//   - Settings: secrets and runtime switches from the environment (.env)
//   - App:      natlas.yml - app title, currency, data folder, enabled plugins, shared lists
//   - Plugin:   plugins/<id>/plugin.yml - one per tab: file, fields, list layout, actions
//
// Everything is validated once at startup. A mistake in any YAML file stops
// the app with a message naming the exact file and key, instead of failing
// later on a random request.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── Settings (.env) ───────────────────────────────────────────────────────

// Settings are read from environment variables. docker compose loads them
// from .env, so no secret ever lives in a YAML file or in git.
type Settings struct {
	Username     string
	Password     string // plain password, or ...
	PasswordHash string // ... a PBKDF2 hash from `natlas hash-password` (preferred)
	SecretKey    string
	Addr         string // listen address, default ":8080"
	ConfigFile   string // natlas.yml
	PluginsDir   string // folder holding plugins/<id>/plugin.yml
	WebDir       string // optional: serve web/ from disk (live edits while developing)
	CookieSecure bool   // false only for plain-http local testing
	TrustProxy   bool   // read the client IP from proxy headers (true behind Caddy/Cloudflare)
}

// LoadSettings reads Settings from the environment and reports every missing
// required value at once.
func LoadSettings() (*Settings, error) {
	s := &Settings{
		Username:     strings.TrimSpace(os.Getenv("NATLAS_USERNAME")),
		Password:     os.Getenv("NATLAS_PASSWORD"),
		PasswordHash: strings.TrimSpace(os.Getenv("NATLAS_PASSWORD_HASH")),
		SecretKey:    strings.TrimSpace(os.Getenv("NATLAS_SECRET_KEY")),
		Addr:         envOr("NATLAS_ADDR", ":8080"),
		ConfigFile:   envOr("NATLAS_CONFIG", "natlas.yml"),
		PluginsDir:   envOr("NATLAS_PLUGINS_DIR", "plugins"),
		WebDir:       os.Getenv("NATLAS_WEB_DIR"),
		CookieSecure: envBool("NATLAS_COOKIE_SECURE", true),
		TrustProxy:   envBool("NATLAS_TRUST_PROXY", true),
	}
	var missing []string
	if s.Username == "" {
		missing = append(missing, "NATLAS_USERNAME")
	}
	if s.Password == "" && s.PasswordHash == "" {
		missing = append(missing, "NATLAS_PASSWORD_HASH (or NATLAS_PASSWORD)")
	}
	if len(s.SecretKey) < 32 {
		missing = append(missing, "NATLAS_SECRET_KEY (at least 32 characters; generate one with: openssl rand -hex 32)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing settings in .env: %s", strings.Join(missing, ", "))
	}
	return s, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

// ── App (natlas.yml) ──────────────────────────────────────────────────────

// App is natlas.yml.
type App struct {
	Title    string              `yaml:"title" json:"title"`
	Currency string              `yaml:"currency" json:"currency"`
	Locale   string              `yaml:"locale" json:"locale"`
	DataDir  string              `yaml:"data_dir" json:"-"`
	Plugins  []PluginRef         `yaml:"plugins" json:"-"`
	Lists    map[string][]string `yaml:"lists" json:"-"`

	// Loaded is filled by Load: enabled plugins, in nav order.
	Loaded []*Plugin `yaml:"-" json:"plugins"`
}

// PluginRef is one entry under `plugins:` in natlas.yml. It is either a bare
// id ("events") or a mapping that overrides the plugin's data file or title.
type PluginRef struct {
	ID    string `yaml:"id"`
	File  string `yaml:"file"`
	Title string `yaml:"title"`
}

// UnmarshalYAML accepts both `- events` and `- {id: events, file: x.md}`.
func (p *PluginRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		p.ID = n.Value
		return nil
	}
	type plain PluginRef
	return n.Decode((*plain)(p))
}

// Load reads natlas.yml and every enabled plugin, then validates the lot.
func Load(s *Settings) (*App, error) {
	app := &App{Title: "Natlas", Currency: "₹", Locale: "en-IN", DataDir: "/data"}
	if err := decodeFile(s.ConfigFile, app); err != nil {
		return nil, err
	}
	if len(app.Plugins) == 0 {
		return nil, fmt.Errorf("%s: `plugins:` is empty - list at least one plugin id", s.ConfigFile)
	}

	var errs []error
	seen := map[string]bool{}
	for _, ref := range app.Plugins {
		if seen[ref.ID] {
			errs = append(errs, fmt.Errorf("%s: plugin %q is listed twice", s.ConfigFile, ref.ID))
			continue
		}
		seen[ref.ID] = true
		path := filepath.Join(s.PluginsDir, ref.ID, "plugin.yml")
		p := &Plugin{}
		if err := decodeFile(path, p); err != nil {
			errs = append(errs, err)
			continue
		}
		p.ID = ref.ID
		if ref.File != "" {
			p.File = ref.File
		}
		if ref.Title != "" {
			p.Title = ref.Title
		}
		if !filepath.IsAbs(p.File) {
			p.File = filepath.Join(app.DataDir, p.File)
		}
		if err := p.prepare(app); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		app.Loaded = append(app.Loaded, p)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return app, nil
}

// Plugin returns a loaded plugin by id, or nil.
func (a *App) Plugin(id string) *Plugin {
	for _, p := range a.Loaded {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// decodeFile strictly decodes a YAML file: unknown keys are errors, so a
// typo like `requird: true` is caught at startup instead of silently ignored.
func decodeFile(path string, into any) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
