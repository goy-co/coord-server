package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/goy-co/coord-server/internal/api"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

func TestNodeEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api_nodes.db")

	st := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := st.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer st.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = true
	cfg.Auth.AdminAPIKey = "valid-test-key"

	router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

	var registeredNodeID string
	authKey := "gc_12345678901234567890"

	// 0. Request without Authorization Header -> 401 Unauthorized
	t.Run("Register Node Unauthorized Missing Header", func(t *testing.T) {
		body := api.RegisterNodeRequest{
			AuthKey: authKey,
			Name:    "unauth-node",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected status 401 for request without header, got: %d", rec.Code)
		}
	})

	// 1. POST /v1/nodes/register with Invalid Auth Key -> 400 Bad Request
	t.Run("Register Invalid Auth Key", func(t *testing.T) {
		body := api.RegisterNodeRequest{
			AuthKey: "short_key",
			Name:    "bad-node",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-test-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected status 400, got: %d", rec.Code)
		}

		var errResp api.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Error != "invalid request" {
			t.Errorf("Unexpected error response: %+v", errResp)
		}
	})

	// 2. POST /v1/nodes/register Success -> 201 Created
	t.Run("Register Valid Node", func(t *testing.T) {
		body := api.RegisterNodeRequest{
			AuthKey: authKey,
			Name:    "my-test-node",
			Storage: &api.StoragePayload{
				ReservedGB:  100,
				AvailableGB: 50,
			},
			MeshURL: "100.64.0.5:8443",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-test-key")
		req.Host = "coord.test:8080"
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got: %d (body: %s)", rec.Code, rec.Body.String())
		}

		var resp api.RegisterNodeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to deserialize JSON: %v", err)
		}

		if resp.NodeID == "" || resp.Name != "my-test-node" || resp.MeshURL != "100.64.0.5:8443" {
			t.Errorf("Incorrect response fields: %+v", resp)
		}

		if resp.RegistryURL != "http://coord.test:8080" {
			t.Errorf("Expected RegistryURL 'http://coord.test:8080', got: '%s'", resp.RegistryURL)
		}

		registeredNodeID = resp.NodeID
	})

	// 3. POST /v1/nodes/register Idempotent (same Auth Key) -> 200 OK
	t.Run("Register Idempotent Node", func(t *testing.T) {
		body := api.RegisterNodeRequest{
			AuthKey: authKey,
			Name:    "duplicate-register",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-test-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for idempotent registration, got: %d", rec.Code)
		}

		var resp api.RegisterNodeResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		if resp.NodeID != registeredNodeID {
			t.Errorf("NodeID should match previous registration (%s), got: %s", registeredNodeID, resp.NodeID)
		}
	})

	// 4. GET /v1/nodes/{id} Success -> 200 OK
	t.Run("Get Node Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/"+registeredNodeID, nil)
		req.Header.Set("Authorization", "Bearer valid-test-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d", rec.Code)
		}

		var node store.Node
		if err := json.Unmarshal(rec.Body.Bytes(), &node); err != nil {
			t.Fatalf("Failed to deserialize node: %v", err)
		}

		if node.ID != registeredNodeID || node.Name != "my-test-node" {
			t.Errorf("Incorrect node data: %+v", node)
		}
	})

	// 5. GET /v1/nodes/{nonexistent} -> 404 Not Found
	t.Run("Get Nonexistent Node", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-nonexistent", nil)
		req.Header.Set("Authorization", "Bearer valid-test-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("Expected status 404, got: %d", rec.Code)
		}

		var errResp api.ErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Error != "not found" || errResp.ID != "goy-node-nonexistent" {
			t.Errorf("Incorrect 404 error response: %+v", errResp)
		}
	})

	// 6. DELETE /v1/nodes/{id} -> 204 No Content
	t.Run("Delete Node Success", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/nodes/"+registeredNodeID, nil)
		req.Header.Set("Authorization", "Bearer valid-test-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("Expected status 204, got: %d", rec.Code)
		}

		getReq := httptest.NewRequest("GET", "/v1/nodes/"+registeredNodeID, nil)
		getReq.Header.Set("Authorization", "Bearer valid-test-key")
		getRec := httptest.NewRecorder()

		router.ServeHTTP(getRec, getReq)

		if getRec.Code != http.StatusNotFound {
			t.Fatalf("Expected status 404 after soft delete, got: %d", getRec.Code)
		}
	})

	// 7. DELETE /v1/nodes/{nonexistent} -> 404 Not Found
	t.Run("Delete Nonexistent Node", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/nodes/goy-node-nonexistent", nil)
		req.Header.Set("Authorization", "Bearer valid-test-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("Expected status 404, got: %d", rec.Code)
		}
	})
}

func TestNodeRegisterVPNIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api_nodes_vpn.db")

	st := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := st.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer st.Close()

	t.Run("Register Node with Headscale VPN Enabled & Valid Key", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = true
		cfg.Auth.AdminAPIKey = "valid-hs-admin-key"
		cfg.VPN.Enabled = true
		cfg.VPN.Provider = "headscale"
		cfg.VPN.HeadscaleAPIURL = "https://vpn.goy.test"
		cfg.VPN.HeadscaleAPIKey = "valid-hs-key"

		mockVPN := vpn.NewMockVPNProvider("tskey-auth-mock-123456", nil)
		mockVPN.Provider = "headscale"
		router := api.NewRouter(cfg, st, time.Now(), mockVPN, nil)

		body := api.RegisterNodeRequest{
			AuthKey: "gc_vpn_test_key_1234567890",
			Name:    "vpn-node-success",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-hs-admin-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got: %d", rec.Code)
		}

		var resp api.RegisterNodeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.VPNConfig.AuthKey != "tskey-auth-mock-123456" {
			t.Errorf("Expected vpn_config.auth_key 'tskey-auth-mock-123456', got: '%s'", resp.VPNConfig.AuthKey)
		}
		if resp.VPNConfig.ControlURL != "https://vpn.goy.test" {
			t.Errorf("Expected vpn_config.control_url 'https://vpn.goy.test', got: '%s'", resp.VPNConfig.ControlURL)
		}
		if resp.VPNConfig.Provider != "headscale" {
			t.Errorf("Expected vpn_config.provider 'headscale', got: '%s'", resp.VPNConfig.Provider)
		}
	})

	t.Run("Register Node with Tailscale VPN Enabled & Valid Key", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = true
		cfg.Auth.AdminAPIKey = "valid-ts-admin-key"
		cfg.VPN.Enabled = true
		cfg.VPN.Provider = "tailscale"
		cfg.VPN.TailscaleAPIKey = "valid-ts-key"
		cfg.VPN.TailscaleTailnet = "my-org.ts.net"

		mockVPN := vpn.NewMockVPNProvider("tskey-auth-tailscale-999", nil)
		mockVPN.Provider = "tailscale"
		router := api.NewRouter(cfg, st, time.Now(), mockVPN, nil)

		body := api.RegisterNodeRequest{
			AuthKey: "gc_ts_vpn_test_key_123456789",
			Name:    "ts-vpn-node-success",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-ts-admin-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got: %d", rec.Code)
		}

		var resp api.RegisterNodeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.VPNConfig.AuthKey != "tskey-auth-tailscale-999" {
			t.Errorf("Expected vpn_config.auth_key 'tskey-auth-tailscale-999', got: '%s'", resp.VPNConfig.AuthKey)
		}
		if resp.VPNConfig.ControlURL != "" {
			t.Errorf("Expected empty vpn_config.control_url for Tailscale, got: '%s'", resp.VPNConfig.ControlURL)
		}
		if resp.VPNConfig.Provider != "tailscale" {
			t.Errorf("Expected vpn_config.provider 'tailscale', got: '%s'", resp.VPNConfig.Provider)
		}
	})

	t.Run("Register Node with VPN Error (Graceful Fallback)", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = true
		cfg.Auth.AdminAPIKey = "valid-hs-admin-key"
		cfg.VPN.Enabled = true
		cfg.VPN.Provider = "headscale"
		cfg.VPN.HeadscaleAPIURL = "https://vpn.goy.test"
		cfg.VPN.HeadscaleAPIKey = "valid-hs-key"

		mockVPN := vpn.NewMockVPNProvider("", errors.New("headscale service down"))
		mockVPN.Provider = "headscale"
		router := api.NewRouter(cfg, st, time.Now(), mockVPN, nil)

		body := api.RegisterNodeRequest{
			AuthKey: "gc_vpn_fallback_key_1234567890",
			Name:    "vpn-node-fallback",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/v1/nodes/register", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-hs-admin-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Expected status 201 even with VPN failure, got: %d", rec.Code)
		}

		var resp api.RegisterNodeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.VPNConfig.AuthKey != "" {
			t.Errorf("Expected empty vpn_config.auth_key on fallback, got: '%s'", resp.VPNConfig.AuthKey)
		}
	})

	t.Run("GET /v1/vpn/status Endpoint", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = true
		cfg.Auth.AdminAPIKey = "valid-hs-admin-key"

		mockVPN := vpn.NewMockVPNProvider("test-key", nil)
		mockVPN.Provider = "tailscale"
		router := api.NewRouter(cfg, st, time.Now(), mockVPN, nil)

		req := httptest.NewRequest("GET", "/v1/vpn/status", nil)
		req.Header.Set("Authorization", "Bearer valid-hs-admin-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d", rec.Code)
		}

		var status vpn.VPNStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("Failed to decode VPN status response: %v", err)
		}

		if !status.VPNEnabled || status.Provider != "tailscale" || status.TailscaleReachable == nil || !*status.TailscaleReachable || status.RegisteredDevices != 3 {
			t.Errorf("Unexpected VPN status data: %+v", status)
		}
	})
}

