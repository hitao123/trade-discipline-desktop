package store

var migrationStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS accounts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		base_currency TEXT NOT NULL CHECK(base_currency = 'CNY'),
		initial_capital_fen INTEGER NOT NULL,
		enabled_at TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('active','inactive'))
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS one_active_account ON accounts(status) WHERE status = 'active'`,
	`CREATE TABLE IF NOT EXISTS rule_versions (
		id TEXT PRIMARY KEY,
		version INTEGER NOT NULL UNIQUE,
		snapshot_json TEXT NOT NULL CHECK(json_valid(snapshot_json)),
		change_reason TEXT NOT NULL,
		created_at TEXT NOT NULL,
		previous_id TEXT REFERENCES rule_versions(id)
	)`,
	`CREATE TABLE IF NOT EXISTS instruments (
		id TEXT PRIMARY KEY,
		market TEXT NOT NULL CHECK(market IN ('HK','SH','SZ')),
		code TEXT NOT NULL,
		name TEXT NOT NULL,
		asset_type TEXT NOT NULL CHECK(asset_type IN ('stock','etf')),
		currency TEXT NOT NULL CHECK(currency IN ('CNY','HKD')),
		lot_size INTEGER NOT NULL CHECK(lot_size > 0),
		lot_source TEXT NOT NULL,
		is_china_tech INTEGER NOT NULL DEFAULT 0,
		is_st INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		latest_price_minor INTEGER,
		latest_price_at TEXT,
		latest_price_source TEXT,
		UNIQUE(market, code)
	)`,
	`CREATE TABLE IF NOT EXISTS market_snapshots (
		id TEXT PRIMARY KEY,
		trade_date TEXT NOT NULL,
		ranking_kind TEXT NOT NULL CHECK(ranking_kind IN ('stock','etf')),
		source TEXT NOT NULL,
		fetched_at TEXT NOT NULL,
		status TEXT NOT NULL,
		version INTEGER NOT NULL,
		UNIQUE(trade_date, ranking_kind, version)
	)`,
	`CREATE TABLE IF NOT EXISTS market_rank_entries (
		id TEXT PRIMARY KEY,
		snapshot_id TEXT NOT NULL REFERENCES market_snapshots(id) ON DELETE CASCADE,
		instrument_id TEXT REFERENCES instruments(id),
		rank INTEGER NOT NULL,
		market TEXT NOT NULL,
		code TEXT NOT NULL,
		name TEXT NOT NULL,
		asset_type TEXT NOT NULL,
		close_minor INTEGER NOT NULL,
		change_bp INTEGER NOT NULL,
		turnover_fen INTEGER NOT NULL,
		trade_date TEXT NOT NULL,
		source_time TEXT,
		UNIQUE(snapshot_id, rank)
	)`,
	`CREATE TABLE IF NOT EXISTS market_daily_bar_observations (
		id TEXT PRIMARY KEY,
		market TEXT NOT NULL CHECK(market IN ('HK','SH','SZ')),
		code TEXT NOT NULL,
		trade_date TEXT NOT NULL,
		close_minor INTEGER NOT NULL CHECK(close_minor >= 0),
		turnover_fen INTEGER NOT NULL CHECK(turnover_fen >= 0),
		source TEXT NOT NULL,
		source_time TEXT NOT NULL,
		fetched_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL,
		UNIQUE(market, code, trade_date, close_minor, turnover_fen, source)
	)`,
	`CREATE INDEX IF NOT EXISTS market_daily_bar_latest ON market_daily_bar_observations(market, code, trade_date, fetched_at DESC)`,
	`CREATE TABLE IF NOT EXISTS market_metric_observations (
		id TEXT PRIMARY KEY,
		metric TEXT NOT NULL CHECK(metric IN ('ashare_turnover','southbound_net_buy','sh_turnover','sz_turnover','southbound_sh_net_buy','southbound_sz_net_buy')),
		trade_date TEXT NOT NULL,
		value_fen INTEGER NOT NULL,
		source TEXT NOT NULL,
		source_time TEXT NOT NULL,
		fetched_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL,
		UNIQUE(metric, trade_date, value_fen, source)
	)`,
	`CREATE INDEX IF NOT EXISTS market_metric_latest ON market_metric_observations(metric, trade_date, fetched_at DESC)`,
	`CREATE TABLE IF NOT EXISTS watchlist_items (
		id TEXT PRIMARY KEY,
		instrument_id TEXT NOT NULL REFERENCES instruments(id),
		source_type TEXT NOT NULL,
		source_snapshot_id TEXT REFERENCES market_snapshots(id),
		reason TEXT NOT NULL,
		added_at TEXT NOT NULL,
		status TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS trade_plans (
		id TEXT PRIMARY KEY,
		instrument_id TEXT NOT NULL REFERENCES instruments(id),
		rule_version_id TEXT NOT NULL REFERENCES rule_versions(id),
		status TEXT NOT NULL,
		draft_json TEXT NOT NULL CHECK(json_valid(draft_json)),
		validation_json TEXT NOT NULL CHECK(json_valid(validation_json)),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		valid_until TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS trade_plan_revisions (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL REFERENCES trade_plans(id),
		revision_number INTEGER NOT NULL,
		before_json TEXT NOT NULL CHECK(json_valid(before_json)),
		after_json TEXT NOT NULL CHECK(json_valid(after_json)),
		change_reason TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(plan_id, revision_number)
	)`,
	`CREATE TABLE IF NOT EXISTS execution_events (
		id TEXT PRIMARY KEY,
		original_event_id TEXT REFERENCES execution_events(id),
		event_type TEXT NOT NULL CHECK(event_type IN ('buy','sell','reversal')),
		plan_id TEXT REFERENCES trade_plans(id),
		instrument_id TEXT NOT NULL REFERENCES instruments(id),
		rule_version_id TEXT NOT NULL REFERENCES rule_versions(id),
		quantity INTEGER NOT NULL,
		local_price_minor INTEGER NOT NULL,
		local_amount_minor INTEGER NOT NULL,
		settlement_fen INTEGER NOT NULL,
		exit_code TEXT,
		evidence TEXT,
		broker_reference TEXT,
		reference_price_minor INTEGER,
		reference_price_at TEXT,
		emotion_json TEXT NOT NULL CHECK(json_valid(emotion_json)),
		executed_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS cash_events (
		id TEXT PRIMARY KEY,
		event_type TEXT NOT NULL,
		amount_fen INTEGER NOT NULL,
		reason TEXT NOT NULL,
		occurred_at TEXT NOT NULL,
		original_event_id TEXT REFERENCES cash_events(id)
	)`,
	`CREATE TABLE IF NOT EXISTS violation_events (
		id TEXT PRIMARY KEY,
		plan_id TEXT REFERENCES trade_plans(id),
		execution_id TEXT REFERENCES execution_events(id),
		rule_code TEXT NOT NULL,
		severity TEXT NOT NULL,
		fact_snapshot_json TEXT NOT NULL CHECK(json_valid(fact_snapshot_json)),
		action TEXT NOT NULL,
		occurred_at TEXT NOT NULL,
		acknowledged_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS cooldown_periods (
		id TEXT PRIMARY KEY,
		reason TEXT NOT NULL,
		severity TEXT NOT NULL,
		starts_at TEXT NOT NULL,
		expected_ends_at TEXT NOT NULL,
		actual_ends_at TEXT,
		allowed_actions_json TEXT NOT NULL CHECK(json_valid(allowed_actions_json))
	)`,
	`CREATE TABLE IF NOT EXISTS weekly_reviews (
		id TEXT PRIMARY KEY,
		period_start TEXT NOT NULL,
		period_end TEXT NOT NULL,
		auto_metrics_json TEXT NOT NULL CHECK(json_valid(auto_metrics_json)),
		user_content_json TEXT NOT NULL CHECK(json_valid(user_content_json)),
		discipline_score_bp INTEGER NOT NULL,
		submitted_at TEXT NOT NULL,
		rule_version_id TEXT NOT NULL REFERENCES rule_versions(id),
		UNIQUE(period_start, period_end)
	)`,
	`CREATE TABLE IF NOT EXISTS audit_events (
		id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		action TEXT NOT NULL,
		before_json TEXT,
		after_json TEXT,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS audit_entity_time ON audit_events(entity_type, entity_id, created_at)`,
	`CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value_json TEXT NOT NULL CHECK(json_valid(value_json)),
		updated_at TEXT NOT NULL
	)`,
}
