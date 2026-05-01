package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

var onePixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00,
	0x01, 0x00, 0x80, 0x00, 0x00, 0xff, 0xff, 0xff,
	0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00, 0x3b,
}

type createInitRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type userInput struct {
	Username    string              `json:"username"`
	Password    string              `json:"password"`
	Role        string              `json:"role"`
	Enabled     *bool               `json:"enabled,omitempty"`
	Permissions []WebsitePermission `json:"permissions"`
}

type websiteInput struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

type pixelInput struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type shareInput struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type funnelInput struct {
	Name  string       `json:"name"`
	Steps []FunnelStep `json:"steps"`
}

type settingsInput struct {
	ListenAddr   string `json:"listen_addr"`
	DatabasePath string `json:"database_path"`
	LogLevel     string `json:"log_level"`
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version,
	})
}

func (a *App) handleTracker(w http.ResponseWriter, r *http.Request) {
	script := `(function () {
  var script = document.currentScript;
  if (!script) return;
  var website = script.getAttribute("data-website-id");
  if (!website) return;
  var origin = new URL(script.src, window.location.href).origin;
  var storageKey = "sitlys.visitor." + website;
  var visitorId = localStorage.getItem(storageKey);
  if (!visitorId) {
    visitorId = (crypto && crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(16).slice(2) + Date.now().toString(16)).replace(/-/g, "");
    localStorage.setItem(storageKey, visitorId);
  }

  function collect(type, payload) {
    var body = JSON.stringify({ type: type, payload: payload });
    if (navigator.sendBeacon) {
      var blob = new Blob([body], { type: "application/json" });
      navigator.sendBeacon(origin + "/api/send", blob);
      return;
    }
    fetch(origin + "/api/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body,
      keepalive: true
    }).catch(function () {});
  }

  function basePayload(extra) {
    var payload = {
      website: website,
      url: window.location.href,
      hostname: window.location.hostname,
      title: document.title,
      referrer: document.referrer || "",
      language: navigator.language || "",
      screen: window.screen ? window.screen.width + "x" + window.screen.height : "",
      id: visitorId,
      timestamp: Date.now()
    };
    return Object.assign(payload, extra || {});
  }

  window.sitlys = {
    track: function (name, data) {
      collect("event", basePayload({ name: name, data: data || {} }));
    },
    revenue: function (name, amount, currency, data) {
      collect("revenue", basePayload({
        name: name,
        data: data || {},
        revenue: { amount: Number(amount || 0), currency: currency || "USD" }
      }));
    }
  };

  collect("event", basePayload());
})();`
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	http.ServeContent(w, r, "tracker.js", nowUTC(), strings.NewReader(script))
}

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

func (a *App) serveStaticFile(w http.ResponseWriter, r *http.Request, name, contentType string) {
	data, err := fs.ReadFile(a.staticFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, name, nowUTC(), strings.NewReader(string(data)))
}

func (a *App) handleStatus(w http.ResponseWriter, _ *http.Request) {
	hasUsers, err := a.hasUsers()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":          true,
		"version":     version,
		"initialized": hasUsers,
	})
}

