# Architecture — `coord-server`

## Overview

`coord-server` is a single-binary Go service that acts as the coordination backbone for the Goy mesh network. It manages node identity, relay discovery, and VPN provisioning. It is intentionally simple: no cluster, no consensus, no external message queue — just a Go HTTP server backed by a SQLite database.

---

## Component Diagram

```
                        ┌─────────────────────────────────────────────────┐
                        │                  coord-server                   │
                        │                                                 │
  Goy Node ──HTTP──▶   │  ┌──────────┐   ┌──────────┐   ┌───────────┐  │
  (onboard/register)    │  │  router  │──▶│ handlers │──▶│   store   │  │
                        │  │  (Chi)   │   │  (api/)  │   │ (SQLite)  │  │
  Goy Node ──HTTP──▶   │  └──────────┘   └────┬─────┘   └───────────┘  │
  (relay heartbeat)     │       ▲              │                          │
                        │       │         ┌────▼─────┐                   │
  Prometheus ──HTTP──▶ │  middleware      │   vpn    │──HTTP──▶ Headscale│
  (scrape /metrics)     │  ┌──────────┐   │ (client) │                   │
                        │  │  auth    │   └──────────┘                   │
                        │  │ratelimit │                                   │
                        │  │ logging  │   ┌──────────┐                   │
                        │  │ metrics  │   │  jobs    │ (background)       │
                        │  └──────────┘   │ (runner) │──▶ store.Cleanup  │
                        │                 └──────────┘                   │
                        └─────────────────────────────────────────────────┘
```

---

## Internal Packages

| Package | Path | Responsibility |
|---|---|---|
| `api` | `internal/api/` | HTTP handlers, router setup, request/response types |
| `config` | `internal/config/` | TOML + env var configuration loading and validation |
| `store` | `internal/store/` | `Store` interface + `SQLiteStore` implementation (WAL mode) |
| `vpn` | `internal/vpn/` | `VPNProvider` interface, `HeadscaleClient`, `NoopVPNProvider` |
| `middleware` | `internal/middleware/` | Auth, rate limiting, request logging, Prometheus metrics |
| `metrics` | `internal/metrics/` | Prometheus metric definitions (counters, histograms, gauges) |
| `jobs` | `internal/jobs/` | Background `Runner`: relay cleanup, node deactivation, gauge refresh |

---

## External Dependencies

| Dependency | Role |
|---|---|
| **SQLite** (via `mattn/go-sqlite3`) | Persistent storage for nodes and relays. WAL mode for concurrent reads. |
| **Headscale** (optional) | Tailscale-compatible control plane for WireGuard VPN. Used only during node registration to generate pre-auth keys. |
| **Chi** (`go-chi/chi`) | HTTP router. Used for URL parameter extraction (`{id}`, `{node_id}`) and middleware composition. |
| **Prometheus** (`prometheus/client_golang`) | Metrics exposition. All metrics defined in `internal/metrics/`. |
| **go-toml** (`pelletier/go-toml/v2`) | TOML config file parsing. |

---

## Data Model

```
┌──────────────────────────────────┐
│             Node                 │
│──────────────────────────────────│
│ id            TEXT  PK           │  ← "goy-node-{8-byte hex}"
│ auth_key_hash TEXT  NOT NULL     │  ← HMAC-SHA256 of original key
│ name          TEXT               │
│ storage_reserved_gb  INTEGER     │
│ storage_available_gb INTEGER     │
│ vpn_public_key TEXT              │
│ mesh_url      TEXT               │
│ status        TEXT  DEFAULT 'active' │  ← active | inactive | deleted
│ created_at    DATETIME           │
│ updated_at    DATETIME           │
│ last_seen_at  DATETIME           │
└──────────────────────┬───────────┘
                       │ 1
                       │
                       │ 0..1
┌──────────────────────▼───────────┐
│             Relay                │
│──────────────────────────────────│
│ node_id       TEXT  PK, FK→Node  │  ← one relay per node
│ url           TEXT  NOT NULL     │  ← "ws://host:port"
│ fingerprint   TEXT               │  ← TLS cert fingerprint
│ storage_reserved_gb  INTEGER     │
│ storage_available_gb INTEGER     │
│ replication_factor   INTEGER     │
│ version       TEXT               │
│ capabilities  TEXT               │  ← JSON array stored as TEXT
│ status        TEXT               │  ← active | unreachable
│ last_seen_at  DATETIME           │
│ created_at    DATETIME           │
└──────────────────────────────────┘
```

**Key design decisions:**
- One `Relay` per `Node` (1:0..1). A node that hasn't announced itself as a relay yet has no `Relay` row.
- `auth_key_hash` stores HMAC-SHA256 of the original auth key. The plaintext key is never persisted.
- `Relay.status` is set to `unreachable` by the background job when `last_seen_at` exceeds the TTL, and permanently deleted after a longer grace period.

