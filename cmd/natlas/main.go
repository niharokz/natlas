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
	"fmt"
	"log"
	"os"
	_ "time/tzdata" // the container image has no zoneinfo; TZ=Asia/Kolkata still works

	"natlas/internal/auth"
	"natlas/internal/config"
)

// version is set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

// main dispatches a subcommand, or runs the server.
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

// loaded is the result of load: the settings and the app with its plugins.
type loaded struct {
	settings *config.Settings
	app      *config.App
}

// load reads .env and natlas.yml and every enabled plugin.
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
