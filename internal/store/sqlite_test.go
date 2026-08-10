package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/goy-co/coord-server/internal/store"
)

func TestSQLiteStoreNodeCRUD(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_nodes.db")

	s := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := s.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer s.Close()

	// 1. Create Node
	n1 := &store.Node{
		ID:                 "goy-node-11111111",
		AuthKeyHash:        "hash11111111",
		Name:               "node-1",
		StorageReservedGB:  100,
		StorageAvailableGB: 50,
		MeshURL:            "100.64.0.1:8443",
	}

	if err := s.CreateNode(ctx, n1); err != nil {
		t.Fatalf("Error creating node 1: %v", err)
	}

	// 2. Get Node by ID
	found, err := s.GetNodeByID(ctx, n1.ID)
	if err != nil {
		t.Fatalf("Error looking up node by ID: %v", err)
	}
	if found.Name != "node-1" || found.StorageReservedGB != 100 {
		t.Errorf("Incorrect node data: %+v", found)
	}

	// 3. Get Node by AuthKeyHash
	foundHash, err := s.GetNodeByAuthKeyHash(ctx, n1.AuthKeyHash)
	if err != nil {
		t.Fatalf("Error looking up node by AuthKeyHash: %v", err)
	}
	if foundHash.ID != n1.ID {
		t.Errorf("Unexpected ID for auth key hash: %s", foundHash.ID)
	}

	// 4. Update Node
	n1.Name = "node-1-updated"
	n1.StorageAvailableGB = 80
	if err := s.UpdateNode(ctx, n1); err != nil {
		t.Fatalf("Error updating node: %v", err)
	}

	updated, err := s.GetNodeByID(ctx, n1.ID)
	if err != nil {
		t.Fatalf("Error looking up updated node: %v", err)
	}
	if updated.Name != "node-1-updated" || updated.StorageAvailableGB != 80 {
		t.Errorf("Node data not updated correctly: %+v", updated)
	}

	// 5. Touch Node (LastSeenAt)
	if err := s.TouchNode(ctx, n1.ID); err != nil {
		t.Fatalf("Error performing TouchNode: %v", err)
	}
	touched, err := s.GetNodeByID(ctx, n1.ID)
	if err != nil {
		t.Fatalf("Error looking up node after TouchNode: %v", err)
	}
	if touched.LastSeenAt == nil {
		t.Errorf("LastSeenAt should be non-nil after TouchNode")
	}

	// 6. ListNodes
	n2 := &store.Node{
		ID:                 "goy-node-22222222",
		AuthKeyHash:        "hash22222222",
		Name:               "node-2",
		StorageReservedGB:  200,
		StorageAvailableGB: 150,
	}
	if err := s.CreateNode(ctx, n2); err != nil {
		t.Fatalf("Error creating node 2: %v", err)
	}

	nodes, total, err := s.ListNodes(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("Error listing nodes: %v", err)
	}
	if total != 2 || len(nodes) != 2 {
		t.Errorf("Expected total 2 nodes, got total=%d, len=%d", total, len(nodes))
	}

	// 7. Soft Delete
	if err := s.DeleteNode(ctx, n1.ID); err != nil {
		t.Fatalf("Error performing soft delete on node 1: %v", err)
	}

	_, err = s.GetNodeByID(ctx, n1.ID)
	if err != store.ErrNodeNotFound {
		t.Errorf("Expected ErrNodeNotFound for deleted node, got: %v", err)
	}

	nodesAfterDelete, totalAfterDelete, err := s.ListNodes(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("Error listing nodes after delete: %v", err)
	}
	if totalAfterDelete != 1 || len(nodesAfterDelete) != 1 {
		t.Errorf("Expected 1 active node remaining, got total=%d, len=%d", totalAfterDelete, len(nodesAfterDelete))
	}
}

func TestSQLiteStoreRelayCRUD(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_relays.db")

	s := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := s.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize SQLiteStore: %v", err)
	}
	defer s.Close()

	// Create Node to serve as Foreign Key
	nodeID := "goy-node-relay-1"
	n := &store.Node{
		ID:          nodeID,
		AuthKeyHash: "hash-relay-1",
		Name:        "node-relay-1",
	}
	if err := s.CreateNode(ctx, n); err != nil {
		t.Fatalf("Error creating parent node: %v", err)
	}

	// 1. Upsert Relay (Create)
	r1 := &store.Relay{
		NodeID:             nodeID,
		URL:                "ws://100.80.1.5:8443",
		Fingerprint:        "sha256:abc123def456",
		StorageReservedGB:  150,
		StorageAvailableGB: 100,
		ReplicationFactor:  3,
		Version:            "0.1.1-alpha",
		Capabilities:       []string{"nip09", "nip40"},
		Status:             store.RelayStatusActive,
		LastSeenAt:         time.Now().UTC(),
	}

	if err := s.UpsertRelay(ctx, r1); err != nil {
		t.Fatalf("Error inserting relay: %v", err)
	}

	// 2. Get Relay By Node ID
	found, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Error looking up relay by NodeID: %v", err)
	}
	if found.URL != r1.URL || found.StorageAvailableGB != 100 || len(found.Capabilities) != 2 {
		t.Errorf("Incorrect relay data: %+v", found)
	}

	// 3. Upsert Relay (Update)
	r1.StorageAvailableGB = 120
	r1.Capabilities = []string{"nip09", "nip40", "backfill"}
	if err := s.UpsertRelay(ctx, r1); err != nil {
		t.Fatalf("Error updating relay via Upsert: %v", err)
	}

	updated, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Error looking up updated relay: %v", err)
	}
	if updated.StorageAvailableGB != 120 || len(updated.Capabilities) != 3 {
		t.Errorf("Relay not updated correctly: %+v", updated)
	}

	// 4. Update Heartbeat
	newStorage := uint64(90)
	if err := s.UpdateRelayHeartbeat(ctx, nodeID, &newStorage); err != nil {
		t.Fatalf("Error in UpdateRelayHeartbeat: %v", err)
	}

	hbFound, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Error looking up relay post heartbeat: %v", err)
	}
	if hbFound.StorageAvailableGB != 90 {
		t.Errorf("Unexpected storage in heartbeat: %d", hbFound.StorageAvailableGB)
	}

	// 5. ListActiveRelays
	relays, total, err := s.ListActiveRelays(ctx, 300, nil, 50, 10)
	if err != nil {
		t.Fatalf("Error listing active relays: %v", err)
	}
	if total != 1 || len(relays) != 1 {
		t.Errorf("Expected 1 active relay, got total=%d, len=%d", total, len(relays))
	}

	// 6. MarkRelayUnreachable
	if err := s.MarkRelayUnreachable(ctx, nodeID); err != nil {
		t.Fatalf("Error marking relay unreachable: %v", err)
	}
	unrFound, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Error looking up unreachable relay: %v", err)
	}
	if unrFound.Status != store.RelayStatusUnreachable {
		t.Errorf("Expected status unreachable, got: %s", unrFound.Status)
	}

	// 7. Delete Relay (Hard Delete)
	if err := s.DeleteRelay(ctx, nodeID); err != nil {
		t.Fatalf("Error deleting relay: %v", err)
	}
	_, err = s.GetRelayByNodeID(ctx, nodeID)
	if err != store.ErrRelayNotFound {
		t.Errorf("Expected ErrRelayNotFound after deleting relay, got: %v", err)
	}
}
