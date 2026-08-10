package sqlstore

import (
	"strconv"
	"strings"
)

// ─── PostgreSQL (pgx stdlib) ────────────────────────────────────────────────

import "fmt"

type postgresDialect struct{}

func (postgresDialect) Name() string       { return "postgres" }
func (postgresDialect) DriverName() string { return "pgx" }
func (postgresDialect) Rebind(q string) string {
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteByte(q[i])
		}
	}
	return b.String()
}

func (postgresDialect) Migrations() []string {
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
			persona_prompt TEXT,
			preferences TEXT,
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
		`CREATE TABLE IF NOT EXISTS feedback_logs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			message_id TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			useful INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS feedback_session_created ON feedback_logs(session_id, created_at)`,

		// ── multi-instance + derived-state tables (batch C) ─────────────────
		`CREATE TABLE IF NOT EXISTS proposals (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending',
			level TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			title TEXT,
			summary TEXT,
			reason TEXT,
			evidence TEXT,
			date TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL DEFAULT '',
			duration_min INTEGER,
			block_type TEXT NOT NULL DEFAULT '',
			lock_level TEXT NOT NULL DEFAULT '',
			lock_reason TEXT,
			rows_json TEXT,
			ops_json TEXT,
			applied_op_ids TEXT,
			accept_op_ids TEXT,
			merge_key TEXT NOT NULL DEFAULT '',
			deliver_after BIGINT,
			delivered_at BIGINT,
			pushed_at BIGINT,
			ttl_policy TEXT NOT NULL DEFAULT '',
			expires_at BIGINT NOT NULL,
			resolution TEXT NOT NULL DEFAULT '',
			origin TEXT NOT NULL DEFAULT '',
			thread_id TEXT NOT NULL DEFAULT '',
			owner_instance TEXT NOT NULL DEFAULT '',
			rev INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS leases (
			lease_name TEXT PRIMARY KEY,
			holder TEXT NOT NULL DEFAULT '',
			acquired_at BIGINT NOT NULL DEFAULT 0,
			expires_at BIGINT NOT NULL DEFAULT 0,
			fence BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS job_runs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			job_name TEXT NOT NULL,
			run_key TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT '',
			instance TEXT NOT NULL DEFAULT '',
			started_at BIGINT NOT NULL,
			ended_at BIGINT,
			attempts INTEGER NOT NULL DEFAULT 1,
			error_text TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS rapport_states (
			session_id TEXT PRIMARY KEY,
			scores_json TEXT,
			cursor_created_at BIGINT NOT NULL DEFAULT 0,
			cursor_id TEXT NOT NULL DEFAULT '',
			fold_version INTEGER NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS rhythm_profiles (
			session_id TEXT PRIMARY KEY,
			wake_hm TEXT NOT NULL DEFAULT '',
			sleep_hm TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'default',
			learned_days INTEGER NOT NULL DEFAULT 0,
			run_since BIGINT,
			last_signal_at BIGINT,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS rhythm_days (
			session_id TEXT NOT NULL,
			day TEXT NOT NULL,
			first_min INTEGER NOT NULL DEFAULT 0,
			last_min INTEGER NOT NULL DEFAULT 0,
			signals INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, day)
		)`,
		`CREATE TABLE IF NOT EXISTS locale_overrides (
			message_key TEXT NOT NULL,
			locale TEXT NOT NULL,
			content TEXT,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (message_key, locale)
		)`,
		`CREATE INDEX IF NOT EXISTS proposals_session_state ON proposals(session_id, state, expires_at)`,
		`CREATE INDEX IF NOT EXISTS proposals_sweep ON proposals(state, ttl_policy, expires_at)`,
		`CREATE INDEX IF NOT EXISTS proposals_session_merge ON proposals(session_id, merge_key)`,
		`CREATE INDEX IF NOT EXISTS proposals_session_date ON proposals(session_id, date)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS job_runs_occurrence ON job_runs(session_id, job_name, run_key)`,
		`CREATE INDEX IF NOT EXISTS job_runs_session_started ON job_runs(session_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS job_runs_status_started ON job_runs(status, started_at)`,
		`CREATE INDEX IF NOT EXISTS locale_overrides_locale ON locale_overrides(locale)`,
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
			is_anonymous INTEGER NOT NULL DEFAULT 0,
			data_session_id TEXT NOT NULL DEFAULT '',
			token_version INTEGER NOT NULL DEFAULT 0,
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
			current_score DOUBLE PRECISION,
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
			points_possible DOUBLE PRECISION,
			submitted INTEGER NOT NULL DEFAULT 0,
			graded INTEGER NOT NULL DEFAULT 0,
			score DOUBLE PRECISION,
			html_url TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'canvas',
			status TEXT NOT NULL DEFAULT 'pending',
			reminders_off INTEGER NOT NULL DEFAULT 0,
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
			type TEXT NOT NULL DEFAULT '',
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
		`CREATE TABLE IF NOT EXISTS settings (
			setting_key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS provider_overrides (
			kind TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			enabled BOOLEAN,
			base_url TEXT NOT NULL DEFAULT '',
			description_json TEXT NOT NULL DEFAULT '',
			description_hash TEXT NOT NULL DEFAULT '',
			approved BOOLEAN NOT NULL DEFAULT FALSE,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (kind, provider_id)
		)`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			thread_id TEXT NOT NULL DEFAULT '',
			message_id TEXT NOT NULL DEFAULT '',
			ref TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'file',
			mime TEXT NOT NULL DEFAULT '',
			size BIGINT NOT NULL DEFAULT 0,
			sha256 TEXT NOT NULL DEFAULT '',
			filename TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS attachments_message ON attachments(session_id, message_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS attachments_thread ON attachments(session_id, thread_id)`,
		`CREATE INDEX IF NOT EXISTS attachments_sweep ON attachments(message_id, created_at)`,
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

// migrationLockKey is an arbitrary constant identifying Daycore's schema lock.
// It only has to be stable and not collide with another application's advisory
// lock in the same database.
const migrationLockKey = 8410740326

// Postgres needs the lock: see Dialect.MigrationLock. Session-level rather than
// transaction-level, because Migrate is a sequence of separate statements with no
// transaction around them.
func (postgresDialect) MigrationLock() (acquire, release []string) {
	return []string{fmt.Sprintf("SELECT pg_advisory_lock(%d)", migrationLockKey)},
		[]string{fmt.Sprintf("SELECT pg_advisory_unlock(%d)", migrationLockKey)}
}

func (postgresDialect) NormalizeDSN(dsn string) string { return dsn }

func (postgresDialect) Quote(ident string) string { return `"` + ident + `"` }

func (postgresDialect) ColumnMigrations() []ColumnMigration {
	return sessionColumnMigrations("TEXT", "INTEGER")
}

// ConditionalMigrations adds a generated tsvector column + GIN index over
// materials (the generated column backfills existing rows automatically).
// 'simple' config: no language stemming, works acceptably across zh/en mixed
// content; requires PostgreSQL ≥ 12 (generated columns) — older servers just
// get a warning and keep the substring fallback.
func (postgresDialect) ConditionalMigrations() []ConditionalMigration {
	return []ConditionalMigration{{
		Name:       "materials_fts",
		CheckQuery: `SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'materials' AND column_name = 'fts'`,
		DDLs: []string{
			`ALTER TABLE materials ADD COLUMN fts tsvector GENERATED ALWAYS AS
				(to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(summary,'') || ' ' || coalesce(body,''))) STORED`,
			`CREATE INDEX IF NOT EXISTS materials_fts_gin ON materials USING GIN (fts)`,
		},
	}}
}
func (postgresDialect) ColumnExistsQuery() string {
	return `SELECT 1 FROM information_schema.columns
	 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`
}
