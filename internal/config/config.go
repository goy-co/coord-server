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

// ServerConfig possui as definições de rede e timeouts do servidor HTTP.
type ServerConfig struct {
	Listen              string `toml:"listen"`
	ReadTimeoutSeconds  int    `toml:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `toml:"write_timeout_seconds"`
}

// DatabaseConfig possui as definições de persistência de dados.
type DatabaseConfig struct {
	Path string `toml:"path"`
}

// AuthConfig possui as chaves e regras de autenticação da API.
type AuthConfig struct {
	RequireAuth bool     `toml:"require_auth"`
	AdminAPIKey string   `toml:"-"` // Nunca lido via TOML, apenas via env var COORD_ADMIN_API_KEY
	HMACSecret  string   `toml:"-"` // Nunca lido via TOML, apenas via env var COORD_AUTH_SECRET
	PublicPaths []string `toml:"public_paths"`
}

// VPNConfig possui os parâmetros de integração com a rede VPN / Headscale.
type VPNConfig struct {
	Enabled               bool   `toml:"enabled"`
	HeadscaleAPIURL       string `toml:"headscale_api_url"`
	HeadscaleAPIKey       string `toml:"-"` // Nunca lido via TOML, apenas via env var COORD_HEADSCALE_API_KEY
	HeadscaleUser         string `toml:"headscale_user"`
	PreAuthKeyExpiryHours int    `toml:"preauth_key_expiry_hours"`
	PreAuthKeyReusable    bool   `toml:"preauth_key_reusable"`
}

// RateLimitConfig possui as regras de limitação de taxa de pedidos HTTP.
type RateLimitConfig struct {
	RequestsPerMinute int `toml:"requests_per_minute"`
	Burst             int `toml:"burst"`
	HeartbeatRPM      int `toml:"heartbeat_rpm"`
}

// JobsConfig possui os intervalos e regras de execução de background jobs.
type JobsConfig struct {
	CleanupRelaysIntervalSeconds int `toml:"cleanup_relays_interval_seconds"`
	CleanupNodesIntervalSeconds  int `toml:"cleanup_nodes_interval_seconds"`
	StatsRefreshIntervalSeconds  int `toml:"stats_refresh_interval_seconds"`
	NodeInactiveThresholdHours   int `toml:"node_inactive_threshold_hours"`
}

// RegistryConfig possui as definições do serviço de descoberta de relays.
type RegistryConfig struct {
	RelayTTLSeconds          int `toml:"relay_ttl_seconds"`
	DiscoveryCacheTTLSeconds int `toml:"discovery_cache_ttl_seconds"`
	MaxRelaysPerResponse     int `toml:"max_relays_per_response"`
}

// Config agrega todas as secções de configuração do coord-server.
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
	DefaultBurst                        = 10
	DefaultHeartbeatRPM                 = 120
	DefaultCleanupRelaysIntervalSeconds = 60
	DefaultCleanupNodesIntervalSeconds  = 300
	DefaultStatsRefreshIntervalSeconds  = 30
	DefaultNodeInactiveThresholdHours   = 24
	DefaultRelayTTLSeconds              = 300
	DefaultDiscoveryCacheTTLSeconds     = 15
	DefaultMaxRelaysPerResponse         = 100
	DefaultHeadscaleUser                = "goy-nodes"
	DefaultPreAuthKeyExpiryHours        = 24
	DefaultPreAuthKeyReusable           = false
)

var DefaultPublicPaths = []string{"/health", "/info", "/metrics"}

// DefaultConfig cria uma instância de Config com os valores por omissão.
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
			Enabled:               false,
			HeadscaleAPIURL:       "",
			HeadscaleAPIKey:       "",
			HeadscaleUser:         DefaultHeadscaleUser,
			PreAuthKeyExpiryHours: DefaultPreAuthKeyExpiryHours,
			PreAuthKeyReusable:    DefaultPreAuthKeyReusable,
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

// Load carrega a configuração a partir do ficheiro TOML indicado, aplica defaults e overrides de env vars.
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
				return nil, fmt.Errorf("erro ao ler ficheiro de configuração %s: %w", path, err)
			}
			if err := toml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("erro ao descodificar TOML de %s: %w", path, err)
			}
		}
	}

	applyDefaults(cfg)
	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validação de configuração falhou: %w", err)
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
	if cfg.VPN.HeadscaleUser == "" {
		cfg.VPN.HeadscaleUser = DefaultHeadscaleUser
	}
	if cfg.VPN.PreAuthKeyExpiryHours <= 0 {
		cfg.VPN.PreAuthKeyExpiryHours = DefaultPreAuthKeyExpiryHours
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
	if envHeadscaleURL := os.Getenv("COORD_HEADSCALE_API_URL"); envHeadscaleURL != "" {
		cfg.VPN.HeadscaleAPIURL = envHeadscaleURL
	}
	if envHeadscaleKey := os.Getenv("COORD_HEADSCALE_API_KEY"); envHeadscaleKey != "" {
		cfg.VPN.HeadscaleAPIKey = envHeadscaleKey
	}
	if envHeadscaleUser := os.Getenv("COORD_HEADSCALE_USER"); envHeadscaleUser != "" {
		cfg.VPN.HeadscaleUser = envHeadscaleUser
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

// Validate verifica se a configuração tem os campos obrigatórios válidos.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" {
		return errors.New("server.listen não pode estar vazio")
	}

	_, _, err := net.SplitHostPort(c.Server.Listen)
	if err != nil {
		return fmt.Errorf("server.listen inválido ('%s'): %w", c.Server.Listen, err)
	}

	if strings.TrimSpace(c.Database.Path) == "" {
		return errors.New("database.path não pode estar vazio")
	}

	if c.Auth.RequireAuth {
		if strings.TrimSpace(c.Auth.AdminAPIKey) == "" {
			return errors.New("auth.require_auth é true mas COORD_ADMIN_API_KEY não foi definida nas variáveis de ambiente")
		}
	}

	if c.VPN.Enabled {
		if strings.TrimSpace(c.VPN.HeadscaleAPIURL) == "" {
			return errors.New("vpn.enabled é true mas vpn.headscale_api_url (COORD_HEADSCALE_API_URL) está vazio")
		}
		if strings.TrimSpace(c.VPN.HeadscaleAPIKey) == "" {
			return errors.New("vpn.enabled é true mas COORD_HEADSCALE_API_KEY está vazio")
		}
	}

	return nil
}

// generateDefaultConfigFile cria o ficheiro config.toml com comentários explicativos em português.
func generateDefaultConfigFile(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	content := `# Configuração do Coord Server (Goy Mesh Network)
# Este ficheiro é gerado automaticamente com os valores por omissão.

[server]
# Endereço e porta em que o servidor HTTP vai escutar.
listen = "0.0.0.0:8080"

# Timeouts do servidor HTTP (em segundos).
read_timeout_seconds = 15
write_timeout_seconds = 15

[database]
# Caminho para o ficheiro de base de dados SQLite.
path = "data/coord-server.db"

[auth]
# Ativar autenticação por API key em endpoints protegidos (default: true)
require_auth = true

# Rotas públicas isentas de autenticação
public_paths = ["/health", "/info", "/metrics"]

# Nota: A chave de API do administrador NUNCA deve ser guardada neste ficheiro.
# Defina a variável de ambiente COORD_ADMIN_API_KEY no ambiente de execução.

[vpn]
# Ativar ou desativar integração com Headscale (default: false)
enabled = false

# URL base da API Headscale (ex: https://vpn.goyco.xyz)
headscale_api_url = ""

# Usuário/Namespace onde os nós são registados no Headscale
headscale_user = "goy-nodes"

# Validade das pre-auth keys geradas (em horas)
preauth_key_expiry_hours = 24

# Se as pre-auth keys geradas podem ser reutilizadas
preauth_key_reusable = false

# Nota: A chave de API do Headscale deve ser fornecida via variável de ambiente COORD_HEADSCALE_API_KEY.

[rate_limit]
# Limite global de pedidos HTTP por minuto por IP
requests_per_minute = 60

# Tamanho do picos curtos (burst) permitidos
burst = 10

# Limite específico para pedidos de heartbeat (/relays/{id} PUT) por minuto por IP
heartbeat_rpm = 120

[jobs]
# Intervalo de execução do job de limpeza de relays stale (em segundos)
cleanup_relays_interval_seconds = 60

# Intervalo de execução do job de verificação de nós inativos (em segundos)
cleanup_nodes_interval_seconds = 300

# Intervalo de atualização das estatísticas e métricas de nós e relays (em segundos)
stats_refresh_interval_seconds = 30

# Threshold temporal para um nó sem atividade ser considerado inativo (em horas)
node_inactive_threshold_hours = 24

[registry]
# Janela temporal de atividade para um relay ser considerado ativo (em segundos).
relay_ttl_seconds = 300

# TTL do cache em memória para a rota GET /relays (em segundos).
discovery_cache_ttl_seconds = 15

# Número máximo de relays retornados por resposta de discovery.
max_relays_per_response = 100
`

	return os.WriteFile(path, []byte(content), 0644)
}
