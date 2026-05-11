// Tido MCP server：给 LLM/Agent 提供本地 todo list 工具集（stdio 传输）。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	mcph "github.com/sumingcheng/tido/internal/mcp"
	"github.com/sumingcheng/tido/internal/store"
)

// Version 编译时由 -ldflags "-X main.Version=..." 注入；默认 dev。
var Version = "dev"

// 运行配置：db 路径解析顺序 = $TIDO_HOME/todos.db > ~/.tido/todos.db。
const dbFileName = "todos.db"

type pingArgs struct{}

type pingResult struct {
	Pong    string `json:"pong"    jsonschema:" 固定 pong 字符串"`
	Version string `json:"version" jsonschema:" tido 二进制版本"`
}

func ping(_ context.Context, _ *mcpsdk.CallToolRequest, _ pingArgs) (*mcpsdk.CallToolResult, pingResult, error) {
	r := pingResult{Pong: "pong", Version: Version}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf("pong from tido %s", Version)},
		},
		StructuredContent: r,
	}, r, nil
}

func main() {
	ctx := context.Background()

	dbPath, err := resolveDBPath()
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}
	st, err := store.New(ctx, dbPath)
	if err != nil {
		log.Fatalf("init store at %s: %v", dbPath, err)
	}
	defer st.Close()

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "tido",
		Version: Version,
	}, nil)

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "ping",
		Description: "健康检查，返回 pong 与 tido 版本号",
	}, ping)

	mcph.Register(server, mcph.NewService(st, nil))

	if err := server.Run(ctx, &mcpsdk.StdioTransport{}); err != nil {
		log.Printf("tido server exited: %v", err)
		os.Exit(1)
	}
}

// resolveDBPath 解析 db 文件路径：$TIDO_HOME 优先，否则 ~/.tido/todos.db。
func resolveDBPath() (string, error) {
	if home := os.Getenv("TIDO_HOME"); home != "" {
		return filepath.Join(home, dbFileName), nil
	}
	uhome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home: %w", err)
	}
	return filepath.Join(uhome, ".tido", dbFileName), nil
}
