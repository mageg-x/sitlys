// Package main - 应用核心模块
// 包含 App 应用实例的创建、初始化、运行和关闭等核心逻辑
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed embed/* embed/**/*
var staticFiles embed.FS

//go:embed tracker.js
var trackerScript string

// New - 创建并初始化新的 App 应用实例
// 负责配置默认值、创建数据目录、打开数据库连接、初始化数据库结构、
// 加载地理位置数据库、设置静态文件服务和启动后台事件写入协程
//
// 参数:
//   - cfg: 应用配置
//
// 返回:
//   - 初始化完成的 App 实例
//   - 错误信息（如果初始化失败）
func New(cfg Config) (*App, error) {
	// 设置默认监听地址
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	// 设置默认数据目录
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(cfg.DBPath)
	}
	// 设置默认数据库路径
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "sitlys.db")
	}
	// 再次检查数据目录（当 DBPath 为空时 DataDir 可能仍为空）
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(cfg.DBPath)
	}
	// 设置默认会话有效期（天）
	if cfg.SessionDays <= 0 {
		cfg.SessionDays = 30
	}

	// 创建数据目录
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		Error("failed to create data directory: %v", err)
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	// 创建数据库文件所在目录
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		Error("failed to create database directory: %v", err)
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	// 打开 SQLite 数据库连接
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		Error("failed to open database: %v", err)
		return nil, fmt.Errorf("open db: %w", err)
	}
	// 配置数据库连接池参数
	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	// 初始化数据库编译指示（WAL 模式、外键等）
	if err := initDBPragmas(db, cfg.DBPath); err != nil {
		Error("failed to initialize database pragmas: %v", err)
		return nil, fmt.Errorf("init pragmas: %w", err)
	}

	// 构建 App 实例，初始化各组件
	app := &App{
		cfg:         cfg,
		db:          db,
		eventQueue:  make(chan queuedEvent, 8192),
		rateBuckets: make(map[string]rateBucket),
		botMode:     "balanced",
		botAudit:    make(map[string]int),
	}
	// 初始化数据库表结构
	if err := app.initSchema(); err != nil {
		Error("failed to initialize database schema: %v", err)
		db.Close()
		return nil, err
	}
	// 加载 GeoIP 地理位置数据库
	if err := app.reloadGeoIPDB(); err != nil {
		Error("failed to load geoip database: %v", err)
		db.Close()
		return nil, err
	}

	// 加载 QQWry 纯真 IP 数据库
	if err := app.reloadQQWryDB(); err != nil {
		Error("failed to load qqwry database: %v", err)
		db.Close()
		return nil, err
	}

	// 加载嵌入的静态文件系统
	sub, err := fs.Sub(staticFiles, "embed")
	if err != nil {
		Error("failed to load static filesystem: %v", err)
		db.Close()
		return nil, fmt.Errorf("static fs: %w", err)
	}
	app.staticFS = sub
	app.staticHTTP = http.FileServer(http.FS(sub))
	// 创建 HTTP 服务器
	app.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// 启动后台事件写入协程
	app.workerCtx, app.workerStop = context.WithCancel(context.Background())
	app.workerWG.Add(1)
	go app.runEventWriter()

	return app, nil
}

// Close - 关闭 App 应用实例
// 停止后台工作协程、关闭数据库连接、释放地理位置数据库资源
func (a *App) Close() error {
	// 停止后台事件写入协程
	if a.workerStop != nil {
		a.workerStop()
	}
	a.workerWG.Wait()
	// 关闭数据库连接
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			Error("failed to close database: %v", err)
			return err
		}
	}
	// 关闭地理位置数据库
	a.closeGeoIPDB()
	return nil
}

