package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SchemaVersion 是当前代码内置的 schema 版本号。
// 升级 schema 时：递增此值 + 在 migrations 列表追加新 SQL。
const SchemaVersion = 1

// migrations 按版本顺序排列；下标 = 该版本要升级到的 SQL。
// migrations[0] 是从空库到 v1 的初始化脚本。
var migrations = []string{
	schemaV1,
}

// schemaV1 = DESIGN.md §3.1 完整定义。
// 一次性建全部表 + 索引 + trigger，单个 SQL 字符串 SQLite 能整体执行。
const schemaV1 = `
-- 全局元数据：ID 计数器 + 版本号
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value INTEGER NOT NULL
);
INSERT INTO meta(key, value) VALUES ('last_id', 0), ('version', 0);

-- 主表
CREATE TABLE todos (
  id          TEXT    PRIMARY KEY,
  scope       TEXT    NOT NULL DEFAULT 'default'
              CHECK(length(scope) BETWEEN 1 AND 64),
  content     TEXT    NOT NULL
              CHECK(length(content) BETWEEN 1 AND 4096),
  status      TEXT    NOT NULL
              CHECK(status IN ('pending','in_progress','completed','cancelled')),
  priority    TEXT    NOT NULL DEFAULT 'medium'
              CHECK(priority IN ('low','medium','high','urgent')),
  difficulty  TEXT    NOT NULL DEFAULT 'medium'
              CHECK(difficulty IN ('trivial','easy','medium','hard')),
  due_at      INTEGER,
  parent_id   TEXT    REFERENCES todos(id) ON DELETE CASCADE,
  version     INTEGER NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_todos_scope_status ON todos(scope, status);
CREATE INDEX idx_todos_scope_version ON todos(scope, version);
CREATE INDEX idx_todos_scope_parent  ON todos(scope, parent_id);

-- 留痕表：append-only
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

-- DELETE trigger：物理删 todos 行后自动写 tombstone。
-- CASCADE 删子任务时也会逐行触发（依赖 PRAGMA recursive_triggers=ON）。
CREATE TRIGGER trg_todos_after_delete
AFTER DELETE ON todos
FOR EACH ROW
BEGIN
  INSERT OR IGNORE INTO deletions(todo_id, scope, version, deleted_at)
  VALUES (
    OLD.id,
    OLD.scope,
    (SELECT value FROM meta WHERE key = 'version'),
    CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)
  );
END;
`

// Migrate 检查 user_version 并按需升级 schema 至 SchemaVersion。
// 三种分支：
//   - v == 0          → 新库，初始化
//   - v < SchemaVer   → 老库，按版本升级（v1 暂无升级路径）
//   - v == SchemaVer  → 已是最新
//   - v > SchemaVer   → 拒绝启动（旧二进制不兼容新库）
func Migrate(ctx context.Context, db *sql.DB) error {
	v, err := readUserVersion(ctx, db)
	if err != nil {
		return err
	}

	switch {
	case v == SchemaVersion:
		return nil
	case v > SchemaVersion:
		return fmt.Errorf("db schema v%d > code v%d, refusing to start (binary too old)", v, SchemaVersion)
	}

	for cur := v; cur < SchemaVersion; cur++ {
		if err := applyMigration(ctx, db, cur); err != nil {
			return fmt.Errorf("apply migration v%d -> v%d: %w", cur, cur+1, err)
		}
	}
	return setUserVersion(ctx, db, SchemaVersion)
}

// applyMigration 在事务内执行一次升级（cur → cur+1）。
func applyMigration(ctx context.Context, db *sql.DB, cur int) error {
	if cur >= len(migrations) {
		return fmt.Errorf("no migration script for v%d -> v%d", cur, cur+1)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, migrations[cur]); err != nil {
		return err
	}
	return tx.Commit()
}

func readUserVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return v, nil
}

// setUserVersion 用 fmt 拼 SQL（PRAGMA 不支持参数化），v 是 int 安全。
func setUserVersion(ctx context.Context, db *sql.DB, v int) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}
