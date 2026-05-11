package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/validate"
	"github.com/sumingcheng/tido/internal/view"
)

// TodoDiffArgs 是 todo_diff 的入参。
type TodoDiffArgs struct {
	Scope string `json:"scope,omitempty" jsonschema:" 工作域，省略=default。"`
	Since int64  `json:"since" jsonschema:" 上次拿到的 next_cursor；首次传 0。"`
	Limit int    `json:"limit,omitempty" jsonschema:" ≤200，省略=50。"`
}

// TodoDiffResult 是 todo_diff 的返回。
type TodoDiffResult struct {
	Changes    []view.ChangeView `json:"changes"`
	NextCursor int64             `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

func todoDiffTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_diff",
		Description: "增量拉取自 since 起的所有变更（含 deletions tombstone）。next_cursor 用于下次调用，HasMore=true 时应继续拉。",
	}
}

func (s *Service) todoDiff(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoDiffArgs) (*mcpsdk.CallToolResult, TodoDiffResult, error) {
	scope := resolveScope(args.Scope)
	if err := validate.Scope(scope); err != nil {
		return errResult[TodoDiffResult](err)
	}
	if args.Since < 0 {
		return errResult[TodoDiffResult](fmt.Errorf("since must be >= 0, got %d", args.Since))
	}

	out, err := s.store.Diff(ctx, scope, args.Since, args.Limit)
	if err != nil {
		return errResult[TodoDiffResult](err)
	}

	res := TodoDiffResult{
		Changes:    view.RenderChanges(out.Changes, s.now()),
		NextCursor: out.NextCursor,
		HasMore:    out.HasMore,
	}
	return okResult(fmt.Sprintf("diff %d changes (cursor → %d, more=%v)",
		len(res.Changes), res.NextCursor, res.HasMore), res)
}
