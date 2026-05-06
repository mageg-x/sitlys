// Package main - 工具函数模块
// 包含通用辅助函数，如路径解析、时间处理、密码加密、URL 解析、
// 用户代理检测、机器人流量判断、数据导出和指标计算等
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// defaultDataDir - 获取默认数据目录路径
// 根据操作系统返回不同的默认数据存储路径：
// Windows: %APPDATA%/sitlys
// macOS: ~/Library/Application Support/sitlys
// Linux: ~/.sitlys
// 如果无法确定用户目录，则回退到 ./data
func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		// 优先使用 APPDATA 环境变量
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "sitlys")
		}
		// 回退到用户主目录下的 AppData/Roaming/sitlys
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			return filepath.Join(homeDir, "AppData", "Roaming", "sitlys")
		}
	case "darwin":
		// macOS 使用 Library/Application Support 目录
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			return filepath.Join(homeDir, "Library", "Application Support", "sitlys")
		}
	default:
		// Linux 及其他系统使用隐藏目录
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			return filepath.Join(homeDir, ".sitlys")
		}
	}
	// 无法确定用户目录时回退到当前目录下的 data 文件夹
	return "./data"
}

// resolvePaths - 解析数据目录和数据库路径
// 根据提供的参数确定最终的数据目录和数据库文件路径
// 如果指定了数据库路径，则数据目录从数据库路径推导
// 否则使用默认数据目录，数据库路径为数据目录下的 sitlys.db
func resolvePaths(dataDir, dbPath string) (string, string) {
	if dbPath != "" {
		// 清理数据库路径
		dbPath = filepath.Clean(dbPath)
		// 如果未指定数据目录，则使用数据库路径的父目录
		if dataDir == "" {
			dataDir = filepath.Dir(dbPath)
		}
		return filepath.Clean(dataDir), dbPath
	}
	// 未指定数据库路径时使用默认数据目录
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	dataDir = filepath.Clean(dataDir)
	return dataDir, filepath.Join(dataDir, "sitlys.db")
}

// withLogging - HTTP 请求日志中间件
// 记录每个请求的方法、路径和耗时
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// nowUTC - 获取当前 UTC 时间（精确到秒）
func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

// iso - 将时间格式化为 ISO 8601 字符串（RFC3339 格式）
func iso(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// parseISO - 解析 ISO 8601 时间字符串
// 如果字符串为空或解析失败，返回零值时间
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

// newID - 生成 32 字符的随机十六进制 ID
// 使用加密安全的随机数生成器，产生 16 字节随机数后编码为十六进制
func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

// shortID - 生成 12 字符的短随机 ID
// 截取 newID 的前 12 个字符，用于像素 slug 和分享链接等场景
func shortID() string {
	return newID()[:12]
}

// tokenHash - 计算令牌的 SHA-256 哈希值
// 用于安全存储会话令牌，避免明文存储
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// passwordHash - 使用 bcrypt 加密密码
// 使用默认成本因子生成密码哈希值
func passwordHash(password string) (string, error) {
	sum, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		Error("failed to generate password hash: %v", err)
		return "", err
	}
	return string(sum), nil
}

// passwordMatch - 验证密码是否匹配哈希值
// 使用 bcrypt 安全比较函数，防止时序攻击
func passwordMatch(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// jsonResponse - 发送 JSON 格式的 HTTP 响应
// 设置 Content-Type 为 application/json 并写入响应体
func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// errorResponse - 发送 JSON 格式的错误响应
// 统一错误响应格式，包含 ok=false 和 error 消息
func errorResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]any{
		"ok":    false,
		"error": message,
	})
}

// decodeJSON - 解析请求体中的 JSON 数据（严格模式）
// 禁止未知字段，适用于管理 API 的请求解析
func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

// decodeCollectionJSON - 解析请求体中的 JSON 数据（宽松模式）
// 允许未知字段，适用于数据采集 API 的请求解析
func decodeCollectionJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(target)
}

// requestIsSecure - 判断请求是否通过 HTTPS 发起
// 检查 TLS 连接或 X-Forwarded-Proto 头（用于反向代理场景）
func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// parseDateInput - 解析日期输入字符串
// 支持两种格式：日期格式（2006-01-02）和 RFC3339 时间格式
func parseDateInput(value string) (time.Time, error) {
	// 如果字符串长度匹配日期格式，按日期解析
	if len(value) == len("2006-01-02") {
		return time.ParseInLocation("2006-01-02", value, time.UTC)
	}
	// 否则按 RFC3339 格式解析
	return time.Parse(time.RFC3339, value)
}

// cleanURL - 清理和解析 URL
// 返回原始 URL、主机名（小写）和路径
// 如果 URL 为空或解析失败，返回空字符串
func cleanURL(raw string) (fullURL, host, path string) {
	if raw == "" {
		return "", "", ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, "", ""
	}
	// 规范化主机名为小写
	host = strings.TrimSpace(strings.ToLower(parsed.Host))
	// 获取转义后的路径
	path = parsed.EscapedPath()
	// 路径为空时默认为根路径
	if path == "" {
		path = "/"
	}
	return raw, host, path
}

