// Package main - 地理位置解析模块
// 负责根据 IP 地址解析访问者的地理位置信息（国家、地区、城市）
// 支持多种数据源：
//   - MaxMind GeoIP2 数据库（国际 IP 精确度高）
//   - 纯真 IP 数据库（中国 IP 覆盖好）
//   - CDN/代理传递的 HTTP 头（Cloudflare、App Engine 等）
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/oschwald/geoip2-golang"
	"github.com/xiaoqidun/qqwry"
)

// closeGeoIPDB - 关闭 MaxMind GeoIP2 数据库连接
func (a *App) closeGeoIPDB() {
	a.geoIPMu.Lock()
	defer a.geoIPMu.Unlock()
	if a.geoIPDB != nil {
		_ = a.geoIPDB.Close()
		a.geoIPDB = nil
	}
}

// cleanGeoLabel - 清理地理位置标签
// 如果 value 为空则返回 fallback 默认值
func cleanGeoLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

// reloadQQWryDB - 重新加载纯真 IP 数据库
// 在多个候选路径中查找 qqwry.ipdb 文件
func (a *App) reloadQQWryDB() error {
	candidates := []string{
		filepath.Join(a.cfg.DataDir, "qqwry.ipdb"),
		filepath.Join("server", "qqwry.ipdb"),
		"qqwry.ipdb",
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "qqwry.ipdb"))
		candidates = append(candidates, filepath.Join(wd, "server", "qqwry.ipdb"))
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		a.closeQQWryDB()
		return nil
	}
	db, err := qqwry.NewClient(path)
	if err != nil {
		Error("qqwry database unavailable at %s: %v", path, err)
		log.Printf("qqwry database unavailable at %s: %v", path, err)
		a.closeQQWryDB()
		return nil
	}
	a.qqwryMu.Lock()
	defer a.qqwryMu.Unlock()
	a.qqwryDB = db
	log.Printf("qqwry database loaded from %s", path)
	return nil
}

// closeQQWryDB - 关闭纯真 IP 数据库连接
func (a *App) closeQQWryDB() {
	a.qqwryMu.Lock()
	defer a.qqwryMu.Unlock()
	a.qqwryDB = nil
}

// lookupQQWry - 通过纯真 IP 数据库查询地理位置
// 返回国家、地区、城市信息
func (a *App) lookupQQWry(ip string) (country, region, city string) {
	a.qqwryMu.RLock()
	db := a.qqwryDB
	a.qqwryMu.RUnlock()
	if db == nil {
		return "", "", ""
	}
	location, err := db.QueryIP(strings.TrimSpace(ip))
	if err != nil || location == nil {
		Error("qqwry query ip failed for ip %s: %v", ip, err)
		return "", "", ""
	}
	country = normalizeGeoCountry(location.Country)
	if isIgnoredGeoCountry(country) {
		return "", "", ""
	}
	region = strings.TrimSpace(location.Province)
	city = strings.TrimSpace(location.City)
	if city == "" {
		city = strings.TrimSpace(location.District)
	}
	return
}

// normalizeGeoCountry - 规范化国家名称
// 将中文国家名转换为标准代码（如 "中国" -> "CN"）
func normalizeGeoCountry(country string) string {
	country = strings.TrimSpace(country)
	switch country {
	case "", "0":
		return ""
	case "中国":
		return "CN"
	default:
		return country
	}
}

// isIgnoredGeoCountry - 判断是否为应忽略的地理位置
// 忽略保留地址、局域网等无效地理位置
func isIgnoredGeoCountry(country string) bool {
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "", "0", "IANA", "LAN":
		return true
	default:
		return country == "局域网"
	}
}

// reloadGeoIPDB - 重新加载 MaxMind GeoIP2 数据库
func (a *App) reloadGeoIPDB() error {
	path, err := resolveGeoIPDBPath(a.cfg.GeoIPDBPath, a.cfg.DataDir)
	if err != nil {
		Error("resolve geoip db path failed: %v", err)
		return err
	}
	if path == "" {
		a.closeGeoIPDB()
		return nil
	}
	reader, err := geoip2.Open(path)
	if err != nil {
		Error("geoip database unavailable at %s: %v", path, err)
		log.Printf("geoip database unavailable at %s, disabling local geo lookup: %v", path, err)
		a.closeGeoIPDB()
		return nil
	}
	a.geoIPMu.Lock()
	defer a.geoIPMu.Unlock()
	if a.geoIPDB != nil {
		_ = a.geoIPDB.Close()
	}
	a.geoIPDB = reader
	return nil
}

