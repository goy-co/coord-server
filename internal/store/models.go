package store

import "time"

// Constantes com os estados possíveis de um nó (Node status).
const (
	NodeStatusActive    = "active"
	NodeStatusInactive  = "inactive"
	NodeStatusSuspended = "suspended"
	NodeStatusDeleted   = "deleted"
)

// Constantes com os estados possíveis de um relay (Relay status).
const (
	RelayStatusActive      = "active"
	RelayStatusInactive    = "inactive"
	RelayStatusUnreachable = "unreachable"
)

// Node representa um nó registado na rede Goy mesh network.
type Node struct {
	ID                 string     `json:"id"`
	AuthKeyHash        string     `json:"auth_key_hash"`
	Name               string     `json:"name"`
	StorageReservedGB  uint64     `json:"storage_reserved_gb"`
	StorageAvailableGB uint64     `json:"storage_available_gb"`
	VPNPublicKey       string     `json:"vpn_public_key"`
	MeshURL            string     `json:"mesh_url"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

// Relay representa um nó que atua como ponto de retransmissão/storage na rede Goy mesh network.
type Relay struct {
	NodeID             string    `json:"node_id"`
	URL                string    `json:"url"`
	Fingerprint        string    `json:"fingerprint"`
	StorageReservedGB  uint64    `json:"storage_reserved_gb"`
	StorageAvailableGB uint64    `json:"storage_available_gb"`
	ReplicationFactor  uint32    `json:"replication_factor"`
	Version            string    `json:"version"`
	Capabilities       []string  `json:"capabilities"`
	Status             string    `json:"status"`
	LastSeenAt         time.Time `json:"last_seen"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
