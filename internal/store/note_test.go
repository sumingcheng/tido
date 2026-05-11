package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sumingcheng/tido/internal/parser"
)

func TestAddNote_Basic(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{{Content: "task", Status: "pending"}}, defaultInsertOpts())

	id1, err := s.AddNote(ctx, ids[0], "first note", time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.AddNote(ctx, ids[0], "second note", time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id1+1 {
		t.Errorf("note ids non-monotonic: id1=%d id2=%d", id1, id2)
	}
}

func TestAddNote_DoesNotBumpTodoVersion(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{{Content: "x", Status: "pending"}}, defaultInsertOpts())
	res1, _ := s.List(ctx, ListOptions{Scope: "default"})
	verBefore := res1.Items[0].Version

	if _, err := s.AddNote(ctx, ids[0], "a note", time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}

	res2, _ := s.List(ctx, ListOptions{Scope: "default"})
	if res2.Items[0].Version != verBefore {
		t.Errorf("todo.version changed by AddNote: %d → %d (must not change per §9.9)",
			verBefore, res2.Items[0].Version)
	}
}

func TestAddNote_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, err := s.AddNote(ctx, "tnope", "x", 0)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestListNotes_Pagination(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	ids, _ := s.InsertBatch(ctx, []parser.Item{{Content: "x", Status: "pending"}}, defaultInsertOpts())
	for i := 0; i < 5; i++ {
		if _, err := s.AddNote(ctx, ids[0], "note", time.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}

	r, err := s.ListNotes(ctx, ids[0], 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Notes) != 2 || !r.HasMore {
		t.Errorf("page1: notes=%d more=%v", len(r.Notes), r.HasMore)
	}

	r, _ = s.ListNotes(ctx, ids[0], 2, 4)
	if len(r.Notes) != 1 || r.HasMore {
		t.Errorf("last page: notes=%d more=%v", len(r.Notes), r.HasMore)
	}
}

func TestListNotes_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, err := s.ListNotes(ctx, "tnope", 10, 0)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestNotesCount_ReflectedInList(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ids, _ := s.InsertBatch(ctx, []parser.Item{{Content: "x", Status: "pending"}}, defaultInsertOpts())
	for i := 0; i < 3; i++ {
		_, _ = s.AddNote(ctx, ids[0], "n", 0)
	}
	res, _ := s.List(ctx, ListOptions{Scope: "default"})
	if res.Items[0].NotesCount != 3 {
		t.Errorf("NotesCount = %d, want 3", res.Items[0].NotesCount)
	}
}
