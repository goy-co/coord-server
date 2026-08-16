package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goy-co/coord-server/internal/api"
	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/jobs"
	"github.com/goy-co/coord-server/internal/middleware"
	"github.com/goy-co/coord-server/internal/store"
	"github.com/goy-co/coord-server/internal/vpn"
)

func main() {
	configPathFlag := flag.String("config", "", "Path to TOML configuration file")
	bindFlag := flag.String("bind", "", "HTTP server listen address (e.g. 0.0.0.0:8080)")
	adminKeyFlag := flag.String("admin-api-key", "", "Administrator API key")
	vpnProviderFlag := flag.String("vpn-provider", "", "VPN provider (tailscale, headscale, or empty)")
	dbPathFlag := flag.String("db-path", "", "Path to SQLite database file")
	logLevelFlag := flag.String("log-level", "", "Logging level (trace, debug, info, warn, error)")
	logFormatFlag := flag.String("log-format", "", "Logging format (pretty, json)")
	generateConfigFlag := flag.Bool("generate-config", false, "Generate default config file and exit")
	validateConfigFlag := flag.Bool("validate-config", false, "Validate configuration and exit")
	versionFlag := flag.Bool("version", false, "Display application version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("coord-server v%s\n", api.ServerVersion)
		os.Exit(0)
	}

	if *generateConfigFlag {
		targetPath := *configPathFlag
		if targetPath == "" {
			targetPath = config.DefaultConfigPath()
		}
		if err := config.GenerateDefault(targetPath); err != nil {
			slog.Error("Failed to generate default configuration", slog.String("error", err.Error()))
			os.Exit(1)
		}
		fmt.Printf("Default configuration generated at: %s\n", targetPath)
		os.Exit(0)
	}

	startTime := time.Now()

	// 1. Resolve configuration with priority cascade
	resolved, err := config.Resolve(config.ResolveOptions{
		ConfigPath:   *configPathFlag,
		Bind:         *bindFlag,
		AdminAPIKey:  *adminKeyFlag,
		VPNProvider:  *vpnProviderFlag,
		DBPath:       *dbPathFlag,
		LogLevel:     *logLevelFlag,
		LogFormat:    *logFormatFlag,
		ValidateOnly: *validateConfigFlag,
	})
	if err != nil {
		slog.Error("Configuration resolution failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	for _, w := range resolved.Warnings {
		slog.Warn(w)
	}

	if *validateConfigFlag {
		fmt.Println("Configuration is valid.")
		os.Exit(0)
	}

	cfg := resolved.Config

	if !cfg.Auth.RequireAuth {
		slog.Warn("WARNING: Authentication disabled (require_auth = false). Do not use in production!")
	}

	// 2. Initialize SQLite Storage
	st := store.NewSQLiteStore(cfg.Database.Path)
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()

	if err := st.Init(initCtx); err != nil {
		slog.Error("Failed to initialize SQLite database", slog.String("path", cfg.Database.Path), slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("SQLite database initialized successfully", slog.String("path", cfg.Database.Path))

	// 3. Initialize VPN Provider
	vpnProvider, err := vpn.NewVPNProvider(&cfg.VPN)
	if err != nil {
		slog.Error("Failed to initialize VPN provider", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if cfg.VPN.Enabled && cfg.VPN.Provider != "" {
		slog.Info("VPN integration enabled", slog.String("provider", vpnProvider.ProviderName()))
	} else {
		slog.Info("VPN integration disabled (using NoopVPNProvider)")
	}

	// 4. Initialize Rate Limiter
	rateLimiter := middleware.NewIPRateLimiter()
	defer rateLimiter.Close()

	// 5. Start Background Jobs Runner
	jobRunner := jobs.NewRunner(
		st,
		cfg.Jobs.CleanupRelaysIntervalSeconds,
		cfg.Jobs.CleanupNodesIntervalSeconds,
		cfg.Registry.RelayTTLSeconds,
		48, // mark nodes inactive after 48h of inactivity
	)
	jobsCtx, jobsCancel := context.WithCancel(context.Background())
	defer jobsCancel()
	go jobRunner.Start(jobsCtx)

	// 6. Create Chi Router
	router := api.NewRouter(cfg, st, startTime, vpnProvider, rateLimiter)

	// 7. Configure HTTP Server
	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		slog.Info("coord-server running",
			slog.String("version", api.ServerVersion),
			slog.String("listen", cfg.Server.Listen),
			slog.String("db_path", cfg.Database.Path),
			slog.Bool("require_auth", cfg.Auth.RequireAuth),
			slog.Bool("vpn_enabled", cfg.VPN.Enabled),
		)
		serverErrors <- srv.ListenAndServe()
	}()

	// 8. Signal handling for Graceful Shutdown
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Fatal error in HTTP server", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case sig := <-shutdownSignal:
		slog.Info("Shutdown signal received, starting graceful shutdown...", slog.String("signal", sig.String()))

		// Stop background jobs first
		jobRunner.Stop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Error shutting down HTTP server", slog.String("error", err.Error()))
			_ = srv.Close()
		}

		if err := st.Close(); err != nil {
			slog.Error("Error closing SQLite database", slog.String("error", err.Error()))
		}

		slog.Info("Graceful shutdown completed successfully")
	}
}
