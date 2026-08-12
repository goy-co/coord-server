package api_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestRelayEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api_relays.db")

	st := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := st.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer st.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = false

	router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

	// 1. Create parent node to allow relay registration
	nodeID := "goy-node-relay-test-1"
	err := st.CreateNode(ctx, &store.Node{
		ID:          nodeID,
		AuthKeyHash: "hash-relay-test-1",
		Name:        "relay-parent-node",
	})
	if err != nil {
		t.Fatalf("Error creating parent node: %v", err)
	}

	validFingerprint := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// 2. POST /relays with Nonexistent node_id -> 404 Not Found
	t.Run("Register Relay Nonexistent Node", func(t *testing.T) {
		body := api.RegisterRelayRequest{
			NodeID:      "goy-node-nonexistent",
			URL:         "ws://100.80.1.5:8443",
			Fingerprint: validFingerprint,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/relays", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("Expected status 404, got: %d", rec.Code)
		}
	})

	// 3. POST /relays with Invalid URL -> 400 Bad Request
	t.Run("Register Relay Invalid URL", func(t *testing.T) {
		body := api.RegisterRelayRequest{
			NodeID:      nodeID,
			URL:         "http://100.80.1.5:8443",
			Fingerprint: validFingerprint,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/relays", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected status 400, got: %d", rec.Code)
		}
	})

	// 4. POST /relays Success -> 201 Created
	t.Run("Register Relay Success", func(t *testing.T) {
		body := api.RegisterRelayRequest{
			NodeID:      nodeID,
			URL:         "ws://100.80.1.5:8443",
			Fingerprint: validFingerprint,
			Storage: &api.StoragePayload{
				ReservedGB:  150,
				AvailableGB: 100,
			},
			ReplicationFactor: 3,
			Version:           "0.1.1-alpha",
			Capabilities:      []string{"nip09", "nip40"},
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/relays", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Expected status 201, got: %d (body: %s)", rec.Code, rec.Body.String())
		}

		var dto api.RelayDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
			t.Fatalf("Failed to deserialize JSON response: %v", err)
		}

		if dto.NodeID != nodeID || dto.URL != "ws://100.80.1.5:8443" || dto.Storage.AvailableGB != 100 {
			t.Errorf("Incorrect relay data received: %+v", dto)
		}
	})

	// 5. GET /relays -> 200 OK (with registered relay)
	t.Run("Get Relays Active List", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/relays", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d", rec.Code)
		}

		var resp api.GetRelaysResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to deserialize JSON response: %v", err)
		}

		if resp.Total != 1 || len(resp.Relays) != 1 {
			t.Errorf("Expected 1 active relay, got total=%d, len=%d", resp.Total, len(resp.Relays))
		}

		if resp.Relays[0].NodeID != nodeID {
			t.Errorf("Unexpected NodeID in relay: %s", resp.Relays[0].NodeID)
		}
	})

	// 6. PUT /relays/{node_id} Heartbeat -> 204 No Content
	t.Run("Put Relay Heartbeat Success", func(t *testing.T) {
		hbBody := map[string]any{
			"storage": map[string]any{
				"available_gb": 85,
			},
		}
		bodyBytes, _ := json.Marshal(hbBody)

		req := httptest.NewRequest("PUT", "/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("Expected status 204, got: %d", rec.Code)
		}
	})

	// 7. DELETE /relays/{node_id} -> 204 No Content
	t.Run("Delete Relay Success", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/relays/"+nodeID, nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("Expected status 204, got: %d", rec.Code)
		}

		getReq := httptest.NewRequest("GET", "/relays", nil)
		getRec := httptest.NewRecorder()

		router.ServeHTTP(getRec, getReq)

		if getRec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d", getRec.Code)
		}

		var resp api.GetRelaysResponse
		_ = json.Unmarshal(getRec.Body.Bytes(), &resp)

		if resp.Total != 0 || len(resp.Relays) != 0 {
			t.Errorf("Expected 0 active relays after deletion, got total=%d, len=%d", resp.Total, len(resp.Relays))
		}
	})
}

