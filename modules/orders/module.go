package orders

import (
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
	core "github.com/FeliceGeracitano/go-modular-monolith/modules/orders/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

type Deps struct {
	Clock     clock.Clock
	Events    core.EventPublisher
	Customers customersapi.API
}

type Module struct {
	handler *core.HTTPHandler
}

var _ module.Module = (*Module)(nil)

func New(deps Deps) *Module {
	svc := core.NewService(
		core.NewMemoryStore(),
		core.NewCustomerDirectory(deps.Customers),
		deps.Events,
		deps.Clock,
	)
	return &Module{handler: core.NewHTTPHandler(svc)}
}

func (m *Module) Name() string                            { return "orders" }
func (m *Module) RegisterRoutes(mux *http.ServeMux)       { m.handler.Register(mux) }
func (m *Module) RegisterSubscribers(eventbus.Subscriber) {}
