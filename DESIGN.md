# Tido — Todo List MCP for LLM Agents

> **v1 形态 = MCP server + Skill**。Go 实现的 MCP server 提供 7 个 todo 工具（数据存 SQLite），配套 `skill/SKILL.md` 教 agent 何时/如何使用这些工具。机械性事务（并发、原子性、查询）由代码保证，语义判断（何时用哪个工具）由 Skill 引导。
>
> 一句话：给 LLM/Agent 提供高性能、低 context 占用的 todo list MCP server，单机本地存储，支持多 agent 并发，多格式写入。

---

## 1. 设计目标与原则

| 目标 | 落地手段 |
|---|---|
| 减少 context 占用 | 智能视图（默认折叠 completed） + 增量 diff（version cursor） + 短码 ID（base36） |
| 多 agent 并发安全 | SQLite WAL 模式 + 全局单调 `version` + 事务原子写 |
| 多格式快速写入 | Markdown checklist / 纯文本 自动识别归一化 |
| 字符与数据安全 | 全量参数化查询 + 输入校验 + 规范化 + Schema CHECK |
| 零依赖分发 | 纯 Go（无 CGO），单二进制 |

**不做的事**：UI、多用户、分布式同步、tags / priority / due_at、人类时间管理字段、全文搜索（v1 内）。

---

## 2. 技术栈

| 项 | 选择 |
|---|---|
| 语言 | Go 1.22+ |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` |
| 存储引擎 | SQLite |
| SQLite 驱动 | `modernc.org/sqlite`（纯 Go，无 CGO，可交叉编译） |
| 传输 | stdio（MCP 默认，IDE 拉起子进程） |
| 数据路径 | `$TIDO_HOME` 优先，否则 `~/.tido/` |
| 数据文件 | `~/.tido/todos.db`（+ WAL 自动产生 `.db-wal` `.db-shm`） |

---

## 3. 数据模型

### 3.1 表结构

```sql
-- 全局元数据：ID 计数器 + 版本号
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value INTEGER NOT NULL
);
INSERT INTO meta(key, value) VALUES ('last_id', 0), ('version', 0);

-- 主表
CREATE TABLE todos (
  id          TEXT    PRIMARY KEY,                        -- 短码 t1, t2, ..., t3a (base36)
  scope       TEXT    NOT NULL DEFAULT 'default'
              CHECK(length(scope) BETWEEN 1 AND 64),
  content     TEXT    NOT NULL
              CHECK(length(content) BETWEEN 1 AND 4096),  -- 4 KiB 硬墙
  status      TEXT    NOT NULL
              CHECK(status IN ('pending','in_progress','completed','cancelled')),
  priority    TEXT    NOT NULL DEFAULT 'medium'
              CHECK(priority IN ('low','medium','high','urgent')),
  difficulty  TEXT    NOT NULL DEFAULT 'medium'
              CHECK(difficulty IN ('trivial','easy','medium','hard')),
  due_at      INTEGER,                                    -- unix ms，NULL 表示无截止
  parent_id   TEXT    REFERENCES todos(id) ON DELETE CASCADE,
  version     INTEGER NOT NULL,                           -- 该行最后修改时的全局版本号
  created_at  INTEGER NOT NULL,                           -- unix ms
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_todos_scope_status ON todos(scope, status);
CREATE INDEX idx_todos_scope_version ON todos(scope, version);
CREATE INDEX idx_todos_scope_parent  ON todos(scope, parent_id);

-- 留痕表：append-only，记录 agent 失败信息、思考过程
CREATE TABLE notes (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  todo_id     TEXT    NOT NULL REFERENCES todos(id) ON DELETE CASCADE,
  content     TEXT    NOT NULL CHECK(length(content) BETWEEN 1 AND 8192),
  created_at  INTEGER NOT NULL
);

CREATE INDEX idx_notes_todo ON notes(todo_id, id);

-- 删除墓碑：让 diff 能传播删除事件
CREATE TABLE deletions (
  todo_id     TEXT    PRIMARY KEY,
  scope       TEXT    NOT NULL,
  version     INTEGER NOT NULL,
  deleted_at  INTEGER NOT NULL
);

CREATE INDEX idx_deletions_scope_version ON deletions(scope, version);

-- 删除 trigger：物理删 todos 后自动写 tombstone
-- 关键：CASCADE 删除子任务时也会逐行触发，保证 diff 不丢事件
CREATE TRIGGER trg_todos_after_delete
AFTER DELETE ON todos
FOR EACH ROW
BEGIN
  INSERT OR IGNORE INTO deletions(todo_id, scope, version, deleted_at)
  VALUES (
    OLD.id,
    OLD.scope,
    (SELECT value FROM meta WHERE key = 'version'),
    CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)  -- unix ms，与 todos.updated_at 精度一致
  );
