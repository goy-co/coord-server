# API Documentation - `coord-server`

## Autenticação & Segurança

Quando `auth.require_auth = true` (por defeito), todos os pedidos aos endpoints protegidos exigem a passagem do header `Authorization: Bearer <key>`. A chave de API do administrador é definida através da variável de ambiente `COORD_ADMIN_API_KEY`.

### Erros de Autenticação & Rate Limiting

**`401 Unauthorized` (Token em falta ou inválido):**
```json
{
  "error": "unauthorized",
  "details": "valid API key required"
}
```

**`429 Too Many Requests` (Limite de pedidos excedido):**
Headers retornados: `Retry-After: N` (tempo de espera recomendado em segundos)
```json
{
  "error": "rate limit exceeded",
  "reason": "limite de pedidos HTTP excedido"
}
```

---

## Base Endpoints (Isentos de Autenticação)

### Health Check

```http
GET /health
```

Retorna o estado de saúde do servidor e conectividade com a base de dados SQLite. Isento de autenticação e rate limit.

**Resposta de Sucesso (`200 OK`):**
```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

**Resposta de Erro (`503 Service Unavailable`):**
```json
{
  "status": "degraded",
  "version": "0.1.0",
  "error": "database unreachable"
}
```

---

### Instância & Info

```http
GET /info
```

Retorna detalhes de execução, uptime e caminhos de configuração. Isento de autenticação e rate limit.

**Resposta de Sucesso (`200 OK`):**
```json
{
  "version": "0.1.0",
  "uptime_seconds": 120,
  "database_path": "data/coord-server.db",
  "listen_address": "0.0.0.0:8080",
  "vpn_integration_enabled": false
}
```

---

## Nodes API (`/v1/nodes` - Protegida)

### Register Node (Onboarding)

```http
POST /v1/nodes/register
Authorization: Bearer <COORD_ADMIN_API_KEY>
Content-Type: application/json
```

**Request Body:**
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

**Resposta de Sucesso (`201 Created` / `200 OK` para registo idempotente):**
```json
{
  "node_id": "goy-node-63898b5aaee9bb45",
  "name": "my-goy-node",
  "mesh_url": "100.80.1.5:8443",
  "vpn_config": {
    "auth_key": "tskey-auth-1234567890abcdef",
    "control_url": "https://vpn.goyco.xyz"
  },
  "registry_url": "http://localhost:8080",
  "created_at": "2026-08-10T15:59:01Z"
}
```

> **Nota:** Se a integração VPN estiver desativada (`vpn.enabled = false`) ou se o Headscale estiver temporariamente indisponível, a chave `vpn_config.auth_key` é retornada vazia (`""`) e o registo do nó sucede normalmente (graceful degradation).

---

### Get Node Details

```http
GET /v1/nodes/{id}
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Resposta de Sucesso (`200 OK`):**
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
  "updated_at": "2026-08-10T15:59:01Z"
}
```

---

### Delete Node (Soft Delete)

```http
DELETE /v1/nodes/{id}
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Resposta de Sucesso (`204 No Content`)**

---

## VPN Diagnostics API (`/v1/vpn` - Protegida)

### Get VPN Status

```http
GET /v1/vpn/status
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Resposta de Sucesso (`200 OK`):**
```json
{
  "vpn_enabled": true,
  "headscale_reachable": true,
  "headscale_user": "goy-nodes",
  "registered_machines": 5
}
```

---

## Relays API (`/relays` - Protegida)

### Peer Discovery (Get Active Relays)

```http
GET /relays
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Query Parameters (Opcionais):**
- `since`: timestamp RFC3339 (ex: `2026-08-10T12:00:00Z`) para descoberta incremental.
- `min_storage_gb`: número inteiro positivo para filtrar por capacidade mínima.

**Resposta de Sucesso (`200 OK`):**
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

---

### Register or Update Relay

```http
POST /relays
Authorization: Bearer <COORD_ADMIN_API_KEY>
Content-Type: application/json
```

**Request Body:**
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

**Resposta de Sucesso (`201 Created`):**
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

---

### Relay Heartbeat

```http
PUT /relays/{node_id}
Authorization: Bearer <COORD_ADMIN_API_KEY>
Content-Type: application/json
```

**Request Body (Parcial Opcional):**
```json
{
  "storage": {
    "available_gb": 145
  }
}
```

**Resposta de Sucesso (`204 No Content`)**

---

### Deregister Relay (Hard Delete)

```http
DELETE /relays/{node_id}
Authorization: Bearer <COORD_ADMIN_API_KEY>
```

**Resposta de Sucesso (`204 No Content`)**
