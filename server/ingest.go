package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"
)

const (
	eventBatchSize     = 512
	eventFlushInterval = 400 * time.Millisecond
)

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
			log.Printf("flush event batch: %v", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-a.workerCtx.Done():
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

func (a *App) flushEventBatch(batch []queuedEvent) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range batch {
		if err := a.applyQueuedEvent(tx, item); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *App) applyQueuedEvent(tx *sql.Tx, item queuedEvent) error {
	visitorID, isNewVisitor, err := a.upsertVisitorTx(tx, item.WebsiteID, item.VisitorKey, item.CreatedAt)
	if err != nil {
		return err
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
		EntryPath:      item.URLPath,
		ExitPath:       item.URLPath,
	})
	if err != nil {
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
		return err
	}
	return a.updateAggregatesTx(tx, record, session, isNewSession, isNewVisitor)
}

func (a *App) upsertVisitorTx(tx *sql.Tx, websiteID, externalID string, seenAt time.Time) (string, bool, error) {
	var visitorID string
	err := tx.QueryRow(`
		select id
		from visitors
		where website_id = ? and external_id = ?
	`, websiteID, externalID).Scan(&visitorID)
	switch {
	case err == nil:
		_, err = tx.Exec(`update visitors set last_seen_at = ? where id = ?`, iso(seenAt), visitorID)
		return visitorID, false, err
	case !errors.Is(err, sql.ErrNoRows):
		return "", false, err
	}
	visitorID = newID()
	_, err = tx.Exec(`
		insert into visitors(id, website_id, external_id, first_seen_at, last_seen_at)
		values(?, ?, ?, ?, ?)
	`, visitorID, websiteID, externalID, iso(seenAt), iso(seenAt))
	return visitorID, true, err
}

