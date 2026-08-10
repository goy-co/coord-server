package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ValidateRelayURL verifica se o URL fornecido utiliza esquema ws:// ou wss:// e possui host/porta válidos.
func ValidateRelayURL(urlStr string) error {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return errors.New("url do relay não pode estar vazia")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("formato de URL inválido: %w", err)
	}

	if u.Scheme != "ws" && u.Scheme != "wss" {
		return fmt.Errorf("esquema de URL inválido ('%s'); deve ser 'ws' ou 'wss'", u.Scheme)
	}

	if u.Host == "" || u.Hostname() == "" {
		return errors.New("URL do relay deve incluir host válido")
	}

	if u.Port() == "" {
		return errors.New("URL do relay deve incluir porta explícita (ex: :8443)")
	}

	return nil
}

// ValidateFingerprint verifica se a fingerprint TLS cumpre o formato 'sha256:{64_hex_chars}'.
func ValidateFingerprint(fp string) error {
	fp = strings.TrimSpace(fp)
	if fp == "" {
		return errors.New("fingerprint não pode estar vazia")
	}

	hexPart := fp
	if strings.HasPrefix(fp, "sha256:") {
		hexPart = strings.TrimPrefix(fp, "sha256:")
	}

	if len(hexPart) != 64 {
		return fmt.Errorf("fingerprint inválida; deve ser um hash SHA-256 de 64 caracteres hexadecimais (comprimento obtido: %d)", len(hexPart))
	}

	if _, err := hex.DecodeString(hexPart); err != nil {
		return fmt.Errorf("caracteres hexadecimais inválidos na fingerprint: %w", err)
	}

	return nil
}

// ValidateCapabilities verifica se a lista de capacidades não possui elementos vazios nem duplicados.
func ValidateCapabilities(caps []string) error {
	seen := make(map[string]bool)

	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c == "" {
			return errors.New("lista de capacidades não pode conter elementos vazios")
		}
		if seen[c] {
			return fmt.Errorf("capacidade duplicada detetada: '%s'", c)
		}
		seen[c] = true
	}

	return nil
}
