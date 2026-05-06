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

func decodeCollectionJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(target)
}

func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
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
	host = strings.TrimSpace(strings.ToLower(parsed.Host))
	path = parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return raw, host, path
}

func normalizeWebsiteDomain(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		return ""
	}
	if port := strings.TrimSpace(parsed.Port()); port != "" {
		return host + ":" + port
	}
	return host
}

func hostMatchesWebsiteDomain(host, configuredDomain string) bool {
	host = normalizeWebsiteDomain(host)
	configuredDomain = normalizeWebsiteDomain(configuredDomain)
	if host == "" || configuredDomain == "" {
		return false
	}
	if host == configuredDomain {
		return true
	}
	hostName := host
	configuredName := configuredDomain
	if strings.Contains(host, ":") {
		hostName = strings.SplitN(host, ":", 2)[0]
	}
	if strings.Contains(configuredDomain, ":") {
		configuredName = strings.SplitN(configuredDomain, ":", 2)[0]
		configuredPort := strings.SplitN(configuredDomain, ":", 2)[1]
		if strings.Contains(host, ":") && strings.SplitN(host, ":", 2)[1] != configuredPort {
			return false
		}
	}
	if hostName == configuredName {
		return true
	}
	if configuredName == "" {
		return false
	}
	return strings.HasSuffix(hostName, "."+configuredName)
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
		"python-requests",
		"go-http-client",
	} {
		if strings.Contains(ua, token) {
			return true
		}
	}
	return false
}

func isPreviewBotTraffic(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
	}
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

func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func normalizeEventCreatedAt(timestamp int64, now time.Time) time.Time {
	createdAt := now
	if timestamp == 0 {
		return createdAt
	}
	if timestamp > 1_000_000_000_000 {
		createdAt = time.UnixMilli(timestamp).UTC()
	} else {
		createdAt = time.Unix(timestamp, 0).UTC()
	}
	if createdAt.Before(now.Add(-7*24*time.Hour)) || createdAt.After(now.Add(10*time.Minute)) {
		return now
	}
	return createdAt
}

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

func retentionRate(hit, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

func writeExport(w http.ResponseWriter, format, name string, headers []string, records [][]string) {
	if format == "json" {
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

func metricDelta(current, previous int64) map[string]any {
	change := current - previous
	changeRate := 0.0
	if previous > 0 {
		changeRate = float64(change) / float64(previous)
	}
	return map[string]any{"current": current, "previous": previous, "change": change, "change_rate": changeRate}
}

func metricDeltaFloat(current, previous float64) map[string]any {
	change := current - previous
	changeRate := 0.0
	if previous > 0 {
		changeRate = change / previous
	}
	return map[string]any{"current": current, "previous": previous, "change": change, "change_rate": changeRate}
}

func attributionKey(session sessionRecord) (string, string, string) {
	source := session.UTMSource
	if source == "" {
		if session.ReferrerDomain != "" {
			source = session.ReferrerDomain
		} else {
			source = "(direct)"
		}
	}
	medium := session.UTMMedium
	if medium == "" {
		if session.ReferrerDomain != "" {
			medium = "referral"
		} else {
			medium = "(none)"
		}
	}
	campaign := session.UTMCampaign
	if campaign == "" {
		campaign = "(none)"
	}
	return source, medium, campaign
}

func nullUnknown(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}

func sessionRollingKey(websiteID, visitorID string, startedAt time.Time) string {
	return websiteID + ":" + visitorID + ":" + startedAt.UTC().Format(time.RFC3339)
}