END;
```

### 3.2 索引设计依据（每个索引都对应一个真实查询）

| 查询模式 | 索引 |
|---|---|
| `WHERE scope=? AND status=?`（list 过滤） | `(scope, status)` |
| `WHERE scope=? AND version > ? ORDER BY version`（diff 主查询） | `(scope, version)` |
| `WHERE scope=? AND parent_id=?`（按父任务过滤） | `(scope, parent_id)` |
| `WHERE id=?` | PRIMARY KEY 自带 |
| `WHERE todo_id=? ORDER BY id`（notes 拉取） | `(todo_id, id)` |
| `WHERE scope=? AND version > ?`（deletions 拉取） | `(scope, version)` |
| `ORDER BY priority / due_at`（list `sort=priority\|due`） | **不建索引**，依赖 `(scope, status)` 过滤后内存排序，几千条无忧 |

### 3.3 字段约定

- **id**：短码格式 `^t[0-9a-z]{1,8}$`，事务内 `UPDATE meta SET value=value+1 RETURNING value` 取下一个，转 base36 加 `t` 前缀。无并发竞争。
- **version**：全局单调递增整数。**一次工具调用 = 1 个新 version**，本次受影响的所有行写同一个 version 值。是 diff cursor 的唯一依据，不依赖时间戳。
- **scope**：agent 显式传，默认 `default`。字符白名单 `^[a-zA-Z0-9_./:-]{1,64}$`。
- **parent_id**：允许任意深度嵌套。`NULL` 表示顶层。`ON DELETE CASCADE` 删父任务自动删子任务。建议层级 ≤ 2 层（在 SKILL.md 中作为文化约束）。
- **status**：4 态枚举，CHECK 约束防写错。
- **priority**：4 档枚举 `low / medium / high / urgent`，默认 `medium`。agent 决策"下一个该做什么"用。
- **difficulty**：4 档枚举 `trivial / easy / medium / hard`，默认 `medium`。agent 决策"是否拆子任务、要不要更多 context"用。
- **due_at**：unix ms，可空。**只表达外部约束**（用户规定的截止），不是 agent 自己的预估。无截止则 `NULL`。

---

## 4. MCP 工具集（7 个）

| 工具 | 入参 | 返回 | 说明 |
|---|---|---|---|
| `todo_write` | `items` (string，≤ 32 KiB，≤ 200 条), `scope?`, `parent_id?`, `priority?`, `difficulty?`, `due_at?` | `{ids:[...], cursor}` | 批量新建。`items` 自动识别 markdown / 纯文本。`priority / difficulty / due_at` 整批应用同一值（不传走默认）。`parent_id` 整批挂在某个已有父任务下。**只新建，不覆盖、不合并** |
| `todo_list` | `scope?`, `status?`, `parent_id?`, `view=compact\|full`, `sort=created\|priority\|due`, `limit=100`, `offset=0` | `{items:[...含 notes_count...], total, has_more}` | compact 折叠 completed/cancelled，见 §6.3。`sort` 默认 `created`；`priority` 按优先级降序、`due` 按截止时间升序（NULL 排最后）。`limit` 最大 500 |
| `todo_update` | `id`, `status?`, `content?`, `priority?`, `difficulty?`, `due_at?` | `{ok, cursor}` | 单条更新。**v1 不允许改 `parent_id`**（避免循环依赖）。`due_at` 传 `0` 表示清除截止 |
| `todo_delete` | `ids[]` 或 `scope` (二选一) | `{ok, cursor, deleted:[ids...]}` | 物理删 + tombstone（含 cascade 子任务）。`scope` 模式：清空整个 scope（一键 reset） |
| `todo_add_note` | `id`, `content` | `{ok, note_id}` | 追加思考笔记。**不更新 todo.version、不污染 diff 流** |
| `todo_get_notes` | `id`, `limit=20`, `offset=0` | `{notes:[{id, content, created_at}], has_more}` | 拉取某 todo 的笔记，按 id 升序（创建顺序） |
| `todo_diff` | `scope?`, `since=cursor`, `limit=50` | `{changes:[{op:upsert (含 notes_count)\|delete, ...}], next_cursor, has_more}` | 增量同步。分页拉取。**不含 notes 内容**，仅 upsert 项含 `notes_count` 提示有几条笔记 |

**刻意不做**：`todo_get(id)`（list 加过滤够用）。

设计原则：

- 每个工具单一责任。`todo_write` 不做合并/覆盖；"清空 scope 重来"用 `todo_delete(scope=?)` + `todo_write` 两步表达，比内嵌 mode 参数语义更清晰。
- `parent_id` 在创建时指定，**不允许后续修改**——避免循环（`t1.parent=t2, t2.parent=t1`）。改动需求由 delete + recreate 表达。

### 4.1 写入流程（事务内）

**所有写事务必须 `BEGIN IMMEDIATE`**，立即获取写锁，避免多进程并发下的 deferred → upgrade 冲突（SQLITE_BUSY）。

```sql
BEGIN IMMEDIATE;
  -- 一次调用分配 1 个 version（所有受影响行共享）
  UPDATE meta SET value = value + 1 WHERE key='version' RETURNING value;  -- → next_ver

  -- 一次调用按需分配 N 个 id（批量写入时）
  UPDATE meta SET value = value + N WHERE key='last_id' RETURNING value;  -- → last_allocated_id
  -- 应用层用 (last_allocated_id - N + 1 .. last_allocated_id) 生成 N 个短码

  INSERT INTO todos(id, scope, content, status, priority, difficulty, due_at,
                    parent_id, version, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, next_ver, ts, ts), ... ;  -- 批量
