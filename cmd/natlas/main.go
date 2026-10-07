// Command natlas serves the Natlas web app.
//
//	natlas                 run the server
//	natlas check           validate .env, natlas.yml and every plugin.yml, then exit
//	natlas hash-password   read a password on stdin, print NATLAS_PASSWORD_HASH
//	natlas health          exit 0 if the local server answers (Docker HEALTHCHECK)
//	natlas version         print the version
//
// Configuration comes from the environment (.env) and natlas.yml; plugins
// from plugins/<id>/plugin.yml. See README.md.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the container image has no zoneinfo; TZ=Asia/Kolkata still works

	"natlas/internal/api"
	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/web"
)

// version is set at build time: -ldflags "-X main.version=2.1.0".
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
		default:
			log.Fatalf("unknown command %q (commands: check, hash-password, health, version)", os.Args[1])
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
	if s.PasswordHash == "" {
		log.Println("tip: store a hash instead of the plain password - see `natlas hash-password` in the README")
	}
	if !s.CookieSecure {
		log.Println("WARNING: NATLAS_COOKIE_SECURE=false - only use this for plain-http testing")
	}
	// A missing data file just shows as an empty list, so say so loudly here:
	// it almost always means NATLAS_DATA_DIR points at the wrong folder.
	for _, p := range app.app.Loaded {
		if _, err := os.Stat(p.File); err != nil {
			log.Printf("WARNING %s: data file %s not found (%v) - check NATLAS_DATA_DIR in .env and data_dir in natlas.yml", p.ID, p.File, err)
		}
	}

	authn := auth.New(s)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte("ok\n"))
	})
	(&api.Server{App: app.app, Auth: authn}).Register(mux)
	site, err := web.Handler(s.WebDir, app.app, version)
	if err != nil {
		log.Fatalf("cannot load web files: %v", err)
	}
	mux.Handle("/", site)

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           recoverer(accessLog(authn, securityHeaders(s.CookieSecure, mux))),
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
