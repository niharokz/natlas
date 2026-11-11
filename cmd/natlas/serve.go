// serve.go — the HTTP server: routes, demo login, conflict merging, shutdown.

package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"natlas/examples"
	"natlas/internal/api"
	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/internal/merge"
	"natlas/internal/store"
	"natlas/web"
)

// serve runs the web server until SIGINT/SIGTERM.
func serve() {
	app, err := load()
	if err != nil {
		log.Fatalf("cannot start:\n%v", err)
	}
	s := app.settings
	store.LockDir = s.LockDir
	if s.Local {
		log.Printf("phone mode: data %s, merging conflicts: %v", app.app.DataDir, s.Main)
	} else if s.PasswordHash == "" {
		log.Println("tip: store a hash instead of the plain password - see `natlas hash-password` in the README")
	}
	if !s.CookieSecure && !s.Local {
		log.Println("WARNING: NATLAS_COOKIE_SECURE=false - only use this for plain-http testing")
	}
	warnMissingFiles(app.app)

	authn := auth.New(s)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte("ok\n"))
	})
	(&api.Server{App: app.app, Auth: authn, Main: s.Main, LayoutFile: filepath.Join(s.StateDir, "layout.yml")}).Register(mux)
	var handler http.Handler = mux
	if s.Demo() {
		demo, err := demoMux(s, authn)
		if err != nil {
			log.Fatalf("demo login: %v", err)
		}
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") && authn.IsDemo(r) {
				demo.ServeHTTP(w, r) // the demo login only ever sees the sample data
				return
			}
			mux.ServeHTTP(w, r)
		})
		log.Printf("demo login %q: read-only sample data in %s", s.DemoUsername, filepath.Join(s.StateDir, "demo"))
	}
	if s.Main {
		go mergeLoop(app.app) // fold Syncthing conflict copies back in
	}
	demo := web.Demo{}
	if s.Demo() {
		demo = web.Demo{User: s.DemoUsername, Password: s.DemoPassword}
	}
	site, err := web.Handler(s.WebDir, app.app, version, demo)
	if err != nil {
		log.Fatalf("cannot load web files: %v", err)
	}
	mux.Handle("/", site)

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           recoverer(accessLog(authn, authn.Guard(securityHeaders(s.CookieSecure, handler)))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		ids := make([]string, 0, len(app.app.Loaded))
		for _, p := range app.app.Loaded {
			ids = append(ids, p.ID)
		}
		log.Printf("natlas %s listening on %s - plugins: %v - today is %s (%s)", version, s.Addr, ids,
			time.Now().Format("2006-01-02"), time.Local)
		if !s.Local {
			remember := "off (NATLAS_DEVICE_KEY not set: every login lasts " + strconv.Itoa(s.SessionHours) + " h)"
			if s.DeviceKey != "" {
				remember = "on (browsers given the device key stay signed in until logout; others " + strconv.Itoa(s.SessionHours) + " h)"
			}
			log.Printf("login: %d failures ban a public IP for %d days (%s) · remember device: %s",
				s.MaxFailures, s.BanDays, filepath.Join(s.StateDir, "bans.yml"), remember)
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx) // let in-flight saves finish
	log.Println("natlas stopped")
}

// warnMissingFiles logs data files that do not exist. A missing file just shows
// as an empty list, so say so loudly: it almost always means NATLAS_DATA_DIR is wrong.
func warnMissingFiles(app *config.App) {
	for _, p := range app.Loaded {
		checked := map[string]bool{}
		for _, c := range p.Collections {
			path := p.FileOf(c)
			if checked[path] {
				continue
			}
			checked[path] = true
			if _, err := os.Stat(path); err == nil {
				continue
			}
			if c.ReadOnly {
				log.Printf("WARNING %s: data file %s not found - check NATLAS_DATA_DIR in .env and data_dir in natlas.yml", p.ID, path)
			} else {
				log.Printf("note %s: %s does not exist yet - it is created on the first save (if you expected data, check NATLAS_DATA_DIR)", p.ID, path)
			}
		}
	}
}

// demoMux serves the API for the demo login: the same plugins over a fresh
// copy of the sample data (<state dir>/demo), read-only, default layout.
func demoMux(s *config.Settings, authn *auth.Auth) (http.Handler, error) {
	dir := filepath.Join(s.StateDir, "demo")
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if _, err := writeDemo(dir); err != nil {
		return nil, err
	}
	ds := *s
	ds.DataDir = dir
	app, err := config.Load(&ds)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	(&api.Server{App: app, Auth: authn, ReadOnly: true}).Register(mux)
	return mux, nil
}

// mergeLoop merges Syncthing conflict copies every 15 s (main device only).
func mergeLoop(app *config.App) {
	for {
		rep, err := merge.Scan(app, time.Now())
		if err != nil {
			log.Printf("[merge] %v", err)
		}
		if rep.Copies > 0 {
			log.Printf("[merge] %d conflict cop(ies) merged: %d record(s) added back, %d waiting for a decision", rep.Copies, rep.Added, rep.Pending)
		}
		time.Sleep(15 * time.Second)
	}
}

// writeDemo copies the built-in sample files into dir, never over an existing file.
func writeDemo(dir string) (int, error) {
	if dir == "" {
		return 0, errors.New("dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	entries, err := fs.ReadDir(examples.Data, "data")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, _ := fs.ReadFile(examples.Data, "data/"+e.Name())
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
