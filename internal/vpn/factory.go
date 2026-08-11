package vpn

import (
	"fmt"

	"github.com/goy-co/coord-server/internal/config"
)

// NewVPNProvider instantiates a VPNProvider based on the provided configuration.
func NewVPNProvider(cfg *config.VPNConfig) (VPNProvider, error) {
	if cfg == nil || !cfg.Enabled || cfg.Provider == "" {
		return NewNoopVPNProvider(), nil
	}

	switch cfg.Provider {
	case "tailscale":
		return NewTailscaleClient(cfg.TailscaleTailnet, cfg.TailscaleAPIKey, cfg.TailscaleTag), nil
	case "headscale":
		return NewHeadscaleClient(cfg.HeadscaleAPIURL, cfg.HeadscaleAPIKey, cfg.HeadscaleUser), nil
	default:
		return nil, fmt.Errorf("invalid vpn provider '%s': valid options are 'tailscale', 'headscale', ''", cfg.Provider)
	}
}
