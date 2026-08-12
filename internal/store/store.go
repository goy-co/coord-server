package store

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotImplemented is returned when a method is not yet implemented.
	ErrNotImplemented = errors.New("method not implemented")
	// ErrNodeNotFound is returned when a node is not found in the database.
	ErrNodeNotFound = errors.New("node not found")
	// ErrRelayNotFound is returned when a relay is not found in the database.
	ErrRelayNotFound = errors.New("relay not found")
)

// Store defines the data persistence interface for coord-server.
type Store interface {
	// Init initializes the database schema (tables/indexes).
	Init(ctx context.Context) error

	// HealthCheck checks database connectivity.
	HealthCheck(ctx context.Context) error

	// Close closes database connections.
	Close() error

	// Node CRUD methods
	CreateNode(ctx context.Context, node *Node) error
	GetNodeByID(ctx context.Context, id string) (*Node, error)
	GetNodeByAuthKeyHash(ctx context.Context, hash string) (*Node, error)
	UpdateNode(ctx context.Context, node *Node) error
	DeleteNode(ctx context.Context, id string) error // Soft delete (status = "deleted")
	ListNodes(ctx context.Context, status string, limit, offset int) ([]Node, int, error)
	TouchNode(ctx context.Context, id string) error
	CleanupInactiveNodes(ctx context.Context, thresholdHours int) (int, error)
	GetNodeCountsByStatus(ctx context.Context) (map[string]int, error)

	// Relay CRUD methods
	UpsertRelay(ctx context.Context, relay *Relay) error
	GetRelayByNodeID(ctx context.Context, nodeID string) (*Relay, error)
	ListActiveRelays(ctx context.Context, ttlWindowSeconds int, since *time.Time, minStorageGB uint64, limit int) ([]Relay, int, error)
	UpdateRelayHeartbeat(ctx context.Context, nodeID string, storageAvailableGB *uint64) error
	UpdateRelayFull(ctx context.Context, relay *Relay) error
	MarkRelayUnreachable(ctx context.Context, nodeID string) error
	DeleteRelay(ctx context.Context, nodeID string) error
	CountActiveRelays(ctx context.Context, ttlWindowSeconds int) (int, error)
	CleanupStaleRelays(ctx context.Context, ttlSeconds int) (markedUnreachable int, deletedExpired int, err error)
}
