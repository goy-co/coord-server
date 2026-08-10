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

	t.Run("Register Node with VPN Enabled & Valid Key", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = true
		cfg.Auth.AdminAPIKey = "valid-hs-admin-key"
		cfg.VPN.Enabled = true
		cfg.VPN.HeadscaleAPIURL = "https://vpn.goy.test"
		cfg.VPN.HeadscaleAPIKey = "valid-hs-key"

		mockVPN := vpn.NewMockVPNProvider("tskey-auth-mock-123456", nil)
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
	})

	t.Run("Register Node with VPN Error (Graceful Fallback)", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = true
		cfg.Auth.AdminAPIKey = "valid-hs-admin-key"
		cfg.VPN.Enabled = true
		cfg.VPN.HeadscaleAPIURL = "https://vpn.goy.test"
		cfg.VPN.HeadscaleAPIKey = "valid-hs-key"

		mockVPN := vpn.NewMockVPNProvider("", errors.New("headscale service down"))
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

		if !status.VPNEnabled || !status.HeadscaleReachable || status.RegisteredMachines != 3 {
			t.Errorf("Unexpected VPN status data: %+v", status)
		}
	})
}
