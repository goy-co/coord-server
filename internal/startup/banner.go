package startup

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

// ComponentStatus representa o estado de um componente no startup.
type ComponentStatus struct {
	Name    string
	OK      bool
	Details string
	Warning string // Mensagem adicional se OK=false ou degraded
}

// RunChecks executa verificações de todos os componentes e retorna a lista de status.
func RunChecks(configPath string, cfg *config.Config, db *store.SQLiteStore, vpnProvider vpn.VPNProvider) []ComponentStatus {
	var statuses []ComponentStatus

	// ── 1. Config ──────────────────────────────────────────────────────
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}
	if _, err := os.Stat(configPath); err == nil {
		statuses = append(statuses, ComponentStatus{
			Name:    "Config",
			OK:      true,
			Details: configPath,
		})
	} else {
		statuses = append(statuses, ComponentStatus{
			Name:    "Config",
			OK:      false,
			Details: configPath,
			Warning: fmt.Sprintf("not found (%v)", err),
		})
	}

	// ── 2. Database ────────────────────────────────────────────────────
	if db != nil {
		tableCount, err := db.TableCount()
		if err == nil {
			statuses = append(statuses, ComponentStatus{
				Name:    "Database",
				OK:      true,
				Details: fmt.Sprintf("%s (SQLite WAL, %d tables)", cfg.Database.Path, tableCount),
			})
		} else {
			statuses = append(statuses, ComponentStatus{
				Name:    "Database",
				OK:      false,
				Details: cfg.Database.Path,
				Warning: fmt.Sprintf("unreachable: %v", err),
			})
		}
	} else {
		statuses = append(statuses, ComponentStatus{
			Name:    "Database",
			OK:      false,
			Details: cfg.Database.Path,
			Warning: "not initialized",
		})
	}

	// ── 3. Auth ────────────────────────────────────────────────────────
	if strings.TrimSpace(cfg.Auth.AdminAPIKey) != "" {
		masked := MaskSecret(cfg.Auth.AdminAPIKey)
		statuses = append(statuses, ComponentStatus{
			Name:    "Auth",
			OK:      true,
			Details: fmt.Sprintf("Admin API key loaded (%s)", masked),
		})
	} else {
		statuses = append(statuses, ComponentStatus{
			Name:    "Auth",
			OK:      false,
			Warning: "admin_api_key is empty — all admin endpoints will reject requests",
		})
	}

	// ── 4. VPN ─────────────────────────────────────────────────────────
	if cfg.VPN.Provider == "" {
		statuses = append(statuses, ComponentStatus{
			Name:    "VPN",
			OK:      true, // Não é erro — VPN é opcional
			Details: "disabled (no provider configured)",
		})
	} else if vpnProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := vpnProvider.HealthCheck(ctx); err != nil {
			statuses = append(statuses, ComponentStatus{
				Name:    "VPN",
				OK:      false,
				Details: fmt.Sprintf("%s configured", cfg.VPN.Provider),
				Warning: fmt.Sprintf("%v\n→ node registration will work but VPN keys will NOT be generated\n→ Fix: verify vpn.%s_api_key in config.toml", err, cfg.VPN.Provider),
			})
		} else {
			detail := cfg.VPN.Provider
			if cfg.VPN.Provider == "tailscale" && cfg.VPN.TailscaleTailnet != "" {
				detail += fmt.Sprintf(" (tailnet: %s)", cfg.VPN.TailscaleTailnet)
			}
			if cfg.VPN.Provider == "headscale" && cfg.VPN.HeadscaleURL != "" {
				detail += fmt.Sprintf(" (url: %s)", cfg.VPN.HeadscaleURL)
			}
			statuses = append(statuses, ComponentStatus{
				Name:    "VPN",
				OK:      true,
				Details: detail,
			})
		}
	} else {
		statuses = append(statuses, ComponentStatus{
			Name:    "VPN",
			OK:      false,
			Details: fmt.Sprintf("%s configured", cfg.VPN.Provider),
			Warning: "provider interface is nil — check initialization",
		})
	}

	// ── 5. Rate Limit ──────────────────────────────────────────────────
	statuses = append(statuses, ComponentStatus{
		Name:    "Rate Limit",
		OK:      true,
		Details: fmt.Sprintf("%d req/min", cfg.RateLimit.RequestsPerMinute),
	})

	return statuses
}

// PrintBanner imprime o banner de startup formatado.
func PrintBanner(version string, listenAddr string, statuses []ComponentStatus) {
	fmt.Println()
	fmt.Printf("🟢 Goy Coord-Server %s starting\n", version)

	maxNameLen := 0
	for _, s := range statuses {
		if len(s.Name) > maxNameLen {
			maxNameLen = len(s.Name)
		}
	}

	hasWarnings := false
	for _, s := range statuses {
		icon := "✅"
		if !s.OK {
			icon = "❌"
			hasWarnings = true
		}

		padding := strings.Repeat(" ", maxNameLen-len(s.Name)+1)
		fmt.Printf("   %s:%s%s %s\n", s.Name, padding, icon, s.Details)

		if s.Warning != "" {
			for _, line := range strings.Split(s.Warning, "\n") {
				fmt.Printf("   %s  → %s\n", strings.Repeat(" ", maxNameLen), line)
			}
		}
	}

	fmt.Printf("   Listening on %s\n", listenAddr)

	if hasWarnings {
		fmt.Println("⚠️  Starting with degraded functionality")
	} else {
		fmt.Println("🟢 Ready to accept connections")
	}
	fmt.Println()
}

// MaskSecret mascara segredos para exibição em logs e banners.
func MaskSecret(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}
