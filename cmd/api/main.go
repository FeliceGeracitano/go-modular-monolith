// Command api serves the HTTP API. It does not migrate the database and
// does not deliver events: run cmd/migrate before it and cmd/worker next
// to it.
package main

import "github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"

func main() {
	bootstrap.Main(bootstrap.Command{
		Name:  "api",
		Roles: bootstrap.Roles{API: true},
	})
}
