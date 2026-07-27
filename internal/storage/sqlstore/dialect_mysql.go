package sqlstore

// ─── MySQL ──────────────────────────────────────────────────────────────────
// MySQL can't index TEXT without a prefix and lacks CREATE INDEX IF NOT EXISTS,
// so keys use VARCHAR and indexes are declared inline in CREATE TABLE.

type mysqlDialect struct{}

func (mysqlDialect) Name() string           { return "mysql" }
func (mysqlDialect) DriverName() string     { return "mysql" }
func (mysqlDialect) Rebind(q string) string { return q }
func (mysqlDialect) Migrations() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id VARCHAR(191) PRIMARY KEY,
			user_id VARCHAR(191),
			interaction_count INT NOT NULL DEFAULT 0,
			sign_in_prompted TINYINT(1) NOT NULL DEFAULT 0,
			assistant_name VARCHAR(191) NOT NULL DEFAULT 'Leo',
			current_theme VARCHAR(64) NOT NULL DEFAULT 'sky',
			language VARCHAR(64) NOT NULL DEFAULT '',
			import_token VARCHAR(64) NOT NULL DEFAULT '',
			persona_prompt TEXT,
			preferences TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS day_plans (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			date VARCHAR(16) NOT NULL,
			blocks LONGTEXT NOT NULL,
			source_type VARCHAR(32) NOT NULL DEFAULT 'text',
			note TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY day_plans_session_date (session_id, date)
		)`,
		`CREATE TABLE IF NOT EXISTS mood_checkins (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			mood VARCHAR(191) NOT NULL,
			ai_response TEXT,
			exercise_offered VARCHAR(64),
			exercise_completed TINYINT(1) NOT NULL DEFAULT 0,
			theme VARCHAR(64),
			source VARCHAR(64) NOT NULL DEFAULT '',
			note TEXT,
			created_at BIGINT NOT NULL,
			KEY mood_session_created (session_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS feedback_logs (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			message_id VARCHAR(191) NOT NULL DEFAULT '',
			reason VARCHAR(512) NOT NULL DEFAULT '',
			useful TINYINT(1) NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			KEY feedback_session_created (session_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS companion_memory (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL UNIQUE,
			key_facts LONGTEXT NOT NULL,
			conversation_history LONGTEXT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS theme_switch_log (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			theme VARCHAR(64) NOT NULL,
			switched_at BIGINT NOT NULL,
			KEY theme_log_session (session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS operation_logs (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			actor VARCHAR(16) NOT NULL DEFAULT 'user',
			action VARCHAR(64) NOT NULL,
			domain VARCHAR(64) NOT NULL DEFAULT '',
			target_id VARCHAR(191) NOT NULL DEFAULT '',
			date VARCHAR(16) NOT NULL DEFAULT '',
			summary VARCHAR(1024) NOT NULL DEFAULT '',
			detail LONGTEXT NOT NULL,
			status VARCHAR(16) NOT NULL DEFAULT 'ok',
			request_id VARCHAR(64) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			KEY operation_logs_session_created (session_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id VARCHAR(191) PRIMARY KEY,
			email VARCHAR(191) UNIQUE,
			name VARCHAR(191),
			avatar_url TEXT,
			is_anonymous INTEGER NOT NULL DEFAULT 0,
			data_session_id VARCHAR(191) NOT NULL DEFAULT '',
			token_version INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS credentials (
			user_id VARCHAR(191) PRIMARY KEY,
			password_hash VARCHAR(255) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_identities (
			id VARCHAR(191) PRIMARY KEY,
			user_id VARCHAR(191) NOT NULL,
			provider VARCHAR(64) NOT NULL,
			provider_user_id VARCHAR(191) NOT NULL,
			created_at BIGINT NOT NULL,
			UNIQUE KEY oauth_provider_uid (provider, provider_user_id),
			KEY oauth_user (user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS prompts (
			prompt_key VARCHAR(128) PRIMARY KEY,
			content LONGTEXT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS prompt_overrides (
			prompt_key VARCHAR(128) NOT NULL,
			locale VARCHAR(16) NOT NULL,
			content LONGTEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (prompt_key, locale)
		)`,
		legacyPromptCopy,
		`CREATE TABLE IF NOT EXISTS schedule_rules (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			title TEXT NOT NULL,
			block_type VARCHAR(32) NOT NULL DEFAULT 'task',
			start_time VARCHAR(8),
			duration_min INT,
			timezone VARCHAR(64) NOT NULL DEFAULT '',
			time_mode VARCHAR(16) NOT NULL DEFAULT 'floating',
			kind VARCHAR(16) NOT NULL DEFAULT 'recurring',
			date VARCHAR(16),
			freq VARCHAR(16) NOT NULL DEFAULT '',
			interval_n INT NOT NULL DEFAULT 1,
			by_weekday VARCHAR(64) NOT NULL DEFAULT '[]',
			start_date VARCHAR(16) NOT NULL DEFAULT '',
			until_date VARCHAR(16),
			active TINYINT(1) NOT NULL DEFAULT 1,
			source VARCHAR(16) NOT NULL DEFAULT 'user',
			note TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			KEY schedule_rules_session (session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			canvas_id VARCHAR(191) NOT NULL,
			name TEXT NOT NULL,
			course_code VARCHAR(191) NOT NULL DEFAULT '',
			current_score DOUBLE,
			current_grade VARCHAR(16),
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY courses_session_canvas (session_id, canvas_id)
		)`,
		`CREATE TABLE IF NOT EXISTS assignments (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			course_id VARCHAR(191) NOT NULL DEFAULT '',
			canvas_id VARCHAR(191) NOT NULL,
			title TEXT NOT NULL,
			due_at BIGINT,
			points_possible DOUBLE,
			submitted TINYINT(1) NOT NULL DEFAULT 0,
			graded TINYINT(1) NOT NULL DEFAULT 0,
			score DOUBLE,
			html_url TEXT,
			source VARCHAR(16) NOT NULL DEFAULT 'canvas',
			status VARCHAR(16) NOT NULL DEFAULT 'pending',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY assignments_session_canvas (session_id, canvas_id),
			KEY assignments_session_due (session_id, due_at)
		)`,
		`CREATE TABLE IF NOT EXISTS custom_themes (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			name VARCHAR(191) NOT NULL,
			base VARCHAR(32) NOT NULL DEFAULT '',
			dark TINYINT(1) NOT NULL DEFAULT 0,
			variables LONGTEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			KEY custom_themes_session (session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS memory_facts (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			fact TEXT NOT NULL,
			source VARCHAR(16) NOT NULL DEFAULT 'chat',
			type VARCHAR(64) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			KEY memory_facts_session (session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS import_history (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			source VARCHAR(16) NOT NULL,
			items INT NOT NULL DEFAULT 0,
			summary TEXT,
			payload LONGTEXT,
			created_at BIGINT NOT NULL,
			KEY import_history_session (session_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS chat_threads (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			title VARCHAR(255) NOT NULL DEFAULT '',
			summary LONGTEXT NOT NULL,
			archived TINYINT(1) NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			KEY chat_threads_session (session_id, updated_at)
		)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id VARCHAR(191) PRIMARY KEY,
			thread_id VARCHAR(191) NOT NULL,
			session_id VARCHAR(191) NOT NULL,
			role VARCHAR(32) NOT NULL,
			content LONGTEXT NOT NULL,
			tool_events LONGTEXT NOT NULL,
			status VARCHAR(64) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			KEY chat_messages_thread (thread_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS ai_call_logs (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			endpoint VARCHAR(64) NOT NULL,
			model VARCHAR(128) NOT NULL,
			prompt_tokens INT NOT NULL DEFAULT 0,
			comp_tokens INT NOT NULL DEFAULT 0,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(16) NOT NULL DEFAULT 'ok',
			error TEXT NOT NULL,
			request_id VARCHAR(191) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			KEY ai_call_logs_session (session_id, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS channel_bindings (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			channel VARCHAR(64) NOT NULL,
			external_id VARCHAR(191) NOT NULL,
			display_name VARCHAR(255) NOT NULL DEFAULT '',
			metadata LONGTEXT NOT NULL,
			verified_at BIGINT,
			created_at BIGINT NOT NULL,
			KEY channel_bindings_session (session_id),
			UNIQUE KEY channel_bindings_channel_ext (channel, external_id)
			)`,
		`CREATE TABLE IF NOT EXISTS materials (
				id VARCHAR(191) PRIMARY KEY,
				session_id VARCHAR(191) NOT NULL,
				category VARCHAR(191) NOT NULL DEFAULT '',
				title TEXT,
				summary LONGTEXT,
				body LONGTEXT,
				source VARCHAR(191) NOT NULL DEFAULT '',
				mime_type VARCHAR(191) NOT NULL DEFAULT '',
				storage_ref TEXT,
				tags LONGTEXT,
				created_at BIGINT NOT NULL,
				updated_at BIGINT NOT NULL,
				KEY materials_session_cat (session_id, category)
			)`,
		`CREATE TABLE IF NOT EXISTS wishes (
				id VARCHAR(191) PRIMARY KEY,
				session_id VARCHAR(191) NOT NULL,
				title TEXT,
				note TEXT,
				effort_min INT,
				status VARCHAR(191) NOT NULL DEFAULT '',
				created_at BIGINT NOT NULL,
				updated_at BIGINT NOT NULL,
				KEY wishes_session (session_id)
			)`,
		`CREATE TABLE IF NOT EXISTS temp_contexts (
				id VARCHAR(191) PRIMARY KEY,
				session_id VARCHAR(191) NOT NULL,
				` + "`key`" + ` VARCHAR(191) NOT NULL,
				payload LONGTEXT,
				ttl BIGINT,
				created_at BIGINT NOT NULL,
				KEY temp_contexts_session_key (session_id, ` + "`key`" + `),
				UNIQUE KEY temp_contexts_session_key_unique (session_id, ` + "`key`" + `)
			)`,
	}
}

func (mysqlDialect) Quote(ident string) string { return "`" + ident + "`" }

func (mysqlDialect) ColumnMigrations() []ColumnMigration {
	return sessionColumnMigrations("VARCHAR(64)")
}

// ConditionalMigrations adds a FULLTEXT index over materials with the built-in
// ngram parser (CJK coverage). Building the index backfills existing rows.
func (mysqlDialect) ConditionalMigrations() []ConditionalMigration {
	return []ConditionalMigration{{
		Name:       "materials_fts",
		CheckQuery: `SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'materials' AND index_name = 'materials_fulltext' LIMIT 1`,
		DDLs: []string{
			`ALTER TABLE materials ADD FULLTEXT INDEX materials_fulltext (title, summary, body) WITH PARSER ngram`,
		},
	}}
}

func (mysqlDialect) ColumnExistsQuery() string {
	return `SELECT 1 FROM information_schema.columns
	 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`
}
