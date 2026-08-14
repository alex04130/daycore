// Package mongostore implements domain.Store on MongoDB. It satisfies the exact
// same contract as the SQL store, proving the database/interface separation:
// the rest of the app is identical regardless of engine. Registered as "mongodb".
package mongostore

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/storage"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	storage.Register("mongodb", func(dsn string) (domain.Store, error) { return Open(dsn) })
}

// Store is the MongoDB-backed domain.Store.
type Store struct {
	client *mongo.Client
	db     *mongo.Database
}

// Open connects to MongoDB. The database name is taken from the URI path
// (mongodb://host:27017/<db>), defaulting to "daycore".
func Open(dsn string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(dsn))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return &Store{client: client, db: client.Database(dbNameFromDSN(dsn))}, nil
}

// DefaultDatabase is used when the connection string names none.
const DefaultDatabase = "daycore"

// dbNameFromDSN reads the default database out of the connection string with a
// real URL parser rather than slicing at the first '/'.
//
// The hand-rolled version cut at the first '/' after "://", which is the same
// mistake the MySQL DSN handling made: a password may contain characters that
// look like structure. `mongodb://user:p/ss@host/mydb` came back with a database
// named "ss@host/mydb", and the server would then quietly create and use it. The
// spec requires such characters to be percent-encoded, so the input is invalid —
// but silently connecting to a nonsense database is the worst way to react to
// invalid input — net/url refuses it, and refusing is what a caller can act on.
//
// net/url rather than the driver's connstring parser: that one resolves SRV
// records for mongodb+srv URIs, so using it here would add a DNS round trip to
// every Open and fail outright when a well-formed +srv URI cannot be resolved
// from where the test runs. net/url is exact for every form that matters —
// multi-host seed lists, mongodb+srv, options with no database, userinfo with
// percent-encoded separators — and touches no network.
func dbNameFromDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return DefaultDatabase
	}
	if db := strings.TrimPrefix(u.Path, "/"); db != "" {
		return db
	}
	return DefaultDatabase
}

func (s *Store) c(name string) *mongo.Collection { return s.db.Collection(name) }

// Repository accessors.
func (s *Store) Sessions() domain.SessionRepository               { return sessionRepo{s} }
func (s *Store) DayPlans() domain.DayPlanRepository               { return dayPlanRepo{s} }
func (s *Store) Moods() domain.MoodRepository                     { return moodRepo{s} }
func (s *Store) Companion() domain.CompanionRepository            { return companionRepo{s} }
func (s *Store) ThemeLog() domain.ThemeLogRepository              { return themeLogRepo{s} }
func (s *Store) Chats() domain.ChatRepository                     { return chatRepo{s} }
func (s *Store) AILogs() domain.AICallLogRepository               { return aiLogRepo{s} }
func (s *Store) OpLogs() domain.OperationLogRepository            { return opLogRepo{s} }
func (s *Store) Users() domain.UserRepository                     { return userRepo{s} }
func (s *Store) Auth() domain.AuthRepository                      { return authRepo{s} }
func (s *Store) Prompts() domain.PromptRepository                 { return promptRepo{s} }
func (s *Store) Rules() domain.RuleRepository                     { return ruleRepo{s} }
func (s *Store) Courses() domain.CourseRepository                 { return courseRepo{s} }
func (s *Store) Assignments() domain.AssignmentRepository         { return assignmentRepo{s} }
func (s *Store) Themes() domain.ThemeRepository                   { return themeRepo{s} }
func (s *Store) Memory() domain.MemoryRepository                  { return memoryRepo{s} }
func (s *Store) Materials() domain.MaterialRepository             { return materialRepo{s} }
func (s *Store) ChannelBindings() domain.ChannelBindingRepository { return channelBindingRepo{s} }
func (s *Store) Feedback() domain.FeedbackLogRepository           { return feedbackRepo{s} }
func (s *Store) TempContexts() domain.TempContextRepository       { return tempContextRepo{s} }
func (s *Store) Wishes() domain.WishRepository                    { return wishRepo{s} }
func (s *Store) WeeklyLetters() domain.WeeklyLetterRepository     { return weeklyLetterRepo{s} }
func (s *Store) River() domain.RiverRepository                    { return riverRepo{s} }

// Batch C — multi-instance coordination and cached derivations.
func (s *Store) Proposals() domain.ProposalRepository { return proposalRepo{s} }
func (s *Store) Leases() domain.LeaseRepository       { return leaseRepo{s} }
func (s *Store) JobRuns() domain.JobRunRepository     { return jobRunRepo{s} }
func (s *Store) Rapport() domain.RapportRepository    { return rapportRepo{s} }
func (s *Store) Rhythm() domain.RhythmRepository      { return rhythmRepo{s} }
func (s *Store) Locales() domain.LocaleRepository     { return localeRepo{s} }

func (s *Store) ThemeKinds() domain.ThemeKindRepository { return themeKindRepo{s} }

// ε — the ownership half of the file bus.
func (s *Store) Attachments() domain.AttachmentRepository { return attachmentRepo{s} }

// θ-F4b — the runtime half of configuration layering.
func (s *Store) Settings() domain.SettingRepository { return settingRepo{s} }
func (s *Store) Roles() domain.RoleRepository       { return roleRepo{s} }

func (s *Store) Frontends() domain.FrontendRepository { return frontendRepo{s} }

func (s *Store) Pairings() domain.PairingRepository { return pairingRepo{s} }

