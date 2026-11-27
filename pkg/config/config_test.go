package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/config"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func TestLoadFromAppliesDefaults(t *testing.T) {
	cfg, err := config.LoadFrom(env(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ShutdownTimeout != 15*time.Second {
		t.Errorf("HTTP.ShutdownTimeout = %v, want 15s", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Env != config.EnvLocal || cfg.Log.Format != "text" {
		t.Errorf("Env/Log.Format = %q/%q, want local/text", cfg.Env, cfg.Log.Format)
	}
	if cfg.Telemetry.Enabled {
		t.Error("telemetry export must be off unless an OTLP endpoint is configured")
	}
}

func TestLoadFromAdaptsDefaultsToTheEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(env(map[string]string{
		"APP_ENV":                     "production",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want json outside local", cfg.Log.Format)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("telemetry export should be on when an OTLP endpoint is configured")
	}

	// An explicit setting always beats the environment's default.
	cfg, err = config.LoadFrom(env(map[string]string{
		"APP_ENV":    "production",
		"LOG_FORMAT": "text",
	}))
	if err != nil || cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q (err %v), want the explicit text", cfg.Log.Format, err)
	}
}

func TestLoadFromReportsEveryProblem(t *testing.T) {
	_, err := config.LoadFrom(env(map[string]string{
		"HTTP_READ_TIMEOUT": "soon",
		"LOG_FORMAT":        "xml",
		"APP_ENV":           "prod",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"HTTP_READ_TIMEOUT", "LOG_FORMAT", "APP_ENV"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should mention %s, got: %v", key, err)
		}
	}
}
