package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/validate"
)

// TodoDeleteArgs 是 todo_delete 的入参。
// IDs 与 Scope 互斥：必须传其中之一。
type TodoDeleteArgs struct {
	IDs   []string `json:"ids,omitempty" jsonschema:" 按短码批量删除；与 scope 互斥。"`
	Scope string   `json:"scope,omitempty" jsonschema:" 按工作域整体清空；与 ids 互斥。"`
}

// TodoDeleteResult 是 todo_delete 的返回。
type TodoDeleteResult struct {
	Deleted []string `json:"deleted"`
	Count   int      `json:"count"`
	Cursor  int64    `json:"cursor,omitempty"`
}

func todoDeleteTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_delete",
		Description: "物理删除 todos（自动写 tombstone 让 todo_diff 能传播）。要么按 ids 删、要么按 scope 整批清；有实际删除时返回 cursor。",
	}
}

func (s *Service) todoDelete(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoDeleteArgs) (*mcpsdk.CallToolResult, TodoDeleteResult, error) {
	hasIDs := len(args.IDs) > 0
	hasScope := args.Scope != ""

	switch {
	case hasIDs && hasScope:
		return errResult[TodoDeleteResult](fmt.Errorf("ids and scope are mutually exclusive"))
	case !hasIDs && !hasScope:
		return errResult[TodoDeleteResult](fmt.Errorf("must provide ids or scope"))
	}

	if hasIDs {
		for _, id := range args.IDs {
			if err := validate.ID(id); err != nil {
				return errResult[TodoDeleteResult](err)
			}
		}
		out, err := s.store.DeleteByIDsDetailed(ctx, args.IDs)
		if err != nil {
			return errResult[TodoDeleteResult](err)
		}
		res := TodoDeleteResult{Deleted: out.Deleted, Count: len(out.Deleted), Cursor: out.Cursor}
		return okResult(fmt.Sprintf("deleted %d todo(s) (cursor=%d)", len(out.Deleted), out.Cursor), res)
	}

	if err := validate.Scope(args.Scope); err != nil {
		return errResult[TodoDeleteResult](err)
	}
	out, err := s.store.DeleteByScopeDetailed(ctx, args.Scope)
	if err != nil {
		return errResult[TodoDeleteResult](err)
	}
	res := TodoDeleteResult{Deleted: out.Deleted, Count: len(out.Deleted), Cursor: out.Cursor}
	return okResult(fmt.Sprintf("deleted %d todo(s) from scope %q (cursor=%d)", len(out.Deleted), args.Scope, out.Cursor), res)
}
