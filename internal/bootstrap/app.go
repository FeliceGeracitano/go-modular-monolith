// Package bootstrap is the composition root: the one place that knows every
// module and every piece of infrastructure, and wires them together.
//
// Dependency injection is done by hand, with constructors. There is no
// container and no reflection: the wiring is ordinary code that the compiler
// checks and that you can read top to bottom.
//
//   - app.go      builds the platform and runs the process
//   - modules.go  constructs the business modules (the dependency graph)
//   - main.go     the shared entry point of the binaries under cmd/
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/config"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/httpx"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/module"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/outbox"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// Roles selects what a process does. Every process contains every module;
// roles decide which inbound adapters are switched on, so the same code can
// run as one process or be split to scale request handling and background
// work independently.
type Roles struct {
	// API serves the modules' HTTP endpoints.
	API bool
	// Worker relays the outbox and runs the modules' event subscribers.
	Worker bool
}

// platform is the shared infrastructure handed to modules.go.
type platform struct {
	cfg    config.Config
	logger *slog.Logger
	clock  clock.Clock
	// events stores integration events atomically with the caller's
	// transaction (the transactional outbox).
	events *outbox.Publisher

	admin  *postgres.DB
	scoped []*postgres.DB
}

// moduleDB returns a database handle restricted to the named module's own
// schema. Every module gets its own; none gets the administrative one.
func (p *platform) moduleDB(name string) (*postgres.DB, error) {
	db, err := p.admin.Scoped(name)
	if err != nil {
		return nil, err
	}
	p.scoped = append(p.scoped, db)
	return db, nil
}

func (p *platform) close() {
	for _, db := range p.scoped {
		db.Close()
	}
	p.admin.Close()
}

// App is the assembled application.
type App struct {
	platform *platform
	roles    Roles
	modules  []module.Module
	relay    *outbox.Relay
	handler  http.Handler
}

// New connects to the infrastructure and assembles every module. The caller
// must Close the returned App.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger, roles Roles) (_ *App, err error) {
	admin, err := postgres.Open(ctx, cfg.Postgres.URL, cfg.Postgres.MaxConns)
	if err != nil {
		return nil, err
	}
	clk := clock.System{}
	p := &platform{
		cfg:    cfg,
		logger: logger,
		clock:  clk,
		events: outbox.NewPublisher(clk),
		admin:  admin,
	}
	defer func() {
		if err != nil {
			p.close()
		}
	}()

	modules, err := p.modules()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(modules))
	for _, m := range modules {
		if seen[m.Name()] {
			return nil, fmt.Errorf("bootstrap: module %q is registered twice", m.Name())
		}
		seen[m.Name()] = true
	}

	// Inbound adapters, switched on by role. Probes are always served, so
	// an orchestrator can supervise workers the same way as API processes.
	mux := http.NewServeMux()
	httpx.RegisterHealth(mux, map[string]httpx.Check{"postgres": admin.Ping})
	if roles.API {
		for _, m := range modules {
			m.RegisterRoutes(mux)
		}
	}

	app := &App{
		platform: p,
		roles:    roles,
		modules:  modules,
		handler: httpx.Chain(mux,
			httpx.RequestID(logger),
			httpx.Telemetry(),
			httpx.AccessLog(logger),
			httpx.Recover(logger),
		),
	}

	if roles.Worker {
		bus := eventbus.New()
		for _, m := range modules {
			m.RegisterSubscribers(bus)
		}
		relayDB, err := p.moduleDB(outbox.Schema)
		if err != nil {
			return nil, err
		}
		app.relay = outbox.NewRelay(relayDB, bus, logger, outbox.RelayConfig{
			PollInterval: cfg.Outbox.PollInterval,
			BatchSize:    cfg.Outbox.BatchSize,
			MaxAttempts:  cfg.Outbox.MaxAttempts,
		})
	}
	return app, nil
}

// Close releases the application's resources.
func (a *App) Close() { a.platform.close() }

// Handler returns the fully assembled HTTP handler. Tests drive the
// application through it without opening a socket.
func (a *App) Handler() http.Handler { return a.handler }

// Relay returns the outbox relay, or nil when the process has no Worker
// role. Tests use it to deliver events deterministically instead of waiting
// for the poller.
func (a *App) Relay() *outbox.Relay { return a.relay }

// Migrate brings every schema up to date: the platform's first, then each
// module's own.
func (a *App) Migrate(ctx context.Context) error {
	admin, logger := a.platform.admin, a.platform.logger

	if err := postgres.Migrate(ctx, admin, outbox.Schema, outbox.Migrations()); err != nil {
		return err
	}
	for _, m := range a.modules {
		migrations := m.Migrations()
		if migrations == nil {
			continue
		}
		if err := postgres.Migrate(ctx, admin, m.Name(), migrations); err != nil {
			return fmt.Errorf("module %s: %w", m.Name(), err)
		}
		logger.InfoContext(ctx, "migrations applied", "module", m.Name())
	}
	return nil
}

// Run serves until ctx is cancelled or a component fails, then shuts
// everything down gracefully.
func (a *App) Run(ctx context.Context) error {
	cfg, logger := a.platform.cfg, a.platform.logger
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.InfoContext(ctx, "http server listening",
			"addr", cfg.HTTP.Addr, "api", a.roles.API, "worker", a.roles.Worker)
		return httpx.Serve(ctx, httpx.ServerConfig{
			Addr:            cfg.HTTP.Addr,
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			WriteTimeout:    cfg.HTTP.WriteTimeout,
			ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
		}, a.handler)
	})
	if a.relay != nil {
		g.Go(func() error {
			return a.relay.Run(ctx)
		})
	}

	return g.Wait()
}
