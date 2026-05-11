# tido

**English** | [中文](./README.zh-CN.md)

Local todo list MCP server. Exposes 7 tools to LLMs/agents: batch write, list, update, delete, notes, incremental sync. Built on Go + SQLite, single-binary deploy, safe for concurrent multi-agent use.

> Design doc: [`DESIGN.md`](./DESIGN.md). Agent usage guide: [`skill/SKILL.md`](./skill/SKILL.md).

## Features

- **MCP stdio**: drop-in for Claude Desktop / Cursor / any MCP client
- **7 tools**: `todo_write` / `todo_list` / `todo_update` / `todo_delete` / `todo_add_note` / `todo_get_notes` / `todo_diff`
- **Context compression**: compact view drops metadata + relative time; `todo_diff` for incremental sync instead of full list
- **Concurrent multi-agent**: SQLite WAL + IMMEDIATE transactions, cross-process safe
- **Short IDs**: `t1` / `t3a` instead of UUIDs, saves tokens
- **Markdown / plain text** auto-detected, indentation expresses parent/child

## Install

```bash
git clone https://github.com/sumingcheng/tido.git
cd tido
make install   # builds and installs to ~/.local/bin/tido
```

Or just build:
```bash
make build     # outputs ./bin/tido
```

Requires Go 1.25+. SQLite uses pure-Go driver `modernc.org/sqlite`; **no CGO needed**.

## Deployment modes

A single binary supports two transports:

| Mode | Start | Use case |
| --- | --- | --- |
| **stdio** (default) | `tido` | Local single-machine; MCP client spawns it as a subprocess |
| **HTTP** | `tido -http :8080` | Containerized / remote access; requires `TIDO_TOKEN` env |

### Option 1: local stdio (simplest)

DB defaults to `~/.tido/todos.db`; override with `TIDO_HOME`.

Claude Desktop / Cursor MCP config:
```json
{
  "mcpServers": {
    "tido": { "command": "/path/to/tido" }
  }
}
```

### Option 2: Docker (HTTP transport, loopback port)

```bash
cp .env.example .env
# Edit .env: set TIDO_TOKEN to a strong random string (openssl rand -hex 32)
make docker-up         # docker-compose up -d
make docker-logs       # tail startup logs
```

Image is ~14 MB (distroless static + statically linked Go binary), runs as nonroot, DB stored in named volume `tido_tido-data`.

Cursor config switched to HTTP transport:
```json
{
  "mcpServers": {
    "tido": {
      "url": "http://127.0.0.1:18080/mcp",
      "headers": { "Authorization": "Bearer <your TIDO_TOKEN>" }
    }
  }
}
```

If port 18080 is taken, set `TIDO_PORT` in `.env` and update the Cursor URL accordingly.

#### Multi-container access (other docker containers calling tido)

Don't open `0.0.0.0` just so neighbor containers can reach tido. Use a shared docker network instead — zero exposure to the host network:

```bash
# one-time on the host
docker network create mcp-net
```

`tido`'s compose already joins `mcp-net` (declared as `external`). Make any other compose project join the same network:

```yaml
services:
  your-service:
    networks: [default, mcp-net]

networks:
  mcp-net:
    external: true
    name: mcp-net
```

Then from inside that container call `http://tido:8080/mcp` (container name as DNS, container-internal port 8080 — not the host's 18080).

#### Backup the named volume

```bash
docker run --rm -v tido_tido-data:/data -v $PWD:/backup alpine \
  tar czf /backup/tido-$(date +%F).tgz -C /data .
```

#### Security notes

- HTTP mode refuses to start if `TIDO_TOKEN` is empty (prevents accidental public exposure)
- Default binds `127.0.0.1` only; never directly exposed to the public internet
- For remote access: put it on Tailscale/WireGuard, or front it with caddy/nginx for TLS

### Smoke test (stdio)

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | tido
```

You should see 8 tools (`ping` + 7 todo tools).

## Development

```bash
make test            # full unit + integration tests (with -race)
make build           # compile binary
make fmt vet tidy    # gofmt / go vet / go mod tidy
make docker-build    # build docker image
make docker-up/down  # docker compose up/down
make docker-logs     # tail container logs
```

Test coverage:

| Package | Scope |
| --- | --- |
| `internal/store` | schema migration, CRUD, cascade tombstone, concurrent versioning |
| `internal/parser` | markdown / text parsing, indentation hierarchy, status mapping |
| `internal/validate` | char checks (UTF-8 / NUL / ANSI / length) |
| `internal/shortid` | base36 encode/decode |
| `internal/view` | compact / full view, relative time rendering |
| `internal/mcp` | end-to-end integration of all 7 tools |

## Architecture

```
cmd/tido/main.go            # MCP server entry (stdio + HTTP transport)
cmd/tido/http.go            # HTTP transport + bearer auth middleware
internal/
  store/                    # SQLite + schema migration + CRUD
  parser/                   # markdown / plain text parsing
  validate/                 # char / scope / id validation
  shortid/                  # base36 short codes
  view/                     # compact / full view rendering
  mcp/                      # 7 MCP tool handlers
skill/
  SKILL.md                  # agent usage guide
```

## License

TBD.
