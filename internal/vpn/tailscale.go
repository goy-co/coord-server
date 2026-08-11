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

// TailscaleClient is the production implementation of the VPNProvider interface for Tailscale.
type TailscaleClient struct {
	tailnet    string
	apiKey     string
	tag        string
	baseURL    string
	httpClient *http.Client
}

// NewTailscaleClient instantiates a new Tailscale client.
func NewTailscaleClient(tailnet, apiKey, tag string) *TailscaleClient {
	return &TailscaleClient{
		tailnet: tailnet,
		apiKey:  apiKey,
		tag:     tag,
		baseURL: "https://api.tailscale.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *TailscaleClient) ProviderName() string {
	return "tailscale"
}

type tailscaleCreateDeviceCapability struct {
	Reusable  bool     `json:"reusable"`
	Ephemeral bool     `json:"ephemeral"`
	Tags      []string `json:"tags,omitempty"`
}

type tailscaleDevicesCapability struct {
	Create tailscaleCreateDeviceCapability `json:"create"`
}

type tailscaleCapabilities struct {
	Devices tailscaleDevicesCapability `json:"devices"`
}

type tailscaleCreateKeyRequest struct {
	Capabilities  tailscaleCapabilities `json:"capabilities"`
	ExpirySeconds int                   `json:"expirySeconds"`
}

type tailscaleCreateKeyResponse struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Created string `json:"created"`
	Expires string `json:"expires"`
}

type tailscaleDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type tailscaleListDevicesResponse struct {
	Devices []tailscaleDevice `json:"devices"`
}

// CreatePreAuthKey generates an auth key on the Tailscale API with single-retry support for transient errors.
func (c *TailscaleClient) CreatePreAuthKey(ctx context.Context, opts CreateKeyOpts) (*VPNConfig, error) {
	expiryHours := opts.ExpiryHours
	if expiryHours <= 0 {
		expiryHours = 24
	}
	expirySeconds := expiryHours * 3600

	tags := opts.Tags
	if len(tags) == 0 && c.tag != "" {
		tags = []string{c.tag}
	}

	reqBody := tailscaleCreateKeyRequest{
		Capabilities: tailscaleCapabilities{
			Devices: tailscaleDevicesCapability{
				Create: tailscaleCreateDeviceCapability{
					Reusable:  opts.Reusable,
					Ephemeral: false,
					Tags:      tags,
				},
			},
		},
		ExpirySeconds: expirySeconds,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		metrics.VPNErrorsTotal.Inc()
		return nil, fmt.Errorf("tailscale: create key failed to encode JSON: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/keys", c.baseURL, c.tailnet)

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			slog.Warn("tailscale: retrying CreatePreAuthKey call after transient error...")
			time.Sleep(1 * time.Second)
		}

		key, retryable, err := c.doCreatePreAuthKey(ctx, endpoint, bodyBytes)
		if err == nil {
			metrics.VPNKeysGeneratedTotal.Inc()
			return &VPNConfig{
				AuthKey:    key,
				ControlURL: "",
				Provider:   "tailscale",
			}, nil
		}

		lastErr = err
		if !retryable {
			break
		}
	}

	metrics.VPNErrorsTotal.Inc()
	return nil, fmt.Errorf("tailscale: create key failed: %w", lastErr)
}

func (c *TailscaleClient) doCreatePreAuthKey(ctx context.Context, endpoint string, bodyBytes []byte) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", false, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", true, err // I/O or timeout error is eligible for retry
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", false, errors.New("invalid or unauthorized Tailscale API key (401)")
	}
	if resp.StatusCode == http.StatusForbidden {
		return "", false, errors.New("insufficient permissions for Tailscale API key (403)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", false, fmt.Errorf("tailnet '%s' not found in Tailscale (404)", c.tailnet)
	}
	if resp.StatusCode >= 500 {
		return "", true, fmt.Errorf("Tailscale server error (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", false, fmt.Errorf("unexpected HTTP response from Tailscale: %d", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}

	var res tailscaleCreateKeyResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", false, fmt.Errorf("failed to decode Tailscale JSON response: %w", err)
	}

	if strings.TrimSpace(res.Key) == "" {
		return "", false, errors.New("Tailscale API response did not contain expected key")
	}

	return res.Key, false, nil
}

// HealthCheck checks connectivity to the Tailscale API.
func (c *TailscaleClient) HealthCheck(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/devices", c.baseURL, c.tailnet)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.VPNErrorsTotal.Inc()
		return fmt.Errorf("tailscale unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		metrics.VPNErrorsTotal.Inc()
		return errors.New("invalid or unauthorized Tailscale API key (401)")
	}
	if resp.StatusCode == http.StatusForbidden {
		metrics.VPNErrorsTotal.Inc()
		return errors.New("insufficient permissions for Tailscale API key (403)")
	}
	if resp.StatusCode != http.StatusOK {
		metrics.VPNErrorsTotal.Inc()
		return fmt.Errorf("tailscale returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// GetStatus retrieves statistics for the diagnostic endpoint /v1/vpn/status.
func (c *TailscaleClient) GetStatus(ctx context.Context) (*VPNStatusResponse, error) {
	status := &VPNStatusResponse{
		VPNEnabled:        true,
		Provider:          "tailscale",
		Tailnet:           c.tailnet,
		RegisteredDevices: 0,
	}

	endpoint := fmt.Sprintf("%s/api/v2/tailnet/%s/devices", c.baseURL, c.tailnet)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return status, nil
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.VPNErrorsTotal.Inc()
		reachable := false
		status.TailscaleReachable = &reachable
		return status, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		reachable := true
		status.TailscaleReachable = &reachable
		var list tailscaleListDevicesResponse
		if err := json.NewDecoder(resp.Body).Decode(&list); err == nil {
			status.RegisteredDevices = len(list.Devices)
		}
	} else {
		metrics.VPNErrorsTotal.Inc()
		reachable := false
		status.TailscaleReachable = &reachable
	}

	return status, nil
}
