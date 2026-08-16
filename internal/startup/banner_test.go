package startup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

type mockVPNProvider struct {
	healthCheckErr error
	keyResult      *vpn.VPNConfig
	keyErr         error
}

func (m *mockVPNProvider) CreatePreAuthKey(ctx context.Context, opts vpn.CreateKeyOpts) (*vpn.VPNConfig, error) {
	return m.keyResult, m.keyErr
}

func (m *mockVPNProvider) HealthCheck(ctx context.Context) error {
	return m.healthCheckErr
}

func (m *mockVPNProvider) GetStatus(ctx context.Context) (*vpn.VPNStatusResponse, error) {
	return &vpn.VPNStatusResponse{VPNEnabled: true}, nil
}

func (m *mockVPNProvider) ProviderName() string {
	return "mock"
}

func newTestDB(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	st := store.NewSQLiteStore(dbPath)
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	return st
}

func findStatus(statuses []ComponentStatus, name string) *ComponentStatus {
	for _, s := range statuses {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

func TestRunChecks_AllOK(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte("# test config"), 0o600)

	cfg := config.Defaults()
	cfg.Auth.AdminAPIKey = "test_key_12345"
	cfg.VPN.Provider = ""

	db := newTestDB(t)
	statuses := RunChecks(cfgPath, cfg, db, nil)

	for _, s := range statuses {
		if !s.OK {
			t.Errorf("expected %s to be OK, got warning: %s", s.Name, s.Warning)
		}
	}
}

func TestRunChecks_VPNHealthFail(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte("# test config"), 0o600)

	cfg := config.Defaults()
	cfg.Auth.AdminAPIKey = "test_key"
	cfg.VPN.Provider = "tailscale"

	mockVPN := &mockVPNProvider{healthCheckErr: fmt.Errorf("401 Unauthorized")}
	statuses := RunChecks(cfgPath, cfg, newTestDB(t), mockVPN)

	vpnStatus := findStatus(statuses, "VPN")
	if vpnStatus == nil {
		t.Fatal("VPN status not found")
	}
	if vpnStatus.OK {
		t.Error("expected VPN status to be not OK")
	}
	if !strings.Contains(vpnStatus.Warning, "401") {
		t.Errorf("expected warning to contain '401', got: %s", vpnStatus.Warning)
	}
}

func TestRunChecks_EmptyAdminKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte("# test config"), 0o600)

	cfg := config.Defaults()
	cfg.Auth.AdminAPIKey = ""

	statuses := RunChecks(cfgPath, cfg, newTestDB(t), nil)

	authStatus := findStatus(statuses, "Auth")
	if authStatus == nil {
		t.Fatal("Auth status not found")
	}
	if authStatus.OK {
		t.Error("expected Auth status to be not OK when key is empty")
	}
}

func TestPrintBanner_Formatting(t *testing.T) {
	statuses := []ComponentStatus{
		{Name: "Config", OK: true, Details: "/test/config.toml"},
		{Name: "VPN", OK: false, Details: "tailscale", Warning: "API unreachable"},
	}

	// Capturar stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintBanner("v0.2.0", "0.0.0.0:8080", statuses)

	w.Close()
	os.Stdout = old

	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "🟢 Goy Coord-Server v0.2.0") {
		t.Error("banner missing header")
	}
	if !strings.Contains(output, "✅") {
		t.Error("banner missing success icon")
	}
	if !strings.Contains(output, "❌") {
		t.Error("banner missing failure icon")
	}
	if !strings.Contains(output, "degraded") {
		t.Error("banner missing degraded warning")
	}
}

func TestBanner_VPNDisabledVsUnconfigured(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte("# test config"), 0o600)

	// Caso 1: VPN disabled (sem provider)
	cfgDisabled := config.Defaults()
	cfgDisabled.VPN.Provider = ""
	cfgDisabled.Auth.AdminAPIKey = "valid_key"

	statusesDisabled := RunChecks(cfgPath, cfgDisabled, newTestDB(t), nil)
	vpnStatusDisabled := findStatus(statusesDisabled, "VPN")
	if vpnStatusDisabled == nil || !vpnStatusDisabled.OK {
		t.Error("expected VPN to be OK when provider is disabled")
	}
	if !strings.Contains(vpnStatusDisabled.Details, "disabled") {
		t.Errorf("expected details to contain 'disabled', got: %s", vpnStatusDisabled.Details)
	}

	// Caso 2: VPN provider configurado mas nil
	cfgUnconfigured := config.Defaults()
	cfgUnconfigured.VPN.Provider = "tailscale"
	cfgUnconfigured.Auth.AdminAPIKey = "valid_key"

	statusesUnconfigured := RunChecks(cfgPath, cfgUnconfigured, newTestDB(t), nil)
	vpnStatusUnconfigured := findStatus(statusesUnconfigured, "VPN")
	if vpnStatusUnconfigured == nil || vpnStatusUnconfigured.OK {
		t.Error("expected VPN to be not OK when provider interface is nil")
	}
	if !strings.Contains(vpnStatusUnconfigured.Warning, "nil") {
		t.Errorf("expected warning to mention 'nil', got: %s", vpnStatusUnconfigured.Warning)
	}
}
