package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goy-co/coord-server/internal/api"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

type mockStore struct {
	store.Store
	healthErr error
}

func (m *mockStore) HealthCheck(ctx context.Context) error {
	return m.healthErr
}

func TestHealthEndpoint(t *testing.T) {
	t.Run("Health OK", func(t *testing.T) {
		st := &mockStore{healthErr: nil}
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = false

		router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got: %d", rec.Code)
		}

		var resp api.HealthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to deserialize JSON response: %v", err)
		}

		if resp.Status != "ok" || resp.Version != api.ServerVersion {
			t.Errorf("Unexpected response: %+v", resp)
		}
	})

	t.Run("Health Degraded", func(t *testing.T) {
		st := &mockStore{healthErr: errors.New("db error")}
		cfg := config.DefaultConfig()
		cfg.Auth.RequireAuth = false

		router := api.NewRouter(cfg, st, time.Now(), vpn.NewNoopVPNProvider(), nil)

		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("Expected status 503, got: %d", rec.Code)
		}

		var resp api.HealthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to deserialize JSON response: %v", err)
		}

		if resp.Status != "degraded" || resp.Error != "database unreachable" {
			t.Errorf("Unexpected response: %+v", resp)
		}
	})
}

func TestInfoEndpoint(t *testing.T) {
	st := &mockStore{healthErr: nil}
	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = false

	startTime := time.Now().Add(-10 * time.Second)
	router := api.NewRouter(cfg, st, startTime, vpn.NewNoopVPNProvider(), nil)

	req := httptest.NewRequest("GET", "/info", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got: %d", rec.Code)
	}

	var resp api.InfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to deserialize JSON response: %v", err)
	}

	if resp.Version != api.ServerVersion {
		t.Errorf("Expected version %s, got: %s", api.ServerVersion, resp.Version)
	}

	if resp.UptimeSeconds < 10 {
		t.Errorf("Expected UptimeSeconds >= 10, got: %d", resp.UptimeSeconds)
	}

	if resp.ListenAddress != cfg.Server.Listen {
		t.Errorf("Expected ListenAddress %s, got: %s", cfg.Server.Listen, resp.ListenAddress)
	}

	if resp.DatabasePath != cfg.Database.Path {
		t.Errorf("Expected DatabasePath %s, got: %s", cfg.Database.Path, resp.DatabasePath)
	}
}
