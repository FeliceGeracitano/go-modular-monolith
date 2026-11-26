// Command worker delivers integration events: it relays the outbox and runs
// every module's event subscribers. It serves only the health probes over
// HTTP. Several workers can run at once.
package main

import "github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"

func main() {
	bootstrap.Main(bootstrap.Command{
		Name:  "worker",
		Roles: bootstrap.Roles{Worker: true},
	})
}
