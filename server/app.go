package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const (
	roleSuperAdmin = "super_admin"
	roleAdmin      = "admin"
	roleAnalyst    = "analyst"
	roleViewer     = "viewer"

	sessionCookieName = "sitlys_session"
	version           = "0.1.0"
)

//go:embed embed/* embed/**/*
var staticFiles embed.FS

//go:embed tracker.js
var trackerScript string

type Config struct {
	Addr        string
	DataDir     string
	DBPath      string
	SessionDays int
}

type App struct {
	cfg         Config
	db          *sql.DB
	server      *http.Server
	staticFS    fs.FS
	staticHTTP  http.Handler
	eventQueue  chan queuedEvent
	workerCtx   context.Context
	workerStop  context.CancelFunc
	workerWG    sync.WaitGroup
	rateMu      sync.Mutex
	rateBuckets map[string]rateBucket
}

type AuthUser struct {
	ID          string              `json:"id"`
	Username    string              `json:"username"`
	Role        string              `json:"role"`
	Enabled     bool                `json:"enabled"`
	CreatedAt   string              `json:"created_at"`
	Permissions []WebsitePermission `json:"permissions,omitempty"`
	AllWebsites bool                `json:"all_websites"`
}

type WebsitePermission struct {
	WebsiteID   string `json:"website_id"`
	AccessLevel string `json:"access_level"`
}

type Website struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Pixel struct {
	ID        string `json:"id"`
	WebsiteID string `json:"website_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

type Share struct {
	ID        string `json:"id"`
	WebsiteID string `json:"website_id"`
	Slug      string `json:"slug"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

type FunnelStep struct {
	Label string `json:"label"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Funnel struct {
	ID        string       `json:"id"`
	WebsiteID string       `json:"website_id"`
	Name      string       `json:"name"`
	Steps     []FunnelStep `json:"steps"`
	CreatedAt string       `json:"created_at"`
}

type SystemSettings struct {
	ListenAddr        string `json:"listen_addr"`
	DatabasePath      string `json:"database_path"`
	LogLevel          string `json:"log_level"`
	DataRetentionDays int    `json:"data_retention_days"`
	LastCleanupAt     string `json:"last_cleanup_at"`
	UpdatedAt         string `json:"updated_at"`
}

type rateBucket struct {
	Count   int
	ResetAt time.Time
}

type eventRequest struct {
	Type    string       `json:"type"`
	Payload eventPayload `json:"payload"`
}

type eventPayload struct {
	Website   string         `json:"website,omitempty"`
	Pixel     string         `json:"pixel,omitempty"`
	URL       string         `json:"url,omitempty"`
	Referrer  string         `json:"referrer,omitempty"`
	Name      string         `json:"name,omitempty"`
	Title     string         `json:"title,omitempty"`
	Hostname  string         `json:"hostname,omitempty"`
	Language  string         `json:"language,omitempty"`
	Screen    string         `json:"screen,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	ID        string         `json:"id,omitempty"`
	Browser   string         `json:"browser,omitempty"`
	OS        string         `json:"os,omitempty"`
	Device    string         `json:"device,omitempty"`
	Country   string         `json:"country,omitempty"`
	Region    string         `json:"region,omitempty"`
	City      string         `json:"city,omitempty"`
	UTMSource string         `json:"utm_source,omitempty"`
	UTMMedium string         `json:"utm_medium,omitempty"`
	UTMCamp   string         `json:"utm_campaign,omitempty"`
	UTMCont   string         `json:"utm_content,omitempty"`
	UTMTerm   string         `json:"utm_term,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Revenue   *RevenueInput  `json:"revenue,omitempty"`
}

type RevenueInput struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type eventRecord struct {
	WebsiteID      string
	PixelID        string
	VisitorID      string
	SessionID      string
	EventType      string
	EventName      string
	PageTitle      string
	Hostname       string
	URL            string
	URLPath        string
	Referrer       string
	ReferrerDomain string
	UTMSource      string
	UTMMedium      string
	UTMCampaign    string
	UTMContent     string
	UTMTerm        string
	Browser        string
	OS             string
	Device         string
	Country        string
	Region         string
	City           string
	Amount         float64
	Currency       string
	Metadata       string
	CreatedAt      time.Time
}

type sessionRecord struct {
	ID             string
	WebsiteID      string
	VisitorID      string
	StartedAt      time.Time
	LastSeenAt     time.Time
	EventCount     int
	Pageviews      int
	Referrer       string
	ReferrerDomain string
	UTMSource      string
	UTMMedium      string
	UTMCampaign    string
	Browser        string
	OS             string
	Device         string
	Country        string
	Region         string
	City           string
	EntryPath      string
	ExitPath       string
}

type queuedEvent struct {
	WebsiteID      string
	PixelID        string
	VisitorKey     string
	EventType      string
	EventName      string
	PageTitle      string
	Hostname       string
	URL            string
	URLPath        string
	Referrer       string
	ReferrerDomain string
	UTMSource      string
	UTMMedium      string
	UTMCampaign    string
	UTMContent     string
	UTMTerm        string
	Browser        string
	OS             string
	Device         string
	Country        string
	Region         string
	City           string
	Amount         float64
	Currency       string
	Metadata       string
	CreatedAt      time.Time
}

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
	}
	if err := app.initSchema(); err != nil {
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
		return a.db.Close()
	}
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
			revenue real not null default 0,
			primary key (website_id, bucket_date)
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

	if _, err := a.db.Exec(`alter table website_permissions add column access_level text not null default 'view'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("migrate website_permissions access_level: %w", err)
	}
	if _, err := a.db.Exec(`
		update website_permissions
		set access_level = case
			when access_level is null or access_level = '' then
				case when can_manage = 1 then 'manage' else 'view' end
			else access_level
		end
	`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such column: can_manage") {
		return fmt.Errorf("backfill website_permissions access_level: %w", err)
	}
	return nil
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

func iso(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseISO(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func shortID() string {
	return newID()[:12]
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func passwordHash(password string) (string, error) {
	sum, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(sum), nil
}

func passwordMatch(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func errorResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]any{
		"ok":    false,
		"error": message,
	})
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func (a *App) hasUsers() (bool, error) {
	var count int
	if err := a.db.QueryRow(`select count(*) from users`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	secure := false
	if r != nil {
		secure = r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expires,
	})
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
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

func parseDateInput(value string) (time.Time, error) {
	if len(value) == len("2006-01-02") {
		return time.ParseInLocation("2006-01-02", value, time.UTC)
	}
	return time.Parse(time.RFC3339, value)
}

func cleanURL(raw string) (fullURL, host, path string) {
	if raw == "" {
		return "", "", ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, "", ""
	}
	host = strings.ToLower(parsed.Hostname())
	path = parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return raw, host, path
}

func referrerDomain(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func normalizeEventType(payload eventPayload, pixelID string) string {
	if payload.Revenue != nil && payload.Revenue.Amount > 0 {
		return "revenue"
	}
	if payload.Name != "" {
		return "event"
	}
	if pixelID != "" {
		return "pixel"
	}
	return "pageview"
}

func detectUserAgent(r *http.Request, payload eventPayload) (browser, osName, device string) {
	if payload.Browser != "" || payload.OS != "" || payload.Device != "" {
		return payload.Browser, payload.OS, payload.Device
	}
	ua := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(ua, "edg"):
		browser = "Edge"
	case strings.Contains(ua, "chrome"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "safari"):
		browser = "Safari"
	default:
		browser = "Unknown"
	}
	switch {
	case strings.Contains(ua, "windows"):
		osName = "Windows"
	case strings.Contains(ua, "android"):
		osName = "Android"
	case strings.Contains(ua, "mac os"):
		osName = "macOS"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
		osName = "iOS"
	case strings.Contains(ua, "linux"):
		osName = "Linux"
	default:
		osName = "Unknown"
	}
	switch {
	case strings.Contains(ua, "ipad"), strings.Contains(ua, "tablet"):
		device = "tablet"
	case strings.Contains(ua, "android") && !strings.Contains(ua, "mobile"):
		device = "tablet"
	case strings.Contains(ua, "mobile"), strings.Contains(ua, "iphone"), strings.Contains(ua, "android"):
		device = "mobile"
	default:
		device = "desktop"
	}
	return
}

func isBotTraffic(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
	}
	for _, token := range []string{
		"bot",
		"spider",
		"crawler",
		"headless",
		"preview",
		"slurp",
		"bingpreview",
		"facebookexternalhit",
		"curl",
		"wget",
		"httpclient",
		"ahrefs",
		"semrush",
		"bytespider",
		"applebot",
		"discordbot",
		"telegrambot",
		"whatsapp",
		"slackbot",
		"python-requests",
		"go-http-client",
	} {
		if strings.Contains(ua, token) {
			return true
		}
	}
	return false
}

func isBotRequest(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Purpose")), "prefetch") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Purpose")), "prefetch") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Moz")), "prefetch") {
		return true
	}
	return false
}

