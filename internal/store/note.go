package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AddNote 给 todo 追加一条笔记；返回 note id。
// **不更新 todos.version**（不变量 §9.9）；不在事务里走 nextVersion。
func (s *Store) AddNote(ctx context.Context, todoID, content string, nowMs int64) (int64, error) {
	tx, err := s.BeginImmediate(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := assertTodoExists(ctx, tx, todoID); err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO notes(todo_id, content, created_at) VALUES (?, ?, ?)`,
		todoID, content, nowMs)
	if err != nil {
		return 0, fmt.Errorf("insert note: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// NotesResult 是 ListNotes 的分页结果。
type NotesResult struct {
	Notes   []Note
	HasMore bool
}

// ListNotes 按时间升序拉取某 todo 的笔记，带分页。limit ≤ 100。
func (s *Store) ListNotes(ctx context.Context, todoID string, limit, offset int) (*NotesResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if err := s.assertTodoExistsRO(ctx, todoID); err != nil {
		return nil, err
	}

	q := `
SELECT id, todo_id, content, created_at
  FROM notes
  WHERE todo_id = ?
  ORDER BY id ASC
  LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, q, todoID, limit+1, offset)
	if err != nil {
		return nil, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var out []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.TodoID, &n.Content, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return &NotesResult{Notes: out, HasMore: hasMore}, nil
}

// assertTodoExistsRO 是只读版本（无事务，给 ListNotes 用）。
func (s *Store) assertTodoExistsRO(ctx context.Context, id string) error {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM todos WHERE id = ?", id).Scan(&n)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	case err != nil:
		return err
	}
	return nil
}
