package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goy-co/coord-server/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("COORD_LISTEN")
	os.Unsetenv("COORD_DB_PATH")
	os.Unsetenv("COORD_REQUIRE_AUTH")
	os.Unsetenv("COORD_ADMIN_API_KEY")
	os.Unsetenv("COORD_AUTH_SECRET")
	os.Unsetenv("COORD_VPN_ENABLED")
	os.Unsetenv("COORD_HEADSCALE_API_URL")
	os.Unsetenv("COORD_HEADSCALE_API_KEY")
	os.Unsetenv("COORD_RATE_LIMIT_RPM")
	os.Unsetenv("COORD_CLEANUP_RELAYS_INTERVAL")
	os.Unsetenv("COORD_RELAY_TTL")
	os.Unsetenv("COORD_DISCOVERY_CACHE_TTL")

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "non_existent_config.toml")

	t.Setenv("COORD_ADMIN_API_KEY", "default-test-admin-key")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Expected success loading defaults, got: %v", err)
	}

	if cfg.Server.Listen != config.DefaultListen {
		t.Errorf("Expected listen %s, got %s", config.DefaultListen, cfg.Server.Listen)
	}

	if cfg.Database.Path != config.DefaultDBPath {
		t.Errorf("Expected db path %s, got %s", config.DefaultDBPath, cfg.Database.Path)
	}

	if cfg.Auth.RequireAuth != true {
		t.Errorf("Expected RequireAuth true by default")
	}

	if cfg.Jobs.CleanupRelaysIntervalSeconds != config.DefaultCleanupRelaysIntervalSeconds {
		t.Errorf("Expected CleanupRelaysIntervalSeconds %d, got %d", config.DefaultCleanupRelaysIntervalSeconds, cfg.Jobs.CleanupRelaysIntervalSeconds)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("COORD_LISTEN", "127.0.0.1:9090")
	t.Setenv("COORD_DB_PATH", "/tmp/test.db")
	t.Setenv("COORD_REQUIRE_AUTH", "true")
	t.Setenv("COORD_ADMIN_API_KEY", "admin-secret-key-123")
	t.Setenv("COORD_AUTH_SECRET", "super-secret-key")
	t.Setenv("COORD_VPN_ENABLED", "true")
	t.Setenv("COORD_HEADSCALE_API_URL", "https://vpn.example.com")
	t.Setenv("COORD_HEADSCALE_API_KEY", "hs-api-key-123")
	t.Setenv("COORD_HEADSCALE_USER", "custom-goy-nodes")
	t.Setenv("COORD_RATE_LIMIT_RPM", "120")
	t.Setenv("COORD_CLEANUP_RELAYS_INTERVAL", "45")

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Unexpected error loading config: %v", err)
	}

	if cfg.Auth.AdminAPIKey != "admin-secret-key-123" {
		t.Errorf("Expected AdminAPIKey admin-secret-key-123, got: %s", cfg.Auth.AdminAPIKey)
	}
	if cfg.RateLimit.RequestsPerMinute != 120 {
		t.Errorf("Expected RateLimit 120, got: %d", cfg.RateLimit.RequestsPerMinute)
	}
	if cfg.Jobs.CleanupRelaysIntervalSeconds != 45 {
		t.Errorf("Expected CleanupRelaysIntervalSeconds 45, got: %d", cfg.Jobs.CleanupRelaysIntervalSeconds)
	}
}

func TestConfigValidation(t *testing.T) {
	invalidListenCfg := config.DefaultConfig()
	invalidListenCfg.Server.Listen = "invalid-address-no-port"
	invalidListenCfg.Auth.RequireAuth = false

	if err := invalidListenCfg.Validate(); err == nil {
		t.Errorf("Expected validation error for invalid listen address")
	}

	emptyDBCfg := config.DefaultConfig()
	emptyDBCfg.Database.Path = ""
	emptyDBCfg.Auth.RequireAuth = false

	if err := emptyDBCfg.Validate(); err == nil {
		t.Errorf("Expected validation error for empty db path")
	}

	authRequiredWithoutKey := config.DefaultConfig()
	authRequiredWithoutKey.Auth.RequireAuth = true
	authRequiredWithoutKey.Auth.AdminAPIKey = ""

	if err := authRequiredWithoutKey.Validate(); err == nil {
		t.Errorf("Expected validation error when RequireAuth is true without COORD_ADMIN_API_KEY")
	}
}
