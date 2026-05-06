// Package main - 类型定义模块
// 定义应用的核心数据结构、常量和配置类型
// 包含：
//   - 角色常量（超级管理员、管理员、分析师、查看者）
//   - 应用配置结构（Config）
//   - 应用主结构（App）
//   - 用户、网站、像素、分享等业务实体
//   - 事件相关的请求/记录/队列类型
//   - 会话记录类型
//   - 漏斗和系统设置类型
//   - 速率限制桶类型
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
	// 用户角色定义
	roleSuperAdmin = "super_admin" // 超级管理员：拥有所有权限
	roleAdmin      = "admin"       // 管理员：可管理网站和用户
	roleAnalyst    = "analyst"     // 分析师：可查看所有数据
	roleViewer     = "viewer"      // 查看者：只能查看被授权的网站

	sessionCookieName = "sitlys_session" // 会话 Cookie 名称
	version           = "0.1.0"          // 应用版本号
	maxRateBuckets    = 4096             // 最大速率限制桶数量
)

// Config - 应用配置
type Config struct {
	Addr        string // HTTP 监听地址，如 "127.0.0.1:8080"
	DataDir     string // 应用数据目录
	DBPath      string // SQLite 数据库文件路径
	SessionDays int    // 会话 Cookie 有效期（天）
	GeoIPDBPath string // GeoIP 数据库文件路径
}

// App - 应用主结构
// 包含所有运行时状态和依赖
type App struct {
	cfg          Config                // 应用配置
	db           *sql.DB               // SQLite 数据库连接
	server       *http.Server          // HTTP 服务器
	staticFS     fs.FS                 // 静态文件系统
	staticHTTP   http.Handler          // 静态文件 HTTP 处理器
	eventQueue   chan queuedEvent      // 事件队列（缓冲通道）
	eventWriteMu sync.Mutex            // 事件写入互斥锁
	geoIPMu      sync.RWMutex          // GeoIP 数据库读写锁
	geoIPDB      *geoip2.Reader        // MaxMind GeoIP2 数据库
	qqwryMu      sync.RWMutex          // 纯真 IP 数据库读写锁
	qqwryDB      *qqwry.Client         // 纯真 IP 数据库客户端
	workerCtx    context.Context       // 后台工作协程上下文
	workerStop   context.CancelFunc    // 取消后台工作协程
	workerWG     sync.WaitGroup        // 等待后台工作协程完成
	rateMu       sync.Mutex            // 速率限制互斥锁
	rateBuckets  map[string]rateBucket // 速率限制桶（按 IP/Token）
	botModeMu    sync.RWMutex          // 机器人过滤模式读写锁
	botMode      string                // 机器人过滤模式
	botModeAt    time.Time             // 机器人过滤模式设置时间
	botAuditMu   sync.Mutex            // 机器人审计互斥锁
	botAudit     map[string]int        // 机器人审计计数
}

// AuthUser - 认证用户信息
type AuthUser struct {
	ID          string              `json:"id"`                    // 用户 ID
	Username    string              `json:"username"`              // 用户名
	Role        string              `json:"role"`                  // 角色
	Enabled     bool                `json:"enabled"`               // 是否启用
	CreatedAt   string              `json:"created_at"`            // 创建时间
	Permissions []WebsitePermission `json:"permissions,omitempty"` // 网站权限列表
	AllWebsites bool                `json:"all_websites"`          // 是否拥有所有网站权限
}

// WebsitePermission - 网站权限
type WebsitePermission struct {
	WebsiteID   string `json:"website_id"`   // 网站 ID
	AccessLevel string `json:"access_level"` // 访问级别（view/manage）
}

// Website - 网站信息
type Website struct {
	ID        string `json:"id"`         // 网站 ID
	Name      string `json:"name"`       // 网站名称
	Domain    string `json:"domain"`     // 网站域名
	CreatedAt string `json:"created_at"` // 创建时间
	UpdatedAt string `json:"updated_at"` // 更新时间
}

