# coord-server

> Coordination server and registry for the Goy mesh network.

[![CI](https://github.com/goy-co/coord-server/actions/workflows/ci.yml/badge.svg)](https://github.com/goy-co/coord-server/actions/workflows/ci.yml)
[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev)
[![License: Goy Source Available](https://img.shields.io/badge/License-Goy_Source_Available-blue)](LICENSE)

---

## What It Does

`coord-server` is the coordination backbone of the Goy mesh network. A single binary that handles:

- **Node onboarding** — registers Goy Nodes, hashes auth keys, provisions VPN credentials
- **Relay registry** — nodes announce themselves and discover peers (`/relays`)
- **VPN integration** — generates Headscale pre-auth keys automatically on registration
- **Auth** — Bearer token authentication with timing-safe comparison
- **Rate limiting** — per-IP token bucket with `Retry-After` header
- **Observability** — Prometheus metrics endpoint (`/metrics`) + structured JSON logs
- **Background maintenance** — stale relay cleanup, inactive node deactivation, gauge refresh

## Quick Start

```bash
# 1. Build
make build          # → bin/coord-server
# or: go build -o bin/coord-server ./cmd/server

# 2. Configure
cp config.toml.example config.toml   # edit as needed
export COORD_ADMIN_API_KEY="your-secret-key"

# 3. Run
./bin/coord-server --config config.toml
```

Server is ready when you see:
```
INFO Servidor coord-server em execução version=0.1.0 listen=0.0.0.0:8080
```

Verify:
```bash
curl http://localhost:8080/health
# {"status":"ok","version":"0.1.0"}
```

---

## Configuration

The server loads configuration from a canonical TOML file (`~/.config/goy-coord/config.toml` by default, customizable via `--config <path>`).

### Unified `config.toml` Reference

```toml
[server]
bind = "0.0.0.0:8080"
read_timeout_seconds = 15
write_timeout_seconds = 15

[auth]
admin_api_key = "your_admin_api_key"
require_auth = true
public_paths = ["/health", "/info", "/metrics"]

[database]
path = "/var/lib/goy-coord/coord.db"

[vpn]
provider = "tailscale" # "tailscale" | "headscale" | ""
tailscale_api_key = "tskey-api-..."
tailscale_tailnet = "example.com"
tailscale_tag = "tag:goy-node"

[ratelimit]
requests_per_minute = 60

[log]
level = "info"
format = "json"
```

---

## 🔄 Environment Variables Deprecation (v0.2.0 → v0.3.0)

Environment variables are supported for backward compatibility in v0.2.0, but are **deprecated** and will be removed in **v0.3.0**. Please migrate to `~/.config/goy-coord/config.toml` or use CLI flags.

| Variable | Replacement in `config.toml` | Alternative CLI Flag | Removal |
|---|---|---|---|
| `COORD_BIND` | `[server] bind` | `--bind` | v0.3.0 |
| `COORD_ADMIN_API_KEY` | `[auth] admin_api_key` | `--admin-api-key` | v0.3.0 |
| `COORD_DB_PATH` | `[database] path` | `--db-path` | v0.3.0 |
| `COORD_VPN_PROVIDER` | `[vpn] provider` | `--vpn-provider` | v0.3.0 |
| `COORD_TAILSCALE_API_KEY` | `[vpn] tailscale_api_key` | *(none)* | v0.3.0 |
| `COORD_HEADSCALE_URL` | `[vpn] headscale_url` | *(none)* | v0.3.0 |
| `COORD_LOG_LEVEL` | `[log] level` | `--log-level` | v0.3.0 |

---

## API Endpoints

Full documentation: [`docs/api.md`](docs/api.md)

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | No | Server health + DB connectivity |
| `GET` | `/info` | No | Version, uptime, config info |
| `GET` | `/metrics` | No | Prometheus metrics |
| `POST` | `/v1/nodes/register` | Bearer | Register a Goy Node (onboarding) |
| `GET` | `/v1/nodes/{id}` | Bearer | Get node details |
| `DELETE` | `/v1/nodes/{id}` | Bearer | Soft-delete a node |
| `GET` | `/v1/vpn/status` | Bearer | Headscale integration status |
| `GET` | `/relays` | Bearer | Discover active relay peers |
| `POST` | `/relays` | Bearer | Announce this node as a relay |
| `PUT` | `/relays/{node_id}` | Bearer | Relay heartbeat (update `last_seen`) |
| `DELETE` | `/relays/{node_id}` | Bearer | Deregister relay |

All protected endpoints require: `Authorization: Bearer <COORD_ADMIN_API_KEY>`

---

## Deployment

### Docker

```bash
docker run -d \
  --name coord-server \
  -p 8080:8080 \
  -v /var/lib/coord-server:/app/data \
  -e COORD_ADMIN_API_KEY="your-secret-key" \
  ghcr.io/goy-co/coord-server:latest
```

With a config file:
```bash
docker run -d \
  --name coord-server \
  -p 8080:8080 \
  -v /etc/coord-server/config.toml:/app/config.toml:ro \
  -v /var/lib/coord-server:/app/data \
  -e COORD_ADMIN_API_KEY="your-secret-key" \
  -e COORD_HEADSCALE_API_KEY="headscale-api-key" \
  ghcr.io/goy-co/coord-server:latest
```

### systemd (Linux)

```bash
# Install using the one-liner install script
curl -fsSL https://raw.githubusercontent.com/goy-co/coord-server/main/deploy/install.sh | bash

# Or manually
sudo cp deploy/coord-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now coord-server
sudo journalctl -u coord-server -f
```

### Manual Binary

```bash
# Download latest release
VERSION=$(curl -s https://api.github.com/repos/goy-co/coord-server/releases/latest | grep tag_name | cut -d'"' -f4)
curl -fsSL "https://github.com/goy-co/coord-server/releases/download/${VERSION}/coord-server_linux_amd64.tar.gz" | tar xz
sudo mv coord-server /usr/local/bin/
```

---

## Integration with Goy Node

Once `coord-server` is running, configure `goy-node` to use it:

```bash
# Set the coord-server URL
export GOY_API_URL="http://your-coord-server:8080"
export GOY_API_KEY="your-secret-key"   # same as COORD_ADMIN_API_KEY

# Onboard the node (this calls POST /v1/nodes/register)
goy-node onboard --auth-key gc_your_auth_key --non-interactive
```

The node will:
1. Register at `POST /v1/nodes/register` → receives `node_id` and optional VPN config
2. Announce its relay at `POST /relays`
3. Send periodic heartbeats via `PUT /relays/{node_id}`
4. Discover peers via `GET /relays`

### VPN Auto-Configuration

When `vpn.enabled = true` and Headscale is reachable, `POST /v1/nodes/register` returns a `vpn_config` with a pre-auth key that `goy-node onboard` uses to join the VPN automatically. No manual intervention needed.

If Headscale is unavailable, the response still succeeds with `vpn_config.auth_key = ""` (graceful degradation) — the operator can configure VPN manually later.

---

## Metrics

`GET /metrics` exposes Prometheus metrics. No authentication required.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `coord_http_requests_total` | Counter | `method`, `path`, `status` | Total HTTP requests processed |
| `coord_http_request_duration_seconds` | Histogram | `method`, `path` | Request latency |
| `coord_nodes_total` | Gauge | `status` | Registered nodes by status (`active`, `inactive`, `deleted`) |
| `coord_relays_active_total` | Gauge | — | Active relays in registry |
| `coord_vpn_keys_generated_total` | Counter | — | Successful Headscale pre-auth key generations |
| `coord_vpn_errors_total` | Counter | — | Headscale API communication errors |
| `coord_auth_failures_total` | Counter | — | Failed authentication attempts |
| `coord_ratelimit_rejected_total` | Counter | — | Requests rejected by rate limiter |
| `coord_db_query_duration_seconds` | Histogram | `operation` | SQLite query duration |

### Example Prometheus scrape config

```yaml
scrape_configs:
  - job_name: coord-server
    static_configs:
      - targets: ['your-coord-server:8080']
```

---

## Development

### Prerequisites

- Go 1.25+
- Make

### Make Targets

```bash
make build      # compile → bin/coord-server
make run        # build + run with defaults
make test       # go test -race ./...
make vet        # go vet ./...
make lint       # golangci-lint run (requires golangci-lint installed)
make clean      # remove build artefacts
```

### Project Structure

```
coord-server/
├── cmd/server/         # Entry point (main.go)
├── internal/
│   ├── api/            # HTTP handlers and router (Chi)
│   ├── config/         # TOML + env configuration loading
│   ├── jobs/           # Background maintenance jobs runner
│   ├── metrics/        # Prometheus metric definitions
│   ├── middleware/     # Auth, rate limit, logging, metrics middleware
│   ├── store/          # SQLite persistence (store.Store interface + SQLiteStore)
│   └── vpn/            # Headscale HTTP client + VPNProvider interface
├── deploy/             # Dockerfile, systemd unit, install script
├── docs/               # API reference, architecture docs
└── .github/workflows/  # CI (lint, test, build, docker) + release pipeline
```

### Running Tests

```bash
go test -race ./...               # all tests with race detector
go test -race ./internal/api/...  # single package
go test -v -run TestNodeEndpoints # single test
```

### Contributing

1. Fork the repo and create a feature branch
2. Follow [Conventional Commits](https://www.conventionalcommits.org/) — enforced by git hook
3. Ensure `go test -race ./...` and `go vet ./...` pass
4. Open a PR against `main`

---

## License

**Goy Source Available** — see [LICENSE](LICENSE) for full terms.

> Related: [goy-node](https://github.com/goy-co/goy-node)
