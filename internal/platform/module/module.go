// Package module defines the contract between a business module and the
// composition root.
package module

import (
	"io/fs"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
)

// Module is what every business module exposes to the composition root. The
// composition root is the only place that knows the full list of modules.
type Module interface {
	// Name is the module's unique name. It is also the name of the database
	// schema the module owns.
	Name() string
	// Migrations returns the module's SQL migrations, or nil when the module
	// has no storage.
	Migrations() fs.FS
	// RegisterRoutes mounts the module's HTTP endpoints.
	RegisterRoutes(mux *http.ServeMux)
	// RegisterSubscribers subscribes the module to integration events
	// published by other modules.
	RegisterSubscribers(sub eventbus.Subscriber)
}
