package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/store"
	"github.com/sumingcheng/tido/internal/validate"
)

// TodoUpdateArgs 是 todo_update 的入参。
// 所有字段（除 ID）省略 = 不变更；至少传一个可变字段。
type TodoUpdateArgs struct {
	ID          string `json:"id" jsonschema:"待更新的 todo 短码（如 t3a）。"`
	Status      string `json:"status,omitempty" jsonschema:"pending | in_progress | completed | cancelled。"`
	Content     string `json:"content,omitempty" jsonschema:"新内容；省略=不变。"`
	Priority    string `json:"priority,omitempty" jsonschema:"low | medium | high | urgent。"`
	Difficulty  string `json:"difficulty,omitempty" jsonschema:"trivial | easy | medium | hard。"`
	DueAt       string `json:"due_at,omitempty" jsonschema:"截止时间：unix ms 或 RFC3339；省略=不变。"`
	ClearDueAt  bool   `json:"clear_due_at,omitempty" jsonschema:"true 表示清除截止时间（与 due_at 互斥）。"`
}

// TodoUpdateResult 是 todo_update 的返回。
type TodoUpdateResult struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
}

func todoUpdateTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_update",
		Description: "更新单条 todo 的可变字段（status/content/priority/difficulty/due_at）；至少传一个字段。",
	}
}

func (s *Service) todoUpdate(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoUpdateArgs) (*mcpsdk.CallToolResult, TodoUpdateResult, error) {
	if err := validate.ID(args.ID); err != nil {
		return errResult[TodoUpdateResult](err)
	}

	fields, err := buildUpdateFields(args)
	if err != nil {
		return errResult[TodoUpdateResult](err)
	}

	if err := s.store.Update(ctx, args.ID, fields, s.now()); err != nil {
		return errResult[TodoUpdateResult](err)
	}

	res := TodoUpdateResult{ID: args.ID, OK: true}
	return okResult(fmt.Sprintf("updated %s", args.ID), res)
}

// buildUpdateFields 把 args 翻译为 store.UpdateFields；
// 同时校验枚举值与 due_at 互斥规则。
func buildUpdateFields(args TodoUpdateArgs) (store.UpdateFields, error) {
	var f store.UpdateFields

	if args.Status != "" {
		st, err := normalizeStatus(args.Status)
		if err != nil {
			return f, err
		}
		f.Status = &st
	}

	if args.Content != "" {
		if err := validate.TodoContent(args.Content); err != nil {
			return f, err
		}
		c := args.Content
		f.Content = &c
	}

	if args.Priority != "" {
		p, err := normalizePriority(args.Priority)
		if err != nil {
			return f, err
		}
		f.Priority = &p
	}

	if args.Difficulty != "" {
		d, err := normalizeDifficulty(args.Difficulty)
		if err != nil {
			return f, err
		}
		f.Difficulty = &d
	}

	switch {
	case args.ClearDueAt && args.DueAt != "":
		return f, fmt.Errorf("due_at and clear_due_at are mutually exclusive")
	case args.ClearDueAt:
		zero := int64(0)
		f.DueAt = &zero
	case args.DueAt != "":
		ms, err := parseDueAt(args.DueAt)
		if err != nil {
			return f, err
		}
		f.DueAt = ms
	}

	return f, nil
}
