package main

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/xiaoqidun/qqwry"
)

const (
	roleSuperAdmin = "super_admin"
	roleAdmin      = "admin"
	roleAnalyst    = "analyst"
	roleViewer     = "viewer"

	sessionCookieName = "sitlys_session"
	version           = "0.1.0"
	maxRateBuckets    = 4096
)

type Config struct {
	Addr        string
	DataDir     string
	DBPath      string
	SessionDays int
	GeoIPDBPath string
}

type App struct {
	cfg          Config
	db           *sql.DB
	server       *http.Server
	staticFS     fs.FS
	staticHTTP   http.Handler
	eventQueue   chan queuedEvent
	eventWriteMu sync.Mutex
	geoIPMu      sync.RWMutex
	geoIPDB      *geoip2.Reader
	qqwryMu      sync.RWMutex
	qqwryDB      *qqwry.Client
	workerCtx    context.Context
	workerStop   context.CancelFunc
	workerWG     sync.WaitGroup
	rateMu       sync.Mutex
	rateBuckets  map[string]rateBucket
	botModeMu    sync.RWMutex
	botMode      string
	botModeAt    time.Time
	botAuditMu   sync.Mutex
	botAudit     map[string]int
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
	GeoIPDatabasePath string `json:"geoip_database_path"`
	LogLevel          string `json:"log_level"`
	DataRetentionDays int    `json:"data_retention_days"`
	BotFilterMode     string `json:"bot_filter_mode"`
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
	SessionKey     string
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
	PrevLastSeenAt time.Time
	PrevPageviews  int
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