COMMIT;
```

`todo_update` / `todo_delete` / `todo_add_note` 同理：进事务先 `+1 version`，再写主表（add_note 写 notes 表，version 不影响 todos 行）。

**Batch insert 顺序**：markdown 缩进解析天然产生父→子序（DFS 遍历输入即可），按此序 INSERT 即可保证 FK 约束在每行写入时已成立。**不会成环**（新建任务的 parent 要么是 batch 内更早缩进的兄弟，要么是 `parent_id` 入参指定的已有 todo）。

### 4.2 diff 查询（分页）

```sql
SELECT t.id, t.scope, t.content, t.status,
       t.priority, t.difficulty, t.due_at,
       t.parent_id, t.version, t.updated_at,
       (SELECT COUNT(*) FROM notes WHERE notes.todo_id = t.id) AS notes_count,
       'upsert' AS op
  FROM todos t WHERE t.scope=? AND t.version > ?
UNION ALL
SELECT todo_id, scope, NULL, NULL,
       NULL, NULL, NULL,
       NULL, version, deleted_at, 0 AS notes_count, 'delete'
  FROM deletions WHERE scope=? AND version > ?
ORDER BY version
LIMIT ?;     -- 默认 50，最大 200
```

返回语义：

- `has_more = true` 时，`next_cursor` = 本批最后一条的 `version`，agent 应继续调用直到 `has_more=false`
- `has_more = false` 时，`next_cursor` = 当前 `meta.version`（已追到最新；下次调用从此处继续）
- `notes_count` 为提示字段，告诉 agent "这条 todo 有几条笔记"，**不返回笔记内容**；要看内容显式调 `todo_get_notes`

### 4.3 `todo_delete(scope=?)` 的实现

`scope` 模式下，先批量取该 scope 的所有 id（含子任务），事务内逐条 DELETE（trigger 自动写 tombstone）：

```sql
BEGIN IMMEDIATE;
  UPDATE meta SET value = value + 1 WHERE key='version' RETURNING value;
  DELETE FROM todos WHERE scope = ?;     -- 一条 DML，trigger 对每行触发
COMMIT;
```

返回 `deleted` 列表为本次实际删除的 id 集合。

### 4.4 `todo_list` 三种排序的 SQL

```sql
-- sort=created（默认）
ORDER BY created_at ASC, id ASC