func TestPutV1RelayHeartbeat(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_api_v1_relays.db")

	st := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := st.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer st.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = true
	cfg.Auth.AdminAPIKey = "admin-secret-api-key"
	cfg.Auth.HMACSecret = "hmac-secret-12345"

	router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

	// Create parent node and register relay
	nodeAuthKey := "gc_valid_node_key_1234567890"
	nodeAuthHash := api.HashAuthKey(nodeAuthKey, cfg.Auth.HMACSecret)
	nodeID := "goy-node-v1-relay-1"

	err := st.CreateNode(ctx, &store.Node{
		ID:          nodeID,
		AuthKeyHash: nodeAuthHash,
		Name:        "v1-relay-parent-node",
	})
	if err != nil {
		t.Fatalf("Error creating parent node: %v", err)
	}

	validFingerprint := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Create initial relay entry
	err = st.UpsertRelay(ctx, &store.Relay{
		NodeID:             nodeID,
		URL:                "ws://100.80.1.5:8443",
		Fingerprint:        validFingerprint,
		StorageReservedGB:  50,
		StorageAvailableGB: 200,
		Version:            "0.1.0",
		Status:             store.RelayStatusActive,
		LastSeenAt:         time.Now().UTC().Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Error creating initial relay: %v", err)
	}

	validPayload := api.HeartbeatV1RelayRequest{
		URL:         "wss://100.80.1.5:8443",
		Fingerprint: validFingerprint,
		Storage: &api.StoragePayload{
			ReservedGB:  50,
			AvailableGB: 180,
		},
		Version:    "0.1.1",
		UptimeSecs: 3600,
	}

	t.Run("Happy Path with Admin Auth", func(t *testing.T) {
		bodyBytes, _ := json.Marshal(validPayload)
		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d (body: %s)", rec.Code, rec.Body.String())
		}

		var dto api.RelayDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
			t.Fatalf("Failed to decode response JSON: %v", err)
		}

		if dto.NodeID != nodeID || dto.URL != "wss://100.80.1.5:8443" || dto.Version != "0.1.1" || dto.Storage.AvailableGB != 180 || dto.UptimeSecs != 3600 {
			t.Errorf("Unexpected DTO values returned: %+v", dto)
		}
	})

	t.Run("Happy Path with Node Auth Key", func(t *testing.T) {
		bodyBytes, _ := json.Marshal(validPayload)
		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+nodeAuthKey)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Unauthorized - Missing Token", func(t *testing.T) {
		bodyBytes, _ := json.Marshal(validPayload)
		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected status 401, got: %d", rec.Code)
		}
	})

	t.Run("Unauthorized - Node Key Ownership Mismatch", func(t *testing.T) {
		// Register a second node with another key
		otherKey := "gc_other_node_key_9876543210"
		otherHash := api.HashAuthKey(otherKey, cfg.Auth.HMACSecret)
		_ = st.CreateNode(ctx, &store.Node{ID: "goy-node-other", AuthKeyHash: otherHash})

		bodyBytes, _ := json.Marshal(validPayload)
		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+otherKey)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected status 401 for ownership mismatch, got: %d", rec.Code)
		}
	})

	t.Run("Nonexistent Relay - 404 Not Found", func(t *testing.T) {
		bodyBytes, _ := json.Marshal(validPayload)
		req := httptest.NewRequest("PUT", "/v1/relays/goy-node-nonexistent", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("Expected status 404, got: %d", rec.Code)
		}
	})

	t.Run("Validation Failure - Invalid Fingerprint", func(t *testing.T) {
		payload := validPayload
		payload.Fingerprint = "invalid-fp"
		bodyBytes, _ := json.Marshal(payload)

		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected status 400 for invalid fingerprint, got: %d", rec.Code)
		}
	})

	t.Run("Validation Failure - Nil Storage", func(t *testing.T) {
		payload := validPayload
		payload.Storage = nil
		bodyBytes, _ := json.Marshal(payload)

		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected status 400 for nil storage, got: %d", rec.Code)
		}
	})

	t.Run("Validation Failure - Empty Version", func(t *testing.T) {
		payload := validPayload
		payload.Version = ""
		bodyBytes, _ := json.Marshal(payload)

		req := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected status 400 for empty version, got: %d", rec.Code)
		}
	})

	t.Run("Idempotency Test", func(t *testing.T) {
		bodyBytes, _ := json.Marshal(validPayload)

		req1 := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec1 := httptest.NewRecorder()

		router.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("First PUT failed: %d", rec1.Code)
		}

		time.Sleep(10 * time.Millisecond)

		req2 := httptest.NewRequest("PUT", "/v1/relays/"+nodeID, bytes.NewBuffer(bodyBytes))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer admin-secret-api-key")
		rec2 := httptest.NewRecorder()

		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("Second PUT failed: %d", rec2.Code)
		}
	})
}
