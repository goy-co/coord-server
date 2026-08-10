package store

import "time"

// Constants defining node status values.
const (
	NodeStatusActive    = "active"
	NodeStatusInactive  = "inactive"
	NodeStatusSuspended = "suspended"
	NodeStatusDeleted   = "deleted"
)

// Constants defining relay status values.
const (
	RelayStatusActive      = "active"
	RelayStatusInactive    = "inactive"
	RelayStatusUnreachable = "unreachable"
)

// Node represents a registered node in the Goy mesh network.
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

// Relay represents a node acting as a relay/storage peer in the Goy mesh network.
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