// Pixel - 追踪像素配置
type Pixel struct {
	ID        string `json:"id"`         // 像素 ID
	WebsiteID string `json:"website_id"` // 所属网站 ID
	Name      string `json:"name"`       // 像素名称
	Slug      string `json:"slug"`       // 像素标识符（URL 友好）
	Enabled   bool   `json:"enabled"`    // 是否启用
	CreatedAt string `json:"created_at"` // 创建时间
}

// Share - 公开分享链接
type Share struct {
	ID        string `json:"id"`         // 分享 ID
	WebsiteID string `json:"website_id"` // 所属网站 ID
	Slug      string `json:"slug"`       // 分享标识符
	Enabled   bool   `json:"enabled"`    // 是否启用
	CreatedAt string `json:"created_at"` // 创建时间
}

// FunnelStep - 漏斗步骤定义
type FunnelStep struct {
	Label string `json:"label"` // 步骤标签
	Type  string `json:"type"`  // 匹配类型（path/event）
	Value string `json:"value"` // 匹配值
}

// Funnel - 转化漏斗
type Funnel struct {
	ID        string       `json:"id"`         // 漏斗 ID
	WebsiteID string       `json:"website_id"` // 所属网站 ID
	Name      string       `json:"name"`       // 漏斗名称
	Steps     []FunnelStep `json:"steps"`      // 漏斗步骤列表
	CreatedAt string       `json:"created_at"` // 创建时间
}

// SystemSettings - 系统设置
type SystemSettings struct {
	ListenAddr        string `json:"listen_addr"`         // 监听地址
	DatabasePath      string `json:"database_path"`       // 数据库路径
	GeoIPDatabasePath string `json:"geoip_database_path"` // GeoIP 数据库路径
	LogLevel          string `json:"log_level"`           // 日志级别
	DataRetentionDays int    `json:"data_retention_days"` // 数据保留天数
	BotFilterMode     string `json:"bot_filter_mode"`     // 机器人过滤模式
	LastCleanupAt     string `json:"last_cleanup_at"`     // 上次清理时间
	UpdatedAt         string `json:"updated_at"`          // 更新时间
}

// rateBucket - 速率限制桶
// 记录请求计数和重置时间
type rateBucket struct {
	Count   int       // 当前窗口内的请求计数
	ResetAt time.Time // 窗口重置时间
}

// eventRequest - 客户端事件请求
type eventRequest struct {
	Type    string       `json:"type"`    // 事件类型（pageview/custom_event）
	Payload eventPayload `json:"payload"` // 事件负载
}

// eventPayload - 事件负载数据
// 包含客户端上报的所有事件信息
type eventPayload struct {
	Website   string         `json:"website,omitempty"`      // 网站域名
	Pixel     string         `json:"pixel,omitempty"`        // 像素标识
	URL       string         `json:"url,omitempty"`          // 页面 URL
	Referrer  string         `json:"referrer,omitempty"`     // 来源 URL
	Name      string         `json:"name,omitempty"`         // 事件名称
	Title     string         `json:"title,omitempty"`        // 页面标题
	Hostname  string         `json:"hostname,omitempty"`     // 主机名
	Language  string         `json:"language,omitempty"`     // 浏览器语言
	Screen    string         `json:"screen,omitempty"`       // 屏幕分辨率
	Timestamp int64          `json:"timestamp,omitempty"`    // 事件时间戳（毫秒）
	ID        string         `json:"id,omitempty"`           // 访客唯一标识
	Browser   string         `json:"browser,omitempty"`      // 浏览器名称
	OS        string         `json:"os,omitempty"`           // 操作系统
	Device    string         `json:"device,omitempty"`       // 设备类型
	Country   string         `json:"country,omitempty"`      // 国家
	Region    string         `json:"region,omitempty"`       // 地区
	City      string         `json:"city,omitempty"`         // 城市
	UTMSource string         `json:"utm_source,omitempty"`   // UTM 来源
	UTMMedium string         `json:"utm_medium,omitempty"`   // UTM 媒介
	UTMCamp   string         `json:"utm_campaign,omitempty"` // UTM 广告系列
	UTMCont   string         `json:"utm_content,omitempty"`  // UTM 内容
	UTMTerm   string         `json:"utm_term,omitempty"`     // UTM 关键词
	Data      map[string]any `json:"data,omitempty"`         // 自定义数据
	Revenue   *RevenueInput  `json:"revenue,omitempty"`      // 收入数据
}

