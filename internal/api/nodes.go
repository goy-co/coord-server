package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

// StoragePayload representa as estatísticas de armazenamento enviadas no onboarding.
type StoragePayload struct {
	ReservedGB  uint64 `json:"reserved_gb"`
	AvailableGB uint64 `json:"available_gb"`
}

// RegisterNodeRequest representa o body do pedido POST /v1/nodes/register.
type RegisterNodeRequest struct {
	AuthKey string          `json:"auth_key"`
	Name    string          `json:"name,omitempty"`
	Storage *StoragePayload `json:"storage,omitempty"`
	MeshURL string          `json:"mesh_url,omitempty"`
}

// VPNConfigResponse representa a configuração VPN retornada ao nó.
type VPNConfigResponse struct {
	AuthKey    string `json:"auth_key"`
	ControlURL string `json:"control_url"`
}

// RegisterNodeResponse representa a resposta do pedido POST /v1/nodes/register.
type RegisterNodeResponse struct {
	NodeID      string            `json:"node_id"`
	Name        string            `json:"name"`
	MeshURL     string            `json:"mesh_url"`
	VPNConfig   VPNConfigResponse `json:"vpn_config"`
	RegistryURL string            `json:"registry_url"`
	CreatedAt   time.Time         `json:"created_at"`
}

// RegisterNodeHandler lida com o pedido POST /v1/nodes/register.
func RegisterNodeHandler(st store.Store, cfg *config.Config, vpnProvider vpn.VPNProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteBadRequest(w, "formato JSON do pedido inválido")
			return
		}

		if !ValidateAuthKeyFormat(req.AuthKey) {
			WriteBadRequest(w, "formato de auth_key inválido (deve iniciar com 'gc_' e ter pelo menos 20 caracteres)")
			return
		}

		authHash := HashAuthKey(req.AuthKey, cfg.Auth.HMACSecret)

		// 1. Verificar idempotência: nó já registado com esta auth key
		existingNode, err := st.GetNodeByAuthKeyHash(r.Context(), authHash)
		if err == nil && existingNode != nil {
			slog.Info("Registo de nó idempotente (já existente)", slog.String("node_id", existingNode.ID))
			sendRegisterResponse(w, r, existingNode, http.StatusOK, cfg, vpnProvider)
			return
		} else if err != nil && !errors.Is(err, store.ErrNodeNotFound) {
			slog.Error("Erro ao procurar nó por auth key hash", slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		// 2. Criar novo nó
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
			slog.Error("Erro ao guardar novo nó na base de dados", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		slog.Info("Novo nó registado com sucesso", slog.String("node_id", nodeID), slog.String("name", req.Name))
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

	if cfg.VPN.Enabled && vpnProvider != nil {
		key, err := vpnProvider.CreatePreAuthKey(r.Context(), cfg.VPN.PreAuthKeyReusable, cfg.VPN.PreAuthKeyExpiryHours)
		if err != nil {
			slog.Warn("headscale: falha ao gerar pre-auth key, prosseguindo com vpn_config vazio", slog.String("error", err.Error()))
		} else {
			vpnAuthKey = key
			vpnControlURL = vpnProvider.GetControlURL()
			slog.Info("Pre-auth key do Headscale gerada com sucesso para o nó", slog.String("node_id", n.ID))
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
		},
		RegistryURL: registryURL,
		CreatedAt:   n.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetNodeHandler lida com o pedido GET /v1/nodes/{id}.
func GetNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "id")
		if nodeID == "" {
			WriteBadRequest(w, "parâmetro id em falta no caminho")
			return
		}

		node, err := st.GetNodeByID(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, store.ErrNodeNotFound) {
				WriteNotFound(w, "node", nodeID)
				return
			}
			slog.Error("Erro ao procurar nó por ID", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(node)
	}
}

// DeleteNodeHandler lida com o pedido DELETE /v1/nodes/{id} (soft delete).
func DeleteNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := chi.URLParam(r, "id")
		if nodeID == "" {
			WriteBadRequest(w, "parâmetro id em falta no caminho")
			return
		}

		if err := st.DeleteNode(r.Context(), nodeID); err != nil {
			if errors.Is(err, store.ErrNodeNotFound) {
				WriteNotFound(w, "node", nodeID)
				return
			}
			slog.Error("Erro ao eliminar nó por ID", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		slog.Info("Nó marcado como apagado (soft delete)", slog.String("node_id", nodeID))
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetVPNStatusHandler lida com o pedido GET /v1/vpn/status para diagnóstico da infraestrutura VPN.
func GetVPNStatusHandler(vpnProvider vpn.VPNProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if vpnProvider == nil {
			WriteJSONError(w, http.StatusOK, ErrorResponse{Error: "vpn provider not initialized"})
			return
		}

		status, err := vpnProvider.GetStatus(r.Context())
		if err != nil {
			slog.Error("Erro ao obter estado de diagnóstico da VPN", slog.String("error", err.Error()))
			WriteInternalServerError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	}
}
