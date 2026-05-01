package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeEventType(t *testing.T) {
	t.Run("revenue", func(t *testing.T) {
		got := normalizeEventType(eventPayload{Revenue: &RevenueInput{Amount: 10}}, "")
		if got != "revenue" {
			t.Fatalf("expected revenue, got %s", got)
		}
	})

	t.Run("event", func(t *testing.T) {
		got := normalizeEventType(eventPayload{Name: "signup"}, "")
		if got != "event" {
			t.Fatalf("expected event, got %s", got)
		}
	})

	t.Run("pixel", func(t *testing.T) {
		got := normalizeEventType(eventPayload{}, "pixel-id")
		if got != "pixel" {
			t.Fatalf("expected pixel, got %s", got)
		}
	})

	t.Run("pageview", func(t *testing.T) {
		got := normalizeEventType(eventPayload{}, "")
		if got != "pageview" {
			t.Fatalf("expected pageview, got %s", got)
		}
	})
}

func TestIsBotRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Purpose", "prefetch")
	if !isBotRequest(req) {
		t.Fatal("expected prefetch request to be treated as bot-like traffic")
	}
}

func TestAllowRateLimit(t *testing.T) {
	app := &App{
		rateBuckets: map[string]rateBucket{},
	}
	key := "test"
	if !app.allowRateLimit(key, 2, 1, time.Minute) {
		t.Fatal("first request should pass")
	}
	if !app.allowRateLimit(key, 2, 1, time.Minute) {
		t.Fatal("second request should pass")
	}
	if app.allowRateLimit(key, 2, 1, time.Minute) {
		t.Fatal("third request should be blocked")
	}
}

func TestDetectGeo(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("CF-IPCountry", "US")
	req.Header.Set("CF-Region-Code", "CA")
	req.Header.Set("CF-IPCity", "San Francisco")
	country, region, city := detectGeo(req, eventPayload{})
	if country != "US" || region != "CA" || city != "San Francisco" {
		t.Fatalf("unexpected geo fallback: %q %q %q", country, region, city)
	}
}

func TestNormalizeEventCreatedAt(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	got := normalizeEventCreatedAt(now.Add(2*time.Minute).Unix(), now)
	if !got.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("expected in-range timestamp to be preserved, got %s", got)
	}

	got = normalizeEventCreatedAt(now.Add(20*time.Minute).Unix(), now)
	if !got.Equal(now) {
		t.Fatalf("expected far-future timestamp to fall back to now, got %s", got)
	}
}

func TestPathMatchesStepValue(t *testing.T) {
	cases := []struct {
		pathValue string
		expected  string
		match     bool
	}{
		{"/about", "/about", true},
		{"/about/", "/about", true},
		{"/products/123", "/products/**", true},
		{"/docs/intro", "/docs/*", true},
		{"/pricing", "/signup", false},
	}
	for _, tc := range cases {
		if got := pathMatchesStepValue(tc.pathValue, tc.expected); got != tc.match {
			t.Fatalf("pathMatchesStepValue(%q, %q) = %v, want %v", tc.pathValue, tc.expected, got, tc.match)
		}
	}
}

func TestShouldIgnoreBotTraffic(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Purpose", "prefetch")
	ignored, reason := app.shouldIgnoreBotTraffic(req)
	if !ignored || reason != "prefetch" {
		t.Fatalf("expected prefetch request to be ignored, got ignored=%v reason=%q", ignored, reason)
	}
}

func TestRecordBotAudit(t *testing.T) {
	app := &App{
		botAudit: map[string]int{},
	}
	app.recordBotAudit("bot")
	app.recordBotAudit("bot")
	app.recordBotAudit("prefetch")

	snapshot := app.botAuditSnapshot()
	if snapshot["bot"] != 2 || snapshot["prefetch"] != 1 {
		t.Fatalf("unexpected bot audit snapshot: %#v", snapshot)
	}
}

