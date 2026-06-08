package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sumingcheng/tido/internal/parser"
)

func TestInsertBatch_Flat(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	items := []parser.Item{
		{Content: "task A", Status: "pending", Depth: 0},
		{Content: "task B", Status: "in_progress", Depth: 0},
	}
	ids, err := s.InsertBatch(ctx, items, defaultInsertOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2", len(ids))
	}
	if ids[0] != "t1" || ids[1] != "t2" {
		t.Errorf("ids = %v, want [t1 t2]", ids)
	}

	// 验证 version 为 1（首次插入）
	res, err := s.List(ctx, ListOptions{Scope: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("list count = %d, want 2", len(res.Items))
	}
	for _, todo := range res.Items {
		if todo.Version != 1 {
			t.Errorf("todo %s version = %d, want 1 (batch shares one version)", todo.ID, todo.Version)
		}
		if todo.ParentID != nil {
			t.Errorf("todo %s parent_id = %v, want nil (root)", todo.ID, *todo.ParentID)
		}
	}
}

func TestInsertBatch_Nested(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	items := []parser.Item{
		{Content: "parent", Status: "pending", Depth: 0},
		{Content: "child A", Status: "in_progress", Depth: 1},
		{Content: "child B", Status: "pending", Depth: 1},
		{Content: "grandchild", Status: "completed", Depth: 2},
		{Content: "another root", Status: "pending", Depth: 0},
	}
	ids, err := s.InsertBatch(ctx, items, defaultInsertOpts())
	if err != nil {
		t.Fatal(err)
	}

	res, _ := s.List(ctx, ListOptions{Scope: "default"})
	parentMap := map[string]*string{}
	for _, todo := range res.Items {
		parentMap[todo.ID] = todo.ParentID
	}
	expectParent(t, parentMap, ids[0], nil)
	expectParent(t, parentMap, ids[1], &ids[0])
	expectParent(t, parentMap, ids[2], &ids[0])
	expectParent(t, parentMap, ids[3], &ids[2]) // grandchild 挂在 child B 下
	expectParent(t, parentMap, ids[4], nil)
}

func TestInsertBatch_RootParent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	roots, _ := s.InsertBatch(ctx, []parser.Item{
		{Content: "epic", Status: "pending", Depth: 0},
	}, defaultInsertOpts())
	epicID := roots[0]

	opts := defaultInsertOpts()
	opts.RootParentID = epicID
	subs, err := s.InsertBatch(ctx, []parser.Item{
		{Content: "sub 1", Status: "pending", Depth: 0},
		{Content: "sub 2", Status: "pending", Depth: 0},
	}, opts)
	if err != nil {
		t.Fatal(err)
	}

	res, _ := s.List(ctx, ListOptions{Scope: "default"})
	for _, todo := range res.Items {
		if todo.ID == epicID {
			continue
		}
		if todo.ParentID == nil || *todo.ParentID != epicID {
			t.Errorf("todo %s parent = %v, want %s", todo.ID, todo.ParentID, epicID)
		}
	}
	if len(subs) != 2 {
		t.Errorf("got %d subs, want 2", len(subs))
	}
}

func TestInsertBatch_RootParentNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	opts := defaultInsertOpts()
	opts.RootParentID = "tnonexistent"
	_, err := s.InsertBatch(ctx, []parser.Item{
		{Content: "x", Status: "pending"},
	}, opts)
	if err == nil || !errors.Is(err, ErrInvalidParent) {
		t.Errorf("want ErrInvalidParent, got %v", err)
	}
}

func TestInsertBatch_DepthJumpRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	items := []parser.Item{
		{Content: "root", Status: "pending", Depth: 0},
		{Content: "skip to depth 2", Status: "pending", Depth: 2}, // 跳级
	}
	_, err := s.InsertBatch(ctx, items, defaultInsertOpts())
	if err == nil || !errors.Is(err, ErrInvalidIndent) {
		t.Errorf("want ErrInvalidIndent, got %v", err)
	}
}

