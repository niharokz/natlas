// commands.go — the one-shot commands: cli, hash-password, health, bans, unban.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"natlas/internal/api"
	"natlas/internal/auth"
	"natlas/internal/store"
)

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
