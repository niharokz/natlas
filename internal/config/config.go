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
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"natlas/plugins"
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

	// Phone app (NATLAS_MODE=local): no login, a per-launch token instead.
	Local      bool
	LocalToken string // NATLAS_TOKEN: the app opens /local?t=<token> once
	Main       bool   // NATLAS_MAIN: this copy merges Syncthing conflict copies (server: yes, phone: only if chosen)
	LockDir    string // NATLAS_LOCK_DIR: lock files shared by every natlas process on this device
	DataDir    string // NATLAS_DATA_DIR: overrides data_dir in natlas.yml (the phone's folder)
	PluginList string // NATLAS_PLUGINS: comma list, overrides plugins: in natlas.yml

	// Login hardening and sessions
	StateDir     string // NATLAS_STATE_DIR: writable folder for bans.yml (default: the config file's folder)
	BanDays      int    // NATLAS_BAN_DAYS: days a public IP is banned after MaxFailures (default 7)
	MaxFailures  int    // NATLAS_MAX_FAILURES: failed logins within a day before the ban (default 5)
	SessionHours int    // NATLAS_SESSION_HOURS: life of a login without the device key (default 12)
	DeviceKey    string // NATLAS_DEVICE_KEY: entered once per browser, keeps it signed in until logout
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
		Local:        strings.EqualFold(os.Getenv("NATLAS_MODE"), "local"),
		LocalToken:   strings.TrimSpace(os.Getenv("NATLAS_TOKEN")),
		LockDir:      os.Getenv("NATLAS_LOCK_DIR"),
		DataDir:      os.Getenv("NATLAS_DATA_DIR"),
		PluginList:   os.Getenv("NATLAS_PLUGINS"),
		StateDir:     os.Getenv("NATLAS_STATE_DIR"),
		BanDays:      envInt("NATLAS_BAN_DAYS", 7),
		MaxFailures:  envInt("NATLAS_MAX_FAILURES", 5),
		SessionHours: envInt("NATLAS_SESSION_HOURS", 12),
		DeviceKey:    strings.TrimSpace(os.Getenv("NATLAS_DEVICE_KEY")),
	}
	if s.StateDir == "" {
		s.StateDir = filepath.Dir(s.ConfigFile)
	}
	if s.Local { // the phone app: loopback only, no password, no proxy
		s.Main = envBool("NATLAS_MAIN", false)
		s.CookieSecure, s.TrustProxy = false, false
		if len(s.LocalToken) < 32 {
			return nil, fmt.Errorf("NATLAS_MODE=local needs NATLAS_TOKEN (at least 32 characters)")
		}
		return s, nil
	}
	s.Main = envBool("NATLAS_MAIN", true)
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
	if s.DeviceKey != "" && len(s.DeviceKey) < 32 {
		missing = append(missing, "NATLAS_DEVICE_KEY must be at least 32 characters (openssl rand -hex 32), or empty")
	}
	if s.BanDays < 1 || s.MaxFailures < 1 || s.SessionHours < 1 {
		missing = append(missing, "NATLAS_BAN_DAYS, NATLAS_MAX_FAILURES and NATLAS_SESSION_HOURS must be 1 or more")
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

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
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
	if _, err := os.Stat(s.ConfigFile); err == nil || !s.Local { // the phone may run without natlas.yml
		if err := decodeFile(s.ConfigFile, app); err != nil {
			return nil, err
		}
	}
	if s.DataDir != "" {
		app.DataDir = s.DataDir
	}
	if strings.TrimSpace(s.PluginList) != "" {
		app.Plugins = nil
		for _, id := range strings.Split(s.PluginList, ",") {
			if id = strings.TrimSpace(id); id != "" {
				app.Plugins = append(app.Plugins, PluginRef{ID: id})
			}
		}
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
		path, data, err := pluginFile(s.PluginsDir, ref.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		p := &Plugin{}
		if err := decodeBytes(path, data, p); err != nil {
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
		for _, c := range p.Collections {
			if c.File != "" && !filepath.IsAbs(c.File) {
				c.File = filepath.Join(app.DataDir, c.File)
			}
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
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	return decodeBytes(path, data, into)
}

func decodeBytes(name string, data []byte, into any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// pluginFile reads plugins/<id>/plugin.yml from disk, or from the copy built
// into the binary when there is no plugins folder (the phone app).
func pluginFile(dir, id string) (string, []byte, error) {
	if strings.ContainsAny(id, `/\.`) || id == "" {
		return id, nil, fmt.Errorf("plugin id %q: use letters, digits, - and _ only", id)
	}
	path := filepath.Join(dir, id, "plugin.yml")
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return path, nil, fmt.Errorf("cannot read %s: %w", path, err)
		}
		return path, data, nil
	}
	data, err := fs.ReadFile(plugins.FS, id+"/plugin.yml")
	if err != nil {
		return path, nil, fmt.Errorf("no plugin %q (not on disk and not built in)", id)
	}
	return "built-in " + id + "/plugin.yml", data, nil
}
