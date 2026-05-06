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

func New(cfg Config) (*App, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(cfg.DBPath)
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "sitlys.db")
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(cfg.DBPath)
	}
	if cfg.SessionDays <= 0 {
		cfg.SessionDays = 30
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	if err := initDBPragmas(db, cfg.DBPath); err != nil {
		return nil, fmt.Errorf("init pragmas: %w", err)
	}

	app := &App{
		cfg:         cfg,
		db:          db,
		eventQueue:  make(chan queuedEvent, 8192),
		rateBuckets: make(map[string]rateBucket),
		botMode:     "balanced",
		botAudit:    make(map[string]int),
	}
	if err := app.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	if err := app.reloadGeoIPDB(); err != nil {
		db.Close()
		return nil, err
	}

	if err := app.reloadQQWryDB(); err != nil {
		db.Close()
		return nil, err
	}

	sub, err := fs.Sub(staticFiles, "embed")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("static fs: %w", err)
	}
	app.staticFS = sub
	app.staticHTTP = http.FileServer(http.FS(sub))
	app.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.workerCtx, app.workerStop = context.WithCancel(context.Background())
	app.workerWG.Add(1)
	go app.runEventWriter()

	return app, nil
}

func (a *App) Close() error {
	if a.workerStop != nil {
		a.workerStop()
	}
	a.workerWG.Wait()
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			return err
		}
	}
	a.closeGeoIPDB()
	return nil
}

