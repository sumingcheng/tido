package store

import (
	"context"
	"database/sql"
	"fmt"
)

// DiffResult 是 todo_diff 的返回。
type DiffResult struct {
	Changes    []Change
	NextCursor int64 // 下次调用应传入的 since 值
	HasMore    bool  // true → next_cursor 为本批最后一条 version；false → 当前 meta.version
}

// Diff 返回 scope 下 version > since 的所有变更（含 deletions tombstone）。
// 一次最多返回 limit 条（≤ 200，默认 50）。
func (s *Store) Diff(ctx context.Context, scope string, since int64, limit int) (*DiffResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	curVer, err := s.currentVersion(ctx)
	if err != nil {
		return nil, err
	}
	if since >= curVer {
		return &DiffResult{NextCursor: curVer, HasMore: false}, nil
	}

	// 多取 1 行用于检测 has_more
	q := `
SELECT t.id, t.scope, t.content, t.status,
       t.priority, t.difficulty, t.due_at, t.parent_id,
       t.version, t.updated_at,
       (SELECT COUNT(*) FROM notes WHERE notes.todo_id = t.id) AS notes_count,
       'upsert' AS op
  FROM todos t WHERE t.scope = ? AND t.version > ?
UNION ALL
SELECT todo_id, scope, '', '',
       '', '', NULL, NULL,
       version, deleted_at,
       0 AS notes_count,
       'delete' AS op
  FROM deletions WHERE scope = ? AND version > ?
ORDER BY version, id
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, scope, since, scope, since, limit+1)
	if err != nil {
		return nil, fmt.Errorf("diff query: %w", err)
	}

	changes, err := scanChanges(rows)
	if err != nil {
		return nil, err
	}

	hasMore := len(changes) > limit
	if hasMore {
		changes = changes[:limit]
	}

	next := curVer
	if hasMore && len(changes) > 0 {
		next = changes[len(changes)-1].Todo.Version
	}
	return &DiffResult{
		Changes:    changes,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// currentVersion 读 meta.version 当前值（用于 diff next_cursor 兜底）。
func (s *Store) currentVersion(ctx context.Context) (int64, error) {
	var v int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = 'version'`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read meta.version: %w", err)
	}
	return v, nil
}

// scanChanges 解析 UNION ALL 的结果（upsert + delete 共用列定义）。
func scanChanges(rows *sql.Rows) ([]Change, error) {
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var (
			c       Change
			due     sql.NullInt64
			parent  sql.NullString
			noteCnt int
			op      string
		)
		err := rows.Scan(
			&c.Todo.ID, &c.Todo.Scope, &c.Todo.Content, &c.Todo.Status,
			&c.Todo.Priority, &c.Todo.Difficulty, &due, &parent,
			&c.Todo.Version, &c.Todo.UpdatedAt,
			&noteCnt, &op,
		)
		if err != nil {
			return nil, err
		}
		if due.Valid {
			d := due.Int64
			c.Todo.DueAt = &d
		}
		if parent.Valid {
			p := parent.String
			c.Todo.ParentID = &p
		}
		c.Todo.NotesCount = noteCnt
		c.Op = ChangeOp(op)
		out = append(out, c)
	}
	return out, rows.Err()
}