func (a *App) findOrCreateSessionTx(tx *sql.Tx, candidate sessionRecord) (sessionRecord, bool, error) {
	var existing sessionRecord
	var startedAtText, lastSeenText string
	row := tx.QueryRow(`
		select id, website_id, visitor_id, started_at, last_seen_at, event_count, pageviews,
		       referrer, referrer_domain, utm_source, utm_medium, utm_campaign,
		       browser, os, device, country, region, city, entry_path, exit_path
		from sessions
		where website_id = ? and visitor_id = ?
		order by last_seen_at desc
		limit 1
	`, candidate.WebsiteID, candidate.VisitorID)
	err := row.Scan(
		&existing.ID, &existing.WebsiteID, &existing.VisitorID, &startedAtText, &lastSeenText,
		&existing.EventCount, &existing.Pageviews, &existing.Referrer, &existing.ReferrerDomain,
		&existing.UTMSource, &existing.UTMMedium, &existing.UTMCampaign, &existing.Browser,
		&existing.OS, &existing.Device, &existing.Country, &existing.Region, &existing.City,
		&existing.EntryPath, &existing.ExitPath,
	)
	if err == nil {
		existing.StartedAt = parseISO(startedAtText)
		existing.LastSeenAt = parseISO(lastSeenText)
	}
	if err == nil && candidate.StartedAt.Sub(existing.LastSeenAt) <= 30*time.Minute {
		existing.LastSeenAt = candidate.LastSeenAt
		existing.ExitPath = candidate.ExitPath
		existing.EventCount++
		if candidate.URLPathLikePageview() {
			existing.Pageviews++
		}
		_, err := tx.Exec(`
			update sessions
			set last_seen_at = ?, event_count = ?, pageviews = ?, exit_path = ?
			where id = ?
		`, iso(existing.LastSeenAt), existing.EventCount, existing.Pageviews, existing.ExitPath, existing.ID)
		return existing, false, err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sessionRecord{}, false, err
	}

	candidate.ID = newID()
	candidate.EventCount = 1
	if candidate.EntryPath != "" {
		candidate.Pageviews = 1
	}
	_, err = tx.Exec(`
		insert into sessions(
			id, website_id, visitor_id, started_at, last_seen_at, event_count, pageviews,
			referrer, referrer_domain, utm_source, utm_medium, utm_campaign,
			browser, os, device, country, region, city, entry_path, exit_path
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		candidate.ID, candidate.WebsiteID, candidate.VisitorID, iso(candidate.StartedAt), iso(candidate.LastSeenAt),
		candidate.EventCount, candidate.Pageviews, candidate.Referrer, candidate.ReferrerDomain,
		candidate.UTMSource, candidate.UTMMedium, candidate.UTMCampaign, candidate.Browser, candidate.OS,
		candidate.Device, candidate.Country, candidate.Region, candidate.City, candidate.EntryPath, candidate.ExitPath,
	)
	return candidate, true, err
}

func (s sessionRecord) URLPathLikePageview() bool {
	return s.EntryPath != ""
}

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

func (a *App) updateAggregatesTx(tx *sql.Tx, record eventRecord, session sessionRecord, isNewSession, _ bool) error {
	day := record.CreatedAt.Format("2006-01-02")

	pageviews := 0
	customEvents := 0
	if record.EventType == "pageview" || record.EventType == "pixel" {
		pageviews = 1
	} else {
		customEvents = 1
	}
	if _, err := tx.Exec(`
		insert into agg_overview_daily(website_id, bucket_date, pageviews, custom_events, revenue)
		values(?, ?, ?, ?, ?)
		on conflict(website_id, bucket_date) do update set
			pageviews = pageviews + excluded.pageviews,
			custom_events = custom_events + excluded.custom_events,
			revenue = revenue + excluded.revenue
	`, record.WebsiteID, day, pageviews, customEvents, record.Amount); err != nil {
		return err
	}

	if record.URLPath != "" && pageviews > 0 {
		if _, err := tx.Exec(`
			insert into agg_pages_daily(website_id, bucket_date, url_path, pageviews)
			values(?, ?, ?, 1)
			on conflict(website_id, bucket_date, url_path) do update set
				pageviews = pageviews + 1
		`, record.WebsiteID, day, record.URLPath); err != nil {
			return err
		}
	}

	if isNewSession {
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
			return err
		}

		if _, err := tx.Exec(`
			insert into agg_devices_daily(website_id, bucket_date, browser, os, device, sessions)
			values(?, ?, ?, ?, ?, 1)
			on conflict(website_id, bucket_date, browser, os, device) do update set
				sessions = sessions + 1
		`, record.WebsiteID, day, nullUnknown(record.Browser), nullUnknown(record.OS), nullUnknown(record.Device)); err != nil {
			return err
		}

		if _, err := tx.Exec(`
			insert into agg_geo_daily(website_id, bucket_date, country, region, city, sessions)
			values(?, ?, ?, ?, ?, 1)
			on conflict(website_id, bucket_date, country, region, city) do update set
				sessions = sessions + 1
		`, record.WebsiteID, day, nullUnknown(record.Country), nullUnknown(record.Region), nullUnknown(record.City)); err != nil {
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
			return err
		}
	} else if record.Amount > 0 {
		referrer := record.ReferrerDomain
		if referrer == "" {
			referrer = "(direct)"
		}
		if _, err := tx.Exec(`
			update agg_referrers_daily set revenue = revenue + ?
			where website_id = ? and bucket_date = ? and referrer_domain = ?
		`, record.Amount, record.WebsiteID, day, referrer); err != nil {
			return err
		}
		source, medium, campaign := attributionKey(session)
		if _, err := tx.Exec(`
			update agg_attribution_daily set revenue = revenue + ?
			where website_id = ? and bucket_date = ? and source = ? and medium = ? and campaign = ?
		`, record.Amount, record.WebsiteID, day, source, medium, campaign); err != nil {
			return err
		}
	}

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
			return err
		}
	}

	return nil
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
