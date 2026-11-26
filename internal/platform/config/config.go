// Package config loads process configuration from environment variables.
//
// Configuration is read once, at startup, by the composition root and handed
// down as plain values. Modules never read the environment themselves.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/buildinfo"
)

// Environments the service can run in. The same binary and image run in
// all of them; only configuration differs.
const (
	EnvLocal      = "local"
	EnvDev        = "dev"
	EnvStaging    = "staging"
	EnvProduction = "production"
)

// Config is the full process configuration.
type Config struct {
	// Env names the environment this process runs in. It labels logs and
	// telemetry and picks a few defaults; it must never switch business
	// behaviour on or off (use an explicit setting for that).
	Env string
	// Service is the name this process reports in logs and telemetry.
	Service   string
	HTTP      HTTP
	Postgres  Postgres
	Log       Log
	Outbox    Outbox
	Telemetry Telemetry
}

// Telemetry configures trace and metric export.
type Telemetry struct {
	// Enabled is true when an OTLP endpoint is configured. Everything else
	// about export is read by the OpenTelemetry SDK from the standard
	// OTEL_* variables.
	Enabled bool
}

// HTTP configures the HTTP server.
type HTTP struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Postgres configures the database connection pool.
type Postgres struct {
	URL      string
	MaxConns int32
}

// Log configures structured logging.
type Log struct {
	Level  slog.Level
	Format string // "json" or "text"
}

// Outbox configures the relay that delivers integration events.
type Outbox struct {
	PollInterval time.Duration
	BatchSize    int
	MaxAttempts  int
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads configuration through lookup, which makes loading testable
// without touching the real environment. Every problem is reported at once.
func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	l := loader{lookup: lookup}

	env := l.oneOf("APP_ENV", EnvLocal, EnvLocal, EnvDev, EnvStaging, EnvProduction)
	// Humans read local logs; machines read everything else.
	logFormat := "json"
	if env == EnvLocal {
		logFormat = "text"
	}

	cfg := Config{
		Env:     env,
		Service: l.str("SERVICE_NAME", buildinfo.Service()),
		HTTP: HTTP{
			Addr:            l.str("HTTP_ADDR", ":8080"),
			ReadTimeout:     l.duration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    l.duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: l.duration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		Postgres: Postgres{
			URL:      l.required("DATABASE_URL"),
			MaxConns: int32(l.integer("DATABASE_MAX_CONNS", 10, 1, 1000)), //nolint:gosec // bounded to [1, 1000] above
		},
		Log: Log{
			Level:  l.level("LOG_LEVEL", slog.LevelInfo),
			Format: l.oneOf("LOG_FORMAT", logFormat, "json", "text"),
		},
		Outbox: Outbox{
			PollInterval: l.duration("OUTBOX_POLL_INTERVAL", time.Second),
			BatchSize:    l.integer("OUTBOX_BATCH_SIZE", 50, 1, 1000),
			MaxAttempts:  l.integer("OUTBOX_MAX_ATTEMPTS", 10, 1, 1000),
		},
		Telemetry: Telemetry{
			Enabled: l.str("OTEL_EXPORTER_OTLP_ENDPOINT", "") != "" && l.str("OTEL_SDK_DISABLED", "") != "true",
		},
	}
	return cfg, errors.Join(l.errs...)
}

type loader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (l *loader) fail(key string, err error) {
	l.errs = append(l.errs, fmt.Errorf("config %s: %w", key, err))
}

func (l *loader) str(key, def string) string {
	if v, ok := l.lookup(key); ok && v != "" {
		return v
	}
	return def
}

func (l *loader) required(key string) string {
	v := l.str(key, "")
	if v == "" {
		l.fail(key, errors.New("is required"))
	}
	return v
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		l.fail(key, fmt.Errorf("%q is not a positive duration", raw))
		return def
	}
	return d
}

func (l *loader) integer(key string, def, minValue, maxValue int) int {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minValue || n > maxValue {
		l.fail(key, fmt.Errorf("%q is not an integer between %d and %d", raw, minValue, maxValue))
		return def
	}
	return n
}

func (l *loader) level(key string, def slog.Level) slog.Level {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		l.fail(key, fmt.Errorf("%q is not a log level (debug, info, warn, error)", raw))
		return def
	}
	return level
}

func (l *loader) oneOf(key, def string, allowed ...string) string {
	v := l.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	l.fail(key, fmt.Errorf("%q is not one of %v", v, allowed))
	return def
}
