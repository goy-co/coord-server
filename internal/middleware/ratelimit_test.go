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
				t.Fatalf("Expected request %d to be allowed (200), got: %d", i+1, rec.Code)
			}
		}
	})

	t.Run("Exceeded Burst Denied with 429 & Retry-After", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/nodes/1", nil)
		req.RemoteAddr = "192.168.1.50:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("Expected status 429 for request exceeding burst, got: %d", rec.Code)
		}

		if rec.Header().Get("Retry-After") == "" {
			t.Errorf("Expected Retry-After header present in 429 response")
		}
	})

	t.Run("Exempt Public Path Allowed Always", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/health", nil)
			req.RemoteAddr = "192.168.1.50:12345"
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Public path /health should be exempt from rate limit, got: %d", rec.Code)
			}
		}
	})
}