---

## Request Lifecycle

### Node Onboarding (`POST /v1/nodes/register`)

```
Client
  │
  ▼
RateLimitMiddleware ── IP bucket OK? ──No──▶ 429
  │ Yes
  ▼
AuthMiddleware ── Bearer token valid? ──No──▶ 401
  │ Yes
  ▼
RegisterNodeHandler
  ├── Validate request (auth_key format, length)
  ├── Hash auth_key (HMAC-SHA256 with COORD_AUTH_SECRET)
  ├── store.GetNodeByAuthKeyHash() ── exists? ──Yes──▶ 200 (idempotent)
  │                                               │
  │   No                                          │
  ├── store.CreateNode()                          │
  ├── vpn.CreatePreAuthKey() ── Headscale OK? ──No──▶ vpn_config.auth_key = ""
  │                              │ Yes
  │                              └──▶ vpn_config.auth_key = "tskey-auth-..."
  └──▶ 201 Created
```

### Relay Discovery (`GET /relays`)

```
Client
  │
  ▼
RateLimitMiddleware ── OK? ──No──▶ 429
  │ Yes
  ▼
AuthMiddleware ── valid? ──No──▶ 401
  │ Yes
  ▼
GetRelaysHandler
  ├── Parse ?since= and ?min_storage_gb= query params
  ├── relayCache.Get() ── cache hit? ──Yes──▶ 200 (cached response)
  │                        │ No
  ├── store.ListActiveRelays(ttl, since, minStorage, limit)
  ├── relayCache.Set(result)
  └──▶ 200 OK
```

### Relay Heartbeat (`PUT /relays/{node_id}`)

```
Client
  │
  ▼
RateLimitMiddleware (heartbeat_rpm = 120/min per IP)
  │
  ▼
AuthMiddleware
  │
  ▼
HeartbeatRelayHandler
  ├── Parse optional body (storage.available_gb)
  ├── store.UpdateRelayHeartbeat(node_id, storageAvailableGB)
  ├── relayCache.Invalidate()
  └──▶ 204 No Content
```

---

## Background Jobs

The `jobs.Runner` runs three periodic goroutines started at server boot:

| Job | Default Interval | Action |
|---|---|---|
| **Relay Cleanup** | 60s | `CleanupStaleRelays`: marks relays with expired `last_seen_at` as `unreachable`; deletes those past a hard expiry |
| **Node Deactivation** | 300s | `CleanupInactiveNodes`: sets `status = inactive` for nodes with no contact for > `node_inactive_threshold_hours` (default 24h) |
| **Gauge Refresh** | 30s | Reads `GetNodeCountsByStatus` and `CountActiveRelays` and pushes values to Prometheus gauges |

On graceful shutdown (`SIGTERM`/`SIGINT`), `Runner.Stop()` is called before the HTTP server drain.

---

## Key Design Decisions

### SQLite over PostgreSQL

The coord-server is designed to run as a single instance alongside a Headscale control plane. SQLite with WAL mode provides sufficient read concurrency for the expected load (hundreds of nodes, not millions) while eliminating an external database dependency. If horizontal scaling becomes necessary, a migration to PostgreSQL is straightforward via the `store.Store` interface.

### VPNProvider Interface

The `vpn.VPNProvider` interface (`CreatePreAuthKey`, `GetStatus`) decouples the onboarding handler from the Headscale implementation. In tests, `MockVPNProvider` is used. When VPN is disabled, `NoopVPNProvider` returns empty keys. This ensures the server always starts and handles registrations, even if the VPN control plane is unavailable.

### In-Memory Rate Limiting

The rate limiter (`middleware.IPRateLimiter`) is in-process. For a single-instance deployment this is correct and zero-dependency. If `coord-server` is ever scaled horizontally, replace it with a Redis-backed limiter — the `RateLimitMiddleware` signature doesn't need to change.

### No Node-Level Authentication

Nodes currently authenticate with the same admin API key as operators. Future work could derive per-node credentials from the `auth_key` using HMAC to restrict each node to only its own relay slot. The `COORD_AUTH_SECRET` env var is already provisioned for this.

---

## Deployment Topology

```
          Internet
              │
         [Firewall]
              │
    ┌─────────▼────────────┐
    │    coord-server VM   │
    │  :8080  coord-server │
    │  :8443  headscale    │◀── tailscale/WireGuard mesh
    │  :5432  (optional)   │
    └──────────────────────┘
              │  VPN (WireGuard)
    ┌─────────▼────┐  ┌──────────────┐
    │  Goy Node A  │  │  Goy Node B  │  ...
    │  :8443 relay │  │  :8443 relay │
    └──────────────┘  └──────────────┘
```

Goy Nodes connect to `coord-server` over HTTPS for onboarding and relay registration. After joining the VPN, peer-to-peer WebSocket connections go directly between nodes without passing through `coord-server`.
