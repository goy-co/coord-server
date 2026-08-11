package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
)

const ServerVersion = "0.1.0"

// HealthResponse defines the JSON response format for the GET /health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}

// InfoResponse defines the JSON response format for the GET /info endpoint.
type InfoResponse struct {
	Version               string `json:"version"`
	UptimeSeconds         int64  `json:"uptime_seconds"`
	DatabasePath          string `json:"database_path"`
	ListenAddress         string `json:"listen_address"`
	VPNIntegrationEnabled bool   `json:"vpn_integration_enabled"`
}

// HealthHandler processes GET /health requests.
func HealthHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := st.HealthCheck(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(HealthResponse{
				Status:  "degraded",
				Version: ServerVersion,
				Error:   "database unreachable",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status:  "ok",
			Version: ServerVersion,
		})
	}
}

// InfoHandler processes GET /info requests.
func InfoHandler(cfg *config.Config, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		uptime := int64(time.Since(startTime).Seconds())
		vpnEnabled := cfg.VPN.Enabled && cfg.VPN.Provider != ""

		resp := InfoResponse{
			Version:               ServerVersion,
			UptimeSeconds:         uptime,
			DatabasePath:          cfg.Database.Path,
			ListenAddress:         cfg.Server.Listen,
			VPNIntegrationEnabled: vpnEnabled,
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
