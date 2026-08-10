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
		t.Fatalf("Falha ao inicializar SQLiteStore: %v", err)
	}
	defer st.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = false

	router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

	// 1. Criar nó para permitir registo de relay
	nodeID := "goy-node-relay-test-1"
	err := st.CreateNode(ctx, &store.Node{
		ID:          nodeID,
		AuthKeyHash: "hash-relay-test-1",
		Name:        "relay-parent-node",
	})
	if err != nil {
		t.Fatalf("Erro ao criar nó pai: %v", err)
	}

	validFingerprint := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// 2. POST /relays com node_id Inexistente -> 404 Not Found
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
			t.Fatalf("Esperava status 404, obtido: %d", rec.Code)
		}
	})

	// 3. POST /relays com URL Inválido -> 400 Bad Request
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
			t.Fatalf("Esperava status 400, obtido: %d", rec.Code)
		}
	})

	// 4. POST /relays com Sucesso -> 201 Created
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
			t.Fatalf("Esperava status 201, obtido: %d (corpo: %s)", rec.Code, rec.Body.String())
		}

		var dto api.RelayDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
			t.Fatalf("Falha ao deserializar JSON de resposta: %v", err)
		}

		if dto.NodeID != nodeID || dto.URL != "ws://100.80.1.5:8443" || dto.Storage.AvailableGB != 100 {
			t.Errorf("Dados de relay recebidos incorretos: %+v", dto)
		}
	})

	// 5. GET /relays -> 200 OK (com o relay registado)
	t.Run("Get Relays Active List", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/relays", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Esperava status 200, obtido: %d", rec.Code)
		}

		var resp api.GetRelaysResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Falha ao deserializar JSON de resposta: %v", err)
		}

		if resp.Total != 1 || len(resp.Relays) != 1 {
			t.Errorf("Esperado 1 relay ativo, obtido total=%d, len=%d", resp.Total, len(resp.Relays))
		}

		if resp.Relays[0].NodeID != nodeID {
			t.Errorf("NodeID inesperado no relay: %s", resp.Relays[0].NodeID)
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
			t.Fatalf("Esperava status 204, obtido: %d", rec.Code)
		}
	})

	// 7. DELETE /relays/{node_id} -> 204 No Content
	t.Run("Delete Relay Success", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/relays/"+nodeID, nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("Esperava status 204, obtido: %d", rec.Code)
		}

		getReq := httptest.NewRequest("GET", "/relays", nil)
		getRec := httptest.NewRecorder()

		router.ServeHTTP(getRec, getReq)

		if getRec.Code != http.StatusOK {
			t.Fatalf("Esperava status 200, obtido: %d", getRec.Code)
		}

		var resp api.GetRelaysResponse
		_ = json.Unmarshal(getRec.Body.Bytes(), &resp)

		if resp.Total != 0 || len(resp.Relays) != 0 {
			t.Errorf("Esperado 0 relays ativos após deleção, obtido total=%d, len=%d", resp.Total, len(resp.Relays))
		}
	})
}
