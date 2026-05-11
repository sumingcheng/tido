package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNewMemory_SchemaInitialized(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, ":memory:")

	tables := []string{"meta", "todos", "notes", "deletions"}
	for _, name := range tables {
		var got string
		err := s.DB().QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&got)
		if err != nil {
			t.Fatalf("table %s missing: %v", name, err)
		}
	}

	var triggerName string
	err := s.DB().QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='trigger' AND name='trg_todos_after_delete'`,
	).Scan(&triggerName)
	if err != nil {
		t.Fatalf("delete trigger missing: %v", err)
	}
}

func TestNewMemory_PragmasApplied(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, ":memory:")

	cases := map[string]string{
		"journal_mode":       "memory", // 内存库下 WAL 不可用，会回退；文件库测见下
		"synchronous":        "1",
		"foreign_keys":       "1",
		"recursive_triggers": "1",
	}
	for pragma, want := range cases {
		var got string
		if err := s.DB().QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
			t.Fatalf("read pragma %s: %v", pragma, err)
		}
		if got != want {
			t.Errorf("pragma %s = %q, want %q", pragma, got, want)
		}
	}
}

func TestNewFile_WALEnabled(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "wal.db")
	s := newTestStore(t, dbPath)

	var mode string
	if err := s.DB().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q on file db, want wal", mode)
	}
}

func TestMetaSeeded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, ":memory:")

	for _, key := range []string{"last_id", "version"} {
		var v int
		err := s.DB().QueryRowContext(ctx,
			`SELECT value FROM meta WHERE key = ?`, key,
		).Scan(&v)
		if err != nil {
			t.Fatalf("meta[%s] missing: %v", key, err)
		}
		if v != 0 {
			t.Errorf("meta[%s] = %d, want 0", key, v)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "idem.db")

	s1 := newTestStore(t, dbPath)
	if _, err := s1.DB().ExecContext(ctx,
		`UPDATE meta SET value = 42 WHERE key = 'version'`); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close s1: %v", err)
	}

	s2 := newTestStore(t, dbPath)
	defer s2.Close()

	var v int
	err := s2.DB().QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = 'version'`).Scan(&v)
	if err != nil {
		t.Fatalf("read version after reopen: %v", err)
	}
	if v != 42 {
		t.Errorf("version = %d after reopen, want 42 (migrate must be idempotent)", v)
	}
}

func TestMigrate_RejectFutureVersion(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "future.db")

	s := newTestStore(t, dbPath)
	if _, err := s.DB().ExecContext(ctx, "PRAGMA user_version = 999"); err != nil {
		t.Fatalf("force future version: %v", err)
	}
	s.Close()

	if _, err := New(ctx, dbPath); err == nil {
		t.Fatal("expected New() to reject db with user_version > SchemaVersion, got nil")
	}
}

// newTestStore 构造测试 Store，t.Cleanup 自动关连接。
func newTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := New(context.Background(), path)
	if err != nil {
		t.Fatalf("New(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
