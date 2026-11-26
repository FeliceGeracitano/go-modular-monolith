// Command migrate applies pending database migrations and exits. Run it as
// a release step, before starting the new version of cmd/api and
// cmd/worker.
package main

import "github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"

func main() {
	bootstrap.Main(bootstrap.Command{
		Name:    "migrate",
		Migrate: true,
	})
}
