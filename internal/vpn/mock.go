package vpn

import "context"

// NoopVPNProvider is a no-op implementation used when VPN integration is disabled.
type NoopVPNProvider struct{}

func NewNoopVPNProvider() *NoopVPNProvider {
	return &NoopVPNProvider{}
}

func (p *NoopVPNProvider) CreatePreAuthKey(ctx context.Context, reusable bool, expiryHours int) (string, error) {
	return "", nil
}

func (p *NoopVPNProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func (p *NoopVPNProvider) GetStatus(ctx context.Context) (*VPNStatusResponse, error) {
	return &VPNStatusResponse{
		VPNEnabled:         false,
		HeadscaleReachable: false,
		HeadscaleUser:      "",
		RegisteredMachines: 0,
	}, nil
}

func (p *NoopVPNProvider) GetControlURL() string {
	return ""
}

// MockVPNProvider is a configurable mock implementation used in unit and integration tests.
type MockVPNProvider struct {
	KeyToReturn        string
	ErrToReturn        error
	ControlURL         string
	Reachable          bool
	RegisteredMachines int
}

func NewMockVPNProvider(key string, err error) *MockVPNProvider {
	return &MockVPNProvider{
		KeyToReturn:        key,
		ErrToReturn:        err,
		ControlURL:         "https://vpn.goy.test",
		Reachable:          true,
		RegisteredMachines: 3,
	}
}

func (m *MockVPNProvider) CreatePreAuthKey(ctx context.Context, reusable bool, expiryHours int) (string, error) {
	if m.ErrToReturn != nil {
		return "", m.ErrToReturn
	}
	return m.KeyToReturn, nil
}

func (m *MockVPNProvider) HealthCheck(ctx context.Context) error {
	if !m.Reachable {
		return m.ErrToReturn
	}
	return nil
}

func (m *MockVPNProvider) GetStatus(ctx context.Context) (*VPNStatusResponse, error) {
	return &VPNStatusResponse{
		VPNEnabled:         true,
		HeadscaleReachable: m.Reachable,
		HeadscaleUser:      "goy-nodes-test",
		RegisteredMachines: m.RegisteredMachines,
	}, nil
}

func (m *MockVPNProvider) GetControlURL() string {
	return m.ControlURL
}
