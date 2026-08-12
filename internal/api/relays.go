package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
)

// RelayCache implements a simple in-memory cache with RWMutex for the GET /relays endpoint.
type RelayCache struct {
	mu        sync.RWMutex
	cachedAt  time.Time
	ttl       time.Duration
	relays    []RelayDTO
	total     int
	hasCached bool
}

// NewRelayCache instantiates a new RelayCache with the configured TTL duration.
func NewRelayCache(ttlSeconds int) *RelayCache {
	if ttlSeconds <= 0 {
		ttlSeconds = 15
	}
	return &RelayCache{
		ttl: time.Duration(ttlSeconds) * time.Second,
	}
}

func (c *RelayCache) Get() ([]RelayDTO, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.hasCached {
		return nil, 0, false
	}
	if time.Since(c.cachedAt) > c.ttl {
		return nil, 0, false
	}

	return c.relays, c.total, true
}

func (c *RelayCache) Set(relays []RelayDTO, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.relays = relays
	c.total = total
	c.cachedAt = time.Now()
	c.hasCached = true
}

func (c *RelayCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hasCached = false
}

// RelayDTO represents the JSON structure exposed for a relay in API requests.
type RelayDTO struct {
	NodeID            string          `json:"node_id"`
	URL               string          `json:"url"`
	Fingerprint       string          `json:"fingerprint"`
	Storage           *StoragePayload `json:"storage,omitempty"`
	ReplicationFactor uint32          `json:"replication_factor"`
	Version           string          `json:"version"`
	UptimeSecs        uint64          `json:"uptime_secs,omitempty"`
	Capabilities      []string        `json:"capabilities"`
	LastSeen          time.Time       `json:"last_seen"`
}

func toRelayDTO(r *store.Relay) RelayDTO {
	return RelayDTO{
		NodeID:      r.NodeID,
		URL:         r.URL,
		Fingerprint: r.Fingerprint,
		Storage: &StoragePayload{
			ReservedGB:  r.StorageReservedGB,
			AvailableGB: r.StorageAvailableGB,
		},
		ReplicationFactor: r.ReplicationFactor,
		Version:           r.Version,
		UptimeSecs:        r.UptimeSecs,
		Capabilities:      r.Capabilities,
		LastSeen:          r.LastSeenAt,
	}
}

