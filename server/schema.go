package main

import (
	"database/sql"
	"fmt"
	"strings"
)

const currentSchemaVersion = 1

type sqlQueryer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func (a *App) initSchema() error {
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema init: %w", err)
	}
	defer tx.Rollback()

	if err := ensureSchemaVersionTable(tx); err != nil {
		return err
	}
	if err := ensureBaseSchema(tx); err != nil {
		return err
	}

	version, err := loadSchemaVersion(tx)
	if err != nil {
		return err
	}
	for version < currentSchemaVersion {
		next := version + 1
		if err := applySchemaUpgrade(tx, next); err != nil {
			return err
		}
		if err := saveSchemaVersion(tx, next); err != nil {
			return err
		}
		version = next
	}
	if err := ensureSchemaIndexes(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureSchemaVersionTable(tx *sql.Tx) error {
	if _, err := tx.Exec(`
		create table if not exists schema_version (
			id integer primary key check (id = 1),
			version integer not null,
			updated_at text not null
		)
	`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}
	if _, err := tx.Exec(`
		insert into schema_version(id, version, updated_at)
		values(1, 0, ?)
		on conflict(id) do nothing
	`, iso(nowUTC())); err != nil {
		return fmt.Errorf("seed schema_version: %w", err)
	}
	return nil
}

func ensureBaseSchema(tx *sql.Tx) error {
	statements := []string{
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
			session_key text not null default '',
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
			unique (session_key),
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
			visitors integer not null default 0,
			sessions integer not null default 0,
			bounced_sessions integer not null default 0,
			session_duration_total_seconds integer not null default 0,
			time_on_page_total_ms integer not null default 0,
			time_on_page_samples integer not null default 0,
			revenue real not null default 0,
			primary key (website_id, bucket_date)
		);`,
		`create table if not exists agg_visitor_daily (
			website_id text not null,
			bucket_date text not null,
			visitor_id text not null,
			primary key (website_id, bucket_date, visitor_id)
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
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("apply base schema: %w", err)
		}
	}
	return nil
}

func loadSchemaVersion(tx *sql.Tx) (int, error) {
	var version int
	if err := tx.QueryRow(`select version from schema_version where id = 1`).Scan(&version); err != nil {
		return 0, fmt.Errorf("load schema version: %w", err)
	}
	return version, nil
}

func saveSchemaVersion(tx *sql.Tx, version int) error {
	if _, err := tx.Exec(`
		update schema_version
		set version = ?, updated_at = ?
		where id = 1
	`, version, iso(nowUTC())); err != nil {
		return fmt.Errorf("save schema version %d: %w", version, err)
	}
	return nil
}

func applySchemaUpgrade(tx *sql.Tx, version int) error {
	switch version {
	case 1:
		return upgradeSchemaV1(tx)
	default:
		return fmt.Errorf("unsupported schema version: %d", version)
	}
}

func upgradeSchemaV1(tx *sql.Tx) error {
	if !tableColumnExists(tx, "website_permissions", "access_level") {
		if _, err := tx.Exec(`alter table website_permissions add column access_level text not null default 'view'`); err != nil {
			return fmt.Errorf("migrate website_permissions access_level: %w", err)
		}
	}
	if tableColumnExists(tx, "website_permissions", "can_manage") {
		if _, err := tx.Exec(`
			update website_permissions
			set access_level = case
				when access_level is null or access_level = '' then
					case when can_manage = 1 then 'manage' else 'view' end
				else access_level
			end
		`); err != nil {
			return fmt.Errorf("backfill website_permissions access_level: %w", err)
		}
	}
	for _, column := range []string{"visitors", "sessions", "bounced_sessions", "session_duration_total_seconds", "time_on_page_total_ms", "time_on_page_samples"} {
		if !tableColumnExists(tx, "agg_overview_daily", column) {
			if _, err := tx.Exec(`alter table agg_overview_daily add column ` + column + ` integer not null default 0`); err != nil {
				return fmt.Errorf("migrate agg_overview_daily %s: %w", column, err)
			}
		}
	}
	if !tableColumnExists(tx, "sessions", "session_key") {
		if _, err := tx.Exec(`alter table sessions add column session_key text not null default ''`); err != nil {
			return fmt.Errorf("migrate sessions session_key: %w", err)
		}
	}
	if _, err := tx.Exec(`
		update sessions
		set session_key = website_id || ':' || visitor_id || ':' || strftime('%s', started_at)
		where session_key = ''
	`); err != nil {
		return fmt.Errorf("backfill sessions session_key: %w", err)
	}
	if _, err := tx.Exec(`
		insert into agg_overview_daily(website_id, bucket_date, visitors, sessions, bounced_sessions, session_duration_total_seconds)
		select
			website_id,
			date(started_at),
			count(distinct visitor_id),
			count(*),
			sum(case when pageviews = 1 then 1 else 0 end),
			sum(case when unixepoch(last_seen_at) - unixepoch(started_at) > 0 then unixepoch(last_seen_at) - unixepoch(started_at) else 0 end)
		from sessions
		group by website_id, date(started_at)
		on conflict(website_id, bucket_date) do update set
			visitors = excluded.visitors,
			sessions = excluded.sessions,
			bounced_sessions = excluded.bounced_sessions,
			session_duration_total_seconds = excluded.session_duration_total_seconds
	`); err != nil {
		return fmt.Errorf("backfill agg_overview_daily session metrics: %w", err)
	}
	if _, err := tx.Exec(`
		insert into agg_overview_daily(website_id, bucket_date, time_on_page_total_ms, time_on_page_samples)
		select
			website_id,
			date(created_at),
			coalesce(sum(cast(json_extract(metadata, '$.duration_ms') as integer)), 0),
			count(*)
		from events
		where event_name in ('page_leave', 'page_ping')
		group by website_id, date(created_at)
		on conflict(website_id, bucket_date) do update set
			time_on_page_total_ms = excluded.time_on_page_total_ms,
			time_on_page_samples = excluded.time_on_page_samples
	`); err != nil {
		return fmt.Errorf("backfill agg_overview_daily time on page: %w", err)
	}
	if _, err := tx.Exec(`
		insert or ignore into agg_visitor_daily(website_id, bucket_date, visitor_id)
		select website_id, date(started_at), visitor_id
		from sessions
	`); err != nil {
		return fmt.Errorf("backfill agg_visitor_daily: %w", err)
	}
	return nil
}

func ensureSchemaIndexes(tx *sql.Tx) error {
	statements := []string{
		`create index if not exists idx_sessions_website_started on sessions(website_id, started_at);`,
		`create index if not exists idx_sessions_website_visitor on sessions(website_id, visitor_id, last_seen_at);`,
		`create index if not exists idx_events_website_created on events(website_id, created_at);`,
		`create index if not exists idx_events_website_type_created on events(website_id, event_type, created_at);`,
		`create index if not exists idx_events_website_path_created on events(website_id, url_path, created_at);`,
		`create index if not exists idx_events_website_name_created on events(website_id, event_name, created_at);`,
		`create unique index if not exists idx_sessions_session_key on sessions(session_key) where session_key <> '';`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("apply schema index: %w", err)
		}
	}
	return nil
}

func tableColumnExists(q sqlQueryer, tableName, columnName string) bool {
	rows, err := q.Query(`pragma table_info(` + tableName + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false
		}
		if strings.EqualFold(name, columnName) {
			return true
		}
	}
	return false
}