func (a *App) handleInit(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := a.hasUsers()
	if err != nil {
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
		errorResponse(w, http.StatusInternalServerError, "create admin failed")
		return
	}

	token, expires, err := a.createSession(userID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "create session failed")
		return
	}
	a.setSessionCookie(w, token, expires)
	jsonResponse(w, http.StatusCreated, map[string]any{"ok": true})
}

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
		errorResponse(w, http.StatusInternalServerError, "create session failed")
		return
	}
	a.setSessionCookie(w, token, expires)
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		_, _ = a.db.Exec(`delete from auth_sessions where token_hash = ?`, tokenHash(cookie.Value))
	}
	a.clearSessionCookie(w)
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

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
		errorResponse(w, http.StatusInternalServerError, "load user failed")
		return
	}
	if !passwordMatch(hash, req.CurrentPassword) {
		errorResponse(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	newHash, err := passwordHash(req.NewPassword)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "hash password failed")
		return
	}
	if _, err := a.db.Exec(`
		update users
		set password_hash = ?, updated_at = ?
		where id = ?
	`, newHash, iso(nowUTC()), user.ID); err != nil {
		errorResponse(w, http.StatusInternalServerError, "update password failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

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
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var users []AuthUser
		for rows.Next() {
			var item AuthUser
			var enabled int
			if err := rows.Scan(&item.ID, &item.Username, &item.Role, &enabled, &item.CreatedAt); err != nil {
				errorResponse(w, http.StatusInternalServerError, err.Error())
				return
			}
			item.Enabled = enabled == 1
			item.AllWebsites = item.Role == roleSuperAdmin
			item.Permissions, err = a.permissionsForUser(item.ID)
			if err != nil {
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
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := tx.Commit(); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": userID})
	default:
		http.NotFound(w, r)
	}
}

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
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

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
			errorResponse(w, http.StatusInternalServerError, "hash password failed")
			return
		}
		parts = append(parts, "password_hash = ?")
		args = append(args, hash)
	}
	parts = append(parts, "updated_at = ?")
	args = append(args, iso(nowUTC()), userID)
	if len(parts) > 1 {
		if _, err := tx.Exec(`update users set `+strings.Join(parts, ", ")+` where id = ?`, args...); err != nil {
			errorResponse(w, http.StatusBadRequest, "update user failed")
			return
		}
	}
	if req.Permissions != nil {
		if err := upsertPermissions(tx, userID, req.Permissions); err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

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

func upsertPermissions(tx *sql.Tx, userID string, permissions []WebsitePermission) error {
	if _, err := tx.Exec(`delete from website_permissions where user_id = ?`, userID); err != nil {
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
			return err
		}
	}
	return nil
}

