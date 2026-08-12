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

// NewRouter configures and returns a new chi.Router with registered middlewares and routes.
func NewRouter(cfg *config.Config, st store.Store, startTime time.Time, vpnProvider vpn.VPNProvider, rateLimiter *custommiddleware.IPRateLimiter) chi.Router {
	r := chi.NewRouter()

	// 1. Global Middlewares
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(custommiddleware.RequestLogger)
	r.Use(custommiddleware.MetricsMiddleware)

	if rateLimiter != nil {
		r.Use(custommiddleware.RateLimitMiddleware(rateLimiter, cfg))
	}

	// In-memory cache for /relays
	relayCache := NewRelayCache(cfg.Registry.DiscoveryCacheTTLSeconds)

	// 2. Public Routes (No Authentication)
	r.Get("/health", HealthHandler(st))
	r.Get("/info", InfoHandler(cfg, startTime))
	r.Handle("/metrics", promhttp.Handler())

	// 3. Protected Routes (Require API Key authentication if require_auth = true)
	r.Group(func(r chi.Router) {
		r.Use(custommiddleware.AuthMiddleware(cfg))

		// Nodes Routes (/v1/nodes/*)
		r.Route("/v1/nodes", func(r chi.Router) {
			r.Post("/register", RegisterNodeHandler(st, cfg, vpnProvider))
			r.Get("/{id}", GetNodeHandler(st))
			r.Delete("/{id}", DeleteNodeHandler(st))
		})

		// VPN Status Route (/v1/vpn/status)
		r.Get("/v1/vpn/status", GetVPNStatusHandler(vpnProvider))

		// Relays Routes (/v1/relays/* and /relays/*)
		r.Route("/v1/relays", func(r chi.Router) {
			r.Put("/{node_id}", PutV1RelayHeartbeatHandler(st, cfg, relayCache))
		})

		r.Route("/relays", func(r chi.Router) {
			r.Get("/", GetRelaysHandler(st, cfg, relayCache))
			r.Post("/", RegisterRelayHandler(st, cfg, relayCache))
			r.Put("/{node_id}", HeartbeatRelayHandler(st, relayCache))
			r.Delete("/{node_id}", DeleteRelayHandler(st, relayCache))
		})
	})

	return r
}