func TestIsPreviewBotTraffic(t *testing.T) {
	if !isPreviewBotTraffic("Slackbot 1.0") {
		t.Fatal("expected Slackbot to be treated as preview bot traffic")
	}
	if isPreviewBotTraffic("Mozilla/5.0") {
		t.Fatal("did not expect normal browser UA to be treated as preview bot traffic")
	}
}

func TestRecordEventUpdatesSessionsAndAggregates(t *testing.T) {
	app := newTestApp(t)
	websiteID := seedWebsite(t, app, "Demo", "demo.local")

	requestFor := func() *http.Request {
		req := httptest.NewRequest("POST", "/api/send", nil)
		req.RemoteAddr = "203.0.113.20:4321"
		req.Header.Set("User-Agent", "Mozilla/5.0")
		return req
	}

	if _, err := app.recordEvent(requestFor(), eventRequest{
		Type: "pageview",
		Payload: eventPayload{
			Website:  websiteID,
			URL:      "https://demo.local/pricing?utm_source=google&utm_medium=cpc&utm_campaign=spring",
			Referrer: "https://google.com/search?q=sitlys",
			ID:       "visitor-1",
		},
	}); err != nil {
		t.Fatalf("record pageview: %v", err)
	}
	if _, err := app.recordEvent(requestFor(), eventRequest{
		Type: "event",
		Payload: eventPayload{
			Website: websiteID,
			URL:     "https://demo.local/pricing?utm_source=google&utm_medium=cpc&utm_campaign=spring",
			Name:    "signup",
			ID:      "visitor-1",
		},
	}); err != nil {
		t.Fatalf("record custom event: %v", err)
	}
	if _, err := app.recordEvent(requestFor(), eventRequest{
		Type: "pageview",
		Payload: eventPayload{
			Website: websiteID,
			URL:     "https://demo.local/checkout?utm_source=google&utm_medium=cpc&utm_campaign=spring",
			ID:      "visitor-1",
		},
	}); err != nil {
		t.Fatalf("record checkout pageview: %v", err)
	}
	if _, err := app.recordEvent(requestFor(), eventRequest{
		Type: "revenue",
		Payload: eventPayload{
			Website: websiteID,
			URL:     "https://demo.local/checkout?utm_source=google&utm_medium=cpc&utm_campaign=spring",
			ID:      "visitor-1",
			Revenue: &RevenueInput{Amount: 99.9, Currency: "USD"},
		},
	}); err != nil {
		t.Fatalf("record revenue event: %v", err)
	}

	waitFor(t, 3*time.Second, func() bool {
		var count int
		if err := app.db.QueryRow(`select count(*) from events where website_id = ?`, websiteID).Scan(&count); err != nil {
			return false
		}
		return count == 4
	})

	var sessions, pageviews, events int
	var entryPath, exitPath string
	if err := app.db.QueryRow(`
		select count(*), coalesce(sum(pageviews), 0), coalesce(sum(event_count), 0), coalesce(min(entry_path), ''), coalesce(min(exit_path), '')
		from sessions
		where website_id = ?
	`, websiteID).Scan(&sessions, &pageviews, &events, &entryPath, &exitPath); err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	if sessions != 1 || pageviews != 2 || events != 4 {
		t.Fatalf("unexpected session rollup: sessions=%d pageviews=%d events=%d", sessions, pageviews, events)
	}
	if entryPath != "/pricing" {
		t.Fatalf("unexpected entry path: %q", entryPath)
	}
	if exitPath != "/checkout" {
		t.Fatalf("unexpected exit path: %q", exitPath)
	}

	var aggPageviews, aggEvents int
	var aggRevenue float64
	if err := app.db.QueryRow(`
		select coalesce(sum(pageviews), 0), coalesce(sum(custom_events), 0), coalesce(sum(revenue), 0)
		from agg_overview_daily
		where website_id = ?
	`, websiteID).Scan(&aggPageviews, &aggEvents, &aggRevenue); err != nil {
		t.Fatalf("query overview aggregate: %v", err)
	}
	if aggPageviews != 2 || aggEvents != 2 || aggRevenue != 99.9 {
		t.Fatalf("unexpected overview aggregate: pageviews=%d events=%d revenue=%v", aggPageviews, aggEvents, aggRevenue)
	}

	var source, medium, campaign string
	var attributionSessions int
	var attributionRevenue float64
	if err := app.db.QueryRow(`
		select source, medium, campaign, sessions, revenue
		from agg_attribution_daily
		where website_id = ?
	`, websiteID).Scan(&source, &medium, &campaign, &attributionSessions, &attributionRevenue); err != nil {
		t.Fatalf("query attribution aggregate: %v", err)
	}
	if source != "google" || medium != "cpc" || campaign != "spring" || attributionSessions != 1 || attributionRevenue != 99.9 {
		t.Fatalf("unexpected attribution aggregate: source=%q medium=%q campaign=%q sessions=%d revenue=%v", source, medium, campaign, attributionSessions, attributionRevenue)
	}

	var revenueSource, currency string
	var revenueEvents int
	var revenueAmount float64
	if err := app.db.QueryRow(`
		select source, currency, event_count, revenue
		from agg_revenue_daily
		where website_id = ?
	`, websiteID).Scan(&revenueSource, &currency, &revenueEvents, &revenueAmount); err != nil {
		t.Fatalf("query revenue aggregate: %v", err)
	}
	if revenueSource != "google" || currency != "USD" || revenueEvents != 1 || revenueAmount != 99.9 {
		t.Fatalf("unexpected revenue aggregate: source=%q currency=%q events=%d revenue=%v", revenueSource, currency, revenueEvents, revenueAmount)
	}
}

