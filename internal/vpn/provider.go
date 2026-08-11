package vpn

import "context"

// CreateKeyOpts holds parameters for key creation across VPN providers.
type CreateKeyOpts struct {
	Reusable    bool
	ExpiryHours int
	Tags        []string // Relevant for Tailscale (ACL tags), ignored by Headscale
}

// VPNConfig contains the generated key and endpoint details returned to nodes upon registration.
type VPNConfig struct {
	AuthKey    string `json:"auth_key"`
	ControlURL string `json:"control_url"` // Empty for Tailscale, filled for Headscale
	Provider   string `json:"provider"`    // "tailscale" | "headscale"
}

// VPNStatusResponse represents the response for the VPN diagnostic endpoint (/v1/vpn/status).
type VPNStatusResponse struct {
	VPNEnabled         bool   `json:"vpn_enabled"`
	Provider           string `json:"provider,omitempty"`
	HeadscaleReachable *bool  `json:"headscale_reachable"`
	HeadscaleUser      string `json:"headscale_user,omitempty"`
	TailscaleReachable *bool  `json:"tailscale_reachable"`
	Tailnet            string `json:"tailnet,omitempty"`
	RegisteredDevices  int    `json:"registered_devices"`
}

// VPNProvider defines the abstract interface for VPN operations (Tailscale/Headscale).
type VPNProvider interface {
	// CreatePreAuthKey generates an authentication key for a new node onboarding.
	CreatePreAuthKey(ctx context.Context, opts CreateKeyOpts) (*VPNConfig, error)

	// HealthCheck checks connectivity to the VPN provider API.
	HealthCheck(ctx context.Context) error

	// GetStatus returns diagnostic status of the VPN service.
	GetStatus(ctx context.Context) (*VPNStatusResponse, error)

	// ProviderName returns the provider identifier: "tailscale", "headscale", or "" (disabled).
	ProviderName() string
}
