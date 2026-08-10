package jobs_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/goy-co/coord-server/internal/jobs"
	"github.com/goy-co/coord-server/internal/store"
)

// mockStore implementa store.Store para testes de background jobs.
type mockStore struct {
	mu sync.Mutex

	cleanupRelaysCalls int
	cleanupNodesCalls  int
	countRelayCalls    int
	nodeCountsCalls    int

	cleanupRelaysErr error
	cleanupNodesErr  error
}

func (m *mockStore) Init(_ context.Context) error                   { return nil }
func (m *mockStore) HealthCheck(_ context.Context) error             { return nil }
func (m *mockStore) Close() error                                    { return nil }
func (m *mockStore) CreateNode(_ context.Context, _ *store.Node) error { return nil }
func (m *mockStore) GetNodeByID(_ context.Context, _ string) (*store.Node, error) {
	return nil, store.ErrNodeNotFound
}
func (m *mockStore) GetNodeByAuthKeyHash(_ context.Context, _ string) (*store.Node, error) {
	return nil, store.ErrNodeNotFound
}
func (m *mockStore) UpdateNode(_ context.Context, _ *store.Node) error { return nil }
func (m *mockStore) DeleteNode(_ context.Context, _ string) error      { return nil }
func (m *mockStore) ListNodes(_ context.Context, _ string, _, _ int) ([]store.Node, int, error) {
	return nil, 0, nil
}
func (m *mockStore) TouchNode(_ context.Context, _ string) error { return nil }
func (m *mockStore) CleanupInactiveNodes(_ context.Context, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupNodesCalls++
	return 0, m.cleanupNodesErr
}
func (m *mockStore) GetNodeCountsByStatus(_ context.Context) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeCountsCalls++
	return map[string]int{"active": 2, "inactive": 1}, nil
}
func (m *mockStore) UpsertRelay(_ context.Context, _ *store.Relay) error { return nil }
func (m *mockStore) GetRelayByNodeID(_ context.Context, _ string) (*store.Relay, error) {
	return nil, store.ErrRelayNotFound
}
func (m *mockStore) ListActiveRelays(_ context.Context, _ int, _ *time.Time, _ uint64, _ int) ([]store.Relay, int, error) {
	return nil, 0, nil
}
func (m *mockStore) UpdateRelayHeartbeat(_ context.Context, _ string, _ *uint64) error { return nil }
func (m *mockStore) MarkRelayUnreachable(_ context.Context, _ string) error             { return nil }
func (m *mockStore) DeleteRelay(_ context.Context, _ string) error                      { return nil }
func (m *mockStore) CountActiveRelays(_ context.Context, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.countRelayCalls++
	return 3, nil
}
func (m *mockStore) CleanupStaleRelays(_ context.Context, _ int) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupRelaysCalls++
	return 0, 0, m.cleanupRelaysErr
}

func TestRunnerStop(t *testing.T) {
	ms := &mockStore{}
	r := jobs.NewRunner(ms, 60, 300, 300, 48)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	// Dar tempo para o goroutine iniciar e executar refreshGauges inicial
	time.Sleep(50 * time.Millisecond)
	r.Stop()

	select {
	case <-done:
		// OK: o runner terminou corretamente
	case <-time.After(2 * time.Second):
		t.Fatal("Runner não terminou após Stop() em 2 segundos")
	}
}

func TestRunnerContextCancel(t *testing.T) {
	ms := &mockStore{}
	r := jobs.NewRunner(ms, 60, 300, 300, 48)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel() // cancelar contexto em vez de Stop

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Runner não terminou após cancelar ctx em 2 segundos")
	}
}

func TestRunnerInitialGaugeRefresh(t *testing.T) {
	ms := &mockStore{}
	r := jobs.NewRunner(ms, 60, 300, 300, 48)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	r.Start(ctx) // bloqueia até ctx expirar

	ms.mu.Lock()
	nodeCalls := ms.nodeCountsCalls
	relayCalls := ms.countRelayCalls
	ms.mu.Unlock()

	if nodeCalls == 0 {
		t.Error("GetNodeCountsByStatus não foi chamado durante o arranque do Runner")
	}
	if relayCalls == 0 {
		t.Error("CountActiveRelays não foi chamado durante o arranque do Runner")
	}
}

func TestRunnerStopIdempotent(t *testing.T) {
	ms := &mockStore{}
	r := jobs.NewRunner(ms, 60, 300, 300, 48)

	// Stop antes de Start não deve panic
	r.Stop()
	r.Stop() // idempotente
}
