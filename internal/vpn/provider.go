package vpn

import "context"

// VPNStatusResponse represents the response for the VPN diagnostic endpoint (/v1/vpn/status).
type VPNStatusResponse struct {
	VPNEnabled         bool   `json:"vpn_enabled"`
	HeadscaleReachable bool   `json:"headscale_reachable"`
	HeadscaleUser      string `json:"headscale_user,omitempty"`
	RegisteredMachines int    `json:"registered_machines"`
}

// VPNProvider defines the abstract interface for VPN / Headscale operations.
type VPNProvider interface {
	// CreatePreAuthKey generates a new pre-auth key in Headscale for node onboarding.
	CreatePreAuthKey(ctx context.Context, reusable bool, expiryHours int) (string, error)

	// HealthCheck checks connectivity to the Headscale API.
	HealthCheck(ctx context.Context) error

	// GetStatus returns diagnostic metrics of the VPN service.
	GetStatus(ctx context.Context) (*VPNStatusResponse, error)

	// GetControlURL returns the VPN control URL exposed to nodes.
	GetControlURL() string
}
