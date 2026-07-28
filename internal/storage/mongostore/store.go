// Package mongostore implements domain.Store on MongoDB. It satisfies the exact
// same contract as the SQL store, proving the database/interface separation:
// the rest of the app is identical regardless of engine. Registered as "mongodb".
package mongostore

import (
	"context"
	"errors"
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

func dbNameFromDSN(dsn string) string {
	s := dsn
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
		if j := strings.IndexAny(s, "?"); j >= 0 {
			s = s[:j]
		}
		if s != "" {
			return s
		}
	}
	return "daycore"
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

// Batch C — multi-instance coordination and cached derivations.
func (s *Store) Proposals() domain.ProposalRepository { return proposalRepo{s} }
func (s *Store) Leases() domain.LeaseRepository       { return leaseRepo{s} }
func (s *Store) JobRuns() domain.JobRunRepository     { return jobRunRepo{s} }
func (s *Store) Rapport() domain.RapportRepository    { return rapportRepo{s} }
func (s *Store) Rhythm() domain.RhythmRepository      { return rhythmRepo{s} }
func (s *Store) Locales() domain.LocaleRepository     { return localeRepo{s} }

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
		// job_runs' unique index is not an optimisation — it IS the mutual
		// exclusion. Claim inserts and reads the duplicate-key failure as
		// "someone else owns this occurrence".
		{"job_runs", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "job_name", Value: 1}, {Key: "run_key", Value: 1}}, Options: uniq}},
		{"job_runs", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "started_at", Value: -1}}}},
		{"job_runs", mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "started_at", Value: 1}}}},
		{"rhythm_days", mongo.IndexModel{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "day", Value: -1}}}},
		{"locale_overrides", mongo.IndexModel{Keys: bson.D{{Key: "message_key", Value: 1}, {Key: "locale", Value: 1}}, Options: uniq}},
		{"locale_overrides", mongo.IndexModel{Keys: bson.D{{Key: "locale", Value: 1}}}},
	}
	for _, sp := range specs {
		if _, err := s.c(sp.coll).Indexes().CreateOne(ctx, sp.model); err != nil {
			return err
		}
	}
	return nil
}

func notFound(err error) bool { return errors.Is(err, mongo.ErrNoDocuments) }
