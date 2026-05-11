package store

import (
	"context"
	"testing"
	"time"

	"github.com/sumingcheng/tido/internal/parser"
)

func TestDiff_EmptyInitial(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	r, err := s.Diff(ctx, "default", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Changes) != 0 || r.HasMore {
		t.Errorf("empty db diff: changes=%d more=%v", len(r.Changes), r.HasMore)
	}
	if r.NextCursor != 0 {
		t.Errorf("NextCursor = %d, want 0 (no writes yet)", r.NextCursor)
	}
}

func TestDiff_UpsertOnly(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.InsertBatch(ctx, []parser.Item{
		{Content: "a"}, {Content: "b"},
	}, defaultInsertOpts()); err != nil {
		t.Fatalf("insert: %v", err)
	}

	r, err := s.Diff(ctx, "default", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Changes) != 2 {
		t.Errorf("changes = %d, want 2", len(r.Changes))
	}
	for _, c := range r.Changes {
		if c.Op != ChangeUpsert {
			t.Errorf("op = %s, want upsert", c.Op)
		}
	}
	if r.HasMore {
		t.Error("HasMore = true, want false (only 2 items, limit 50)")
	}
}

func TestDiff_DeleteShowsTombstone(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{{Content: "a"}}, defaultInsertOpts())
	if _, err := s.DeleteByIDs(ctx, ids); err != nil {
		t.Fatal(err)
	}

	r, err := s.Diff(ctx, "default", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	// hard delete：todos 不再有 t1 → 不出现 upsert，只剩 tombstone
	if len(r.Changes) != 1 {
		t.Fatalf("changes = %d, want 1 (only delete)", len(r.Changes))
	}
	if r.Changes[0].Op != ChangeDelete {
		t.Errorf("op = %s, want delete", r.Changes[0].Op)
	}
	if r.Changes[0].Todo.ID != ids[0] {
		t.Errorf("delete id = %s, want %s", r.Changes[0].Todo.ID, ids[0])
	}
}

func TestDiff_IncrementalCursor(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// 第一波
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "first batch"}}, defaultInsertOpts())
	r1, _ := s.Diff(ctx, "default", 0, 50)
	if r1.HasMore {
		t.Error("first diff should be complete")
	}

	// 第二波（agent 接着上次 cursor 继续）
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "second batch"}}, defaultInsertOpts())
	r2, _ := s.Diff(ctx, "default", r1.NextCursor, 50)
	if len(r2.Changes) != 1 {
		t.Errorf("second diff changes = %d, want 1 (only the new batch)", len(r2.Changes))
	}
	if r2.Changes[0].Todo.Content != "second batch" {
		t.Errorf("got content %q, want 'second batch'", r2.Changes[0].Todo.Content)
	}
}

func TestDiff_PaginationHasMore(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "x"}}, defaultInsertOpts())
	}

	r, _ := s.Diff(ctx, "default", 0, 3)
	if !r.HasMore {
		t.Error("HasMore should be true (5 changes, limit 3)")
	}
	if len(r.Changes) != 3 {
		t.Errorf("page1: %d changes, want 3", len(r.Changes))
	}

	r2, _ := s.Diff(ctx, "default", r.NextCursor, 3)
	if r2.HasMore {
		t.Error("page2 should be last")
	}
	if len(r2.Changes) != 2 {
		t.Errorf("page2: %d changes, want 2", len(r2.Changes))
	}
}

func TestDiff_ScopeIsolated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	o := defaultInsertOpts()
	o.Scope = "alpha"
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "a"}}, o)

	o.Scope = "beta"
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "b"}}, o)

	r, _ := s.Diff(ctx, "alpha", 0, 50)
	if len(r.Changes) != 1 || r.Changes[0].Todo.Scope != "alpha" {
		t.Errorf("alpha diff = %+v, want exactly 1 from alpha scope", r.Changes)
	}
}

// 防御：DueAt 在 diff 中被正确填充。
func TestDiff_DueAtPropagated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	due := time.Now().Add(24 * time.Hour).UnixMilli()
	o := defaultInsertOpts()
	o.DueAt = &due
	_, _ = s.InsertBatch(ctx, []parser.Item{{Content: "x"}}, o)

	r, _ := s.Diff(ctx, "default", 0, 50)
	if r.Changes[0].Todo.DueAt == nil || *r.Changes[0].Todo.DueAt != due {
		t.Errorf("DueAt not propagated correctly; got %v, want %d", r.Changes[0].Todo.DueAt, due)
	}
}
