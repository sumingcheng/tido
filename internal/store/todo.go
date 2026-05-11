package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/sumingcheng/tido/internal/parser"
	"github.com/sumingcheng/tido/internal/shortid"
)

// 公共错误。
var (
	ErrNotFound        = errors.New("todo not found")
	ErrEmptyUpdate     = errors.New("no fields to update")
	ErrInvalidParent   = errors.New("invalid parent_id")
	ErrInvalidIndent   = errors.New("invalid markdown indent (depth jumped)")
	ErrIDsScopeBoth    = errors.New("must provide ids OR scope, not both")
	ErrIDsScopeNeither = errors.New("must provide ids OR scope")
)

// InsertOptions 是 InsertBatch 的非内容字段配置。
type InsertOptions struct {
	Scope        string
	RootParentID string // 整批挂载到此父任务下；空字符串 = 顶层
	Priority     Priority
	Difficulty   Difficulty
	DueAt        *int64 // nil = 无截止
	NowMs        int64  // created_at = updated_at
}

// InsertBatch 在一个事务内批量插入 items（含父子关系），返回新生成的短码 ids（与 items 一一对应）。
// items 须按 parser DFS 序排列（parent → child 自然成立）。
func (s *Store) InsertBatch(ctx context.Context, items []parser.Item, opts InsertOptions) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	tx, err := s.BeginImmediate(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if opts.RootParentID != "" {
		if err := assertTodoExists(ctx, tx, opts.RootParentID); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidParent, opts.RootParentID)
		}
	}

	ver, err := nextVersion(ctx, tx)
	if err != nil {
		return nil, err
	}
	ids, err := allocateIDs(ctx, tx, len(items))
	if err != nil {
		return nil, err
	}

	parents, err := resolveParents(items, ids, opts.RootParentID)
	if err != nil {
		return nil, err
	}

	if err := batchInsertTodos(ctx, tx, items, ids, parents, opts, ver); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

// resolveParents 用 lastByDepth 字典在 batch 内解析父子关系。
// 顶层（depth=0）的 parent = rootParent（rootParent 为空则 nil = 顶层）。
// 深层节点必须紧接前一层节点出现，否则 ErrInvalidIndent。
func resolveParents(items []parser.Item, ids []string, rootParent string) ([]*string, error) {
	parents := make([]*string, len(items))
	lastByDepth := map[int]string{}

	for i, it := range items {
		switch {
		case it.Depth == 0:
			if rootParent != "" {
				p := rootParent
				parents[i] = &p
			}
		default:
			parent, ok := lastByDepth[it.Depth-1]
			if !ok {
				return nil, fmt.Errorf("%w: item %d at depth %d has no parent at depth %d",
					ErrInvalidIndent, i, it.Depth, it.Depth-1)
			}
			p := parent
			parents[i] = &p
		}
		lastByDepth[it.Depth] = ids[i]
		// 清理更深层级（新兄弟出现 → 老的孙子失效）
		for d := range lastByDepth {
			if d > it.Depth {
				delete(lastByDepth, d)
			}
		}
	}
	return parents, nil
}

// batchInsertTodos 用单条多行 VALUES 批量插入。
func batchInsertTodos(ctx context.Context, tx *sql.Tx, items []parser.Item,
	ids []string, parents []*string, opts InsertOptions, ver int64,
) error {
	const cols = `(id, scope, content, status, priority, difficulty, due_at,
                   parent_id, version, created_at, updated_at)`
	placeholders := strings.TrimRight(strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?),", len(items)), ",")
	args := make([]any, 0, len(items)*11)
	for i, it := range items {
		// Status 兜底：parser 不会输出空，但调用方/测试可省略
		status := it.Status
		if status == "" {
			status = string(StatusPending)
		}
		args = append(args,
			ids[i], opts.Scope, it.Content, status,
			string(opts.Priority), string(opts.Difficulty), nullableInt64(opts.DueAt),
			nullableStringPtr(parents[i]), ver, opts.NowMs, opts.NowMs,
		)
	}
	q := "INSERT INTO todos " + cols + " VALUES " + placeholders
	_, err := tx.ExecContext(ctx, q, args...)
	return err
}

