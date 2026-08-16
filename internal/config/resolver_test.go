package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_CLIOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte(`
[server]
bind = "0.0.0.0:8080"
[auth]
admin_api_key = "file_key"
[database]
path = "/tmp/test.db"
[vpn]
provider = ""
[ratelimit]
requests_per_minute = 60
[log]
level = "info"
format = "json"
`), 0o644)

	resolved, err := Resolve(ResolveOptions{
		ConfigPath: cfgPath,
		Bind:       "0.0.0.0:9090",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Config.Server.Bind != "0.0.0.0:9090" {
		t.Errorf("expected CLI override, got %s", resolved.Config.Server.Bind)
	}
	if resolved.Sources["server.bind"] != SourceCLIFlag {
		t.Errorf("expected source CLIFlag, got %s", resolved.Sources["server.bind"])
	}
}

func TestResolve_AutoGeneratesAdminKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte(`
[server]
bind = "0.0.0.0:8080"
[auth]
admin_api_key = ""
[database]
path = "/tmp/test.db"
[vpn]
provider = ""
[ratelimit]
requests_per_minute = 60
[log]
level = "info"
format = "json"
`), 0o644)

	resolved, err := Resolve(ResolveOptions{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Config.Auth.AdminAPIKey == "" {
		t.Fatal("expected auto-generated admin key")
	}
	if resolved.Sources["auth.admin_api_key"] != SourceGeneratedDefault {
		t.Errorf("expected GeneratedDefault source, got %s", resolved.Sources["auth.admin_api_key"])
	}
}

func TestResolve_NoConfigFile_UsesDefaults(t *testing.T) {
	resolved, err := Resolve(ResolveOptions{
		ConfigPath: filepath.Join(t.TempDir(), "nonexistent.toml"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Config.Server.Bind != "0.0.0.0:8080" {
		t.Errorf("expected default bind, got %s", resolved.Config.Server.Bind)
	}
}

func TestResolve_EnvVarDeprecationAndPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte(`
[server]
bind = "0.0.0.0:8080"
[auth]
admin_api_key = "file_key"
[database]
path = "/tmp/test.db"
[vpn]
provider = ""
[ratelimit]
requests_per_minute = 60
[log]
level = "info"
format = "json"
`), 0o644)

	t.Setenv("COORD_BIND", "127.0.0.1:9999")
	resolved, err := Resolve(ResolveOptions{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Config file takes precedence over env var
	if resolved.Config.Server.Bind != "0.0.0.0:8080" {
		t.Errorf("expected config file value 0.0.0.0:8080, got %s", resolved.Config.Server.Bind)
	}

	hasWarning := false
	for _, w := range resolved.Warnings {
		if len(w) > 0 {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Error("expected deprecation warning for COORD_BIND")
	}
}

func TestAutoGenerate_CreatesConfigOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	resolved, err := Resolve(ResolveOptions{
		ConfigPath: cfgPath,
		Bind:       "0.0.0.0:9090",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("expected auto-generated config file")
	}
	if resolved.Config.Server.Bind != "0.0.0.0:9090" {
		t.Errorf("expected CLI override in generated config, got %s", resolved.Config.Server.Bind)
	}
	if resolved.Config.Auth.AdminAPIKey == "" {
		t.Fatal("expected auto-generated admin key")
	}

	// Verify permissions (0600)
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}
}

func TestAutoGenerate_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte(`
[server]
bind = "0.0.0.0:8080"
[auth]
admin_api_key = "existing_key"
[database]
path = "/tmp/test.db"
[vpn]
provider = ""
[ratelimit]
requests_per_minute = 60
[log]
level = "info"
format = "json"
`), 0o644)

	resolved, err := Resolve(ResolveOptions{
		ConfigPath: cfgPath,
		Bind:       "0.0.0.0:9090",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CLI override applies in memory
	if resolved.Config.Server.Bind != "0.0.0.0:9090" {
		t.Error("expected CLI override in memory")
	}

	// Disk file preserves original content
	content, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(content), "0.0.0.0:8080") {
		t.Error("disk config should not be overwritten")
	}
}

func TestResolve_DeprecationWarningContainsActionableDetails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte(`
[server]
bind = "0.0.0.0:8080"
[auth]
admin_api_key = "file_key"
[database]
path = "/tmp/test.db"
[vpn]
provider = ""
[ratelimit]
requests_per_minute = 60
[log]
level = "info"
format = "json"
`), 0o644)

	t.Setenv("COORD_ADMIN_API_KEY", "secret_env_key_123")

	resolved, err := Resolve(ResolveOptions{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundWarning string
	for _, w := range resolved.Warnings {
		if strings.Contains(w, "COORD_ADMIN_API_KEY") {
			foundWarning = w
			break
		}
	}

	if foundWarning == "" {
		t.Fatal("expected deprecation warning for COORD_ADMIN_API_KEY")
	}

	if !strings.Contains(foundWarning, "v0.3.0") {
		t.Errorf("expected warning to mention v0.3.0, got: %s", foundWarning)
	}
	if !strings.Contains(foundWarning, "--admin-api-key") {
		t.Errorf("expected warning to mention --admin-api-key, got: %s", foundWarning)
	}
	if !strings.Contains(foundWarning, "config.toml") {
		t.Errorf("expected warning to mention config.toml, got: %s", foundWarning)
	}
}
