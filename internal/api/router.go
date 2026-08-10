package api

import (
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/goy-co/coord-server/internal/config"
	custommiddleware "github.com/goy-co/coord-server/internal/middleware"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

// NewRouter configura e retorna um novo chi.Router com os middlewares e rotas registadas.
func NewRouter(cfg *config.Config, st store.Store, startTime time.Time, vpnProvider vpn.VPNProvider, rateLimiter *custommiddleware.IPRateLimiter) chi.Router {
	r := chi.NewRouter()

	// 1. Middlewares Globais
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(custommiddleware.RequestLogger)
	r.Use(custommiddleware.MetricsMiddleware)

	if rateLimiter != nil {
		r.Use(custommiddleware.RateLimitMiddleware(rateLimiter, cfg))
	}

	// In-memory cache para /relays
	relayCache := NewRelayCache(cfg.Registry.DiscoveryCacheTTLSeconds)

	// 2. Rotas Públicas (Sem Autenticação)
	r.Get("/health", HealthHandler(st))
	r.Get("/info", InfoHandler(cfg, startTime))
	r.Handle("/metrics", promhttp.Handler())

	// 3. Rotas Protegidas (Exigem Autenticação por API Key se require_auth = true)
	r.Group(func(r chi.Router) {
		r.Use(custommiddleware.AuthMiddleware(cfg))

		// Rotas Nodes (/v1/nodes/*)
		r.Route("/v1/nodes", func(r chi.Router) {
			r.Post("/register", RegisterNodeHandler(st, cfg, vpnProvider))
			r.Get("/{id}", GetNodeHandler(st))
			r.Delete("/{id}", DeleteNodeHandler(st))
		})

		// Rota VPN Status (/v1/vpn/status)
		r.Get("/v1/vpn/status", GetVPNStatusHandler(vpnProvider))

		// Rotas Relays (/relays/*)
		r.Route("/relays", func(r chi.Router) {
			r.Get("/", GetRelaysHandler(st, cfg, relayCache))
			r.Post("/", RegisterRelayHandler(st, cfg, relayCache))
			r.Put("/{node_id}", HeartbeatRelayHandler(st, relayCache))
			r.Delete("/{node_id}", DeleteRelayHandler(st, relayCache))
		})
	})

	return r
}
