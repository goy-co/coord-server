package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
)

// RelayCache implementa um cache em memória simples com RWMutex para o endpoint GET /relays.
type RelayCache struct {
	mu        sync.RWMutex
	cachedAt  time.Time
	ttl       time.Duration
	relays    []RelayDTO
	total     int
	hasCached bool
}

// NewRelayCache instancia um novo RelayCache com a duração de TTL configurada.
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

// RelayDTO representa a estrutura JSON exposta para um relay nos pedidos de API.
type RelayDTO struct {
	NodeID            string          `json:"node_id"`
	URL               string          `json:"url"`
	Fingerprint       string          `json:"fingerprint"`
	Storage           *StoragePayload `json:"storage,omitempty"`
	ReplicationFactor uint32          `json:"replication_factor"`
	Version           string          `json:"version"`
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
		Capabilities:      r.Capabilities,
		LastSeen:          r.LastSeenAt,
	}
}

// GetRelaysResponse representa a resposta do pedido GET /relays.
type GetRelaysResponse struct {
	Relays      []RelayDTO `json:"relays"`
	Total       int        `json:"total"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// RegisterRelayRequest representa o payload do pedido POST /relays.
type RegisterRelayRequest struct {
	NodeID            string          `json:"node_id"`
	URL               string          `json:"url"`
	Fingerprint       string          `json:"fingerprint"`
	Storage           *StoragePayload `json:"storage,omitempty"`
	ReplicationFactor uint32          `json:"replication_factor,omitempty"`
	Version           string          `json:"version,omitempty"`
	Capabilities      []string        `json:"capabilities,omitempty"`
}

// HeartbeatRelayRequest representa o payload parcial do pedido PUT /relays/{node_id}.
type HeartbeatRelayRequest struct {
	Storage *struct {
		AvailableGB uint64 `json:"available_gb"`
	} `json:"storage,omitempty"`
}

// GetRelaysHandler lida com o pedido GET /relays para peer discovery.
func GetRelaysHandler(st store.Store, cfg *config.Config, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sinceStr := r.URL.Query().Get("since")
		minStorageStr := r.URL.Query().Get("min_storage_gb")

		var sincePtr *time.Time
		if sinceStr != "" {
			t, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				WriteBadRequest(w, "parâmetro 'since' com formato RFC3339 inválido")
				return
			}
			sincePtr = &t
		}

		var minStorageGB uint64
		if minStorageStr != "" {
			val, err := strconv.ParseUint(minStorageStr, 10, 64)
			if err != nil {
				WriteBadRequest(w, "parâmetro 'min_storage_gb' deve ser um inteiro positivo")
				return
			}
			minStorageGB = val
		}

		// Se não houver filtros por query param, tentar servir do cache em memória
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
			slog.Error("Erro ao procurar relays ativos", slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		dtos := make([]RelayDTO, len(relays))
		for i, relay := range relays {
			dtos[i] = toRelayDTO(&relay)
		}

		// Atualizar cache se não havia filtros aplicados
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

// RegisterRelayHandler lida com o pedido POST /relays.
func RegisterRelayHandler(st store.Store, cfg *config.Config, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRelayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteBadRequest(w, "formato JSON do pedido inválido")
			return
		}

		if req.NodeID == "" {
			WriteBadRequest(w, "campo node_id obrigatório")
			return
		}
		if err := ValidateRelayURL(req.URL); err != nil {
			WriteBadRequest(w, fmt.Sprintf("validação de url falhou: %v", err))
			return
		}
		if req.Fingerprint != "" {
			if err := ValidateFingerprint(req.Fingerprint); err != nil {
				WriteBadRequest(w, fmt.Sprintf("validação de fingerprint falhou: %v", err))
				return
			}
		}
		if err := ValidateCapabilities(req.Capabilities); err != nil {
			WriteBadRequest(w, fmt.Sprintf("validação de capabilities falhou: %v", err))
			return
		}

		// 1. Verificar se o node_id existe e está ativo na tabela nodes
		node, err := st.GetNodeByID(r.Context(), req.NodeID)
		if err != nil {
			if errors.Is(err, store.ErrNodeNotFound) {
				WriteNotFound(w, "node", req.NodeID)
				return
			}
			slog.Error("Erro ao validar existência do nó para registo de relay", slog.String("node_id", req.NodeID), slog.String("error", err.Error()))
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
			slog.Error("Erro no UpsertRelay", slog.String("node_id", req.NodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		slog.Info("Relay registado/atualizado com sucesso", slog.String("node_id", req.NodeID), slog.String("url", req.URL))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(toRelayDTO(relay))
	}
}

// HeartbeatRelayHandler lida com o pedido PUT /relays/{node_id}.
func HeartbeatRelayHandler(st store.Store, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "node_id")
		if nodeID == "" {
			WriteBadRequest(w, "parâmetro node_id em falta no caminho")
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
			slog.Error("Erro ao atualizar heartbeat do relay", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteRelayHandler lida com o pedido DELETE /relays/{node_id}.
func DeleteRelayHandler(st store.Store, cache *RelayCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "node_id")
		if nodeID == "" {
			WriteBadRequest(w, "parâmetro node_id em falta no caminho")
			return
		}

		if err := st.DeleteRelay(r.Context(), nodeID); err != nil {
			if errors.Is(err, store.ErrRelayNotFound) {
				WriteNotFound(w, "relay", nodeID)
				return
			}
			slog.Error("Erro ao eliminar relay", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		if cache != nil {
			cache.Invalidate()
		}

		slog.Info("Relay removido do registry com sucesso", slog.String("node_id", nodeID))
		w.WriteHeader(http.StatusNoContent)
	}
}
