package config

import (
	"os"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TIMESCALE_DSN", "postgres://user:pass@localhost:5432/db")
}

func TestNewConfigDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if cfg.LogLevel != "error" {
		t.Errorf("expected default log level 'error', got %q", cfg.LogLevel)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.TimescaleTable != "sensors.pmsa003i" {
		t.Errorf("expected default table 'sensors.pmsa003i', got %q", cfg.TimescaleTable)
	}
	if cfg.QueryTimeout != 10*time.Second {
		t.Errorf("expected default query timeout 10s, got %v", cfg.QueryTimeout)
	}
	if cfg.DragonflyHost != "" {
		t.Errorf("expected dragonfly host unset by default, got %q", cfg.DragonflyHost)
	}
	if cfg.DragonflyPort != 6379 {
		t.Errorf("expected default dragonfly port 6379, got %d", cfg.DragonflyPort)
	}
	if cfg.DragonflyKeyPrefix != "aqi" {
		t.Errorf("expected default dragonfly key prefix 'aqi', got %q", cfg.DragonflyKeyPrefix)
	}
	if cfg.CacheResultsDuration != 5*time.Minute {
		t.Errorf("expected default cache duration 5m, got %v", cfg.CacheResultsDuration)
	}
	if cfg.PublishPrefix != "v1" {
		t.Errorf("expected default publish prefix 'v1', got %q", cfg.PublishPrefix)
	}
	if cfg.PublishCacheControl != "public, max-age=60" {
		t.Errorf("expected default publish cache control 'public, max-age=60', got %q", cfg.PublishCacheControl)
	}
	if cfg.AuthenticationEnabled {
		t.Error("expected authentication disabled by default")
	}
	if !cfg.MetricsEnabled {
		t.Error("expected metrics enabled by default")
	}
	if cfg.MetricsPort != 8081 {
		t.Errorf("expected default metrics port 8081, got %d", cfg.MetricsPort)
	}
	if cfg.TracingEnabled {
		t.Error("expected tracing disabled by default")
	}
	if cfg.TracingService != "aqi-api" {
		t.Errorf("expected default tracing service 'aqi-api', got %q", cfg.TracingService)
	}
}

func TestNewConfigOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("API_KEYS", "key-one,key-two")
	t.Setenv("AUTHENTICATION_ENABLED", "true")
	t.Setenv("PUBLISH_BUCKET_URL", "file:///tmp/out")

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if !cfg.AuthenticationEnabled {
		t.Error("expected authentication enabled")
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "key-one" || cfg.APIKeys[1] != "key-two" {
		t.Errorf("expected api keys [key-one key-two], got %v", cfg.APIKeys)
	}
	if cfg.PublishBucketURL != "file:///tmp/out" {
		t.Errorf("expected publish bucket url 'file:///tmp/out', got %q", cfg.PublishBucketURL)
	}
}

func TestNewConfigMissingRequired(t *testing.T) {
	// t.Setenv registers cleanup so the unset below does not leak between tests.
	t.Setenv("TIMESCALE_DSN", "placeholder")
	_ = os.Unsetenv("TIMESCALE_DSN")

	_, err := NewConfig()
	if err == nil {
		t.Fatal("expected error when required env vars are missing, got nil")
	}
}
