package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ValidateRelayURL verifies that the provided URL uses the ws:// or wss:// scheme and has a valid host and port.
func ValidateRelayURL(urlStr string) error {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return errors.New("relay URL cannot be empty")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if u.Scheme != "ws" && u.Scheme != "wss" {
		return fmt.Errorf("invalid URL scheme ('%s'); must be 'ws' or 'wss'", u.Scheme)
	}

	if u.Host == "" || u.Hostname() == "" {
		return errors.New("relay URL must include a valid host")
	}

	if u.Port() == "" {
		return errors.New("relay URL must include an explicit port (e.g. :8443)")
	}

	return nil
}

// ValidateFingerprint verifies that the TLS fingerprint matches the format 'sha256:{64_hex_chars}'.
func ValidateFingerprint(fp string) error {
	fp = strings.TrimSpace(fp)
	if fp == "" {
		return errors.New("fingerprint cannot be empty")
	}

	hexPart := fp
	if strings.HasPrefix(fp, "sha256:") {
		hexPart = strings.TrimPrefix(fp, "sha256:")
	}

	if len(hexPart) != 64 {
		return fmt.Errorf("invalid fingerprint; must be a 64-character hex SHA-256 hash (length obtained: %d)", len(hexPart))
	}

	if _, err := hex.DecodeString(hexPart); err != nil {
		return fmt.Errorf("invalid hexadecimal characters in fingerprint: %w", err)
	}

	return nil
}

// ValidateCapabilities verifies that the capability list does not contain empty or duplicate elements.
func ValidateCapabilities(caps []string) error {
	seen := make(map[string]bool)

	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c == "" {
			return errors.New("capabilities list cannot contain empty elements")
		}
		if seen[c] {
			return fmt.Errorf("duplicate capability detected: '%s'", c)
		}
		seen[c] = true
	}

	return nil
}
