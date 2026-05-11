---
name: tido
description: Personal todo list for AI agents using markdown files. Use when the user asks you to track tasks, plan multi-step work, remember progress across sessions, resume interrupted work, or coordinate with other agents on shared todos. Tasks live in `~/.tido/<scope>.md`, completed items archived to `~/.tido/<scope>.archive.md`.
---

# Tido — AI Agent 的待办本

> 用 markdown 文件管理 todo，让 AI 在长任务里**不忘事、不重复干、可接手别人留下的活**。

---

## 何时使用

**该用**：

- 用户给你一个多步任务（≥ 3 步），需要跟踪进度
- 用户说"记一下"、"加个 todo"、"todo list"、"计划一下"
- 长会话中你自己规划了一串 plan，需要持久化（防上下文丢失）
- 接手用户/其他 agent 留下的工作，需要看现状

**不该用**：

- 单步任务（直接做完即可，不必记）
- 临时想法（< 5 分钟就能解决的）
- 用户只是闲聊或问问题

---

## 数据位置

| 文件 | 用途 |
|---|---|
| `~/.tido/<scope>.md` | 活跃任务（pending / in_progress） |
| `~/.tido/<scope>.archive.md` | 已完成 / 已取消（归档） |

**scope 命名**：

- 默认 `default`，用户没指定就用这个
- 强烈建议按项目/任务上下文命名，例如：`bugfix-login`、`refactor-auth-2026-01`、`daily`
- 字符限制：`[a-zA-Z0-9_./:-]`，长度 1-64
- kebab-case 优先（避免空格、避免大写）

**第一次写之前**：

```bash
mkdir -p ~/.tido
```

文件不存在就创建一个空文件再写。

---

## 文件格式（必须严格遵守）

### 单行任务的完整结构

```
- [STATUS] [PRIORITY?] [DIFFICULTY?] [DUE_AT?] CONTENT
```

| 部分 | 取值 | 默认 |
|---|---|---|
| `[STATUS]` | `[ ]` pending / `[x]` completed / `[-]` in_progress / `[~]` cancelled | 必填 |
| `PRIORITY` | `!low` / `!medium` / `!high` / `!urgent` | medium 时省略 |
| `DIFFICULTY` | `#trivial` / `#easy` / `#medium` / `#hard` | medium 时省略 |
| `DUE_AT` | `@YYYY-MM-DD` 或 `@YYYY-MM-DD HH:mm` | 无截止时省略 |
| `CONTENT` | 任务描述（一行内），不含换行 | 必填 |

**示例**：

```markdown
- [ ] 改个文档 typo
- [-] !high 重构 store 模块
- [ ] !urgent #hard @2026-01-16 修复登录无法跳转 bug
- [x] !low #easy @2026-01-10 写单元测试
- [~] 接入 Sentry（取消：暂缓）
```

**严格要求**：marker 必须按 `! → # → @` 顺序出现，便于人和 agent 一眼识别。多个相同类型 marker 不允许（如 `!high !urgent` 非法）。

### 子任务（缩进 2 空格 = 下一级）

```markdown
- [-] 重构 store 模块
  - [x] 设计草案
  - [ ] 抽出 todo.go
  - [ ] 抽出 note.go
    - [ ] 移动 CRUD
    - [ ] 移动 schema migration
```

任意深度，每级 +2 空格。父任务的 status 由 agent 自行推断（通常子任务全完成 → 父也完成）。

### 笔记（思考留痕、失败原因）

笔记附在任务下方，**缩进 2 空格 + `>` 引用块**：

```markdown
- [-] !high 修复登录 bug
  > 2026-01-15 14:32 — cookie domain 配错，已找到根因
  > 2026-01-15 15:01 — chrome 修好，safari 仍有问题
  - [ ] safari 兼容性 fallback
```

**笔记 vs 子任务的视觉区分**：
- `>` 开头 = 笔记（人/agent 的注释）
- `-` 开头 = 子任务

笔记格式建议：`> YYYY-MM-DD HH:MM — 内容`，时间用本地时区即可。

---

## 6 个核心操作

### 1. 创建任务

1. 读 `~/.tido/<scope>.md`（不存在则当空文件处理）
2. **追加**新行到文件末尾（不要插到中间，破坏其他 agent 的视图稳定性）
3. 写回文件

```markdown
# 追加示例
- [ ] !high 新加的紧急任务
- [ ] 普通任务
```

### 2. 查看清单

1. 读 `~/.tido/<scope>.md`
2. 内存里按这个顺序排序展示给用户：
   - 先按 status：`in_progress` → `pending`（不显示 completed/cancelled）
   - 再按 priority：`urgent` → `high` → `medium` → `low`
   - 再按 due_at：早的在前（NULL 排最后）
   - 最后按文件中的出现顺序

