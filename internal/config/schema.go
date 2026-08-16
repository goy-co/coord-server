package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Config é a configuração raiz do coord-server.
type Config struct {
	Server    ServerConfig    `toml:"server"`
	Auth      AuthConfig      `toml:"auth"`
	Database  DatabaseConfig  `toml:"database"`
	VPN       VPNConfig       `toml:"vpn"`
	RateLimit RateLimitConfig `toml:"ratelimit"`
	Log       LogConfig       `toml:"log"`
	Jobs      JobsConfig      `toml:"jobs,omitempty"`
	Registry  RegistryConfig  `toml:"registry,omitempty"`
}

type ServerConfig struct {
	Bind                string `toml:"bind"`
	Listen              string `toml:"listen,omitempty"`
	ReadTimeoutSeconds  int    `toml:"read_timeout_seconds,omitempty"`
	WriteTimeoutSeconds int    `toml:"write_timeout_seconds,omitempty"`
}

type AuthConfig struct {
	AdminAPIKey string   `toml:"admin_api_key"`
	RequireAuth bool     `toml:"require_auth,omitempty"`
	HMACSecret  string   `toml:"hmac_secret,omitempty"`
	PublicPaths []string `toml:"public_paths,omitempty"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type VPNConfig struct {
	Provider                string `toml:"provider"` // "tailscale" | "headscale" | ""
	Enabled                 bool   `toml:"enabled,omitempty"`
	TailscaleAPIKey         string `toml:"tailscale_api_key"`
	TailscaleTailnet        string `toml:"tailscale_tailnet"`
	TailscaleTag            string `toml:"tailscale_tag,omitempty"`
	TailscaleKeyExpiryHours int    `toml:"tailscale_key_expiry_hours,omitempty"`
	TailscaleKeyReusable    bool   `toml:"tailscale_key_reusable,omitempty"`
	HeadscaleURL            string `toml:"headscale_url"`
	HeadscaleAPIURL         string `toml:"headscale_api_url,omitempty"`
	HeadscaleAPIKey         string `toml:"headscale_api_key"`
	HeadscaleUser           string `toml:"headscale_user"`
	HeadscaleKeyExpiryHours int    `toml:"headscale_key_expiry_hours,omitempty"`
	HeadscaleKeyReusable    bool   `toml:"headscale_key_reusable,omitempty"`
	KeyExpiryHours          int    `toml:"key_expiry_hours"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `toml:"requests_per_minute"`
	Burst             int `toml:"burst,omitempty"`
	HeartbeatRPM      int `toml:"heartbeat_rpm,omitempty"`
}

type LogConfig struct {
	Level  string `toml:"level"`  // "trace","debug","info","warn","error"
	Format string `toml:"format"` // "pretty","json"
}

type JobsConfig struct {
	CleanupRelaysIntervalSeconds int `toml:"cleanup_relays_interval_seconds,omitempty"`
	CleanupNodesIntervalSeconds  int `toml:"cleanup_nodes_interval_seconds,omitempty"`
	StatsRefreshIntervalSeconds  int `toml:"stats_refresh_interval_seconds,omitempty"`
	NodeInactiveThresholdHours   int `toml:"node_inactive_threshold_hours,omitempty"`
}

type RegistryConfig struct {
	RelayTTLSeconds          int `toml:"relay_ttl_seconds,omitempty"`
	DiscoveryCacheTTLSeconds int `toml:"discovery_cache_ttl_seconds,omitempty"`
	MaxRelaysPerResponse     int `toml:"max_relays_per_response,omitempty"`
	OnlineThresholdSeconds   int `toml:"online_threshold_seconds,omitempty"`
}