// RevenueInput - 收入输入数据
type RevenueInput struct {
	Amount   float64 `json:"amount"`   // 收入金额
	Currency string  `json:"currency"` // 货币代码
}

// eventRecord - 事件记录（数据库写入用）
type eventRecord struct {
	WebsiteID      string    // 网站 ID
	PixelID        string    // 像素 ID
	VisitorID      string    // 访客 ID
	SessionID      string    // 会话 ID
	EventType      string    // 事件类型
	EventName      string    // 事件名称
	PageTitle      string    // 页面标题
	Hostname       string    // 主机名
	URL            string    // 完整 URL
	URLPath        string    // URL 路径
	Referrer       string    // 来源 URL
	ReferrerDomain string    // 来源域名
	UTMSource      string    // UTM 来源
	UTMMedium      string    // UTM 媒介
	UTMCampaign    string    // UTM 广告系列
	UTMContent     string    // UTM 内容
	UTMTerm        string    // UTM 关键词
	Browser        string    // 浏览器
	OS             string    // 操作系统
	Device         string    // 设备类型
	Country        string    // 国家
	Region         string    // 地区
	City           string    // 城市
	Amount         float64   // 收入金额
	Currency       string    // 货币代码
	Metadata       string    // 元数据（JSON 字符串）
	CreatedAt      time.Time // 创建时间
}

// sessionRecord - 会话记录
type sessionRecord struct {
	ID             string    // 会话 ID
	SessionKey     string    // 会话唯一键
	WebsiteID      string    // 网站 ID
	VisitorID      string    // 访客 ID
	StartedAt      time.Time // 会话开始时间
	LastSeenAt     time.Time // 最后活跃时间
	EventCount     int       // 事件计数
	Pageviews      int       // 页面浏览计数
	Referrer       string    // 来源 URL
	ReferrerDomain string    // 来源域名
	UTMSource      string    // UTM 来源
	UTMMedium      string    // UTM 媒介
	UTMCampaign    string    // UTM 广告系列
	Browser        string    // 浏览器
	OS             string    // 操作系统
	Device         string    // 设备类型
	Country        string    // 国家
	Region         string    // 地区
	City           string    // 城市
	EntryPath      string    // 入口页面路径
	ExitPath       string    // 出口页面路径
	PrevLastSeenAt time.Time // 上一次最后活跃时间（用于计算时长增量）
	PrevPageviews  int       // 上一次页面浏览数（用于判断跳出状态变化）
}

// queuedEvent - 队列中的事件
// 经过解析和丰富后放入事件队列
type queuedEvent struct {
	WebsiteID      string    // 网站 ID
	PixelID        string    // 像素 ID
	VisitorKey     string    // 访客唯一标识
	EventType      string    // 事件类型
	EventName      string    // 事件名称
	PageTitle      string    // 页面标题
	Hostname       string    // 主机名
	URL            string    // 完整 URL
	URLPath        string    // URL 路径
	Referrer       string    // 来源 URL
	ReferrerDomain string    // 来源域名
	UTMSource      string    // UTM 来源
	UTMMedium      string    // UTM 媒介
	UTMCampaign    string    // UTM 广告系列
	UTMContent     string    // UTM 内容
	UTMTerm        string    // UTM 关键词
	Browser        string    // 浏览器
	OS             string    // 操作系统
	Device         string    // 设备类型
	Country        string    // 国家
	Region         string    // 地区
	City           string    // 城市
	Amount         float64   // 收入金额
	Currency       string    // 货币代码
	Metadata       string    // 元数据（JSON 字符串）
	CreatedAt      time.Time // 创建时间
}