func TestIsNodeOnline(t *testing.T) {
	now := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	threshold := 180

	t.Run("Nil LastSeen", func(t *testing.T) {
		if api.IsNodeOnline(nil, now, threshold) {
			t.Errorf("Expected false for nil lastSeen")
		}
	})

	t.Run("Zero LastSeen", func(t *testing.T) {
		zero := time.Time{}
		if api.IsNodeOnline(&zero, now, threshold) {
			t.Errorf("Expected false for zero lastSeen")
		}
	})

	t.Run("Recent LastSeen (Online)", func(t *testing.T) {
		recent := now.Add(-30 * time.Second)
		if !api.IsNodeOnline(&recent, now, threshold) {
			t.Errorf("Expected true for recent lastSeen")
		}
	})

	t.Run("Exact Threshold Boundary (Online at second N)", func(t *testing.T) {
		exact := now.Add(-time.Duration(threshold) * time.Second)
		if !api.IsNodeOnline(&exact, now, threshold) {
			t.Errorf("Expected true for exact threshold boundary")
		}
	})

	t.Run("Past Threshold Boundary (Offline at second N+1)", func(t *testing.T) {
		past := now.Add(-time.Duration(threshold+1) * time.Second)
		if api.IsNodeOnline(&past, now, threshold) {
			t.Errorf("Expected false for past threshold boundary")
		}
	})

	t.Run("Invalid Threshold Uses Default (180s)", func(t *testing.T) {
		recent := now.Add(-100 * time.Second)
		if !api.IsNodeOnline(&recent, now, 0) {
			t.Errorf("Expected true with 0 threshold falling back to default 180s")
		}

		old := now.Add(-200 * time.Second)
		if api.IsNodeOnline(&old, now, -5) {
			t.Errorf("Expected false with negative threshold falling back to default 180s")
		}
	})
}

func TestGetNodeStatusEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api_node_status.db")

	st := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := st.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer st.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = true
	cfg.Auth.AdminAPIKey = "admin-secret-key"
	cfg.Registry.OnlineThresholdSeconds = 180

	router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

	now := time.Now().UTC()

	// Seed relays in DB
	onlineRelay := &store.Relay{
		NodeID:             "goy-node-online-123",
		URL:                "ws://100.80.1.5:8443",
		Version:            "0.1.1",
		UptimeSecs:         3600,
		StorageReservedGB:  50,
		StorageAvailableGB: 200,
		Fingerprint:        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		LastSeenAt:         now.Add(-30 * time.Second),
		CreatedAt:          now.Add(-1 * time.Hour),
		UpdatedAt:          now.Add(-30 * time.Second),
	}
	if err := st.UpsertRelay(ctx, onlineRelay); err != nil {
		t.Fatalf("Failed to seed online relay: %v", err)
	}

	offlineRelay := &store.Relay{
		NodeID:             "goy-node-offline-456",
		URL:                "ws://100.80.1.6:8443",
		Version:            "0.1.0",
		UptimeSecs:         1200,
		StorageReservedGB:  50,
		StorageAvailableGB: 100,
		Fingerprint:        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		LastSeenAt:         now.Add(-600 * time.Second),
		CreatedAt:          now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-600 * time.Second),
	}
	if err := st.UpsertRelay(ctx, offlineRelay); err != nil {
		t.Fatalf("Failed to seed offline relay: %v", err)
	}

	neverRelay := &store.Relay{
		NodeID:             "goy-node-never-789",
		URL:                "ws://100.80.1.7:8443",
		Version:            "0.1.1",
		UptimeSecs:         0,
		StorageReservedGB:  50,
		StorageAvailableGB: 50,
		Fingerprint:        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		LastSeenAt:         time.Time{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := st.UpsertRelay(ctx, neverRelay); err != nil {
		t.Fatalf("Failed to seed never-seen relay: %v", err)
	}

	t.Run("Happy Path - Online Node", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-online-123/status", nil)
		req.Header.Set("Authorization", "Bearer admin-secret-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got: %d (body: %s)", rec.Code, rec.Body.String())
		}

		var resp api.NodeStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if resp.NodeID != "goy-node-online-123" {
			t.Errorf("Expected node_id 'goy-node-online-123', got: '%s'", resp.NodeID)
		}
		if !resp.IsOnline {
			t.Errorf("Expected is_online true")
		}
		if resp.LastSeen == nil {
			t.Errorf("Expected last_seen non-nil")
		}
		if resp.URL != "ws://100.80.1.5:8443" || resp.Version != "0.1.1" || resp.UptimeSecs != 3600 {
			t.Errorf("Unexpected relay metadata fields: %+v", resp)
		}
		if resp.Storage == nil || resp.Storage.ReservedGB != 50 || resp.Storage.AvailableGB != 200 {
			t.Errorf("Unexpected storage fields: %+v", resp.Storage)
		}
	})

	t.Run("Offline Node (> 180s ago)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-offline-456/status", nil)
		req.Header.Set("Authorization", "Bearer admin-secret-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got: %d", rec.Code)
		}

		var resp api.NodeStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if resp.NodeID != "goy-node-offline-456" {
			t.Errorf("Expected node_id 'goy-node-offline-456', got: '%s'", resp.NodeID)
		}
		if resp.IsOnline {
			t.Errorf("Expected is_online false")
		}
		if resp.LastSeen == nil {
			t.Errorf("Expected last_seen non-nil for offline node")
		}
	})

	t.Run("Never Seen Node (LastSeen Zero)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-never-789/status", nil)
		req.Header.Set("Authorization", "Bearer admin-secret-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got: %d", rec.Code)
		}

		var resp api.NodeStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if resp.IsOnline {
			t.Errorf("Expected is_online false for never seen node")
		}
		if resp.LastSeen != nil {
			t.Errorf("Expected last_seen null for never seen node, got: %v", resp.LastSeen)
		}
	})

	t.Run("Nonexistent Node -> 404 Not Found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-unknown/status", nil)
		req.Header.Set("Authorization", "Bearer admin-secret-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("Expected 404 Not Found, got: %d", rec.Code)
		}
	})

	t.Run("Unauthorized - Missing Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-online-123/status", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized for missing header, got: %d", rec.Code)
		}
	})

	t.Run("Unauthorized - Wrong Admin Key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-online-123/status", nil)
		req.Header.Set("Authorization", "Bearer wrong-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized for invalid key, got: %d", rec.Code)
		}
	})

	t.Run("Unauthorized - Rejects Node Auth Key (gc_...)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-online-123/status", nil)
		req.Header.Set("Authorization", "Bearer gc_test_node_key_1234567890")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized when presenting node auth key instead of admin key, got: %d", rec.Code)
		}
	})
}
