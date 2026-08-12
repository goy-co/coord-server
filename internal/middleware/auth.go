package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/metrics"
)

type errorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func writeUnauthorized(w http.ResponseWriter, details string) {
	if details == "" {
		details = "valid API key required"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error:   "unauthorized",
		Details: details,
	})
}

// AuthMiddleware intercepts HTTP requests and validates the Bearer token in the Authorization header.
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If authentication is disabled globally, allow access
			if !cfg.Auth.RequireAuth {
				next.ServeHTTP(w, r)
				return
			}

			// Check if the path is included in the list of public paths exempt from auth
			path := r.URL.Path
			for _, pubPath := range cfg.Auth.PublicPaths {
				if path == pubPath || strings.HasPrefix(path, pubPath+"/") {
					next.ServeHTTP(w, r)
					return
				}
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				slog.Warn("Authentication failed: missing Authorization header",
					slog.String("path", path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				metrics.AuthFailuresTotal.Inc()
				writeUnauthorized(w, "missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				slog.Warn("Authentication failed: invalid Authorization header format (expected 'Bearer <token>')",
					slog.String("path", path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				metrics.AuthFailuresTotal.Inc()
				writeUnauthorized(w, "Authorization header format must be 'Bearer <token>'")
				return
			}

			token := strings.TrimSpace(parts[1])
			adminKey := cfg.Auth.AdminAPIKey

			isAdmin := len(token) > 0 && subtle.ConstantTimeCompare([]byte(token), []byte(adminKey)) == 1
			isNodeKeyFormat := strings.HasPrefix(token, "gc_") && len(token) >= 20

			if !isAdmin && !isNodeKeyFormat {
				slog.Warn("Authentication failed: invalid API key",
					slog.String("path", path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				metrics.AuthFailuresTotal.Inc()
				writeUnauthorized(w, "invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
