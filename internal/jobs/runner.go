package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/goy-co/coord-server/internal/metrics"
	"github.com/goy-co/coord-server/internal/store"
)

// Runner executes background maintenance jobs for coord-server.
type Runner struct {
	store                        store.Store
	cleanupRelaysIntervalSeconds int
	cleanupNodesIntervalSeconds  int
	relayTTLSeconds              int
	nodeInactiveThresholdHours   int
	done                         chan struct{}
}

// NewRunner creates a new background jobs Runner.
//
//   - cleanupRelaysIntervalSeconds: interval between stale relay cleanup runs.
//   - cleanupNodesIntervalSeconds: interval between inactive node cleanup runs.
//   - relayTTLSeconds: TTL threshold to consider a relay stale.
//   - nodeInactiveThresholdHours: hours of inactivity before marking a node inactive.
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

// Start starts the background jobs loop.
// Should be called once after server initialization.
// Blocks until ctx is cancelled or Stop() is called.
func (r *Runner) Start(ctx context.Context) {
	slog.Info("Background jobs started",
		slog.Int("relay_cleanup_interval_s", r.cleanupRelaysIntervalSeconds),
		slog.Int("node_cleanup_interval_s", r.cleanupNodesIntervalSeconds),
	)

	relayTicker := time.NewTicker(time.Duration(r.cleanupRelaysIntervalSeconds) * time.Second)
	nodeTicker := time.NewTicker(time.Duration(r.cleanupNodesIntervalSeconds) * time.Second)
	gaugeRefreshTicker := time.NewTicker(30 * time.Second)

	defer relayTicker.Stop()
	defer nodeTicker.Stop()
	defer gaugeRefreshTicker.Stop()

	// Update gauges immediately on start
	r.refreshGauges(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Background jobs stopped (context cancelled)")
			return
		case <-r.done:
			slog.Info("Background jobs stopped (stop called)")
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

// Stop signals the Runner to stop. Safe to call even if Start was not called.
func (r *Runner) Stop() {
	select {
	case <-r.done:
		// already closed
	default:
		close(r.done)
	}
}

// runCleanupStaleRelays runs stale relay cleanup and records metrics.
func (r *Runner) runCleanupStaleRelays(ctx context.Context) {
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	marked, deleted, err := r.store.CleanupStaleRelays(jobCtx, r.relayTTLSeconds)
	if err != nil {
		slog.Error("Error cleaning up stale relays", slog.String("error", err.Error()))
		return
	}

	if marked > 0 || deleted > 0 {
		slog.Info("Stale relay cleanup completed",
			slog.Int("marked_unreachable", marked),
			slog.Int("deleted_expired", deleted),
		)
	}

	// Update active relay gauge after cleanup
	count, err := r.store.CountActiveRelays(jobCtx, r.relayTTLSeconds)
	if err == nil {
		metrics.RelaysActiveTotal.Set(float64(count))
	}
}

// runCleanupInactiveNodes runs inactive node cleanup and records metrics.
func (r *Runner) runCleanupInactiveNodes(ctx context.Context) {
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	deactivated, err := r.store.CleanupInactiveNodes(jobCtx, r.nodeInactiveThresholdHours)
	if err != nil {
		slog.Error("Error cleaning up inactive nodes", slog.String("error", err.Error()))
		return
	}

	if deactivated > 0 {
		slog.Info("Inactive node cleanup completed", slog.Int("deactivated", deactivated))
	}

	// Update node gauges after cleanup
	r.refreshNodeGauges(jobCtx)
}

// refreshGauges updates all Prometheus gauges with current database values.
func (r *Runner) refreshGauges(ctx context.Context) {
	gaugeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	r.refreshNodeGauges(gaugeCtx)

	count, err := r.store.CountActiveRelays(gaugeCtx, r.relayTTLSeconds)
	if err != nil {
		slog.Warn("Error fetching active relay count for gauge", slog.String("error", err.Error()))
		return
	}
	metrics.RelaysActiveTotal.Set(float64(count))
}

// refreshNodeGauges updates node gauges.
func (r *Runner) refreshNodeGauges(ctx context.Context) {
	counts, err := r.store.GetNodeCountsByStatus(ctx)
	if err != nil {
		slog.Warn("Error fetching node status counts for gauge", slog.String("error", err.Error()))
		return
	}
	for status, count := range counts {
		metrics.NodesTotal.WithLabelValues(status).Set(float64(count))
	}
}
