package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goy-co/coord-server/internal/config"
	custommiddleware "github.com/goy-co/coord-server/internal/middleware"
)

func TestAuthMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auth.RequireAuth = true
	cfg.Auth.AdminAPIKey = "secret-admin-key"
	cfg.Auth.PublicPaths = []string{"/health", "/info"}

	handler := custommiddleware.AuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	t.Run("Valid Bearer Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-1", nil)
		req.Header.Set("Authorization", "Bearer secret-admin-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for valid token, got: %d", rec.Code)
		}
	})

	t.Run("Missing Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-1", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected status 401 for missing header, got: %d", rec.Code)
		}
	})

	t.Run("Invalid Bearer Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-1", nil)
		req.Header.Set("Authorization", "Bearer wrong-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected status 401 for invalid token, got: %d", rec.Code)
		}
	})

	t.Run("Public Path Exemption (/health)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for public path /health without auth, got: %d", rec.Code)
		}
	})

	t.Run("RequireAuth Disabled Mode", func(t *testing.T) {
		disabledCfg := config.DefaultConfig()
		disabledCfg.Auth.RequireAuth = false

		disabledHandler := custommiddleware.AuthMiddleware(disabledCfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/v1/nodes/goy-node-1", nil)
		rec := httptest.NewRecorder()

		disabledHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Expected status 200 when RequireAuth=false, got: %d", rec.Code)
		}
	})
}