-- sort=priority（高在前；同优先级按 due_at 升序、created_at 升序）
ORDER BY CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1
                       WHEN 'medium' THEN 2 WHEN 'low' THEN 3 END ASC,
         CASE WHEN due_at IS NULL THEN 1 ELSE 0 END ASC,  -- 有截止的在前
         due_at ASC,
         created_at ASC, id ASC

-- sort=due（有截止的按时间升序，无截止的统一排最后）
ORDER BY CASE WHEN due_at IS NULL THEN 1 ELSE 0 END ASC,
         due_at ASC,
         created_at ASC, id ASC
```

### 4.5 错误码

MCP 工具失败时通过 `isError: true` + 结构化 payload 返回，code 取以下枚举：

| 错误码 | 触发场景 |
|---|---|
| `INVALID_INPUT` | 参数缺失 / 字符非法 / 长度越界 / scope 字符集不合规 / `todo_update` 未传任何可选字段 / `todo_delete` 同时传 `ids` 和 `scope` 或都未传 / `priority` 或 `difficulty` 不在枚举值 / `due_at` 不是合法 unix ms / `sort` 不在枚举值 |
| `NOT_FOUND` | `id` 在 db 中不存在（update / delete / add_note / get_notes / `parent_id` 入参引用不存在） |
| `PARSE_ERROR` | `items` 既不是合法 markdown checklist 也无法当作纯文本切分（罕见，比如全空或全非法控制字符） |
| `TOO_LARGE` | `items` 超 32 KiB 或解析后 > 200 条 / `diff.limit` > 200 / `list.limit` > 500 / `get_notes.limit` > 100 / 单条 content 超 4 KiB |
| `INTERNAL` | DB 层错误（含 SQLITE_BUSY 重试用尽、磁盘 IO 失败等） |

---

## 5. 输入解析

`items` 是一段文本，自动识别两种格式：

| 格式 | 识别规则 | 解析方式 |
|---|---|---|
| **Markdown checklist** | 任一非空行匹配 `^\s*-\s+\[[ x\-~]\]\s+` | 正则 `^(\s*)-\s+\[([ x\-~])\]\s+(.+)$`；状态映射：` `→pending, `x`→completed, `-`/`~`→in_progress；**缩进每 2 空格 = 下一级父子**，任意深度 |
| **纯文本** | 其他 | 按 `\n` 分割，非空行即一个 pending todo（无父子） |

最终归一化为内部数组 `[{content, status, depth}, ...]`，`depth` 用于在 batch 内构建父子树。`todo_write` 入参 `parent_id` 作为整个 batch 的根父任务（不传则顶层节点 `parent_id=NULL`）。

**markdown 不携带元信息**（priority / difficulty / due_at）。这些字段通过 `todo_write` 顶层入参整批应用，或事后用 `todo_update` 单条修改。理由：保持解析简单、避免与内容字面量歧义。

**示例**：

```markdown
- [ ] 实现登录功能
  - [ ] 前端表单
  - [-] 后端 API
    - [ ] JWT 签发
