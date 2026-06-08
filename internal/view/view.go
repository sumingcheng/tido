// Package view 把 store 实体渲染为 MCP 工具返回的 JSON 视图。
// 区分 compact / full：compact 省元字段 + 相对时间，full 全字段 + ISO8601。
// 设计依据：DESIGN.md §12.5。
package view

import (
	"time"

	"github.com/sumingcheng/tido/internal/store"
)

// Mode 是视图模式。
type Mode string

const (
	ModeCompact Mode = "compact"
	ModeFull    Mode = "full"
)

// TodoView 是单条 todo 的输出 DTO。
// 字段用 omitempty 控制 compact / full 差异：compact 模式不填的字段会被裁掉。
type TodoView struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	Priority   string `json:"priority,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	DueAt      string `json:"due_at,omitempty"`
	ParentID   string `json:"parent_id,omitempty"`
	Notes      int    `json:"notes,omitempty"`

	// full 专属
	Scope     string `json:"scope,omitempty"`
	Version   int64  `json:"version,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RenderTodo 按 mode 把单条 store.Todo 渲染为 TodoView。
// nowMs 用于 compact 下的相对时间渲染（agent 友好）。
func RenderTodo(t store.Todo, mode Mode, nowMs int64) TodoView {
	v := TodoView{
		ID:      t.ID,
		Content: t.Content,
		Status:  string(t.Status),
		Notes:   t.NotesCount,
	}
	if t.ParentID != nil {
		v.ParentID = *t.ParentID
	}

	switch mode {
	case ModeCompact:
		if t.Priority != "" && t.Priority != store.PriorityMedium {
			v.Priority = string(t.Priority)
		}
		if t.Difficulty != "" && t.Difficulty != store.DifficultyMedium {
			v.Difficulty = string(t.Difficulty)
		}
		if t.DueAt != nil {
			v.DueAt = relativeDue(*t.DueAt, nowMs)
		}
	default: // ModeFull
		v.Priority = string(t.Priority)
		v.Difficulty = string(t.Difficulty)
		if t.DueAt != nil {
			v.DueAt = msToISO(*t.DueAt)
		}
		v.Scope = t.Scope
		v.Version = t.Version
		v.CreatedAt = msToISO(t.CreatedAt)
		v.UpdatedAt = msToISO(t.UpdatedAt)
	}
	return v
}

// RenderTodos 批量渲染。
func RenderTodos(items []store.Todo, mode Mode, nowMs int64) []TodoView {
	out := make([]TodoView, len(items))
	for i, t := range items {
		out[i] = RenderTodo(t, mode, nowMs)
	}
	return out
}

// ChangeView 是 diff 单项的输出 DTO（compact-only，diff 总是给 agent 用）。
type ChangeView struct {
	Op   string   `json:"op"` // upsert | delete
	Todo TodoView `json:"todo"`
}

// RenderChanges 把 store.Change 列表渲染为 compact 视图。
func RenderChanges(changes []store.Change, nowMs int64) []ChangeView {
	out := make([]ChangeView, len(changes))
	for i, c := range changes {
		out[i] = ChangeView{
			Op:   string(c.Op),
			Todo: RenderTodo(c.Todo, ModeCompact, nowMs),
		}
	}
	return out
}

// NoteView 是 note 输出 DTO。
type NoteView struct {
	ID        int64  `json:"id"`
	TodoID    string `json:"todo_id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// RenderNote 渲染单条 note（始终用 ISO8601）。
func RenderNote(n store.Note) NoteView {
	return NoteView{
		ID:        n.ID,
		TodoID:    n.TodoID,
		Content:   n.Content,
		CreatedAt: msToISO(n.CreatedAt),
	}
}

// RenderNotes 批量渲染。
func RenderNotes(ns []store.Note) []NoteView {
	out := make([]NoteView, len(ns))
	for i, n := range ns {
		out[i] = RenderNote(n)
	}
	return out
}

// msToISO 把 unix ms 转 RFC3339（UTC）。
func msToISO(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
