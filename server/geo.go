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

func (a *App) closeGeoIPDB() {
	a.geoIPMu.Lock()
	defer a.geoIPMu.Unlock()
	if a.geoIPDB != nil {
		_ = a.geoIPDB.Close()
		a.geoIPDB = nil
	}
}

func cleanGeoLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

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

func (a *App) closeQQWryDB() {
	a.qqwryMu.Lock()
	defer a.qqwryMu.Unlock()
	a.qqwryDB = nil
}

func (a *App) lookupQQWry(ip string) (country, region, city string) {
	a.qqwryMu.RLock()
	db := a.qqwryDB
	a.qqwryMu.RUnlock()
	if db == nil {
		return "", "", ""
	}
	location, err := db.QueryIP(strings.TrimSpace(ip))
	if err != nil || location == nil {
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

func isIgnoredGeoCountry(country string) bool {
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "", "0", "IANA", "LAN":
		return true
	default:
		return country == "局域网"
	}
}

func (a *App) reloadGeoIPDB() error {
	path, err := resolveGeoIPDBPath(a.cfg.GeoIPDBPath, a.cfg.DataDir)
	if err != nil {
		return err
	}
	if path == "" {
		a.closeGeoIPDB()
		return nil
	}
	reader, err := geoip2.Open(path)
	if err != nil {
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
			return "", err
		}
	}
	return "", nil
}

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

func (a *App) lookupGeoIP(rawIP string) (country, region, city string) {
	ip := net.ParseIP(strings.TrimSpace(rawIP))
	if ip == nil {
		return "", "", ""
	}

	country, region, city = a.lookupQQWry(rawIP)
	if country != "" {
		return
	}

	a.geoIPMu.RLock()
	reader := a.geoIPDB
	a.geoIPMu.RUnlock()
	if reader == nil {
		return "", "", ""
	}

	record, err := reader.City(ip)
	if err != nil {
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

func (a *App) detectGeo(r *http.Request, payload eventPayload) (country, region, city string) {
	country = strings.TrimSpace(payload.Country)
	region = strings.TrimSpace(payload.Region)
	city = strings.TrimSpace(payload.City)
	if country != "" || region != "" || city != "" {
		return
	}

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
	return a.lookupGeoIP(clientIP(r))
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