// resolveGeoIPDBPath - 解析 GeoIP 数据库文件路径
// 优先使用配置的路径，否则在默认候选路径中查找
func resolveGeoIPDBPath(configuredPath, dataDir string) (string, error) {
	if path := strings.TrimSpace(configuredPath); path != "" {
		return validateGeoIPDBPath(path)
	}
	for _, candidate := range defaultGeoIPDBCandidates(dataDir) {
		path, err := validateGeoIPDBPath(candidate)
		if err == nil && path != "" {
			return path, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			Error("validate geoip db candidate %s failed: %v", candidate, err)
			return "", err
		}
	}
	return "", nil
}

// defaultGeoIPDBCandidates - 获取默认的 GeoIP 数据库候选路径列表
func defaultGeoIPDBCandidates(dataDir string) []string {
	candidates := make([]string, 0, 4)
	if strings.TrimSpace(dataDir) != "" {
		candidates = append(candidates, filepath.Join(dataDir, "GeoLite2-City.mmdb"))
	}
	if exePath, err := os.Executable(); err == nil && strings.TrimSpace(exePath) != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "GeoLite2-City.mmdb"))
	}
	candidates = append(candidates, "GeoLite2-City.mmdb", filepath.Join("server", "GeoLite2-City.mmdb"))
	return uniqueNonEmptyPaths(candidates)
}

// uniqueNonEmptyPaths - 去重并过滤空路径
func uniqueNonEmptyPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

// validateGeoIPDBPath - 验证 GeoIP 数据库路径是否有效
// 检查路径是否存在且不是目录
func validateGeoIPDBPath(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("geoip db path is a directory: %s", path)
	}
	return path, nil
}

// lookupGeoIP - 通过 IP 地址查询地理位置
// 查询顺序：纯真 IP 数据库 -> MaxMind GeoIP2 数据库
func (a *App) lookupGeoIP(rawIP string) (country, region, city string) {
	ip := net.ParseIP(strings.TrimSpace(rawIP))
	if ip == nil {
		return "", "", ""
	}

	// 优先使用纯真 IP 数据库查询
	country, region, city = a.lookupQQWry(rawIP)
	if country != "" {
		return
	}

	// 回退到 MaxMind GeoIP2 数据库查询
	a.geoIPMu.RLock()
	reader := a.geoIPDB
	a.geoIPMu.RUnlock()
	if reader == nil {
		return "", "", ""
	}

	record, err := reader.City(ip)
	if err != nil {
		Error("geoip2 city lookup failed for ip %s: %v", rawIP, err)
		return "", "", ""
	}
	if record.Country.IsoCode != "" {
		country = strings.TrimSpace(record.Country.IsoCode)
	}
	for _, subdivision := range record.Subdivisions {
		if subdivision.IsoCode != "" {
			region = strings.TrimSpace(subdivision.IsoCode)
			break
		}
	}
	if name := record.City.Names["en"]; name != "" {
		city = strings.TrimSpace(name)
	}
	return
}

// detectGeo - 从 HTTP 请求和事件负载中检测地理位置（App 方法）
// 检测顺序：事件负载中的地理位置 -> CDN/代理头 -> GeoIP 数据库
func (a *App) detectGeo(r *http.Request, payload eventPayload) (country, region, city string) {
	// 优先使用事件负载中客户端上报的地理位置
	country = strings.TrimSpace(payload.Country)
	region = strings.TrimSpace(payload.Region)
	city = strings.TrimSpace(payload.City)
	if country != "" || region != "" || city != "" {
		return
	}

	// 尝试从 CDN/代理头获取地理位置
	country = strings.TrimSpace(firstNonEmpty(
		r.Header.Get("CF-IPCountry"),
		r.Header.Get("X-Appengine-Country"),
		r.Header.Get("X-Country-Code"),
	))
	region = strings.TrimSpace(firstNonEmpty(
		r.Header.Get("X-Appengine-Region"),
		r.Header.Get("CF-Region-Code"),
		r.Header.Get("X-Region-Code"),
	))
	city = strings.TrimSpace(firstNonEmpty(
		r.Header.Get("X-Appengine-City"),
		r.Header.Get("CF-IPCity"),
		r.Header.Get("X-City"),
	))
	if country != "" || region != "" || city != "" {
		return
	}
	// 回退到 GeoIP 数据库查询
	return a.lookupGeoIP(clientIP(r))
}

// detectGeo - 从 HTTP 请求和事件负载中检测地理位置（独立函数）
// 检测顺序：事件负载中的地理位置 -> CDN/代理头
func detectGeo(r *http.Request, payload eventPayload) (country, region, city string) {
	// 优先使用事件负载中客户端上报的地理位置
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
