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
		t.Fatalf("Erro ao inicializar SQLiteStore: %v", err)
	}
	defer s.Close()

	// 1. Criar Nó
	n1 := &store.Node{
		ID:                 "goy-node-11111111",
		AuthKeyHash:        "hash11111111",
		Name:               "node-1",
		StorageReservedGB:  100,
		StorageAvailableGB: 50,
		MeshURL:            "100.64.0.1:8443",
	}

	if err := s.CreateNode(ctx, n1); err != nil {
		t.Fatalf("Erro ao criar nó 1: %v", err)
	}

	// 2. Procurar Nó por ID
	found, err := s.GetNodeByID(ctx, n1.ID)
	if err != nil {
		t.Fatalf("Erro ao procurar nó por ID: %v", err)
	}
	if found.Name != "node-1" || found.StorageReservedGB != 100 {
		t.Errorf("Dados de nó incorretos: %+v", found)
	}

	// 3. Procurar Nó por AuthKeyHash
	foundHash, err := s.GetNodeByAuthKeyHash(ctx, n1.AuthKeyHash)
	if err != nil {
		t.Fatalf("Erro ao procurar nó por AuthKeyHash: %v", err)
	}
	if foundHash.ID != n1.ID {
		t.Errorf("ID inesperado para auth key hash: %s", foundHash.ID)
	}

	// 4. Update Node
	n1.Name = "node-1-updated"
	n1.StorageAvailableGB = 80
	if err := s.UpdateNode(ctx, n1); err != nil {
		t.Fatalf("Erro ao atualizar nó: %v", err)
	}

	updated, err := s.GetNodeByID(ctx, n1.ID)
	if err != nil {
		t.Fatalf("Erro ao procurar nó atualizado: %v", err)
	}
	if updated.Name != "node-1-updated" || updated.StorageAvailableGB != 80 {
		t.Errorf("Dados de nó não atualizados corretamente: %+v", updated)
	}

	// 5. Touch Node (LastSeenAt)
	if err := s.TouchNode(ctx, n1.ID); err != nil {
		t.Fatalf("Erro ao efetuar TouchNode: %v", err)
	}
	touched, err := s.GetNodeByID(ctx, n1.ID)
	if err != nil {
		t.Fatalf("Erro ao procurar nó após TouchNode: %v", err)
	}
	if touched.LastSeenAt == nil {
		t.Errorf("LastSeenAt deve ser diferente de nil após TouchNode")
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
		t.Fatalf("Erro ao criar nó 2: %v", err)
	}

	nodes, total, err := s.ListNodes(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("Erro ao listar nós: %v", err)
	}
	if total != 2 || len(nodes) != 2 {
		t.Errorf("Esperado total 2 nós, obtido total=%d, len=%d", total, len(nodes))
	}

	// 7. Soft Delete
	if err := s.DeleteNode(ctx, n1.ID); err != nil {
		t.Fatalf("Erro ao efetuar soft delete do nó 1: %v", err)
	}

	_, err = s.GetNodeByID(ctx, n1.ID)
	if err != store.ErrNodeNotFound {
		t.Errorf("Esperava ErrNodeNotFound para nó apagado, obtido: %v", err)
	}

	nodesAfterDelete, totalAfterDelete, err := s.ListNodes(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("Erro ao listar nós após delete: %v", err)
	}
	if totalAfterDelete != 1 || len(nodesAfterDelete) != 1 {
		t.Errorf("Esperado 1 nó ativo restante, obtido total=%d, len=%d", totalAfterDelete, len(nodesAfterDelete))
	}
}

func TestSQLiteStoreRelayCRUD(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_relays.db")

	s := store.NewSQLiteStore(dbPath)
	ctx := context.Background()

	if err := s.Init(ctx); err != nil {
		t.Fatalf("Erro ao inicializar SQLiteStore: %v", err)
	}
	defer s.Close()

	// Criar Nó para servir de Foreign Key
	nodeID := "goy-node-relay-1"
	n := &store.Node{
		ID:          nodeID,
		AuthKeyHash: "hash-relay-1",
		Name:        "node-relay-1",
	}
	if err := s.CreateNode(ctx, n); err != nil {
		t.Fatalf("Erro ao criar nó pai: %v", err)
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
		t.Fatalf("Erro ao inserir relay: %v", err)
	}

	// 2. Get Relay By Node ID
	found, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Erro ao procurar relay por NodeID: %v", err)
	}
	if found.URL != r1.URL || found.StorageAvailableGB != 100 || len(found.Capabilities) != 2 {
		t.Errorf("Dados de relay incorretos: %+v", found)
	}

	// 3. Upsert Relay (Update)
	r1.StorageAvailableGB = 120
	r1.Capabilities = []string{"nip09", "nip40", "backfill"}
	if err := s.UpsertRelay(ctx, r1); err != nil {
		t.Fatalf("Erro ao atualizar relay via Upsert: %v", err)
	}

	updated, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Erro ao procurar relay atualizado: %v", err)
	}
	if updated.StorageAvailableGB != 120 || len(updated.Capabilities) != 3 {
		t.Errorf("Relay não atualizado corretamente: %+v", updated)
	}

	// 4. Update Heartbeat
	newStorage := uint64(90)
	if err := s.UpdateRelayHeartbeat(ctx, nodeID, &newStorage); err != nil {
		t.Fatalf("Erro no UpdateRelayHeartbeat: %v", err)
	}

	hbFound, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Erro ao procurar relay pós heartbeat: %v", err)
	}
	if hbFound.StorageAvailableGB != 90 {
		t.Errorf("Storage em heartbeat inesperado: %d", hbFound.StorageAvailableGB)
	}

	// 5. ListActiveRelays
	relays, total, err := s.ListActiveRelays(ctx, 300, nil, 50, 10)
	if err != nil {
		t.Fatalf("Erro ao listar relays ativos: %v", err)
	}
	if total != 1 || len(relays) != 1 {
		t.Errorf("Esperava 1 relay ativo, obtido total=%d, len=%d", total, len(relays))
	}

	// 6. MarkRelayUnreachable
	if err := s.MarkRelayUnreachable(ctx, nodeID); err != nil {
		t.Fatalf("Erro ao marcar relay como unreachable: %v", err)
	}
	unrFound, err := s.GetRelayByNodeID(ctx, nodeID)
	if err != nil {
		t.Fatalf("Erro ao procurar relay unreachable: %v", err)
	}
	if unrFound.Status != store.RelayStatusUnreachable {
		t.Errorf("Status esperado unreachable, obtido: %s", unrFound.Status)
	}

	// 7. Delete Relay (Hard Delete)
	if err := s.DeleteRelay(ctx, nodeID); err != nil {
		t.Fatalf("Erro ao eliminar relay: %v", err)
	}
	_, err = s.GetRelayByNodeID(ctx, nodeID)
	if err != store.ErrRelayNotFound {
		t.Errorf("Esperava ErrRelayNotFound após eliminar relay, obtido: %v", err)
	}
}