func initDBPragmas(db *sql.DB, dbPath string) error {
	if _, err := db.Exec(`pragma foreign_keys = on;`); err != nil {
		return err
	}
	if _, err := db.Exec(`pragma busy_timeout = 5000;`); err != nil {
		return err
	}

	var mode string
	if err := db.QueryRow(`pragma journal_mode = wal;`).Scan(&mode); err == nil {
		return nil
	}

	log.Printf("sqlite WAL unavailable for %s, falling back to DELETE journal mode", dbPath)
	if _, fallbackErr := db.Exec(`pragma journal_mode = delete;`); fallbackErr != nil {
		log.Printf("sqlite journal mode fallback failed for %s, continuing with driver default: %v", dbPath, fallbackErr)
		return nil
	}
	return nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("sitlys %s listening on http://%s", version, a.cfg.Addr)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

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

func (a *App) initSchema() error {
	schema := []string{
		`create table if not exists users (
			id text primary key,
			username text not null unique,
			password_hash text not null,
			role text not null,
			enabled integer not null default 1,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists websites (
			id text primary key,
			name text not null,
			domain text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists website_permissions (
			user_id text not null,
			website_id text not null,
			access_level text not null default 'view',
			created_at text not null,
			primary key (user_id, website_id),
			foreign key (user_id) references users(id) on delete cascade,
			foreign key (website_id) references websites(id) on delete cascade
		);`,
		`create table if not exists pixels (
			id text primary key,
			website_id text not null,
			name text not null,
			slug text not null unique,
			enabled integer not null default 1,
			created_at text not null,
			foreign key (website_id) references websites(id) on delete cascade
		);`,
		`create table if not exists shares (
			id text primary key,
			website_id text not null,
			slug text not null unique,
			enabled integer not null default 1,
			created_at text not null,
			foreign key (website_id) references websites(id) on delete cascade
		);`,
		`create table if not exists auth_sessions (
			id text primary key,
			user_id text not null,
			token_hash text not null unique,
			expires_at text not null,
			created_at text not null,
			foreign key (user_id) references users(id) on delete cascade
		);`,
		`create table if not exists visitors (
			id text primary key,
			website_id text not null,
			external_id text not null,
			first_seen_at text not null,
			last_seen_at text not null,
			unique (website_id, external_id),
			foreign key (website_id) references websites(id) on delete cascade
		);`,
		`create table if not exists sessions (
			id text primary key,
			session_key text not null default '',
			website_id text not null,
			visitor_id text not null,
			started_at text not null,
			last_seen_at text not null,
			event_count integer not null default 0,
			pageviews integer not null default 0,
			referrer text not null default '',
			referrer_domain text not null default '',
			utm_source text not null default '',
			utm_medium text not null default '',
			utm_campaign text not null default '',
			browser text not null default '',
			os text not null default '',
			device text not null default '',
			country text not null default '',
			region text not null default '',
			city text not null default '',
			entry_path text not null default '',
			exit_path text not null default '',
			unique (session_key),
			foreign key (website_id) references websites(id) on delete cascade,
			foreign key (visitor_id) references visitors(id) on delete cascade
		);`,
		`create table if not exists events (
			id text primary key,
			website_id text not null,
			session_id text not null,
			visitor_id text not null,
			pixel_id text,
			event_type text not null,
			event_name text not null default '',
			page_title text not null default '',
			hostname text not null default '',
			url text not null default '',
			url_path text not null default '',
			referrer text not null default '',
			referrer_domain text not null default '',
			utm_source text not null default '',
			utm_medium text not null default '',
			utm_campaign text not null default '',
			utm_content text not null default '',
			utm_term text not null default '',
			browser text not null default '',
			os text not null default '',
			device text not null default '',
			country text not null default '',
			region text not null default '',
			city text not null default '',
			amount real not null default 0,
			currency text not null default '',
			metadata text not null default '{}',
			created_at text not null,
			foreign key (website_id) references websites(id) on delete cascade,
			foreign key (session_id) references sessions(id) on delete cascade,
			foreign key (visitor_id) references visitors(id) on delete cascade,
			foreign key (pixel_id) references pixels(id) on delete set null
		);`,
		`create table if not exists funnels (
			id text primary key,
			website_id text not null,
			name text not null,
			steps_json text not null,
			created_at text not null,
			foreign key (website_id) references websites(id) on delete cascade
		);`,
		`create table if not exists system_settings (
			key text primary key,
			value text not null,
			updated_at text not null
		);`,
		`create table if not exists agg_overview_daily (
			website_id text not null,
			bucket_date text not null,
			pageviews integer not null default 0,
			custom_events integer not null default 0,
			visitors integer not null default 0,
			sessions integer not null default 0,
			bounced_sessions integer not null default 0,
			session_duration_total_seconds integer not null default 0,
			time_on_page_total_ms integer not null default 0,
			time_on_page_samples integer not null default 0,
			revenue real not null default 0,
			primary key (website_id, bucket_date)
		);`,
		`create table if not exists agg_visitor_daily (
			website_id text not null,
			bucket_date text not null,
			visitor_id text not null,
			primary key (website_id, bucket_date, visitor_id)
		);`,
		`create table if not exists agg_pages_daily (
			website_id text not null,
			bucket_date text not null,
			url_path text not null,
			pageviews integer not null default 0,
			primary key (website_id, bucket_date, url_path)
		);`,
		`create table if not exists agg_referrers_daily (
			website_id text not null,
			bucket_date text not null,
			referrer_domain text not null,
			sessions integer not null default 0,
			revenue real not null default 0,
			primary key (website_id, bucket_date, referrer_domain)
		);`,
		`create table if not exists agg_devices_daily (
			website_id text not null,
			bucket_date text not null,
			browser text not null default '',
			os text not null default '',
			device text not null default '',
			sessions integer not null default 0,
			primary key (website_id, bucket_date, browser, os, device)
		);`,
		`create table if not exists agg_geo_daily (
			website_id text not null,
			bucket_date text not null,
			country text not null default '',
			region text not null default '',
			city text not null default '',
			sessions integer not null default 0,
			primary key (website_id, bucket_date, country, region, city)
		);`,
		`create table if not exists agg_attribution_daily (
			website_id text not null,
			bucket_date text not null,
			source text not null,
			medium text not null,
			campaign text not null,
			sessions integer not null default 0,
			revenue real not null default 0,
			primary key (website_id, bucket_date, source, medium, campaign)
		);`,
		`create table if not exists agg_revenue_daily (
			website_id text not null,
			bucket_date text not null,
			source text not null,
			currency text not null,
			event_count integer not null default 0,
			revenue real not null default 0,
			primary key (website_id, bucket_date, source, currency)
		);`,
		`create index if not exists idx_sessions_website_started on sessions(website_id, started_at);`,
		`create index if not exists idx_sessions_website_visitor on sessions(website_id, visitor_id, last_seen_at);`,
		`create index if not exists idx_events_website_created on events(website_id, created_at);`,
		`create index if not exists idx_events_website_type_created on events(website_id, event_type, created_at);`,
		`create index if not exists idx_events_website_path_created on events(website_id, url_path, created_at);`,
		`create index if not exists idx_events_website_name_created on events(website_id, event_name, created_at);`,
	}
	for _, stmt := range schema {
		if _, err := a.db.Exec(stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}

	if !a.tableColumnExists("website_permissions", "access_level") {
		if _, err := a.db.Exec(`alter table website_permissions add column access_level text not null default 'view'`); err != nil {
			return fmt.Errorf("migrate website_permissions access_level: %w", err)
		}
	}
	if a.tableColumnExists("website_permissions", "can_manage") {
		if _, err := a.db.Exec(`
			update website_permissions
			set access_level = case
				when access_level is null or access_level = '' then
					case when can_manage = 1 then 'manage' else 'view' end
				else access_level
			end
		`); err != nil {
			return fmt.Errorf("backfill website_permissions access_level: %w", err)
		}
	}
	for _, column := range []string{"visitors", "sessions", "bounced_sessions", "session_duration_total_seconds", "time_on_page_total_ms", "time_on_page_samples"} {
		if !a.tableColumnExists("agg_overview_daily", column) {
			if _, err := a.db.Exec(`alter table agg_overview_daily add column ` + column + ` integer not null default 0`); err != nil {
				return fmt.Errorf("migrate agg_overview_daily %s: %w", column, err)
			}
		}
	}
	if !a.tableColumnExists("sessions", "session_key") {
		if _, err := a.db.Exec(`alter table sessions add column session_key text not null default ''`); err != nil {
			return fmt.Errorf("migrate sessions session_key: %w", err)
		}
	}
	if _, err := a.db.Exec(`create unique index if not exists idx_sessions_session_key on sessions(session_key) where session_key <> ''`); err != nil {
		return fmt.Errorf("create sessions session_key index: %w", err)
	}
	if _, err := a.db.Exec(`
		update sessions
		set session_key = website_id || ':' || visitor_id || ':' || strftime('%s', started_at)
		where session_key = ''
	`); err != nil {
		return fmt.Errorf("backfill sessions session_key: %w", err)
	}
	if _, err := a.db.Exec(`
		insert into agg_overview_daily(website_id, bucket_date, visitors, sessions, bounced_sessions, session_duration_total_seconds)
		select
			website_id,
			date(started_at),
			count(distinct visitor_id),
			count(*),
			sum(case when pageviews = 1 then 1 else 0 end),
			sum(case when unixepoch(last_seen_at) - unixepoch(started_at) > 0 then unixepoch(last_seen_at) - unixepoch(started_at) else 0 end)
		from sessions
		group by website_id, date(started_at)
		on conflict(website_id, bucket_date) do update set
			visitors = excluded.visitors,
			sessions = excluded.sessions,
			bounced_sessions = excluded.bounced_sessions,
			session_duration_total_seconds = excluded.session_duration_total_seconds
	`); err != nil {
		return fmt.Errorf("backfill agg_overview_daily session metrics: %w", err)
	}
	if _, err := a.db.Exec(`
		insert into agg_overview_daily(website_id, bucket_date, time_on_page_total_ms, time_on_page_samples)
		select
			website_id,
			date(created_at),
			coalesce(sum(cast(json_extract(metadata, '$.duration_ms') as integer)), 0),
			count(*)
		from events
		where event_name in ('page_leave', 'page_ping')
		group by website_id, date(created_at)
		on conflict(website_id, bucket_date) do update set
			time_on_page_total_ms = excluded.time_on_page_total_ms,
			time_on_page_samples = excluded.time_on_page_samples
	`); err != nil {
		return fmt.Errorf("backfill agg_overview_daily time on page: %w", err)
	}
	if _, err := a.db.Exec(`
		insert or ignore into agg_visitor_daily(website_id, bucket_date, visitor_id)
		select website_id, date(started_at), visitor_id
		from sessions
	`); err != nil {
		return fmt.Errorf("backfill agg_visitor_daily: %w", err)
	}
	return nil
}

func (a *App) tableColumnExists(tableName, columnName string) bool {
	rows, err := a.db.Query(`pragma table_info(` + tableName + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false
		}
		if strings.EqualFold(name, columnName) {
			return true
		}
	}
	return false
}

func (a *App) hasUsers() (bool, error) {
	var count int
	if err := a.db.QueryRow(`select count(*) from users`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

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

func (a *App) createSession(userID string) (string, time.Time, error) {
	if err := a.cleanupExpiredSessions(); err != nil {
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

func (a *App) cleanupExpiredSessions() error {
	_, err := a.db.Exec(`delete from auth_sessions where expires_at < ?`, iso(nowUTC()))
	return err
}

func (a *App) currentUser(r *http.Request) (*AuthUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}

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
	user.AllWebsites = user.Role == roleSuperAdmin
	perms, err := a.permissionsForUser(user.ID)
	if err != nil {
		return nil, err
	}
	user.Permissions = perms
	return user, nil
}

func (a *App) requireUser(w http.ResponseWriter, r *http.Request) (*AuthUser, bool) {
	user, err := a.currentUser(r)
	if err != nil || !user.Enabled {
		errorResponse(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return user, true
}

func (a *App) permissionsForUser(userID string) ([]WebsitePermission, error) {
	rows, err := a.db.Query(`
		select website_id, access_level
		from website_permissions
		where user_id = ?
		order by website_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WebsitePermission
	for rows.Next() {
		var perm WebsitePermission
		if err := rows.Scan(&perm.WebsiteID, &perm.AccessLevel); err != nil {
			return nil, err
		}
		if perm.AccessLevel == "" {
			perm.AccessLevel = "view"
		}
		out = append(out, perm)
	}
	return out, rows.Err()
}

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

func (a *App) requireWebsiteView(w http.ResponseWriter, user *AuthUser, websiteID string) bool {
	if !a.canViewWebsite(user, websiteID) {
		errorResponse(w, http.StatusForbidden, "no access to website")
		return false
	}
	return true
}

func (a *App) requireWebsiteManage(w http.ResponseWriter, user *AuthUser, websiteID string) bool {
	if !a.canManageWebsite(user, websiteID) {
		errorResponse(w, http.StatusForbidden, "manage permission required")
		return false
	}
	return true
}

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
		if len(to) == len("2006-01-02") {
			end = end.Add(24*time.Hour - time.Second)
		}
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid date range")
	}
	return start.UTC(), end.UTC(), nil
}

func (a *App) allowCollectionRequest(r *http.Request, websiteID string, cost int) bool {
	ip := clientIP(r)
	userAgent := strings.TrimSpace(r.UserAgent())
	websiteKey := "collection:website:" + websiteID
	ipKey := "collection:ip:" + ip
	clientKey := "collection:client:" + websiteID + ":" + ip + ":" + tokenHash(userAgent)[:16]
	if !a.allowRateLimit(websiteKey, 1200, cost, time.Minute) {
		return false
	}
	if !a.allowRateLimit(ipKey, 240, cost, time.Minute) {
		return false
	}
	if !a.allowRateLimit(clientKey, 120, cost, time.Minute) {
		return false
	}
	return true
}

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
		return settings, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value, updatedAt string
		if err := rows.Scan(&key, &value, &updatedAt); err != nil {
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

func (a *App) setSystemSettings(values map[string]string) error {
	now := iso(nowUTC())
	nextGeoIPPath, hasGeoIPPath := values["geoip_database_path"]
	nextBotMode, hasBotMode := values["bot_filter_mode"]
	tx, err := a.db.Begin()
	if err != nil {
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
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if hasGeoIPPath {
		a.cfg.GeoIPDBPath = strings.TrimSpace(nextGeoIPPath)
		if err := a.reloadGeoIPDB(); err != nil {
			return err
		}
	}
	if hasBotMode {
		a.updateBotFilterModeCache(nextBotMode)
	}
	return nil
}

func (a *App) createBackup() (string, error) {
	backupDir := filepath.Join(a.cfg.DataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("sitlys-%s.db", nowUTC().Format("20060102-150405"))
	targetPath := filepath.Join(backupDir, filename)
	cleanBackupDir := filepath.Clean(backupDir)
	cleanTargetPath := filepath.Clean(targetPath)
	if filepath.Dir(cleanTargetPath) != cleanBackupDir {
		return "", fmt.Errorf("invalid backup target path")
	}
	if strings.ContainsAny(cleanTargetPath, "'\x00\r\n") {
		return "", fmt.Errorf("invalid backup target path")
	}
	if _, err := a.db.Exec(`pragma wal_checkpoint(full);`); err != nil {
		return "", err
	}
	if _, err := a.db.Exec(`vacuum into ` + sqliteStringLiteral(targetPath)); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (a *App) shouldIgnoreBotTraffic(r *http.Request) (bool, string) {
	mode := a.botFilterModeValue()
	if mode == "off" {
		return false, ""
	}
	if isBotRequest(r) {
		return true, "prefetch"
	}
	if isBotTraffic(r.UserAgent()) {
		return true, "bot"
	}
	if mode == "strict" && isPreviewBotTraffic(r.UserAgent()) {
		return true, "preview_bot"
	}
	return false, ""
}

func (a *App) updateBotFilterModeCache(mode string) {
	if a == nil {
		return
	}
	a.botModeMu.Lock()
	defer a.botModeMu.Unlock()
	a.botMode = normalizeBotFilterMode(mode)
	a.botModeAt = nowUTC()
}

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
	if !loadedAt.IsZero() && time.Since(loadedAt) < 5*time.Second {
		return mode
	}
	if a.db == nil {
		return mode
	}
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

func (a *App) recordBotAudit(reason string) {
	if reason == "" {
		return
	}
	a.botAuditMu.Lock()
	defer a.botAuditMu.Unlock()
	a.botAudit[reason]++
}

func (a *App) botAuditSnapshot() map[string]int {
	a.botAuditMu.Lock()
	defer a.botAuditMu.Unlock()
	out := make(map[string]int, len(a.botAudit))
	for key, value := range a.botAudit {
		out[key] = value
	}
	return out
}

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

	for itemKey, bucket := range a.rateBuckets {
		if now.After(bucket.ResetAt) {
			delete(a.rateBuckets, itemKey)
		}
	}
	bucket, ok := a.rateBuckets[key]
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
	if !ok || now.After(bucket.ResetAt) {
		bucket = rateBucket{ResetAt: now.Add(window)}
	}
	if bucket.Count+cost > limit {
		a.rateBuckets[key] = bucket
		return false
	}
	bucket.Count += cost
	a.rateBuckets[key] = bucket
	return true
}

func (a *App) cleanupHistoricalData(retentionDays int) (map[string]any, error) {
	if retentionDays <= 0 {
		retentionDays = 365
	}
	cutoff := nowUTC().AddDate(0, 0, -retentionDays)
	cutoffDate := cutoff.Format("2006-01-02")
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result := map[string]any{
		"retention_days": retentionDays,
		"cutoff_at":      iso(cutoff),
	}

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

	if err := collectDelete("deleted_events", `delete from events where created_at < ?`, iso(cutoff)); err != nil {
		return nil, err
	}
	if err := collectDelete("deleted_sessions", `delete from sessions where last_seen_at < ?`, iso(cutoff)); err != nil {
		return nil, err
	}
	if err := collectDelete("deleted_visitors", `delete from visitors where last_seen_at < ? and not exists (select 1 from sessions where sessions.visitor_id = visitors.id)`, iso(cutoff)); err != nil {
		return nil, err
	}

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
			return nil, err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		aggregateRows += count
	}
	result["deleted_aggregate_rows"] = aggregateRows

	cleanupAt := iso(nowUTC())
	if _, err := tx.Exec(`
		insert into system_settings(key, value, updated_at)
		values('last_cleanup_at', ?, ?)
		on conflict(key) do update set
			value = excluded.value,
			updated_at = excluded.updated_at
	`, cleanupAt, cleanupAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result["last_cleanup_at"] = cleanupAt
	return result, nil
}
