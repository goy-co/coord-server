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
	configPathFlag := flag.String("config", "config.toml", "Caminho para o ficheiro de configuração TOML")
	versionFlag := flag.Bool("version", false, "Exibir versão da aplicação e sair")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("coord-server v%s\n", api.ServerVersion)
		os.Exit(0)
	}

	startTime := time.Now()

	// 1. Carregar configuração
	cfg, err := config.Load(*configPathFlag)
	if err != nil {
		slog.Error("Falha ao carregar configuração", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if !cfg.Auth.RequireAuth {
		slog.Warn("ATENÇÃO: Autenticação desativada (require_auth = false). Não utilize em produção!")
	}

	// 2. Inicializar Storage SQLite
	st := store.NewSQLiteStore(cfg.Database.Path)
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()

	if err := st.Init(initCtx); err != nil {
		slog.Error("Falha ao inicializar base de dados SQLite", slog.String("path", cfg.Database.Path), slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("Base de dados SQLite inicializada com sucesso", slog.String("path", cfg.Database.Path))

	// 3. Inicializar VPN Provider (Headscale)
	var vpnProvider vpn.VPNProvider
	if cfg.VPN.Enabled {
		vpnProvider = vpn.NewHeadscaleClient(cfg.VPN.HeadscaleAPIURL, cfg.VPN.HeadscaleAPIKey, cfg.VPN.HeadscaleUser)
		slog.Info("Integração Headscale VPN ativada", slog.String("url", cfg.VPN.HeadscaleAPIURL), slog.String("user", cfg.VPN.HeadscaleUser))
	} else {
		vpnProvider = vpn.NewNoopVPNProvider()
		slog.Info("Integração Headscale VPN desativada (utilizando NoopVPNProvider)")
	}

	// 4. Inicializar Rate Limiter
	rateLimiter := middleware.NewIPRateLimiter()
	defer rateLimiter.Close()

	// 5. Iniciar Background Jobs Runner
	jobRunner := jobs.NewRunner(
		st,
		cfg.Jobs.CleanupRelaysIntervalSeconds,
		cfg.Jobs.CleanupNodesIntervalSeconds,
		cfg.Registry.RelayTTLSeconds,
		48, // nós inativos há mais de 48h são marcados como inativos
	)
	jobsCtx, jobsCancel := context.WithCancel(context.Background())
	defer jobsCancel()
	go jobRunner.Start(jobsCtx)

	// 6. Criar Router Chi
	router := api.NewRouter(cfg, st, startTime, vpnProvider, rateLimiter)

	// 6. Configurar HTTP Server
	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		slog.Info("Servidor coord-server em execução",
			slog.String("version", api.ServerVersion),
			slog.String("listen", cfg.Server.Listen),
			slog.String("db_path", cfg.Database.Path),
			slog.Bool("require_auth", cfg.Auth.RequireAuth),
			slog.Bool("vpn_enabled", cfg.VPN.Enabled),
		)
		serverErrors <- srv.ListenAndServe()
	}()

	// 7. Signal handling para Graceful Shutdown
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Erro fatal no servidor HTTP", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case sig := <-shutdownSignal:
		slog.Info("Sinal de paragem recebido, iniciando graceful shutdown...", slog.String("signal", sig.String()))

		// Parar background jobs primeiro
		jobRunner.Stop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Erro ao encerrar servidor HTTP", slog.String("error", err.Error()))
			_ = srv.Close()
		}

		if err := st.Close(); err != nil {
			slog.Error("Erro ao fechar base de dados SQLite", slog.String("error", err.Error()))
		}

		slog.Info("Graceful shutdown concluído com sucesso")
	}
}
