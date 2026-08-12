package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

// StoragePayload represents the storage statistics sent during onboarding.
type StoragePayload struct {
	ReservedGB  uint64 `json:"reserved_gb"`
	AvailableGB uint64 `json:"available_gb"`
}

// RegisterNodeRequest represents the request body for POST /v1/nodes/register.
type RegisterNodeRequest struct {
	AuthKey string          `json:"auth_key"`
	Name    string          `json:"name,omitempty"`
	Storage *StoragePayload `json:"storage,omitempty"`
	MeshURL string          `json:"mesh_url,omitempty"`
}

// VPNConfigResponse represents the VPN configuration returned to the node.
type VPNConfigResponse struct {
	AuthKey    string `json:"auth_key"`
	ControlURL string `json:"control_url"`
	Provider   string `json:"provider"`
}

// RegisterNodeResponse represents the response for POST /v1/nodes/register.
type RegisterNodeResponse struct {
	NodeID      string            `json:"node_id"`
	Name        string            `json:"name"`
	MeshURL     string            `json:"mesh_url"`
	VPNConfig   VPNConfigResponse `json:"vpn_config"`
	RegistryURL string            `json:"registry_url"`
	CreatedAt   time.Time         `json:"created_at"`
}

// RegisterNodeHandler handles the POST /v1/nodes/register request.
func RegisterNodeHandler(st store.Store, cfg *config.Config, vpnProvider vpn.VPNProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteBadRequest(w, "invalid request JSON body")
			return
		}

		if !ValidateAuthKeyFormat(req.AuthKey) {
			WriteBadRequest(w, "invalid auth_key format (must start with 'gc_' and be at least 20 characters long)")
			return
		}

		authHash := HashAuthKey(req.AuthKey, cfg.Auth.HMACSecret)

		// 1. Check idempotency: node already registered with this auth key
		existingNode, err := st.GetNodeByAuthKeyHash(r.Context(), authHash)
		if err == nil && existingNode != nil {
			slog.Info("Idempotent node registration (already existing)", slog.String("node_id", existingNode.ID))
			sendRegisterResponse(w, r, existingNode, http.StatusOK, cfg, vpnProvider)
			return
		} else if err != nil && !errors.Is(err, store.ErrNodeNotFound) {
			slog.Error("Error looking up node by auth key hash", slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		// 2. Create new node
		nodeID := GenerateNodeID()
		newNode := &store.Node{
			ID:          nodeID,
			AuthKeyHash: authHash,
			Name:        req.Name,
			MeshURL:     req.MeshURL,
			Status:      store.NodeStatusActive,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		if req.Storage != nil {
			newNode.StorageReservedGB = req.Storage.ReservedGB
			newNode.StorageAvailableGB = req.Storage.AvailableGB
		}

		if err := st.CreateNode(r.Context(), newNode); err != nil {
			slog.Error("Error saving new node to database", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		slog.Info("New node registered successfully", slog.String("node_id", nodeID), slog.String("name", req.Name))
		sendRegisterResponse(w, r, newNode, http.StatusCreated, cfg, vpnProvider)
	}
}

func sendRegisterResponse(w http.ResponseWriter, r *http.Request, n *store.Node, statusCode int, cfg *config.Config, vpnProvider vpn.VPNProvider) {
	registryURL := fmt.Sprintf("http://%s", r.Host)
	if r.Host == "" {
		registryURL = "http://coord-server:8080"
	}

	vpnAuthKey := ""
	vpnControlURL := ""
	providerName := ""

	if cfg.VPN.Enabled && cfg.VPN.Provider != "" && vpnProvider != nil {
		var expiryHours int
		var reusable bool
		var tags []string

		if cfg.VPN.Provider == "tailscale" {
			expiryHours = cfg.VPN.TailscaleKeyExpiryHours
			reusable = cfg.VPN.TailscaleKeyReusable
			if cfg.VPN.TailscaleTag != "" {
				tags = []string{cfg.VPN.TailscaleTag}
			}
		} else if cfg.VPN.Provider == "headscale" {
			expiryHours = cfg.VPN.HeadscaleKeyExpiryHours
			reusable = cfg.VPN.HeadscaleKeyReusable
		}

		opts := vpn.CreateKeyOpts{
			Reusable:    reusable,
			ExpiryHours: expiryHours,
			Tags:        tags,
		}

		vpnCfg, err := vpnProvider.CreatePreAuthKey(r.Context(), opts)
		if err != nil {
			slog.Warn("vpn: failed to generate pre-auth key, proceeding with empty vpn_config", slog.String("provider", vpnProvider.ProviderName()), slog.String("error", err.Error()))
		} else if vpnCfg != nil {
			vpnAuthKey = vpnCfg.AuthKey
			vpnControlURL = vpnCfg.ControlURL
			providerName = vpnCfg.Provider
			slog.Info("VPN pre-auth key generated successfully for node", slog.String("provider", providerName), slog.String("node_id", n.ID))
		}
	} else {
		slog.Debug("VPN integration disabled, returning empty vpn_config")
	}

	resp := RegisterNodeResponse{
		NodeID:  n.ID,
		Name:    n.Name,
		MeshURL: n.MeshURL,
		VPNConfig: VPNConfigResponse{
			AuthKey:    vpnAuthKey,
			ControlURL: vpnControlURL,
			Provider:   providerName,
		},
		RegistryURL: registryURL,
		CreatedAt:   n.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetNodeHandler handles the GET /v1/nodes/{id} request.
func GetNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "id")
		if nodeID == "" {
			WriteBadRequest(w, "missing id URL parameter")
			return
		}

		node, err := st.GetNodeByID(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, store.ErrNodeNotFound) {
				WriteNotFound(w, "node", nodeID)
				return
			}
			slog.Error("Error looking up node by ID", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(node)
	}
}

// DeleteNodeHandler handles the DELETE /v1/nodes/{id} request (soft delete).
func DeleteNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "id")
		if nodeID == "" {
			WriteBadRequest(w, "missing id URL parameter")
			return
		}

		if err := st.DeleteNode(r.Context(), nodeID); err != nil {
			if errors.Is(err, store.ErrNodeNotFound) {
				WriteNotFound(w, "node", nodeID)
				return
			}
			slog.Error("Error deleting node by ID", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		slog.Info("Node marked as deleted (soft delete)", slog.String("node_id", nodeID))
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetVPNStatusHandler handles the GET /v1/vpn/status request for VPN infrastructure diagnostics.
func GetVPNStatusHandler(vpnProvider vpn.VPNProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if vpnProvider == nil {
			WriteJSONError(w, http.StatusOK, ErrorResponse{Error: "vpn provider not initialized"})
			return
		}

		status, err := vpnProvider.GetStatus(r.Context())
		if err != nil {
			slog.Error("Error obtaining VPN diagnostic status", slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	}
}

// NodeStatusResponse represents the read-only status payload for GET /v1/nodes/{node_id}/status.
type NodeStatusResponse struct {
	NodeID     string          `json:"node_id"`
	IsOnline   bool            `json:"is_online"`
	LastSeen   *time.Time      `json:"last_seen"`
	URL        string          `json:"url"`
	Version    string          `json:"version"`
	UptimeSecs uint64          `json:"uptime_secs"`
	Storage    *StoragePayload `json:"storage"`
}

// IsNodeOnline calculates whether a node is online given its last_seen timestamp, current time, and threshold in seconds.
func IsNodeOnline(lastSeen *time.Time, now time.Time, thresholdSecs int) bool {
	if lastSeen == nil || lastSeen.IsZero() {
		return false
	}
	if thresholdSecs <= 0 {
		thresholdSecs = config.DefaultOnlineThresholdSeconds
	}
	cutoff := now.Add(-time.Duration(thresholdSecs) * time.Second)
	return !lastSeen.Before(cutoff)
}

func authorizeAdminOnly(r *http.Request, cfg *config.Config) bool {
	if !cfg.Auth.RequireAuth {
		return true
	}
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" || cfg.Auth.AdminAPIKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(cfg.Auth.AdminAPIKey)) == 1
}

// GetNodeStatusHandler handles the GET /v1/nodes/{node_id}/status request.
func GetNodeStatusHandler(st store.Store, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Admin Auth Check (strictly require Admin API Key, reject node auth keys)
		if !authorizeAdminOnly(r, cfg) {
			WriteUnauthorized(w, "admin API key required")
			return
		}

		nodeID := chi.URLParam(r, "node_id")
		if nodeID == "" {
			nodeID = chi.URLParam(r, "id")
		}
		if nodeID == "" {
			WriteBadRequest(w, "missing node_id URL parameter")
			return
		}

		relay, err := st.GetRelayByNodeID(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, store.ErrRelayNotFound) {
				WriteNotFound(w, "node", nodeID)
				return
			}
			slog.Error("Error looking up node relay for status", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		var lastSeenPtr *time.Time
		if !relay.LastSeenAt.IsZero() && relay.LastSeenAt.Year() > 1 {
			t := relay.LastSeenAt.UTC()
			lastSeenPtr = &t
		}

		isOnline := IsNodeOnline(lastSeenPtr, time.Now().UTC(), cfg.Registry.OnlineThresholdSeconds)

		resp := NodeStatusResponse{
			NodeID:     relay.NodeID,
			IsOnline:   isOnline,
			LastSeen:   lastSeenPtr,
			URL:        relay.URL,
			Version:    relay.Version,
			UptimeSecs: relay.UptimeSecs,
			Storage: &StoragePayload{
				ReservedGB:  relay.StorageReservedGB,
				AvailableGB: relay.StorageAvailableGB,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
