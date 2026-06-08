package view

import (
	"strings"
	"testing"
	"time"

	"github.com/sumingcheng/tido/internal/store"
)

func TestRenderTodo_Compact(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC).UnixMilli()
	due := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC).UnixMilli()
	parent := "t1"

	td := store.Todo{
		ID:         "t2",
		Scope:      "default",
		Content:    "hello",
		Status:     store.StatusPending,
		Priority:   store.PriorityHigh,
		Difficulty: store.DifficultyMedium,
		DueAt:      &due,
		ParentID:   &parent,
		Version:    7,
		CreatedAt:  now,
		UpdatedAt:  now,
		NotesCount: 2,
	}
	v := RenderTodo(td, ModeCompact, now)

	if v.Scope != "" || v.Version != 0 || v.CreatedAt != "" || v.UpdatedAt != "" {
		t.Errorf("compact must omit scope/version/created/updated; got %+v", v)
	}
	if v.DueAt != "@2d" {
		t.Errorf("DueAt = %q, want \"@2d\"", v.DueAt)
	}
	if v.ParentID != "t1" || v.Notes != 2 {
		t.Errorf("parent/notes lost: %+v", v)
	}
	if v.Priority != "high" || v.Difficulty != "" {
		t.Errorf("compact should keep non-default priority and omit default difficulty; got %+v", v)
	}
}

func TestRenderTodo_Full(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	td := store.Todo{
		ID:         "t1",
		Scope:      "alpha",
		Content:    "x",
		Status:     store.StatusInProgress,
		Priority:   store.PriorityUrgent,
		Difficulty: store.DifficultyHard,
		Version:    3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	v := RenderTodo(td, ModeFull, now)

	if v.Scope != "alpha" || v.Version != 3 {
		t.Errorf("full must keep scope/version; got %+v", v)
	}
	if !strings.HasPrefix(v.CreatedAt, "2026-01-01") {
		t.Errorf("CreatedAt should be ISO8601; got %q", v.CreatedAt)
	}
}

func TestRelativeDue(t *testing.T) {
	base := time.Now().UnixMilli()
	cases := []struct {
		offset time.Duration
		want   string
	}{
		{2 * 24 * time.Hour, "@2d"},
		{3 * time.Hour, "@3h"},
		{45 * time.Minute, "@45m"},
		{-2 * time.Hour, "@overdue 2h"},
		{-3 * 24 * time.Hour, "@overdue 3d"},
		{30 * time.Second, "@now"},
		{-30 * time.Second, "@now"},
	}
	for _, c := range cases {
		due := base + c.offset.Milliseconds()
		got := relativeDue(due, base)
		if got != c.want {
			t.Errorf("offset %v: got %q, want %q", c.offset, got, c.want)
		}
	}
}
