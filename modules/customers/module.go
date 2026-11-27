package customers

import (
	"context"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
	core "github.com/FeliceGeracitano/go-modular-monolith/modules/customers/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

type Deps struct {
	Clock clock.Clock
}

type Module struct {
	svc     *core.Service
	handler *core.HTTPHandler
}

var _ module.Module = (*Module)(nil)

func New(deps Deps) *Module {
	svc := core.NewService(core.NewMemoryStore(), deps.Clock)
	return &Module{svc: svc, handler: core.NewHTTPHandler(svc)}
}

func (m *Module) Name() string                            { return "customers" }
func (m *Module) RegisterRoutes(mux *http.ServeMux)       { m.handler.Register(mux) }
func (m *Module) RegisterSubscribers(eventbus.Subscriber) {}
func (m *Module) API() customersapi.API                   { return api{svc: m.svc} }

type api struct{ svc *core.Service }

func (a api) GetCustomer(ctx context.Context, id string) (customersapi.Customer, error) {
	c, err := a.svc.Get(ctx, id)
	if err != nil {
		return customersapi.Customer{}, err
	}
	return customersapi.Customer{ID: c.ID, Name: c.Name, Email: c.Email}, nil
}
