package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

//go:embed default_config.toml
var defaultConfigContent string

// Load carrega e valida a configuração do path especificado ou do path canónico.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"config file not found at %s. Run 'coord-server --generate-config' to create one",
				path,
			)
		}
		return nil, fmt.Errorf("failed to read config at %s: %w", path, err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config at %s: %w", path, err)
	}

	// Apply env var overrides if present
	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config at %s: %w", path, err)
	}

	return cfg, nil
}

// GenerateDefault escreve o template default no path especificado.
func GenerateDefault(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	content := defaultConfigContent
	if content == "" {
		content = defaultConfigFileTemplate
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write config at %s: %w", path, err)
	}

	return nil
}

// WriteConfig serializa e escreve a configuração para o path especificado com permissões 0600.
func WriteConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	header := "# Goy Coord-Server Configuration\n" +
		"# Auto-generated on first run.\n" +
		"# Edit manually or re-run with flags to update.\n\n"

	var buf bytes.Buffer
	buf.WriteString(header)
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
