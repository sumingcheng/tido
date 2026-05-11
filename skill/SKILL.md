---
name: tido
description: 通过 tido MCP server 在本地维护待办列表，支持多 agent 并发、增量同步、上下文压缩。任何 ≥3 步的任务、需要跨会话/跨 agent 协作的场景、或想给任务挂笔记/堆栈/决策时使用。
---

# Tido — Agent 的待办本

`tido` 是个本地 MCP server，提供 7 个工具把待办、笔记、增量变更暴露给 LLM/agent。基于 SQLite，单进程内多 agent 安全并发。

## 何时使用

- 多步任务（≥3 步）追踪与状态同步
- 多个 agent 同读同写同一份待办（设计/实现/测试分工）
- 长会话上下文压缩：用 `todo_diff` 拿增量，避免每次 `todo_list` 全表
- 给单个任务挂决策记录、堆栈、错误信息
- 跨会话恢复：重新打开 agent 时 `todo_list` 拿到之前的进度

**不要**用于：单步任务、临时计数器、不需要持久化的瞬时状态。

## 7 个工具速查

| 工具 | 作用 | 必需参数 |
| --- | --- | --- |
| `todo_write` | 批量写入 | `items` |
| `todo_list` | 查询列表 | — |
| `todo_update` | 单条更新 | `id` + 至少一个变更字段 |
| `todo_delete` | 删除 | `ids` 或 `scope`（互斥） |
| `todo_add_note` | 追加笔记 | `todo_id`, `content` |
| `todo_get_notes` | 拉取笔记 | `todo_id` |
| `todo_diff` | 增量变更 | `since` |

## 标准工作流

### 1. 任务开始 → 一次写入计划

`todo_write` 接受 markdown checklist 或纯文本。markdown 缩进 2 空格表达父子层级。

```jsonc
todo_write({
  "items": "- [ ] 调研方案\n- [ ] 实现\n  - [ ] 模块 A\n  - [ ] 模块 B\n- [ ] 写测试",
  "priority": "high"
})
// → { ids: ["t1","t2","t3","t4","t5"], count: 5 }
```

记下返回的 `ids`，后续更新/笔记/删除都用短码引用。

### 2. 推进任务 → 单条更新

```jsonc
todo_update({ "id": "t2", "status": "in_progress" })
todo_update({ "id": "t3", "status": "completed" })
```

关键决策落到笔记，避免污染 `content`：
```jsonc
todo_add_note({ "todo_id": "t2", "content": "选 sqlite 而非 pg：单文件部署，agent 能跨进程并发。" })
```

### 3. 多 agent 同步 → 用 diff，不要全表 list

首次 `since: 0`；后续每次传上一次返回的 `next_cursor`。`has_more: true` 时持续 diff 直到 false。

```jsonc
todo_diff({ "since": 0 })
// → { changes: [...], next_cursor: 7, has_more: false }

// 隔一会儿再来：
todo_diff({ "since": 7 })
```

`changes[].op` 是 `upsert`（新建或更新）或 `delete`（包含 cascade 删除的子任务）。

### 4. 任务结束 → 清理

```jsonc
todo_delete({ "ids": ["t3","t5"] })           // 选删
todo_delete({ "scope": "feature-x-rebuild" }) // 整批清空（不可与 ids 同时传）
```

## 字段语义

- **id**：`t` + base36，如 `t1`/`t3a`/`tabc`。短，省 token。
- **scope**：工作域；省略=`default`。用来隔离不同 epic/feature。仅同 scope 内 list/diff 互通。
- **status**：`pending` / `in_progress` / `completed` / `cancelled`
- **priority**：`low` / `medium` / `high` / `urgent`（默认 `medium`）
- **difficulty**：`trivial` / `easy` / `medium` / `hard`（默认 `medium`）
- **due_at**：截止时间。**入参**接受 unix ms 数字串或 RFC3339 字符串；**输出**统一 ISO8601（full 视图）或相对时间 `@2d`/`@overdue 1h`（compact 视图）。
- **clear_due_at**：仅 `todo_update` 用，`true` 表示清除截止时间。**与 `due_at` 互斥**。
- **parent_id**：挂载到此父任务下；通过 `todo_write({ items, parent_id: "t1" })` 给已有 todo 加子任务。
- **next_cursor**：`todo_diff` 返回，是单调递增的 version。**持久化它**，下次接着传。

## 输入格式（todo_write 的 items）

**Markdown checklist**（推荐，能表层级）：
```markdown
- [ ] pending
- [x] completed
- [-] in_progress
- [~] cancelled
- [ ] parent task
  - [ ] child task           # 缩进必须是 2 空格的整数倍
    - [x] grandchild
```

**纯文本**（每行一个 pending todo，无层级）：
```
task one
task two
task three
```

自动识别：含任一 `- [ ]` 行 → markdown；否则 → text。

## 视图模式（todo_list / todo_diff）

- **`view: "compact"`**（默认）：省略 `scope`/`version`/`created_at`/`updated_at`；`due_at` 用相对时间。**给 agent 用，token 最省**。
- **`view: "full"`**：所有字段，时间用 ISO8601。审计、迁移、人工排查时用。

## 反模式

- 每次 update 后立刻 `todo_list` 全表 → **用 `todo_diff`**
- 把堆栈/错误信息塞进 `content` → **用 `todo_add_note`**
- 同时传 `ids` 和 `scope` 给 `todo_delete` → **互斥，会报错**
- 同时传 `due_at` 和 `clear_due_at: true` → **互斥**
- 在 update 时只改一个字段就调 → 没问题，store 内部一个事务一次 version 增量

## 并发与一致性

- 所有写入走 IMMEDIATE 事务，跨 agent / 跨进程安全。
- 同 scope 内 `version` 严格单调，`todo_diff` 保证不漏不重。
- 删除是 hard delete + 自动 tombstone，`todo_diff` 能传播 `delete` 事件。
- 笔记 (`todo_add_note`) 不动 `todos.version`，**不会触发 diff**——这是设计：避免笔记刷屏。
