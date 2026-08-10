package api_test

import (
	"strings"
	"testing"

	"github.com/goy-co/coord-server/internal/api"
)

func TestValidateAuthKeyFormat(t *testing.T) {
	validKeys := []string{
		"gc_12345678901234567890",
		"gc_abcdefghijklmnopqrstuvwxyz",
		"gc_SECRET_KEY_123456789012345",
	}

	invalidKeys := []string{
		"",
		"gc_short",
		"invalid_prefix_1234567890",
		"12345678901234567890",
		"gc_12345",
	}

	for _, k := range validKeys {
		if !api.ValidateAuthKeyFormat(k) {
			t.Errorf("Esperava key válida para '%s'", k)
		}
	}

	for _, k := range invalidKeys {
		if api.ValidateAuthKeyFormat(k) {
			t.Errorf("Esperava key inválida para '%s'", k)
		}
	}
}

func TestHashAuthKey(t *testing.T) {
	key := "gc_12345678901234567890"
	secret := "my-secret-key"

	h1 := api.HashAuthKey(key, secret)
	h2 := api.HashAuthKey(key, secret)

	if h1 != h2 {
		t.Errorf("Hash HMAC deve ser determinístico")
	}

	if len(h1) == 0 {
		t.Errorf("Hash HMAC não pode ser vazio")
	}

	h3 := api.HashAuthKey(key, "different-secret")
	if h1 == h3 {
		t.Errorf("Hashes com secrets diferentes não devem ser iguais")
	}
}

func TestGenerateNodeID(t *testing.T) {
	id1 := api.GenerateNodeID()
	id2 := api.GenerateNodeID()

	if id1 == id2 {
		t.Errorf("Node IDs gerados devem ser únicos")
	}

	if !strings.HasPrefix(id1, "goy-node-") {
		t.Errorf("Node ID deve ter o prefixo 'goy-node-', obtido: %s", id1)
	}
}