func TestWebsitePermissionGuards(t *testing.T) {
	app := newTestApp(t)
	handler := app.routes()
	websiteID := seedWebsite(t, app, "Primary", "primary.local")
	otherWebsiteID := seedWebsite(t, app, "Other", "other.local")

	if err := app.writeEventImmediately(queuedEvent{
		WebsiteID:  websiteID,
		VisitorKey: "seed-visitor",
		EventType:  "pageview",
		URL:        "https://primary.local/",
		URLPath:    "/",
		CreatedAt:  nowUTC(),
	}); err != nil {
		t.Fatalf("seed analytics event: %v", err)
	}

	viewerID, viewerToken := seedUser(t, app, "viewer", roleViewer, []WebsitePermission{{WebsiteID: websiteID, AccessLevel: "view"}})
	_, _ = viewerID, viewerToken
	managerID, managerToken := seedUser(t, app, "manager", roleAdmin, []WebsitePermission{{WebsiteID: websiteID, AccessLevel: "manage"}})
	_, _ = managerID, managerToken

	t.Run("website list is scoped to assigned sites", func(t *testing.T) {
		req := authedJSONRequest(t, http.MethodGet, "/api/websites", nil, viewerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Websites []Website `json:"websites"`
		}
		decodeTestJSON(t, rec.Body.Bytes(), &payload)
		if len(payload.Websites) != 1 || payload.Websites[0].ID != websiteID {
			t.Fatalf("unexpected scoped websites: %#v", payload.Websites)
		}
	})

	t.Run("viewer can read assigned website analytics", func(t *testing.T) {
		req := authedJSONRequest(t, http.MethodGet, "/api/analytics/overview?website_id="+websiteID+"&from="+nowUTC().Format("2006-01-02")+"&to="+nowUTC().Format("2006-01-02"), nil, viewerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("viewer cannot read unassigned website analytics", func(t *testing.T) {
		req := authedJSONRequest(t, http.MethodGet, "/api/analytics/overview?website_id="+otherWebsiteID+"&from="+nowUTC().Format("2006-01-02")+"&to="+nowUTC().Format("2006-01-02"), nil, viewerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("viewer cannot manage pixels", func(t *testing.T) {
		req := authedJSONRequest(t, http.MethodPost, "/api/websites/"+websiteID+"/pixels", map[string]any{"name": "Main Pixel"}, viewerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("manager can manage assigned website", func(t *testing.T) {
		req := authedJSONRequest(t, http.MethodPost, "/api/websites/"+websiteID+"/pixels", map[string]any{"name": "Main Pixel"}, managerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestRunFunnelCalculatesConversionAndDropOff(t *testing.T) {
	app := newTestApp(t)
	websiteID := seedWebsite(t, app, "Demo", "demo.local")
	base := nowUTC().Add(-2 * time.Hour)

	seedFunnelSession := func(visitor string, events []queuedEvent) {
		for _, item := range events {
			item.WebsiteID = websiteID
			item.VisitorKey = visitor
			item.CreatedAt = base
			if err := app.writeEventImmediately(item); err != nil {
				t.Fatalf("seed funnel event for %s: %v", visitor, err)
			}
			base = base.Add(1 * time.Minute)
		}
		base = base.Add(31 * time.Minute)
	}

	seedFunnelSession("visitor-a", []queuedEvent{
		{EventType: "pageview", URL: "https://demo.local/landing", URLPath: "/landing"},
		{EventType: "event", EventName: "signup", URL: "https://demo.local/landing", URLPath: "/landing"},
		{EventType: "pageview", URL: "https://demo.local/checkout", URLPath: "/checkout"},
	})
	seedFunnelSession("visitor-b", []queuedEvent{
		{EventType: "pageview", URL: "https://demo.local/landing", URLPath: "/landing"},
	})
	seedFunnelSession("visitor-c", []queuedEvent{
		{EventType: "pageview", URL: "https://demo.local/landing", URLPath: "/landing"},
		{EventType: "event", EventName: "signup", URL: "https://demo.local/landing", URLPath: "/landing"},
	})

	report, err := app.runFunnel(Funnel{
		WebsiteID: websiteID,
		Name:      "Signup funnel",
		Steps: []FunnelStep{
			{Label: "Landing", Type: "page", Value: "/landing"},
			{Label: "Signup", Type: "event", Value: "signup"},
			{Label: "Checkout", Type: "page", Value: "/checkout"},
		},
	}, base.Add(-24*time.Hour), base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("run funnel: %v", err)
	}

	steps, ok := report["steps"].([]map[string]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("unexpected funnel payload: %#v", report["steps"])
	}
	assertStep := func(index int, sessions int, conversion float64, dropOffCount int, dropOffRate float64) {
		step := steps[index]
		if step["sessions"].(int) != sessions {
			t.Fatalf("step %d sessions = %#v, want %d", index+1, step["sessions"], sessions)
		}
		if step["conversion"].(float64) != conversion {
			t.Fatalf("step %d conversion = %#v, want %v", index+1, step["conversion"], conversion)
		}
		if step["drop_off_count"].(int) != dropOffCount {
			t.Fatalf("step %d drop_off_count = %#v, want %d", index+1, step["drop_off_count"], dropOffCount)
		}
		if step["drop_off_rate"].(float64) != dropOffRate {
			t.Fatalf("step %d drop_off_rate = %#v, want %v", index+1, step["drop_off_rate"], dropOffRate)
		}
	}
	assertStep(0, 3, 1, 0, 0)
	assertStep(1, 2, 2.0/3.0, 1, 1.0/3.0)
	assertStep(2, 1, 1.0/3.0, 1, 0.5)
}

func TestRetentionReportUsesCohorts(t *testing.T) {
	app := newTestApp(t)
	handler := app.routes()
	websiteID := seedWebsite(t, app, "Demo", "demo.local")
	_, token := seedUser(t, app, "analyst", roleAnalyst, []WebsitePermission{{WebsiteID: websiteID, AccessLevel: "view"}})

	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	seedVisit := func(visitor string, at time.Time) {
		if err := app.writeEventImmediately(queuedEvent{
			WebsiteID:  websiteID,
			VisitorKey: visitor,
			EventType:  "pageview",
			URL:        "https://demo.local/",
			URLPath:    "/",
			CreatedAt:  at,
		}); err != nil {
			t.Fatalf("seed retention visit for %s at %s: %v", visitor, at, err)
		}
	}

	seedVisit("visitor-1", base)
	seedVisit("visitor-1", base.AddDate(0, 0, 1))
	seedVisit("visitor-1", base.AddDate(0, 0, 7))
	seedVisit("visitor-1", base.AddDate(0, 0, 30))
	seedVisit("visitor-2", base)
	seedVisit("visitor-2", base.AddDate(0, 0, 1))

	req := authedJSONRequest(t, http.MethodGet, "/api/analytics/retention?website_id="+websiteID+"&from="+base.Format("2006-01-02")+"&to="+base.Format("2006-01-02"), nil, token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []struct {
			Cohort string  `json:"cohort"`
			Size   int     `json:"size"`
			Day1   float64 `json:"day_1"`
			Day7   float64 `json:"day_7"`
			Day30  float64 `json:"day_30"`
		} `json:"items"`
	}
	decodeTestJSON(t, rec.Body.Bytes(), &payload)
	if len(payload.Items) != 1 {
		t.Fatalf("unexpected retention rows: %#v", payload.Items)
	}
	row := payload.Items[0]
	if row.Cohort != "2026-03-01" || row.Size != 2 || row.Day1 != 1 || row.Day7 != 0.5 || row.Day30 != 0.5 {
		t.Fatalf("unexpected retention row: %#v", row)
	}
}

func TestCreateBackupWritesDatabaseSnapshot(t *testing.T) {
	app := newTestApp(t)
	websiteID := seedWebsite(t, app, "Demo", "demo.local")
	if err := app.writeEventImmediately(queuedEvent{
		WebsiteID:  websiteID,
		VisitorKey: "visitor-1",
		EventType:  "pageview",
		URL:        "https://demo.local/",
		URLPath:    "/",
		CreatedAt:  nowUTC(),
	}); err != nil {
		t.Fatalf("seed backup event: %v", err)
	}

	backupPath, err := app.createBackup()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if filepath.Dir(backupPath) != filepath.Join(app.cfg.DataDir, "backups") {
		t.Fatalf("backup path escaped backup dir: %s", backupPath)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}

	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatalf("open backup db: %v", err)
	}
	defer backupDB.Close()

	var count int
	if err := backupDB.QueryRow(`select count(*) from websites where id = ?`, websiteID).Scan(&count); err != nil {
		t.Fatalf("query backup db: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected website in backup, got %d", count)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	dataDir := t.TempDir()
	app, err := New(Config{
		Addr:        "127.0.0.1:0",
		DataDir:     dataDir,
		DBPath:      filepath.Join(dataDir, "sitlys.db"),
		SessionDays: 30,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})
	return app
}

func seedWebsite(t *testing.T, app *App, name, domain string) string {
	t.Helper()
	websiteID := newID()
	now := iso(nowUTC())
	if _, err := app.db.Exec(`
		insert into websites(id, name, domain, created_at, updated_at)
		values(?, ?, ?, ?, ?)
	`, websiteID, name, domain, now, now); err != nil {
		t.Fatalf("seed website: %v", err)
	}
	return websiteID
}

func seedUser(t *testing.T, app *App, username, role string, permissions []WebsitePermission) (string, string) {
	t.Helper()
	userID := newID()
	hash, err := passwordHash("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	now := iso(nowUTC())
	if _, err := app.db.Exec(`
		insert into users(id, username, password_hash, role, enabled, created_at, updated_at)
		values(?, ?, ?, ?, 1, ?, ?)
	`, userID, username, hash, role, now, now); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if len(permissions) > 0 {
		tx, err := app.db.Begin()
		if err != nil {
			t.Fatalf("begin permission tx: %v", err)
		}
		if err := upsertPermissions(tx, userID, permissions); err != nil {
			_ = tx.Rollback()
			t.Fatalf("seed permissions: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit permissions: %v", err)
		}
	}
	token, _, err := app.createSession(userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return userID, token
}

func authedJSONRequest(t *testing.T, method, target string, body any, token string) *http.Request {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		req = httptest.NewRequest(method, target, bytes.NewReader(payload))
		req.Body = io.NopCloser(bytes.NewReader(payload))
		req.ContentLength = int64(len(payload))
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	return req
}

func decodeTestJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode json: %v; body=%s", err, string(data))
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not satisfied before timeout")
}
