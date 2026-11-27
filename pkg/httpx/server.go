package httpx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ServerConfig tunes the HTTP server.
type ServerConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Serve runs an HTTP server until ctx is cancelled, then shuts it down
// gracefully: it stops accepting connections and waits up to
// ShutdownTimeout for in-flight requests to finish.
func Serve(ctx context.Context, cfg ServerConfig, handler http.Handler) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       2 * time.Minute,
		// Requests outlive the signal that triggers shutdown, so they are
		// not derived from ctx.
		BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}

	// Bind before serving, so a port conflict fails startup immediately
	// instead of surfacing later from a goroutine.
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("http: listen: %w", err)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http: serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		// In-flight requests outlived the grace period: cut them off.
		_ = srv.Close()
	}
	err = <-serveErr // always wait for the serving goroutine to exit

	if shutdownErr != nil {
		return fmt.Errorf("http: shutdown: %w", shutdownErr)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http: serve: %w", err)
	}
	return nil
}
