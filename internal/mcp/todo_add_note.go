package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/validate"
)

// TodoAddNoteArgs 是 todo_add_note 的入参。
type TodoAddNoteArgs struct {
	TodoID  string `json:"todo_id" jsonschema:"目标 todo 短码。"`
	Content string `json:"content" jsonschema:"笔记内容；≤ 8 KiB。"`
}

// TodoAddNoteResult 是 todo_add_note 的返回。
type TodoAddNoteResult struct {
	NoteID int64  `json:"note_id"`
	TodoID string `json:"todo_id"`
}

func todoAddNoteTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_add_note",
		Description: "给 todo 追加一条留痕笔记（append-only）。不会触发 todo_diff（不动 todos.version）。",
	}
}

func (s *Service) todoAddNote(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoAddNoteArgs) (*mcpsdk.CallToolResult, TodoAddNoteResult, error) {
	if err := validate.ID(args.TodoID); err != nil {
		return errResult[TodoAddNoteResult](err)
	}
	if err := validate.NoteContent(args.Content); err != nil {
		return errResult[TodoAddNoteResult](err)
	}

	id, err := s.store.AddNote(ctx, args.TodoID, args.Content, s.now())
	if err != nil {
		return errResult[TodoAddNoteResult](err)
	}

	res := TodoAddNoteResult{NoteID: id, TodoID: args.TodoID}
	return okResult(fmt.Sprintf("note #%d added to %s", id, args.TodoID), res)
}
