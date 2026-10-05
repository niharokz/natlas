// Package web serves the browser app: index.html, CSS, JS, icons, the
// service worker and a manifest generated from the enabled plugins.
//
// Files are built into the binary (go:embed). Set NATLAS_WEB_DIR=web to serve
// them from disk instead while editing the UI.
package web

import (
	"bytes"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"natlas/internal/config"
)

//go:embed index.html app.js natlas.css sw.js vendor icons
var files embed.FS

// Handler returns the static-site handler. Every asset URL carries a version
// (a hash of the files), so browsers and the service worker pick up a new
// release immediately after an update.
func Handler(dir string, app *config.App) (http.Handler, error) {
	var root fs.FS = files
	if dir != "" {
		root = os.DirFS(dir)
	}
	version, err := hashFS(root)
	if err != nil {
		return nil, err
	}
	manifest := buildManifest(app)
	mux := http.NewServeMux()

	page := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			b, err := fs.ReadFile(root, name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			b = bytes.ReplaceAll(b, []byte("{{VERSION}}"), []byte(version))
			b = bytes.ReplaceAll(b, []byte("{{TITLE}}"), []byte(app.Title))
			w.Header().Set("Cache-Control", "no-cache")
			if strings.HasSuffix(name, ".js") {
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			}
			w.Write(b)
		}
	}
	mux.HandleFunc("GET /{$}", page("index.html"))
	mux.HandleFunc("GET /sw.js", page("sw.js")) // at the root so it can control the whole app
	mux.HandleFunc("GET /manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(manifest)
	})
	assets := http.StripPrefix("/assets/", http.FileServerFS(root))
	mux.Handle("GET /assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		assets.ServeHTTP(w, r)
	}))
	return mux, nil
}

// hashFS fingerprints every file so a new release changes the version.
func hashFS(root fs.FS) (string, error) {
	h := sha1.New()
	var names []string
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := fs.ReadFile(root, n)
		if err != nil {
			return "", err
		}
		h.Write([]byte(path.Clean(n)))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:10], nil
}

// buildManifest makes the PWA manifest, with a long-press shortcut per plugin.
func buildManifest(app *config.App) []byte {
	type icon struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose,omitempty"`
	}
	type shortcut struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Icons []icon `json:"icons"`
	}
	m := map[string]any{
		"name": app.Title, "short_name": app.Title, "start_url": "/", "scope": "/",
		"display": "standalone", "background_color": "#070b0f", "theme_color": "#070b0f",
		"icons": []icon{
			{Src: "/assets/icons/icon-192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
			{Src: "/assets/icons/icon-512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
			{Src: "/assets/icons/maskable-512.png", Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
		},
	}
	var sc []shortcut
	for _, p := range app.Loaded {
		if len(sc) == 4 { // Android shows at most four
			break
		}
		sc = append(sc, shortcut{Name: p.Title, URL: "/#/p/" + p.ID,
			Icons: []icon{{Src: "/assets/icons/icon-192.png", Sizes: "192x192", Type: "image/png"}}})
	}
	m["shortcuts"] = sc
	b, _ := json.MarshalIndent(m, "", "  ")
	return b
}
