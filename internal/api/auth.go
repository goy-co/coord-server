package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	AuthKeyPrefix     = "gc_"
	MinAuthKeyLength  = 20
	DefaultHMACSecret = "coord-server-default-hmac-secret"
)

// ValidateAuthKeyFormat verifica se a auth key cumpre o formato esperado (prefixo 'gc_', comprimento >= 20).
func ValidateAuthKeyFormat(key string) bool {
	key = strings.TrimSpace(key)
	if len(key) < MinAuthKeyLength {
		return false
	}
	if !strings.HasPrefix(key, AuthKeyPrefix) {
		return false
	}
	return true
}

// HashAuthKey computa o hash HMAC-SHA256 da auth key utilizando o secret configurado.
func HashAuthKey(key string, secret string) string {
	if secret == "" {
		secret = DefaultHMACSecret
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateNodeID gera um identificador único de nó no formato "goy-node-{random_hex_8}".
func GenerateNodeID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback estático de emergência se a fonte entrópica falhar
		return fmt.Sprintf("goy-node-%x", hex.EncodeToString([]byte("fallback")))
	}
	return fmt.Sprintf("goy-node-%s", hex.EncodeToString(bytes))
}
