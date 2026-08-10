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

// ValidateAuthKeyFormat verifies that the auth key meets the expected format ('gc_' prefix, length >= 20).
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

// HashAuthKey computes the HMAC-SHA256 hash of the auth key using the configured secret.
func HashAuthKey(key string, secret string) string {
	if secret == "" {
		secret = DefaultHMACSecret
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateNodeID generates a unique node identifier in the format "goy-node-{random_hex_8}".
func GenerateNodeID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Emergency fallback if entropy source fails
		return fmt.Sprintf("goy-node-%x", hex.EncodeToString([]byte("fallback")))
	}
	return fmt.Sprintf("goy-node-%s", hex.EncodeToString(bytes))
}
