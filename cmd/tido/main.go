// Tido MCP server: 给 LLM/Agent 提供本地 todo list 工具集（stdio 传输）。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version 编译时由 -ldflags "-X main.Version=..." 注入；默认 dev。
var Version = "dev"

type pingArgs struct{}

type pingResult struct {
	Pong    string `json:"pong"    jsonschema:"固定 pong 字符串"`
	Version string `json:"version" jsonschema:"tido 二进制版本"`
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ pingArgs) (*mcp.CallToolResult, pingResult, error) {
	r := pingResult{Pong: "pong", Version: Version}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("pong from tido %s", Version)},
		},
		StructuredContent: r,
	}, r, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "tido",
		Version: Version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "健康检查，返回 pong 与 tido 版本号",
	}, ping)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("tido server exited: %v", err)
		os.Exit(1)
	}
}
