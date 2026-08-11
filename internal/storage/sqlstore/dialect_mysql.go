package sqlstore

// ─── MySQL ──────────────────────────────────────────────────────────────────
// MySQL can't index TEXT without a prefix and lacks CREATE INDEX IF NOT EXISTS,
// so keys use VARCHAR and indexes are declared inline in CREATE TABLE.

import (
	"daycore/internal/domain"

	mysqldriver "github.com/go-sql-driver/mysql"
)

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
			usage_fast_calls BIGINT NOT NULL DEFAULT 0,
			usage_fast_tokens BIGINT NOT NULL DEFAULT 0,
			usage_fast_start BIGINT NOT NULL DEFAULT 0,
			usage_slow_calls BIGINT NOT NULL DEFAULT 0,
			usage_slow_tokens BIGINT NOT NULL DEFAULT 0,
			usage_slow_start BIGINT NOT NULL DEFAULT 0,
			usage_calls BIGINT NOT NULL DEFAULT 0,
			usage_prompt_tokens BIGINT NOT NULL DEFAULT 0,
			usage_comp_tokens BIGINT NOT NULL DEFAULT 0,
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

		// ── multi-instance + derived-state tables (batch C) ─────────────────
		`CREATE TABLE IF NOT EXISTS proposals (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			state VARCHAR(64) NOT NULL DEFAULT 'pending',
			level VARCHAR(64) NOT NULL DEFAULT '',
			kind VARCHAR(64) NOT NULL DEFAULT '',
			title TEXT,
			summary TEXT,
			reason TEXT,
			evidence TEXT,
			date VARCHAR(64) NOT NULL DEFAULT '',
			start_time VARCHAR(64) NOT NULL DEFAULT '',
			duration_min INT,
			block_type VARCHAR(64) NOT NULL DEFAULT '',
			lock_level VARCHAR(64) NOT NULL DEFAULT '',
			lock_reason TEXT,
			rows_json LONGTEXT,
			ops_json LONGTEXT,
			applied_op_ids LONGTEXT,
			accept_op_ids LONGTEXT,
			merge_key VARCHAR(191) NOT NULL DEFAULT '',
			deliver_after BIGINT,
			delivered_at BIGINT,
			pushed_at BIGINT,
			ttl_policy VARCHAR(64) NOT NULL DEFAULT '',
			expires_at BIGINT NOT NULL,
			resolution VARCHAR(64) NOT NULL DEFAULT '',
			origin VARCHAR(64) NOT NULL DEFAULT '',
			thread_id VARCHAR(191) NOT NULL DEFAULT '',
			owner_instance VARCHAR(191) NOT NULL DEFAULT '',
			rev INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			KEY proposals_session_state (session_id, state, expires_at),
			KEY proposals_sweep (state, ttl_policy, expires_at),
			KEY proposals_session_merge (session_id, merge_key),
			KEY proposals_session_date (session_id, date)
		)`,
		`CREATE TABLE IF NOT EXISTS leases (
			lease_name VARCHAR(64) PRIMARY KEY,
			holder VARCHAR(191) NOT NULL DEFAULT '',
			acquired_at BIGINT NOT NULL DEFAULT 0,
			expires_at BIGINT NOT NULL DEFAULT 0,
			fence BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS job_runs (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			job_name VARCHAR(64) NOT NULL,
			run_key VARCHAR(64) NOT NULL,
			status VARCHAR(64) NOT NULL DEFAULT '',
			instance VARCHAR(191) NOT NULL DEFAULT '',
			started_at BIGINT NOT NULL,
			ended_at BIGINT,
			attempts INT NOT NULL DEFAULT 1,
			error_text TEXT,
			UNIQUE KEY job_runs_occurrence (session_id, job_name, run_key),
			KEY job_runs_session_started (session_id, started_at),
			KEY job_runs_status_started (status, started_at)
		)`,
		`CREATE TABLE IF NOT EXISTS rapport_states (
			session_id VARCHAR(191) PRIMARY KEY,
			scores_json LONGTEXT,
			cursor_created_at BIGINT NOT NULL DEFAULT 0,
			cursor_id VARCHAR(191) NOT NULL DEFAULT '',
			fold_version INT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS rhythm_profiles (
			session_id VARCHAR(191) PRIMARY KEY,
			wake_hm VARCHAR(64) NOT NULL DEFAULT '',
			sleep_hm VARCHAR(64) NOT NULL DEFAULT '',
			source VARCHAR(64) NOT NULL DEFAULT 'default',
			learned_days INT NOT NULL DEFAULT 0,
			run_since BIGINT,
			last_signal_at BIGINT,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS rhythm_days (
			session_id VARCHAR(191) NOT NULL,
			day VARCHAR(64) NOT NULL,
			first_min INT NOT NULL DEFAULT 0,
			last_min INT NOT NULL DEFAULT 0,
			signals INT NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, day)
		)`,
		`CREATE TABLE IF NOT EXISTS ai_usage_daily (
			day VARCHAR(10) NOT NULL,
			model VARCHAR(191) NOT NULL,
			endpoint VARCHAR(64) NOT NULL,
			calls BIGINT NOT NULL DEFAULT 0,
			errors BIGINT NOT NULL DEFAULT 0,
			prompt_tokens BIGINT NOT NULL DEFAULT 0,
			comp_tokens BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (day, model, endpoint)
		)`,
		`CREATE TABLE IF NOT EXISTS pairings (
			id VARCHAR(191) PRIMARY KEY,
			name VARCHAR(191) NOT NULL,
			description LONGTEXT,
			secret_hash VARCHAR(191) NOT NULL,
			roles_json LONGTEXT,
			full_access INTEGER NOT NULL DEFAULT 0,
			last_seen_at BIGINT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS frontend_families (
			id VARCHAR(191) PRIMARY KEY,
			display_name VARCHAR(191),
			tokens_json LONGTEXT,
			rules LONGTEXT,
			rules_accepted INTEGER NOT NULL DEFAULT 0,
			pinned INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS frontend_builds (
			build_hash VARCHAR(191) PRIMARY KEY,
			family_id VARCHAR(191) NOT NULL,
			display_name VARCHAR(191),
			build_version VARCHAR(64),
			min_api BIGINT NOT NULL DEFAULT 0,
			first_seen_at BIGINT NOT NULL,
			last_seen_at BIGINT NOT NULL,
			KEY frontend_builds_family (family_id)
		)`,
		`CREATE TABLE IF NOT EXISTS locale_overrides (
			message_key VARCHAR(191) NOT NULL,
			locale VARCHAR(64) NOT NULL,
			content LONGTEXT,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (message_key, locale),
			KEY locale_overrides_locale (locale)
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
			family_id VARCHAR(64) NOT NULL DEFAULT '` + domain.FallbackFamilyID + `',
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
			is_owner TINYINT(1) NOT NULL DEFAULT 0,
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
			reminders_off TINYINT(1) NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY assignments_session_canvas (session_id, canvas_id),
			KEY assignments_session_due (session_id, due_at)
		)`,
		`CREATE TABLE IF NOT EXISTS custom_themes (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			family_id VARCHAR(64) NOT NULL DEFAULT '` + domain.FallbackFamilyID + `',
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
		`CREATE TABLE IF NOT EXISTS settings (
			setting_key VARCHAR(191) PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS roles (
			name VARCHAR(64) PRIMARY KEY,
			description TEXT,
			permissions_json TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS role_members (
			role_name VARCHAR(64) NOT NULL,
			user_id VARCHAR(191) NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (role_name, user_id),
			KEY role_members_user (user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS provider_overrides (
			kind VARCHAR(32) NOT NULL,
			provider_id VARCHAR(128) NOT NULL,
			enabled TINYINT(1),
			base_url TEXT,
			description_json TEXT,
			description_hash VARCHAR(64) NOT NULL DEFAULT '',
			approved TINYINT(1) NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (kind, provider_id)
		)`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id VARCHAR(191) PRIMARY KEY,
			session_id VARCHAR(191) NOT NULL,
			thread_id VARCHAR(191) NOT NULL DEFAULT '',
			message_id VARCHAR(191) NOT NULL DEFAULT '',
			ref TEXT NOT NULL,
			kind VARCHAR(32) NOT NULL DEFAULT 'file',
			mime VARCHAR(191) NOT NULL DEFAULT '',
			size BIGINT NOT NULL DEFAULT 0,
			sha256 VARCHAR(64) NOT NULL DEFAULT '',
			filename VARCHAR(255) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			KEY attachments_message (session_id, message_id, created_at),
			KEY attachments_thread (session_id, thread_id),
			KEY attachments_sweep (message_id, created_at)
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

// NormalizeDSN forces clientFoundRows=true.
//
// Without it MySQL reports rows CHANGED from an UPDATE, where SQLite and
// Postgres both report rows MATCHED. Twenty-three places in this package branch
// on RowsAffected() — every UPDATE-then-INSERT upsert, and three that turn a
// zero into ErrNotFound — so on MySQL, saving a theme or a rule without altering
// a single value returns "not found" to the user, and an upsert that rewrites
// identical content falls through to an INSERT that then violates its own unique
// key.
//
// It is enforced here rather than written into the documented DSN because the
// DSN is operator-supplied: a deployment that copied the string from an older
// README would silently get the broken semantics back.
// MySQL's metadata locking makes a concurrent identical CREATE TABLE IF NOT
// EXISTS a no-op rather than an error, so no explicit lock is needed. Using
// GET_LOCK would also tie the lock to a connection this pool may hand back
// between statements.
func (mysqlDialect) MigrationLock() (acquire, release []string) { return nil, nil }

func (mysqlDialect) NormalizeDSN(dsn string) string {
	// Parse and re-format with the driver rather than doing string surgery.
	//
	// A MySQL DSN's parameter list begins at the first '?' AFTER the last '/',
	// and a password may legally contain '?' — the driver's own README says
	// escaping is unnecessary. Splitting on the first '?' anywhere therefore
	// appends the parameter to the DATABASE NAME for a password like "pa?ss",
	// and the server then fails to boot with "Unknown database
	// 'daycore&clientfoundrows=true'". Parsing is exact by construction.
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		// Let sql.Open surface the real parse error instead of masking it with a
		// mangled string.
		return dsn
	}
	cfg.ClientFoundRows = true
	return cfg.FormatDSN()
}

func (mysqlDialect) Quote(ident string) string { return "`" + ident + "`" }

func (mysqlDialect) ColumnMigrations() []ColumnMigration {
	return sessionColumnMigrations("VARCHAR(64)", "TINYINT(1)")
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
