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
	configPathFlag := flag.String("config", "config.toml", "Path to TOML configuration file")
	versionFlag := flag.Bool("version", false, "Display application version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("coord-server v%s\n", api.ServerVersion)
		os.Exit(0)
	}

	startTime := time.Now()

	// 1. Load configuration
	cfg, err := config.Load(*configPathFlag)
	if err != nil {
		slog.Error("Failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

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

	// 3. Initialize VPN Provider (Headscale)
	var vpnProvider vpn.VPNProvider
	if cfg.VPN.Enabled {
		vpnProvider = vpn.NewHeadscaleClient(cfg.VPN.HeadscaleAPIURL, cfg.VPN.HeadscaleAPIKey, cfg.VPN.HeadscaleUser)
		slog.Info("Headscale VPN integration enabled", slog.String("url", cfg.VPN.HeadscaleAPIURL), slog.String("user", cfg.VPN.HeadscaleUser))
	} else {
		vpnProvider = vpn.NewNoopVPNProvider()
		slog.Info("Headscale VPN integration disabled (using NoopVPNProvider)")
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
