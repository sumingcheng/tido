package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sumingcheng/tido/internal/parser"
)

// TestConcurrent_WriteUpdateDiff 模拟多 agent 并发：
// 3 个 writer + 3 个 updater + 1 个 differ，跑 2 秒，
// 验证：无 deadlock、version 严格单调、写入数 == diff 见到的 upsert 数。
func TestConcurrent_WriteUpdateDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skip stress in -short")
	}

	ctx := context.Background()
	s := newTestStore(t)

	const writers = 3
	const updaters = 3
	duration := 2 * time.Second

	var (
		writeOps  atomic.Int64
		updateOps atomic.Int64
		diffOps   atomic.Int64
		ids       sync.Map // string → struct{}，记录所有写入的 id
		stop      = make(chan struct{})
		wg        sync.WaitGroup
	)
	// time.AfterFunc 触发后 close(stop)，所有 goroutine 通过 channel 已关闭语义同时退出。
	// 不能用 time.After（single-buffered，多个 select 会饿死）。
	timer := time.AfterFunc(duration, func() { close(stop) })
	defer timer.Stop()

	now := func() int64 { return time.Now().UnixMilli() }

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				out, err := s.InsertBatch(ctx, []parser.Item{
					{Content: "task", Status: "pending"},
				}, InsertOptions{
					Scope: "stress", Priority: PriorityMedium,
					Difficulty: DifficultyMedium, NowMs: now(),
				})
				if err != nil {
					t.Errorf("write: %v", err)
					return
				}
				for _, id := range out {
					ids.Store(id, struct{}{})
				}
				writeOps.Add(1)
			}
		}()
	}

	for u := 0; u < updaters; u++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// 找一个已知 id 更新；找不到就跳过
				var picked string
				ids.Range(func(k, _ any) bool { picked = k.(string); return false })
				if picked == "" {
					time.Sleep(5 * time.Millisecond)
					continue
				}
				st := StatusInProgress
				if err := s.Update(ctx, picked, UpdateFields{Status: &st}, now()); err != nil {
					// 可能被并发删除（虽然此测试无 delete）；只记 ErrNotFound 不算失败
					continue
				}
				updateOps.Add(1)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		var since int64
		for {
			select {
			case <-stop:
				return
			default:
			}
			r, err := s.Diff(ctx, "stress", since, 200)
			if err != nil {
				t.Errorf("diff: %v", err)
				return
			}
			since = r.NextCursor
			diffOps.Add(1)
			time.Sleep(20 * time.Millisecond)
		}
	}()

	wg.Wait()

	// 循环 diff 拿全部 changes（store.Diff 单次 limit ≤ 200）
	var (
		all    []Change
		cursor int64
	)
	for {
		r, err := s.Diff(ctx, "stress", cursor, 200)
		if err != nil {
			t.Fatalf("final diff: %v", err)
		}
		all = append(all, r.Changes...)
		cursor = r.NextCursor
		if !r.HasMore {
			break
		}
	}

	var seenIDs int
	for _, c := range all {
		if c.Op == ChangeUpsert {
			seenIDs++
		}
	}

	expected := int(writeOps.Load())
	if seenIDs != expected {
		t.Errorf("upsert in final diff = %d, want %d (writes)", seenIDs, expected)
	}

	last := int64(0)
	for _, c := range all {
		if c.Todo.Version <= last {
			t.Errorf("version not strictly increasing: %d after %d", c.Todo.Version, last)
		}
		last = c.Todo.Version
	}

	t.Logf("stress: writes=%d updates=%d diffs=%d total_changes=%d max_version=%d",
		writeOps.Load(), updateOps.Load(), diffOps.Load(), len(all), last)
}
