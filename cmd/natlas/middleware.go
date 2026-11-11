// middleware.go — handlers wrapped around every request: headers, access log, panic recovery.

package main

import (
	"log"
	"net/http"
	"time"

	"natlas/internal/auth"
)

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

// WriteHeader records the status, then writes it.
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
