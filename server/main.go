// Package main - 应用入口
// 解析命令行参数，创建 App 实例并启动 HTTP 服务
// 支持优雅关闭：监听 SIGINT/SIGTERM 信号
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"
)

// main - 应用入口函数
// 流程：解析参数 -> 创建 App -> 启动服务 -> 等待信号 -> 优雅关闭
func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	dataDir := flag.String("data", defaultDataDir(), "Application data directory")
	dbPath := flag.String("db", "", "SQLite database path (overrides -data)")
	sessionDays := flag.Int("session-days", 30, "Session cookie validity in days")
	flag.Parse()

	resolvedDataDir, resolvedDBPath := resolvePaths(*dataDir, *dbPath)

	svc, err := New(Config{
		Addr:        *addr,
		DataDir:     resolvedDataDir,
		DBPath:      resolvedDBPath,
		SessionDays: *sessionDays,
	})
	if err != nil {
		Error("create app failed: %v", err)
		log.Fatalf("create app: %v", err)
	}
	defer svc.Close()

	// 监听系统信号，实现优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := svc.Run(ctx); err != nil {
		Error("run app failed: %v", err)
		log.Fatalf("run app: %v", err)
	}
}
