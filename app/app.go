package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/FeliceGeracitano/go-modular-monolith/docs"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/buildinfo"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/config"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/httpx"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/logging"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/outbox"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/telemetry"
)

type deps struct {
	cfg    config.Config
	logger *slog.Logger
	clock  clock.Clock
	events *outbox.Publisher
}

type App struct {
	deps    *deps
	modules []module.Module
	relay   *outbox.Relay
	handler http.Handler
}

func Main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() (err error) {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.Log.Level, cfg.Log.Format)
	logger.Info("starting",
		"service", cfg.Service, "env", cfg.Env,
		"version", buildinfo.Version(), "telemetry", cfg.Telemetry.Enabled)
	if cfg.Env != config.EnvLocal {
		logger = logger.With("service", cfg.Service, "env", cfg.Env, "version", buildinfo.Version())
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Setup(ctx, telemetry.Config{
		Enabled: cfg.Telemetry.Enabled,
		Service: cfg.Service,
		Version: buildinfo.Version(),
		Env:     cfg.Env,
	})
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = errors.Join(err, shutdownTelemetry(flushCtx))
	}()

	application, err := New(cfg, logger)
	if err != nil {
		return err
	}
	if err := application.Run(ctx); err != nil {
		return err
	}
	logger.InfoContext(ctx, "shut down cleanly")
	return nil
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	clk := clock.System{}
	bus := eventbus.New()
	store := outbox.NewMemoryStore()
	d := &deps{
		cfg:    cfg,
		logger: logger,
		clock:  clk,
		events: outbox.NewPublisher(store, clk),
	}

	modules := d.modules()
	seen := make(map[string]bool, len(modules))
	for _, m := range modules {
		if seen[m.Name()] {
			return nil, fmt.Errorf("app: module %q is registered twice", m.Name())
		}
		seen[m.Name()] = true
	}

	mux := http.NewServeMux()
	httpx.RegisterHealth(mux, nil)
	httpx.RegisterDocs(mux, docs.OpenAPI)
	for _, m := range modules {
		m.RegisterRoutes(mux)
		m.RegisterSubscribers(bus)
	}

	return &App{
		deps:    d,
		modules: modules,
		relay:   outbox.NewRelay(store, bus, clk, logger, outbox.RelayConfig{}),
		handler: httpx.Chain(mux,
			httpx.RequestID(logger),
			httpx.Telemetry(),
			httpx.AccessLog(logger),
			httpx.Recover(logger),
		),
	}, nil
}

func (a *App) Handler() http.Handler { return a.handler }
func (a *App) Relay() *outbox.Relay  { return a.relay }

func (a *App) Run(ctx context.Context) error {
	cfg, logger := a.deps.cfg, a.deps.logger
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.InfoContext(ctx, "http server listening", "addr", cfg.HTTP.Addr)
		return httpx.Serve(ctx, httpx.ServerConfig{
			Addr:            cfg.HTTP.Addr,
			ReadTimeout:     cfg.HTTP.ReadTimeout,
			WriteTimeout:    cfg.HTTP.WriteTimeout,
			ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
		}, a.handler)
	})
	g.Go(func() error {
		return a.relay.Run(ctx)
	})

	return g.Wait()
}
