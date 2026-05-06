// Package main - 事件摄取模块
// 负责接收、处理和存储来自客户端的事件数据
// 核心流程：
//  1. 事件入队（handleEvent -> eventQueue）
//  2. 后台工作协程批量消费事件（runEventWriter）
//  3. 批量刷新到数据库（flushEventBatch）
//  4. 单事件写入（applyQueuedEvent）
//  5. 访客管理（upsertVisitorTx）
//  6. 会话管理（findOrCreateSessionTx）
//  7. 聚合数据实时更新（updateAggregatesTx）
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"
)

const (
	eventBatchSize     = 512                      // 事件批次大小
	eventFlushInterval = 400 * time.Millisecond   // 事件刷新间隔
)

// runEventWriter - 后台事件写入协程
// 从事件队列中批量消费事件，定时或批量满时刷新到数据库
func (a *App) runEventWriter() {
	defer a.workerWG.Done()

	ticker := time.NewTicker(eventFlushInterval)
	defer ticker.Stop()

	batch := make([]queuedEvent, 0, eventBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := a.flushEventBatch(batch); err != nil {
			Error("flush event batch failed: %v", err)
			log.Printf("flush event batch: %v", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-a.workerCtx.Done():
			// 关闭前排空队列中的剩余事件
			drain := true
			for drain {
				select {
				case item := <-a.eventQueue:
					batch = append(batch, item)
					if len(batch) >= eventBatchSize {
						flush()
					}
				default:
					drain = false
				}
			}
			flush()
			return
		case item := <-a.eventQueue:
			batch = append(batch, item)
			if len(batch) >= eventBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// flushEventBatch - 刷新事件批次到数据库
// 在互斥锁保护下开启事务，批量写入事件
func (a *App) flushEventBatch(batch []queuedEvent) error {
	a.eventWriteMu.Lock()
	defer a.eventWriteMu.Unlock()

	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		Error("begin event batch transaction failed: %v", err)
		return err
	}
	defer tx.Rollback()

	for _, item := range batch {
		if err := a.applyQueuedEvent(tx, item); err != nil {
			Error("apply queued event failed: %v", err)
			return err
		}
	}
	return tx.Commit()
}

// writeEventImmediately - 立即写入单个事件
// 用于需要同步写入的场景（如初始化事件）
func (a *App) writeEventImmediately(item queuedEvent) error {
	return a.flushEventBatch([]queuedEvent{item})
}

// applyQueuedEvent - 在事务中处理单个队列事件
// 执行流程：更新访客 -> 查找/创建会话 -> 插入事件 -> 更新聚合
func (a *App) applyQueuedEvent(tx *sql.Tx, item queuedEvent) error {
	visitorID, isNewVisitor, err := a.upsertVisitorTx(tx, item.WebsiteID, item.VisitorKey, item.CreatedAt)
	if err != nil {
		Error("upsert visitor failed: %v", err)
		return err
	}
	pageviews := 0
	entryPath := ""
	exitPath := ""
	if item.EventType == "pageview" && item.URLPath != "" {
		pageviews = 1
		entryPath = item.URLPath
		exitPath = item.URLPath
	}
	session, isNewSession, err := a.findOrCreateSessionTx(tx, sessionRecord{
		WebsiteID:      item.WebsiteID,
		VisitorID:      visitorID,
		StartedAt:      item.CreatedAt,
		LastSeenAt:     item.CreatedAt,
		Referrer:       item.Referrer,
		ReferrerDomain: item.ReferrerDomain,
		UTMSource:      item.UTMSource,
		UTMMedium:      item.UTMMedium,
		UTMCampaign:    item.UTMCampaign,
		Browser:        item.Browser,
		OS:             item.OS,
		Device:         item.Device,
		Country:        item.Country,
		Region:         item.Region,
		City:           item.City,
		Pageviews:      pageviews,
		EntryPath:      entryPath,
		ExitPath:       exitPath,
	})
	if err != nil {
		Error("find or create session failed: %v", err)
		return err
	}

	record := eventRecord{
		WebsiteID:      item.WebsiteID,
		PixelID:        item.PixelID,
		VisitorID:      visitorID,
		SessionID:      session.ID,
		EventType:      item.EventType,
		EventName:      item.EventName,
		PageTitle:      item.PageTitle,
		Hostname:       item.Hostname,
		URL:            item.URL,
		URLPath:        item.URLPath,
		Referrer:       item.Referrer,
		ReferrerDomain: item.ReferrerDomain,
		UTMSource:      item.UTMSource,
		UTMMedium:      item.UTMMedium,
		UTMCampaign:    item.UTMCampaign,
		UTMContent:     item.UTMContent,
		UTMTerm:        item.UTMTerm,
		Browser:        item.Browser,
		OS:             item.OS,
		Device:         item.Device,
		Country:        item.Country,
		Region:         item.Region,
		City:           item.City,
		Amount:         item.Amount,
		Currency:       item.Currency,
		Metadata:       item.Metadata,
		CreatedAt:      item.CreatedAt,
	}
	if err := a.insertEventTx(tx, record); err != nil {
		Error("insert event failed: %v", err)
		return err
	}
	return a.updateAggregatesTx(tx, record, session, isNewSession, isNewVisitor)
}

// upsertVisitorTx - 在事务中更新或创建访客记录
// 如果访客已存在则更新最后访问时间，否则创建新访客
// 返回访客 ID、是否新访客和错误
func (a *App) upsertVisitorTx(tx *sql.Tx, websiteID, externalID string, seenAt time.Time) (string, bool, error) {
	var visitorID string
	err := tx.QueryRow(`
		select id
		from visitors
		where website_id = ? and external_id = ?
	`, websiteID, externalID).Scan(&visitorID)
	switch {
	case err == nil:
		// 访客已存在，更新最后访问时间
		_, err = tx.Exec(`update visitors set last_seen_at = ? where id = ?`, iso(seenAt), visitorID)
		if err != nil {
			Error("update visitor last_seen_at failed: %v", err)
		}
		return visitorID, false, err
	case !errors.Is(err, sql.ErrNoRows):
		// 查询出错（非"未找到"错误）
		Error("query visitor failed: %v", err)
		return "", false, err
	}
	// 创建新访客
	visitorID = newID()
	_, err = tx.Exec(`
		insert into visitors(id, website_id, external_id, first_seen_at, last_seen_at)
		values(?, ?, ?, ?, ?)
	`, visitorID, websiteID, externalID, iso(seenAt), iso(seenAt))
	if err != nil {
		Error("insert visitor failed: %v", err)
	}
	return visitorID, true, err
}

// findOrCreateSessionTx - 在事务中查找或创建会话
// 会话窗口为 30 分钟，超时则创建新会话
// 返回会话记录、是否新会话和错误
func (a *App) findOrCreateSessionTx(tx *sql.Tx, candidate sessionRecord) (sessionRecord, bool, error) {
	var existing sessionRecord
	var startedAtText, lastSeenText string
	row := tx.QueryRow(`
		select id, session_key, website_id, visitor_id, started_at, last_seen_at, event_count, pageviews,
		       referrer, referrer_domain, utm_source, utm_medium, utm_campaign,
		       browser, os, device, country, region, city, entry_path, exit_path
		from sessions
		where website_id = ? and visitor_id = ?
		order by last_seen_at desc
		limit 1
	`, candidate.WebsiteID, candidate.VisitorID)
	err := row.Scan(
		&existing.ID, &existing.SessionKey, &existing.WebsiteID, &existing.VisitorID, &startedAtText, &lastSeenText,
		&existing.EventCount, &existing.Pageviews, &existing.Referrer, &existing.ReferrerDomain,
		&existing.UTMSource, &existing.UTMMedium, &existing.UTMCampaign, &existing.Browser,
		&existing.OS, &existing.Device, &existing.Country, &existing.Region, &existing.City,
		&existing.EntryPath, &existing.ExitPath,
	)
	if err == nil {
		existing.StartedAt = parseISO(startedAtText)
		existing.LastSeenAt = parseISO(lastSeenText)
		existing.PrevLastSeenAt = existing.LastSeenAt
		existing.PrevPageviews = existing.Pageviews
	}
	if err == nil && candidate.StartedAt.Sub(existing.LastSeenAt) <= 30*time.Minute {
		// 会话仍在 30 分钟窗口内，更新现有会话
		existing.SessionKey = sessionRollingKey(existing.WebsiteID, existing.VisitorID, existing.StartedAt)
		existing.LastSeenAt = candidate.LastSeenAt
		if candidate.ExitPath != "" {
			existing.ExitPath = candidate.ExitPath
		}
		existing.EventCount++
		if candidate.Pageviews > 0 {
			existing.Pageviews++
		}
		_, err := tx.Exec(`
			update sessions
			set last_seen_at = ?, event_count = ?, pageviews = ?, exit_path = ?
			where id = ?
		`, iso(existing.LastSeenAt), existing.EventCount, existing.Pageviews, existing.ExitPath, existing.ID)
		if err != nil {
			Error("update session failed: %v", err)
		}
		return existing, false, err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// 查询出错（非"未找到"错误）
		Error("query session failed: %v", err)
		return sessionRecord{}, false, err
	}

	// 创建新会话
	candidate.ID = newID()
	candidate.SessionKey = sessionRollingKey(candidate.WebsiteID, candidate.VisitorID, candidate.StartedAt)
	candidate.EventCount = 1
	if candidate.Pageviews > 0 {
		candidate.Pageviews = 1
	}
	_, err = tx.Exec(`
		insert into sessions(
			id, session_key, website_id, visitor_id, started_at, last_seen_at, event_count, pageviews,
			referrer, referrer_domain, utm_source, utm_medium, utm_campaign,
			browser, os, device, country, region, city, entry_path, exit_path
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		candidate.ID, candidate.SessionKey, candidate.WebsiteID, candidate.VisitorID, iso(candidate.StartedAt), iso(candidate.LastSeenAt),
		candidate.EventCount, candidate.Pageviews, candidate.Referrer, candidate.ReferrerDomain,
		candidate.UTMSource, candidate.UTMMedium, candidate.UTMCampaign, candidate.Browser, candidate.OS,
		candidate.Device, candidate.Country, candidate.Region, candidate.City, candidate.EntryPath, candidate.ExitPath,
	)
	if err != nil {
		Error("insert session failed: %v", err)
	}
	return candidate, true, err
}

// insertEventTx - 在事务中插入事件记录到 events 表
func (a *App) insertEventTx(tx *sql.Tx, record eventRecord) error {
	_, err := tx.Exec(`
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

// updateAggregatesTx - 在事务中更新聚合数据
// 根据事件类型和会话状态更新各类聚合表：
//   - agg_overview_daily: 概览聚合（PV、UV、会话数、跳出率、收入等）
//   - agg_pages_daily: 页面聚合
//   - agg_referrers_daily: 来源聚合
//   - agg_devices_daily: 设备聚合
//   - agg_geo_daily: 地理位置聚合
//   - agg_attribution_daily: 归因聚合
//   - agg_revenue_daily: 收入聚合
func (a *App) updateAggregatesTx(tx *sql.Tx, record eventRecord, session sessionRecord, isNewSession, _ bool) error {
	day := record.CreatedAt.Format("2006-01-02")

	// 计算页面浏览和自定义事件计数
	pageviews := 0
	customEvents := 0
	if record.EventType == "pageview" {
		pageviews = 1
	} else {
		customEvents = 1
	}

	// 计算访客、会话、跳出等增量
	visitorDelta := 0
	sessionDelta := 0
	bouncedSessionsDelta := 0
	sessionDurationDelta := int64(0)
	timeOnPageDelta := int64(0)
	timeOnPageSamplesDelta := 0

	if isNewSession {
		sessionDelta = 1
		if session.Pageviews == 1 {
			bouncedSessionsDelta = 1
		}
		// 尝试插入访客日聚合，如果已存在则忽略
		result, err := tx.Exec(`
			insert or ignore into agg_visitor_daily(website_id, bucket_date, visitor_id)
			values(?, ?, ?)
		`, record.WebsiteID, day, record.VisitorID)
		if err != nil {
			Error("insert agg_visitor_daily failed: %v", err)
			return err
		}
		if affected, err := result.RowsAffected(); err == nil && affected > 0 {
			visitorDelta = 1
		}
	} else {
		// 非新会话：计算跳出状态变化和会话时长增量
		if session.PrevPageviews == 1 && session.Pageviews > 1 {
			bouncedSessionsDelta = -1
		}
		durationDiff := session.LastSeenAt.Unix() - session.PrevLastSeenAt.Unix()
		if durationDiff > 0 {
			sessionDurationDelta = durationDiff
		}
	}

	// 处理页面停留时间事件（page_leave / page_ping）
	if record.EventName == "page_leave" || record.EventName == "page_ping" {
		var payload struct {
			DurationMS int64 `json:"duration_ms"`
		}
		if err := json.Unmarshal([]byte(record.Metadata), &payload); err == nil && payload.DurationMS > 0 {
			timeOnPageDelta = payload.DurationMS
			timeOnPageSamplesDelta = 1
		}
	}

	// 更新概览日聚合表
	if _, err := tx.Exec(`
		insert into agg_overview_daily(website_id, bucket_date, pageviews, custom_events, visitors, sessions, bounced_sessions, session_duration_total_seconds, time_on_page_total_ms, time_on_page_samples, revenue)
		values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(website_id, bucket_date) do update set
			pageviews = pageviews + excluded.pageviews,
			custom_events = custom_events + excluded.custom_events,
			visitors = visitors + excluded.visitors,
			sessions = sessions + excluded.sessions,
			bounced_sessions = bounced_sessions + excluded.bounced_sessions,
			session_duration_total_seconds = session_duration_total_seconds + excluded.session_duration_total_seconds,
			time_on_page_total_ms = time_on_page_total_ms + excluded.time_on_page_total_ms,
			time_on_page_samples = time_on_page_samples + excluded.time_on_page_samples,
			revenue = revenue + excluded.revenue
	`, record.WebsiteID, day, pageviews, customEvents, visitorDelta, sessionDelta, bouncedSessionsDelta, sessionDurationDelta, timeOnPageDelta, timeOnPageSamplesDelta, record.Amount); err != nil {
		Error("upsert agg_overview_daily failed: %v", err)
		return err
	}

	// 更新页面日聚合表
	if record.URLPath != "" && pageviews > 0 {
		if _, err := tx.Exec(`
			insert into agg_pages_daily(website_id, bucket_date, url_path, pageviews)
			values(?, ?, ?, 1)
			on conflict(website_id, bucket_date, url_path) do update set
				pageviews = pageviews + 1
		`, record.WebsiteID, day, record.URLPath); err != nil {
			Error("upsert agg_pages_daily failed: %v", err)
			return err
		}
	}

	if isNewSession {
		// 新会话：更新来源、设备、地理位置、归因聚合
		referrer := record.ReferrerDomain
		if referrer == "" {
			referrer = "(direct)"
		}
		if _, err := tx.Exec(`
			insert into agg_referrers_daily(website_id, bucket_date, referrer_domain, sessions, revenue)
			values(?, ?, ?, 1, ?)
			on conflict(website_id, bucket_date, referrer_domain) do update set
				sessions = sessions + 1,
				revenue = revenue + excluded.revenue
		`, record.WebsiteID, day, referrer, record.Amount); err != nil {
			Error("upsert agg_referrers_daily failed: %v", err)
			return err
		}

		if _, err := tx.Exec(`
			insert into agg_devices_daily(website_id, bucket_date, browser, os, device, sessions)
			values(?, ?, ?, ?, ?, 1)
			on conflict(website_id, bucket_date, browser, os, device) do update set
				sessions = sessions + 1
		`, record.WebsiteID, day, nullUnknown(record.Browser), nullUnknown(record.OS), nullUnknown(record.Device)); err != nil {
			Error("upsert agg_devices_daily failed: %v", err)
			return err
		}

		if _, err := tx.Exec(`
			insert into agg_geo_daily(website_id, bucket_date, country, region, city, sessions)
			values(?, ?, ?, ?, ?, 1)
			on conflict(website_id, bucket_date, country, region, city) do update set
				sessions = sessions + 1
		`, record.WebsiteID, day, nullUnknown(record.Country), nullUnknown(record.Region), nullUnknown(record.City)); err != nil {
			Error("upsert agg_geo_daily failed: %v", err)
			return err
		}

		source, medium, campaign := attributionKey(session)
		if _, err := tx.Exec(`
			insert into agg_attribution_daily(website_id, bucket_date, source, medium, campaign, sessions, revenue)
			values(?, ?, ?, ?, ?, 1, ?)
			on conflict(website_id, bucket_date, source, medium, campaign) do update set
				sessions = sessions + 1,
				revenue = revenue + excluded.revenue
		`, record.WebsiteID, day, source, medium, campaign, record.Amount); err != nil {
			Error("upsert agg_attribution_daily failed: %v", err)
			return err
		}
	} else if record.Amount > 0 {
		// 非新会话但有收入：更新来源和归因的收入数据
		referrer := session.ReferrerDomain
		if referrer == "" {
			referrer = "(direct)"
		}
		if _, err := tx.Exec(`
			insert into agg_referrers_daily(website_id, bucket_date, referrer_domain, sessions, revenue)
			values(?, ?, ?, 0, ?)
			on conflict(website_id, bucket_date, referrer_domain) do update set
				revenue = revenue + excluded.revenue
		`, record.WebsiteID, day, referrer, record.Amount); err != nil {
			Error("upsert agg_referrers_daily revenue failed: %v", err)
			return err
		}
		source, medium, campaign := attributionKey(session)
		if _, err := tx.Exec(`
			insert into agg_attribution_daily(website_id, bucket_date, source, medium, campaign, sessions, revenue)
			values(?, ?, ?, ?, ?, 0, ?)
			on conflict(website_id, bucket_date, source, medium, campaign) do update set
				revenue = revenue + excluded.revenue
		`, record.WebsiteID, day, source, medium, campaign, record.Amount); err != nil {
			Error("upsert agg_attribution_daily revenue failed: %v", err)
			return err
		}
	}

	// 更新收入日聚合表
	if record.Amount > 0 {
		source, _, _ := attributionKey(session)
		currency := record.Currency
		if currency == "" {
			currency = "N/A"
		}
		if _, err := tx.Exec(`
			insert into agg_revenue_daily(website_id, bucket_date, source, currency, event_count, revenue)
			values(?, ?, ?, ?, 1, ?)
			on conflict(website_id, bucket_date, source, currency) do update set
				event_count = event_count + 1,
				revenue = revenue + excluded.revenue
		`, record.WebsiteID, day, source, currency, record.Amount); err != nil {
			Error("upsert agg_revenue_daily failed: %v", err)
			return err
		}
	}

	return nil
}
