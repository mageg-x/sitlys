package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

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
		log.Fatalf("create app: %v", err)
	}
	defer svc.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
}

func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "sitlys")
		}
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			return filepath.Join(homeDir, "AppData", "Roaming", "sitlys")
		}
	case "darwin":
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			return filepath.Join(homeDir, "Library", "Application Support", "sitlys")
		}
	default:
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			return filepath.Join(homeDir, ".sitlys")
		}
	}
	return "./data"
}

func resolvePaths(dataDir, dbPath string) (string, string) {
	if dbPath != "" {
		dbPath = filepath.Clean(dbPath)
		if dataDir == "" {
			dataDir = filepath.Dir(dbPath)
		}
		return filepath.Clean(dataDir), dbPath
	}
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	dataDir = filepath.Clean(dataDir)
	return dataDir, filepath.Join(dataDir, "sitlys.db")
}
