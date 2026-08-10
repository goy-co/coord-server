package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/goy-co/coord-server/internal/metrics"
	"github.com/goy-co/coord-server/internal/store"
)

// Runner executa background jobs de manutenção do coord-server.
type Runner struct {
	store                        store.Store
	cleanupRelaysIntervalSeconds int
	cleanupNodesIntervalSeconds  int
	relayTTLSeconds              int
	nodeInactiveThresholdHours   int
	done                         chan struct{}
}

// NewRunner cria um novo Runner de background jobs.
//
//   - cleanupRelaysIntervalSeconds: intervalo entre execuções de limpeza de relays stale.
//   - cleanupNodesIntervalSeconds: intervalo entre execuções de limpeza de nós inativos.
//   - relayTTLSeconds: TTL para considerar um relay como stale.
//   - nodeInactiveThresholdHours: horas de inactividade antes de um nó ser marcado como inativo.
func NewRunner(
	st store.Store,
	cleanupRelaysIntervalSeconds int,
	cleanupNodesIntervalSeconds int,
	relayTTLSeconds int,
	nodeInactiveThresholdHours int,
) *Runner {
	return &Runner{
		store:                        st,
		cleanupRelaysIntervalSeconds: cleanupRelaysIntervalSeconds,
		cleanupNodesIntervalSeconds:  cleanupNodesIntervalSeconds,
		relayTTLSeconds:              relayTTLSeconds,
		nodeInactiveThresholdHours:   nodeInactiveThresholdHours,
		done:                         make(chan struct{}),
	}
}

// Start inicia os background jobs em goroutines separadas.
// Deve ser chamado uma vez após a inicialização do servidor.
// Bloqueia até que ctx seja cancelado ou Stop() seja chamado.
func (r *Runner) Start(ctx context.Context) {
	slog.Info("Background jobs iniciados",
		slog.Int("relay_cleanup_interval_s", r.cleanupRelaysIntervalSeconds),
		slog.Int("node_cleanup_interval_s", r.cleanupNodesIntervalSeconds),
	)

	relayTicker := time.NewTicker(time.Duration(r.cleanupRelaysIntervalSeconds) * time.Second)
	nodeTicker := time.NewTicker(time.Duration(r.cleanupNodesIntervalSeconds) * time.Second)
	gaugeRefreshTicker := time.NewTicker(30 * time.Second)

	defer relayTicker.Stop()
	defer nodeTicker.Stop()
	defer gaugeRefreshTicker.Stop()

	// Atualizar gauges imediatamente ao arrancar
	r.refreshGauges(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Background jobs terminados (contexto cancelado)")
			return
		case <-r.done:
			slog.Info("Background jobs terminados (stop chamado)")
			return
		case <-relayTicker.C:
			r.runCleanupStaleRelays(ctx)
		case <-nodeTicker.C:
			r.runCleanupInactiveNodes(ctx)
		case <-gaugeRefreshTicker.C:
			r.refreshGauges(ctx)
		}
	}
}

// Stop sinaliza ao Runner para parar. Pode ser chamado mesmo se Start ainda não foi chamado.
func (r *Runner) Stop() {
	select {
	case <-r.done:
		// já fechado
	default:
		close(r.done)
	}
}

// runCleanupStaleRelays executa a limpeza de relays stale e regista as métricas.
func (r *Runner) runCleanupStaleRelays(ctx context.Context) {
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	marked, deleted, err := r.store.CleanupStaleRelays(jobCtx, r.relayTTLSeconds)
	if err != nil {
		slog.Error("Erro ao limpar relays stale", slog.String("error", err.Error()))
		return
	}

	if marked > 0 || deleted > 0 {
		slog.Info("Limpeza de relays stale concluída",
			slog.Int("marked_unreachable", marked),
			slog.Int("deleted_expired", deleted),
		)
	}

	// Atualizar gauge de relays ativos após limpeza
	count, err := r.store.CountActiveRelays(jobCtx, r.relayTTLSeconds)
	if err == nil {
		metrics.RelaysActiveTotal.Set(float64(count))
	}
}

// runCleanupInactiveNodes executa a limpeza de nós inativos e regista as métricas.
func (r *Runner) runCleanupInactiveNodes(ctx context.Context) {
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	deactivated, err := r.store.CleanupInactiveNodes(jobCtx, r.nodeInactiveThresholdHours)
	if err != nil {
		slog.Error("Erro ao limpar nós inativos", slog.String("error", err.Error()))
		return
	}

	if deactivated > 0 {
		slog.Info("Limpeza de nós inativos concluída", slog.Int("deactivated", deactivated))
	}

	// Atualizar gauges de nós após limpeza
	r.refreshNodeGauges(jobCtx)
}

// refreshGauges atualiza todos os gauges Prometheus com valores actuais da base de dados.
func (r *Runner) refreshGauges(ctx context.Context) {
	gaugeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	r.refreshNodeGauges(gaugeCtx)

	count, err := r.store.CountActiveRelays(gaugeCtx, r.relayTTLSeconds)
	if err != nil {
		slog.Warn("Erro ao obter contagem de relays ativos para gauge", slog.String("error", err.Error()))
		return
	}
	metrics.RelaysActiveTotal.Set(float64(count))
}

// refreshNodeGauges actualiza os gauges de nós.
func (r *Runner) refreshNodeGauges(ctx context.Context) {
	counts, err := r.store.GetNodeCountsByStatus(ctx)
	if err != nil {
		slog.Warn("Erro ao obter contagens de nós para gauge", slog.String("error", err.Error()))
		return
	}
	for status, count := range counts {
		metrics.NodesTotal.WithLabelValues(status).Set(float64(count))
	}
}
