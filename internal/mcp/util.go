package mcp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// 默认 scope；多数 agent 不传 scope，统一归入 "default"。
const defaultScope = "default"

// resolveScope 把可选 scope 入参映射到实际 scope。
func resolveScope(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultScope
	}
	return s
}

// parseDueAt 把入参字符串解析为 unix ms。
// 接受两种格式（DESIGN.md §12.4）：
//   - 纯数字串 → unix ms
//   - RFC3339 / ISO8601 串 → time.Parse 后转 ms
//
// 空串 → 返回 nil（表示不设/不变）。
func parseDueAt(s string) (*int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return &n, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("due_at: must be unix ms or RFC3339 string, got %q", s)
	}
	ms := t.UnixMilli()
	return &ms, nil
}

// errResult 把 error 包装为 MCP 协议的错误返回（Content+IsError=true）。
// 同时返回 zero-value 的 structured T，避免 schema 校验报错。
func errResult[T any](err error) (*mcpsdk.CallToolResult, T, error) {
	var zero T
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
	}, zero, nil
}

// okResult 构造一条带文本摘要 + structured payload 的成功返回。
// summary 给人/agent 一眼看懂；result 给程序解析。
func okResult[T any](summary string, result T) (*mcpsdk.CallToolResult, T, error) {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: summary}},
		StructuredContent: result,
	}, result, nil
}

// 编译时校验 errors 包可用（避免空 import 警告）；保留以备将来工具扩展使用。
var _ = errors.New
