# API Documentation — `coord-server` v0.1.0

## Overview

Base URL: `http://<host>:8080` (configurable via `server.listen`)

All responses are `application/json`. Timestamps are RFC3339 UTC.

---

## Authentication

When `auth.require_auth = true` (default), protected endpoints require:

```http
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

The API key is configured exclusively via the `COORD_ADMIN_API_KEY` environment variable (never in `config.toml`).

**Public endpoints** (no auth required): `GET /health`, `GET /info`, `GET /metrics`

### Auth Error Responses

**`401 Unauthorized`** — missing or invalid token:
```json
{
  "error": "unauthorized",
  "details": "header Authorization em falta"
}
```

**`429 Too Many Requests`** — IP rate limit exceeded (header `Retry-After: N` included):
```json
{
  "error": "rate limit exceeded",
  "reason": "limite de pedidos HTTP excedido"
}
```

---

## Public Endpoints

### `GET /health`

Server health check and database connectivity. Exempt from auth and rate limiting. Suitable for load balancer and orchestrator probes.

**Response `200 OK`:**
```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

**Response `503 Service Unavailable`** — database unreachable:
```json
{
  "status": "degraded",
  "version": "0.1.0",
  "error": "database unreachable"
}
```

---

### `GET /info`

Runtime metadata. Exempt from auth and rate limiting.

**Response `200 OK`:**
```json
{
  "version": "0.1.0",
  "uptime_seconds": 3721,
  "database_path": "data/coord-server.db",
  "listen_address": "0.0.0.0:8080",
  "vpn_integration_enabled": true
}
```

---

### `GET /metrics`

Prometheus metrics in text exposition format. Exempt from auth and rate limiting.

**Response `200 OK`** (text/plain; version=0.0.4):
```
# HELP coord_http_requests_total Total de pedidos HTTP processados por método, caminho e código de estado.
# TYPE coord_http_requests_total counter
coord_http_requests_total{method="GET",path="/health",status="200"} 42
coord_http_requests_total{method="POST",path="/v1/nodes/register",status="201"} 5
...
```

Available metrics:

| Metric | Type | Labels |
|---|---|---|
| `coord_http_requests_total` | Counter | `method`, `path`, `status` |
| `coord_http_request_duration_seconds` | Histogram | `method`, `path` |
| `coord_nodes_total` | Gauge | `status` |
| `coord_relays_active_total` | Gauge | — |
| `coord_vpn_keys_generated_total` | Counter | — |
| `coord_vpn_errors_total` | Counter | — |
| `coord_auth_failures_total` | Counter | — |
| `coord_ratelimit_rejected_total` | Counter | — |
| `coord_db_query_duration_seconds` | Histogram | `operation` |

> **Note:** Paths containing node/relay IDs are normalised to `:id` to prevent cardinality explosion (e.g. `/v1/nodes/goy-node-abc123` → `/v1/nodes/:id`).

---

## Nodes API — `/v1/nodes` (Auth Required)

### `POST /v1/nodes/register`

Register a new Goy Node (onboarding). If a node with the same `auth_key` already exists, the registration is idempotent and returns `200 OK`.

**Request:**
```http
POST /v1/nodes/register
Authorization: Bearer <COORD_ADMIN_API_KEY>
Content-Type: application/json
```

