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

// AuthMiddleware intercepta os pedidos HTTP e valida o token Bearer enviado no header Authorization.
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Se a autenticação estiver desativada globalmente, permitir acesso
			if !cfg.Auth.RequireAuth {
				next.ServeHTTP(w, r)
				return
			}

			// Verificar se o caminho faz parte da lista de caminhos públicos isentos de auth
			path := r.URL.Path
			for _, pubPath := range cfg.Auth.PublicPaths {
				if path == pubPath || strings.HasPrefix(path, pubPath+"/") {
					next.ServeHTTP(w, r)
					return
				}
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				slog.Warn("Autenticação falhou: header Authorization em falta",
					slog.String("path", path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				metrics.AuthFailuresTotal.Inc()
				writeUnauthorized(w, "header Authorization em falta")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				slog.Warn("Autenticação falhou: formato de header Authorization inválido (esperado 'Bearer <token>')",
					slog.String("path", path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				metrics.AuthFailuresTotal.Inc()
				writeUnauthorized(w, "formato do header Authorization deve ser 'Bearer <token>'")
				return
			}

			token := strings.TrimSpace(parts[1])
			adminKey := cfg.Auth.AdminAPIKey

			// Comparação em tempo constante para prevenir timing attacks
			if len(token) == 0 || subtle.ConstantTimeCompare([]byte(token), []byte(adminKey)) != 1 {
				slog.Warn("Autenticação falhou: API key inválida",
					slog.String("path", path),
					slog.String("remote_addr", r.RemoteAddr),
				)
				metrics.AuthFailuresTotal.Inc()
				writeUnauthorized(w, "API key inválida")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
