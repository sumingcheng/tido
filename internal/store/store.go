// Package store 封装 SQLite 连接、PRAGMA 配置与事务原语。
// 设计依据：DESIGN.md §3 / §7。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 纯 Go 驱动，注册 "sqlite" driver
)

// Store 持有底层 *sql.DB；并发安全（database/sql 自带连接池）。
type Store struct {
	db *sql.DB
}

// 启用的 PRAGMA 列表（DESIGN.md §7.1）。
// recursive_triggers 是 cascade tombstone 不丢的关键，绝对不能删。
var defaultPragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"busy_timeout(5000)",
	"foreign_keys(ON)",
	"recursive_triggers(ON)",
	"temp_store(MEMORY)",
	"cache_size(-64000)",
	"mmap_size(134217728)",
}

// New 打开（或创建）SQLite 数据库，应用全部 PRAGMA + 自动 schema migration。
// dbPath 传 ":memory:" 表示用内存库（仅测试用）。
func New(ctx context.Context, dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{db: db}
	if err := Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return s, nil
}

// Close 关闭连接池。
func (s *Store) Close() error {
	return s.db.Close()
}

// DB 暴露底层句柄给同包 CRUD 使用；外部包不应访问。
func (s *Store) DB() *sql.DB {
	return s.db
}

// BeginImmediate 开启 IMMEDIATE 事务，立即拿写锁。
// DESIGN.md §4.1 不变量：所有写操作必须走此入口。
// _txlock=immediate 已让 BeginTx 自动用 IMMEDIATE，这里仍提供命名入口便于阅读。
func (s *Store) BeginImmediate(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

// buildDSN 用 modernc.org/sqlite 的 DSN 语法拼出连接串：
//
//	file:<path>?_pragma=...(...)&_pragma=...(...)&_txlock=immediate
//
// _txlock=immediate 让 BeginTx 自动 BEGIN IMMEDIATE，避免 deferred → upgrade 冲突。
func buildDSN(path string) string {
	q := url.Values{}
	for _, p := range defaultPragmas {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	if path == ":memory:" {
		return "file::memory:?" + q.Encode()
	}
	return "file:" + path + "?" + q.Encode()
}
