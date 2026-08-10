# coord-server

> Coordination server and registry for the Goy mesh network.

## Visão Geral

O `coord-server` é a infraestrutura de coordenação e registo da rede Goy mesh network. É responsável por gerir o onboarding de nós, coordenar conectividade (relays e VPN), e fornecer informação sobre o estado da rede.

> **Nota:** Work in progress — endpoints de onboarding e registry em desenvolvimento.

## Quick Start

### Requisitos

- Go 1.23+
- Make (opcional)

### Compilar e Executar

```bash
# Compilar a aplicação
make build

# Executar localmente
make run
```

Ou diretamente via Go:

```bash
go run ./cmd/server
```

## Configuração

O servidor pode ser configurado através de um ficheiro TOML (por defeito `config.toml`) e/ou variáveis de ambiente com o prefixo `COORD_`.

### Ficheiro `config.toml` (Template gerado automaticamente se não existir)

```toml
# Configuração do Coord Server

[server]
listen = "0.0.0.0:8080"
read_timeout_seconds = 15
write_timeout_seconds = 15

[database]
path = "data/coord-server.db"

[auth]
# Importante: O segredo HMAC deve ser definido via env COORD_AUTH_SECRET

[vpn]
headscale_api_url = ""
headscale_api_key = ""
```

### Variáveis de Ambiente (Overrides)

| Variável de Ambiente | Descrição | Defeito |
|---|---|---|
| `COORD_LISTEN` | Endereço e porta de escuta HTTP | `0.0.0.0:8080` |
| `COORD_DB_PATH` | Caminho para o ficheiro SQLite | `data/coord-server.db` |
| `COORD_AUTH_SECRET` | Segredo HMAC para validação de chaves `gc_...` | "" |

## Endpoints Disponíveis

- `GET /health` — Verificação de estado do servidor e conectividade à base de dados.
- `GET /info` — Informação geral sobre a instância (versão, uptime, DB path, listen address).

## Repositórios Relacionados

- [Goy Node](https://github.com/goy-co/goy-node)
