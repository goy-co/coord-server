package config

import (
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
