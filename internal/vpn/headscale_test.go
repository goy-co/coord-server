package vpn_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goy-co/coord-server/internal/vpn"
)

func TestHeadscaleClientCreatePreAuthKeySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/preauthkey" {
			t.Fatalf("Unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Fatalf("Incorrect Authorization header: %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"preAuthKey": {"key": "tskey-auth-1234567890", "expiration": "2026-08-11T12:00:00Z"}}`))
	}))
	defer server.Close()

	client := vpn.NewHeadscaleClient(server.URL, "test-api-key", "goy-nodes")
	ctx := context.Background()

	opts := vpn.CreateKeyOpts{
		Reusable:    false,
		ExpiryHours: 24,
	}

	cfg, err := client.CreatePreAuthKey(ctx, opts)
	if err != nil {
		t.Fatalf("Expected success generating pre-auth key, got error: %v", err)
	}

	if cfg.AuthKey != "tskey-auth-1234567890" {
		t.Errorf("Expected key 'tskey-auth-1234567890', got: '%s'", cfg.AuthKey)
	}

	if cfg.ControlURL != server.URL {
		t.Errorf("Expected ControlURL %s, got: %s", server.URL, cfg.ControlURL)
	}

	if cfg.Provider != "headscale" {
		t.Errorf("Expected provider 'headscale', got: '%s'", cfg.Provider)
	}

	if client.GetControlURL() != server.URL {
		t.Errorf("Expected GetControlURL %s, got: %s", server.URL, client.GetControlURL())
	}
}

func TestHeadscaleClientCreatePreAuthKeyUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := vpn.NewHeadscaleClient(server.URL, "invalid-key", "goy-nodes")
	ctx := context.Background()

	opts := vpn.CreateKeyOpts{
		Reusable:    false,
		ExpiryHours: 24,
	}

	_, err := client.CreatePreAuthKey(ctx, opts)
	if err == nil {
		t.Fatalf("Expected error for unauthorized API key")
	}
}

func TestHeadscaleClientRetryOnServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"preAuthKey": {"key": "tskey-auth-recovered"}}`))
	}))
	defer server.Close()

	client := vpn.NewHeadscaleClient(server.URL, "test-key", "goy-nodes")
	ctx := context.Background()

	opts := vpn.CreateKeyOpts{
		Reusable:    false,
		ExpiryHours: 24,
	}

	cfg, err := client.CreatePreAuthKey(ctx, opts)
	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if cfg.AuthKey != "tskey-auth-recovered" {
		t.Errorf("Expected key 'tskey-auth-recovered', got: '%s'", cfg.AuthKey)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 HTTP attempts, got: %d", attempts)
	}
}
