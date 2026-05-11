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
	Scope    string `json:"scope,omitempty" jsonschema:"工作域，省略=default。"`
	Status   string `json:"status,omitempty" jsonschema:"按状态过滤：pending | in_progress | completed | cancelled。省略=不过滤。"`
	ParentID string `json:"parent_id,omitempty" jsonschema:"按父任务过滤；传 'root' 仅顶层；省略=不过滤。"`
	View     string `json:"view,omitempty" jsonschema:"compact (省元字段+相对时间) | full。省略=compact。"`
	Sort     string `json:"sort,omitempty" jsonschema:"created | priority | due。省略=created。"`
	Limit    int    `json:"limit,omitempty" jsonschema:"≤500，省略=100。"`
	Offset   int    `json:"offset,omitempty" jsonschema:"分页偏移；省略=0。"`
}

// TodoListResult 是 todo_list 的返回。
type TodoListResult struct {
	Items   []view.TodoView `json:"items"`
	Total   int             `json:"total"`
	HasMore bool            `json:"has_more"`
}

func todoListTool() *mcpsdk.Tool {
	return &mcpsdk.Tool{
		Name:        "todo_list",
		Description: "查询待办列表：可按 scope/status/parent_id 过滤，按 created|priority|due 排序，分页返回。view=compact 用于节省 LLM 上下文。",
	}
}

func (s *Service) todoList(ctx context.Context, _ *mcpsdk.CallToolRequest, args TodoListArgs) (*mcpsdk.CallToolResult, TodoListResult, error) {
	scope := resolveScope(args.Scope)
	if err := validate.Scope(scope); err != nil {
		return errResult[TodoListResult](err)
	}

	status, err := normalizeStatus(args.Status)
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

	out, err := s.store.List(ctx, store.ListOptions{
		Scope:    scope,
		Status:   status,
		ParentID: parentFilter,
		Sort:     sort,
		Limit:    args.Limit,
		Offset:   args.Offset,
	})
	if err != nil {
		return errResult[TodoListResult](err)
	}

	res := TodoListResult{
		Items:   view.RenderTodos(out.Items, mode, s.now()),
		Total:   out.Total,
		HasMore: out.HasMore,
	}
	summary := fmt.Sprintf("listed %d/%d todos in scope %q (sort=%s view=%s)",
		len(res.Items), res.Total, scope, sort, mode)
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
