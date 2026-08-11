package vpn_test

import (
	"testing"

	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/vpn"
)

func TestNewVPNProvider(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *config.VPNConfig
		wantProvider string
		wantErr      bool
	}{
		{
			name: "Disabled VPN",
			cfg: &config.VPNConfig{
				Enabled:  false,
				Provider: "tailscale",
			},
			wantProvider: "",
			wantErr:      false,
		},
		{
			name: "Empty Provider",
			cfg: &config.VPNConfig{
				Enabled:  true,
				Provider: "",
			},
			wantProvider: "",
			wantErr:      false,
		},
		{
			name: "Tailscale Provider",
			cfg: &config.VPNConfig{
				Enabled:          true,
				Provider:         "tailscale",
				TailscaleAPIKey:  "ts-key",
				TailscaleTailnet: "my-net.ts.net",
			},
			wantProvider: "tailscale",
			wantErr:      false,
		},
		{
			name: "Headscale Provider",
			cfg: &config.VPNConfig{
				Enabled:         true,
				Provider:        "headscale",
				HeadscaleAPIURL: "https://vpn.goy.test",
				HeadscaleAPIKey: "hs-key",
				HeadscaleUser:   "goy-nodes",
			},
			wantProvider: "headscale",
			wantErr:      false,
		},
		{
			name: "Invalid Provider",
			cfg: &config.VPNConfig{
				Enabled:  true,
				Provider: "wireguard",
			},
			wantProvider: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := vpn.NewVPNProvider(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewVPNProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && provider.ProviderName() != tt.wantProvider {
				t.Errorf("ProviderName() = %v, want %v", provider.ProviderName(), tt.wantProvider)
			}
		})
	}
}
