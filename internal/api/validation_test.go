package api_test

import (
	"testing"

	"github.com/goy-co/coord-server/internal/api"
)

func TestValidateRelayURL(t *testing.T) {
	validURLs := []string{
		"ws://100.80.1.5:8443",
		"wss://relay.goy.network:443",
		"ws://localhost:8080/ws",
	}

	invalidURLs := []string{
		"",
		"http://100.80.1.5:8443",
		"https://100.80.1.5:8443",
		"ws://100.80.1.5", // missing port
		"ws://:8443",      // missing host
		"invalid-url",
	}

	for _, u := range validURLs {
		if err := api.ValidateRelayURL(u); err != nil {
			t.Errorf("Expected valid URL for '%s', got error: %v", u, err)
		}
	}

	for _, u := range invalidURLs {
		if err := api.ValidateRelayURL(u); err == nil {
			t.Errorf("Expected error for invalid URL '%s'", u)
		}
	}
}

func TestValidateFingerprint(t *testing.T) {
	validFP := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if err := api.ValidateFingerprint(validFP); err != nil {
		t.Errorf("Expected valid fingerprint, got error: %v", err)
	}

	invalidFPs := []string{
		"",
		"sha256:short",
		"sha256:invalid_hex_chars_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	}

	for _, fp := range invalidFPs {
		if err := api.ValidateFingerprint(fp); err == nil {
			t.Errorf("Expected error for invalid fingerprint '%s'", fp)
		}
	}
}

func TestValidateCapabilities(t *testing.T) {
	validCaps := []string{"nip09", "nip40", "backfill"}
	if err := api.ValidateCapabilities(validCaps); err != nil {
		t.Errorf("Expected valid capabilities, got error: %v", err)
	}

	invalidCapsList := [][]string{
		{"nip09", ""},
		{"nip09", "nip09"}, // duplicate
	}

	for _, caps := range invalidCapsList {
		if err := api.ValidateCapabilities(caps); err == nil {
			t.Errorf("Expected validation error for capabilities list: %v", caps)
		}
	}
}
