// Package mcp 把 store 层暴露为 7 个 MCP 工具（DESIGN.md §4）。
// Service 是 handler 的容器；通过依赖注入接受 store 与时钟函数（便于测试）。
package mcp

import (
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sumingcheng/tido/internal/store"
)

// Service 把 store 与时钟封装给 MCP handler。
type Service struct {
	store *store.Store
	now   func() int64 // unix ms；测试可注入固定值
}

// NewService 构造 Service。nowFn 传 nil 时用 time.Now。
func NewService(s *store.Store, nowFn func() int64) *Service {
	if nowFn == nil {
		nowFn = func() int64 { return time.Now().UnixMilli() }
	}
	return &Service{store: s, now: nowFn}
}

// Register 把所有工具挂到 server 上。
// 工具与文件一一对应（todo_write.go / todo_list.go ...）。
func Register(server *mcpsdk.Server, svc *Service) {
	mcpsdk.AddTool(server, todoWriteTool(), svc.todoWrite)
	mcpsdk.AddTool(server, todoListTool(), svc.todoList)
	mcpsdk.AddTool(server, todoUpdateTool(), svc.todoUpdate)
	mcpsdk.AddTool(server, todoDeleteTool(), svc.todoDelete)
	mcpsdk.AddTool(server, todoAddNoteTool(), svc.todoAddNote)
	mcpsdk.AddTool(server, todoGetNotesTool(), svc.todoGetNotes)
	mcpsdk.AddTool(server, todoDiffTool(), svc.todoDiff)
}
