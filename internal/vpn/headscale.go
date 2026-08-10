package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/goy-co/coord-server/internal/metrics"
)

// HeadscaleClient é a implementação de produção da interface VPNProvider para o Headscale.
type HeadscaleClient struct {
	baseURL    string
	apiKey     string
	user       string
	httpClient *http.Client
}

// NewHeadscaleClient instancia um novo cliente Headscale.
func NewHeadscaleClient(baseURL, apiKey, user string) *HeadscaleClient {
	baseURL = strings.TrimSuffix(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/v1")

	return &HeadscaleClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		user:    user,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HeadscaleClient) GetControlURL() string {
	return c.baseURL
}

type createPreAuthKeyRequest struct {
	User       string `json:"user"`
	Reusable   bool   `json:"reusable"`
	Ephemeral  bool   `json:"ephemeral"`
	Expiration string `json:"expiration"`
}

type preAuthKeyObject struct {
	Key        string `json:"key"`
	Expiration string `json:"expiration"`
}

type createPreAuthKeyResponse struct {
	PreAuthKey preAuthKeyObject `json:"preAuthKey"`
	Key        string           `json:"key,omitempty"` // Fallback em algumas versões da API
}

type machineObject struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	IPAddresses []string  `json:"ipAddresses"`
	LastSeen    time.Time `json:"lastSeen"`
}

type listNodesResponse struct {
	Nodes []machineObject `json:"nodes"`
}

// CreatePreAuthKey gera uma pre-auth key na API Headscale com suporte a 1 retry para erros transitórios.
func (c *HeadscaleClient) CreatePreAuthKey(ctx context.Context, reusable bool, expiryHours int) (string, error) {
	if expiryHours <= 0 {
		expiryHours = 24
	}

	expiration := time.Now().UTC().Add(time.Duration(expiryHours) * time.Hour).Format(time.RFC3339)
	reqBody := createPreAuthKeyRequest{
		User:       c.user,
		Reusable:   reusable,
		Ephemeral:  false,
		Expiration: expiration,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		metrics.VPNErrorsTotal.Inc()
		return "", fmt.Errorf("headscale: falha ao codificar JSON: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/v1/preauthkey", c.baseURL)

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			slog.Warn("headscale: a tentar novamente chamada CreatePreAuthKey após erro transitório...")
			time.Sleep(1 * time.Second)
		}

		key, retryable, err := c.doCreatePreAuthKey(ctx, endpoint, bodyBytes)
		if err == nil {
			metrics.VPNKeysGeneratedTotal.Inc()
			return key, nil
		}

		lastErr = err
		if !retryable {
			break
		}
	}

	metrics.VPNErrorsTotal.Inc()
	return "", fmt.Errorf("headscale: falha ao gerar pre-auth key: %w", lastErr)
}

func (c *HeadscaleClient) doCreatePreAuthKey(ctx context.Context, endpoint string, bodyBytes []byte) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", false, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", true, err // Erro de I/O ou timeout é elegível para retry
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", false, errors.New("chave de API do Headscale inválida ou não autorizada (401/403)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", false, fmt.Errorf("usuário '%s' não encontrado no Headscale (404)", c.user)
	}
	if resp.StatusCode >= 500 {
		return "", true, fmt.Errorf("erro no servidor Headscale (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("resposta HTTP inesperada do Headscale: %d", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}

	var res createPreAuthKeyResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", false, fmt.Errorf("falha ao descodificar resposta JSON do Headscale: %w", err)
	}

	key := res.PreAuthKey.Key
	if key == "" {
		key = res.Key
	}

	if key == "" {
		return "", false, errors.New("resposta da API Headscale não continha a chave pre-auth esperada")
	}

	return key, false, nil
}

// HealthCheck verifica a acessibilidade da API Headscale.
func (c *HeadscaleClient) HealthCheck(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/api/v1/node?user=%s", c.baseURL, c.user)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.VPNErrorsTotal.Inc()
		return fmt.Errorf("headscale inacessível: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		metrics.VPNErrorsTotal.Inc()
		return fmt.Errorf("headscale retornou HTTP %d", resp.StatusCode)
	}

	return nil
}

// GetStatus obtém estatísticas para o endpoint de diagnóstico /v1/vpn/status.
func (c *HeadscaleClient) GetStatus(ctx context.Context) (*VPNStatusResponse, error) {
	status := &VPNStatusResponse{
		VPNEnabled:         true,
		HeadscaleReachable: false,
		HeadscaleUser:      c.user,
		RegisteredMachines: 0,
	}

	endpoint := fmt.Sprintf("%s/api/v1/node?user=%s", c.baseURL, c.user)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return status, nil
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.VPNErrorsTotal.Inc()
		return status, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		status.HeadscaleReachable = true
		var list listNodesResponse
		if err := json.NewDecoder(resp.Body).Decode(&list); err == nil {
			status.RegisteredMachines = len(list.Nodes)
		}
	} else {
		metrics.VPNErrorsTotal.Inc()
	}

	return status, nil
}
