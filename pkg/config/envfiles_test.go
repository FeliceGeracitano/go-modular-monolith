package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/config"
)

// The per-environment settings files are deployed without anyone running
// them first. This test is that run: every file must be a configuration the
// service accepts, so a typo fails in CI instead of on release day.
func TestEnvironmentFilesAreValid(t *testing.T) {
	files, err := filepath.Glob("../../deploy/env/*.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no environment files found in deploy/env")
	}

	// Learn which settings exist by recording what the loader asks for.
	known := map[string]bool{}
	_, _ = config.LoadFrom(func(key string) (string, bool) {
		known[key] = true
		return "", false
	})

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".env")
		t.Run(name, func(t *testing.T) {
			settings := readEnvFile(t, file)

			for key := range settings {
				// The OpenTelemetry SDK reads its own OTEL_* settings.
				if !known[key] && !strings.HasPrefix(key, "OTEL_") {
					t.Errorf("%s is not a setting the service reads (typo?)", key)
				}
			}

			cfg, err := config.LoadFrom(func(key string) (string, bool) {
				value, ok := settings[key]
				return value, ok
			})
			if err != nil {
				t.Fatalf("invalid configuration: %v", err)
			}
			if cfg.Env != name {
				t.Errorf("APP_ENV = %q, want %q to match the file name", cfg.Env, name)
			}
		})
	}
}

func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s: malformed line %q", path, line)
		}
		settings[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return settings
}
