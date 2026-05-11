package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/validate"
	"github.com/sumingcheng/tido/internal/view"
)

// TodoGetNotesArgs 是 todo_get_notes 的入参。
type TodoGetNotesArgs struct {
	TodoID string `json:"todo_id" jsonschema:" 目标 todo 短码。"`
	Limit  int    `json:"limit,omitempty" jsonschema:" ≤100，省略=20。"`
	Offset int    `json:"offset,omitempty" jsonschema:" 分页偏移；省略=0。"`
}

// TodoGetNotesResult 是 todo_get_notes 的返回。
type TodoGetNotesResult struct {
	Notes   []view.NoteView `json:"notes"`
	HasMore bool            `json:"has_more"`
}

func todoGetNotesTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_get_notes",
		Description: "拉取某 todo 的笔记，按时间升序，分页返回。",
	}
}

func (s *Service) todoGetNotes(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoGetNotesArgs) (*mcpsdk.CallToolResult, TodoGetNotesResult, error) {
	if err := validate.ID(args.TodoID); err != nil {
		return errResult[TodoGetNotesResult](err)
	}

	out, err := s.store.ListNotes(ctx, args.TodoID, args.Limit, args.Offset)
	if err != nil {
		return errResult[TodoGetNotesResult](err)
	}

	res := TodoGetNotesResult{
		Notes:   view.RenderNotes(out.Notes),
		HasMore: out.HasMore,
	}
	return okResult(fmt.Sprintf("returned %d notes for %s", len(res.Notes), args.TodoID), res)
}