func TestList_FilterByStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, _ = s.InsertBatch(ctx, []parser.Item{
		{Content: "p1", Status: "pending", Depth: 0},
		{Content: "p2", Status: "pending", Depth: 0},
		{Content: "d1", Status: "completed", Depth: 0},
	}, defaultInsertOpts())

	res, err := s.List(ctx, ListOptions{Scope: "default", Status: StatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || len(res.Items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", res.Total, len(res.Items))
	}
	if res.Counts.Pending != 2 || res.Counts.Completed != 1 {
		t.Errorf("counts = %+v, want pending=2 completed=1", res.Counts)
	}
	if res.Cursor == 0 {
		t.Error("cursor should be set after insert")
	}
}

func TestList_FilterByStatusesAndIDs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{
		{Content: "p1", Status: "pending", Depth: 0},
		{Content: "i1", Status: "in_progress", Depth: 0},
		{Content: "d1", Status: "completed", Depth: 0},
	}, defaultInsertOpts())

	res, err := s.List(ctx, ListOptions{
		Scope:    "default",
		IDs:      []string{ids[0], ids[2]},
		Statuses: []Status{StatusPending, StatusInProgress},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0].ID != ids[0] {
		t.Errorf("items = %+v total=%d, want only %s", res.Items, res.Total, ids[0])
	}
}

func TestList_SortByPriority(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	insertOne := func(prio Priority) {
		opts := defaultInsertOpts()
		opts.Priority = prio
		_, _ = s.InsertBatch(ctx, []parser.Item{{Content: string(prio), Status: "pending"}}, opts)
	}
	insertOne(PriorityLow)
	insertOne(PriorityUrgent)
	insertOne(PriorityHigh)
	insertOne(PriorityMedium)

	res, _ := s.List(ctx, ListOptions{Scope: "default", Sort: SortByPriority})
	want := []string{"urgent", "high", "medium", "low"}
	for i, todo := range res.Items {
		if string(todo.Priority) != want[i] {
			t.Errorf("position %d: priority = %s, want %s", i, todo.Priority, want[i])
		}
	}
}

func TestList_Pagination(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	items := make([]parser.Item, 10)
	for i := 0; i < 10; i++ {
		items[i] = parser.Item{Content: "task", Status: "pending"}
	}
	_, _ = s.InsertBatch(ctx, items, defaultInsertOpts())

	res, _ := s.List(ctx, ListOptions{Scope: "default", Limit: 4, Offset: 0})
	if res.Total != 10 || len(res.Items) != 4 || !res.HasMore {
		t.Errorf("page1: total=%d items=%d more=%v", res.Total, len(res.Items), res.HasMore)
	}
	res, _ = s.List(ctx, ListOptions{Scope: "default", Limit: 4, Offset: 8})
	if len(res.Items) != 2 || res.HasMore {
		t.Errorf("last page: items=%d more=%v", len(res.Items), res.HasMore)
	}
}

func TestList_ParentFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{
		{Content: "root1", Status: "pending", Depth: 0},
		{Content: "child", Status: "pending", Depth: 1},
		{Content: "root2", Status: "pending", Depth: 0},
	}, defaultInsertOpts())

	rootStr := ""
	res, _ := s.List(ctx, ListOptions{Scope: "default", ParentID: &rootStr})
	if res.Total != 2 {
		t.Errorf("root only: got %d, want 2", res.Total)
	}

	parentID := ids[0]
	res, _ = s.List(ctx, ListOptions{Scope: "default", ParentID: &parentID})
	if res.Total != 1 {
		t.Errorf("children of %s: got %d, want 1", parentID, res.Total)
	}
}

func TestUpdate_Status(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{
		{Content: "task", Status: "pending"},
	}, defaultInsertOpts())

	newStatus := StatusInProgress
	if err := s.Update(ctx, ids[0], UpdateFields{Status: &newStatus}, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	res, _ := s.List(ctx, ListOptions{Scope: "default"})
	if res.Items[0].Status != StatusInProgress {
		t.Errorf("status = %s, want in_progress", res.Items[0].Status)
	}
	if res.Items[0].Version != 2 {
		t.Errorf("version = %d, want 2 (insert+update)", res.Items[0].Version)
	}
}

