package vpn_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/goy-co/coord-server/internal/vpn"
)

func TestTailscaleClient_CreatePreAuthKey_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/tailnet/my-org.ts.net/keys" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
			t.Errorf("Unexpected Authorization header: %s", auth)
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		expiry, ok := reqBody["expirySeconds"].(float64)
		if !ok || expiry != 86400 {
			t.Errorf("Expected expirySeconds 86400, got %v", reqBody["expirySeconds"])
		}

		caps, _ := reqBody["capabilities"].(map[string]any)
		devices, _ := caps["devices"].(map[string]any)
		create, _ := devices["create"].(map[string]any)
		tags, _ := create["tags"].([]any)

		if len(tags) != 1 || tags[0] != "tag:goy-node" {
			t.Errorf("Expected tags ['tag:goy-node'], got %v", tags)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "k123456789",
			"key": "tskey-auth-mock-9999",
			"created": "2026-08-11T00:00:00Z",
			"expires": "2026-08-12T00:00:00Z"
		}`))
	}))
	defer server.Close()

	client := vpn.NewTailscaleClient("my-org.ts.net", "test-api-key", "tag:default")
	setTailscaleBaseURL(client, server.URL)

	opts := vpn.CreateKeyOpts{
		Reusable:    false,
		ExpiryHours: 24,
		Tags:        []string{"tag:goy-node"},
	}

	cfg, err := client.CreatePreAuthKey(context.Background(), opts)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.AuthKey != "tskey-auth-mock-9999" {
		t.Errorf("Expected auth_key 'tskey-auth-mock-9999', got '%s'", cfg.AuthKey)
	}
	if cfg.ControlURL != "" {
		t.Errorf("Expected empty control_url for Tailscale, got '%s'", cfg.ControlURL)
	}
	if cfg.Provider != "tailscale" {
		t.Errorf("Expected provider 'tailscale', got '%s'", cfg.Provider)
	}
}

func TestTailscaleClient_CreatePreAuthKey_401Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "invalid key"}`))
	}))
	defer server.Close()

	client := vpn.NewTailscaleClient("my-org.ts.net", "bad-key", "")
	setTailscaleBaseURL(client, server.URL)

	_, err := client.CreatePreAuthKey(context.Background(), vpn.CreateKeyOpts{ExpiryHours: 12})
	if err == nil {
		t.Fatal("Expected error on 401, got nil")
	}
}

func TestTailscaleClient_CreatePreAuthKey_Retry5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": "k1", "key": "tskey-auth-retry-ok"}`))
	}))
	defer server.Close()

	client := vpn.NewTailscaleClient("my-org.ts.net", "key", "")
	setTailscaleBaseURL(client, server.URL)

	cfg, err := client.CreatePreAuthKey(context.Background(), vpn.CreateKeyOpts{ExpiryHours: 1})
	if err != nil {
		t.Fatalf("Expected success on retry, got err: %v", err)
	}
	if cfg.AuthKey != "tskey-auth-retry-ok" {
		t.Errorf("Unexpected auth key: %s", cfg.AuthKey)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestTailscaleClient_HealthCheck_SuccessAndError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer valid-key" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"devices": [{"id": "d1"}]}`))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()

	clientValid := vpn.NewTailscaleClient("my-org.ts.net", "valid-key", "")
	setTailscaleBaseURL(clientValid, server.URL)

	if err := clientValid.HealthCheck(context.Background()); err != nil {
		t.Errorf("Expected HealthCheck success, got: %v", err)
	}

	clientInvalid := vpn.NewTailscaleClient("my-org.ts.net", "invalid-key", "")
	setTailscaleBaseURL(clientInvalid, server.URL)

	if err := clientInvalid.HealthCheck(context.Background()); err == nil {
		t.Error("Expected HealthCheck error for invalid key, got nil")
	}
}

func TestTailscaleClient_GetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"devices": [{"id": "d1"}, {"id": "d2"}]}`))
	}))
	defer server.Close()

	client := vpn.NewTailscaleClient("my-org.ts.net", "valid-key", "")
	setTailscaleBaseURL(client, server.URL)

	status, err := client.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus unexpected error: %v", err)
	}
	if !status.VPNEnabled {
		t.Error("Expected VPNEnabled true")
	}
	if status.Provider != "tailscale" {
		t.Errorf("Expected provider 'tailscale', got '%s'", status.Provider)
	}
	if status.TailscaleReachable == nil || !*status.TailscaleReachable {
		t.Error("Expected TailscaleReachable true")
	}
	if status.RegisteredDevices != 2 {
		t.Errorf("Expected RegisteredDevices 2, got %d", status.RegisteredDevices)
	}
}

// setTailscaleBaseURL helper via URL replacement or exportable internal mechanism
func setTailscaleBaseURL(client *vpn.TailscaleClient, targetURL string) {
	// In Go tests across packages, we can use reflection or set BaseURL if unexported.
	// Since client is constructed with https://api.tailscale.com, let's export setBaseURL in internal/vpn package export test helper.
	vpn.SetTailscaleClientBaseURL(client, targetURL)
}
