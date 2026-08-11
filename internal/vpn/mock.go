package vpn

import "context"

// NoopVPNProvider is a no-op implementation used when VPN integration is disabled.
type NoopVPNProvider struct{}

func NewNoopVPNProvider() *NoopVPNProvider {
	return &NoopVPNProvider{}
}

func (p *NoopVPNProvider) ProviderName() string {
	return ""
}

func (p *NoopVPNProvider) CreatePreAuthKey(ctx context.Context, opts CreateKeyOpts) (*VPNConfig, error) {
	return &VPNConfig{
		AuthKey:    "",
		ControlURL: "",
		Provider:   "",
	}, nil
}

func (p *NoopVPNProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func (p *NoopVPNProvider) GetStatus(ctx context.Context) (*VPNStatusResponse, error) {
	return &VPNStatusResponse{
		VPNEnabled:        false,
		Provider:          "",
		RegisteredDevices: 0,
	}, nil
}

// MockVPNProvider is a configurable mock implementation used in unit and integration tests.
type MockVPNProvider struct {
	Provider           string
	KeyToReturn        string
	ErrToReturn        error
	ControlURL         string
	Reachable          bool
	RegisteredMachines int
}

func NewMockVPNProvider(key string, err error) *MockVPNProvider {
	return &MockVPNProvider{
		Provider:           "headscale",
		KeyToReturn:        key,
		ErrToReturn:        err,
		ControlURL:         "https://vpn.goy.test",
		Reachable:          true,
		RegisteredMachines: 3,
	}
}

func (m *MockVPNProvider) ProviderName() string {
	if m.Provider != "" {
		return m.Provider
	}
	return "headscale"
}

func (m *MockVPNProvider) CreatePreAuthKey(ctx context.Context, opts CreateKeyOpts) (*VPNConfig, error) {
	if m.ErrToReturn != nil {
		return nil, m.ErrToReturn
	}
	ctrlURL := m.ControlURL
	if m.ProviderName() == "tailscale" {
		ctrlURL = ""
	}
	return &VPNConfig{
		AuthKey:    m.KeyToReturn,
		ControlURL: ctrlURL,
		Provider:   m.ProviderName(),
	}, nil
}

func (m *MockVPNProvider) HealthCheck(ctx context.Context) error {
	if !m.Reachable {
		return m.ErrToReturn
	}
	return nil
}

func (m *MockVPNProvider) GetStatus(ctx context.Context) (*VPNStatusResponse, error) {
	reachable := m.Reachable
	resp := &VPNStatusResponse{
		VPNEnabled:        true,
		Provider:          m.ProviderName(),
		RegisteredDevices: m.RegisteredMachines,
	}
	if m.ProviderName() == "headscale" {
		resp.HeadscaleReachable = &reachable
		resp.HeadscaleUser = "goy-nodes-test"
	} else if m.ProviderName() == "tailscale" {
		resp.TailscaleReachable = &reachable
		resp.Tailnet = "my-org.ts.net"
	}
	return resp, nil
}
