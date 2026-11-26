package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/config"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func TestLoadFromAppliesDefaults(t *testing.T) {
	cfg, err := config.LoadFrom(env(map[string]string{
		"DATABASE_URL": "postgres://localhost/app",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.Outbox.PollInterval != time.Second {
		t.Errorf("Outbox.PollInterval = %v, want 1s", cfg.Outbox.PollInterval)
	}
}

func TestLoadFromReportsEveryProblem(t *testing.T) {
	_, err := config.LoadFrom(env(map[string]string{
		"HTTP_READ_TIMEOUT": "soon",
		"LOG_FORMAT":        "xml",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"DATABASE_URL", "HTTP_READ_TIMEOUT", "LOG_FORMAT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should mention %s, got: %v", key, err)
		}
	}
}
