// Package main - HTTP 请求处理模块
// 包含所有 API 端点的处理函数，包括用户认证、网站管理、数据采集和分析报表
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// onePixelGIF - 1x1 透明 GIF 像素图片数据
// 用于像素追踪响应，最小化网络传输开销
var onePixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00,
	0x01, 0x00, 0x80, 0x00, 0x00, 0xff, 0xff, 0xff,
	0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00, 0x3b,
}

// 请求/响应类型定义

// createInitRequest - 系统初始化请求
type createInitRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginRequest - 用户登录请求
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// changePasswordRequest - 修改密码请求
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// userInput - 用户创建/更新输入
type userInput struct {
	Username    string              `json:"username"`
	Password    string              `json:"password"`
	Role        string              `json:"role"`
	Enabled     *bool               `json:"enabled,omitempty"`
	Permissions []WebsitePermission `json:"permissions"`
}

// websiteInput - 网站创建/更新输入
type websiteInput struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// pixelInput - 像素创建/更新输入
type pixelInput struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// shareInput - 分享链接更新输入
type shareInput struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// funnelInput - 漏斗创建/更新输入
type funnelInput struct {
	Name  string       `json:"name"`
	Steps []FunnelStep `json:"steps"`
}

// settingsInput - 系统设置更新输入
type settingsInput struct {
	ListenAddr        string `json:"listen_addr"`
	DatabasePath      string `json:"database_path"`
	GeoIPDatabasePath string `json:"geoip_database_path"`
	LogLevel          string `json:"log_level"`
	DataRetentionDays int    `json:"data_retention_days"`
	BotFilterMode     string `json:"bot_filter_mode"`
}

// handleHealth - 健康检查端点
// GET /healthz - 返回服务版本和运行状态
func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version,
	})
}

// handleTracker - 追踪脚本端点
// GET /tracker.js - 返回前端追踪 JavaScript 脚本
func (a *App) handleTracker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	http.ServeContent(w, r, "tracker.js", nowUTC(), strings.NewReader(trackerScript))
}

// setCollectionCORS - 设置数据采集接口的 CORS 响应头
// 允许跨域请求，支持携带凭据
func setCollectionCORS(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Vary", "Origin")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// handleApp - SPA 前端应用路由
// 处理前端单页应用的路由，包括首页、分享页面和静态资源
func (a *App) handleApp(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/", r.URL.Path == "/index.html":
		a.serveStaticFile(w, r, "index.html", "text/html; charset=utf-8")
		return
	case strings.HasPrefix(r.URL.Path, "/share/"):
		a.serveStaticFile(w, r, "index.html", "text/html; charset=utf-8")
		return
	case strings.HasPrefix(r.URL.Path, "/assets/"):
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		contentType := "text/plain; charset=utf-8"
		switch {
		case strings.HasSuffix(name, ".js"):
			contentType = "application/javascript; charset=utf-8"
		case strings.HasSuffix(name, ".css"):
			contentType = "text/css; charset=utf-8"
		}
		a.serveStaticFile(w, r, path.Join("assets", name), contentType)
		return
	default:
		http.NotFound(w, r)
	}
}

// serveStaticFile - 提供静态文件服务
// 从嵌入的文件系统中读取文件并返回给客户端
func (a *App) serveStaticFile(w http.ResponseWriter, r *http.Request, name, contentType string) {
	data, err := fs.ReadFile(a.staticFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, name, nowUTC(), strings.NewReader(string(data)))
}

// handleStatus - 系统状态端点
// GET /api/status - 返回系统版本和初始化状态
func (a *App) handleStatus(w http.ResponseWriter, _ *http.Request) {
	hasUsers, err := a.hasUsers()
	if err != nil {
		Error("failed to check users for status: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":          true,
		"version":     version,
		"initialized": hasUsers,
	})
}

// handleInit - 系统初始化端点
// POST /api/init - 创建首个超级管理员账户并完成系统初始化
func (a *App) handleInit(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := a.hasUsers()
	if err != nil {
		Error("failed to check users for init: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hasUsers {
		errorResponse(w, http.StatusConflict, "system already initialized")
		return
	}

	var req createInitRequest
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		errorResponse(w, http.StatusBadRequest, "username and password(min 8) required")
		return
	}
	hash, err := passwordHash(req.Password)
	if err != nil {
		Error("failed to hash password for init: %v", err)
		errorResponse(w, http.StatusInternalServerError, "hash password failed")
		return
	}
	now := iso(nowUTC())
	userID := newID()
	_, err = a.db.Exec(`
		insert into users(id, username, password_hash, role, enabled, created_at, updated_at)
		values(?, ?, ?, ?, 1, ?, ?)
	`, userID, req.Username, hash, roleSuperAdmin, now, now)
	if err != nil {
		Error("failed to create admin user: %v", err)
		errorResponse(w, http.StatusInternalServerError, "create admin failed")
		return
	}

	token, expires, err := a.createSession(userID)
	if err != nil {
		Error("failed to create session for init: %v", err)
		errorResponse(w, http.StatusInternalServerError, "create session failed")
		return
	}
	a.setSessionCookie(w, r, token, expires)
	jsonResponse(w, http.StatusCreated, map[string]any{"ok": true})
}

// handleLogin - 用户登录端点
// POST /api/auth/login - 验证用户名密码并创建会话
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var userID, username, role, hash string
	var enabled int
	err := a.db.QueryRow(`
		select id, username, role, password_hash, enabled
		from users
		where username = ?
	`, strings.TrimSpace(req.Username)).Scan(&userID, &username, &role, &hash, &enabled)
	if err != nil {
		errorResponse(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if enabled != 1 || !passwordMatch(hash, req.Password) {
		errorResponse(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, expires, err := a.createSession(userID)
	if err != nil {
		Error("failed to create session for login: %v", err)
		errorResponse(w, http.StatusInternalServerError, "create session failed")
		return
	}
	a.setSessionCookie(w, r, token, expires)
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// handleLogout - 用户登出端点
// POST /api/auth/logout - 删除会话令牌并清除 Cookie
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		_, _ = a.db.Exec(`delete from auth_sessions where token_hash = ?`, tokenHash(cookie.Value))
	}
	a.clearSessionCookieForRequest(w, r)
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMe - 当前用户信息端点
// GET /api/auth/me - 返回当前登录用户的详细信息
func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":   true,
		"user": user,
	})
}