// Browser is the console's collection window — see domain/tables.go.
func (s *Store) Browser() domain.Browser { return browserRepo{s} }
func (s *Store) ProviderOverrides() domain.ProviderOverrideRepository {
	return providerOverrideRepo{s}
}

func (s *Store) Ping(ctx context.Context) error { return s.client.Ping(ctx, nil) }
func (s *Store) Close() error                   { return s.client.Disconnect(context.Background()) }

// Migrate creates the indexes (collections are created lazily by Mongo).
func (s *Store) Migrate(ctx context.Context) error {
	uniq := options.Index().SetUnique(true)
	specs := []struct {
		coll  string
		model mongo.IndexModel
	}{
		{"day_plans", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "date", Value: 1}}, Options: uniq}},
		{"mood_checkins", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "created_at", Value: -1}}}},
		{"companion_memory", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}}, Options: uniq}},
		{"theme_switch_log", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}}}},
		{"operation_logs", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "created_at", Value: -1}}}},
		{"users", mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)}},
		{"oauth_identities", mongo.IndexModel{Keys: bson.D{{Key: "provider", Value: 1}, {Key: "provider_user_id", Value: 1}}, Options: uniq}},
		{"schedule_rules", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}}}},
		{"custom_themes", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}}}},
		{"memory_facts", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}}}},
		{"import_history", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "created_at", Value: -1}}}},
		{"courses", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "canvas_id", Value: 1}}, Options: uniq}},
		{"assignments", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "canvas_id", Value: 1}}, Options: uniq}},
		{"prompt_overrides", mongo.IndexModel{Keys: bson.D{{Key: "prompt_key", Value: 1}, {Key: "locale", Value: 1}}, Options: uniq}},
		{"channel_bindings", mongo.IndexModel{Keys: bson.D{{Key: "channel", Value: 1}, {Key: "external_id", Value: 1}}, Options: uniq}},
		{"feedback_logs", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "created_at", Value: -1}}}},
		{"temp_contexts", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "key", Value: 1}}, Options: uniq}},
		{"materials", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "updated_at", Value: -1}}}},
		{"materials", mongo.IndexModel{Keys: bson.D{{Key: "title", Value: "text"}, {Key: "summary", Value: "text"}, {Key: "body", Value: "text"}}}},
		{"wishes", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "status", Value: 1}}}},

		// Batch C. These mirror the SQL indexes one for one; a uniqueness
		// invariant that holds on three engines and not the fourth is not an
		// invariant (dialect_parity_test.go enforces the SQL half).
		{"proposals", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "state", Value: 1}, {Key: "expires_at", Value: 1}}}},
		{"proposals", mongo.IndexModel{Keys: bson.D{{Key: "state", Value: 1}, {Key: "ttl_policy", Value: 1}, {Key: "expires_at", Value: 1}}}},
		{"proposals", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "merge_key", Value: 1}}}},
		{"proposals", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "date", Value: 1}}}},
		// job_runs needs no unique index over (session_id, job_name, run_key):
		// _id IS that tuple, so the primary key already is the mutual exclusion
		// Claim relies on. A second index over the same thing would cost writes
		// and enforce nothing new.
		{"job_runs", mongo.IndexModel{Keys: bson.D{{Key: "claim_id", Value: 1}}}},
		{"job_runs", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "started_at", Value: -1}}}},
		{"job_runs", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "started_at", Value: 1}}}},
		{"rhythm_days", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "day", Value: -1}}}},
		{"locale_overrides", mongo.IndexModel{Keys: bson.D{{Key: "message_key", Value: 1}, {Key: "locale", Value: 1}}, Options: uniq}},
		{"locale_overrides", mongo.IndexModel{Keys: bson.D{{Key: "locale", Value: 1}}}},

		// ε. Mirrors the three SQL indexes: hydrate a page of messages, delete a
		// thread's bytes, sweep uploads nobody sent.
		{"attachments", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "message_id", Value: 1}, {Key: "created_at", Value: 1}}}},
		{"attachments", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "thread_id", Value: 1}}}},
		{"attachments", mongo.IndexModel{Keys: bson.D{{Key: "message_id", Value: 1}, {Key: "created_at", Value: 1}}}},
	}
	for _, sp := range specs {
		if _, err := s.c(sp.coll).Indexes().CreateOne(ctx, sp.model); err != nil {
			return err
		}
	}
	return s.backfillFamilyID(ctx)
}

// ⚠️ The one migration Mongo does NOT get for free.
//
// On all three SQL engines, adding `family_id NOT NULL DEFAULT 'default'` fills
// existing rows with it as part of the ALTER. Mongo has no such rule: a document
// written before the field existed simply has no field, and `{"family_id":
// "default"}` does not match it. So every theme anybody made before this batch
// would VANISH from the list — not error, not warn, just be gone, on exactly one
// of the four backends.
//
// Idempotent by construction (`$exists: false` matches nothing on the second
// run), so it costs one indexed-miss scan per boot and nothing else.
func (s *Store) backfillFamilyID(ctx context.Context) error {
	for _, coll := range []string{"custom_themes", "theme_switch_log"} {
		if _, err := s.c(coll).UpdateMany(ctx,
			bson.M{"family_id": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"family_id": domain.FallbackFamilyID}},
		); err != nil {
			return err
		}
	}
	return nil
}

func notFound(err error) bool { return errors.Is(err, mongo.ErrNoDocuments) }
