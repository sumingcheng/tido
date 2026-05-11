# tido

本地 todo list MCP server，给 LLM/agent 提供 7 个工具：批量写入、查询、更新、删除、笔记、增量同步。基于 Go + SQLite，单二进制部署，多 agent 并发安全。

> 设计文档见 [`DESIGN.md`](./DESIGN.md)；agent 使用指引见 [`skill/SKILL.md`](./skill/SKILL.md)。

## 特性

- **MCP stdio**：开箱即用接 Claude Desktop / Cursor / 其他 MCP 客户端
- **7 个工具**：`todo_write` / `todo_list` / `todo_update` / `todo_delete` / `todo_add_note` / `todo_get_notes` / `todo_diff`
- **上下文压缩**：compact 视图省元字段、相对时间渲染；`todo_diff` 增量同步，避免全表 list
- **多 agent 并发**：SQLite WAL + IMMEDIATE 事务，跨进程安全
- **短 ID**：`t1` / `t3a` 而非 UUID，省 token
- **markdown / 纯文本**自动识别，markdown 缩进表父子层级

## 安装

```bash
git clone https://github.com/sumingcheng/tido.git
cd tido
make install   # 编译并放到 ~/.local/bin/tido
```

或自己编译：
```bash
make build     # 输出到 ./bin/tido
```

依赖：Go 1.25+。SQLite 用纯 Go 驱动 `modernc.org/sqlite`，**无需 CGO**。

## 部署形态

tido 一份 binary 支持两种 transport：

| 模式 | 启动 | 适用场景 |
| --- | --- | --- |
| **stdio**（默认） | `tido` | 本地单机；MCP client 直接 spawn 子进程 |
| **HTTP** | `tido -http :8080` | 容器化、远程访问；需要 `TIDO_TOKEN` env |

### 方式 1：本地 stdio（最简单）

数据库默认存在 `~/.tido/todos.db`，可用 `TIDO_HOME` 覆盖。

Claude Desktop / Cursor 的 MCP 配置：
```json
{
  "mcpServers": {
    "tido": { "command": "/path/to/tido" }
  }
}
```

### 方式 2：Docker 部署（HTTP transport，本机端口）

```bash
cp .env.example .env
# 编辑 .env：把 TIDO_TOKEN 改成强随机串（openssl rand -hex 32）
make docker-up         # docker-compose up -d
make docker-logs       # 看启动日志
```

镜像约 14 MB（distroless static + 静态链接的 Go binary），nonroot 运行，db 落在 named volume `tido_tido-data`。

Cursor 配置切换为 HTTP transport：
```json
{
  "mcpServers": {
    "tido": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer <你的 TIDO_TOKEN>" }
    }
  }
}
```

端口冲突时改 `.env` 里的 `TIDO_PORT`，对应 Cursor URL 也改。

#### 备份 named volume

```bash
docker run --rm -v tido_tido-data:/data -v $PWD:/backup alpine \
  tar czf /backup/tido-$(date +%F).tgz -C /data .
```

#### 安全提示

- HTTP 模式启动时若 `TIDO_TOKEN` 为空，server 拒绝启动（避免裸奔）
- 默认只绑回环 `127.0.0.1`，不暴公网
- 要远程访问：上 Tailscale/WireGuard，或在前面挂 caddy/nginx 做 TLS

### 验证连通（stdio）

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | tido
```

应该看到 8 个工具（`ping` + 7 个 todo 工具）。

## 开发

```bash
make test            # 全量单元 + 集成测试（含 -race）
make build           # 编译二进制
make fmt vet tidy    # 代码格式化 / vet / 依赖整理
make docker-build    # 构建 docker 镜像
make docker-up/down  # docker compose 起/停
make docker-logs     # 看容器日志
```

测试覆盖：

| 包 | 覆盖范围 |
| --- | --- |
| `internal/store` | schema migration、CRUD、cascade tombstone、并发版本号 |
| `internal/parser` | markdown / 文本解析、缩进父子、状态映射 |
| `internal/validate` | 字符校验（UTF-8 / NUL / ANSI / 长度） |
| `internal/shortid` | base36 编解码 |
| `internal/view` | compact / full 视图、相对时间渲染 |
| `internal/mcp` | 7 个工具端到端集成 |

## 架构

```
cmd/tido/main.go            # MCP server 入口（stdio）
internal/
  store/                    # SQLite + schema migration + CRUD
  parser/                   # markdown / 纯文本解析
  validate/                 # 字符 / scope / id 校验
  shortid/                  # base36 短码
  view/                     # compact / full 视图渲染
  mcp/                      # 7 个 MCP 工具 handler
skill/
  SKILL.md                  # agent 使用指引
```

## License

待定。