// normalizeWebsiteDomain - 规范化网站域名
// 将域名转换为小写，自动补全 https:// 协议前缀
// 返回格式为 hostname 或 hostname:port
func normalizeWebsiteDomain(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	// 如果缺少协议前缀，补上 https://
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	// 提取主机名（小写）
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		return ""
	}
	// 如果包含非标准端口，保留端口号
	if port := strings.TrimSpace(parsed.Port()); port != "" {
		return host + ":" + port
	}
	return host
}

// hostMatchesWebsiteDomain - 判断请求主机名是否匹配网站配置域名
// 支持精确匹配、端口匹配和子域名匹配（如 www.example.com 匹配 example.com）
func hostMatchesWebsiteDomain(host, configuredDomain string) bool {
	// 先规范化两端的主机名
	host = normalizeWebsiteDomain(host)
	configuredDomain = normalizeWebsiteDomain(configuredDomain)
	// 任一为空则不匹配
	if host == "" || configuredDomain == "" {
		return false
	}
	// 精确匹配
	if host == configuredDomain {
		return true
	}
	// 提取主机名部分（去掉端口）
	hostName := host
	configuredName := configuredDomain
	if strings.Contains(host, ":") {
		hostName = strings.SplitN(host, ":", 2)[0]
	}
	// 如果配置域名包含端口，需要额外检查端口匹配
	if strings.Contains(configuredDomain, ":") {
		configuredName = strings.SplitN(configuredDomain, ":", 2)[0]
		configuredPort := strings.SplitN(configuredDomain, ":", 2)[1]
		// 如果请求也包含端口，则端口必须一致
		if strings.Contains(host, ":") && strings.SplitN(host, ":", 2)[1] != configuredPort {
			return false
		}
	}
	// 主机名精确匹配
	if hostName == configuredName {
		return true
	}
	if configuredName == "" {
		return false
	}
	// 子域名匹配：请求主机名以 .配置域名 结尾
	return strings.HasSuffix(hostName, "."+configuredName)
}

// referrerDomain - 提取来源 URL 的域名
// 返回小写的域名部分，解析失败时返回空字符串
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

// clientIP - 获取客户端的真实 IP 地址
// 优先从代理头（X-Forwarded-For、X-Real-Ip）中提取
// 如果代理头不存在，则从 RemoteAddr 中解析
func clientIP(r *http.Request) string {
	// 遍历常见的代理头，提取第一个 IP 地址
	for _, header := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		// X-Forwarded-For 可能包含多个 IP，取第一个
		parts := strings.Split(value, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// 从 RemoteAddr 中分离主机和端口
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// normalizeEventType - 规范化事件类型
// 根据事件负载中的字段判断事件类型：
// 有收入信息 → revenue，有事件名称 → event，有像素 ID → pixel，否则 → pageview
func normalizeEventType(payload eventPayload, pixelID string) string {
	if payload.Revenue != nil {
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

// detectUserAgent - 检测用户代理信息
// 从请求的 User-Agent 头中识别浏览器、操作系统和设备类型
// 如果事件负载中已提供这些信息，则直接使用
func detectUserAgent(r *http.Request, payload eventPayload) (browser, osName, device string) {
	// 优先使用客户端提供的浏览器、OS 和设备信息
	if payload.Browser != "" || payload.OS != "" || payload.Device != "" {
		return payload.Browser, payload.OS, payload.Device
	}
	ua := strings.ToLower(r.UserAgent())
	// 检测浏览器类型（注意 Edge 包含 Chrome 关键字，需优先判断）
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
	// 检测操作系统
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
	// 检测设备类型
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

// isBotTraffic - 判断是否为机器人流量
// 通过检查 User-Agent 中的已知机器人关键字来识别
func isBotTraffic(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
	}
	// 遍历已知的机器人关键字列表
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
		"python-requests",
		"go-http-client",
	} {
		if strings.Contains(ua, token) {
			return true
		}
	}
	return false
}

// isPreviewBotTraffic - 判断是否为预览机器人流量
// 识别社交媒体预览爬虫（如 Discord、Telegram、WhatsApp 等）
func isPreviewBotTraffic(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
	}
	// 遍历已知的社交媒体预览机器人关键字
	for _, token := range []string{
		"discordbot",
		"telegrambot",
		"whatsapp",
		"slackbot",
		"facebookexternalhit",
		"linkedinbot",
	} {
		if strings.Contains(ua, token) {
			return true
		}
	}
	return false
}

// isBotRequest - 判断请求是否为机器人预取请求
// 通过检查 Purpose、Sec-Purpose 和 X-Moz 头来识别浏览器预取行为
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

// isValidRole - 验证用户角色是否合法
// 支持的角色：super_admin、admin、analyst、viewer
func isValidRole(role string) bool {
	switch role {
	case roleSuperAdmin, roleAdmin, roleAnalyst, roleViewer:
		return true
	default:
		return false
	}
}

// boolInt - 将布尔值转换为整数
// true → 1，false → 0，用于 SQLite 数据库存储
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// sqliteStringLiteral - 生成 SQLite 字符串字面量
// 用单引号包裹字符串，并转义内部的单引号
func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// normalizeEventCreatedAt - 规范化事件创建时间
// 处理秒级和毫秒级时间戳，并验证时间范围的合理性
// 超过 7 天前或 10 分钟后的时间戳将被修正为当前时间
func normalizeEventCreatedAt(timestamp int64, now time.Time) time.Time {
	createdAt := now
	// 时间戳为 0 时使用当前时间
	if timestamp == 0 {
		return createdAt
	}
	// 根据时间戳量级判断是毫秒还是秒
	if timestamp > 1_000_000_000_000 {
		createdAt = time.UnixMilli(timestamp).UTC()
	} else {
		createdAt = time.Unix(timestamp, 0).UTC()
	}
	// 验证时间范围：不允许超过 7 天前或 10 分钟后的时间
	if createdAt.Before(now.Add(-7*24*time.Hour)) || createdAt.After(now.Add(10*time.Minute)) {
		return now
	}
	return createdAt
}

// normalizeBotFilterMode - 规范化机器人过滤模式
// 支持三种模式：off（关闭）、strict（严格）、balanced（平衡，默认）
func normalizeBotFilterMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "off":
		return "off"
	case "strict":
		return "strict"
	default:
		return "balanced"
	}
}

