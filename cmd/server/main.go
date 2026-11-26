// Command server runs everything in one process: it migrates the database,
// serves the HTTP API and delivers events.
//
// It is the simplest way to run the service: locally, and in production for
// as long as one process is enough. When request handling and background
// work need to scale separately, deploy cmd/api and cmd/worker instead, with
// cmd/migrate as a release step.
package main

import "github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"

func main() {
	bootstrap.Main(bootstrap.Command{
		Name:    "server",
		Migrate: true,
		Roles:   bootstrap.Roles{API: true, Worker: true},
	})
}