```json
{
  "auth_key": "gc_12345678901234567890",
  "name": "my-goy-node",
  "storage": {
    "reserved_gb": 150,
    "available_gb": 234
  },
  "mesh_url": "100.80.1.5:8443"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `auth_key` | string | ✅ | Node auth key (format: `gc_<hex>`, min 20 chars) |
| `name` | string | No | Human-readable node name (max 64 chars) |
| `storage.reserved_gb` | uint | No | Storage reserved for Goy (GB) |
| `storage.available_gb` | uint | No | Currently available storage (GB) |
| `mesh_url` | string | No | Node's mesh endpoint (host:port) |

**Response `201 Created`** — new registration:
```json
{
  "node_id": "goy-node-63898b5aaee9bb45",
  "name": "my-goy-node",
  "mesh_url": "100.80.1.5:8443",
  "vpn_config": {
    "auth_key": "tskey-auth-1234567890abcdef",
    "control_url": "https://vpn.goyco.xyz"
  },
  "registry_url": "http://coord-server:8080",
  "created_at": "2026-08-10T15:59:01Z"
}
```

**Response `200 OK`** — idempotent re-registration (same `auth_key`):
Same body as `201` but with original `created_at`.

> **VPN Graceful Degradation:** If `vpn.enabled = false` or Headscale is unreachable, `vpn_config.auth_key` is returned as `""` and registration still succeeds. The node can be manually joined to the VPN later.

**Error Responses:**

| Code | Condition |
|---|---|
| `400 Bad Request` | Missing `auth_key`, key too short, or malformed JSON |
| `401 Unauthorized` | Missing or invalid `Authorization` header |
| `429 Too Many Requests` | IP rate limit exceeded |
| `500 Internal Server Error` | Database write failure |

---

### `GET /v1/nodes/{id}`

Retrieve full details for a registered node.

**Request:**
```http
GET /v1/nodes/goy-node-63898b5aaee9bb45
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Response `200 OK`:**
```json
{
  "id": "goy-node-63898b5aaee9bb45",
  "auth_key_hash": "e9df3b9d2dcc9fd5d2caf1012d36c28859e4c4e6a8ef709c31dd03853eb6e20f",
  "name": "my-goy-node",
  "storage_reserved_gb": 150,
  "storage_available_gb": 234,
  "vpn_public_key": "",
  "mesh_url": "100.80.1.5:8443",
  "status": "active",
  "created_at": "2026-08-10T15:59:01Z",
  "updated_at": "2026-08-10T16:02:09Z"
}
```

`status` values: `active` | `inactive` | `deleted`

**Error Responses:**

| Code | Condition |
|---|---|
| `401 Unauthorized` | Missing or invalid token |
| `404 Not Found` | Node ID does not exist |
| `429 Too Many Requests` | IP rate limit exceeded |

---

### `GET /v1/nodes/{node_id}/status`

Retrieve real-time online/offline status, last activity timestamp, software version, uptime, and storage metrics for a node.

> **Administrative Authorization:** Strictly requires `COORD_ADMIN_API_KEY`. Node auth keys (`gc_...`) are rejected with `401 Unauthorized`.

**Request:**
```http
GET /v1/nodes/goy-node-63898b5aaee9bb45/status
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Response `200 OK`:**
```json
{
  "node_id": "goy-node-63898b5aaee9bb45",
  "is_online": true,
  "last_seen": "2026-08-12T17:58:30Z",
  "url": "ws://100.80.1.5:8443",
  "version": "0.1.1",
  "uptime_secs": 3600,
  "storage": {
    "reserved_gb": 50,
    "available_gb": 200
  }
}
```

If the node has never sent a heartbeat or advertised activity, `last_seen` will be `null` and `is_online` will be `false`:
```json
{
  "node_id": "goy-node-63898b5aaee9bb45",
  "is_online": false,
  "last_seen": null,
  "url": "",
  "version": "",
  "uptime_secs": 0,
  "storage": {
    "reserved_gb": 0,
    "available_gb": 0
  }
}
```

**Online Logic (`is_online`):**
A node is considered online (`is_online = true`) if:
1. `last_seen` is non-null AND
2. `(now - last_seen) <= registry.online_threshold_seconds` (default: 180s = 3× 60s heartbeat interval).

**Error Responses:**

| Code | Condition |
|---|---|
| `401 Unauthorized` | Missing token, invalid token, or node auth key (`gc_...`) presented |
| `404 Not Found` | Node ID does not exist |
| `429 Too Many Requests` | IP rate limit exceeded |

---

### `DELETE /v1/nodes/{id}`

Soft-delete a node (sets `status = "deleted"`). The node's relay entry is also removed from the registry.

**Request:**
```http
DELETE /v1/nodes/goy-node-63898b5aaee9bb45
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Response `204 No Content`** — success, no body.

**Error Responses:**

| Code | Condition |
|---|---|
| `401 Unauthorized` | Missing or invalid token |
| `404 Not Found` | Node ID does not exist |
| `429 Too Many Requests` | IP rate limit exceeded |

---

## VPN Diagnostics API — `/v1/vpn` (Auth Required)

### `GET /v1/vpn/status`

Diagnostic endpoint for Headscale integration status.