- [x] 写文档
```

解析后：5 条 todo，"前端表单"和"后端 API"挂在"实现登录功能"下，"JWT 签发"挂在"后端 API"下。

**给已有任务加子任务**：

```text
todo_write(items="- [ ] 子任务A\n- [ ] 子任务B", parent_id="t3")
```

整批挂在 `t3` 下。`parent_id` 必须是 db 中已存在的 todo id，否则 `NOT_FOUND`。

---

## 6. 字符处理策略（分层防御）

### 6.1 必做（安全/正确性）

| 防线 | 措施 |
|---|---|
| SQL 注入 | **100% 参数化查询，永禁字面量拼接 SQL**。CI 加规则检测 |
| 字符编码 | 入口 `utf8.ValidString()` 校验，无效拒绝 |
| NUL 字节 | content/scope 含 `\x00` 直接拒绝 |
| 控制字符 | 除 `\n` `\t` 外的 `< 0x20` 字符拒绝（含 ANSI escape `\x1b[`） |
| 长度上限 | content ≤ 4 KiB byte / ≤ 2000 rune；note ≤ 8 KiB；scope ≤ 64 char |
| scope 字符集 | 白名单 `^[a-zA-Z0-9_./:-]{1,64}$` |
| id 字符集 | 白名单 `^t[0-9a-z]{1,8}$` |

### 6.2 规范化（normalize 而非拒绝）

```go
func Normalize(s string) string {
    s = strings.ReplaceAll(s, "\r\n", "\n")
    s = strings.ReplaceAll(s, "\r",   "\n")
    s = strings.TrimRight(s, " \t\n")
    return s
}
```

不做：Unicode NFC/NFD 归一化、零宽字符剥离、大小写转换。

### 6.3 输出渲染

输出格式：markdown checklist（`- [ ]` pending / `- [x]` completed / `- [-]` in_progress / `- [~]` cancelled），子任务每级缩进 2 空格。

**数据获取层 SQL**（仅取数，不负责树形排序）：

```sql
ORDER BY created_at ASC, id ASC    -- 稳定，避免 base36 字典序歧义如 t10 < t9
```

**应用层负责构建父子树并按 DFS 展开输出**——这样无论嵌套多少层，都能正确渲染为缩进 markdown，避免在 SQL 里写复杂递归 CTE。同父兄弟节点按 `created_at` 排序。

| 视图 | pending / in_progress | completed / cancelled |
|---|---|---|
| `compact`（默认） | 完整显示，content 单行（`\n` → `↵`），超 80 rune 末尾 `…` | 折叠成一行汇总：`↳ 12 completed, 3 cancelled (use status filter to expand)` |
| `full` | content 原样输出（保留换行），元信息全部展示 | 同左 |

**元信息渲染规则（compact）**——只在**非默认值**时展示，节省 token：

| 字段 | 默认值 | 非默认时的标记 |
|---|---|---|
| `priority` | `medium` | `!low` / `!high` / `!urgent` |
| `difficulty` | `medium` | `#trivial` / `#easy` / `#hard` |
| `due_at` | `NULL` | `@<相对时间>` 例 `@2d`, `@3h`, `@overdue` |

示例：`- [ ] t3 !urgent #hard @2d 修复登录` ← 急、难、2 天后到期。普通任务直接 `- [ ] t4 改文档`，无任何标记。

LLM 渲染时**不转义**反引号、`|` 等 markdown 元字符（保留 content 原始语义）。

### 6.4 故意允许的"危险字符"

反斜杠、引号、HTML 标签、SQL 关键字、emoji、多语言、组合字符——**全部原样存**。安全靠机制（参数化查询），不靠字符黑名单。

---

## 7. SQLite 配置

### 7.1 PRAGMA

```sql
PRAGMA journal_mode       = WAL;        -- 多进程并发不阻塞读
PRAGMA synchronous        = NORMAL;     -- WAL 下安全，比 FULL 快
PRAGMA busy_timeout       = 5000;       -- 写竞争时等 5s
PRAGMA foreign_keys       = ON;         -- 启用 CASCADE
PRAGMA recursive_triggers = ON;         -- CASCADE 删除时也激活 trigger（tombstone 不丢，关键！）
PRAGMA temp_store         = MEMORY;
PRAGMA cache_size         = -64000;     -- 64 MiB
PRAGMA mmap_size          = 134217728;  -- 128 MiB
```

> ⚠️ `recursive_triggers = ON` 是不变量 §9.5 成立的前提：SQLite 默认 OFF 时，FK CASCADE 触发的删除**不会**激活 `trg_todos_after_delete`，导致子任务 tombstone 丢失，diff 数据不一致。

### 7.2 DSN 模板（modernc.org/sqlite）

PRAGMA 与 `_txlock=immediate` 一并写入 DSN，让 `db.BeginTx()` 自动用 `BEGIN IMMEDIATE`，避免 deferred → upgrade 冲突：

```go
// internal/store/store.go
dsn := "file:" + dbPath +
    "?_pragma=journal_mode(WAL)" +
    "&_pragma=synchronous(NORMAL)" +
    "&_pragma=busy_timeout(5000)" +
    "&_pragma=foreign_keys(ON)" +
    "&_pragma=recursive_triggers(ON)" +
    "&_pragma=temp_store(MEMORY)" +
    "&_pragma=cache_size(-64000)" +
    "&_pragma=mmap_size(134217728)" +
    "&_txlock=immediate"

db, err := sql.Open("sqlite", dsn)
```

连接池策略：保持 `database/sql` 默认设置即可。WAL + `busy_timeout=5000` + `_txlock=immediate` 已能处理多连接的写竞争（writer 排队，reader 不阻塞）。**不要** `SetMaxOpenConns(1)`——那会让读也串行化，浪费 WAL。

### 7.3 Schema 版本管理

用 SQLite 内置 `PRAGMA user_version` 追踪 schema 版本：

- v1 = `1`
- 启动时读 `PRAGMA user_version`，与代码内置版本比较：
  - `0` → 新库，执行初始化建表 + `PRAGMA user_version = 1`
  - `< current` → 按顺序执行 migration 脚本至当前版本
  - `> current` → 拒绝启动（旧版二进制不兼容新库）
- `internal/store/migration.go` 维护版本号 → SQL 脚本的映射表。

---

## 8. 目录结构

```
tido/
├── cmd/
│   └── tido/
│       └── main.go               # MCP server 入口（stdio）
├── internal/
│   ├── store/                    # SQLite 封装
│   │   ├── store.go              # 连接、PRAGMA、事务
│   │   ├── todo.go               # CRUD
│   │   ├── note.go               # notes append
│   │   ├── diff.go               # 增量查询
│   │   └── migration.go          # schema 初始化与版本迁移
│   ├── shortid/                  # base36 编码 / id 校验
│   ├── validate/                 # 字符与字段校验
│   │   ├── content.go
│   │   ├── scope.go
│   │   └── id.go
│   ├── parser/                   # 输入解析：markdown / 纯文本
│   │   ├── parse.go              # detect + 两种格式解析 + DFS 构树
│   │   └── normalize.go          # 换行规范化、trim
│   ├── view/                     # 输出渲染（compact / full / diff）
│   └── mcp/                      # MCP tool handler
│       ├── server.go             # New + RegisterTools + Run
│       └── tools.go              # 7 个 tool 的 handler（args struct + 复杂 schema patch）
├── skill/
│   └── SKILL.md                  # 给 agent 的使用指引
├── go.mod
├── DESIGN.md                     # 本文档
└── Makefile
```

**模块化原则**：
- `store` 不依赖 `mcp`（可单独 import 做嵌入式）
- `validate` `parser` `shortid` 是纯函数包，无 I/O，易测
- `mcp` 是最薄的一层，只做协议转发与参数校验

---

## 9. 关键不变量（实现时必须保持）

1. **一次工具调用 = 1 个新 version**。本次受影响的所有行写同一个 version 值（批量 insert / batch update / cascade delete 都遵守）。
2. **所有写事务必须 `BEGIN IMMEDIATE`**，立即获取写锁。
3. **id 一旦分配永不复用**（`last_id` 单调递增，删除不回收）。
4. **deletions 表对一个 `todo_id` 至多一条**（`INSERT OR IGNORE`，由 `AFTER DELETE` trigger 维护）。
5. **物理删 + tombstone**：`todo_delete` 物理删除 todos 行，trigger 自动写 deletions；CASCADE 删除子任务时 trigger 同样触发（**依赖 `PRAGMA recursive_triggers=ON`，见 §7.1**），diff 不丢事件。
6. **content / scope 进 DB 前必经 validate + normalize**（§6）。
7. **任何 SQL 经 prepared statement 执行**，禁止字符串拼接。
8. **diff 返回的 changes 按 `version` 升序**；分页语义见 §4.2。
9. **`todo_add_note` 不更新 `todos.version`**，不影响 diff 流。**`notes` 表不参与 diff**——是只读追加的思考留痕通道，通过 `todo_get_notes` 显式读取。
10. **`parent_id` 在创建时确定，禁止修改**。
11. **Batch insert 按 markdown 缩进 DFS 序写入**（parent → child 自然成立），不依赖 `defer_foreign_keys`；输入语法不允许构造环。

---

## 10. v1 范围之外（明确不做）

| 功能 | 推迟原因 |
|---|---|
| 全文搜索 | 后期可加 FTS5 virtual table，不破坏 schema |
| 软删除/恢复 | tombstone 已表达删除事件，恢复操作 YAGNI |
| tags | 优先级 + 难度已覆盖大多数分类需求；标签易膨胀难管理 |
| markdown 行内元信息（如 `!high #hard @2d`） | 解析复杂、与内容字面量歧义；元信息走 API 即可 |
| 提醒/通知（基于 due_at 的主动提醒） | agent 不是 cron，主动提醒由调用方实现 |
| 多用户 / 权限 | 单机本地不需要 |
| 远程同步 | 本项目明确单机定位 |
| Web UI | 纯后端，命令行 + MCP 工具足够 |
| 任务依赖 (depends_on) | parent_id 已能表达 90% 场景 |
| 周期性清理 deletions | v1 不清理（数据小），v2 加保留 7 天策略 |

---

## 11. 验收标准

**功能**：

- 单二进制启动，stdio 模式跑通 MCP handshake
- 7 个工具全部可用，2 种输入格式（markdown / 纯文本）都能写入
- markdown 解析支持任意深度缩进父子
- `todo_write` 的 `parent_id` / `priority` / `difficulty` / `due_at` 入参能整批应用
- `todo_update` 单条改 `priority` / `difficulty` / `due_at` 生效；`due_at=0` 清除截止
- `todo_list` 分页正确（`limit / offset / has_more`），upsert 项含 `notes_count`
- `todo_list` 的 `sort=created|priority|due` 三种排序均符合预期（NULL due_at 排最后）
- compact 视图仅在非默认值时展示元信息标记
- `todo_delete` 同时支持 `ids` 和 `scope` 两种模式
- diff 分页正确（`has_more` / `next_cursor` 语义一致）
- diff 能正确反映 upsert + delete + cascade delete（子任务 tombstone 不丢），且**不包含 notes 变更**
- list 默认按创建时间稳定排序，应用层 DFS 构树后输出 markdown 缩进
- 所有非法输入返回明确错误码（§4.5）
- `skill/SKILL.md` 完成：含 cursor 持久化策略、view 选择、note 用法、scope 命名建议、`parent_id` 入参用法

**并发与正确性**：

- **自动化压测脚本** `scripts/stress_concurrent.sh` 纳入 CI：3 个进程对同一 db 持续读写 60s，断言 0 错误、0 数据丢失、version 全程单调
- 删除父任务后，子任务的 delete 事件能在 diff 中被消费（验证 `recursive_triggers=ON` 生效）

**性能（本地 SSD）**：

- 单条 write p99 < 5 ms
- `todo_diff(limit=50)` p99 < 10 ms
- 100 万行数据下 list / diff 不退化（依赖索引命中）

**测试**：

- 单元测试覆盖 store / parser / validate / shortid
- 端到端测试覆盖 MCP handshake + 7 个工具的正常路径与错误路径
- 并发压测作为 CI 必需任务（见上）

---

## 12. 实现细节决策（开工前固化）

### 12.1 MCP SDK 版本策略

- 锁定 `github.com/modelcontextprotocol/go-sdk` 当前最新 stable tag（写入 `go.mod`），不跟主线
- 升级 SDK 须作为独立 PR 评审，CI 跑全部 e2e 通过才合并

### 12.2 inputSchema 设计（用 SDK 自动推导，非手写 JSON）

调研 `go-sdk@v1.6.0` 后采用其推荐方式：**每个工具定义一个 Go struct，用 `mcp.AddTool` 泛型从 struct + `jsonschema` tag 自动生成 schema**。简单字段直接 tag 描述，复杂约束（enum / pattern / default）用 `jsonschema.For[T](nil)` 生成后手工 patch。

```go
type writeArgs struct {
    Items      string `json:"items"                 jsonschema:"markdown checklist 或纯文本，每行一个任务"`
    Scope      string `json:"scope,omitempty"       jsonschema:"scope 名称，默认 default"`
    ParentID   string `json:"parent_id,omitempty"   jsonschema:"挂载到已有父任务 id"`
    Priority   string `json:"priority,omitempty"    jsonschema:"low|medium|high|urgent，默认 medium"`
    Difficulty string `json:"difficulty,omitempty"  jsonschema:"trivial|easy|medium|hard，默认 medium"`
    DueAt      any    `json:"due_at,omitempty"      jsonschema:"ISO8601 字符串或 unix ms 整数，见 §12.4"`
}

// 复杂约束 patch 示例
schema, _ := jsonschema.For[writeArgs](nil)
schema.Properties["scope"].Pattern    = `^[a-zA-Z0-9_./:-]{1,64}$`
schema.Properties["scope"].Default    = json.RawMessage(`"default"`)
schema.Properties["priority"].Enum    = []any{"low","medium","high","urgent"}
schema.Properties["priority"].Default = json.RawMessage(`"medium"`)
// ...
mcp.AddTool(server, &mcp.Tool{Name: "todo_write", InputSchema: schema, Description: "..."}, todoWrite)
```

**好处**：
- 类型安全：handler 直接拿到强类型 args，无需 unmarshal map
- 描述紧邻定义：改字段时描述同步改，不会过期
- 减少冗余：不再需要 `internal/mcp/schemas/*.json` + `go:embed`

**与 §6 字符校验的关系**：schema validate 拦截类型/pattern/enum/length 错误；`internal/validate` 包做语义层兜底校验（UTF-8、NUL 字节、ANSI escape 等 schema 表达不了的）。

### 12.3 错误返回 payload 结构

工具失败时返回 `isError: true` + 以下 JSON payload：

```json
{
  "code": "INVALID_INPUT",
  "message": "due_at 不是合法日期格式",
  "details": {
    "field": "due_at",
    "got": "2025-13-99",
    "expected": "ISO8601 string or unix ms integer"
  }
}
```

| 字段 | 必填 | 用途 |
|---|---|---|
| `code` | ✓ | 错误码枚举（见 §4.5），agent 用此做分支决策 |
| `message` | ✓ | 人类可读，给用户/调试看 |
| `details` | 可选 | 结构化补充（field/got/expected/...），定位问题用 |

### 12.4 `due_at` 输入/输出格式

**输入**（自动识别）：

| 格式 | 示例 | 解析 |
|---|---|---|
| ISO8601 字符串 | `"2026-01-15T23:59:59Z"` / `"2026-01-15T23:59:59+08:00"` / `"2026-01-15"` | `time.Parse` 多 layout 尝试，缺时区按本地解析后转 UTC |
| unix ms 整数 | `1768435199000` | 直接用 |
| `0` | `0` | 表示**清除截止**（仅 `todo_update` 语义） |
| `null` / 不传 | — | 不变更（update）/ 默认无截止（write） |

**db 存储**：unix ms 整数。

**输出**（list / diff）：统一 ISO8601 UTC 字符串：

```json
"due_at": "2026-01-15T23:59:59Z"
```

理由：agent 写入要自然（ISO8601 友好），输出要稳定可解析（统一 UTC 避免时区歧义）。

### 12.5 compact view 相对时间格式

`due_at` 在 compact view 渲染为相对时间标记 `@<delta>`：

| 距现在 | 渲染 | 备注 |
|---|---|---|
| 已逾期 ≥ 1h | `@overdue` | 不显示具体小时数（避免 token 浪费） |
| 已逾期 < 1h | `@<1h overdue` | 紧迫提示 |
| 0 ~ 1h | `@<1h` | 即将到期 |
| 1h ~ 24h | `@3h` | 取整小时（向下） |
| 1d ~ 7d | `@2d` | 取整天 |
| 1w ~ 4w | `@2w` | 取整周 |
| ≥ 4w | `@1mo` | 取整月（30 天） |

**时区**：按 server 进程的本地时区计算"今天"边界。db 存 UTC，渲染时按 `time.Local` 转换。

### 12.6 测试 db 策略

- 单元测试：`:memory:` SQLite，每个测试独立 db handle
- 端到端测试：`tmpdir/test-<uuid>.db`，测试结束 cleanup
- 并发压测：`tmpdir/stress.db`，3 个进程 × 60s

### 12.7 日志策略

- 写 stderr（stdout 留给 MCP 协议）
- 默认 level = `info`，可由 `TIDO_LOG_LEVEL` 环境变量调整（`debug` / `info` / `warn` / `error`）
- 格式：`<时间> <level> <message> key=value ...`，单行 logfmt 风格
- 工具调用必记一行：`tool=todo_write scope=default items_count=3 took=2ms`

---

> 此文档为最终设计稿，开工后如需重大调整须先更新本文档。
