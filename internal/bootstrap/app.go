// Package bootstrap is the composition root: the one place that knows every
// module and every piece of infrastructure, and wires them together.
//
// Dependency injection is done by hand, with constructors. There is no
// container and no reflection: the wiring is ordinary code that the compiler
// checks and that you can read top to bottom.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/config"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/httpx"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/module"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/outbox"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// App is the assembled application.
type App struct {
	cfg     config.Config
	logger  *slog.Logger
	db      *postgres.DB
	modules []module.Module
	relay   *outbox.Relay
	handler http.Handler
}

// New connects to the infrastructure and assembles every module. The caller
// must Close the returned App.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	db, err := postgres.Open(ctx, cfg.Postgres.URL, cfg.Postgres.MaxConns)
	if err != nil {
		return nil, err
	}

	// Shared infrastructure.
	clk := clock.System{}
	bus := eventbus.New()
	events := outbox.NewPublisher(db, clk)

	// Modules. This block is the system's dependency graph: a module that
	// needs another one receives its public API here, explicitly.
	customersModule := customers.New(customers.Deps{
		DB:    db,
		Clock: clk,
	})
	ordersModule := orders.New(orders.Deps{
		DB:        db,
		Clock:     clk,
		Events:    events,
		Customers: customersModule.API(),
	})
	notificationsModule := notifications.New(notifications.Deps{
		DB:        db,
		Clock:     clk,
		Logger:    logger,
		Customers: customersModule.API(),
	})
	modules := []module.Module{customersModule, ordersModule, notificationsModule}

	// Inbound adapters.
	mux := http.NewServeMux()
	httpx.RegisterHealth(mux, map[string]httpx.Check{"postgres": db.Ping})
	for _, m := range modules {
		m.RegisterRoutes(mux)
		m.RegisterSubscribers(bus)
	}

	return &App{
		cfg:     cfg,
		logger:  logger,
		db:      db,
		modules: modules,
		relay: outbox.NewRelay(db, bus, logger, outbox.RelayConfig{
			PollInterval: cfg.Outbox.PollInterval,
			BatchSize:    cfg.Outbox.BatchSize,
			MaxAttempts:  cfg.Outbox.MaxAttempts,
		}),
		handler: httpx.Chain(mux,
			httpx.RequestID(logger),
			httpx.AccessLog(logger),
			httpx.Recover(logger),
		),
	}, nil
}

// Close releases the application's resources.
func (a *App) Close() { a.db.Close() }

// Handler returns the fully assembled HTTP handler. Tests drive the
// application through it without opening a socket.
func (a *App) Handler() http.Handler { return a.handler }

// Relay returns the outbox relay, so tests can deliver events
// deterministically instead of waiting for the poller.
func (a *App) Relay() *outbox.Relay { return a.relay }

// Migrate brings every schema up to date: the platform's first, then each
// module's own.
func (a *App) Migrate(ctx context.Context) error {
	if err := postgres.Migrate(ctx, a.db, outbox.Schema, outbox.Migrations()); err != nil {
		return err
	}
	for _, m := range a.modules {
		migrations := m.Migrations()
		if migrations == nil {
			continue
		}
		if err := postgres.Migrate(ctx, a.db, m.Name(), migrations); err != nil {
			return fmt.Errorf("module %s: %w", m.Name(), err)
		}
		a.logger.InfoContext(ctx, "migrations applied", "module", m.Name())
	}
	return nil
}

// Run serves HTTP and relays events until ctx is cancelled or one of the
// two fails, then shuts both down gracefully.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		a.logger.InfoContext(ctx, "http server listening", "addr", a.cfg.HTTP.Addr)
		return httpx.Serve(ctx, httpx.ServerConfig{
			Addr:            a.cfg.HTTP.Addr,
			ReadTimeout:     a.cfg.HTTP.ReadTimeout,
			WriteTimeout:    a.cfg.HTTP.WriteTimeout,
			ShutdownTimeout: a.cfg.HTTP.ShutdownTimeout,
		}, a.handler)
	})
	g.Go(func() error {
		return a.relay.Run(ctx)
	})

	return g.Wait()
}
