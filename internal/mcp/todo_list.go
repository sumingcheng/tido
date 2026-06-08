package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/store"
	"github.com/sumingcheng/tido/internal/validate"
	"github.com/sumingcheng/tido/internal/view"
)

// TodoListArgs 是 todo_list 的入参。
type TodoListArgs struct {
	Scope    string   `json:"scope,omitempty" jsonschema:" 工作域，省略=default。"`
	IDs      []string `json:"ids,omitempty" jsonschema:" 按短码精确查询；省略 status 时不做状态过滤。"`
	Status   string   `json:"status,omitempty" jsonschema:" active | all | pending | in_progress | completed | cancelled。省略=active；ids 非空时省略=all。"`
	ParentID string   `json:"parent_id,omitempty" jsonschema:" 按父任务过滤；传 'root' 仅顶层；省略=不过滤。"`
	View     string   `json:"view,omitempty" jsonschema:" compact (省元字段+相对时间) | full。省略=compact。"`
	Sort     string   `json:"sort,omitempty" jsonschema:" created | priority | due。省略=created。"`
	Limit    int      `json:"limit,omitempty" jsonschema:" ≤500，省略=100。"`
	Offset   int      `json:"offset,omitempty" jsonschema:" 分页偏移；省略=0。"`
}

// TodoListResult 是 todo_list 的返回。
type TodoListResult struct {
	Items   []view.TodoView    `json:"items"`
	Total   int                `json:"total"`
	HasMore bool               `json:"has_more"`
	Cursor  int64              `json:"cursor"`
	Counts  store.StatusCounts `json:"counts"`
}

func todoListTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_list",
		Description: "查询待办列表：默认只返回 active(pending+in_progress) compact 工作视图，并返回各状态 counts 与 cursor；查完成/取消/全部必须显式传 status=completed|cancelled|all，审计才用 view=full。",
	}
}

func (s *Service) todoList(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoListArgs) (*mcpsdk.CallToolResult, TodoListResult, error) {
	scope := resolveScope(args.Scope)
	if err := validate.Scope(scope); err != nil {
		return errResult[TodoListResult](err)
	}

	if err := validateListIDs(args.IDs); err != nil {
		return errResult[TodoListResult](err)
	}

	statuses, statusLabel, err := normalizeListStatuses(args.Status, len(args.IDs) > 0)
	if err != nil {
		return errResult[TodoListResult](err)
	}

	sort, err := normalizeSort(args.Sort)
	if err != nil {
		return errResult[TodoListResult](err)
	}

	mode, err := normalizeView(args.View)
	if err != nil {
		return errResult[TodoListResult](err)
	}

	parentFilter, err := resolveParentFilter(args.ParentID)
	if err != nil {
		return errResult[TodoListResult](err)
	}

	limit := args.Limit
	if len(args.IDs) > 0 && limit <= 0 {
		limit = len(args.IDs)
	}

	out, err := s.store.List(ctx, store.ListOptions{
		Scope:    scope,
		Statuses: statuses,
		IDs:      args.IDs,
		ParentID: parentFilter,
		Sort:     sort,
		Limit:    limit,
		Offset:   args.Offset,
	})
	if err != nil {
		return errResult[TodoListResult](err)
	}

	res := TodoListResult{
		Items:   view.RenderTodos(out.Items, mode, s.now()),
		Total:   out.Total,
		HasMore: out.HasMore,
		Cursor:  out.Cursor,
		Counts:  out.Counts,
	}
	summary := fmt.Sprintf("listed %d/%d todos in scope %q (status=%s sort=%s view=%s cursor=%d)",
		len(res.Items), res.Total, scope, statusLabel, sort, mode, res.Cursor)
	return okResult(summary, res)
}

// normalizeSort 校验并默认填充 sort。
func normalizeSort(s string) (store.SortOrder, error) {
	switch s {
	case "":
		return store.SortByCreated, nil
	case string(store.SortByCreated), string(store.SortByPriority), string(store.SortByDue):
		return store.SortOrder(s), nil
	}
	return "", fmt.Errorf("invalid sort %q (allowed: created|priority|due)", s)
}

// normalizeView 校验并默认填充 view 模式。
func normalizeView(s string) (view.Mode, error) {
	switch s {
	case "":
		return view.ModeCompact, nil
	case string(view.ModeCompact), string(view.ModeFull):
		return view.Mode(s), nil
	}
	return "", fmt.Errorf("invalid view %q (allowed: compact|full)", s)
}

// normalizeListStatuses 把 list 的意图型 status 转成 store 多状态过滤。
// nil 表示不过滤；默认 active，精确 ids 查询默认 all，避免查单条 completed 时意外空结果。
func normalizeListStatuses(s string, hasIDs bool) ([]store.Status, string, error) {
	switch s {
	case "":
		if hasIDs {
			return nil, "all", nil
		}
		return []store.Status{store.StatusPending, store.StatusInProgress}, "active", nil
	case "active":
		return []store.Status{store.StatusPending, store.StatusInProgress}, "active", nil
	case "all":
		return nil, "all", nil
	case string(store.StatusPending), string(store.StatusInProgress), string(store.StatusCompleted), string(store.StatusCancelled):
		return []store.Status{store.Status(s)}, s, nil
	}
	return nil, "", fmt.Errorf("invalid status %q (allowed: active|all|pending|in_progress|completed|cancelled)", s)
}

func validateListIDs(ids []string) error {
	if len(ids) > 500 {
		return fmt.Errorf("ids exceeds limit: %d > 500", len(ids))
	}
	for _, id := range ids {
		if err := validate.ID(id); err != nil {
			return err
		}
	}
	return nil
}

// resolveParentFilter 把字符串入参翻译为 ListOptions.ParentID 三态。
//   - ""      → nil（不过滤）
//   - "root"  → 指向 ""（IS NULL）
//   - 其他    → 指向该 id
func resolveParentFilter(s string) (*string, error) {
	switch s {
	case "":
		return nil, nil
	case "root":
		empty := ""
		return &empty, nil
	}
	if err := validate.ID(s); err != nil {
		return nil, err
	}
	id := s
	return &id, nil
}
