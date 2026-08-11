package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ServerConfig holds network and timeout definitions for the HTTP server.
type ServerConfig struct {
	Listen              string `toml:"listen"`
	ReadTimeoutSeconds  int    `toml:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `toml:"write_timeout_seconds"`
}

// DatabaseConfig holds data persistence definitions.
type DatabaseConfig struct {
	Path string `toml:"path"`
}

// AuthConfig holds keys and authentication rules for the API.
type AuthConfig struct {
	RequireAuth bool     `toml:"require_auth"`
	AdminAPIKey string   `toml:"-"` // Never read from TOML, only via env var COORD_ADMIN_API_KEY
	HMACSecret  string   `toml:"-"` // Never read from TOML, only via env var COORD_AUTH_SECRET
	PublicPaths []string `toml:"public_paths"`
}

// VPNConfig holds parameters for integration with Tailscale or Headscale VPN control planes.
type VPNConfig struct {
	Enabled                 bool   `toml:"enabled"`
	Provider                string `toml:"provider"`
	TailscaleAPIKey         string `toml:"-"` // Never read from TOML, only via env var COORD_TAILSCALE_API_KEY
	TailscaleTailnet        string `toml:"tailscale_tailnet"`
	TailscaleTag            string `toml:"tailscale_tag"`
	TailscaleKeyExpiryHours int    `toml:"tailscale_key_expiry_hours"`
	TailscaleKeyReusable    bool   `toml:"tailscale_key_reusable"`
	HeadscaleAPIURL         string `toml:"headscale_api_url"`
	HeadscaleAPIKey         string `toml:"-"` // Never read from TOML, only via env var COORD_HEADSCALE_API_KEY
	HeadscaleUser           string `toml:"headscale_user"`
	HeadscaleKeyExpiryHours int    `toml:"headscale_key_expiry_hours"`
	HeadscaleKeyReusable    bool   `toml:"headscale_key_reusable"`
}

// RateLimitConfig holds HTTP request rate limiting rules.
type RateLimitConfig struct {
	RequestsPerMinute int `toml:"requests_per_minute"`
	Burst             int `toml:"burst"`
	HeartbeatRPM      int `toml:"heartbeat_rpm"`
}

// JobsConfig holds execution intervals for background maintenance jobs.
type JobsConfig struct {
	CleanupRelaysIntervalSeconds int `toml:"cleanup_relays_interval_seconds"`
	CleanupNodesIntervalSeconds  int `toml:"cleanup_nodes_interval_seconds"`
	StatsRefreshIntervalSeconds  int `toml:"stats_refresh_interval_seconds"`
	NodeInactiveThresholdHours   int `toml:"node_inactive_threshold_hours"`
}

// RegistryConfig holds definitions for the relay discovery service.
type RegistryConfig struct {
	RelayTTLSeconds          int `toml:"relay_ttl_seconds"`
	DiscoveryCacheTTLSeconds int `toml:"discovery_cache_ttl_seconds"`
	MaxRelaysPerResponse     int `toml:"max_relays_per_response"`
}

// Config aggregates all configuration sections of coord-server.
type Config struct {
	Server    ServerConfig    `toml:"server"`
	Database  DatabaseConfig  `toml:"database"`
	Auth      AuthConfig      `toml:"auth"`
	VPN       VPNConfig       `toml:"vpn"`
	RateLimit RateLimitConfig `toml:"rate_limit"`
	Jobs      JobsConfig      `toml:"jobs"`
	Registry  RegistryConfig  `toml:"registry"`
}

const (
	DefaultListen                       = "0.0.0.0:8080"
	DefaultDBPath                       = "data/coord-server.db"
	DefaultReadTimeoutSeconds           = 15
	DefaultWriteTimeoutSeconds          = 15
	DefaultRequireAuth                  = true
	DefaultRequestsPerMinute            = 60
	DefaultBurst                        = 30
	DefaultHeartbeatRPM                 = 120
	DefaultCleanupRelaysIntervalSeconds = 60
	DefaultCleanupNodesIntervalSeconds  = 300
	DefaultStatsRefreshIntervalSeconds  = 30
	DefaultNodeInactiveThresholdHours   = 24
	DefaultRelayTTLSeconds              = 300
	DefaultDiscoveryCacheTTLSeconds     = 15
	DefaultMaxRelaysPerResponse         = 100
	DefaultHeadscaleUser                = "goy-nodes"
	DefaultTailscaleKeyExpiryHours     = 24
	DefaultTailscaleKeyReusable        = false
	DefaultHeadscaleKeyExpiryHours     = 24
	DefaultHeadscaleKeyReusable        = false
)

var DefaultPublicPaths = []string{"/health", "/info", "/metrics"}

// DefaultConfig creates a Config instance with default values.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:              DefaultListen,
			ReadTimeoutSeconds:  DefaultReadTimeoutSeconds,
			WriteTimeoutSeconds: DefaultWriteTimeoutSeconds,
		},
		Database: DatabaseConfig{
			Path: DefaultDBPath,
		},
		Auth: AuthConfig{
			RequireAuth: DefaultRequireAuth,
			AdminAPIKey: "",
			HMACSecret:  "",
			PublicPaths: DefaultPublicPaths,
		},
		VPN: VPNConfig{
			Enabled:                 false,
			Provider:                "",
			TailscaleAPIKey:         "",
			TailscaleTailnet:        "",
			TailscaleTag:            "",
			TailscaleKeyExpiryHours: DefaultTailscaleKeyExpiryHours,
			TailscaleKeyReusable:    DefaultTailscaleKeyReusable,
			HeadscaleAPIURL:         "",
			HeadscaleAPIKey:         "",
			HeadscaleUser:           DefaultHeadscaleUser,
			HeadscaleKeyExpiryHours: DefaultHeadscaleKeyExpiryHours,
			HeadscaleKeyReusable:    DefaultHeadscaleKeyReusable,
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: DefaultRequestsPerMinute,
			Burst:             DefaultBurst,
			HeartbeatRPM:      DefaultHeartbeatRPM,
		},
		Jobs: JobsConfig{
			CleanupRelaysIntervalSeconds: DefaultCleanupRelaysIntervalSeconds,
			CleanupNodesIntervalSeconds:  DefaultCleanupNodesIntervalSeconds,
			StatsRefreshIntervalSeconds:  DefaultStatsRefreshIntervalSeconds,
			NodeInactiveThresholdHours:   DefaultNodeInactiveThresholdHours,
		},
		Registry: RegistryConfig{
			RelayTTLSeconds:          DefaultRelayTTLSeconds,
			DiscoveryCacheTTLSeconds: DefaultDiscoveryCacheTTLSeconds,
			MaxRelaysPerResponse:     DefaultMaxRelaysPerResponse,
		},
	}
}

// Load loads configuration from the specified TOML file, applying defaults and env var overrides.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := generateDefaultConfigFile(path); err != nil {
				_ = err
			}
		} else if err == nil {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("error reading configuration file %s: %w", path, err)
			}
			if err := toml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("error decoding TOML from %s: %w", path, err)
			}
		}
	}

	applyDefaults(cfg)
	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = DefaultListen
	}
	if cfg.Server.ReadTimeoutSeconds <= 0 {
		cfg.Server.ReadTimeoutSeconds = DefaultReadTimeoutSeconds
	}
	if cfg.Server.WriteTimeoutSeconds <= 0 {
		cfg.Server.WriteTimeoutSeconds = DefaultWriteTimeoutSeconds
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = DefaultDBPath
	}
	if len(cfg.Auth.PublicPaths) == 0 {
		cfg.Auth.PublicPaths = DefaultPublicPaths
	}
	if cfg.VPN.TailscaleKeyExpiryHours <= 0 {
		cfg.VPN.TailscaleKeyExpiryHours = DefaultTailscaleKeyExpiryHours
	}
	if cfg.VPN.HeadscaleUser == "" {
		cfg.VPN.HeadscaleUser = DefaultHeadscaleUser
	}
	if cfg.VPN.HeadscaleKeyExpiryHours <= 0 {
		cfg.VPN.HeadscaleKeyExpiryHours = DefaultHeadscaleKeyExpiryHours
	}
	if cfg.RateLimit.RequestsPerMinute <= 0 {
		cfg.RateLimit.RequestsPerMinute = DefaultRequestsPerMinute
	}
	if cfg.RateLimit.Burst <= 0 {
		cfg.RateLimit.Burst = DefaultBurst
	}
	if cfg.RateLimit.HeartbeatRPM <= 0 {
		cfg.RateLimit.HeartbeatRPM = DefaultHeartbeatRPM
	}
	if cfg.Jobs.CleanupRelaysIntervalSeconds <= 0 {
		cfg.Jobs.CleanupRelaysIntervalSeconds = DefaultCleanupRelaysIntervalSeconds
	}
	if cfg.Jobs.CleanupNodesIntervalSeconds <= 0 {
		cfg.Jobs.CleanupNodesIntervalSeconds = DefaultCleanupNodesIntervalSeconds
	}
	if cfg.Jobs.StatsRefreshIntervalSeconds <= 0 {
		cfg.Jobs.StatsRefreshIntervalSeconds = DefaultStatsRefreshIntervalSeconds
	}
	if cfg.Jobs.NodeInactiveThresholdHours <= 0 {
		cfg.Jobs.NodeInactiveThresholdHours = DefaultNodeInactiveThresholdHours
	}
	if cfg.Registry.RelayTTLSeconds <= 0 {
		cfg.Registry.RelayTTLSeconds = DefaultRelayTTLSeconds
	}
	if cfg.Registry.DiscoveryCacheTTLSeconds <= 0 {
		cfg.Registry.DiscoveryCacheTTLSeconds = DefaultDiscoveryCacheTTLSeconds
	}
	if cfg.Registry.MaxRelaysPerResponse <= 0 {
		cfg.Registry.MaxRelaysPerResponse = DefaultMaxRelaysPerResponse
	}
}

func applyEnvOverrides(cfg *Config) {
	if envListen := os.Getenv("COORD_LISTEN"); envListen != "" {
		cfg.Server.Listen = envListen
	}
	if envDBPath := os.Getenv("COORD_DB_PATH"); envDBPath != "" {
		cfg.Database.Path = envDBPath
	}

	// Auth Overrides
	if envReqAuth := os.Getenv("COORD_REQUIRE_AUTH"); envReqAuth != "" {
		cfg.Auth.RequireAuth = envReqAuth == "true" || envReqAuth == "1"
	}
	if envAdminKey := os.Getenv("COORD_ADMIN_API_KEY"); envAdminKey != "" {
		cfg.Auth.AdminAPIKey = envAdminKey
	}
	if envAuthSecret := os.Getenv("COORD_AUTH_SECRET"); envAuthSecret != "" {
		cfg.Auth.HMACSecret = envAuthSecret
	}

	// VPN Overrides
	if envVPNEnabled := os.Getenv("COORD_VPN_ENABLED"); envVPNEnabled != "" {
		cfg.VPN.Enabled = envVPNEnabled == "true" || envVPNEnabled == "1"
	}
	if envVPNProvider := os.Getenv("COORD_VPN_PROVIDER"); envVPNProvider != "" {
		cfg.VPN.Provider = envVPNProvider
	}

	// Tailscale Overrides
	if envTailscaleKey := os.Getenv("COORD_TAILSCALE_API_KEY"); envTailscaleKey != "" {
		cfg.VPN.TailscaleAPIKey = envTailscaleKey
	}
	if envTailscaleTailnet := os.Getenv("COORD_TAILSCALE_TAILNET"); envTailscaleTailnet != "" {
		cfg.VPN.TailscaleTailnet = envTailscaleTailnet
	}
	if envTailscaleTag := os.Getenv("COORD_TAILSCALE_TAG"); envTailscaleTag != "" {
		cfg.VPN.TailscaleTag = envTailscaleTag
	}
	if envTailscaleExpiry := os.Getenv("COORD_TAILSCALE_KEY_EXPIRY_HOURS"); envTailscaleExpiry != "" {
		if val, err := strconv.Atoi(envTailscaleExpiry); err == nil && val > 0 {
			cfg.VPN.TailscaleKeyExpiryHours = val
		}
	}
	if envTailscaleReusable := os.Getenv("COORD_TAILSCALE_KEY_REUSABLE"); envTailscaleReusable != "" {
		cfg.VPN.TailscaleKeyReusable = envTailscaleReusable == "true" || envTailscaleReusable == "1"
	}

	// Headscale Overrides
	if envHeadscaleURL := os.Getenv("COORD_HEADSCALE_API_URL"); envHeadscaleURL != "" {
		cfg.VPN.HeadscaleAPIURL = envHeadscaleURL
	}
	if envHeadscaleKey := os.Getenv("COORD_HEADSCALE_API_KEY"); envHeadscaleKey != "" {
		cfg.VPN.HeadscaleAPIKey = envHeadscaleKey
	}
	if envHeadscaleUser := os.Getenv("COORD_HEADSCALE_USER"); envHeadscaleUser != "" {
		cfg.VPN.HeadscaleUser = envHeadscaleUser
	}
	if envHeadscaleExpiry := os.Getenv("COORD_HEADSCALE_KEY_EXPIRY_HOURS"); envHeadscaleExpiry != "" {
		if val, err := strconv.Atoi(envHeadscaleExpiry); err == nil && val > 0 {
			cfg.VPN.HeadscaleKeyExpiryHours = val
		}
	}
	if envHeadscaleReusable := os.Getenv("COORD_HEADSCALE_KEY_REUSABLE"); envHeadscaleReusable != "" {
		cfg.VPN.HeadscaleKeyReusable = envHeadscaleReusable == "true" || envHeadscaleReusable == "1"
	}

	// Rate Limit Overrides
	if envRateLimit := os.Getenv("COORD_RATE_LIMIT_RPM"); envRateLimit != "" {
		if val, err := strconv.Atoi(envRateLimit); err == nil && val > 0 {
			cfg.RateLimit.RequestsPerMinute = val
		}
	}

	// Jobs Overrides
	if envCleanupRelays := os.Getenv("COORD_CLEANUP_RELAYS_INTERVAL"); envCleanupRelays != "" {
		if val, err := strconv.Atoi(envCleanupRelays); err == nil && val > 0 {
			cfg.Jobs.CleanupRelaysIntervalSeconds = val
		}
	}
	if envCleanupNodes := os.Getenv("COORD_CLEANUP_NODES_INTERVAL"); envCleanupNodes != "" {
		if val, err := strconv.Atoi(envCleanupNodes); err == nil && val > 0 {
			cfg.Jobs.CleanupNodesIntervalSeconds = val
		}
	}
	if envStatsRefresh := os.Getenv("COORD_STATS_REFRESH_INTERVAL"); envStatsRefresh != "" {
		if val, err := strconv.Atoi(envStatsRefresh); err == nil && val > 0 {
			cfg.Jobs.StatsRefreshIntervalSeconds = val
		}
	}
	if envInactiveThreshold := os.Getenv("COORD_NODE_INACTIVE_THRESHOLD_HOURS"); envInactiveThreshold != "" {
		if val, err := strconv.Atoi(envInactiveThreshold); err == nil && val > 0 {
			cfg.Jobs.NodeInactiveThresholdHours = val
		}
	}

	// Registry Overrides
	if envRelayTTL := os.Getenv("COORD_RELAY_TTL"); envRelayTTL != "" {
		if val, err := strconv.Atoi(envRelayTTL); err == nil && val > 0 {
			cfg.Registry.RelayTTLSeconds = val
		}
	}
	if envDiscoveryCacheTTL := os.Getenv("COORD_DISCOVERY_CACHE_TTL"); envDiscoveryCacheTTL != "" {
		if val, err := strconv.Atoi(envDiscoveryCacheTTL); err == nil && val > 0 {
			cfg.Registry.DiscoveryCacheTTLSeconds = val
		}
	}
}

// Validate verifies that required configuration fields are valid.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" {
		return errors.New("server.listen cannot be empty")
	}

	_, _, err := net.SplitHostPort(c.Server.Listen)
	if err != nil {
		return fmt.Errorf("invalid server.listen ('%s'): %w", c.Server.Listen, err)
	}

	if strings.TrimSpace(c.Database.Path) == "" {
		return errors.New("database.path cannot be empty")
	}

	if c.Auth.RequireAuth {
		if strings.TrimSpace(c.Auth.AdminAPIKey) == "" {
			return errors.New("auth.require_auth is true but COORD_ADMIN_API_KEY is not set in environment variables")
		}
	}

	if c.VPN.Enabled && c.VPN.Provider != "" {
		switch c.VPN.Provider {
		case "tailscale":
			if strings.TrimSpace(c.VPN.TailscaleAPIKey) == "" {
				return errors.New("vpn.provider is 'tailscale' but COORD_TAILSCALE_API_KEY is empty")
			}
			if strings.TrimSpace(c.VPN.TailscaleTailnet) == "" {
				return errors.New("vpn.provider is 'tailscale' but vpn.tailscale_tailnet (COORD_TAILSCALE_TAILNET) is empty")
			}
		case "headscale":
			if strings.TrimSpace(c.VPN.HeadscaleAPIURL) == "" {
				return errors.New("vpn.provider is 'headscale' but vpn.headscale_api_url (COORD_HEADSCALE_API_URL) is empty")
			}
			if strings.TrimSpace(c.VPN.HeadscaleAPIKey) == "" {
				return errors.New("vpn.provider is 'headscale' but COORD_HEADSCALE_API_KEY is empty")
			}
			if strings.TrimSpace(c.VPN.HeadscaleUser) == "" {
				return errors.New("vpn.provider is 'headscale' but vpn.headscale_user (COORD_HEADSCALE_USER) is empty")
			}
		default:
			return fmt.Errorf("invalid vpn.provider '%s': valid options are 'tailscale', 'headscale', ''", c.VPN.Provider)
		}
	}

	return nil
}

// generateDefaultConfigFile creates the config.toml file with explanatory comments.
func generateDefaultConfigFile(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	content := `# Coord Server Configuration (Goy Mesh Network)
# This file is automatically generated with default values.

[server]
# Address and port on which the HTTP server will listen.
listen = "0.0.0.0:8080"

# HTTP server timeouts (in seconds).
read_timeout_seconds = 15
write_timeout_seconds = 15

[database]
# Path to SQLite database file.
path = "data/coord-server.db"

[auth]
# Enable API key authentication on protected endpoints (default: true)
require_auth = true

# Public routes exempt from authentication
public_paths = ["/health", "/info", "/metrics"]

# Note: The administrator API key must NEVER be stored in this file.
# Set the COORD_ADMIN_API_KEY environment variable in your execution environment.

[vpn]
# Enable or disable VPN integration (default: false)
enabled = false

# Provider type: "tailscale" | "headscale" | "" (disabled)
provider = ""

# Tailscale config (used when provider = "tailscale")
tailscale_tailnet = ""
tailscale_tag = ""
tailscale_key_expiry_hours = 24
tailscale_key_reusable = false
# Note: COORD_TAILSCALE_API_KEY must be provided via environment variable.

# Headscale config (used when provider = "headscale")
headscale_api_url = ""
headscale_user = "goy-nodes"
headscale_key_expiry_hours = 24
headscale_key_reusable = false
# Note: COORD_HEADSCALE_API_KEY must be provided via environment variable.

[rate_limit]
# Global HTTP request limit per minute per IP
requests_per_minute = 60

# Maximum burst size allowed
burst = 30

# Specific limit for heartbeat requests (PUT /relays/{id}) per minute per IP
heartbeat_rpm = 120

[jobs]
# Execution interval for stale relay cleanup job (in seconds)
cleanup_relays_interval_seconds = 60

# Execution interval for inactive node cleanup job (in seconds)
cleanup_nodes_interval_seconds = 300

# Update interval for node and relay metrics and stats (in seconds)
stats_refresh_interval_seconds = 30

# Time threshold for a node without activity to be considered inactive (in hours)
node_inactive_threshold_hours = 24

[registry]
# Activity time window for a relay to be considered active (in seconds).
relay_ttl_seconds = 300

# In-memory cache TTL for GET /relays route (in seconds).
discovery_cache_ttl_seconds = 15

# Maximum number of relays returned per discovery response.
max_relays_per_response = 100
`

	return os.WriteFile(path, []byte(content), 0644)
}
