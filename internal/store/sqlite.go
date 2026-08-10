package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore é a implementação da interface Store utilizando SQLite via modernc.org/sqlite.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteStore instancia um novo SQLiteStore com o caminho especificado.
func NewSQLiteStore(dbPath string) *SQLiteStore {
	return &SQLiteStore{
		dbPath: dbPath,
	}
}

// Init abre a conexão, ativa o WAL mode e executa as DDLs iniciais de migração.
func (s *SQLiteStore) Init(ctx context.Context) error {
	dir := filepath.Dir(s.dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("falha ao criar diretoria da base de dados %s: %w", dir, err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", s.dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("falha ao abrir base de dados sqlite (%s): %w", s.dbPath, err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)

	s.db = db

	if err := s.HealthCheck(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("verificação de conectividade falhou na inicialização: %w", err)
	}

	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("falha ao executar migração da base de dados: %w", err)
	}

	return nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		auth_key_hash TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL DEFAULT '',
		storage_reserved_gb INTEGER NOT NULL DEFAULT 0,
		storage_available_gb INTEGER NOT NULL DEFAULT 0,
		vpn_public_key TEXT NOT NULL DEFAULT '',
		mesh_url TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		last_seen_at DATETIME,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS relays (
		node_id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		storage_reserved_gb INTEGER NOT NULL DEFAULT 0,
		storage_available_gb INTEGER NOT NULL DEFAULT 0,
		replication_factor INTEGER NOT NULL DEFAULT 1,
		version TEXT NOT NULL DEFAULT '',
		capabilities TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL DEFAULT 'active',
		last_seen DATETIME NOT NULL DEFAULT (datetime('now')),
		last_seen_at DATETIME NOT NULL DEFAULT (datetime('now')),
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
		FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
	);
	`

	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("erro ao criar esquema de tabelas: %w", err)
	}

	// Adicionar colunas se as tabelas tiverem sido criadas numa versão prévia (Step 1)
	alterNodeQueries := []string{
		"ALTER TABLE nodes ADD COLUMN name TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE nodes ADD COLUMN storage_reserved_gb INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE nodes ADD COLUMN storage_available_gb INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE nodes ADD COLUMN vpn_public_key TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE nodes ADD COLUMN mesh_url TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE nodes ADD COLUMN status TEXT NOT NULL DEFAULT 'active'",
		"ALTER TABLE nodes ADD COLUMN last_seen_at DATETIME",
		"ALTER TABLE nodes ADD COLUMN deleted_at DATETIME",
	}
	for _, query := range alterNodeQueries {
		_, _ = s.db.ExecContext(ctx, query)
	}

	alterRelayQueries := []string{
		"ALTER TABLE relays ADD COLUMN replication_factor INTEGER NOT NULL DEFAULT 1",
		"ALTER TABLE relays ADD COLUMN version TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE relays ADD COLUMN capabilities TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE relays ADD COLUMN status TEXT NOT NULL DEFAULT 'active'",
		"ALTER TABLE relays ADD COLUMN last_seen DATETIME NOT NULL DEFAULT (datetime('now'))",
		"ALTER TABLE relays ADD COLUMN last_seen_at DATETIME NOT NULL DEFAULT (datetime('now'))",
		"ALTER TABLE relays ADD COLUMN created_at DATETIME NOT NULL DEFAULT (datetime('now'))",
		"ALTER TABLE relays ADD COLUMN updated_at DATETIME NOT NULL DEFAULT (datetime('now'))",
	}
	for _, query := range alterRelayQueries {
		_, _ = s.db.ExecContext(ctx, query)
	}

	indexes := `
	CREATE INDEX IF NOT EXISTS idx_nodes_auth_key_hash ON nodes(auth_key_hash);
	CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
	CREATE INDEX IF NOT EXISTS idx_relays_status ON relays(status);
	CREATE INDEX IF NOT EXISTS idx_relays_last_seen_at ON relays(last_seen_at);
	`
	_, err = s.db.ExecContext(ctx, indexes)
	if err != nil {
		return fmt.Errorf("erro ao criar índices: %w", err)
	}

	return nil
}

// HealthCheck executa um teste simples na conexão SQLite.
func (s *SQLiteStore) HealthCheck(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("base de dados não inicializada")
	}

	var ping int
	err := s.db.QueryRowContext(ctx, "SELECT 1").Scan(&ping)
	if err != nil {
		return fmt.Errorf("erro ao pingar base de dados SQLite: %w", err)
	}

	return nil
}

// Close encerra a conexão com o ficheiro SQLite.
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// --- Node Store Operations ---

func (s *SQLiteStore) CreateNode(ctx context.Context, node *Node) error {
	now := time.Now().UTC()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = now
	}
	if node.Status == "" {
		node.Status = NodeStatusActive
	}

	query := `
	INSERT INTO nodes (
		id, auth_key_hash, name, storage_reserved_gb, storage_available_gb,
		vpn_public_key, mesh_url, status, created_at, updated_at, last_seen_at, deleted_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := s.db.ExecContext(ctx, query,
		node.ID,
		node.AuthKeyHash,
		node.Name,
		node.StorageReservedGB,
		node.StorageAvailableGB,
		node.VPNPublicKey,
		node.MeshURL,
		node.Status,
		node.CreatedAt,
		node.UpdatedAt,
		node.LastSeenAt,
		node.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("erro ao criar nó (%s): %w", node.ID, err)
	}

	return nil
}

func scanNode(scanner interface{ Scan(dest ...any) error }) (*Node, error) {
	var n Node
	var lastSeenNull sql.NullTime
	var deletedNull sql.NullTime

	err := scanner.Scan(
		&n.ID,
		&n.AuthKeyHash,
		&n.Name,
		&n.StorageReservedGB,
		&n.StorageAvailableGB,
		&n.VPNPublicKey,
		&n.MeshURL,
		&n.Status,
		&n.CreatedAt,
		&n.UpdatedAt,
		&lastSeenNull,
		&deletedNull,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}

	if lastSeenNull.Valid {
		t := lastSeenNull.Time.UTC()
		n.LastSeenAt = &t
	}
	if deletedNull.Valid {
		t := deletedNull.Time.UTC()
		n.DeletedAt = &t
	}

	n.CreatedAt = n.CreatedAt.UTC()
	n.UpdatedAt = n.UpdatedAt.UTC()

	return &n, nil
}

func (s *SQLiteStore) GetNodeByID(ctx context.Context, id string) (*Node, error) {
	query := `
	SELECT id, auth_key_hash, name, storage_reserved_gb, storage_available_gb,
	       vpn_public_key, mesh_url, status, created_at, updated_at, last_seen_at, deleted_at
	FROM nodes
	WHERE id = ? AND status != 'deleted' AND deleted_at IS NULL;
	`

	row := s.db.QueryRowContext(ctx, query, id)
	node, err := scanNode(row)
	if err != nil {
		if errors.Is(err, ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("erro ao procurar nó por ID (%s): %w", id, err)
	}

	return node, nil
}

func (s *SQLiteStore) GetNodeByAuthKeyHash(ctx context.Context, hash string) (*Node, error) {
	query := `
	SELECT id, auth_key_hash, name, storage_reserved_gb, storage_available_gb,
	       vpn_public_key, mesh_url, status, created_at, updated_at, last_seen_at, deleted_at
	FROM nodes
	WHERE auth_key_hash = ? AND status != 'deleted' AND deleted_at IS NULL;
	`

	row := s.db.QueryRowContext(ctx, query, hash)
	node, err := scanNode(row)
	if err != nil {
		if errors.Is(err, ErrNodeNotFound) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("erro ao procurar nó por auth key hash: %w", err)
	}

	return node, nil
}

func (s *SQLiteStore) UpdateNode(ctx context.Context, node *Node) error {
	node.UpdatedAt = time.Now().UTC()

	query := `
	UPDATE nodes
	SET name = ?, storage_reserved_gb = ?, storage_available_gb = ?,
	    vpn_public_key = ?, mesh_url = ?, status = ?, updated_at = ?
	WHERE id = ? AND status != 'deleted' AND deleted_at IS NULL;
	`

	res, err := s.db.ExecContext(ctx, query,
		node.Name,
		node.StorageReservedGB,
		node.StorageAvailableGB,
		node.VPNPublicKey,
		node.MeshURL,
		node.Status,
		node.UpdatedAt,
		node.ID,
	)
	if err != nil {
		return fmt.Errorf("erro ao atualizar nó (%s): %w", node.ID, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

func (s *SQLiteStore) DeleteNode(ctx context.Context, id string) error {
	now := time.Now().UTC()

	query := `
	UPDATE nodes
	SET status = 'deleted', deleted_at = ?, updated_at = ?
	WHERE id = ? AND status != 'deleted' AND deleted_at IS NULL;
	`

	res, err := s.db.ExecContext(ctx, query, now, now, id)
	if err != nil {
		return fmt.Errorf("erro ao efetuar soft delete do nó (%s): %w", id, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

func (s *SQLiteStore) ListNodes(ctx context.Context, status string, limit, offset int) ([]Node, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var countQuery string
	var selectQuery string
	var args []any
	var countArgs []any

	if status != "" {
		countQuery = "SELECT COUNT(*) FROM nodes WHERE status = ?;"
		countArgs = append(countArgs, status)

		selectQuery = `
		SELECT id, auth_key_hash, name, storage_reserved_gb, storage_available_gb,
		       vpn_public_key, mesh_url, status, created_at, updated_at, last_seen_at, deleted_at
		FROM nodes
		WHERE status = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?;
		`
		args = append(args, status, limit, offset)
	} else {
		countQuery = "SELECT COUNT(*) FROM nodes WHERE status != 'deleted' AND deleted_at IS NULL;"

		selectQuery = `
		SELECT id, auth_key_hash, name, storage_reserved_gb, storage_available_gb,
		       vpn_public_key, mesh_url, status, created_at, updated_at, last_seen_at, deleted_at
		FROM nodes
		WHERE status != 'deleted' AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?;
		`
		args = append(args, limit, offset)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("erro ao contar nós: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao listar nós: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("erro ao ler registo de nó: %w", err)
		}
		nodes = append(nodes, *node)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return nodes, total, nil
}

func (s *SQLiteStore) TouchNode(ctx context.Context, id string) error {
	now := time.Now().UTC()

	query := `
	UPDATE nodes
	SET last_seen_at = ?, updated_at = ?
	WHERE id = ? AND status != 'deleted' AND deleted_at IS NULL;
	`

	res, err := s.db.ExecContext(ctx, query, now, now, id)
	if err != nil {
		return fmt.Errorf("erro ao atualizar last_seen_at do nó (%s): %w", id, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

func (s *SQLiteStore) CleanupInactiveNodes(ctx context.Context, thresholdHours int) (int, error) {
	if thresholdHours <= 0 {
		thresholdHours = 24
	}

	cutoff := time.Now().UTC().Add(-time.Duration(thresholdHours) * time.Hour)
	now := time.Now().UTC()

	query := `
	UPDATE nodes
	SET status = 'inactive', updated_at = ?
	WHERE status = 'active' AND (last_seen_at IS NULL OR last_seen_at < ?);
	`

	res, err := s.db.ExecContext(ctx, query, now, cutoff)
	if err != nil {
		return 0, fmt.Errorf("erro ao marcar nós inativos: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(affected), nil
}

func (s *SQLiteStore) GetNodeCountsByStatus(ctx context.Context) (map[string]int, error) {
	query := "SELECT status, COUNT(*) FROM nodes GROUP BY status;"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter contagem de nós por estado: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}

	return counts, nil
}

// --- Relay Store Operations ---

func scanRelay(scanner interface{ Scan(dest ...any) error }) (*Relay, error) {
	var r Relay
	var capsJSON string

	err := scanner.Scan(
		&r.NodeID,
		&r.URL,
		&r.Fingerprint,
		&r.StorageReservedGB,
		&r.StorageAvailableGB,
		&r.ReplicationFactor,
		&r.Version,
		&capsJSON,
		&r.Status,
		&r.LastSeenAt,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRelayNotFound
		}
		return nil, err
	}

	if capsJSON != "" {
		_ = json.Unmarshal([]byte(capsJSON), &r.Capabilities)
	}
	if r.Capabilities == nil {
		r.Capabilities = []string{}
	}

	r.LastSeenAt = r.LastSeenAt.UTC()
	r.CreatedAt = r.CreatedAt.UTC()
	r.UpdatedAt = r.UpdatedAt.UTC()

	return &r, nil
}

func (s *SQLiteStore) UpsertRelay(ctx context.Context, relay *Relay) error {
	now := time.Now().UTC()
	if relay.LastSeenAt.IsZero() {
		relay.LastSeenAt = now
	}
	if relay.CreatedAt.IsZero() {
		relay.CreatedAt = now
	}
	relay.UpdatedAt = now

	if relay.Status == "" {
		relay.Status = RelayStatusActive
	}
	if relay.ReplicationFactor == 0 {
		relay.ReplicationFactor = 1
	}

	capsBytes, err := json.Marshal(relay.Capabilities)
	if err != nil {
		capsBytes = []byte("[]")
	}

	query := `
	INSERT INTO relays (
		node_id, url, fingerprint, storage_reserved_gb, storage_available_gb,
		replication_factor, version, capabilities, status, last_seen, last_seen_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_id) DO UPDATE SET
		url = excluded.url,
		fingerprint = excluded.fingerprint,
		storage_reserved_gb = excluded.storage_reserved_gb,
		storage_available_gb = excluded.storage_available_gb,
		replication_factor = excluded.replication_factor,
		version = excluded.version,
		capabilities = excluded.capabilities,
		status = excluded.status,
		last_seen = excluded.last_seen,
		last_seen_at = excluded.last_seen_at,
		updated_at = excluded.updated_at;
	`

	_, err = s.db.ExecContext(ctx, query,
		relay.NodeID,
		relay.URL,
		relay.Fingerprint,
		relay.StorageReservedGB,
		relay.StorageAvailableGB,
		relay.ReplicationFactor,
		relay.Version,
		string(capsBytes),
		relay.Status,
		relay.LastSeenAt,
		relay.LastSeenAt,
		relay.CreatedAt,
		relay.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("erro no UpsertRelay para node_id (%s): %w", relay.NodeID, err)
	}

	return nil
}

func (s *SQLiteStore) GetRelayByNodeID(ctx context.Context, nodeID string) (*Relay, error) {
	query := `
	SELECT node_id, url, fingerprint, storage_reserved_gb, storage_available_gb,
	       replication_factor, version, capabilities, status, last_seen_at, created_at, updated_at
	FROM relays
	WHERE node_id = ?;
	`

	row := s.db.QueryRowContext(ctx, query, nodeID)
	relay, err := scanRelay(row)
	if err != nil {
		if errors.Is(err, ErrRelayNotFound) {
			return nil, ErrRelayNotFound
		}
		return nil, fmt.Errorf("erro ao procurar relay por node_id (%s): %w", nodeID, err)
	}

	return relay, nil
}

func (s *SQLiteStore) ListActiveRelays(ctx context.Context, ttlWindowSeconds int, since *time.Time, minStorageGB uint64, limit int) ([]Relay, int, error) {
	if ttlWindowSeconds <= 0 {
		ttlWindowSeconds = 300
	}
	if limit <= 0 {
		limit = 100
	}

	cutoffTime := time.Now().UTC().Add(-time.Duration(ttlWindowSeconds) * time.Second)

	whereClause := "WHERE status = 'active' AND last_seen_at >= ?"
	args := []any{cutoffTime}

	if since != nil && !since.IsZero() {
		whereClause += " AND last_seen_at >= ?"
		args = append(args, since.UTC())
	}
	if minStorageGB > 0 {
		whereClause += " AND storage_available_gb >= ?"
		args = append(args, minStorageGB)
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM relays %s;", whereClause)
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("erro ao contar relays ativos: %w", err)
	}

	selectQuery := fmt.Sprintf(`
	SELECT node_id, url, fingerprint, storage_reserved_gb, storage_available_gb,
	       replication_factor, version, capabilities, status, last_seen_at, created_at, updated_at
	FROM relays
	%s
	ORDER BY last_seen_at DESC
	LIMIT ?;
	`, whereClause)

	selectArgs := append(args, limit)
	rows, err := s.db.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao listar relays ativos: %w", err)
	}
	defer rows.Close()

	var relays []Relay
	for rows.Next() {
		r, err := scanRelay(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("erro ao ler registo de relay: %w", err)
		}
		relays = append(relays, *r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return relays, total, nil
}

func (s *SQLiteStore) UpdateRelayHeartbeat(ctx context.Context, nodeID string, storageAvailableGB *uint64) error {
	now := time.Now().UTC()

	var query string
	var args []any

	if storageAvailableGB != nil {
		query = `
		UPDATE relays
		SET last_seen = ?, last_seen_at = ?, status = 'active', storage_available_gb = ?, updated_at = ?
		WHERE node_id = ?;
		`
		args = []any{now, now, *storageAvailableGB, now, nodeID}
	} else {
		query = `
		UPDATE relays
		SET last_seen = ?, last_seen_at = ?, status = 'active', updated_at = ?
		WHERE node_id = ?;
		`
		args = []any{now, now, now, nodeID}
	}

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("erro ao atualizar heartbeat do relay (%s): %w", nodeID, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRelayNotFound
	}

	return nil
}

func (s *SQLiteStore) MarkRelayUnreachable(ctx context.Context, nodeID string) error {
	now := time.Now().UTC()

	query := `
	UPDATE relays
	SET status = 'unreachable', updated_at = ?
	WHERE node_id = ?;
	`

	res, err := s.db.ExecContext(ctx, query, now, nodeID)
	if err != nil {
		return fmt.Errorf("erro ao marcar relay como unreachable (%s): %w", nodeID, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRelayNotFound
	}

	return nil
}

func (s *SQLiteStore) DeleteRelay(ctx context.Context, nodeID string) error {
	query := "DELETE FROM relays WHERE node_id = ?;"

	res, err := s.db.ExecContext(ctx, query, nodeID)
	if err != nil {
		return fmt.Errorf("erro ao eliminar relay (%s): %w", nodeID, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRelayNotFound
	}

	return nil
}

func (s *SQLiteStore) CountActiveRelays(ctx context.Context, ttlWindowSeconds int) (int, error) {
	if ttlWindowSeconds <= 0 {
		ttlWindowSeconds = 300
	}

	cutoffTime := time.Now().UTC().Add(-time.Duration(ttlWindowSeconds) * time.Second)
	query := "SELECT COUNT(*) FROM relays WHERE status = 'active' AND last_seen_at >= ?;"

	var count int
	if err := s.db.QueryRowContext(ctx, query, cutoffTime).Scan(&count); err != nil {
		return 0, fmt.Errorf("erro ao contar relays ativos: %w", err)
	}

	return count, nil
}

func (s *SQLiteStore) CleanupStaleRelays(ctx context.Context, ttlSeconds int) (int, int, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}

	now := time.Now().UTC()
	unreachableCutoff := now.Add(-time.Duration(ttlSeconds) * time.Second)
	deleteCutoff := now.Add(-2 * time.Duration(ttlSeconds) * time.Second)

	// 1. Marcar como unreachable relays em estado 'active' mas com last_seen_at < unreachableCutoff
	unreachableQuery := `
	UPDATE relays
	SET status = 'unreachable', updated_at = ?
	WHERE status = 'active' AND last_seen_at < ?;
	`
	res1, err := s.db.ExecContext(ctx, unreachableQuery, now, unreachableCutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("erro ao marcar relays como unreachable: %w", err)
	}
	markedUnreachable, _ := res1.RowsAffected()

	// 2. Eliminar (hard delete) relays que estejam unreachable há mais de 2x TTL
	deleteQuery := `
	DELETE FROM relays
	WHERE status = 'unreachable' AND last_seen_at < ?;
	`
	res2, err := s.db.ExecContext(ctx, deleteQuery, deleteCutoff)
	if err != nil {
		return int(markedUnreachable), 0, fmt.Errorf("erro ao eliminar relays expirados: %w", err)
	}
	deletedExpired, _ := res2.RowsAffected()

	return int(markedUnreachable), int(deletedExpired), nil
}
