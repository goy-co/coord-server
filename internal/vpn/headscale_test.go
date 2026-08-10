package vpn_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goy-co/coord-server/internal/vpn"
)

func TestHeadscaleClientCreatePreAuthKeySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/preauthkey" {
			t.Fatalf("Caminho inesperado: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Fatalf("Header Authorization incorreto: %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"preAuthKey": {"key": "tskey-auth-1234567890", "expiration": "2026-08-11T12:00:00Z"}}`))
	}))
	defer server.Close()

	client := vpn.NewHeadscaleClient(server.URL, "test-api-key", "goy-nodes")
	ctx := context.Background()

	key, err := client.CreatePreAuthKey(ctx, false, 24)
	if err != nil {
		t.Fatalf("Esperava sucesso na geração de pre-auth key, obtido erro: %v", err)
	}

	if key != "tskey-auth-1234567890" {
		t.Errorf("Esperava key 'tskey-auth-1234567890', obtida: '%s'", key)
	}

	if client.GetControlURL() != server.URL {
		t.Errorf("ControlURL esperada %s, obtida: %s", server.URL, client.GetControlURL())
	}
}

func TestHeadscaleClientCreatePreAuthKeyUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := vpn.NewHeadscaleClient(server.URL, "invalid-key", "goy-nodes")
	ctx := context.Background()

	_, err := client.CreatePreAuthKey(ctx, false, 24)
	if err == nil {
		t.Fatalf("Esperava erro para chave de API não autorizada")
	}
}

func TestHeadscaleClientRetryOnServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"preAuthKey": {"key": "tskey-auth-recovered"}}`))
	}))
	defer server.Close()

	client := vpn.NewHeadscaleClient(server.URL, "test-key", "goy-nodes")
	ctx := context.Background()

	key, err := client.CreatePreAuthKey(ctx, false, 24)
	if err != nil {
		t.Fatalf("Esperava sucesso após retry, obtido erro: %v", err)
	}

	if key != "tskey-auth-recovered" {
		t.Errorf("Esperava key 'tskey-auth-recovered', obtida: '%s'", key)
	}

	if attempts != 2 {
		t.Errorf("Esperava 2 tentativas HTTP, obtidas: %d", attempts)
	}
}
