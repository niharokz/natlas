// Command natlas serves the Natlas web app.
//
// Configuration comes from the environment (.env) and natlas.yml; plugins
// from plugins/<id>/plugin.yml. See README.md.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the container image has no zoneinfo; TZ=Asia/Kolkata still works

	"natlas/internal/api"
	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/web"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	if len(os.Args) > 1 && os.Args[1] == "check" {
		// `natlas check`: validate .env, natlas.yml and every plugin.yml, then exit.
		if _, err := load(); err != nil {
			log.Fatalf("config check failed:\n%v", err)
		}
		log.Println("config OK")
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "health" {
		// `natlas health`: used by the Docker HEALTHCHECK (the image has no curl).
		addr := os.Getenv("NATLAS_ADDR")
		if addr == "" || addr[0] == ':' {
			addr = "127.0.0.1" + addr
			if addr == "127.0.0.1" {
				addr += ":8080"
			}
		}
		res, err := (&http.Client{Timeout: 4 * time.Second}).Get("http://" + addr + "/api/me")
		if err != nil || res.StatusCode != http.StatusOK {
			log.Fatalf("unhealthy: %v", err)
		}
		return
	}

	app, err := load()
	if err != nil {
		log.Fatalf("cannot start:\n%v", err)
	}
	s := app.settings
	mux := http.NewServeMux()
	(&api.Server{App: app.app, Auth: auth.New(s)}).Register(mux)
	site, err := web.Handler(s.WebDir, app.app)
	if err != nil {
		log.Fatalf("cannot load web files: %v", err)
	}
	mux.Handle("/", site)

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		ids := make([]string, 0, len(app.app.Loaded))
		for _, p := range app.app.Loaded {
			ids = append(ids, p.ID)
		}
		log.Printf("natlas listening on %s - plugins: %v - today is %s (%s)", s.Addr, ids,
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
	app, err := config.Load(s)
	if err != nil {
		return nil, err
	}
	return &loaded{s, app}, nil
}

// securityHeaders adds conservative browser protections to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