func TestUpdate_DueAtClear(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	due := int64(1735689600000)
	opts := defaultInsertOpts()
	opts.DueAt = &due
	ids, _ := s.InsertBatch(ctx, []parser.Item{{Content: "x", Status: "pending"}}, opts)

	zero := int64(0)
	if err := s.Update(ctx, ids[0], UpdateFields{DueAt: &zero}, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	res, _ := s.List(ctx, ListOptions{Scope: "default"})
	if res.Items[0].DueAt != nil {
		t.Errorf("due_at = %v, want nil after clear (DueAt=0)", *res.Items[0].DueAt)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	st := StatusCompleted
	err := s.Update(ctx, "tnonexistent", UpdateFields{Status: &st}, 0)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestUpdate_NoFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.Update(ctx, "t1", UpdateFields{}, 0); !errors.Is(err, ErrEmptyUpdate) {
		t.Errorf("want ErrEmptyUpdate, got %v", err)
	}
}

func TestDeleteByIDs_TombstoneCreated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{
		{Content: "a", Status: "pending"},
		{Content: "b", Status: "pending"},
	}, defaultInsertOpts())

	deleted, err := s.DeleteByIDs(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 {
		t.Errorf("deleted = %d, want 2", len(deleted))
	}

	res, _ := s.List(ctx, ListOptions{Scope: "default"})
	if res.Total != 0 {
		t.Errorf("after delete, total = %d, want 0", res.Total)
	}

	var tombstones int
	_ = s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM deletions").Scan(&tombstones)
	if tombstones != 2 {
		t.Errorf("tombstones = %d, want 2", tombstones)
	}
}

func TestDelete_CascadeTombstone(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{
		{Content: "parent", Status: "pending", Depth: 0},
		{Content: "child", Status: "pending", Depth: 1},
		{Content: "grandchild", Status: "pending", Depth: 2},
	}, defaultInsertOpts())

	// 删根节点 → 子+孙都被 cascade，trigger 全部触发
	if _, err := s.DeleteByIDs(ctx, []string{ids[0]}); err != nil {
		t.Fatal(err)
	}

	var tombstones int
	if err := s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM deletions").Scan(&tombstones); err != nil {
		t.Fatal(err)
	}
	if tombstones != 3 {
		t.Errorf("cascade tombstones = %d, want 3 (root+child+grandchild). recursive_triggers off?", tombstones)
	}
}

func TestDeleteByScope(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	o1 := defaultInsertOpts()
	o1.Scope = "alpha"
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "a"}, {Content: "b"}}, o1)

	o2 := defaultInsertOpts()
	o2.Scope = "beta"
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "c"}}, o2)

	deleted, err := s.DeleteByScope(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 {
		t.Errorf("deleted from alpha: %d, want 2", len(deleted))
	}
	res, _ := s.List(ctx, ListOptions{Scope: "beta"})
	if res.Total != 1 {
		t.Errorf("beta untouched expected 1, got %d", res.Total)
	}
}

// --- helpers ---

func defaultInsertOpts() InsertOptions {
	return InsertOptions{
		Scope:      "default",
		Priority:   PriorityMedium,
		Difficulty: DifficultyMedium,
		NowMs:      time.Now().UnixMilli(),
	}
}

func expectParent(t *testing.T, m map[string]*string, id string, want *string) {
	t.Helper()
	got, ok := m[id]
	if !ok {
		t.Errorf("id %s not in result", id)
		return
	}
	switch {
	case want == nil && got == nil:
		// ok
	case want == nil && got != nil:
		t.Errorf("id %s parent = %s, want nil", id, *got)
	case want != nil && got == nil:
		t.Errorf("id %s parent = nil, want %s", id, *want)
	case *want != *got:
		t.Errorf("id %s parent = %s, want %s", id, *got, *want)
	}
}

// 必须用 parser.Item 时也要建一些 ad-hoc Item，避免提示 _ = parser.Item{} 引用。
var _ = parser.Item{}
