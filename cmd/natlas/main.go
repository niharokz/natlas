// Command natlas serves the Natlas web app.
//
//	natlas                 run the server
//	natlas check           validate .env, natlas.yml and every plugin.yml, then exit
//	natlas hash-password   read a password on stdin, print NATLAS_PASSWORD_HASH
//	natlas health          exit 0 if the local server answers (Docker HEALTHCHECK)
//	natlas cli             one JSON request on stdin, JSON answer on stdout (phone widgets)
//	natlas version         print the version
//
// Configuration comes from the environment (.env) and natlas.yml; plugins
// from plugins/<id>/plugin.yml. See README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	_ "time/tzdata" // the container image has no zoneinfo; TZ=Asia/Kolkata still works

	"natlas/examples"
	"natlas/internal/api"
	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/internal/merge"
	"natlas/internal/store"
	"natlas/web"
)

// version is set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("natlas", version)
			return
		case "check":
			if _, err := load(); err != nil {
				log.Fatalf("config check failed:\n%v", err)
			}
			log.Println("config OK")
			return
		case "hash-password":
			hashPassword()
			return
		case "health":
			health()
			return
		case "cli":
			cli()
			return
		case "bans", "unban":
			bans(os.Args[1], os.Args[2:])
			return
		default:
			log.Fatalf("unknown command %q (commands: check, cli, hash-password, health, version, bans, unban <ip|all>)", os.Args[1])
		}
	}
	serve()
}

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
	// A missing data file just shows as an empty list, so say so loudly here:
	// it almost always means NATLAS_DATA_DIR points at the wrong folder.
	for _, p := range app.app.Loaded {
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

	authn := auth.New(s)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte("ok\n"))
	})
	(&api.Server{App: app.app, Auth: authn, Main: s.Main}).Register(mux)
	if s.Main {
		go mergeLoop(app.app) // fold Syncthing conflict copies back in
	}
	site, err := web.Handler(s.WebDir, app.app, version)
	if err != nil {
		log.Fatalf("cannot load web files: %v", err)
	}
	mux.Handle("/", site)

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           recoverer(accessLog(authn, authn.Guard(securityHeaders(s.CookieSecure, mux)))),
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

// cli answers one request from the phone app's widgets without a server:
// {"op":"snapshot"} | {"op":"action",plugin,coll,id,rev,action} | {"op":"patch",…,values} | {"op":"quick",plugin,coll,text} | {"op":"merge"}.
// Exit code 0 even for a refused request; the JSON carries "error" and "status".
func cli() {
	out := json.NewEncoder(os.Stdout)
	var req map[string]any
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&req); err != nil {
		out.Encode(map[string]any{"status": 400, "error": "bad request: " + err.Error()})
		os.Exit(1)
	}
	if req["op"] == "demo" { // {"op":"demo","dir":…}: copy the sample data into an empty folder
		dir, _ := req["dir"].(string)
		n, err := writeDemo(dir)
		if err != nil {
			out.Encode(map[string]any{"status": 500, "error": err.Error()})
			os.Exit(1)
		}
		out.Encode(map[string]any{"status": 200, "written": n})
		return
	}
	app, err := load()
	if err != nil {
		out.Encode(map[string]any{"status": 500, "error": err.Error()})
		os.Exit(1)
	}
	store.LockDir = app.settings.LockDir
	srv := &api.Server{App: app.app, Auth: auth.New(app.settings), Main: app.settings.Main}
	code, res := srv.Exec(req)
	res["status"] = code
	out.Encode(res)
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

type loaded struct {
	settings *config.Settings
	app      *config.App
}

func load() (*loaded, error) {
	s, err := config.LoadSettings()
	if err != nil {
		return nil, err
	}
	if s.PasswordHash != "" {
		if err := auth.ValidHash(s.PasswordHash); err != nil {
			return nil, fmt.Errorf("NATLAS_PASSWORD_HASH: %w", err)
		}
	}
	app, err := config.Load(s)
	if err != nil {
		return nil, err
	}
	return &loaded{s, app}, nil
}

// hashPassword reads one line from stdin and prints the hash for .env.
// Keep the password out of your shell history:
//
//	read -rs PW && printf '%s' "$PW" | docker compose run --rm -T natlas hash-password
//
// bans lists the banned IPs; unban <ip|all> lifts bans. The running server
// picks the change up at once (it re-reads bans.yml when it changes).
func bans(cmd string, args []string) {
	dir := os.Getenv("NATLAS_STATE_DIR")
	if dir == "" {
		dir = filepath.Dir(os.Getenv("NATLAS_CONFIG"))
		if os.Getenv("NATLAS_CONFIG") == "" {
			dir = "."
		}
	}
	b := auth.OpenBans(filepath.Join(dir, "bans.yml"), 7, 5)
	if cmd == "unban" {
		if len(args) != 1 {
			log.Fatal("usage: natlas unban <ip|all>")
		}
		n, err := b.Unban(args[0])
		if err != nil {
			log.Fatalf("cannot save: %v", err)
		}
		fmt.Printf("lifted %d ban(s)\n", n)
		return
	}
	list := b.List()
	if len(list) == 0 {
		fmt.Println("no banned addresses")
		return
	}
	for _, x := range list {
		fmt.Printf("%-40s until %s  (since %s, %s)\n", x.IP, x.Until.Format("2006-01-02 15:04"), x.Since.Format("2006-01-02 15:04"), x.Reason)
	}
}

func hashPassword() {
	fmt.Fprintln(os.Stderr, "Password (then Enter):")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		log.Fatal("no password given on stdin")
	}
	pw := strings.TrimRight(line, "\r\n")
	if len(pw) < 12 {
		fmt.Fprintln(os.Stderr, "warning: shorter than 12 characters - a longer passphrase is much safer")
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("NATLAS_PASSWORD_HASH=" + h)
	fmt.Fprintln(os.Stderr, "Put that line in .env, remove NATLAS_PASSWORD, then: docker compose up -d")
}

// health is the Docker HEALTHCHECK (the image has no curl).
func health() {
	addr := os.Getenv("NATLAS_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	res, err := (&http.Client{Timeout: 4 * time.Second}).Get("http://" + addr + "/healthz")
	if err != nil {
		log.Fatalf("unhealthy: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Fatalf("unhealthy: HTTP %d", res.StatusCode)
	}
}

// securityHeaders adds browser protections to every response. Alpine.js
// needs 'unsafe-eval' (it compiles x-* expressions); everything else is
// same-origin only, and the page cannot be framed.
func securityHeaders(https bool, next http.Handler) http.Handler {
	csp := "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; connect-src 'self'; manifest-src 'self'; worker-src 'self'; " +
		"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// statusWriter remembers the response status for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// accessLog records every change (and every failed request) with the client
// IP - never request bodies, cookies or query strings.
func accessLog(a *auth.Auth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead || sw.status >= 400 && sw.status != 401 && sw.status != 404 {
			log.Printf("%s %s %s %d %s", a.ClientIP(r), r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

// recoverer turns a panic in one request into a logged 500 instead of a
// dropped connection.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil && v != http.ErrAbortHandler {
				log.Printf("panic in %s %s: %v", r.Method, r.URL.Path, v)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