**Request:**
```http
GET /v1/vpn/status
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Response `200 OK`** — integration enabled and reachable:
```json
{
  "vpn_enabled": true,
  "headscale_reachable": true,
  "headscale_user": "goy-nodes",
  "registered_machines": 5
}
```

**Response `200 OK`** — integration disabled:
```json
{
  "vpn_enabled": false,
  "headscale_reachable": false,
  "headscale_user": "",
  "registered_machines": 0
}
```

**Response `200 OK`** — enabled but Headscale unreachable:
```json
{
  "vpn_enabled": true,
  "headscale_reachable": false,
  "headscale_user": "goy-nodes",
  "registered_machines": 0
}
```

**Error Responses:**

| Code | Condition |
|---|---|
| `401 Unauthorized` | Missing or invalid token |
| `429 Too Many Requests` | IP rate limit exceeded |

---

## Relays API — `/relays` (Auth Required)

### `GET /relays`

Discover active relay peers. Results are cached for `registry.discovery_cache_ttl_seconds` (default 15s).

**Request:**
```http
GET /relays
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Query Parameters (optional):**

| Parameter | Type | Description |
|---|---|---|
| `since` | RFC3339 timestamp | Return only relays seen after this time (incremental sync) |
| `min_storage_gb` | uint | Filter by minimum available storage (GB) |

**Example:**
```
GET /relays?since=2026-08-10T12:00:00Z&min_storage_gb=50
```

**Response `200 OK`:**
```json
{
  "relays": [
    {
      "node_id": "goy-node-760cada1b87b1d48",
      "url": "ws://100.80.1.5:8443",
      "fingerprint": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "storage": {
        "reserved_gb": 200,
        "available_gb": 150
      },
      "replication_factor": 3,
      "version": "0.1.1-alpha",
      "capabilities": ["nip09", "nip40", "backfill"],
      "last_seen": "2026-08-10T16:02:09Z"
    }
  ],
  "total": 1,
  "generated_at": "2026-08-10T16:02:12Z"
}
```

> **Stale relay filtering:** Relays that haven't sent a heartbeat within `registry.relay_ttl_seconds` (default 300s) are excluded from results and eventually cleaned up by the background job.

**Error Responses:**

| Code | Condition |
|---|---|
| `400 Bad Request` | Malformed `since` timestamp or `min_storage_gb` value |
| `401 Unauthorized` | Missing or invalid token |
| `429 Too Many Requests` | IP rate limit exceeded |
| `500 Internal Server Error` | Database read failure |

---

### `POST /relays`

Announce this node as an active relay. Idempotent — calling again updates the existing entry.

**Request:**
```http
POST /relays
Authorization: Bearer <COORD_ADMIN_API_KEY>
Content-Type: application/json
```

```json
{
  "node_id": "goy-node-760cada1b87b1d48",
  "url": "ws://100.80.1.5:8443",
  "fingerprint": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "storage": {
    "reserved_gb": 200,
    "available_gb": 150
  },
  "replication_factor": 3,
  "version": "0.1.1-alpha",
  "capabilities": ["nip09", "nip40", "backfill"]
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `node_id` | string | ✅ | Must match an existing registered node |
| `url` | string | ✅ | WebSocket relay URL (must start with `ws://` or `wss://`) |
| `fingerprint` | string | No | TLS certificate fingerprint for mTLS TOFU |
| `storage.reserved_gb` | uint | No | Storage reserved for Goy (GB) |
| `storage.available_gb` | uint | No | Currently available storage (GB) |
| `replication_factor` | uint | No | Target replication factor for hosted objects |
| `version` | string | No | Goy Node software version |
| `capabilities` | []string | No | Supported NIPs/protocols |

**Response `201 Created`:**
```json
{
  "node_id": "goy-node-760cada1b87b1d48",
  "url": "ws://100.80.1.5:8443",
  "fingerprint": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "storage": {
    "reserved_gb": 200,
    "available_gb": 150
  },
  "replication_factor": 3,
  "version": "0.1.1-alpha",
  "capabilities": ["nip09", "nip40", "backfill"],
  "last_seen": "2026-08-10T16:02:09Z"
}
```

**Error Responses:**