func (a *App) handleWebsites(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		websites, err := a.listWebsites(user)
		if err != nil {
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
		req.Domain = strings.TrimSpace(req.Domain)
		if req.Name == "" || req.Domain == "" {
			errorResponse(w, http.StatusBadRequest, "name and domain required")
			return
		}
		now := iso(nowUTC())
		websiteID := newID()
		tx, err := a.db.Begin()
		if err != nil {
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
		if user.Role != roleSuperAdmin {
			if _, err := tx.Exec(`
				insert into website_permissions(user_id, website_id, access_level, created_at)
				values(?, ?, ?, ?)
			`, user.ID, websiteID, "manage", now); err != nil {
				errorResponse(w, http.StatusInternalServerError, "assign permission failed")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": websiteID})
	default:
		http.NotFound(w, r)
	}
}

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
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Domain) == "" {
			errorResponse(w, http.StatusBadRequest, "name and domain required")
			return
		}
		_, err := a.db.Exec(`
			update websites
			set name = ?, domain = ?, updated_at = ?
			where id = ?
		`, strings.TrimSpace(req.Name), strings.TrimSpace(req.Domain), iso(nowUTC()), websiteID)
		if err != nil {
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
			errorResponse(w, http.StatusInternalServerError, "delete website failed")
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

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
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		var pixels []Pixel
		for rows.Next() {
			var item Pixel
			var enabled int
			if err := rows.Scan(&item.ID, &item.WebsiteID, &item.Name, &item.Slug, &enabled, &item.CreatedAt); err != nil {
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
			errorResponse(w, http.StatusInternalServerError, "create pixel failed")
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": pixelID})
	default:
		http.NotFound(w, r)
	}
}

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
		errorResponse(w, http.StatusInternalServerError, "update pixel failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

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
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		var shares []Share
		for rows.Next() {
			var item Share
			var enabled int
			if err := rows.Scan(&item.ID, &item.WebsiteID, &item.Slug, &enabled, &item.CreatedAt); err != nil {
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
			errorResponse(w, http.StatusInternalServerError, "create share failed")
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": shareID})
	default:
		http.NotFound(w, r)
	}
}

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
		errorResponse(w, http.StatusInternalServerError, "update share failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleWebsiteFunnels(w http.ResponseWriter, r *http.Request, user *AuthUser, websiteID string) {
	switch r.Method {
	case http.MethodGet:
		funnels, err := a.listFunnels(websiteID)
		if err != nil {
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
			errorResponse(w, http.StatusInternalServerError, "create funnel failed")
			return
		}
		jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "id": funnelID})
	default:
		http.NotFound(w, r)
	}
}

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
		return nil, err
	}
	defer rows.Close()
	var websites []Website
	for rows.Next() {
		var item Website
		if err := rows.Scan(&item.ID, &item.Name, &item.Domain, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		websites = append(websites, item)
	}
	return websites, rows.Err()
}

func (a *App) handleCollectPixel(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/collect/p/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	var pixelID string
	var enabled int
	err := a.db.QueryRow(`select id, enabled from pixels where slug = ?`, slug).Scan(&pixelID, &enabled)
	if err != nil || enabled != 1 {
		http.NotFound(w, r)
		return
	}
	req := eventRequest{
		Type: "event",
		Payload: eventPayload{
			Pixel:    pixelID,
			URL:      r.URL.String(),
			Referrer: r.Referer(),
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

func (a *App) handleSend(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if err := decodeJSON(r, &req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := a.recordEvent(r, req)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

func (a *App) handleBatch(w http.ResponseWriter, r *http.Request) {
	var reqs []eventRequest
	if err := decodeJSON(r, &reqs); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	results := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		res, err := a.recordEvent(r, req)
		if err != nil {
			results = append(results, map[string]any{"ok": false, "error": err.Error()})
			continue
		}
		results = append(results, map[string]any{"ok": true, "result": res})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": results})
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
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
		settings, err := a.getSystemSettings()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{
			"ok":       true,
			"settings": settings,
			"version":  version,
		})
	case http.MethodPut:
		var req settingsInput
		if err := decodeJSON(r, &req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.ListenAddr = strings.TrimSpace(req.ListenAddr)
		req.DatabasePath = strings.TrimSpace(req.DatabasePath)
		req.LogLevel = strings.TrimSpace(strings.ToLower(req.LogLevel))

		if req.ListenAddr == "" || req.DatabasePath == "" {
			errorResponse(w, http.StatusBadRequest, "listen_addr and database_path required")
			return
		}
		if req.LogLevel == "" {
			req.LogLevel = "info"
		}
		if err := a.setSystemSettings(map[string]string{
			"listen_addr":   req.ListenAddr,
			"database_path": req.DatabasePath,
			"log_level":     req.LogLevel,
		}); err != nil {
			errorResponse(w, http.StatusInternalServerError, "save settings failed")
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleBackup(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if user.Role != roleSuperAdmin {
		errorResponse(w, http.StatusForbidden, "super admin required")
		return
	}
	path, err := a.createBackup()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "backup failed")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":          true,
		"backup_path": path,
	})
}

func (a *App) recordEvent(r *http.Request, req eventRequest) (map[string]any, error) {
	if req.Type == "" {
		req.Type = "event"
	}
	if req.Type != "event" && req.Type != "revenue" && req.Type != "identify" {
		return nil, fmt.Errorf("unsupported event type")
	}

	payload := req.Payload
	websiteID := strings.TrimSpace(payload.Website)
	pixelID := strings.TrimSpace(payload.Pixel)
	if websiteID == "" && pixelID == "" {
		return nil, fmt.Errorf("website or pixel is required")
	}
	if websiteID != "" && pixelID != "" {
		return nil, fmt.Errorf("website and pixel cannot both be provided")
	}

	if pixelID != "" {
		var enabled int
		if err := a.db.QueryRow(`select website_id, enabled from pixels where id = ?`, pixelID).Scan(&websiteID, &enabled); err != nil {
			return nil, fmt.Errorf("pixel not found")
		}
		if enabled != 1 {
			return nil, fmt.Errorf("pixel disabled")
		}
	}
	if !a.websiteExists(websiteID) {
		return nil, fmt.Errorf("website not found")
	}

	createdAt := nowUTC()
	if payload.Timestamp != 0 {
		if payload.Timestamp > 1_000_000_000_000 {
			createdAt = time.UnixMilli(payload.Timestamp).UTC()
		} else {
			createdAt = time.Unix(payload.Timestamp, 0).UTC()
		}
	}

	fullURL := payload.URL
	if fullURL == "" {
		fullURL = r.Header.Get("Origin")
	}
	if !strings.Contains(fullURL, "://") && fullURL != "" {
		fullURL = "https://" + strings.TrimPrefix(fullURL, "/")
	}
	parsedURL, host, pathValue, _ := cleanURL(fullURL)
	refDomain := referrerDomain(payload.Referrer)
	browser, osName, device := detectUserAgent(r, payload)
	if payload.Hostname != "" {
		host = payload.Hostname
	}
	if pathValue == "" {
		pathValue = "/"
	}

	visitorKey := payload.ID
	if visitorKey == "" {
		visitorKey = tokenHash(websiteID + "|" + clientIP(r) + "|" + r.UserAgent())
	}

	metadata, _ := json.Marshal(payload.Data)
	eventType := normalizeEventType(payload, pixelID)
	amount := 0.0
	currency := ""
	if payload.Revenue != nil {
		amount = payload.Revenue.Amount
		currency = strings.ToUpper(strings.TrimSpace(payload.Revenue.Currency))
	}
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
		Country:        payload.Country,
		Region:         payload.Region,
		City:           payload.City,
		Amount:         amount,
		Currency:       currency,
		Metadata:       string(metadata),
		CreatedAt:      createdAt,
	}

	select {
	case a.eventQueue <- item:
	default:
		return nil, fmt.Errorf("event queue is full")
	}

	return map[string]any{
		"website_id": websiteID,
		"event_type": eventType,
		"queued":     true,
	}, nil
}

func (a *App) websiteExists(websiteID string) bool {
	var count int
	if err := a.db.QueryRow(`select count(*) from websites where id = ?`, websiteID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func (a *App) upsertVisitor(websiteID, externalID string, seenAt time.Time) (string, error) {
	var visitorID string
	err := a.db.QueryRow(`
		select id
		from visitors
		where website_id = ? and external_id = ?
	`, websiteID, externalID).Scan(&visitorID)
	switch {
	case err == nil:
		_, err = a.db.Exec(`update visitors set last_seen_at = ? where id = ?`, iso(seenAt), visitorID)
		return visitorID, err
	case !errors.Is(err, sql.ErrNoRows):
		return "", err
	}
	visitorID = newID()
	_, err = a.db.Exec(`
		insert into visitors(id, website_id, external_id, first_seen_at, last_seen_at)
		values(?, ?, ?, ?, ?)
	`, visitorID, websiteID, externalID, iso(seenAt), iso(seenAt))
	return visitorID, err
}

func (a *App) findOrCreateSession(candidate sessionRecord) (sessionRecord, error) {
	var existing sessionRecord
	var startedAtText, lastSeenText string
	row := a.db.QueryRow(`
		select id, website_id, visitor_id, started_at, last_seen_at, event_count, pageviews,
		       referrer, referrer_domain, utm_source, utm_medium, utm_campaign,
		       browser, os, device, country, region, city, entry_path, exit_path
		from sessions
		where website_id = ? and visitor_id = ?
		order by last_seen_at desc
		limit 1
	`, candidate.WebsiteID, candidate.VisitorID)
	err := row.Scan(
		&existing.ID, &existing.WebsiteID, &existing.VisitorID, &startedAtText, &lastSeenText,
		&existing.EventCount, &existing.Pageviews, &existing.Referrer, &existing.ReferrerDomain,
		&existing.UTMSource, &existing.UTMMedium, &existing.UTMCampaign, &existing.Browser,
		&existing.OS, &existing.Device, &existing.Country, &existing.Region, &existing.City,
		&existing.EntryPath, &existing.ExitPath,
	)
	if err == nil {
		existing.StartedAt = parseISO(startedAtText)
		existing.LastSeenAt = parseISO(lastSeenText)
	}
	if err == nil && candidate.StartedAt.Sub(existing.LastSeenAt) <= 30*time.Minute {
		existing.LastSeenAt = candidate.LastSeenAt
		existing.ExitPath = candidate.ExitPath
		existing.EventCount++
		if candidate.EntryPath != "" {
			existing.Pageviews++
		}
		_, err := a.db.Exec(`
			update sessions
			set last_seen_at = ?, event_count = ?, pageviews = ?, exit_path = ?
			where id = ?
		`, iso(existing.LastSeenAt), existing.EventCount, existing.Pageviews, existing.ExitPath, existing.ID)
		return existing, err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sessionRecord{}, err
	}

	candidate.ID = newID()
	candidate.EventCount = 1
	if candidate.EntryPath != "" {
		candidate.Pageviews = 1
	}
	_, err = a.db.Exec(`
		insert into sessions(
			id, website_id, visitor_id, started_at, last_seen_at, event_count, pageviews,
			referrer, referrer_domain, utm_source, utm_medium, utm_campaign,
			browser, os, device, country, region, city, entry_path, exit_path
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		candidate.ID, candidate.WebsiteID, candidate.VisitorID, iso(candidate.StartedAt), iso(candidate.LastSeenAt),
		candidate.EventCount, candidate.Pageviews, candidate.Referrer, candidate.ReferrerDomain,
		candidate.UTMSource, candidate.UTMMedium, candidate.UTMCampaign, candidate.Browser, candidate.OS,
		candidate.Device, candidate.Country, candidate.Region, candidate.City, candidate.EntryPath, candidate.ExitPath,
	)
	return candidate, err
}

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
	return err
}

func (r eventRecord) PixelValue() any {
	if r.PixelID == "" {
		return nil
	}
	return r.PixelID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractUTM(rawURL, key string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Query().Get(key)
}

func (a *App) handleOverview(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}

	type overview struct {
		Pageviews int64   `json:"pageviews"`
		Visitors  int64   `json:"visitors"`
		Sessions  int64   `json:"sessions"`
		Events    int64   `json:"events"`
		Revenue   float64 `json:"revenue"`
	}
	var out overview
	if err := a.db.QueryRow(`
		select
			coalesce(sum(pageviews), 0),
			coalesce(sum(custom_events), 0),
			coalesce(sum(revenue), 0)
		from agg_overview_daily
		where website_id = ? and bucket_date between ? and ?
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&out.Pageviews, &out.Events, &out.Revenue); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.db.QueryRow(`
		select count(distinct visitor_id), count(*)
		from sessions
		where website_id = ? and started_at between ? and ?
	`, websiteID, iso(from), iso(to)).Scan(&out.Visitors, &out.Sessions); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	trendRows, err := a.db.Query(`
		select bucket_date, pageviews, custom_events, revenue
		from agg_overview_daily
		where website_id = ? and bucket_date between ? and ?
		order by bucket_date asc
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer trendRows.Close()
	var trend []map[string]any
	for trendRows.Next() {
		var day string
		var pageviews, events int64
		var revenue float64
		if err := trendRows.Scan(&day, &pageviews, &events, &revenue); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		trend = append(trend, map[string]any{
			"date":      day,
			"pageviews": pageviews,
			"events":    events,
			"revenue":   revenue,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "overview": out, "trend": trend})
}

func (a *App) handlePages(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	rows, err := a.db.Query(`
		select url_path, sum(pageviews) as pageviews
		from agg_pages_daily
		where website_id = ? and bucket_date between ? and ?
		group by url_path
		order by pageviews desc, url_path asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var path string
		var pageviews int64
		if err := rows.Scan(&path, &pageviews); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		var sessions int64
		_ = a.db.QueryRow(`
			select count(distinct session_id)
			from events
			where website_id = ? and url_path = ? and event_type = 'pageview' and created_at between ? and ?
		`, websiteID, path, iso(from), iso(to)).Scan(&sessions)
		items = append(items, map[string]any{
			"path":      path,
			"pageviews": pageviews,
			"sessions":  sessions,
		})
	}
	entryRows, err := a.db.Query(`
		select entry_path, count(*) as sessions
		from sessions
		where website_id = ? and started_at between ? and ? and trim(entry_path) <> ''
		group by entry_path
		order by sessions desc, entry_path asc
		limit 20
	`, websiteID, iso(from), iso(to))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer entryRows.Close()
	var entries []map[string]any
	for entryRows.Next() {
		var path string
		var sessions int64
		if err := entryRows.Scan(&path, &sessions); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		entries = append(entries, map[string]any{
			"path":     path,
			"sessions": sessions,
		})
	}

	exitRows, err := a.db.Query(`
		select exit_path, count(*) as sessions
		from sessions
		where website_id = ? and started_at between ? and ? and trim(exit_path) <> ''
		group by exit_path
		order by sessions desc, exit_path asc
		limit 20
	`, websiteID, iso(from), iso(to))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer exitRows.Close()
	var exits []map[string]any
	for exitRows.Next() {
		var path string
		var sessions int64
		if err := exitRows.Scan(&path, &sessions); err != nil {
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

func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
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
			and created_at between ? and ?
		group by event_type, label
		order by events desc, label asc
		limit 100
	`, websiteID, iso(from), iso(to))
	if err != nil {
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
	var types []map[string]any
	for eventType, events := range typeRows {
		types = append(types, map[string]any{
			"type":   eventType,
			"events": events,
		})
	}
	sort.Slice(types, func(i, j int) bool {
		return types[i]["events"].(int64) > types[j]["events"].(int64)
	})
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items, "types": types})
}

func (a *App) handleReferrers(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	rows, err := a.db.Query(`
		select referrer_domain, sum(sessions) as visits, sum(revenue) as revenue
		from agg_referrers_daily
		where website_id = ? and bucket_date between ? and ?
		group by referrer_domain
		order by visits desc, referrer_domain asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
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
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
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

func (a *App) handleDevices(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	payload := map[string]any{
		"browsers": a.aggDeviceCount(websiteID, from, to, "browser"),
		"os":       a.aggDeviceCount(websiteID, from, to, "os"),
		"devices":  a.aggDeviceCount(websiteID, from, to, "device"),
		"matrix":   a.aggDeviceMatrix(websiteID, from, to),
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": payload})
}

func (a *App) handleGeo(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	rows, err := a.db.Query(`
		select country, sum(sessions) as visits
		from agg_geo_daily
		where website_id = ? and bucket_date between ? and ?
		group by country
		order by visits desc, country asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var country string
		var visits int64
		if err := rows.Scan(&country, &visits); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if country == "" {
			country = "(unknown)"
		}
		items = append(items, map[string]any{"country": country, "visits": visits})
	}

	regionRows, err := a.db.Query(`
		select region, sum(sessions) as visits
		from agg_geo_daily
		where website_id = ? and bucket_date between ? and ? and trim(region) <> ''
		group by region
		order by visits desc, region asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer regionRows.Close()
	var regions []map[string]any
	for regionRows.Next() {
		var region string
		var visits int64
		if err := regionRows.Scan(&region, &visits); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		regions = append(regions, map[string]any{"region": region, "visits": visits})
	}

	cityRows, err := a.db.Query(`
		select city, sum(sessions) as visits
		from agg_geo_daily
		where website_id = ? and bucket_date between ? and ? and trim(city) <> ''
		group by city
		order by visits desc, city asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cityRows.Close()
	var cities []map[string]any
	for cityRows.Next() {
		var city string
		var visits int64
		if err := cityRows.Scan(&city, &visits); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		cities = append(cities, map[string]any{"city": city, "visits": visits})
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":      true,
		"items":   items,
		"regions": regions,
		"cities":  cities,
	})
}

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

func (a *App) handleAttribution(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	rows, err := a.db.Query(`
		select source, medium, campaign, sum(sessions) as sessions, sum(revenue) as revenue
		from agg_attribution_daily
		where website_id = ? and bucket_date between ? and ?
		group by source, medium, campaign
		order by sessions desc, source asc
		limit 100
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
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
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, row)
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

func (a *App) handleRevenue(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	rows, err := a.db.Query(`
		select source, currency, sum(event_count) as events, sum(revenue) as revenue
		from agg_revenue_daily
		where website_id = ? and bucket_date between ? and ?
		group by source, currency
		order by revenue desc, source asc
	`, websiteID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
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

func (a *App) handleRetention(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	rows, err := a.db.Query(`
		select visitor_id, date(started_at)
		from sessions
		where website_id = ?
		order by visitor_id asc, started_at asc
	`, websiteID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

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
		var visitorID, dayText string
		if err := rows.Scan(&visitorID, &dayText); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		day, _ := time.ParseInLocation("2006-01-02", dayText, time.UTC)
		if _, ok := first[visitorID]; !ok {
			first[visitorID] = day
		}
		seen[visitorID] = append(seen[visitorID], day)
	}

	for visitorID, days := range seen {
		cohortDay := first[visitorID]
		if cohortDay.Before(from.Truncate(24*time.Hour)) || cohortDay.After(to) {
			continue
		}
		key := cohortDay.Format("2006-01-02")
		if cohorts[key] == nil {
			cohorts[key] = &retentionData{}
		}
		data := cohorts[key]
		data.Size++
		unique := map[int]bool{}
		for _, day := range days {
			delta := int(day.Sub(cohortDay).Hours() / 24)
			unique[delta] = true
		}
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

func retentionRate(hit, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

func (a *App) handleFunnelReport(w http.ResponseWriter, r *http.Request) {
	user, websiteID, from, to, ok := a.analyticsContext(w, r)
	if !ok || !a.requireWebsiteView(w, user, websiteID) {
		return
	}
	funnelID := strings.TrimSpace(r.URL.Query().Get("funnel_id"))
	if funnelID == "" {
		errorResponse(w, http.StatusBadRequest, "funnel_id required")
		return
	}
	funnel, err := a.getFunnel(websiteID, funnelID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "funnel not found")
		return
	}
	report, err := a.runFunnel(funnel, from, to)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "funnel": funnel, "report": report})
}

func (a *App) listFunnels(websiteID string) ([]Funnel, error) {
	rows, err := a.db.Query(`
		select id, website_id, name, steps_json, created_at
		from funnels
		where website_id = ?
		order by created_at asc
	`, websiteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Funnel
	for rows.Next() {
		var item Funnel
		var stepsJSON string
		if err := rows.Scan(&item.ID, &item.WebsiteID, &item.Name, &stepsJSON, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(stepsJSON), &item.Steps)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) getFunnel(websiteID, funnelID string) (Funnel, error) {
	var item Funnel
	var stepsJSON string
	err := a.db.QueryRow(`
		select id, website_id, name, steps_json, created_at
		from funnels
		where website_id = ? and id = ?
	`, websiteID, funnelID).Scan(&item.ID, &item.WebsiteID, &item.Name, &stepsJSON, &item.CreatedAt)
	if err != nil {
		return Funnel{}, err
	}
	_ = json.Unmarshal([]byte(stepsJSON), &item.Steps)
	return item, nil
}

func (a *App) runFunnel(funnel Funnel, from, to time.Time) (map[string]any, error) {
	rows, err := a.db.Query(`
		select session_id, event_type, event_name, url_path, created_at
		from events
		where website_id = ? and created_at between ? and ?
		order by session_id asc, created_at asc
	`, funnel.WebsiteID, iso(from), iso(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type event struct {
		Type string
		Name string
		Path string
	}
	eventsBySession := map[string][]event{}
	for rows.Next() {
		var sessionID, eventType, eventName, urlPath, createdText string
		if err := rows.Scan(&sessionID, &eventType, &eventName, &urlPath, &createdText); err != nil {
			return nil, err
		}
		eventsBySession[sessionID] = append(eventsBySession[sessionID], event{
			Type: eventType,
			Name: eventName,
			Path: urlPath,
		})
	}

	counts := make([]int, len(funnel.Steps))
	for _, events := range eventsBySession {
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

	var steps []map[string]any
	firstCount := 0
	if len(counts) > 0 {
		firstCount = counts[0]
	}
	for i, step := range funnel.Steps {
		conversion := 0.0
		if firstCount > 0 {
			conversion = float64(counts[i]) / float64(firstCount)
		}
		steps = append(steps, map[string]any{
			"index":      i + 1,
			"label":      step.Label,
			"type":       step.Type,
			"value":      step.Value,
			"sessions":   counts[i],
			"conversion": conversion,
		})
	}
	return map[string]any{
		"steps": steps,
	}, nil
}

func matchesStep(step FunnelStep, item struct {
	Type string
	Name string
	Path string
}) bool {
	switch step.Type {
	case "page":
		return item.Path == step.Value
	case "event":
		return item.Name == step.Value
	default:
		return false
	}
}

func (a *App) handlePublicShare(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/public/shares/")
	var share Share
	var enabled int
	err := a.db.QueryRow(`
		select id, website_id, slug, enabled, created_at
		from shares
		where slug = ?
	`, slug).Scan(&share.ID, &share.WebsiteID, &share.Slug, &enabled, &share.CreatedAt)
	if err != nil || enabled != 1 {
		http.NotFound(w, r)
		return
	}
	share.Enabled = true
	from, to, err := a.parseDateRange(r)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	var website Website
	if err := a.db.QueryRow(`select id, name, domain, created_at, updated_at from websites where id = ?`, share.WebsiteID).
		Scan(&website.ID, &website.Name, &website.Domain, &website.CreatedAt, &website.UpdatedAt); err != nil {
		http.NotFound(w, r)
		return
	}
	overview := a.publicOverview(share.WebsiteID, from, to)
	pages := a.queryGroupedItems(`
		select url_path as label, count(*) as count
		from events
		where website_id = ? and event_type = 'pageview' and created_at between ? and ?
		group by url_path
		order by count desc, label asc
		limit 20
	`, share.WebsiteID, from, to)
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

func (a *App) publicOverview(websiteID string, from, to time.Time) map[string]any {
	type overview struct {
		Pageviews int64   `json:"pageviews"`
		Visitors  int64   `json:"visitors"`
		Sessions  int64   `json:"sessions"`
		Events    int64   `json:"events"`
		Revenue   float64 `json:"revenue"`
	}
	var out overview
	_ = a.db.QueryRow(`
		select
			sum(case when event_type = 'pageview' then 1 else 0 end) as pageviews,
			count(distinct visitor_id) as visitors,
			count(distinct session_id) as sessions,
			sum(case when event_type <> 'pageview' then 1 else 0 end) as events,
			sum(amount) as revenue
		from events
		where website_id = ? and created_at between ? and ?
	`, websiteID, iso(from), iso(to)).Scan(&out.Pageviews, &out.Visitors, &out.Sessions, &out.Events, &out.Revenue)
	return map[string]any{
		"pageviews": out.Pageviews,
		"visitors":  out.Visitors,
		"sessions":  out.Sessions,
		"events":    out.Events,
		"revenue":   out.Revenue,
	}
}

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

func (a *App) queryGroupedItems(query, websiteID string, from, to time.Time) []map[string]any {
	rows, err := a.db.Query(query, websiteID, iso(from), iso(to))
	if err != nil {
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

func (a *App) queryRevenueItems(websiteID string, from, to time.Time) []map[string]any {
	rows, err := a.db.Query(`
		select case when currency = '' then 'N/A' else currency end as currency, sum(amount) as revenue
		from events
		where website_id = ? and created_at between ? and ? and amount > 0
		group by currency
		order by revenue desc, currency asc
	`, websiteID, iso(from), iso(to))
	if err != nil {
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

func (a *App) analyticsContext(w http.ResponseWriter, r *http.Request) (*AuthUser, string, time.Time, time.Time, bool) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return nil, "", time.Time{}, time.Time{}, false
	}
	websiteID := strings.TrimSpace(r.URL.Query().Get("website_id"))
	if websiteID == "" {
		errorResponse(w, http.StatusBadRequest, "website_id required")
		return nil, "", time.Time{}, time.Time{}, false
	}
	from, to, err := a.parseDateRange(r)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return nil, "", time.Time{}, time.Time{}, false
	}
	return user, websiteID, from, to, true
}
