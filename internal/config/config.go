package config

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	DefaultListen                       = "0.0.0.0:8080"
	DefaultDBPath                       = "/var/lib/goy-coord/coord.db"
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
	DefaultOnlineThresholdSeconds       = 180
	DefaultHeadscaleUser                = "goy-nodes"
	DefaultTailscaleKeyExpiryHours     = 24
	DefaultTailscaleKeyReusable        = false
	DefaultHeadscaleKeyExpiryHours     = 24
	DefaultHeadscaleKeyReusable        = false
)

var DefaultPublicPaths = []string{"/health", "/info", "/metrics"}

// DefaultConfig is an alias to Defaults for backwards compatibility.
func DefaultConfig() *Config {
	return Defaults()
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Bind == "" && cfg.Server.Listen == "" {
		cfg.Server.Bind = DefaultListen
		cfg.Server.Listen = DefaultListen
	} else if cfg.Server.Bind == "" {
		cfg.Server.Bind = cfg.Server.Listen
	} else if cfg.Server.Listen == "" {
		cfg.Server.Listen = cfg.Server.Bind
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
	if cfg.Registry.OnlineThresholdSeconds <= 0 {
		cfg.Registry.OnlineThresholdSeconds = DefaultOnlineThresholdSeconds
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
}

func applyEnvOverrides(cfg *Config) {
	if envListen := os.Getenv("COORD_LISTEN"); envListen != "" {
		cfg.Server.Listen = envListen
		cfg.Server.Bind = envListen
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
		cfg.VPN.HeadscaleURL = envHeadscaleURL
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
	if envOnlineThreshold := os.Getenv("COORD_ONLINE_THRESHOLD_SECS"); envOnlineThreshold != "" {
		if val, err := strconv.Atoi(envOnlineThreshold); err == nil && val > 0 {
			cfg.Registry.OnlineThresholdSeconds = val
		}
	}

	// Log Overrides
	if envLogLevel := os.Getenv("COORD_LOG_LEVEL"); envLogLevel != "" {
		cfg.Log.Level = envLogLevel
	}
	if envLogFormat := os.Getenv("COORD_LOG_FORMAT"); envLogFormat != "" {
		cfg.Log.Format = envLogFormat
	}
}

const defaultConfigFileTemplate = `# =====================================================================
# Goy Coord-Server Configuration
# Generated by: coord-server --generate-config
# Docs: https://docs.goy.company/coord-server/config
# =====================================================================

[server]
bind = "0.0.0.0:8080"

[auth]
# Admin API key para proteger endpoints administrativos.
# Gerada automaticamente no primeiro run se vazia.
admin_api_key = ""

[database]
path = "/var/lib/goy-coord/coord.db"

[vpn]
# Provider VPN: "tailscale", "headscale", ou "" (sem VPN)
provider = ""

# Tailscale (se provider = "tailscale")
# tailscale_api_key = "tskey-api-..."
# tailscale_tailnet = "example.com"

# Headscale (se provider = "headscale")
# headscale_url = "https://headscale.example.com"
# headscale_api_key = "..."
# headscale_user = "goy"

# Expiração das pre-auth keys em horas
key_expiry_hours = 168

[ratelimit]
requests_per_minute = 60

[log]
level = "info"
format = "json"
`

// generateDefaultConfigFile creates the config.toml file with default values.
func generateDefaultConfigFile(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(defaultConfigFileTemplate), 0644)
}