// initDBPragmas - 初始化 SQLite 数据库编译指示
// 设置外键约束、忙等待超时、WAL 日志模式等
func initDBPragmas(db *sql.DB, dbPath string) error {
	// 启用外键约束
	if _, err := db.Exec(`pragma foreign_keys = on;`); err != nil {
		Error("failed to enable foreign keys: %v", err)
		return err
	}
	// 设置忙等待超时时间（毫秒）
	if _, err := db.Exec(`pragma busy_timeout = 5000;`); err != nil {
		Error("failed to set busy timeout: %v", err)
		return err
	}

	// 尝试启用 WAL 日志模式（Write-Ahead Logging）
	var mode string
	if err := db.QueryRow(`pragma journal_mode = wal;`).Scan(&mode); err == nil {
		return nil
	}

	// WAL 不可用时回退到 DELETE 日志模式
	log.Printf("sqlite WAL unavailable for %s, falling back to DELETE journal mode", dbPath)
	if _, fallbackErr := db.Exec(`pragma journal_mode = delete;`); fallbackErr != nil {
		log.Printf("sqlite journal mode fallback failed for %s, continuing with driver default: %v", dbPath, fallbackErr)
		return nil
	}
	return nil
}

// Run - 启动 HTTP 服务器并等待关闭信号
// 在单独的 goroutine 中运行服务器，同时监听 context 取消信号以优雅关闭
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("sitlys %s listening on http://%s", version, a.cfg.Addr)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Error("server listen and serve failed: %v", err)
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		// 收到关闭信号，优雅关闭服务器（最多等待 10 秒）
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// routes - 注册所有 HTTP 路由
// 返回配置好路由的 HTTP Handler，包含健康检查、追踪脚本、数据采集、
// 用户认证、网站管理、分析报表等所有 API 端点
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /tracker.js", a.handleTracker)
	mux.HandleFunc("GET /collect/p/", a.handleCollectPixel)

	mux.HandleFunc("GET /api/status", a.handleStatus)
	mux.HandleFunc("POST /api/init", a.handleInit)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	mux.HandleFunc("POST /api/auth/password", a.handleChangePassword)

	mux.HandleFunc("GET /api/users", a.handleUsers)
	mux.HandleFunc("POST /api/users", a.handleUsers)
	mux.HandleFunc("PUT /api/users/", a.handleUserByID)

	mux.HandleFunc("GET /api/websites", a.handleWebsites)
	mux.HandleFunc("POST /api/websites", a.handleWebsites)
	mux.HandleFunc("PUT /api/websites/", a.handleWebsiteByID)
	mux.HandleFunc("DELETE /api/websites/", a.handleWebsiteByID)

	mux.HandleFunc("GET /api/websites/", a.handleNestedRoutes)
	mux.HandleFunc("POST /api/websites/", a.handleNestedRoutes)
	mux.HandleFunc("PUT /api/pixels/", a.handlePixelByID)
	mux.HandleFunc("PUT /api/shares/", a.handleShareByID)

	mux.HandleFunc("POST /api/send", a.handleSend)
	mux.HandleFunc("OPTIONS /api/send", a.handleSend)
	mux.HandleFunc("POST /api/batch", a.handleBatch)
	mux.HandleFunc("OPTIONS /api/batch", a.handleBatch)

	mux.HandleFunc("GET /api/settings", a.handleSettings)
	mux.HandleFunc("PUT /api/settings", a.handleSettings)
	mux.HandleFunc("POST /api/settings/backup", a.handleBackup)

	mux.HandleFunc("GET /api/analytics/overview", a.handleOverview)
	mux.HandleFunc("GET /api/analytics/pages", a.handlePages)
	mux.HandleFunc("GET /api/analytics/events", a.handleEvents)
	mux.HandleFunc("GET /api/analytics/referrers", a.handleReferrers)
	mux.HandleFunc("GET /api/analytics/devices", a.handleDevices)
	mux.HandleFunc("GET /api/analytics/geo", a.handleGeo)
	mux.HandleFunc("GET /api/analytics/attribution", a.handleAttribution)
	mux.HandleFunc("GET /api/analytics/retention", a.handleRetention)
	mux.HandleFunc("GET /api/analytics/revenue", a.handleRevenue)
	mux.HandleFunc("GET /api/analytics/funnel", a.handleFunnelReport)
	mux.HandleFunc("GET /api/analytics/realtime", a.handleRealtime)
	mux.HandleFunc("GET /api/analytics/export", a.handleExport)

	mux.HandleFunc("GET /api/public/shares/", a.handlePublicShare)
	mux.HandleFunc("POST /api/settings/cleanup", a.handleCleanup)
	mux.HandleFunc("/", a.handleApp)
	return withLogging(mux)
}

