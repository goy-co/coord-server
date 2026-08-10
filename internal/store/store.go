package store

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotImplemented é retornado quando um método ainda não está implementado.
	ErrNotImplemented = errors.New("method not implemented")
	// ErrNodeNotFound é retornado quando um nó não é encontrado na base de dados.
	ErrNodeNotFound = errors.New("node not found")
	// ErrRelayNotFound é retornado quando um relay não é encontrado na base de dados.
	ErrRelayNotFound = errors.New("relay not found")
)

// Store define a interface de acesso à persistência de dados do coord-server.
type Store interface {
	// Init inicializa o esquema da base de dados (tabelas/índices).
	Init(ctx context.Context) error

	// HealthCheck verifica a conectividade à base de dados.
	HealthCheck(ctx context.Context) error

	// Close fecha as conexões com a base de dados.
	Close() error

	// Métodos CRUD para Node
	CreateNode(ctx context.Context, node *Node) error
	GetNodeByID(ctx context.Context, id string) (*Node, error)
	GetNodeByAuthKeyHash(ctx context.Context, hash string) (*Node, error)
	UpdateNode(ctx context.Context, node *Node) error
	DeleteNode(ctx context.Context, id string) error // Soft delete (status = "deleted")
	ListNodes(ctx context.Context, status string, limit, offset int) ([]Node, int, error)
	TouchNode(ctx context.Context, id string) error
	CleanupInactiveNodes(ctx context.Context, thresholdHours int) (int, error)
	GetNodeCountsByStatus(ctx context.Context) (map[string]int, error)

	// Métodos CRUD para Relay
	UpsertRelay(ctx context.Context, relay *Relay) error
	GetRelayByNodeID(ctx context.Context, nodeID string) (*Relay, error)
	ListActiveRelays(ctx context.Context, ttlWindowSeconds int, since *time.Time, minStorageGB uint64, limit int) ([]Relay, int, error)
	UpdateRelayHeartbeat(ctx context.Context, nodeID string, storageAvailableGB *uint64) error
	MarkRelayUnreachable(ctx context.Context, nodeID string) error
	DeleteRelay(ctx context.Context, nodeID string) error
	CountActiveRelays(ctx context.Context, ttlWindowSeconds int) (int, error)
	CleanupStaleRelays(ctx context.Context, ttlSeconds int) (markedUnreachable int, deletedExpired int, err error)
}
