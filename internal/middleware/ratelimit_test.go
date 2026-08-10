package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goy-co/coord-server/internal/config"
	custommiddleware "github.com/goy-co/coord-server/internal/middleware"
)

func TestRateLimitMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RateLimit.RequestsPerMinute = 60
	cfg.RateLimit.Burst = 3
	cfg.Auth.PublicPaths = []string{"/health"}

	limiter := custommiddleware.NewIPRateLimiter()
	defer limiter.Close()

	handler := custommiddleware.RateLimitMiddleware(limiter, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Requests Within Burst Allowed", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest("GET", "/v1/nodes/1", nil)
			req.RemoteAddr = "192.168.1.50:12345"
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Esperava pedido %d ser permitido (200), obtido: %d", i+1, rec.Code)
			}
		}
	})

	t.Run("Exceeded Burst Denied with 429 & Retry-After", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/1", nil)
		req.RemoteAddr = "192.168.1.50:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("Esperava status 429 para pedido excedendo o burst, obtido: %d", rec.Code)
		}

		if rec.Header().Get("Retry-After") == "" {
			t.Errorf("Esperava header Retry-After presente na resposta 429")
		}
	})

	t.Run("Exempt Public Path Allowed Always", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/health", nil)
			req.RemoteAddr = "192.168.1.50:12345"
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Caminho público /health deve estar isento de rate limit, obtido: %d", rec.Code)
			}
		}
	})
}
