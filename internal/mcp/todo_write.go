package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/parser"
	"github.com/sumingcheng/tido/internal/store"
	"github.com/sumingcheng/tido/internal/validate"
)

// TodoWriteArgs 是 todo_write 的入参。
type TodoWriteArgs struct {
	Items      string `json:"items" jsonschema:" 待办内容；markdown checklist 或纯文本（每行一条）。markdown 缩进 2 空格 = 父子。"`
	Scope      string `json:"scope,omitempty" jsonschema:" 工作域，省略=default。仅同 scope 内 diff/list 互通。"`
	ParentID   string `json:"parent_id,omitempty" jsonschema:" 挂载到此父任务下；省略=本批为顶层任务。"`
	Priority   string `json:"priority,omitempty" jsonschema:" low | medium | high | urgent，省略=medium。"`
	Difficulty string `json:"difficulty,omitempty" jsonschema:" trivial | easy | medium | hard，省略=medium。"`
	DueAt      string `json:"due_at,omitempty" jsonschema:" 截止时间：unix ms 数字串 或 RFC3339 字符串。省略=无截止。"`
}

// TodoWriteResult 是 todo_write 的返回。
type TodoWriteResult struct {
	IDs   []string `json:"ids"`
	Count int      `json:"count"`
}

func todoWriteTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_write",
		Description: "批量写入待办：自动识别 markdown checklist / 纯文本，支持父子层级、优先级、难度、截止时间。返回新生成的短码 ids。",
	}
}

func (s *Service) todoWrite(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoWriteArgs) (*mcpsdk.CallToolResult, TodoWriteResult, error) {
	scope := resolveScope(args.Scope)
	if err := validate.Scope(scope); err != nil {
		return errResult[TodoWriteResult](err)
	}

	priority, err := normalizePriority(args.Priority)
	if err != nil {
		return errResult[TodoWriteResult](err)
	}
	difficulty, err := normalizeDifficulty(args.Difficulty)
	if err != nil {
		return errResult[TodoWriteResult](err)
	}

	dueAt, err := parseDueAt(args.DueAt)
	if err != nil {
		return errResult[TodoWriteResult](err)
	}

	if args.ParentID != "" {
		if err := validate.ID(args.ParentID); err != nil {
			return errResult[TodoWriteResult](err)
		}
	}

	items, err := parser.Parse(args.Items)
	if err != nil {
		return errResult[TodoWriteResult](err)
	}
	for i, it := range items {
		if err := validate.TodoContent(it.Content); err != nil {
			return errResult[TodoWriteResult](fmt.Errorf("item %d: %w", i+1, err))
		}
	}

	ids, err := s.store.InsertBatch(ctx, items, store.InsertOptions{
		Scope:        scope,
		RootParentID: args.ParentID,
		Priority:     priority,
		Difficulty:   difficulty,
		DueAt:        dueAt,
		NowMs:        s.now(),
	})
	if err != nil {
		return errResult[TodoWriteResult](err)
	}

	res := TodoWriteResult{IDs: ids, Count: len(ids)}
	return okResult(fmt.Sprintf("wrote %d todo(s) into scope %q", len(ids), scope), res)
}

// normalizePriority 校验并填充默认 priority。
func normalizePriority(s string) (store.Priority, error) {
	if s == "" {
		return store.PriorityMedium, nil
	}
	switch store.Priority(s) {
	case store.PriorityLow, store.PriorityMedium, store.PriorityHigh, store.PriorityUrgent:
		return store.Priority(s), nil
	}
	return "", fmt.Errorf("invalid priority %q (allowed: low|medium|high|urgent)", s)
}

// normalizeDifficulty 校验并填充默认 difficulty。
func normalizeDifficulty(s string) (store.Difficulty, error) {
	if s == "" {
		return store.DifficultyMedium, nil
	}
	switch store.Difficulty(s) {
	case store.DifficultyTrivial, store.DifficultyEasy, store.DifficultyMedium, store.DifficultyHard:
		return store.Difficulty(s), nil
	}
	return "", fmt.Errorf("invalid difficulty %q (allowed: trivial|easy|medium|hard)", s)
}

// normalizeStatus 校验并返回 store.Status；空串保留为空（"不变更"语义）。
func normalizeStatus(s string) (store.Status, error) {
	if s == "" {
		return "", nil
	}
	switch store.Status(s) {
	case store.StatusPending, store.StatusInProgress, store.StatusCompleted, store.StatusCancelled:
		return store.Status(s), nil
	}
	return "", fmt.Errorf("invalid status %q (allowed: pending|in_progress|completed|cancelled)", s)
}