// ListOptions 是 List 的过滤/排序/分页参数。
type ListOptions struct {
	Scope    string
	Status   Status  // 空 = 不过滤
	ParentID *string // nil = 不过滤；指向 "" = 仅顶层 (IS NULL)；其他 = = 值
	Sort     SortOrder
	Limit    int // ≤ 500，默认 100
	Offset   int
}

// ListResult 包含分页元信息。
type ListResult struct {
	Items   []Todo
	Total   int  // 满足过滤的总数（不受 Limit/Offset 影响）
	HasMore bool // 还有下一页
}

// List 按过滤条件返回 todos（含 NotesCount）。
func (s *Store) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}

	where, whereArgs := buildListWhere(opts)
	orderBy := buildOrderBy(opts.Sort)

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM todos "+where, whereArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}

	q := `
SELECT id, scope, content, status, priority, difficulty, due_at, parent_id,
       version, created_at, updated_at,
       (SELECT COUNT(*) FROM notes WHERE notes.todo_id = todos.id) AS notes_count
  FROM todos ` + where + ` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`

	args := append(append([]any{}, whereArgs...), opts.Limit, opts.Offset)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	items, err := scanTodos(rows)
	if err != nil {
		return nil, err
	}
	return &ListResult{
		Items:   items,
		Total:   total,
		HasMore: opts.Offset+len(items) < total,
	}, nil
}

// buildListWhere 拼出 WHERE 子句与参数。
func buildListWhere(opts ListOptions) (string, []any) {
	clauses := []string{"scope = ?"}
	args := []any{opts.Scope}

	if opts.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, string(opts.Status))
	}
	if opts.ParentID != nil {
		switch *opts.ParentID {
		case "":
			clauses = append(clauses, "parent_id IS NULL")
		default:
			clauses = append(clauses, "parent_id = ?")
			args = append(args, *opts.ParentID)
		}
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// buildOrderBy 按 sort 选择 ORDER BY 子句（DESIGN.md §4.4）。
func buildOrderBy(s SortOrder) string {
	switch s {
	case SortByPriority:
		return `CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1
                              WHEN 'medium' THEN 2 WHEN 'low' THEN 3 END ASC,
                CASE WHEN due_at IS NULL THEN 1 ELSE 0 END ASC,
                due_at ASC, created_at ASC, id ASC`
	case SortByDue:
		return `CASE WHEN due_at IS NULL THEN 1 ELSE 0 END ASC,
                due_at ASC, created_at ASC, id ASC`
	default:
		return "created_at ASC, id ASC"
	}
}

// UpdateFields 是 todo_update 的可选字段。
// DueAt: nil = 不变更；指向 0 = 清除截止；其他 = 设为该值。
type UpdateFields struct {
	Status     *Status
	Content    *string
	Priority   *Priority
	Difficulty *Difficulty
	DueAt      *int64
}

// Update 修改单条 todo；至少传一个字段；id 不存在返回 ErrNotFound。
func (s *Store) Update(ctx context.Context, id string, f UpdateFields, nowMs int64) error {
	sets, args := buildUpdateSets(f)
	if len(sets) == 0 {
		return ErrEmptyUpdate
	}

	tx, err := s.BeginImmediate(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := assertTodoExists(ctx, tx, id); err != nil {
		return err
	}
	ver, err := nextVersion(ctx, tx)
	if err != nil {
		return err
	}

	sets = append(sets, "version = ?", "updated_at = ?")
	args = append(args, ver, nowMs, id)
	q := "UPDATE todos SET " + strings.Join(sets, ", ") + " WHERE id = ?"

	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	return tx.Commit()
}

// buildUpdateSets 把 UpdateFields 翻译为 SQL SET 片段。
// DueAt 特殊语义：*int64(0) → SET due_at = NULL（清除截止）。
func buildUpdateSets(f UpdateFields) ([]string, []any) {
	var sets []string
	var args []any
	if f.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, string(*f.Status))
	}
	if f.Content != nil {
		sets = append(sets, "content = ?")
		args = append(args, *f.Content)
	}
	if f.Priority != nil {
		sets = append(sets, "priority = ?")
		args = append(args, string(*f.Priority))
	}
	if f.Difficulty != nil {
		sets = append(sets, "difficulty = ?")
		args = append(args, string(*f.Difficulty))
	}
	if f.DueAt != nil {
		switch *f.DueAt {
		case 0:
			sets = append(sets, "due_at = NULL")
		default:
			sets = append(sets, "due_at = ?")
			args = append(args, *f.DueAt)
		}
	}
	return sets, args
}