// firstNonEmpty - 返回第一个非空字符串
// 遍历参数列表，返回第一个去除空白后非空的字符串
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// extractUTM - 从 URL 中提取 UTM 参数
// 解析 URL 并返回指定 key 的查询参数值
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

// retentionRate - 计算留存率
// 返回命中数占总数的比例，总数为 0 时返回 0
func retentionRate(hit, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// writeExport - 写入数据导出响应
// 支持两种导出格式：JSON 和 CSV
// 根据格式参数设置对应的 Content-Type 和 Content-Disposition 头
func writeExport(w http.ResponseWriter, format, name string, headers []string, records [][]string) {
	if format == "json" {
		// JSON 格式：将记录转换为对象数组
		items := make([]map[string]string, 0, len(records))
		for _, record := range records {
			item := make(map[string]string, len(headers))
			for index, header := range headers {
				if index < len(record) {
					item[header] = record[index]
				}
			}
			items = append(items, item)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, name))
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "items": items})
		return
	}

	// CSV 格式：写入表头和数据行
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write(headers)
	for _, record := range records {
		_ = writer.Write(record)
	}
	writer.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.Bytes())
}

// metricDelta - 计算整数指标的环比变化
// 返回当前值、前值、变化量和变化率
func metricDelta(current, previous int64) map[string]any {
	change := current - previous
	changeRate := 0.0
	// 前值大于 0 时才计算变化率，避免除零
	if previous > 0 {
		changeRate = float64(change) / float64(previous)
	}
	return map[string]any{"current": current, "previous": previous, "change": change, "change_rate": changeRate}
}

// metricDeltaFloat - 计算浮点数指标的环比变化
// 返回当前值、前值、变化量和变化率
func metricDeltaFloat(current, previous float64) map[string]any {
	change := current - previous
	changeRate := 0.0
	// 前值大于 0 时才计算变化率，避免除零
	if previous > 0 {
		changeRate = change / previous
	}
	return map[string]any{"current": current, "previous": previous, "change": change, "change_rate": changeRate}
}

// attributionKey - 提取会话的归因键
// 根据会话的 UTM 参数和来源域名确定来源、媒介和广告活动
// UTM 参数优先，来源域名次之，无来源时标记为直接访问
func attributionKey(session sessionRecord) (string, string, string) {
	// 确定流量来源
	source := session.UTMSource
	if source == "" {
		if session.ReferrerDomain != "" {
			source = session.ReferrerDomain
		} else {
			source = "(direct)"
		}
	}
	// 确定流量媒介
	medium := session.UTMMedium
	if medium == "" {
		if session.ReferrerDomain != "" {
			medium = "referral"
		} else {
			medium = "(none)"
		}
	}
	// 确定广告活动
	campaign := session.UTMCampaign
	if campaign == "" {
		campaign = "(none)"
	}
	return source, medium, campaign
}

// nullUnknown - 将空字符串替换为 "Unknown"
// 用于显示层面对空值的统一处理
func nullUnknown(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}

// sessionRollingKey - 生成会话滚动键
// 用于会话窗口的标识，格式为 websiteID:visitorID:startedAt
func sessionRollingKey(websiteID, visitorID string, startedAt time.Time) string {
	return websiteID + ":" + visitorID + ":" + startedAt.UTC().Format(time.RFC3339)
}
