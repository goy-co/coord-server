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