// DeleteByIDs 物理删除指定 ids；trigger 自动写 tombstone。返回实际删除的 ids。
func (s *Store) DeleteByIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.deleteByCondition(ctx,
		"id IN ("+placeholders(len(ids))+")",
		toAnySlice(ids))
}

// DeleteByScope 物理删除整个 scope 的所有 todos。返回实际删除的 ids。
func (s *Store) DeleteByScope(ctx context.Context, scope string) ([]string, error) {
	if scope == "" {
		return nil, ErrIDsScopeNeither
	}
	return s.deleteByCondition(ctx, "scope = ?", []any{scope})
}

// deleteByCondition 通用删除入口：先 SELECT 实际命中的 ids，再 +1 version 后 DELETE。
// 返回的 ids 与 trigger 生成的 deletions 一一对应。
func (s *Store) deleteByCondition(ctx context.Context, whereCond string, args []any) ([]string, error) {
	tx, err := s.BeginImmediate(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 先查命中的 ids（含 cascade 时的子 ids 不会被 SELECT 看到——SELECT 只匹配条件）
	// 但 trigger 会对 cascade 删除的每行触发，所以 deletions 表里会有完整记录
	rows, err := tx.QueryContext(ctx, "SELECT id FROM todos WHERE "+whereCond, args...)
	if err != nil {
		return nil, fmt.Errorf("select ids: %w", err)
	}
	deleted, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	if len(deleted) == 0 {
		return nil, tx.Commit()
	}

	// 在 DELETE 前 +1 version，让 trigger 读到最新值并写入 deletions 行
	if _, err := nextVersion(ctx, tx); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM todos WHERE "+whereCond, args...); err != nil {
		return nil, fmt.Errorf("delete: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return deleted, nil
}

// nextVersion 在事务内递增 meta.version 并返回新值。
func nextVersion(ctx context.Context, tx *sql.Tx) (int64, error) {
	var v int64
	err := tx.QueryRowContext(ctx,
		`UPDATE meta SET value = value + 1 WHERE key='version' RETURNING value`).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("alloc version: %w", err)
	}
	return v, nil
}

// allocateIDs 在事务内批量分配 N 个短码（last_id += N）。
func allocateIDs(ctx context.Context, tx *sql.Tx, n int) ([]string, error) {
	var lastID int64
	err := tx.QueryRowContext(ctx,
		`UPDATE meta SET value = value + ? WHERE key='last_id' RETURNING value`, n,
	).Scan(&lastID)
	if err != nil {
		return nil, fmt.Errorf("alloc ids: %w", err)
	}
	first := lastID - int64(n) + 1
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = shortid.Encode(first + int64(i))
	}
	return out, nil
}

// assertTodoExists 校验 id 在 db 中存在；不存在返回 ErrNotFound。
func assertTodoExists(ctx context.Context, tx *sql.Tx, id string) error {
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM todos WHERE id = ?", id).Scan(&n); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return err
	}
	return nil
}

// --- 通用 helper ---

func placeholders(n int) string {
	if n == 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func nullableInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableStringPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func scanTodos(rows *sql.Rows) ([]Todo, error) {
	defer rows.Close()
	var out []Todo
	for rows.Next() {
		var (
			t        Todo
			due      sql.NullInt64
			parent   sql.NullString
			noteCnt  int
		)
		if err := rows.Scan(
			&t.ID, &t.Scope, &t.Content, &t.Status,
			&t.Priority, &t.Difficulty, &due, &parent,
			&t.Version, &t.CreatedAt, &t.UpdatedAt, &noteCnt,
		); err != nil {
			return nil, err
		}
		if due.Valid {
			d := due.Int64
			t.DueAt = &d
		}
		if parent.Valid {
			p := parent.String
			t.ParentID = &p
		}
		t.NotesCount = noteCnt
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanIDs(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