// handleChangePassword - 修改密码端点
// POST /api/auth/password - 验证当前密码后更新为新密码
func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}

	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		errorResponse(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	var hash string
	if err := a.db.QueryRow(`select password_hash from users where id = ?`, user.ID).Scan(&hash); err != nil {
		Error("failed to load user password hash: %v", err)
		errorResponse(w, http.StatusInternalServerError, "load user failed")
		return
	}
	if !passwordMatch(hash, req.CurrentPassword) {
		errorResponse(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	newHash, err := passwordHash(req.NewPassword)
	if err != nil {
		Error("failed to hash new password: %v", err)
		errorResponse(w, http.StatusInternalServerError, "hash password failed")
		return
	}
	if _, err := a.db.Exec(`
		update users
		set password_hash = ?, updated_at = ?
		where id = ?
	`, newHash, iso(nowUTC()), user.ID); err != nil {
		Error("failed to update password: %v", err)
		errorResponse(w, http.StatusInternalServerError, "update password failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// handleUsers - 用户管理端点
// GET /api/users - 获取所有用户列表（仅超级管理员）
// POST /api/users - 创建新用户（仅超级管理员）
func (a *App) handleUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if user.Role != roleSuperAdmin {
		errorResponse(w, http.StatusForbidden, "super admin required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(`
			select id, username, role, enabled, created_at
			from users
			order by created_at asc
		`)
		if err != nil {
			Error("failed to query users: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var users []AuthUser
		for rows.Next() {
			var item AuthUser
			var enabled int
			if err := rows.Scan(&item.ID, &item.Username, &item.Role, &enabled, &item.CreatedAt); err != nil {
				Error("failed to scan user row: %v", err)
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			item.Enabled = enabled == 1
			item.AllWebsites = item.Role == roleSuperAdmin
			item.Permissions, err = a.permissionsForUser(item.ID)
			if err != nil {
				Error("failed to load permissions for user %s: %v", item.ID, err)
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			users = append(users, item)
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "users": users})
	case http.MethodPost:
		var req userInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := validateUserInput(req, true); err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := passwordHash(req.Password)
		if err != nil {
			Error("failed to hash password for new user: %v", err)
			errorResponse(w, http.StatusInternalServerError, "hash password failed")
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		userID := newID()
		now := iso(nowUTC())
		tx, err := a.db.Begin()
		if err != nil {
			Error("failed to begin transaction for user creation: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer tx.Rollback()

		if _, err := tx.Exec(`
			insert into users(id, username, password_hash, role, enabled, created_at, updated_at)
			values(?, ?, ?, ?, ?, ?, ?)
		`, userID, strings.TrimSpace(req.Username), hash, req.Role, boolInt(enabled), now, now); err != nil {
			errorResponse(w, http.StatusBadRequest, "create user failed")
			return
		}
		if err := upsertPermissions(tx, userID, req.Permissions); err != nil {
			Error("failed to upsert permissions for new user: %v", err)
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := tx.Commit(); err != nil {
			Error("failed to commit user creation: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": userID})
	default:
		http.NotFound(w, r)
	}
}

// handleUserByID - 单个用户管理端点
// PUT /api/users/{id} - 更新指定用户的信息（仅超级管理员）
func (a *App) handleUserByID(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if user.Role != roleSuperAdmin {
		errorResponse(w, http.StatusForbidden, "super admin required")
		return
	}
	userID := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if userID == "" {
		http.NotFound(w, r)
		return
	}

	var req userInput
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateUserInput(req, false); err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		Error("failed to begin transaction for user update: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var currentRole string
	var currentEnabled int
	if err := tx.QueryRow(`select role, enabled from users where id = ?`, userID).Scan(&currentRole, &currentEnabled); err != nil {
		errorResponse(w, http.StatusNotFound, "user not found")
		return
	}

	nextRole := currentRole
	if req.Role != "" {
		nextRole = req.Role
	}
	nextEnabled := currentEnabled == 1
	if req.Enabled != nil {
		nextEnabled = *req.Enabled
	}
	// 防止超级管理员移除自己的权限
	if user.ID == userID && (nextRole != roleSuperAdmin || !nextEnabled) {
		errorResponse(w, http.StatusBadRequest, "cannot remove your own super admin access")
		return
	}
	// 确保至少保留一个启用的超级管理员
	if currentRole == roleSuperAdmin && (nextRole != roleSuperAdmin || !nextEnabled) {
		var enabledSuperAdmins int
		if err := tx.QueryRow(`select count(*) from users where role = ? and enabled = 1`, roleSuperAdmin).Scan(&enabledSuperAdmins); err != nil {
			Error("failed to count super admins: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if enabledSuperAdmins <= 1 {
			errorResponse(w, http.StatusBadRequest, "at least one enabled super admin is required")
			return
		}
	}

	// 构建动态更新 SQL
	var parts []string
	var args []any
	if strings.TrimSpace(req.Username) != "" {
		parts = append(parts, "username = ?")
		args = append(args, strings.TrimSpace(req.Username))
	}
	if req.Role != "" {
		parts = append(parts, "role = ?")
		args = append(args, req.Role)
	}
	if req.Enabled != nil {
		parts = append(parts, "enabled = ?")
		args = append(args, boolInt(*req.Enabled))
	}
	if req.Password != "" {
		hash, err := passwordHash(req.Password)
		if err != nil {
			Error("failed to hash password for user update: %v", err)
			errorResponse(w, http.StatusInternalServerError, "hash password failed")
			return
		}
		parts = append(parts, "password_hash = ?")
		args = append(args, hash)
	}
	changedFields := len(parts)
	parts = append(parts, "updated_at = ?")
	args = append(args, iso(nowUTC()), userID)
	if changedFields > 0 {
		if _, err := tx.Exec(`update users set `+strings.Join(parts, ", ")+` where id = ?`, args...); err != nil {
			errorResponse(w, http.StatusBadRequest, "update user failed")
			return
		}
		// 禁用用户时撤销其所有会话
		if req.Enabled != nil && !*req.Enabled {
			if _, err := tx.Exec(`delete from auth_sessions where user_id = ?`, userID); err != nil {
				Error("failed to revoke user sessions: %v", err)
				errorResponse(w, http.StatusInternalServerError, "revoke user sessions failed")
				return
			}
		}
	}
	if req.Permissions != nil {
		if err := upsertPermissions(tx, userID, req.Permissions); err != nil {
			Error("failed to upsert permissions for user update: %v", err)
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		Error("failed to commit user update: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// validateUserInput - 验证用户输入数据
func validateUserInput(req userInput, create bool) error {
	if create && strings.TrimSpace(req.Username) == "" {
		return fmt.Errorf("username required")
	}
	if create && len(req.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if req.Role != "" && !isValidRole(req.Role) {
		return fmt.Errorf("invalid role")
	}
	for _, perm := range req.Permissions {
		switch perm.AccessLevel {
		case "view", "manage":
		default:
			return fmt.Errorf("invalid access level")
		}
	}
	return nil
}

// upsertPermissions - 更新用户网站权限
// 先删除用户的所有现有权限，再插入新的权限列表
func upsertPermissions(tx *sql.Tx, userID string, permissions []WebsitePermission) error {
	if _, err := tx.Exec(`delete from website_permissions where user_id = ?`, userID); err != nil {
		Error("failed to delete existing permissions: %v", err)
		return err
	}
	now := iso(nowUTC())
	for _, perm := range permissions {
		if strings.TrimSpace(perm.WebsiteID) == "" {
			return fmt.Errorf("website permission requires website_id")
		}
		if _, err := tx.Exec(`
			insert into website_permissions(user_id, website_id, access_level, created_at)
			values(?, ?, ?, ?)
		`, userID, perm.WebsiteID, perm.AccessLevel, now); err != nil {
			Error("failed to insert permission: %v", err)
			return err
		}
	}
	return nil
}

// handleWebsites - 网站管理端点
// GET /api/websites - 获取用户可访问的网站列表
// POST /api/websites - 创建新网站（管理员及以上）
func (a *App) handleWebsites(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		websites, err := a.listWebsites(user)
		if err != nil {
			Error("failed to list websites: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "websites": websites})
	case http.MethodPost:
		if user.Role != roleSuperAdmin && user.Role != roleAdmin {
			errorResponse(w, http.StatusForbidden, "admin permission required")
			return
		}
		var req websiteInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Domain = normalizeWebsiteDomain(req.Domain)
		if req.Name == "" || req.Domain == "" {
			errorResponse(w, http.StatusBadRequest, "name and domain required")
			return
		}
		now := iso(nowUTC())
		websiteID := newID()
		tx, err := a.db.Begin()
		if err != nil {
			Error("failed to begin transaction for website creation: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`
			insert into websites(id, name, domain, created_at, updated_at)
			values(?, ?, ?, ?, ?)
		`, websiteID, req.Name, req.Domain, now, now); err != nil {
			errorResponse(w, http.StatusBadRequest, "create website failed")
			return
		}
		// 非超级管理员创建网站时自动授予管理权限
		if user.Role != roleSuperAdmin {
			if _, err := tx.Exec(`
				insert into website_permissions(user_id, website_id, access_level, created_at)
				values(?, ?, ?, ?)
			`, user.ID, websiteID, "manage", now); err != nil {
				Error("failed to assign permission for website creator: %v", err)
				errorResponse(w, http.StatusInternalServerError, "assign permission failed")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			Error("failed to commit website creation: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": websiteID})
	default:
		http.NotFound(w, r)
	}
}

// handleWebsiteByID - 单个网站管理端点
// PUT /api/websites/{id} - 更新网站信息
// DELETE /api/websites/{id} - 删除网站
func (a *App) handleWebsiteByID(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	websiteID := strings.TrimPrefix(r.URL.Path, "/api/websites/")
	if strings.Contains(websiteID, "/") || websiteID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !a.requireWebsiteManage(w, user, websiteID) {
			return
		}
		var req websiteInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name := strings.TrimSpace(req.Name)
		domain := normalizeWebsiteDomain(req.Domain)
		if name == "" || domain == "" {
			errorResponse(w, http.StatusBadRequest, "name and domain required")
			return
		}
		_, err := a.db.Exec(`
			update websites
			set name = ?, domain = ?, updated_at = ?
			where id = ?
		`, name, domain, iso(nowUTC()), websiteID)
		if err != nil {
			Error("failed to update website: %v", err)
			errorResponse(w, http.StatusInternalServerError, "update website failed")
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		if !a.requireWebsiteManage(w, user, websiteID) {
			return
		}
		_, err := a.db.Exec(`delete from websites where id = ?`, websiteID)
		if err != nil {
			Error("failed to delete website: %v", err)
			errorResponse(w, http.StatusInternalServerError, "delete website failed")
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// handleNestedRoutes - 网站嵌套资源路由分发
// 根据 URL 路径将请求分发到像素、分享、漏斗等子资源处理器
func (a *App) handleNestedRoutes(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/websites/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	websiteID := parts[0]
	resource := parts[1]
	if !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	switch resource {
	case "pixels":
		a.handleWebsitePixels(w, r, user, websiteID)
	case "shares":
		a.handleWebsiteShares(w, r, user, websiteID)
	case "funnels":
		a.handleWebsiteFunnels(w, r, user, websiteID)
	default:
		http.NotFound(w, r)
	}
}

// handleWebsitePixels - 网站像素管理
// GET - 获取网站的像素列表
// POST - 创建新像素
func (a *App) handleWebsitePixels(w http.ResponseWriter, r *http.Request, user *AuthUser, websiteID string) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(`
			select id, website_id, name, slug, enabled, created_at
			from pixels
			where website_id = ?
			order by created_at asc
		`, websiteID)
		if err != nil {
			Error("failed to query pixels: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		var pixels []Pixel
		for rows.Next() {
			var item Pixel
			var enabled int
			if err := rows.Scan(&item.ID, &item.WebsiteID, &item.Name, &item.Slug, &enabled, &item.CreatedAt); err != nil {
				Error("failed to scan pixel row: %v", err)
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			item.Enabled = enabled == 1
			pixels = append(pixels, item)
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "pixels": pixels})
	case http.MethodPost:
		if !a.requireWebsiteManage(w, user, websiteID) {
			return
		}
		var req pixelInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "pixel name required")
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		pixelID := newID()
		_, err := a.db.Exec(`
			insert into pixels(id, website_id, name, slug, enabled, created_at)
			values(?, ?, ?, ?, ?, ?)
		`, pixelID, websiteID, name, shortID(), boolInt(enabled), iso(nowUTC()))
		if err != nil {
			Error("failed to create pixel: %v", err)
			errorResponse(w, http.StatusInternalServerError, "create pixel failed")
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": pixelID})
	default:
		http.NotFound(w, r)
	}
}

// handlePixelByID - 单个像素管理端点
// PUT /api/pixels/{id} - 更新像素信息
func (a *App) handlePixelByID(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	pixelID := strings.TrimPrefix(r.URL.Path, "/api/pixels/")
	var websiteID string
	if err := a.db.QueryRow(`select website_id from pixels where id = ?`, pixelID).Scan(&websiteID); err != nil {
		http.NotFound(w, r)
		return
	}
	if !a.requireWebsiteManage(w, user, websiteID) {
		return
	}
	var req pixelInput
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" || req.Enabled == nil {
		errorResponse(w, http.StatusBadRequest, "name and enabled required")
		return
	}
	_, err := a.db.Exec(`update pixels set name = ?, enabled = ? where id = ?`, strings.TrimSpace(req.Name), boolInt(*req.Enabled), pixelID)
	if err != nil {
		Error("failed to update pixel: %v", err)
		errorResponse(w, http.StatusInternalServerError, "update pixel failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// handleWebsiteShares - 网站分享链接管理
// GET - 获取网站的分享链接列表
// POST - 创建新分享链接
func (a *App) handleWebsiteShares(w http.ResponseWriter, r *http.Request, user *AuthUser, websiteID string) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(`
			select id, website_id, slug, enabled, created_at
			from shares
			where website_id = ?
			order by created_at asc
		`, websiteID)
		if err != nil {
			Error("failed to query shares: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		var shares []Share
		for rows.Next() {
			var item Share
			var enabled int
			if err := rows.Scan(&item.ID, &item.WebsiteID, &item.Slug, &enabled, &item.CreatedAt); err != nil {
				Error("failed to scan share row: %v", err)
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			item.Enabled = enabled == 1
			shares = append(shares, item)
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "shares": shares})
	case http.MethodPost:
		if !a.requireWebsiteManage(w, user, websiteID) {
			return
		}
		shareID := newID()
		_, err := a.db.Exec(`
			insert into shares(id, website_id, slug, enabled, created_at)
			values(?, ?, ?, 1, ?)
		`, shareID, websiteID, shortID(), iso(nowUTC()))
		if err != nil {
			Error("failed to create share: %v", err)
			errorResponse(w, http.StatusInternalServerError, "create share failed")
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": shareID})
	default:
		http.NotFound(w, r)
	}
}

// handleShareByID - 单个分享链接管理端点
// PUT /api/shares/{id} - 更新分享链接的启用状态
func (a *App) handleShareByID(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	shareID := strings.TrimPrefix(r.URL.Path, "/api/shares/")
	var websiteID string
	if err := a.db.QueryRow(`select website_id from shares where id = ?`, shareID).Scan(&websiteID); err != nil {
		http.NotFound(w, r)
		return
	}
	if !a.requireWebsiteManage(w, user, websiteID) {
		return
	}
	var req shareInput
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Enabled == nil {
		errorResponse(w, http.StatusBadRequest, "enabled required")
		return
	}
	_, err := a.db.Exec(`update shares set enabled = ? where id = ?`, boolInt(*req.Enabled), shareID)
	if err != nil {
		Error("failed to update share: %v", err)
		errorResponse(w, http.StatusInternalServerError, "update share failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

// handleWebsiteFunnels - 网站漏斗管理
// GET - 获取网站的漏斗列表
// POST - 创建新漏斗
func (a *App) handleWebsiteFunnels(w http.ResponseWriter, r *http.Request, user *AuthUser, websiteID string) {
	switch r.Method {
	case http.MethodGet:
		funnels, err := a.listFunnels(websiteID)
		if err != nil {
			Error("failed to list funnels: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "funnels": funnels})
	case http.MethodPost:
		if !a.requireWebsiteManage(w, user, websiteID) {
			return
		}
		var req funnelInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" || len(req.Steps) < 2 {
			errorResponse(w, http.StatusBadRequest, "funnel requires a name and at least two steps")
			return
		}
		for _, step := range req.Steps {
			if (step.Type != "page" && step.Type != "event") || strings.TrimSpace(step.Value) == "" {
				errorResponse(w, http.StatusBadRequest, "invalid funnel step")
				return
			}
		}
		stepsJSON, _ := json.Marshal(req.Steps)
		funnelID := newID()
		_, err := a.db.Exec(`
			insert into funnels(id, website_id, name, steps_json, created_at)
			values(?, ?, ?, ?, ?)
		`, funnelID, websiteID, req.Name, string(stepsJSON), iso(nowUTC()))
		if err != nil {
			Error("failed to create funnel: %v", err)
			errorResponse(w, http.StatusInternalServerError, "create funnel failed")
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": funnelID})
	default:
		http.NotFound(w, r)
	}
}

// listWebsites - 获取用户可访问的网站列表
// 超级管理员可查看所有网站，其他用户只能查看有权限的网站
func (a *App) listWebsites(user *AuthUser) ([]Website, error) {
	var rows *sql.Rows
	var err error
	if user.Role == roleSuperAdmin {
		rows, err = a.db.Query(`
			select id, name, domain, created_at, updated_at
			from websites
			order by created_at asc
		`)
	} else {
		rows, err = a.db.Query(`
			select w.id, w.name, w.domain, w.created_at, w.updated_at
			from websites w
			join website_permissions p on p.website_id = w.id
			where p.user_id = ?
			order by w.created_at asc
		`, user.ID)
	}
	if err != nil {
		Error("failed to query websites: %v", err)
		return nil, err
	}
	defer rows.Close()
	var websites []Website
	for rows.Next() {
		var item Website
		if err := rows.Scan(&item.ID, &item.Name, &item.Domain, &item.CreatedAt, &item.UpdatedAt); err != nil {
			Error("failed to scan website row: %v", err)
			return nil, err
		}
		websites = append(websites, item)
	}
	return websites, rows.Err()
}

// handleCollectPixel - 像素追踪采集端点
// GET /collect/p/{slug} - 通过像素 slug 采集页面访问数据，返回 1x1 GIF
func (a *App) handleCollectPixel(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/collect/p/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	setCollectionCORS(w, r)
	var pixelID, websiteID string
	var enabled int
	err := a.db.QueryRow(`select id, website_id, enabled from pixels where slug = ?`, slug).Scan(&pixelID, &websiteID, &enabled)
	if err != nil || enabled != 1 {
		http.NotFound(w, r)
		return
	}
	// 速率限制检查
	if !a.allowCollectionRequest(r, websiteID, 1) {
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(onePixelGIF)
		return
	}
	query := r.URL.Query()
	pixelURL := firstNonEmpty(strings.TrimSpace(query.Get("url")), r.Referer(), r.URL.String())
	_, pixelHost, _ := cleanURL(pixelURL)
	_, originHost, _ := cleanURL(strings.TrimSpace(r.Header.Get("Origin")))
	_, refererHost, _ := cleanURL(strings.TrimSpace(r.Referer()))
	// 验证请求来源域名是否匹配网站配置
	if !a.websiteAllowsAnyHost(websiteID, pixelHost, originHost, refererHost) {
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(onePixelGIF)
		return
	}
	visitorID := firstNonEmpty(strings.TrimSpace(query.Get("id")), strings.TrimSpace(query.Get("vid")))
	req := eventRequest{
		Type: "event",
		Payload: eventPayload{
			Website:  websiteID,
			Pixel:    pixelID,
			URL:      pixelURL,
			Referrer: firstNonEmpty(strings.TrimSpace(query.Get("referrer")), r.Referer()),
			ID:       visitorID,
		},
	}
	if _, err := a.recordEvent(r, req); err != nil {
		// Intentionally ignore collection failures for pixel responses.
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(onePixelGIF)
}

// handleSend - 事件发送端点
// POST /api/send - 接收并记录单个分析事件
func (a *App) handleSend(w http.ResponseWriter, r *http.Request) {
	setCollectionCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var req eventRequest
	if err := decodeCollectionJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	websiteID := strings.TrimSpace(firstNonEmpty(req.Payload.Website, a.websiteForPixel(strings.TrimSpace(req.Payload.Pixel))))
	if websiteID != "" && !a.allowCollectionRequest(r, websiteID, 1) {
		errorResponse(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	result, err := a.recordEvent(r, req)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

// handleBatch - 批量事件发送端点
// POST /api/batch - 接收并批量记录多个分析事件
func (a *App) handleBatch(w http.ResponseWriter, r *http.Request) {
	setCollectionCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var reqs []eventRequest
	if err := decodeCollectionJSON(r, &reqs); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	results := make([]map[string]any, 0, len(reqs))
	limitedCount := 0
	for _, req := range reqs {
		websiteID := strings.TrimSpace(firstNonEmpty(req.Payload.Website, a.websiteForPixel(strings.TrimSpace(req.Payload.Pixel))))
		if websiteID != "" && !a.allowCollectionRequest(r, websiteID, 1) {
			results = append(results, map[string]any{"ok": false, "error": "rate limit exceeded", "website_id": websiteID})
			limitedCount++
			continue
		}
		res, err := a.recordEvent(r, req)
		if err != nil {
			results = append(results, map[string]any{"ok": false, "error": err.Error()})
			continue
		}
		results = append(results, map[string]any{"ok": true, "result": res})
	}
	status := http.StatusOK
	if limitedCount > 0 && limitedCount == len(reqs) {
		status = http.StatusTooManyRequests
	}
	jsonResponse(w, status, map[string]any{"ok": true, "items": results, "partial": limitedCount > 0})
}

// handleSettings - 系统设置端点
// GET /api/settings - 获取系统设置（仅超级管理员）
// PUT /api/settings - 更新系统设置（仅超级管理员）
func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	// 验证用户身份
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	// 仅超级管理员可操作
	if user.Role != roleSuperAdmin {
		errorResponse(w, http.StatusForbidden, "super admin required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 从数据库获取当前系统设置
		settings, err := a.getSystemSettings()
		if err != nil {
			Error("failed to get system settings: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		// 同时返回版本号和机器人审计信息
		jsonResponse(w, http.StatusOK, map[string]any{
			"ok":        true,
			"settings":  settings,
			"version":   version,
			"bot_audit": a.botAuditSnapshot(),
		})
	case http.MethodPut:
		var req settingsInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		// 清理和规范化输入字段
		req.ListenAddr = strings.TrimSpace(req.ListenAddr)
		req.DatabasePath = strings.TrimSpace(req.DatabasePath)
		req.GeoIPDatabasePath = strings.TrimSpace(req.GeoIPDatabasePath)
		req.LogLevel = strings.TrimSpace(strings.ToLower(req.LogLevel))
		req.BotFilterMode = strings.TrimSpace(strings.ToLower(req.BotFilterMode))

		// 必填字段校验
		if req.ListenAddr == "" || req.DatabasePath == "" {
			errorResponse(w, http.StatusBadRequest, "listen_addr and database_path required")
			return
		}
		// 设置日志级别默认值
		if req.LogLevel == "" {
			req.LogLevel = "info"
		}
		// 设置机器人过滤模式默认值
		if req.BotFilterMode == "" {
			req.BotFilterMode = "balanced"
		}
		// 处理数据保留天数，未提供时使用当前值或默认 365 天
		retentionDays := req.DataRetentionDays
		if retentionDays <= 0 {
			retentionDays = 365
			if current, err := a.getSystemSettings(); err == nil && current.DataRetentionDays > 0 {
				retentionDays = current.DataRetentionDays
			}
		}
		// 将设置写入数据库
		if err := a.setSystemSettings(map[string]string{
			"listen_addr":         req.ListenAddr,
			"database_path":       req.DatabasePath,
			"geoip_database_path": req.GeoIPDatabasePath,
			"log_level":           req.LogLevel,
			"data_retention_days": strconv.Itoa(retentionDays),
			"bot_filter_mode":     req.BotFilterMode,
		}); err != nil {
			Error("failed to save system settings: %v", err)
			errorResponse(w, http.StatusInternalServerError, "save settings failed")
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// handleCleanup - 数据清理端点
// POST /api/settings/cleanup - 清理历史数据（仅超级管理员）
func (a *App) handleCleanup(w http.ResponseWriter, r *http.Request) {
	// 验证用户身份
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	// 仅超级管理员可操作
	if user.Role != roleSuperAdmin {
		errorResponse(w, http.StatusForbidden, "super admin required")
		return
	}
	// 获取当前系统设置以确定数据保留天数
	settings, err := a.getSystemSettings()
	if err != nil {
		Error("failed to get system settings for cleanup: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 执行历史数据清理
	result, err := a.cleanupHistoricalData(settings.DataRetentionDays)
	if err != nil {
		Error("failed to cleanup historical data: %v", err)
		errorResponse(w, http.StatusInternalServerError, "cleanup failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

// handleBackup - 数据库备份端点
// POST /api/settings/backup - 创建数据库备份（仅超级管理员）
func (a *App) handleBackup(w http.ResponseWriter, r *http.Request) {
	// 验证用户身份
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	// 仅超级管理员可操作
	if user.Role != roleSuperAdmin {
		errorResponse(w, http.StatusForbidden, "super admin required")
		return
	}
	// 执行数据库备份
	path, err := a.createBackup()
	if err != nil {
		Error("failed to create backup: %v", err)
		errorResponse(w, http.StatusInternalServerError, "backup failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":          true,
		"backup_path": path,
	})
}

// recordEvent - 记录单个分析事件
// 验证事件数据、检测地理位置、过滤机器人流量，然后将事件加入写入队列
func (a *App) recordEvent(r *http.Request, req eventRequest) (map[string]any, error) {
	// 默认事件类型为 event
	if req.Type == "" {
		req.Type = "event"
	}
	// 校验事件类型是否合法
	if req.Type != "event" && req.Type != "revenue" && req.Type != "identify" && req.Type != "pageview" {
		return nil, fmt.Errorf("unsupported event type")
	}

	payload := req.Payload
	websiteID := strings.TrimSpace(payload.Website)
	pixelID := strings.TrimSpace(payload.Pixel)
	// website_id 和 pixel_id 必须提供其一
	if websiteID == "" && pixelID == "" {
		return nil, fmt.Errorf("website or pixel is required")
	}
	// website_id 和 pixel_id 不能同时提供
	if websiteID != "" && pixelID != "" {
		return nil, fmt.Errorf("website and pixel cannot both be provided")
	}

	// 如果通过 pixel_id 提交，查找对应的网站并验证像素是否启用
	if pixelID != "" {
		var enabled int
		if err := a.db.QueryRow(`select website_id, enabled from pixels where id = ?`, pixelID).Scan(&websiteID, &enabled); err != nil {
			return nil, fmt.Errorf("pixel not found")
		}
		if enabled != 1 {
			return nil, fmt.Errorf("pixel disabled")
		}
	}
	// 验证网站是否存在
	if !a.websiteExists(websiteID) {
		return nil, fmt.Errorf("website not found")
	}

	// 规范化事件创建时间
	createdAt := normalizeEventCreatedAt(payload.Timestamp, nowUTC())

	// 解析和补全页面 URL
	fullURL := payload.URL
	if fullURL == "" {
		fullURL = r.Header.Get("Origin")
	}
	// 如果 URL 缺少协议前缀，补上 https://
	if !strings.Contains(fullURL, "://") && fullURL != "" {
		fullURL = "https://" + strings.TrimPrefix(fullURL, "/")
	}
	parsedURL, host, pathValue := cleanURL(fullURL)
	_, originHost, _ := cleanURL(strings.TrimSpace(r.Header.Get("Origin")))
	_, refererHost, _ := cleanURL(strings.TrimSpace(firstNonEmpty(payload.Referrer, r.Referer())))
	// 验证请求来源域名是否匹配网站配置
	if !a.websiteAllowsAnyHost(websiteID, payload.Hostname, host, originHost, refererHost) {
		return nil, fmt.Errorf("website domain mismatch")
	}
	// 解析来源域名
	refDomain := referrerDomain(payload.Referrer)
	// 检测浏览器、操作系统和设备类型
	browser, osName, device := detectUserAgent(r, payload)
	// 检测地理位置信息
	country, region, city := a.detectGeo(r, payload)
	// 机器人流量过滤
	if ignored, reason := a.shouldIgnoreBotTraffic(r); ignored {
		a.recordBotAudit(reason)
		return map[string]any{
			"website_id": websiteID,
			"ignored":    true,
			"reason":     reason,
		}, nil
	}
	// 补全主机名
	if payload.Hostname != "" {
		host = payload.Hostname
	} else if host == "" {
		host = firstNonEmpty(originHost, refererHost)
	}
	// 路径默认为根路径
	if pathValue == "" {
		pathValue = "/"
	}

	// 生成访客标识：优先使用客户端提供的 ID，否则基于网站+IP+UA 生成
	visitorKey := payload.ID
	if visitorKey == "" {
		visitorKey = tokenHash(websiteID + "|" + clientIP(r) + "|" + r.UserAgent())
	}

	// 序列化元数据并确定事件类型
	metadata, _ := json.Marshal(payload.Data)
	eventType := normalizeEventType(payload, pixelID)
	// 提取收入信息
	amount := 0.0
	currency := ""
	if payload.Revenue != nil {
		amount = payload.Revenue.Amount
		currency = strings.ToUpper(strings.TrimSpace(payload.Revenue.Currency))
	}
	// 构建待写入的事件对象
	item := queuedEvent{
		WebsiteID:      websiteID,
		PixelID:        pixelID,
		VisitorKey:     visitorKey,
		EventType:      eventType,
		EventName:      strings.TrimSpace(payload.Name),
		PageTitle:      strings.TrimSpace(payload.Title),
		Hostname:       host,
		URL:            parsedURL,
		URLPath:        pathValue,
		Referrer:       payload.Referrer,
		ReferrerDomain: refDomain,
		UTMSource:      firstNonEmpty(payload.UTMSource, extractUTM(parsedURL, "utm_source")),
		UTMMedium:      firstNonEmpty(payload.UTMMedium, extractUTM(parsedURL, "utm_medium")),
		UTMCampaign:    firstNonEmpty(payload.UTMCamp, extractUTM(parsedURL, "utm_campaign")),
		UTMContent:     firstNonEmpty(payload.UTMCont, extractUTM(parsedURL, "utm_content")),
		UTMTerm:        firstNonEmpty(payload.UTMTerm, extractUTM(parsedURL, "utm_term")),
		Browser:        browser,
		OS:             osName,
		Device:         device,
		Country:        country,
		Region:         region,
		City:           city,
		Amount:         amount,
		Currency:       currency,
		Metadata:       string(metadata),
		CreatedAt:      createdAt,
	}

	// 将事件放入异步写入队列
	select {
	case a.eventQueue <- item:
	default:
		// 队列已满时尝试同步写入
		if err := a.writeEventImmediately(item); err != nil {
			Error("failed to write event immediately, queue full: %v", err)
			return nil, fmt.Errorf("event queue is full")
		}
		return map[string]any{
			"website_id": websiteID,
			"event_type": eventType,
			"queued":     false,
		}, nil
	}

	return map[string]any{
		"website_id": websiteID,
		"event_type": eventType,
		"queued":     true,
	}, nil
}

// websiteExists - 检查指定网站是否存在
func (a *App) websiteExists(websiteID string) bool {
	var count int
	if err := a.db.QueryRow(`select count(*) from websites where id = ?`, websiteID).Scan(&count); err != nil {
		Error("failed to query website existence for %s: %v", websiteID, err)
		return false
	}
	return count > 0
}

// websiteAllowsHost - 检查网站是否允许指定主机名的请求
// 通过比较请求主机名与网站配置域名来判断
func (a *App) websiteAllowsHost(websiteID, host string) bool {
	// 规范化主机名
	host = normalizeWebsiteDomain(host)
	if host == "" {
		return false
	}
	// 从数据库查询网站配置的域名
	var configuredDomain string
	if err := a.db.QueryRow(`select domain from websites where id = ?`, websiteID).Scan(&configuredDomain); err != nil {
		Error("failed to query website domain for host check: %v", err)
		return false
	}
	return hostMatchesWebsiteDomain(host, configuredDomain)
}

// websiteAllowsAnyHost - 检查网站是否允许任一给定的主机名
// 只要有一个主机名匹配即返回 true
func (a *App) websiteAllowsAnyHost(websiteID string, hosts ...string) bool {
	for _, host := range hosts {
		if a.websiteAllowsHost(websiteID, host) {
			return true
		}
	}
	return false
}

// websiteForPixel - 根据像素 ID 查找所属的网站 ID
func (a *App) websiteForPixel(pixelID string) string {
	if pixelID == "" {
		return ""
	}
	var websiteID string
	if err := a.db.QueryRow(`select website_id from pixels where id = ?`, pixelID).Scan(&websiteID); err != nil {
		Error("failed to query website for pixel %s: %v", pixelID, err)
		return ""
	}
	return websiteID
}

// insertEvent - 将事件记录插入数据库
func (a *App) insertEvent(record eventRecord) error {
	_, err := a.db.Exec(`
		insert into events(
			id, website_id, session_id, visitor_id, pixel_id, event_type, event_name,
			page_title, hostname, url, url_path, referrer, referrer_domain,
			utm_source, utm_medium, utm_campaign, utm_content, utm_term,
			browser, os, device, country, region, city, amount, currency, metadata, created_at
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		newID(), record.WebsiteID, record.SessionID, record.VisitorID, record.PixelValue(), record.EventType,
		record.EventName, record.PageTitle, record.Hostname, record.URL, record.URLPath, record.Referrer,
		record.ReferrerDomain, record.UTMSource, record.UTMMedium, record.UTMCampaign, record.UTMContent,
		record.UTMTerm, record.Browser, record.OS, record.Device, record.Country, record.Region,
		record.City, record.Amount, record.Currency, record.Metadata, iso(record.CreatedAt),
	)
	if err != nil {
		Error("failed to insert event for website %s: %v", record.WebsiteID, err)
	}
	return err
}

// PixelValue - 返回像素 ID，如果为空则返回 nil（用于数据库 NULL 值）
func (r eventRecord) PixelValue() any {
	if r.PixelID == "" {
		return nil
	}
	return r.PixelID
}

// handleOverview - 数据概览端点
// GET /api/analytics/overview - 返回网站的综合统计数据，包括页面浏览量、访客数、会话数、跳出率等
func (a *App) handleOverview(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文，获取用户、网站ID和时间范围
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}

	// 定义概览数据结构
	type overview struct {
		Pageviews          int64   `json:"pageviews"`
		Visitors           int64   `json:"visitors"`
		Sessions           int64   `json:"sessions"`
		Events             int64   `json:"events"`
		Revenue            float64 `json:"revenue"`
		BounceRate         float64 `json:"bounce_rate"`
		AvgSessionDuration int64   `json:"avg_session_duration_seconds"`
		AvgTimeOnPage      int64   `json:"avg_time_on_page_seconds"`
	}
	var out overview
	var customEvents int64
	var bouncedSessions, sessionDurationTotalSeconds, timeOnPageTotalMS, timeOnPageSamples int64
	// 从每日聚合表查询概览指标
	if err := a.db.QueryRow(`
		select
			coalesce(sum(pageviews), 0),
			coalesce(sum(custom_events), 0),
			coalesce(sum(visitors), 0),
			coalesce(sum(sessions), 0),
			coalesce(sum(bounced_sessions), 0),
			coalesce(sum(session_duration_total_seconds), 0),
			coalesce(sum(time_on_page_total_ms), 0),
			coalesce(sum(time_on_page_samples), 0),
			coalesce(sum(revenue), 0)
		from agg_overview_daily
		where website_id = ? and bucket_date between ? and ?
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&out.Pageviews, &customEvents, &out.Visitors, &out.Sessions, &bouncedSessions, &sessionDurationTotalSeconds, &timeOnPageTotalMS, &timeOnPageSamples, &out.Revenue); err != nil {
		Error("failed to query overview for website %s: %v", websiteID, err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 计算自定义事件总数
	out.Events = customEvents
	// 计算跳出率和平均会话时长
	if out.Sessions > 0 {
		out.BounceRate = float64(bouncedSessions) / float64(out.Sessions)
		out.AvgSessionDuration = int64(math.Round(float64(sessionDurationTotalSeconds) / float64(out.Sessions)))
	}
	// 计算平均页面停留时间
	if timeOnPageSamples > 0 {
		out.AvgTimeOnPage = int64(math.Round(float64(timeOnPageTotalMS) / float64(timeOnPageSamples) / 1000))
	} else {
		out.AvgTimeOnPage = 0
	}
	// 查询趋势数据：按日分组的各项指标
	trendRows, err := a.db.Query(`
		select
			o.bucket_date,
			o.pageviews,
			o.custom_events,
			o.visitors,
			o.sessions,
			o.revenue,
			coalesce(avg(cast(json_extract(e.metadata, '$.duration_ms') as integer) / 1000), 0) as avg_time_on_page_seconds
		from agg_overview_daily
		o
		left join events e
			on e.website_id = o.website_id
			and e.event_name in ('page_leave', 'page_ping')
			and date(e.created_at) = o.bucket_date
		where o.website_id = ? and o.bucket_date between ? and ?
		group by o.bucket_date, o.pageviews, o.custom_events, o.visitors, o.sessions, o.revenue
		order by o.bucket_date asc
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query overview trend for website %s: %v", websiteID, err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer trendRows.Close()
	var trend []map[string]any
	for trendRows.Next() {
		var day string
		var pageviews, customEvents, visitors, sessions int64
		var avgTimeOnPage float64
		var revenue float64
		if err := trendRows.Scan(&day, &pageviews, &customEvents, &visitors, &sessions, &revenue, &avgTimeOnPage); err != nil {
			Error("failed to scan overview trend row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		trend = append(trend, map[string]any{
			"date":                     day,
			"pageviews":                pageviews,
			"visitors":                 visitors,
			"sessions":                 sessions,
			"events":                   customEvents,
			"revenue":                  revenue,
			"avg_time_on_page_seconds": int64(math.Round(avgTimeOnPage)),
		})
	}
	// 加载环比对比数据
	compare, _ := a.loadOverviewCompare(websiteID, from, to)
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "overview": out, "trend": trend, "compare": compare})
}

// handlePages - 页面分析端点
// GET /api/analytics/pages - 返回页面浏览量、会话数、停留时间、入口页和出口页统计
func (a *App) handlePages(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 查询每个路径的独立会话数
	sessionRows, err := a.db.Query(`
		select url_path, count(distinct session_id) as sessions
		from events
		where website_id = ? and event_type = 'pageview' and created_at between ? and ?
		group by url_path
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query page session counts: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	sessionCounts := map[string]int64{}
	for sessionRows.Next() {
		var path string
		var sessions int64
		if err := sessionRows.Scan(&path, &sessions); err != nil {
			sessionRows.Close()
			Error("failed to scan page session row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		sessionCounts[path] = sessions
	}
	if err := sessionRows.Close(); err != nil {
		Error("failed to close page session rows: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 查询页面停留时间指标
	dwellByPath, err := a.queryPageDwellMetrics(websiteID, from, to)
	if err != nil {
		Error("failed to query page dwell metrics: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 从每日聚合表查询页面浏览量
	rows, err := a.db.Query(`
		select url_path, sum(pageviews) as pageviews
		from agg_pages_daily
		where website_id = ? and bucket_date between ? and ?
		group by url_path
		order by pageviews desc, url_path asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query pages aggregate: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var path string
		var pageviews int64
		if err := rows.Scan(&path, &pageviews); err != nil {
			Error("failed to scan page aggregate row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		// 计算平均停留时间
		dwell := dwellByPath[path]
		avgDwell := int64(0)
		if dwell.Count > 0 {
			avgDwell = dwell.DurationMS / dwell.Count / 1000
		}
		items = append(items, map[string]any{
			"path":                      path,
			"pageviews":                 pageviews,
			"sessions":                  sessionCounts[path],
			"avg_time_on_page_seconds":  avgDwell,
			"time_on_page_sample_count": dwell.Count,
		})
	}
	// 查询入口页统计（用户首次访问的页面）
	entryRows, err := a.db.Query(`
		select entry_path, count(*) as sessions
		from sessions
		where website_id = ? and started_at between ? and ? and trim(entry_path) <> ''
		group by entry_path
		order by sessions desc, entry_path asc
		limit 20
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query entry pages: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer entryRows.Close()
	var entries []map[string]any
	for entryRows.Next() {
		var path string
		var sessions int64
		if err := entryRows.Scan(&path, &sessions); err != nil {
			Error("failed to scan entry page row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		entries = append(entries, map[string]any{
			"path":     path,
			"sessions": sessions,
		})
	}

	// 查询出口页统计（用户最后访问的页面）
	exitRows, err := a.db.Query(`
		select exit_path, count(*) as sessions
		from sessions
		where website_id = ? and started_at between ? and ? and trim(exit_path) <> ''
		group by exit_path
		order by sessions desc, exit_path asc
		limit 20
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query exit pages: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer exitRows.Close()
	var exits []map[string]any
	for exitRows.Next() {
		var path string
		var sessions int64
		if err := exitRows.Scan(&path, &sessions); err != nil {
			Error("failed to scan exit page row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		exits = append(exits, map[string]any{
			"path":     path,
			"sessions": sessions,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":      true,
		"items":   items,
		"entries": entries,
		"exits":   exits,
	})
}

// handleEvents - 自定义事件分析端点
// GET /api/analytics/events - 返回自定义事件的统计信息，按事件类型和名称分组
func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 查询自定义事件，排除 pageview、page_leave、page_ping
	rows, err := a.db.Query(`
		select
			event_type,
			case
				when trim(event_name) = '' and event_type = 'pixel' then '(pixel)'
				when trim(event_name) = '' and event_type = 'revenue' then '(revenue)'
				when trim(event_name) = '' then '(unnamed)'
				else event_name
			end as label,
			count(*) as events,
			count(distinct session_id) as sessions,
			coalesce(sum(amount), 0) as revenue
		from events
		where website_id = ?
			and event_type <> 'pageview'
			and event_name not in ('page_leave', 'page_ping')
			and created_at between ? and ?
		group by event_type, label
		order by events desc, label asc
		limit 100
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query custom events: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	typeRows := map[string]int64{}
	for rows.Next() {
		var eventType, label string
		var events, sessions int64
		var revenue float64
		if err := rows.Scan(&eventType, &label, &events, &sessions, &revenue); err != nil {
			Error("failed to scan event row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, map[string]any{
			"type":     eventType,
			"name":     label,
			"events":   events,
			"sessions": sessions,
			"revenue":  revenue,
		})
		typeRows[eventType] += events
	}
	// 按事件类型汇总
	var types []map[string]any
	for eventType, events := range typeRows {
		types = append(types, map[string]any{
			"type":   eventType,
			"events": events,
		})
	}
	// 按事件数量降序排列
	sort.Slice(types, func(i, j int) bool {
		return types[i]["events"].(int64) > types[j]["events"].(int64)
	})
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items, "types": types})
}

// handleReferrers - 来源分析端点
// GET /api/analytics/referrers - 返回流量来源域名的访问量和收入统计
func (a *App) handleReferrers(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 从每日聚合表查询来源域名统计
	rows, err := a.db.Query(`
		select referrer_domain, sum(sessions) as visits, sum(revenue) as revenue
		from agg_referrers_daily
		where website_id = ? and bucket_date between ? and ?
		group by referrer_domain
		order by visits desc, referrer_domain asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query referrers: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var ref string
		var visits int64
		var revenue float64
		if err := rows.Scan(&ref, &visits, &revenue); err != nil {
			Error("failed to scan referrer row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		// 空来源标记为直接访问
		if ref == "" {
			ref = "(direct)"
		}
		items = append(items, map[string]any{
			"referrer": ref,
			"visits":   visits,
			"revenue":  revenue,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

// handleDevices - 设备分析端点
// GET /api/analytics/devices - 返回浏览器、操作系统、设备类型及交叉矩阵统计
func (a *App) handleDevices(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 分别查询浏览器、操作系统、设备类型和交叉矩阵数据
	payload := map[string]any{
		"browsers": a.aggDeviceCount(websiteID, from, to, "browser"),
		"os":       a.aggDeviceCount(websiteID, from, to, "os"),
		"devices":  a.aggDeviceCount(websiteID, from, to, "device"),
		"matrix":   a.aggDeviceMatrix(websiteID, from, to),
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": payload})
}

// handleGeo - 地理位置分析端点
// GET /api/analytics/geo - 返回按国家、地区、城市分组的访问量统计
func (a *App) handleGeo(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 查询按国家分组的访问量
	rows, err := a.db.Query(`
		select country, sum(sessions) as visits
		from agg_geo_daily
		where website_id = ? and bucket_date between ? and ?
		group by country
		order by visits desc, country asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query geo countries: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var country string
		var visits int64
		if err := rows.Scan(&country, &visits); err != nil {
			Error("failed to scan geo country row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		country = cleanGeoLabel(country, "(unknown)")
		items = append(items, map[string]any{"country": country, "visits": visits})
	}

	// 查询按地区分组的访问量
	regionRows, err := a.db.Query(`
		select region, sum(sessions) as visits
		from agg_geo_daily
		where website_id = ? and bucket_date between ? and ? and trim(region) <> ''
		group by region
		order by visits desc, region asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query geo regions: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer regionRows.Close()
	var regions []map[string]any
	for regionRows.Next() {
		var region string
		var visits int64
		if err := regionRows.Scan(&region, &visits); err != nil {
			Error("failed to scan geo region row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		regions = append(regions, map[string]any{"region": cleanGeoLabel(region, "Unknown"), "visits": visits})
	}

	// 查询按城市分组的访问量
	cityRows, err := a.db.Query(`
		select city, sum(sessions) as visits
		from agg_geo_daily
		where website_id = ? and bucket_date between ? and ? and trim(city) <> ''
		group by city
		order by visits desc, city asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query geo cities: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cityRows.Close()
	var cities []map[string]any
	for cityRows.Next() {
		var city string
		var visits int64
		if err := cityRows.Scan(&city, &visits); err != nil {
			Error("failed to scan geo city row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		cities = append(cities, map[string]any{"city": cleanGeoLabel(city, "Unknown"), "visits": visits})
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":      true,
		"items":   items,
		"regions": regions,
		"cities":  cities,
	})
}

// aggDeviceCount - 聚合设备维度的访问量统计
// 按指定列（browser/os/device）分组查询每日聚合数据
func (a *App) aggDeviceCount(websiteID string, from, to time.Time, column string) []map[string]any {
	query := fmt.Sprintf(`
		select %s as value, sum(sessions) as visits
		from agg_devices_daily
		where website_id = ? and bucket_date between ? and ?
		group by value
		order by visits desc, value asc
		limit 50
	`, column)
	rows, err := a.db.Query(query, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var value string
		var visits int64
		if err := rows.Scan(&value, &visits); err == nil {
			if value == "" {
				value = "(unknown)"
			}
			items = append(items, map[string]any{"value": value, "visits": visits})
		}
	}
	return items
}

// aggDeviceMatrix - 聚合设备交叉矩阵
// 返回浏览器 × 操作系统 × 设备类型的交叉访问量统计
func (a *App) aggDeviceMatrix(websiteID string, from, to time.Time) []map[string]any {
	rows, err := a.db.Query(`
		select browser, os, device, sum(sessions) as visits
		from agg_devices_daily
		where website_id = ? and bucket_date between ? and ?
		group by browser, os, device
		order by visits desc, browser asc, os asc, device asc
		limit 30
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var browser, osName, device string
		var visits int64
		if err := rows.Scan(&browser, &osName, &device, &visits); err == nil {
			if browser == "" {
				browser = "(unknown)"
			}
			if osName == "" {
				osName = "(unknown)"
			}
			if device == "" {
				device = "(unknown)"
			}
			items = append(items, map[string]any{
				"browser": browser,
				"os":      osName,
				"device":  device,
				"visits":  visits,
			})
		}
	}
	return items
}

// handleAttribution - 流量归因分析端点
// GET /api/analytics/attribution - 返回按来源、媒介、广告活动分组的会话和收入统计
func (a *App) handleAttribution(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 从每日归因聚合表查询分组统计
	rows, err := a.db.Query(`
		select source, medium, campaign, sum(sessions) as sessions, sum(revenue) as revenue
		from agg_attribution_daily
		where website_id = ? and bucket_date between ? and ?
		group by source, medium, campaign
		order by sessions desc, source asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query attribution: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		Source   string  `json:"source"`
		Medium   string  `json:"medium"`
		Campaign string  `json:"campaign"`
		Sessions int64   `json:"sessions"`
		Revenue  float64 `json:"revenue"`
	}
	var items []item
	for rows.Next() {
		var row item
		if err := rows.Scan(&row.Source, &row.Medium, &row.Campaign, &row.Sessions, &row.Revenue); err != nil {
			Error("failed to scan attribution row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, row)
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

// handleRevenue - 收入分析端点
// GET /api/analytics/revenue - 返回按来源和货币分组的收入统计
func (a *App) handleRevenue(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 从每日收入聚合表查询分组统计
	rows, err := a.db.Query(`
		select source, currency, sum(event_count) as events, sum(revenue) as revenue
		from agg_revenue_daily
		where website_id = ? and bucket_date between ? and ?
		group by source, currency
		order by revenue desc, source asc
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		Error("failed to query revenue: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var source, currency string
		var events int64
		var revenue float64
		if err := rows.Scan(&source, &currency, &events, &revenue); err != nil {
			Error("failed to scan revenue row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, map[string]any{
			"source":   source,
			"currency": currency,
			"events":   events,
			"revenue":  revenue,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

// handleRetention - 用户留存分析端点
// GET /api/analytics/retention - 计算按日分组的用户留存率（第1天、第7天、第30天）
func (a *App) handleRetention(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 使用 CTE 查询同期群访客及其后续回访记录
	rows, err := a.db.Query(`
		with cohort_visitors as (
			select visitor_id, min(date(started_at)) as cohort_day
			from sessions
			where website_id = ?
			group by visitor_id
			having cohort_day between ? and ?
		)
		select s.visitor_id, date(s.started_at), c.cohort_day
		from sessions s
		join cohort_visitors c on c.visitor_id = s.visitor_id
		where s.website_id = ?
			and date(s.started_at) between c.cohort_day and date(c.cohort_day, '+30 day')
		order by s.visitor_id asc, s.started_at asc
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"), websiteID)
	if err != nil {
		Error("failed to query retention data: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	// 留存数据结构：记录每个同期群在第1天、第7天、第30天的回访人数
	type retentionData struct {
		Day1  int `json:"day_1"`
		Day7  int `json:"day_7"`
		Day30 int `json:"day_30"`
		Size  int `json:"size"`
	}
	cohorts := map[string]*retentionData{}
	seen := map[string][]time.Time{}
	first := map[string]time.Time{}
	for rows.Next() {
		var visitorID, dayText, cohortText string
		if err := rows.Scan(&visitorID, &dayText, &cohortText); err != nil {
			Error("failed to scan retention row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		day, _ := time.ParseInLocation("2006-01-02", dayText, time.UTC)
		cohortDay, _ := time.ParseInLocation("2006-01-02", cohortText, time.UTC)
		first[visitorID] = cohortDay
		seen[visitorID] = append(seen[visitorID], day)
	}

	for visitorID, days := range seen {
		cohortDay := first[visitorID]
		key := cohortDay.Format("2006-01-02")
		if cohorts[key] == nil {
			cohorts[key] = &retentionData{}
		}
		data := cohorts[key]
		// 增加同期群人数
		data.Size++
		// 计算访客回访的唯一天数差
		unique := map[int]bool{}
		for _, day := range days {
			delta := int(day.Sub(cohortDay).Hours() / 24)
			unique[delta] = true
		}
		// 统计第1天、第7天、第30天的回访情况
		if unique[1] {
			data.Day1++
		}
		if unique[7] {
			data.Day7++
		}
		if unique[30] {
			data.Day30++
		}
	}

	// 按日期排序并计算留存率
	keys := make([]string, 0, len(cohorts))
	for key := range cohorts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var items []map[string]any
	for _, key := range keys {
		data := cohorts[key]
		items = append(items, map[string]any{
			"cohort": key,
			"size":   data.Size,
			"day_1":  retentionRate(data.Day1, data.Size),
			"day_7":  retentionRate(data.Day7, data.Size),
			"day_30": retentionRate(data.Day30, data.Size),
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

// handleFunnelReport - 漏斗分析端点
// GET /api/analytics/funnel - 根据漏斗 ID 计算漏斗转化报告
func (a *App) handleFunnelReport(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 获取漏斗 ID 参数
	funnelID := strings.TrimSpace(r.URL.Query().Get("funnel_id"))
	if funnelID == "" {
		errorResponse(w, http.StatusBadRequest, "funnel_id required")
		return
	}
	// 查询漏斗定义
	funnel, err := a.getFunnel(websiteID, funnelID)
	if err != nil {
		Error("failed to get funnel %s: %v", funnelID, err)
		errorResponse(w, http.StatusNotFound, "funnel not found")
		return
	}
	// 执行漏斗分析计算
	report, err := a.runFunnel(funnel, from, to)
	if err != nil {
		Error("failed to run funnel report: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "funnel": funnel, "report": report})
}

// handleRealtime - 实时数据端点
// GET /api/analytics/realtime - 返回最近 5 分钟内的活跃访客数、会话数和事件时间线
func (a *App) handleRealtime(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, _, _, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 计算最近 5 分钟的时间窗口
	since := nowUTC().Add(-5 * time.Minute)
	// 查询活跃访客数和会话数
	var activeVisitors, activeSessions int64
	if err := a.db.QueryRow(`
		select count(distinct visitor_id), count(*)
		from sessions
		where website_id = ? and last_seen_at >= ?
	`, websiteID, iso(since)).Scan(&activeVisitors, &activeSessions); err != nil {
		Error("failed to query realtime active visitors: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 查询按分钟分组的事件时间线
	rows, err := a.db.Query(`
		select strftime('%Y-%m-%dT%H:%M:00Z', created_at) as bucket, count(*) as events
		from events
		where website_id = ? and created_at >= ?
		group by bucket
		order by bucket asc
	`, websiteID, iso(since))
	if err != nil {
		Error("failed to query realtime event timeline: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var timeline []map[string]any
	for rows.Next() {
		var bucket string
		var events int64
		if err := rows.Scan(&bucket, &events); err != nil {
			Error("failed to scan realtime timeline row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		timeline = append(timeline, map[string]any{"bucket": bucket, "events": events})
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok": true,
		"realtime": map[string]any{
			"window_minutes":   5,
			"active_visitors":  activeVisitors,
			"active_sessions":  activeSessions,
			"event_timeline":   timeline,
			"generated_at_utc": iso(nowUTC()),
		},
	})
}

// handleExport - 数据导出端点
// GET /api/analytics/export - 导出事件、页面或会话数据为 CSV 或 JSON 格式
func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	// 解析请求上下文
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	// 解析导出类型和格式参数
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind == "" {
		kind = "events"
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}
	// 根据类型分发到不同的导出处理器
	switch kind {
	case "events":
		a.exportEvents(w, r, websiteID, from, to, format)
	case "pages":
		a.exportPages(w, r, websiteID, from, to, format)
	case "sessions":
		a.exportSessions(w, r, websiteID, from, to, format)
	default:
		errorResponse(w, http.StatusBadRequest, "unsupported export kind")
	}
}

// exportEvents - 导出事件数据
// 从 events 表中查询指定时间范围的事件记录并导出
func (a *App) exportEvents(w http.ResponseWriter, _ *http.Request, websiteID string, from, to time.Time, format string) {
	// 查询事件记录，最多 20000 条
	rows, err := a.db.Query(`
		select created_at, event_type, event_name, url_path, referrer_domain, browser, os, device, country, region, city, amount, currency, coalesce(cast(json_extract(metadata, '$.duration_ms') as integer), 0) as duration_ms
		from events
		where website_id = ? and created_at between ? and ?
		order by created_at asc
		limit 20000
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query events for export: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	headers := []string{"created_at", "event_type", "event_name", "url_path", "referrer_domain", "browser", "os", "device", "country", "region", "city", "amount", "currency", "duration_ms"}
	var records [][]string
	for rows.Next() {
		var createdAt, eventType, eventName, urlPath, referrer, browser, osName, device, country, region, city, currency string
		var amount float64
		var durationMS int64
		if err := rows.Scan(&createdAt, &eventType, &eventName, &urlPath, &referrer, &browser, &osName, &device, &country, &region, &city, &amount, &currency, &durationMS); err != nil {
			Error("failed to scan event export row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		records = append(records, []string{
			createdAt, eventType, eventName, urlPath, referrer, browser, osName, device, country, region, city, fmt.Sprintf("%.2f", amount), currency, strconv.FormatInt(durationMS, 10),
		})
	}
	writeExport(w, format, "events", headers, records)
}

// exportSessions - 导出会话数据
// 从 sessions 表中查询指定时间范围的会话记录并导出
func (a *App) exportSessions(w http.ResponseWriter, _ *http.Request, websiteID string, from, to time.Time, format string) {
	// 查询会话记录，最多 20000 条
	rows, err := a.db.Query(`
		select started_at, last_seen_at, event_count, pageviews, referrer_domain, utm_source, utm_medium, utm_campaign, browser, os, device, country, region, city, entry_path, exit_path
		from sessions
		where website_id = ? and started_at between ? and ?
		order by started_at asc
		limit 20000
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query sessions for export: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	headers := []string{"started_at", "last_seen_at", "event_count", "pageviews", "referrer_domain", "utm_source", "utm_medium", "utm_campaign", "browser", "os", "device", "country", "region", "city", "entry_path", "exit_path"}
	var records [][]string
	for rows.Next() {
		var startedAt, lastSeenAt, referrer, utmSource, utmMedium, utmCampaign, browser, osName, device, country, region, city, entryPath, exitPath string
		var eventCount, pageviews int
		if err := rows.Scan(&startedAt, &lastSeenAt, &eventCount, &pageviews, &referrer, &utmSource, &utmMedium, &utmCampaign, &browser, &osName, &device, &country, &region, &city, &entryPath, &exitPath); err != nil {
			Error("failed to scan session export row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		records = append(records, []string{
			startedAt, lastSeenAt, strconv.Itoa(eventCount), strconv.Itoa(pageviews), referrer, utmSource, utmMedium, utmCampaign, browser, osName, device, country, region, city, entryPath, exitPath,
		})
	}
	writeExport(w, format, "sessions", headers, records)
}

// exportPages - 导出页面数据
// 查询页面浏览量、会话数和停留时间指标并导出
func (a *App) exportPages(w http.ResponseWriter, _ *http.Request, websiteID string, from, to time.Time, format string) {
	// 查询页面停留时间指标
	dwellByPath, err := a.queryPageDwellMetrics(websiteID, from, to)
	if err != nil {
		Error("failed to query page dwell metrics for export: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 查询页面浏览量和会话数，最多 20000 条
	rows, err := a.db.Query(`
		select
			e.url_path,
			count(*) as pageviews,
			count(distinct e.session_id) as sessions
		from events e
		where e.website_id = ? and e.event_type = 'pageview' and e.created_at between ? and ?
		group by e.url_path
		order by pageviews desc, e.url_path asc
		limit 20000
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query pages for export: %v", err)
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	headers := []string{"path", "pageviews", "sessions", "avg_time_on_page_seconds", "time_on_page_sample_count"}
	var records [][]string
	for rows.Next() {
		var path string
		var pageviews, sessions int64
		if err := rows.Scan(&path, &pageviews, &sessions); err != nil {
			Error("failed to scan page export row: %v", err)
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		// 计算平均停留时间
		dwell := dwellByPath[path]
		avgSeconds := int64(0)
		if dwell.Count > 0 {
			avgSeconds = dwell.DurationMS / dwell.Count / 1000
		}
		records = append(records, []string{
			path,
			strconv.FormatInt(pageviews, 10),
			strconv.FormatInt(sessions, 10),
			strconv.FormatInt(avgSeconds, 10),
			strconv.FormatInt(dwell.Count, 10),
		})
	}
	writeExport(w, format, "pages", headers, records)
}

// loadOverviewCompare - 加载概览对比数据
// 计算当前时间段与上一时间段的指标变化（环比）
func (a *App) loadOverviewCompare(websiteID string, from, to time.Time) (map[string]any, error) {
	// 计算当前时间段的时长
	duration := to.Sub(from)
	// 如果时长为负值，返回空结果
	if duration < 0 {
		return nil, nil
	}
	// 计算上一时间段的起止时间
	prevTo := from.Add(-time.Second)
	prevFrom := prevTo.Add(-duration)

	// 概览数据结构，包含各项核心指标
	type overview struct {
		Pageviews          int64
		Visitors           int64
		Sessions           int64
		Events             int64
		Revenue            float64
		BounceRate         float64
		AvgSessionDuration int64
		AvgTimeOnPage      int64
	}
	var current, previous overview
	var currentCustomEvents, previousCustomEvents int64
	var currentBouncedSessions, previousBouncedSessions int64
	var currentTimeOnPageTotalMS, previousTimeOnPageTotalMS int64
	var currentTimeOnPageSamples, previousTimeOnPageSamples int64
	// 查询当前时间段的聚合数据
	if err := a.db.QueryRow(`
		select coalesce(sum(pageviews), 0), coalesce(sum(custom_events), 0), coalesce(sum(visitors), 0), coalesce(sum(sessions), 0), coalesce(sum(bounced_sessions), 0), coalesce(sum(session_duration_total_seconds), 0), coalesce(sum(time_on_page_total_ms), 0), coalesce(sum(time_on_page_samples), 0), coalesce(sum(revenue), 0)
		from agg_overview_daily
		where website_id = ? and bucket_date between ? and ?
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&current.Pageviews, &currentCustomEvents, &current.Visitors, &current.Sessions, &currentBouncedSessions, &current.AvgSessionDuration, &currentTimeOnPageTotalMS, &currentTimeOnPageSamples, &current.Revenue); err != nil {
		Error("failed to query current period overview compare for website %s: %v", websiteID, err)
		return nil, err
	}
	// 设置当前时间段的自定义事件总数
	current.Events = currentCustomEvents
	// 计算当前时间段的跳出率和平均会话时长
	if current.Sessions > 0 {
		current.BounceRate = float64(currentBouncedSessions) / float64(current.Sessions)
		current.AvgSessionDuration = int64(math.Round(float64(current.AvgSessionDuration) / float64(current.Sessions)))
	}
	// 计算当前时间段的平均页面停留时间
	if currentTimeOnPageSamples > 0 {
		current.AvgTimeOnPage = int64(math.Round(float64(currentTimeOnPageTotalMS) / float64(currentTimeOnPageSamples) / 1000))
	}
	// 查询上一时间段的聚合数据
	if err := a.db.QueryRow(`
		select coalesce(sum(pageviews), 0), coalesce(sum(custom_events), 0), coalesce(sum(visitors), 0), coalesce(sum(sessions), 0), coalesce(sum(bounced_sessions), 0), coalesce(sum(session_duration_total_seconds), 0), coalesce(sum(time_on_page_total_ms), 0), coalesce(sum(time_on_page_samples), 0), coalesce(sum(revenue), 0)
		from agg_overview_daily
		where website_id = ? and bucket_date between ? and ?
	`, websiteID, prevFrom.Format("2006-01-02"), prevTo.Format("2006-01-02")).Scan(&previous.Pageviews, &previousCustomEvents, &previous.Visitors, &previous.Sessions, &previousBouncedSessions, &previous.AvgSessionDuration, &previousTimeOnPageTotalMS, &previousTimeOnPageSamples, &previous.Revenue); err != nil {
		Error("failed to query previous period overview compare for website %s: %v", websiteID, err)
		return nil, err
	}
	// 设置上一时间段的自定义事件总数
	previous.Events = previousCustomEvents
	// 计算上一时间段的跳出率和平均会话时长
	if previous.Sessions > 0 {
		previous.BounceRate = float64(previousBouncedSessions) / float64(previous.Sessions)
		previous.AvgSessionDuration = int64(math.Round(float64(previous.AvgSessionDuration) / float64(previous.Sessions)))
	}
	// 计算上一时间段的平均页面停留时间
	if previousTimeOnPageSamples > 0 {
		previous.AvgTimeOnPage = int64(math.Round(float64(previousTimeOnPageTotalMS) / float64(previousTimeOnPageSamples) / 1000))
	}

	// 返回对比结果，包含前后时间段的各项指标变化
	return map[string]any{
		"from": iso(prevFrom),
		"to":   iso(prevTo),
		"metrics": map[string]any{
			"pageviews": metricDelta(current.Pageviews, previous.Pageviews),
			"visitors":  metricDelta(current.Visitors, previous.Visitors),
			"sessions":  metricDelta(current.Sessions, previous.Sessions),
			"events":    metricDelta(current.Events, previous.Events),
			"revenue":   metricDeltaFloat(current.Revenue, previous.Revenue),
			"bounce_rate": map[string]any{
				"current":     current.BounceRate,
				"previous":    previous.BounceRate,
				"change":      current.BounceRate - previous.BounceRate,
				"change_rate": 0.0,
			},
			"avg_session_duration_seconds": map[string]any{
				"current":     current.AvgSessionDuration,
				"previous":    previous.AvgSessionDuration,
				"change":      current.AvgSessionDuration - previous.AvgSessionDuration,
				"change_rate": 0.0,
			},
			"avg_time_on_page_seconds": map[string]any{
				"current":     current.AvgTimeOnPage,
				"previous":    previous.AvgTimeOnPage,
				"change":      current.AvgTimeOnPage - previous.AvgTimeOnPage,
				"change_rate": 0.0,
			},
		},
	}, nil
}

// listFunnels - 获取指定网站的漏斗列表
// 从数据库查询指定网站下的所有漏斗定义，并反序列化步骤 JSON
func (a *App) listFunnels(websiteID string) ([]Funnel, error) {
	// 查询漏斗列表，按创建时间升序排列
	rows, err := a.db.Query(`
		select id, website_id, name, steps_json, created_at
		from funnels
		where website_id = ?
		order by created_at asc
	`, websiteID)
	if err != nil {
		Error("failed to query funnels for website %s: %v", websiteID, err)
		return nil, err
	}
	defer rows.Close()
	var items []Funnel
	for rows.Next() {
		var item Funnel
		var stepsJSON string
		if err := rows.Scan(&item.ID, &item.WebsiteID, &item.Name, &stepsJSON, &item.CreatedAt); err != nil {
			Error("failed to scan funnel row: %v", err)
			return nil, err
		}
		// 反序列化漏斗步骤 JSON
		_ = json.Unmarshal([]byte(stepsJSON), &item.Steps)
		items = append(items, item)
	}
	return items, rows.Err()
}

// getFunnel - 获取指定漏斗的详细信息
// 根据网站 ID 和漏斗 ID 查询漏斗定义，并反序列化步骤 JSON
func (a *App) getFunnel(websiteID, funnelID string) (Funnel, error) {
	var item Funnel
	var stepsJSON string
	// 查询漏斗记录
	err := a.db.QueryRow(`
		select id, website_id, name, steps_json, created_at
		from funnels
		where website_id = ? and id = ?
	`, websiteID, funnelID).Scan(&item.ID, &item.WebsiteID, &item.Name, &stepsJSON, &item.CreatedAt)
	if err != nil {
		Error("failed to query funnel %s for website %s: %v", funnelID, websiteID, err)
		return Funnel{}, err
	}
	// 反序列化漏斗步骤 JSON
	_ = json.Unmarshal([]byte(stepsJSON), &item.Steps)
	return item, nil
}

// runFunnel - 执行漏斗分析
// 按会话追踪用户在漏斗各步骤中的转化情况，计算每步的会话数、转化率和流失率
func (a *App) runFunnel(funnel Funnel, from, to time.Time) (map[string]any, error) {
	// 查询指定时间范围内的所有事件，按会话和时间排序
	rows, err := a.db.Query(`
		select session_id, event_type, event_name, url_path, created_at
		from events
		where website_id = ? and created_at between ? and ?
		order by session_id asc, created_at asc
	`, funnel.WebsiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query events for funnel analysis: %v", err)
		return nil, err
	}
	defer rows.Close()

	// 事件结构，用于按会话分组存储
	type event struct {
		Type string
		Name string
		Path string
	}
	// 按会话 ID 分组存储事件序列
	eventsBySession := map[string][]event{}
	for rows.Next() {
		var sessionID, eventType, eventName, urlPath, createdText string
		if err := rows.Scan(&sessionID, &eventType, &eventName, &urlPath, &createdText); err != nil {
			Error("failed to scan event row for funnel analysis: %v", err)
			return nil, err
		}
		eventsBySession[sessionID] = append(eventsBySession[sessionID], event{
			Type: eventType,
			Name: eventName,
			Path: urlPath,
		})
	}

	// 统计每个漏斗步骤的匹配会话数
	counts := make([]int, len(funnel.Steps))
	for _, events := range eventsBySession {
		// 按事件顺序逐步匹配漏斗步骤
		stepIndex := 0
		for _, item := range events {
			if stepIndex >= len(funnel.Steps) {
				break
			}
			step := funnel.Steps[stepIndex]
			if matchesStep(step, item) {
				counts[stepIndex]++
				stepIndex++
			}
		}
	}

	// 计算每个步骤的转化率和流失率
	var steps []map[string]any
	// 记录第一步的会话数，作为转化率计算的基准
	firstCount := 0
	if len(counts) > 0 {
		firstCount = counts[0]
	}
	for i, step := range funnel.Steps {
		conversion := 0.0
		dropOffCount := 0
		dropOffRate := 0.0
		// 计算相对第一步的总转化率
		if firstCount > 0 {
			conversion = float64(counts[i]) / float64(firstCount)
		}
		// 计算相对上一步的流失数和流失率
		if i > 0 {
			dropOffCount = counts[i-1] - counts[i]
			if counts[i-1] > 0 {
				dropOffRate = float64(dropOffCount) / float64(counts[i-1])
			}
		}
		steps = append(steps, map[string]any{
			"index":          i + 1,
			"label":          step.Label,
			"type":           step.Type,
			"value":          step.Value,
			"sessions":       counts[i],
			"conversion":     conversion,
			"drop_off_count": dropOffCount,
			"drop_off_rate":  dropOffRate,
		})
	}
	return map[string]any{
		"steps": steps,
	}, nil
}

// matchesStep - 判断事件是否匹配漏斗步骤
// 支持页面类型（pageview + 路径匹配）和事件类型（名称匹配）
func matchesStep(step FunnelStep, item struct {
	Type string
	Name string
	Path string
}) bool {
	switch step.Type {
	case "page":
		return item.Type == "pageview" && pathMatchesStepValue(item.Path, step.Value)
	case "event":
		return strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(step.Value))
	default:
		return false
	}
}

// pathMatchesStepValue - 判断请求路径是否匹配漏斗步骤的预期值
// 支持精确匹配、通配符匹配（/* 和 /**）以及尾部斜杠忽略
func pathMatchesStepValue(pathValue, expected string) bool {
	pathValue = strings.TrimSpace(pathValue)
	expected = strings.TrimSpace(expected)
	if pathValue == expected {
		return true
	}
	if expected == "" {
		return false
	}
	if strings.HasSuffix(expected, "/**") {
		prefix := strings.TrimSuffix(expected, "**")
		return strings.HasPrefix(pathValue, prefix)
	}
	if strings.HasSuffix(expected, "/*") {
		prefix := strings.TrimSuffix(expected, "*")
		return strings.HasPrefix(pathValue, prefix)
	}
	expected = strings.TrimRight(expected, "/")
	pathValue = strings.TrimRight(pathValue, "/")
	return pathValue == expected
}

// handlePublicShare - 公开分享数据端点
// GET /api/public/shares/{slug} - 通过分享链接 slug 获取网站的公开统计数据
// 无需认证，但分享链接必须处于启用状态
func (a *App) handlePublicShare(w http.ResponseWriter, r *http.Request) {
	// 从 URL 路径中提取分享链接 slug
	slug := strings.TrimPrefix(r.URL.Path, "/api/public/shares/")
	var share Share
	var enabled int
	// 根据 slug 查询分享链接记录
	err := a.db.QueryRow(`
		select id, website_id, slug, enabled, created_at
		from shares
		where slug = ?
	`, slug).Scan(&share.ID, &share.WebsiteID, &share.Slug, &enabled, &share.CreatedAt)
	// 分享链接不存在或未启用时返回 404
	if err != nil || enabled != 1 {
		http.NotFound(w, r)
		return
	}
	share.Enabled = true
	// 解析日期范围参数
	from, to, err := a.parseDateRange(r)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	// 查询关联的网站信息
	var website Website
	if err := a.db.QueryRow(`select id, name, domain, created_at, updated_at from websites where id = ?`, share.WebsiteID).
		Scan(&website.ID, &website.Name, &website.Domain, &website.CreatedAt, &website.UpdatedAt); err != nil {
		Error("failed to query website for public share %s: %v", share.ID, err)
		http.NotFound(w, r)
		return
	}
	// 获取各项公开统计数据
	overview := a.publicOverview(share.WebsiteID, from, to)
	pages := a.queryPublicPages(share.WebsiteID, from, to)
	// 查询来源域名分组统计
	referrers := a.queryGroupedItems(`
		select case when referrer_domain = '' then '(direct)' else referrer_domain end as label, count(*) as count
		from sessions
		where website_id = ? and started_at between ? and ?
		group by label
		order by count desc, label asc
		limit 20
	`, share.WebsiteID, from, to)
	revenue := a.queryRevenueItems(share.WebsiteID, from, to)
	attribution := a.queryPublicAttributionItems(share.WebsiteID, from, to)
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":          true,
		"website":     website,
		"overview":    overview,
		"pages":       pages,
		"referrers":   referrers,
		"revenue":     revenue,
		"attribution": attribution,
	})
}

// publicOverview - 获取公开分享的概览数据
// 从事件表中聚合页面浏览量、访客数、会话数、事件数、收入和平均停留时间
func (a *App) publicOverview(websiteID string, from, to time.Time) map[string]any {
	// 概览数据结构
	type overview struct {
		Pageviews     int64   `json:"pageviews"`
		Visitors      int64   `json:"visitors"`
		Sessions      int64   `json:"sessions"`
		Events        int64   `json:"events"`
		Revenue       float64 `json:"revenue"`
		AvgTimeOnPage int64   `json:"avg_time_on_page_seconds"`
	}
	var out overview
	// 从事件表直接聚合统计（不使用每日聚合表，确保实时性）
	_ = a.db.QueryRow(`
		select
			sum(case when event_type = 'pageview' then 1 else 0 end) as pageviews,
			count(distinct visitor_id) as visitors,
			count(distinct session_id) as sessions,
			count(*) as events,
			sum(amount) as revenue
		from events
		where website_id = ? and created_at between ? and ?
	`, websiteID, iso(from), iso(to)).Scan(&out.Pageviews, &out.Visitors, &out.Sessions, &out.Events, &out.Revenue)
	// 查询平均页面停留时间
	out.AvgTimeOnPage = a.queryAverageTimeOnPageSeconds(websiteID, from, to)
	return map[string]any{
		"pageviews":                out.Pageviews,
		"visitors":                 out.Visitors,
		"sessions":                 out.Sessions,
		"events":                   out.Events,
		"revenue":                  out.Revenue,
		"avg_time_on_page_seconds": out.AvgTimeOnPage,
	}
}

// pageDwellMetric - 页面停留时间指标
// 记录某个页面的总停留时长（毫秒）和采样次数
type pageDwellMetric struct {
	DurationMS int64
	Count      int64
}

// queryPageDwellMetrics - 查询页面停留时间指标
// 从事件表中统计每个路径的页面停留总时长和采样次数
func (a *App) queryPageDwellMetrics(websiteID string, from, to time.Time) (map[string]pageDwellMetric, error) {
	// 查询 page_leave 和 page_ping 事件的停留时长
	rows, err := a.db.Query(`
		select
			url_path,
			coalesce(sum(cast(json_extract(metadata, '$.duration_ms') as integer)), 0) as duration_ms,
			count(*) as samples
		from events
		where website_id = ?
			and event_name in ('page_leave', 'page_ping')
			and created_at between ? and ?
			and trim(url_path) <> ''
		group by url_path
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query page dwell metrics for website %s: %v", websiteID, err)
		return nil, err
	}
	defer rows.Close()

	// 按路径聚合停留时间指标
	out := map[string]pageDwellMetric{}
	for rows.Next() {
		var path string
		var durationMS, samples int64
		if err := rows.Scan(&path, &durationMS, &samples); err != nil {
			Error("failed to scan page dwell metric row: %v", err)
			return nil, err
		}
		out[path] = pageDwellMetric{DurationMS: durationMS, Count: samples}
	}
	return out, rows.Err()
}

// queryAverageTimeOnPageSeconds - 查询平均页面停留时间（秒）
// 从 page_leave 和 page_ping 事件中计算平均停留时长
func (a *App) queryAverageTimeOnPageSeconds(websiteID string, from, to time.Time) int64 {
	var avgSeconds float64
	if err := a.db.QueryRow(`
		select coalesce(avg(cast(json_extract(metadata, '$.duration_ms') as integer) / 1000), 0)
		from events
		where website_id = ?
			and event_name in ('page_leave', 'page_ping')
			and created_at between ? and ?
	`, websiteID, iso(from), iso(to)).Scan(&avgSeconds); err != nil {
		Error("failed to query average time on page for website %s: %v", websiteID, err)
		return 0
	}
	return int64(math.Round(avgSeconds))
}

// queryPublicAttributionItems - 查询公开分享的归因数据
// 按来源和媒介分组统计会话数，用于公开分享页面展示
func (a *App) queryPublicAttributionItems(websiteID string, from, to time.Time) []map[string]any {
	rows, err := a.db.Query(`
		select
			case when utm_source <> '' then utm_source when referrer_domain <> '' then referrer_domain else '(direct)' end as source,
			case when utm_medium <> '' then utm_medium when referrer_domain <> '' then 'referral' else '(none)' end as medium,
			count(*) as sessions
		from sessions
		where website_id = ? and started_at between ? and ?
		group by source, medium
		order by sessions desc, source asc
		limit 20
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query public attribution items for website %s: %v", websiteID, err)
		return []map[string]any{}
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var source, medium string
		var sessions int64
		if err := rows.Scan(&source, &medium, &sessions); err == nil {
			items = append(items, map[string]any{"source": source, "medium": medium, "sessions": sessions})
		}
	}
	return items
}

// queryGroupedItems - 通用分组查询函数
// 执行指定的 SQL 查询，返回按 label 和 count 分组的统计结果
func (a *App) queryGroupedItems(query, websiteID string, from, to time.Time) []map[string]any {
	rows, err := a.db.Query(query, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query grouped items for website %s: %v", websiteID, err)
		return nil
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var label string
		var count int64
		if err := rows.Scan(&label, &count); err == nil {
			items = append(items, map[string]any{"label": label, "count": count})
		}
	}
	return items
}

// queryRevenueItems - 查询收入统计项
// 按货币分组统计收入金额，用于公开分享页面展示
func (a *App) queryRevenueItems(websiteID string, from, to time.Time) []map[string]any {
	rows, err := a.db.Query(`
		select case when currency = '' then 'N/A' else currency end as currency, sum(amount) as revenue
		from events
		where website_id = ? and created_at between ? and ? and amount > 0
		group by currency
		order by revenue desc, currency asc
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query revenue items for website %s: %v", websiteID, err)
		return nil
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var currency string
		var revenue float64
		if err := rows.Scan(&currency, &revenue); err == nil {
			items = append(items, map[string]any{"currency": currency, "revenue": revenue})
		}
	}
	return items
}

// queryPublicPages - 查询公开分享的页面统计数据
// 返回页面浏览量、会话数和平均停留时间，用于公开分享页面展示
func (a *App) queryPublicPages(websiteID string, from, to time.Time) []map[string]any {
	// 先查询页面停留时间指标
	dwellByPath, err := a.queryPageDwellMetrics(websiteID, from, to)
	if err != nil {
		Error("failed to query page dwell metrics for public pages: %v", err)
		return nil
	}
	// 查询页面浏览量统计，按浏览量降序排列
	rows, err := a.db.Query(`
		select url_path, count(*) as count
		from events
		where website_id = ? and event_type = 'pageview' and created_at between ? and ?
		group by url_path
		order by count desc, url_path asc
		limit 20
	`, websiteID, iso(from), iso(to))
	if err != nil {
		Error("failed to query public pages for website %s: %v", websiteID, err)
		return nil
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var path string
		var count int64
		if err := rows.Scan(&path, &count); err == nil {
			// 计算平均停留时间
			dwell := dwellByPath[path]
			avgSeconds := int64(0)
			if dwell.Count > 0 {
				avgSeconds = dwell.DurationMS / dwell.Count / 1000
			}
			items = append(items, map[string]any{
				"label":                    path,
				"count":                    count,
				"avg_time_on_page_seconds": avgSeconds,
			})
		}
	}
	return items
}

// analyticsContext - 解析分析请求的通用上下文
// 提取用户身份、网站 ID 和时间范围，用于所有分析端点的公共参数解析
func (a *App) analyticsContext(w http.ResponseWriter, r *http.Request) (*AuthUser, string, time.Time, time.Time, bool) {
	// 验证用户身份
	user, ok := a.requireUser(w, r)
	if !ok {
		return nil, "", time.Time{}, time.Time{}, false
	}
	// 获取网站 ID 参数
	websiteID := strings.TrimSpace(r.URL.Query().Get("website_id"))
	if websiteID == "" {
		errorResponse(w, http.StatusBadRequest, "website_id required")
		return nil, "", time.Time{}, time.Time{}, false
	}
	// 解析日期范围参数
	from, to, err := a.parseDateRange(r)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return nil, "", time.Time{}, time.Time{}, false
	}
	return user, websiteID, from, to, true
}
