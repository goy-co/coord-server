package jobs_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/goy-co/coord-server/internal/jobs"
	"github.com/goy-co/coord-server/internal/store"
)

// mockStore implements store.Store for background jobs tests.
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
func (m *mockStore) UpdateRelayFull(_ context.Context, _ *store.Relay) error           { return nil }
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

	// Allow goroutine to start and execute initial refreshGauges
	time.Sleep(50 * time.Millisecond)
	r.Stop()

	select {
	case <-done:
		// OK: runner completed correctly
	case <-time.After(2 * time.Second):
		t.Fatal("Runner did not stop within 2 seconds after Stop()")
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
	cancel() // cancel context instead of calling Stop

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Runner did not stop within 2 seconds after context cancellation")
	}
}

func TestRunnerInitialGaugeRefresh(t *testing.T) {
	ms := &mockStore{}
	r := jobs.NewRunner(ms, 60, 300, 300, 48)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	r.Start(ctx) // blocks until ctx expires

	ms.mu.Lock()
	nodeCalls := ms.nodeCountsCalls
	relayCalls := ms.countRelayCalls
	ms.mu.Unlock()

	if nodeCalls == 0 {
		t.Error("GetNodeCountsByStatus was not called during Runner startup")
	}
	if relayCalls == 0 {
		t.Error("CountActiveRelays was not called during Runner startup")
	}
}

func TestRunnerStopIdempotent(t *testing.T) {
	ms := &mockStore{}
	r := jobs.NewRunner(ms, 60, 300, 300, 48)

	// Calling Stop before Start should not panic
	r.Stop()
	r.Stop() // idempotent
}
