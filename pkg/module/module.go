package module

import (
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
)

type Module interface {
	Name() string
	RegisterRoutes(mux *http.ServeMux)
	RegisterSubscribers(sub eventbus.Subscriber)
}