// Defaults retorna um Config com valores default sensatos.
func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Bind:                "0.0.0.0:8080",
			Listen:              "0.0.0.0:8080",
			ReadTimeoutSeconds:  15,
			WriteTimeoutSeconds: 15,
		},
		Auth: AuthConfig{
			AdminAPIKey: "", // Deve ser gerada ou definida pelo operador
			RequireAuth: true,
			PublicPaths: []string{"/health", "/info", "/metrics"},
		},
		Database: DatabaseConfig{
			Path: "/var/lib/goy-coord/coord.db",
		},
		VPN: VPNConfig{
			Provider:                "",
			KeyExpiryHours:          168, // 7 dias
			TailscaleKeyExpiryHours: 24,
			HeadscaleUser:           "goy-nodes",
			HeadscaleKeyExpiryHours: 24,
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: 60,
			Burst:             30,
			HeartbeatRPM:      120,
		},
		Jobs: JobsConfig{
			CleanupRelaysIntervalSeconds: 60,
			CleanupNodesIntervalSeconds:  300,
			StatsRefreshIntervalSeconds:  30,
			NodeInactiveThresholdHours:   24,
		},
		Registry: RegistryConfig{
			RelayTTLSeconds:          300,
			DiscoveryCacheTTLSeconds: 15,
			MaxRelaysPerResponse:     100,
			OnlineThresholdSeconds:   180,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Validate valida semanticamente a configuração.
func (c *Config) Validate() error {
	bindAddr := c.Server.Bind
	if bindAddr == "" || (c.Server.Listen != "" && c.Server.Listen != "0.0.0.0:8080" && c.Server.Bind == "0.0.0.0:8080") {
		bindAddr = c.Server.Listen
	}
	if c.Server.Listen != "" && c.Server.Listen != bindAddr {
		if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
			return fmt.Errorf("server.listen is invalid: %w", err)
		}
	}
	if _, _, err := net.SplitHostPort(bindAddr); err != nil {
		return fmt.Errorf("server.bind is invalid: %w", err)
	}

	// 2. Admin API key obrigatória se RequireAuth estiver ativo
	if c.Auth.RequireAuth && strings.TrimSpace(c.Auth.AdminAPIKey) == "" {
		return fmt.Errorf("auth.admin_api_key cannot be empty")
	}

	// 3. DB path absoluto
	if !filepath.IsAbs(c.Database.Path) {
		return fmt.Errorf("database.path must be absolute, got '%s'", c.Database.Path)
	}

	// 4. VPN provider válido (se configurado)
	switch c.VPN.Provider {
	case "", "tailscale", "headscale":
		// OK
	default:
		return fmt.Errorf("vpn.provider must be 'tailscale', 'headscale', or empty. Got '%s'", c.VPN.Provider)
	}

	// 5. Se provider = tailscale, credenciais obrigatórias
	if c.VPN.Provider == "tailscale" {
		if strings.TrimSpace(c.VPN.TailscaleAPIKey) == "" {
			return fmt.Errorf("vpn.tailscale_api_key is required when provider is 'tailscale'")
		}
		if strings.TrimSpace(c.VPN.TailscaleTailnet) == "" {
			return fmt.Errorf("vpn.tailscale_tailnet is required when provider is 'tailscale'")
		}
	}

	// 6. Se provider = headscale, credenciais obrigatórias
	if c.VPN.Provider == "headscale" {
		url := c.VPN.HeadscaleURL
		if url == "" {
			url = c.VPN.HeadscaleAPIURL
		}
		if strings.TrimSpace(url) == "" {
			return fmt.Errorf("vpn.headscale_url is required when provider is 'headscale'")
		}
		if strings.TrimSpace(c.VPN.HeadscaleAPIKey) == "" {
			return fmt.Errorf("vpn.headscale_api_key is required when provider is 'headscale'")
		}
	}

	// 7. Rate limit positivo
	if c.RateLimit.RequestsPerMinute <= 0 {
		return fmt.Errorf("ratelimit.requests_per_minute must be positive, got %d", c.RateLimit.RequestsPerMinute)
	}

	// 8. Log level válido
	switch c.Log.Level {
	case "trace", "debug", "info", "warn", "error":
		// OK
	default:
		return fmt.Errorf("log.level must be one of: trace, debug, info, warn, error. Got '%s'", c.Log.Level)
	}

	// 9. Log format válido
	switch c.Log.Format {
	case "pretty", "json":
		// OK
	default:
		return fmt.Errorf("log.format must be 'pretty' or 'json'. Got '%s'", c.Log.Format)
	}

	return nil
}

// DefaultConfigPath retorna o path canónico do config.toml.
func DefaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "config.toml")
	}
	return filepath.Join(configDir, "goy-coord", "config.toml")
}