// GetRelaysResponse represents the response for GET /relays.
type GetRelaysResponse struct {
	Relays      []RelayDTO `json:"relays"`
	Total       int        `json:"total"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// RegisterRelayRequest represents the payload for POST /relays.
type RegisterRelayRequest struct {
	NodeID            string          `json:"node_id"`
	URL               string          `json:"url"`
	Fingerprint       string          `json:"fingerprint"`
	Storage           *StoragePayload `json:"storage,omitempty"`
	ReplicationFactor uint32          `json:"replication_factor,omitempty"`
	Version           string          `json:"version,omitempty"`
	Capabilities      []string        `json:"capabilities,omitempty"`
}

// HeartbeatRelayRequest represents the partial payload for PUT /relays/{node_id}.
type HeartbeatRelayRequest struct {
	Storage *struct {
		AvailableGB uint64 `json:"available_gb"`
	} `json:"storage,omitempty"`
}

// HeartbeatV1RelayRequest represents the full payload for PUT /v1/relays/{node_id}.
type HeartbeatV1RelayRequest struct {
	URL         string          `json:"url"`
	Fingerprint string          `json:"fingerprint"`
	Storage     *StoragePayload `json:"storage"`
	Version     string          `json:"version"`
	UptimeSecs  uint64          `json:"uptime_secs,omitempty"`
}

// GetRelaysHandler handles the GET /relays request for peer discovery.
func GetRelaysHandler(st store.Store, cfg *config.Config, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sinceStr := r.URL.Query().Get("since")
		minStorageStr := r.URL.Query().Get("min_storage_gb")

		var sincePtr *time.Time
		if sinceStr != "" {
			t, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				WriteBadRequest(w, "'since' parameter must be a valid RFC3339 timestamp")
				return
			}
			sincePtr = &t
		}

		var minStorageGB uint64
		if minStorageStr != "" {
			val, err := strconv.ParseUint(minStorageStr, 10, 64)
			if err != nil {
				WriteBadRequest(w, "'min_storage_gb' parameter must be a positive integer")
				return
			}
			minStorageGB = val
		}

		// If no query parameter filters are applied, try serving from in-memory cache
		if sincePtr == nil && minStorageGB == 0 && cache != nil {
			if cachedRelays, total, ok := cache.Get(); ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(GetRelaysResponse{
					Relays:      cachedRelays,
					Total:       total,
					GeneratedAt: time.Now().UTC(),
				})
				return
			}
		}

		relays, total, err := st.ListActiveRelays(
			r.Context(),
			cfg.Registry.RelayTTLSeconds,
			sincePtr,
			minStorageGB,
			cfg.Registry.MaxRelaysPerResponse,
		)
		if err != nil {
			slog.Error("Error looking up active relays", slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		dtos := make([]RelayDTO, len(relays))
		for i, relay := range relays {
			dtos[i] = toRelayDTO(&relay)
		}

		// Update cache if no filters were applied
		if sincePtr == nil && minStorageGB == 0 && cache != nil {
			cache.Set(dtos, total)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(GetRelaysResponse{
			Relays:      dtos,
			Total:       total,
			GeneratedAt: time.Now().UTC(),
		})
	}
}

// RegisterRelayHandler handles the POST /relays request.
func RegisterRelayHandler(st store.Store, cfg *config.Config, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRelayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteBadRequest(w, "invalid request JSON body")
			return
		}

		if req.NodeID == "" {
			WriteBadRequest(w, "node_id field is required")
			return
		}
		if err := ValidateRelayURL(req.URL); err != nil {
			WriteBadRequest(w, fmt.Sprintf("url validation failed: %v", err))
			return
		}
		if req.Fingerprint != "" {
			if err := ValidateFingerprint(req.Fingerprint); err != nil {
				WriteBadRequest(w, fmt.Sprintf("fingerprint validation failed: %v", err))
				return
			}
		}
		if err := ValidateCapabilities(req.Capabilities); err != nil {
			WriteBadRequest(w, fmt.Sprintf("capabilities validation failed: %v", err))
			return
		}

		// 1. Verify that node_id exists and is active in the nodes table
		node, err := st.GetNodeByID(r.Context(), req.NodeID)
		if err != nil {
			if errors.Is(err, store.ErrNodeNotFound) {
				WriteNotFound(w, "node", req.NodeID)
				return
			}
			slog.Error("Error validating node existence for relay registration", slog.String("node_id", req.NodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		relay := &store.Relay{
			NodeID:            node.ID,
			URL:               req.URL,
			Fingerprint:       req.Fingerprint,
			ReplicationFactor: req.ReplicationFactor,
			Version:           req.Version,
			Capabilities:      req.Capabilities,
			Status:            store.RelayStatusActive,
			LastSeenAt:        time.Now().UTC(),
		}

		if req.Storage != nil {
			relay.StorageReservedGB = req.Storage.ReservedGB
			relay.StorageAvailableGB = req.Storage.AvailableGB
		} else {
			relay.StorageReservedGB = node.StorageReservedGB
			relay.StorageAvailableGB = node.StorageAvailableGB
		}

		if err := st.UpsertRelay(r.Context(), relay); err != nil {
			slog.Error("Error in UpsertRelay", slog.String("node_id", req.NodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		slog.Info("Relay registered/updated successfully", slog.String("node_id", req.NodeID), slog.String("url", req.URL))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(toRelayDTO(relay))
	}
}

func authorizeNodeOwnership(r *http.Request, nodeID string, cfg *config.Config, st store.Store) bool {
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
	if token == "" {
		return false
	}

	// 1. Admin API key override
	if len(token) > 0 && subtle.ConstantTimeCompare([]byte(token), []byte(cfg.Auth.AdminAPIKey)) == 1 {
		return true
	}

	// 2. Node auth key ownership check
	hash := HashAuthKey(token, cfg.Auth.HMACSecret)
	node, err := st.GetNodeByAuthKeyHash(r.Context(), hash)
	if err != nil || node == nil {
		return false
	}

	return node.ID == nodeID
}

// PutV1RelayHeartbeatHandler handles the PUT /v1/relays/{node_id} heartbeat request.
func PutV1RelayHeartbeatHandler(st store.Store, cfg *config.Config, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "node_id")
		if nodeID == "" {
			WriteBadRequest(w, "missing node_id URL parameter")
			return
		}

		// 1. Authenticate & Verify node ownership
		if !authorizeNodeOwnership(r, nodeID, cfg, st) {
			WriteUnauthorized(w, "unauthorized access to node_id")
			return
		}

		// 2. Parse & Validate JSON request body BEFORE touching DB for relay update
		var req HeartbeatV1RelayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteBadRequest(w, "invalid request JSON body")
			return
		}

		if err := ValidateRelayURL(req.URL); err != nil {
			WriteBadRequest(w, fmt.Sprintf("url validation failed: %v", err))
			return
		}

		if err := ValidateFingerprint(req.Fingerprint); err != nil {
			WriteBadRequest(w, fmt.Sprintf("fingerprint validation failed: %v", err))
			return
		}

		if req.Storage == nil {
			WriteBadRequest(w, "storage payload is required")
			return
		}

		if strings.TrimSpace(req.Version) == "" {
			WriteBadRequest(w, "version field is required and cannot be empty")
			return
		}

		// 3. Look up relay in database
		relay, err := st.GetRelayByNodeID(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, store.ErrRelayNotFound) {
				WriteNotFound(w, "relay", nodeID)
				return
			}
			slog.Error("Error looking up relay for heartbeat", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		// 4. Update relay fields atomically
		relay.URL = req.URL
		relay.Fingerprint = req.Fingerprint
		relay.StorageReservedGB = req.Storage.ReservedGB
		relay.StorageAvailableGB = req.Storage.AvailableGB
		relay.Version = req.Version
		relay.UptimeSecs = req.UptimeSecs
		relay.Status = store.RelayStatusActive
		relay.LastSeenAt = time.Now().UTC()

		if err := st.UpdateRelayFull(r.Context(), relay); err != nil {
			slog.Error("Error updating relay full heartbeat", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		slog.Info("V1 Relay heartbeat updated successfully", slog.String("node_id", nodeID), slog.String("url", req.URL))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(toRelayDTO(relay))
	}
}

// HeartbeatRelayHandler handles the PUT /relays/{node_id} request.
func HeartbeatRelayHandler(st store.Store, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "node_id")
		if nodeID == "" {
			WriteBadRequest(w, "missing node_id URL parameter")
			return
		}

		var req HeartbeatRelayRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		var storageAvailableGB *uint64
		if req.Storage != nil {
			storageAvailableGB = &req.Storage.AvailableGB
		}

		if err := st.UpdateRelayHeartbeat(r.Context(), nodeID, storageAvailableGB); err != nil {
			if errors.Is(err, store.ErrRelayNotFound) {
				WriteNotFound(w, "relay", nodeID)
				return
			}
			slog.Error("Error updating relay heartbeat", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteRelayHandler handles the DELETE /relays/{node_id} request.
func DeleteRelayHandler(st store.Store, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "node_id")
		if nodeID == "" {
			WriteBadRequest(w, "missing node_id URL parameter")
			return
		}

		if err := st.DeleteRelay(r.Context(), nodeID); err != nil {
			if errors.Is(err, store.ErrRelayNotFound) {
				WriteNotFound(w, "relay", nodeID)
				return
			}
			slog.Error("Error deleting relay", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		slog.Info("Relay removed from registry successfully", slog.String("node_id", nodeID))
		w.WriteHeader(http.StatusNoContent)
	}
}