| Code | Condition |
|---|---|
| `400 Bad Request` | Missing required fields or invalid URL format |
| `401 Unauthorized` | Missing or invalid token |
| `404 Not Found` | `node_id` not found in nodes registry |
| `429 Too Many Requests` | IP rate limit exceeded |
| `500 Internal Server Error` | Database write failure |

---

### `PUT /relays/{node_id}`

Relay heartbeat. Updates `last_seen` and optionally refreshes available storage. Should be called every 60–120 seconds by active nodes.

Rate limit for this endpoint is more generous (`rate_limit.heartbeat_rpm`, default 120/min).

**Request:**
```http
PUT /relays/goy-node-760cada1b87b1d48
Authorization: Bearer <COORD_ADMIN_API_KEY>
Content-Type: application/json
```

Body is optional. If provided, `storage.available_gb` updates the relay's available capacity:
```json
{
  "storage": {
    "available_gb": 145
  }
}
```

**Response `204 No Content`** — success, no body.

**Error Responses:**

| Code | Condition |
|---|---|
| `401 Unauthorized` | Missing or invalid token |
| `404 Not Found` | Relay for this `node_id` not found |
| `429 Too Many Requests` | IP rate limit exceeded |
| `500 Internal Server Error` | Database write failure |

---

### `PUT /v1/relays/{node_id}`

Full relay heartbeat update. Refreshes `last_seen_at` and updates mutable relay state (`url`, `fingerprint`, `storage`, `version`, `uptime_secs`). Should be called periodically (every 60–120 seconds) by active Goy Nodes acting as storage relays.

Authorized using either the global `COORD_ADMIN_API_KEY` or the node's `auth_key` (`gc_...`). When called with a node `auth_key`, ownership validation enforces that the token matches `{node_id}`.

**Request:**
```http
PUT /v1/relays/goy-node-760cada1b87b1d48
Authorization: Bearer <COORD_ADMIN_API_KEY | gc_auth_key>
Content-Type: application/json
```

```json
{
  "url": "ws://100.80.1.5:8443",
  "fingerprint": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "storage": {
    "reserved_gb": 50,
    "available_gb": 200
  },
  "version": "0.1.1",
  "uptime_secs": 3600
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `url` | string | ✅ | WebSocket relay URL (`ws://` or `wss://` with explicit port) |
| `fingerprint` | string | ✅ | 64-character hex SHA-256 TLS fingerprint |
| `storage.reserved_gb` | uint64 | ✅ | Storage reserved for Goy network (GB) |
| `storage.available_gb` | uint64 | ✅ | Currently available storage capacity (GB) |
| `version` | string | ✅ | Goy Node software version (non-empty) |
| `uptime_secs` | uint64 | No | Uptime of the relay process in seconds |

**Response `200 OK`:**
```json
{
  "node_id": "goy-node-760cada1b87b1d48",
  "url": "ws://100.80.1.5:8443",
  "fingerprint": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "storage": {
    "reserved_gb": 50,
    "available_gb": 200
  },
  "replication_factor": 1,
  "version": "0.1.1",
  "uptime_secs": 3600,
  "capabilities": ["nip09", "nip40"],
  "last_seen": "2026-08-12T17:30:00Z"
}
```

**Error Responses:**

| Code | Condition |
|---|---|
| `400 Bad Request` | Invalid URL, invalid fingerprint format, missing storage, or empty version |
| `401 Unauthorized` | Missing/invalid token or token ownership mismatch for `node_id` |
| `404 Not Found` | Relay entry for this `node_id` does not exist |
| `429 Too Many Requests` | IP rate limit exceeded |
| `500 Internal Server Error` | Database write failure |

---

### `DELETE /relays/{node_id}`

Deregister a relay (hard delete). The node registration is preserved; only the relay entry is removed.

**Request:**
```http
DELETE /relays/goy-node-760cada1b87b1d48
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Response `204 No Content`** — success, no body.

**Error Responses:**

| Code | Condition |
|---|---|
| `401 Unauthorized` | Missing or invalid token |
| `404 Not Found` | Relay for this `node_id` not found |
| `429 Too Many Requests` | IP rate limit exceeded |

---

## Common Error Format

All error responses use this envelope:

```json
{
  "error": "machine-readable-key",
  "details": "human-readable explanation"
}
```

Or for validation errors:
```json
{
  "error": "invalid_request",
  "details": "auth_key: must be at least 20 characters"
}
```
