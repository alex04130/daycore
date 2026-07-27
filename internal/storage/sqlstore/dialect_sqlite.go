package sqlstore

// ─── SQLite (modernc.org/sqlite, pure Go, no CGO) ───────────────────────────

type sqliteDialect struct{}

func (sqliteDialect) Name() string           { return "sqlite" }
func (sqliteDialect) DriverName() string     { return "sqlite" }
func (sqliteDialect) Rebind(q string) string { return q }
func (sqliteDialect) Migrations() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			interaction_count INTEGER NOT NULL DEFAULT 0,
			sign_in_prompted INTEGER NOT NULL DEFAULT 0,
			assistant_name TEXT NOT NULL DEFAULT 'Leo',
			current_theme TEXT NOT NULL DEFAULT 'sky',
			language TEXT NOT NULL DEFAULT '',
			import_token TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS day_plans (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			date TEXT NOT NULL,
			blocks TEXT NOT NULL DEFAULT '[]',
			source_type TEXT NOT NULL DEFAULT 'text',
			note TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS day_plans_session_date ON day_plans(session_id, date)`,
		`CREATE TABLE IF NOT EXISTS mood_checkins (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			mood TEXT NOT NULL,
			ai_response TEXT,
			exercise_offered TEXT,
			exercise_completed INTEGER NOT NULL DEFAULT 0,
			theme TEXT,
			source TEXT NOT NULL DEFAULT '',
			note TEXT,
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS mood_session_created ON mood_checkins(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS companion_memory (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL UNIQUE,
			key_facts TEXT NOT NULL DEFAULT '[]',
			conversation_history TEXT NOT NULL DEFAULT '[]',
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS theme_switch_log (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			theme TEXT NOT NULL,
			switched_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS theme_log_session ON theme_switch_log(session_id)`,
		`CREATE TABLE IF NOT EXISTS operation_logs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			actor TEXT NOT NULL DEFAULT 'user',
			action TEXT NOT NULL,
			domain TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			date TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			request_id TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS operation_logs_session_created ON operation_logs(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE,
			name TEXT,
			avatar_url TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS credentials (
			user_id TEXT PRIMARY KEY,
			password_hash TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_identities (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			created_at BIGINT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS oauth_provider_uid ON oauth_identities(provider, provider_user_id)`,
		`CREATE INDEX IF NOT EXISTS oauth_user ON oauth_identities(user_id)`,
		`CREATE TABLE IF NOT EXISTS prompts (
			prompt_key TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS prompt_overrides (
			prompt_key TEXT NOT NULL,
			locale TEXT NOT NULL,
			content TEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (prompt_key, locale)
		)`,
		legacyPromptCopy,
		`CREATE TABLE IF NOT EXISTS schedule_rules (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			title TEXT NOT NULL,
			block_type TEXT NOT NULL DEFAULT 'task',
			start_time TEXT,
			duration_min INTEGER,
			timezone TEXT NOT NULL DEFAULT '',
			time_mode TEXT NOT NULL DEFAULT 'floating',
			kind TEXT NOT NULL DEFAULT 'recurring',
			date TEXT,
			freq TEXT NOT NULL DEFAULT '',
			interval_n INTEGER NOT NULL DEFAULT 1,
			by_weekday TEXT NOT NULL DEFAULT '[]',
			start_date TEXT NOT NULL DEFAULT '',
			until_date TEXT,
			active INTEGER NOT NULL DEFAULT 1,
			source TEXT NOT NULL DEFAULT 'user',
			note TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS schedule_rules_session ON schedule_rules(session_id)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			canvas_id TEXT NOT NULL,
			name TEXT NOT NULL,
			course_code TEXT NOT NULL DEFAULT '',
			current_score REAL,
			current_grade TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS courses_session_canvas ON courses(session_id, canvas_id)`,
		`CREATE TABLE IF NOT EXISTS assignments (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			course_id TEXT NOT NULL DEFAULT '',
			canvas_id TEXT NOT NULL,
			title TEXT NOT NULL,
			due_at BIGINT,
			points_possible REAL,
			submitted INTEGER NOT NULL DEFAULT 0,
			graded INTEGER NOT NULL DEFAULT 0,
			score REAL,
			html_url TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'canvas',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS assignments_session_canvas ON assignments(session_id, canvas_id)`,
		`CREATE INDEX IF NOT EXISTS assignments_session_due ON assignments(session_id, due_at)`,
		`CREATE TABLE IF NOT EXISTS custom_themes (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			name TEXT NOT NULL,
			base TEXT NOT NULL DEFAULT '',
			dark INTEGER NOT NULL DEFAULT 0,
			variables TEXT NOT NULL DEFAULT '{}',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS custom_themes_session ON custom_themes(session_id)`,
		`CREATE TABLE IF NOT EXISTS memory_facts (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			fact TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'chat',
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS memory_facts_session ON memory_facts(session_id)`,
		`CREATE TABLE IF NOT EXISTS import_history (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			source TEXT NOT NULL,
			items INTEGER NOT NULL DEFAULT 0,
			summary TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS import_history_session ON import_history(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS chat_threads (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			archived INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS chat_threads_session ON chat_threads(session_id, updated_at)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			tool_events TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS chat_messages_thread ON chat_messages(thread_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS ai_call_logs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			model TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			comp_tokens INTEGER NOT NULL DEFAULT 0,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'ok',
			error TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ai_call_logs_session ON ai_call_logs(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS channel_bindings (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			channel TEXT NOT NULL,
			external_id TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '{}',
			verified_at BIGINT,
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS channel_bindings_session ON channel_bindings(session_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS channel_bindings_channel_ext ON channel_bindings(channel, external_id)`,
		`CREATE TABLE IF NOT EXISTS materials (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				category TEXT NOT NULL DEFAULT '',
				title TEXT,
				summary TEXT,
				body TEXT,
				source TEXT NOT NULL DEFAULT '',
				mime_type TEXT NOT NULL DEFAULT '',
				storage_ref TEXT,
				tags TEXT,
				created_at BIGINT NOT NULL,
				updated_at BIGINT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS materials_session_cat ON materials(session_id, category)`,
		`CREATE TABLE IF NOT EXISTS wishes (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				title TEXT,
				note TEXT,
				effort_min INTEGER,
				status TEXT NOT NULL DEFAULT '',
				created_at BIGINT NOT NULL,
				updated_at BIGINT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS wishes_session ON wishes(session_id)`,
		`CREATE TABLE IF NOT EXISTS temp_contexts (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				key TEXT NOT NULL,
				payload TEXT,
				ttl BIGINT,
				created_at BIGINT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS temp_contexts_session_key ON temp_contexts(session_id, key)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS temp_contexts_session_key_unique ON temp_contexts(session_id, key)`,
	}
}

func (sqliteDialect) ColumnMigrations() []ColumnMigration { return sessionColumnMigrations("TEXT") }

// ConditionalMigrations builds the FTS5 external-content index over materials.
// trigram tokenizer: CJK-friendly substring-ish matching (unicode61 would treat
// a whole Chinese sentence as one token). The triggers keep it in sync; the
// final 'rebuild' backfills pre-existing rows exactly once (creation time).
func (sqliteDialect) ConditionalMigrations() []ConditionalMigration {
	return []ConditionalMigration{{
		Name:       "materials_fts",
		CheckQuery: `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'materials_fts'`,
		DDLs: []string{
			`CREATE VIRTUAL TABLE materials_fts USING fts5(
				title, summary, body,
				content='materials', content_rowid='rowid', tokenize='trigram')`,
			`CREATE TRIGGER materials_fts_ai AFTER INSERT ON materials BEGIN
				INSERT INTO materials_fts(rowid, title, summary, body)
				VALUES (new.rowid, new.title, new.summary, new.body);
			END`,
			`CREATE TRIGGER materials_fts_ad AFTER DELETE ON materials BEGIN
				INSERT INTO materials_fts(materials_fts, rowid, title, summary, body)
				VALUES ('delete', old.rowid, old.title, old.summary, old.body);
			END`,
			`CREATE TRIGGER materials_fts_au AFTER UPDATE ON materials BEGIN
				INSERT INTO materials_fts(materials_fts, rowid, title, summary, body)
				VALUES ('delete', old.rowid, old.title, old.summary, old.body);
				INSERT INTO materials_fts(rowid, title, summary, body)
				VALUES (new.rowid, new.title, new.summary, new.body);
			END`,
			`INSERT INTO materials_fts(materials_fts) VALUES ('rebuild')`,
		},
	}}
}
func (sqliteDialect) ColumnExistsQuery() string {
	return `SELECT 1 FROM pragma_table_info(?) WHERE name = ?`
}