**展示给用户时**：默认输出原始 markdown（用户看得懂），不要二次渲染成表格。

### 3. 改状态

1. 读文件
2. 找到目标行（用 content 前缀匹配）
3. 把 `[ ]` 改成新状态字符（`[x]` / `[-]` / `[~]`）
4. 写回

```markdown
# 改前
- [ ] !high 修复登录 bug

# 改后（开始做）
- [-] !high 修复登录 bug
```

### 4. 完成 → 立即归档（强约束）

任务标记 `[x]` 或 `[~]` 后，**立即**移到 archive 文件。理由：主文件保持精简，下次读全文不浪费 context。

步骤：

1. 从 `~/.tido/<scope>.md` 删除该行（含子任务和笔记，整块迁移）
2. 追加到 `~/.tido/<scope>.archive.md`，每月一个二级标题分组：

```markdown
## 2026-01

- [x] !high 修复登录 bug
  > 2026-01-15 — 原因 cookie domain
- [x] 改文档 typo

## 2025-12

- [x] 上个月的事
```

如果 archive 文件不存在，创建并加月份标题。

### 5. 删除（彻底删除，不归档）

只有用户**显式说"删掉"**才删，且不进归档（用户不要的事情真的就消失）。否则一律归档。

### 6. 加笔记

任务执行过程遇到任何值得记录的事（失败原因、关键决策、改主意），**加笔记**：

1. 读文件
2. 找到目标任务行
3. 在它下一行（缩进 2 空格 + `>`）插入笔记

```markdown
- [-] 重构 store
  > 2026-01-15 14:32 — 决定用 sqlx 而非 ent，简单
  > 2026-01-15 15:30 — sqlx 不支持 returning，回到原方案
```

笔记**只增不删**，是这个任务的历史。

---

## 多 agent 并发：脏写风险

**风险**：如果两个 agent 同时改同一个文件，后写覆盖前写 → 任务丢失。

**风险有多大**：日常使用极低（不同 agent 通常处理不同 scope）。但要知道这个风险。

**降低风险的简单做法**：

1. **写之前再读一次**：准备写入前，重新读一次文件，确认你内存中的版本和磁盘一致。如果不一致，把别人的修改合并进来再写。
2. **细粒度修改**：只追加（创建任务）比"读全文-改某行-写全文"安全得多。如果只是创建任务，**直接 append 一行**，不重写整个文件。
3. **scope 隔离**：建议用户给不同任务用不同 scope。物理隔离 = 零冲突。

**绝不要做**：

- 不要在写入前读了之后中间停顿很久（context 切换、长链推理），增加冲突窗口
- 不要清空整个文件再写——一旦中途失败，所有 todo 全没

---

## scope 使用建议

| 场景 | 建议 scope |
|---|---|
| 用户没指定，闲聊式 todo | `default` |
| 一个独立项目的工作 | 项目名，如 `tido`、`auth-refactor` |
| 一次性密集任务 | 任务名 + 日期，如 `bugfix-2026-01-15` |
| 长期 daily | `daily` |

**切 scope 的判断**：用户讲到一个新的、与当前 scope 明显不同的工作主题时，问一句"这条记到 `<推荐 scope>` 吗？还是 `default`？"。

---

## 完整示例：一个工作日的 `default.md`

```markdown
- [-] !high 重构 store 模块
  > 2026-01-15 09:30 — 设计完成，开始拆
  - [x] 设计草案
  - [ ] 抽出 todo.go
  - [ ] 抽出 note.go
- [ ] !urgent @2026-01-16 修复登录无法跳转 bug
- [ ] #easy 改 README 里的几个 typo
- [ ] !low #hard 调研接入 Sentry 的方案
- [x] 回老板邮件
```

agent 调 `查看清单` 时，渲染顺序应是：

```
[in_progress + urgent/high]
- [-] !high 重构 store 模块
- [ ] !urgent @2026-01-16 修复登录无法跳转 bug

[pending]
- [ ] #easy 改 README 里的几个 typo
- [ ] !low #hard 调研接入 Sentry 的方案
```

`[x] 回老板邮件` 应在你下次操作时立即归档（按规则 4）。

---

## 关键不变量（agent 必须遵守）

1. **markdown 行格式严格按 `[STATUS] [!P] [#D] [@DUE] CONTENT` 顺序**，agent 不可自创变体
2. **缩进 2 空格 = 下一级父子**，不要用 4 空格或 tab
3. **完成/取消任务立即归档**，不在主文件留 `[x]` 和 `[~]`
4. **创建任务用追加（append）**，不重写整个文件
5. **删除 = 用户显式要求**，否则一律走归档
6. **笔记只增不改不删**，是任务的历史
7. **scope 物理隔离**，不在一个文件里塞多个项目的 todo
