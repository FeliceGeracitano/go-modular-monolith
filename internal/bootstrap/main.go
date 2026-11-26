package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/buildinfo"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/config"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/logging"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/telemetry"
)

// Command describes one of the binaries under cmd/. They all share this
// entry point and differ only in what they switch on.
type Command struct {
	// Name identifies the process in logs ("api", "worker", ...).
	Name string
	// Migrate applies pending migrations before doing anything else.
	Migrate bool
	// Roles are what the process runs afterwards. With no role it exits
	// once migrations are done.
	Roles Roles
}

// Main runs a command and exits the process: 0 on a clean shutdown, 1 on
// failure.
func Main(cmd Command) {
	if err := run(cmd); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cmd Command) (err error) {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.Log.Level, cfg.Log.Format)
	logger.Info("starting",
		"service", cfg.Service, "process", cmd.Name, "env", cfg.Env,
		"version", buildinfo.Version(), "telemetry", cfg.Telemetry.Enabled)
	if cfg.Env != config.EnvLocal {
		// Identify the source of every line once logs from many services
		// and environments land in the same place.
		logger = logger.With(
			"service", cfg.Service, "process", cmd.Name, "env", cfg.Env, "version", buildinfo.Version())
	}
	slog.SetDefault(logger)

	// SIGINT / SIGTERM cancel ctx, which starts a graceful shutdown.
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
		// Flush buffered spans and metrics, even though ctx is cancelled.
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = errors.Join(err, shutdownTelemetry(flushCtx))
	}()

	app, err := New(ctx, cfg, logger, cmd.Roles)
	if err != nil {
		return err
	}
	defer app.Close()

	if cmd.Migrate {
		if err := app.Migrate(ctx); err != nil {
			return err
		}
	}
	if !cmd.Roles.API && !cmd.Roles.Worker {
		return nil
	}

	if err := app.Run(ctx); err != nil {
		return err
	}
	logger.InfoContext(ctx, "shut down cleanly")
	return nil
}
