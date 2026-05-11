package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/store"
)

func TestTodoWriteThenList(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	wr, _, err := svc.todoWrite(ctx, nil, TodoWriteArgs{
		Items:    "- [ ] task A\n- [x] task B\n  - [-] subtask",
		Priority: "high",
	})
	requireOK(t, wr, err)
	if wr.StructuredContent.(TodoWriteResult).Count != 3 {
		t.Errorf("count = %d, want 3", wr.StructuredContent.(TodoWriteResult).Count)
	}

	lr, _, err := svc.todoList(ctx, nil, TodoListArgs{})
	requireOK(t, lr, err)
	res := lr.StructuredContent.(TodoListResult)
	if res.Total != 3 {
		t.Errorf("total = %d, want 3", res.Total)
	}
	for _, it := range res.Items {
		if it.Priority != "high" {
			t.Errorf("priority = %s, want high", it.Priority)
		}
		if it.Scope != "" || it.Version != 0 {
			t.Errorf("compact view should hide scope/version; got %+v", it)
		}
	}
}

func TestTodoWrite_PlainText(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	wr, _, err := svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "first\nsecond\nthird"})
	requireOK(t, wr, err)
	if wr.StructuredContent.(TodoWriteResult).Count != 3 {
		t.Errorf("plain text 3 lines → %d todos", wr.StructuredContent.(TodoWriteResult).Count)
	}
}

func TestTodoWrite_BadDueAt(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	res, _, err := svc.todoWrite(ctx, nil, TodoWriteArgs{
		Items: "task", DueAt: "not-a-date",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for bad due_at")
	}
}

func TestTodoUpdate_StatusAndDue(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	wr, _, _ := svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "task"})
	id := wr.StructuredContent.(TodoWriteResult).IDs[0]

	ur, _, err := svc.todoUpdate(ctx, nil, TodoUpdateArgs{
		ID: id, Status: "completed", DueAt: "1735689600000",
	})
	requireOK(t, ur, err)

	lr, _, _ := svc.todoList(ctx, nil, TodoListArgs{View: "full"})
	got := lr.StructuredContent.(TodoListResult).Items[0]
	if got.Status != "completed" {
		t.Errorf("status = %s, want completed", got.Status)
	}
	if got.DueAt == "" {
		t.Error("DueAt should be set")
	}
}

func TestTodoUpdate_ClearDue(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	wr, _, _ := svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "task", DueAt: "1735689600000"})
	id := wr.StructuredContent.(TodoWriteResult).IDs[0]

	ur, _, err := svc.todoUpdate(ctx, nil, TodoUpdateArgs{ID: id, ClearDueAt: true})
	requireOK(t, ur, err)

	lr, _, _ := svc.todoList(ctx, nil, TodoListArgs{View: "full"})
	if lr.StructuredContent.(TodoListResult).Items[0].DueAt != "" {
		t.Error("DueAt should be empty after clear")
	}
}

func TestTodoUpdate_DueAndClearMutex(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	wr, _, _ := svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "x"})
	id := wr.StructuredContent.(TodoWriteResult).IDs[0]

	res, _, err := svc.todoUpdate(ctx, nil, TodoUpdateArgs{
		ID: id, DueAt: "1700000000000", ClearDueAt: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected mutex error")
	}
}

func TestTodoDelete_ByIDs(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	wr, _, _ := svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "a\nb\nc"})
	ids := wr.StructuredContent.(TodoWriteResult).IDs

	dr, _, err := svc.todoDelete(ctx, nil, TodoDeleteArgs{IDs: ids[:2]})
	requireOK(t, dr, err)
	if dr.StructuredContent.(TodoDeleteResult).Count != 2 {
		t.Errorf("delete count = %d, want 2", dr.StructuredContent.(TodoDeleteResult).Count)
	}
	lr, _, _ := svc.todoList(ctx, nil, TodoListArgs{})
	if lr.StructuredContent.(TodoListResult).Total != 1 {
		t.Errorf("after delete, total = %d, want 1", lr.StructuredContent.(TodoListResult).Total)
	}
}

func TestTodoDelete_Mutex(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	res, _, err := svc.todoDelete(ctx, nil, TodoDeleteArgs{
		IDs: []string{"t1"}, Scope: "default",
	})
	if err != nil || !res.IsError {
		t.Errorf("expected IsError for ids+scope, got err=%v IsError=%v", err, res.IsError)
	}
}

func TestNotesAddAndGet(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	wr, _, _ := svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "task"})
	id := wr.StructuredContent.(TodoWriteResult).IDs[0]

	for _, c := range []string{"first", "second", "third"} {
		ar, _, err := svc.todoAddNote(ctx, nil, TodoAddNoteArgs{TodoID: id, Content: c})
		requireOK(t, ar, err)
	}

	nr, _, err := svc.todoGetNotes(ctx, nil, TodoGetNotesArgs{TodoID: id})
	requireOK(t, nr, err)
	notes := nr.StructuredContent.(TodoGetNotesResult).Notes
	if len(notes) != 3 || notes[0].Content != "first" || notes[2].Content != "third" {
		t.Errorf("notes = %+v, want 3 in insertion order", notes)
	}
}

func TestDiff_Incremental(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_, _, _ = svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "first"})
	d1, _, err := svc.todoDiff(ctx, nil, TodoDiffArgs{Since: 0})
	requireOK(t, d1, err)

	cursor := d1.StructuredContent.(TodoDiffResult).NextCursor
	_, _, _ = svc.todoWrite(ctx, nil, TodoWriteArgs{Items: "second"})

	d2, _, err := svc.todoDiff(ctx, nil, TodoDiffArgs{Since: cursor})
	requireOK(t, d2, err)
	changes := d2.StructuredContent.(TodoDiffResult).Changes
	if len(changes) != 1 {
		t.Fatalf("incremental diff = %d changes, want 1", len(changes))
	}
	if changes[0].Todo.Content != "second" {
		t.Errorf("got content %q, want 'second'", changes[0].Todo.Content)
	}
}

func TestList_ParentRoot(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_, _, _ = svc.todoWrite(ctx, nil, TodoWriteArgs{
		Items: "- [ ] root\n  - [ ] child",
	})

	lr, _, err := svc.todoList(ctx, nil, TodoListArgs{ParentID: "root"})
	requireOK(t, lr, err)
	if lr.StructuredContent.(TodoListResult).Total != 1 {
		t.Errorf("root only total = %d, want 1", lr.StructuredContent.(TodoListResult).Total)
	}
}

func TestInvalidID(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	res, _, err := svc.todoUpdate(ctx, nil, TodoUpdateArgs{ID: "garbage", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError for invalid id")
	}
	text := contentText(res)
	if !strings.Contains(text, "invalid id") {
		t.Errorf("error text = %q, want to mention 'invalid id'", text)
	}
}

// --- helpers ---

func newTestService(t *testing.T) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// 固定时钟，避免 view 相对时间不稳定
	var clock int64 = 1_700_000_000_000
	return NewService(st, func() int64 {
		clock += 1000
		return clock
	})
}

func requireOK(t *testing.T, res *mcpsdk.CallToolResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError unexpectedly set: %s", contentText(res))
	}
}

// contentText 读取 CallToolResult 的第一个 TextContent。
func contentText(res *mcpsdk.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