func detectGeo(r *http.Request, payload eventPayload) (country, region, city string) {
	country = strings.TrimSpace(firstNonEmpty(
		payload.Country,
		r.Header.Get("CF-IPCountry"),
		r.Header.Get("X-Appengine-Country"),
		r.Header.Get("X-Country-Code"),
	))
	region = strings.TrimSpace(firstNonEmpty(
		payload.Region,
		r.Header.Get("X-Appengine-Region"),
		r.Header.Get("CF-Region-Code"),
		r.Header.Get("X-Region-Code"),
	))
	city = strings.TrimSpace(firstNonEmpty(
		payload.City,
		r.Header.Get("X-Appengine-City"),
		r.Header.Get("CF-IPCity"),
		r.Header.Get("X-City"),
	))
	return
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

func isValidRole(role string) bool {
	switch role {
	case roleSuperAdmin, roleAdmin, roleAnalyst, roleViewer:
		return true
	default:
		return false
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (a *App) getSystemSettings() (SystemSettings, error) {
	settings := SystemSettings{
		ListenAddr:        a.cfg.Addr,
		DatabasePath:      a.cfg.DBPath,
		LogLevel:          "info",
		DataRetentionDays: 365,
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
		case "log_level":
			settings.LogLevel = value
		case "data_retention_days":
			if days, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && days > 0 {
				settings.DataRetentionDays = days
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
	return tx.Commit()
}

func (a *App) createBackup() (string, error) {
	backupDir := filepath.Join(a.cfg.DataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("sitlys-%s.db", nowUTC().Format("20060102-150405"))
	targetPath := filepath.Join(backupDir, filename)
	escaped := strings.ReplaceAll(targetPath, "'", "''")

	if _, err := a.db.Exec(`pragma wal_checkpoint(full);`); err != nil {
		return "", err
	}
	if _, err := a.db.Exec(`vacuum into '` + escaped + `'`); err != nil {
		return "", err
	}
	return targetPath, nil
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
