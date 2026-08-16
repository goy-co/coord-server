package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults_Valid(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test_key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}
}

func TestValidate_EmptyAdminKey(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty admin key")
	}
}

func TestValidate_TailscaleWithoutCredentials(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.VPN.Provider = "tailscale"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for tailscale without credentials")
	}

	cfg.VPN.TailscaleAPIKey = "key"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for tailscale without tailnet")
	}

	cfg.VPN.TailscaleTailnet = "net"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid tailscale config, got: %v", err)
	}
}

func TestValidate_HeadscaleWithoutCredentials(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.VPN.Provider = "headscale"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for headscale without url and key")
	}

	cfg.VPN.HeadscaleURL = "https://headscale.example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for headscale without api key")
	}

	cfg.VPN.HeadscaleAPIKey = "hs-key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid headscale config, got: %v", err)
	}
}

func TestValidate_InvalidBind(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.Server.Bind = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid bind")
	}
}

func TestValidate_RelativeDBPath(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.Database.Path = "relative/path.db"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for relative database path")
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.Log.Level = "verbose"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.Log.Format = "xml"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid log format")
	}
}

func TestValidate_InvalidRateLimit(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.AdminAPIKey = "test"
	cfg.RateLimit.RequestsPerMinute = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for non-positive rate limit")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Fatal("expected non-empty default config path")
	}
	if filepath.Base(path) != "config.toml" {
		t.Fatalf("expected filename config.toml, got %s", filepath.Base(path))
	}
}

func TestGenerateDefault(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "goy-coord", "config.toml")

	if err := GenerateDefault(path); err != nil {
		t.Fatalf("GenerateDefault failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("generated config is empty")
	}
}
