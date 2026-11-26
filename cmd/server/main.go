// Command server runs the modular monolith.
//
// Usage:
//
//	server            migrate the database, then serve (convenient for development)
//	server serve      serve only; the schema must already be up to date
//	server migrate    migrate the database and exit (run this as a deploy step)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/config"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/logging"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "", "serve", "migrate":
	default:
		return fmt.Errorf("unknown command %q (expected: serve, migrate)", command)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(logger)

	// SIGINT / SIGTERM cancel ctx, which starts a graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := bootstrap.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	if command != "serve" {
		if err := app.Migrate(ctx); err != nil {
			return err
		}
	}
	if command == "migrate" {
		return nil
	}

	if err := app.Run(ctx); err != nil {
		return err
	}
	logger.Info("shut down cleanly")
	return nil
}
