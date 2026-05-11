package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTP endpoint：MCP Streamable HTTP 协议挂在此路径上。
const mcpPath = "/mcp"

// runHTTPServer 启动 HTTP transport：bearer auth 中间件 + Streamable HTTP handler。
// 必须传 token；空 token 拒绝启动，避免裸奔。
func runHTTPServer(ctx context.Context, server *mcpsdk.Server, addr, token string) error {
	if token == "" {
		return errors.New("HTTP mode requires non-empty TIDO_TOKEN env")
	}

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)

	mux := http.NewServeMux()
	mux.Handle(mcpPath, bearerAuth(token, mcpHandler))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 优雅停机：ctx 取消时 Shutdown，让正在处理的请求完成
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("tido HTTP listening on %s (POST %s, bearer auth required)", addr, mcpPath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

// bearerAuth 是一个最小 http.Handler 包装：检查 Authorization: Bearer <token>。
// 用 subtle.ConstantTimeCompare 防 timing attack。
func bearerAuth(token string, next http.Handler) http.Handler {
	tokenBytes := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, prefix) {
			unauthorized(w)
			return
		}
		got := []byte(strings.TrimPrefix(h, prefix))
		if subtle.ConstantTimeCompare(got, tokenBytes) != 1 {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="tido"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