// hasUsers - 检查系统中是否已存在用户
// 用于判断系统是否已完成初始化
func (a *App) hasUsers() (bool, error) {
	var count int
	if err := a.db.QueryRow(`select count(*) from users`).Scan(&count); err != nil {
		Error("failed to query user count: %v", err)
		return false, err
	}
	return count > 0, nil
}

// setSessionCookie - 设置会话 Cookie
// 将认证令牌写入客户端的 HTTP Cookie 中
func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  expires,
	})
}

// clearSessionCookieForRequest - 清除会话 Cookie
// 通过设置 MaxAge=-1 使客户端的会话 Cookie 立即过期
func (a *App) clearSessionCookieForRequest(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		MaxAge:   -1,
	})
}

// createSession - 创建新的用户会话
// 生成认证令牌并存入数据库，返回令牌和过期时间
func (a *App) createSession(userID string) (string, time.Time, error) {
	// 清理已过期的会话
	if err := a.cleanupExpiredSessions(); err != nil {
		Error("failed to cleanup expired sessions: %v", err)
		return "", time.Time{}, err
	}
	token := newID() + newID()
	expires := nowUTC().Add(time.Duration(a.cfg.SessionDays) * 24 * time.Hour)
	_, err := a.db.Exec(`
		insert into auth_sessions(id, user_id, token_hash, expires_at, created_at)
		values(?, ?, ?, ?, ?)
	`, newID(), userID, tokenHash(token), iso(expires), iso(nowUTC()))
	return token, expires, err
}

// cleanupExpiredSessions - 清理已过期的会话记录
func (a *App) cleanupExpiredSessions() error {
	_, err := a.db.Exec(`delete from auth_sessions where expires_at < ?`, iso(nowUTC()))
	return err
}

// currentUser - 从请求的 Cookie 中获取当前登录用户
// 通过会话令牌查找对应的用户信息，包括角色和网站权限
func (a *App) currentUser(r *http.Request) (*AuthUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}

	// 通过令牌哈希查找有效会话及对应用户
	row := a.db.QueryRow(`
		select u.id, u.username, u.role, u.enabled, u.created_at
		from auth_sessions s
		join users u on u.id = s.user_id
		where s.token_hash = ? and s.expires_at >= ?
	`, tokenHash(cookie.Value), iso(nowUTC()))

	user := &AuthUser{}
	var enabled int
	if err := row.Scan(&user.ID, &user.Username, &user.Role, &enabled, &user.CreatedAt); err != nil {
		return nil, err
	}
	user.Enabled = enabled == 1
	// 超级管理员拥有所有网站的访问权限
	user.AllWebsites = user.Role == roleSuperAdmin
	// 加载用户的网站权限列表
	perms, err := a.permissionsForUser(user.ID)
	if err != nil {
		Error("failed to load user permissions: %v", err)
		return nil, err
	}
	user.Permissions = perms
	return user, nil
}

