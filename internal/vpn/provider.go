package vpn

import "context"

// VPNStatusResponse representa a resposta do endpoint de diagnóstico da VPN (/v1/vpn/status).
type VPNStatusResponse struct {
	VPNEnabled         bool   `json:"vpn_enabled"`
	HeadscaleReachable bool   `json:"headscale_reachable"`
	HeadscaleUser      string `json:"headscale_user,omitempty"`
	RegisteredMachines int    `json:"registered_machines"`
}

// VPNProvider define a interface abstrata para operações VPN / Headscale.
type VPNProvider interface {
	// CreatePreAuthKey gera uma nova pre-auth key no Headscale para onboarding de um nó.
	CreatePreAuthKey(ctx context.Context, reusable bool, expiryHours int) (string, error)

	// HealthCheck verifica a conectividade à API Headscale.
	HealthCheck(ctx context.Context) error

	// GetStatus retorna métricas de diagnóstico do serviço VPN.
	GetStatus(ctx context.Context) (*VPNStatusResponse, error)

	// GetControlURL retorna o URL de controlo VPN exposto aos nós.
	GetControlURL() string
}