// requireUser - 验证请求是否包含有效的用户认证
// 如果未认证或用户被禁用，返回 401 错误
func (a *App) requireUser(w http.ResponseWriter, r *http.Request) (*AuthUser, bool) {
	user, err := a.currentUser(r)
	if err != nil || !user.Enabled {
		errorResponse(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return user, true
}

// permissionsForUser - 获取用户的网站权限列表
func (a *App) permissionsForUser(userID string) ([]WebsitePermission, error) {
	rows, err := a.db.Query(`
		select website_id, access_level
		from website_permissions
		where user_id = ?
		order by website_id
	`, userID)
	if err != nil {
		Error("failed to query user permissions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var out []WebsitePermission
	for rows.Next() {
		var perm WebsitePermission
		if err := rows.Scan(&perm.WebsiteID, &perm.AccessLevel); err != nil {
			Error("failed to scan permission row: %v", err)
			return nil, err
		}
		if perm.AccessLevel == "" {
			perm.AccessLevel = "view"
		}
		out = append(out, perm)
	}
	return out, rows.Err()
}

// canViewWebsite - 检查用户是否有查看指定网站的权限
func (a *App) canViewWebsite(user *AuthUser, websiteID string) bool {
	if user.Role == roleSuperAdmin {
		return true
	}
	for _, perm := range user.Permissions {
		if perm.WebsiteID == websiteID && (perm.AccessLevel == "view" || perm.AccessLevel == "manage") {
			return true
		}
	}
	return false
}

// canManageWebsite - 检查用户是否有管理指定网站的权限
func (a *App) canManageWebsite(user *AuthUser, websiteID string) bool {
	if user.Role == roleSuperAdmin {
		return true
	}
	if user.Role == roleViewer {
		return false
	}
	for _, perm := range user.Permissions {
		if perm.WebsiteID == websiteID && perm.AccessLevel == "manage" {
			return true
		}
	}
	return false
}

// requireWebsiteView - 验证用户是否有查看指定网站的权限
// 无权限时返回 403 错误
func (a *App) requireWebsiteView(w http.ResponseWriter, user *AuthUser, websiteID string) bool {
	if !a.canViewWebsite(user, websiteID) {
		errorResponse(w, http.StatusForbidden, "no access to website")
		return false
	}
	return true
}

// requireWebsiteManage - 验证用户是否有管理指定网站的权限
// 无权限时返回 403 错误
func (a *App) requireWebsiteManage(w http.ResponseWriter, user *AuthUser, websiteID string) bool {
	if !a.canManageWebsite(user, websiteID) {
		errorResponse(w, http.StatusForbidden, "manage permission required")
		return false
	}
	return true
}

// parseDateRange - 解析请求中的日期范围参数
// 从查询参数中提取 from 和 to，默认返回最近 30 天的范围
func (a *App) parseDateRange(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	now := nowUTC()
	end := now
	start := now.AddDate(0, 0, -30)

	if from != "" {
		t, err := parseDateInput(from)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start = t
	}
	if to != "" {
		t, err := parseDateInput(to)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end = t
		// 如果只提供了日期（无时间部分），将结束时间设为当天最后一秒
		if len(to) == len("2006-01-02") {
			end = end.Add(24*time.Hour - time.Second)
		}
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid date range")
	}
	return start.UTC(), end.UTC(), nil
}

// allowCollectionRequest - 检查数据采集请求是否超过速率限制
// 对网站、IP 和客户端三个维度分别进行速率限制检查
func (a *App) allowCollectionRequest(r *http.Request, websiteID string, cost int) bool {
	ip := clientIP(r)
	userAgent := strings.TrimSpace(r.UserAgent())
	websiteKey := "collection:website:" + websiteID
	ipKey := "collection:ip:" + ip
	clientKey := "collection:client:" + websiteID + ":" + ip + ":" + tokenHash(userAgent)[:16]
	// 网站级别：每分钟最多 1200 次
	if !a.allowRateLimit(websiteKey, 1200, cost, time.Minute) {
		return false
	}
	// IP 级别：每分钟最多 240 次
	if !a.allowRateLimit(ipKey, 240, cost, time.Minute) {
		return false
	}
	// 客户端级别：每分钟最多 120 次
	if !a.allowRateLimit(clientKey, 120, cost, time.Minute) {
		return false
	}
	return true
}

// getSystemSettings - 获取系统设置
// 从数据库读取所有系统配置项并返回结构化的设置对象
func (a *App) getSystemSettings() (SystemSettings, error) {
	settings := SystemSettings{
		ListenAddr:        a.cfg.Addr,
		DatabasePath:      a.cfg.DBPath,
		GeoIPDatabasePath: a.cfg.GeoIPDBPath,
		LogLevel:          "info",
		DataRetentionDays: 365,
		BotFilterMode:     "balanced",
	}
	rows, err := a.db.Query(`
		select key, value, updated_at
		from system_settings
	`)
	if err != nil {
		Error("failed to query system settings: %v", err)
		return settings, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value, updatedAt string
		if err := rows.Scan(&key, &value, &updatedAt); err != nil {
			Error("failed to scan system setting row: %v", err)
			return settings, err
		}
		switch key {
		case "listen_addr":
			settings.ListenAddr = value
		case "database_path":
			settings.DatabasePath = value
		case "geoip_database_path":
			settings.GeoIPDatabasePath = value
		case "log_level":
			settings.LogLevel = value
		case "data_retention_days":
			if days, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && days > 0 {
				settings.DataRetentionDays = days
			}
		case "bot_filter_mode":
			if strings.TrimSpace(value) != "" {
				settings.BotFilterMode = strings.TrimSpace(value)
			}
		case "last_cleanup_at":
			settings.LastCleanupAt = value
		}
		if updatedAt > settings.UpdatedAt {
			settings.UpdatedAt = updatedAt
		}
	}
	return settings, rows.Err()
}

// setSystemSettings - 保存系统设置
// 将设置键值对写入数据库，并处理 GeoIP 路径变更和机器人过滤模式更新
func (a *App) setSystemSettings(values map[string]string) error {
	now := iso(nowUTC())
	nextGeoIPPath, hasGeoIPPath := values["geoip_database_path"]
	nextBotMode, hasBotMode := values["bot_filter_mode"]
	tx, err := a.db.Begin()
	if err != nil {
		Error("failed to begin transaction for settings: %v", err)
		return err
	}
	defer tx.Rollback()

	for key, value := range values {
		if _, err := tx.Exec(`
			insert into system_settings(key, value, updated_at)
			values(?, ?, ?)
			on conflict(key) do update set
				value = excluded.value,
				updated_at = excluded.updated_at
		`, key, value, now); err != nil {
			Error("failed to upsert system setting %s: %v", key, err)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		Error("failed to commit settings transaction: %v", err)
		return err
	}
	// 如果 GeoIP 路径变更，重新加载 GeoIP 数据库
	if hasGeoIPPath {
		a.cfg.GeoIPDBPath = strings.TrimSpace(nextGeoIPPath)
		if err := a.reloadGeoIPDB(); err != nil {
			Error("failed to reload geoip database after settings change: %v", err)
			return err
		}
	}
	if hasBotMode {
		a.updateBotFilterModeCache(nextBotMode)
	}
	return nil
}

// createBackup - 创建数据库备份
// 执行 WAL 检查点后将数据库 VACUUM 到备份文件
func (a *App) createBackup() (string, error) {
	backupDir := filepath.Join(a.cfg.DataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		Error("failed to create backup directory: %v", err)
		return "", err
	}
	filename := fmt.Sprintf("sitlys-%s.db", nowUTC().Format("20060102-150405"))
	targetPath := filepath.Join(backupDir, filename)
	// 安全检查：确保备份路径在备份目录内
	cleanBackupDir := filepath.Clean(backupDir)
	cleanTargetPath := filepath.Clean(targetPath)
	if filepath.Dir(cleanTargetPath) != cleanBackupDir {
		return "", fmt.Errorf("invalid backup target path")
	}
	if strings.ContainsAny(cleanTargetPath, "'\x00\r\n") {
		return "", fmt.Errorf("invalid backup target path")
	}
	// 执行 WAL 检查点，将所有 WAL 数据写入主数据库文件
	if _, err := a.db.Exec(`pragma wal_checkpoint(full);`); err != nil {
		Error("failed to execute wal checkpoint for backup: %v", err)
		return "", err
	}
	// 使用 VACUUM INTO 创建备份
	if _, err := a.db.Exec(`vacuum into ` + sqliteStringLiteral(targetPath)); err != nil {
		Error("failed to vacuum into backup file: %v", err)
		return "", err
	}
	return targetPath, nil
}

// shouldIgnoreBotTraffic - 判断是否应忽略机器人流量
// 根据当前的机器人过滤模式检查请求是否为机器人流量
func (a *App) shouldIgnoreBotTraffic(r *http.Request) (bool, string) {
	mode := a.botFilterModeValue()
	if mode == "off" {
		return false, ""
	}
	// 检查是否为预取请求
	if isBotRequest(r) {
		return true, "prefetch"
	}
	// 检查 User-Agent 是否为已知机器人
	if isBotTraffic(r.UserAgent()) {
		return true, "bot"
	}
	// 严格模式下额外检查预览机器人
	if mode == "strict" && isPreviewBotTraffic(r.UserAgent()) {
		return true, "preview_bot"
	}
	return false, ""
}

// updateBotFilterModeCache - 更新机器人过滤模式缓存
func (a *App) updateBotFilterModeCache(mode string) {
	if a == nil {
		return
	}
	a.botModeMu.Lock()
	defer a.botModeMu.Unlock()
	a.botMode = normalizeBotFilterMode(mode)
	a.botModeAt = nowUTC()
}

// botFilterModeValue - 获取当前的机器人过滤模式
// 优先使用缓存值，缓存过期后从数据库重新加载
func (a *App) botFilterModeValue() string {
	if a == nil {
		return "balanced"
	}

	a.botModeMu.RLock()
	mode := a.botMode
	loadedAt := a.botModeAt
	a.botModeMu.RUnlock()
	if mode == "" {
		mode = "balanced"
	}
	// 缓存有效期内直接返回
	if !loadedAt.IsZero() && time.Since(loadedAt) < 5*time.Second {
		return mode
	}
	if a.db == nil {
		return mode
	}
	// 缓存过期，从数据库重新加载
	settings, err := a.getSystemSettings()
	if err != nil {
		return mode
	}
	a.updateBotFilterModeCache(settings.BotFilterMode)

	a.botModeMu.RLock()
	defer a.botModeMu.RUnlock()
	if a.botMode == "" {
		return "balanced"
	}
	return a.botMode
}

// recordBotAudit - 记录机器人审计信息
func (a *App) recordBotAudit(reason string) {
	if reason == "" {
		return
	}
	a.botAuditMu.Lock()
	defer a.botAuditMu.Unlock()
	a.botAudit[reason]++
}

// botAuditSnapshot - 获取机器人审计快照
func (a *App) botAuditSnapshot() map[string]int {
	a.botAuditMu.Lock()
	defer a.botAuditMu.Unlock()
	out := make(map[string]int, len(a.botAudit))
	for key, value := range a.botAudit {
		out[key] = value
	}
	return out
}

// allowRateLimit - 检查并更新速率限制
// 使用滑动窗口算法，在指定时间窗口内限制请求次数
func (a *App) allowRateLimit(key string, limit, cost int, window time.Duration) bool {
	if key == "" {
		return true
	}
	if cost <= 0 {
		cost = 1
	}
	now := nowUTC()

	a.rateMu.Lock()
	defer a.rateMu.Unlock()

	// 清理已过期的速率桶
	for itemKey, bucket := range a.rateBuckets {
		if now.After(bucket.ResetAt) {
			delete(a.rateBuckets, itemKey)
		}
	}
	bucket, ok := a.rateBuckets[key]
	// 桶数量达到上限时，淘汰最旧的桶
	if !ok && len(a.rateBuckets) >= maxRateBuckets {
		for len(a.rateBuckets) >= maxRateBuckets {
			oldestKey := ""
			var oldestReset time.Time
			for itemKey, candidate := range a.rateBuckets {
				if oldestKey == "" || candidate.ResetAt.Before(oldestReset) {
					oldestKey = itemKey
					oldestReset = candidate.ResetAt
				}
			}
			if oldestKey == "" {
				break
			}
			delete(a.rateBuckets, oldestKey)
		}
	}
	// 创建或重置速率桶
	if !ok || now.After(bucket.ResetAt) {
		bucket = rateBucket{ResetAt: now.Add(window)}
	}
	// 检查是否超过限制
	if bucket.Count+cost > limit {
		a.rateBuckets[key] = bucket
		return false
	}
	bucket.Count += cost
	a.rateBuckets[key] = bucket
	return true
}

// cleanupHistoricalData - 清理历史数据
// 根据保留天数删除过期的事件、会话、访客和聚合数据
func (a *App) cleanupHistoricalData(retentionDays int) (map[string]any, error) {
	if retentionDays <= 0 {
		retentionDays = 365
	}
	cutoff := nowUTC().AddDate(0, 0, -retentionDays)
	cutoffDate := cutoff.Format("2006-01-02")
	tx, err := a.db.Begin()
	if err != nil {
		Error("failed to begin transaction for cleanup: %v", err)
		return nil, err
	}
	defer tx.Rollback()

	result := map[string]any{
		"retention_days": retentionDays,
		"cutoff_at":      iso(cutoff),
	}

	// 辅助函数：执行删除并记录影响行数
	collectDelete := func(key, query string, args ...any) error {
		res, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		result[key] = count
		return nil
	}

	// 删除过期事件
	if err := collectDelete("deleted_events", `delete from events where created_at < ?`, iso(cutoff)); err != nil {
		Error("failed to delete expired events: %v", err)
		return nil, err
	}
	// 删除过期会话
	if err := collectDelete("deleted_sessions", `delete from sessions where last_seen_at < ?`, iso(cutoff)); err != nil {
		Error("failed to delete expired sessions: %v", err)
		return nil, err
	}
	// 删除过期访客（且无关联会话的）
	if err := collectDelete("deleted_visitors", `delete from visitors where last_seen_at < ? and not exists (select 1 from sessions where sessions.visitor_id = visitors.id)`, iso(cutoff)); err != nil {
		Error("failed to delete expired visitors: %v", err)
		return nil, err
	}

	// 删除过期的聚合数据
	aggregateTables := []string{
		"agg_overview_daily",
		"agg_visitor_daily",
		"agg_pages_daily",
		"agg_referrers_daily",
		"agg_devices_daily",
		"agg_geo_daily",
		"agg_attribution_daily",
		"agg_revenue_daily",
	}
	var aggregateRows int64
	for _, table := range aggregateTables {
		res, err := tx.Exec(`delete from `+table+` where bucket_date < ?`, cutoffDate)
		if err != nil {
			Error("failed to delete from aggregate table %s: %v", table, err)
			return nil, err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		aggregateRows += count
	}
	result["deleted_aggregate_rows"] = aggregateRows

	// 记录清理时间
	cleanupAt := iso(nowUTC())
	if _, err := tx.Exec(`
		insert into system_settings(key, value, updated_at)
		values('last_cleanup_at', ?, ?)
		on conflict(key) do update set
			value = excluded.value,
			updated_at = excluded.updated_at
	`, cleanupAt, cleanupAt); err != nil {
		Error("failed to record cleanup timestamp: %v", err)
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		Error("failed to commit cleanup transaction: %v", err)
		return nil, err
	}
	result["last_cleanup_at"] = cleanupAt
	return result, nil
}
