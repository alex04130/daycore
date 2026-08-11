package mongostore

import (
	"context"
	"os"
	"testing"
	"time"

	"daycore/internal/domain"
	"daycore/internal/storage/storagetest"

	"go.mongodb.org/mongo-driver/bson"
)

// The shared behavioural suite, run against a real MongoDB.
//
// This package had no integration coverage at all, which is how the divergences
// an adversarial review pass found — the Go type an agent tool asserts on, what a
// lease returns when it loses, whether a takeover orphans a zombie's verdict —
// went unnoticed. Serialisation is covered without a server in bson_test.go;
// everything about behaviour needs one.
//
// Set MONGO_TEST_DSN to run it. Each case gets its own database, dropped
// afterwards, because the suite requires isolation between cases.
func TestConformance(t *testing.T) {
	dsn := os.Getenv("MONGO_TEST_DSN")
	if dsn == "" {
		t.Skip("set MONGO_TEST_DSN to run the storage conformance suite against MongoDB")
	}
	var n int
	storagetest.Run(t, func(t *testing.T) storagetest.Harness {
		n++
		db := "daycore_conformance_" + time.Now().UTC().Format("150405") + "_" + itoa(n)
		s, err := Open(dsn + "/" + db)
		if err != nil {
			t.Fatalf("open mongo: %v", err)
		}
		ctx := context.Background()
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		t.Cleanup(func() {
			_ = s.db.Drop(context.Background())
			_ = s.Close()
		})
		return mongoHarness{s}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type mongoHarness struct{ s *Store }

func (h mongoHarness) Store() domain.Store { return h.s }

func (h mongoHarness) BackdateJobRuns(t *testing.T, sessionID string, d time.Duration) {
	t.Helper()
	ctx := context.Background()
	cur, err := h.s.c("job_runs").Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		t.Fatalf("backdate job runs: %v", err)
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var doc jobRunDoc
		if err := cur.Decode(&doc); err != nil {
			t.Fatalf("backdate decode: %v", err)
		}
		if _, err := h.s.c("job_runs").UpdateOne(ctx, bson.M{"_id": doc.Key},
			bson.M{"$set": bson.M{"started_at": doc.StartedAt.Add(-d)}}); err != nil {
			t.Fatalf("backdate update: %v", err)
		}
	}
}

func (h mongoHarness) BackdateProposals(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	if _, err := h.s.c("proposals").UpdateMany(context.Background(),
		bson.M{"session_id": sessionID},
		bson.M{"$set": bson.M{"updated_at": at.UTC()}}); err != nil {
		t.Fatalf("backdate proposals: %v", err)
	}
}

func (h mongoHarness) ForceAILogCreatedAt(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	// Milliseconds, not a BSON date: ai_call_logs stores created_at as an int64
	// (see aiCallLogDoc), unlike proposals. Writing a time.Time here would make
	// every later comparison in this collection compare a date against a number
	// and quietly match nothing.
	if _, err := h.s.c("ai_call_logs").UpdateMany(context.Background(),
		bson.M{"session_id": sessionID},
		bson.M{"$set": bson.M{"created_at": at.UnixMilli()}}); err != nil {
		t.Fatalf("force ai log created_at: %v", err)
	}
}

func (h mongoHarness) ForceProposalCreatedAt(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	if _, err := h.s.c("proposals").UpdateMany(context.Background(),
		bson.M{"session_id": sessionID},
		bson.M{"$set": bson.M{"created_at": at.UTC()}}); err != nil {
		t.Fatalf("force created_at: %v", err)
	}
}

// The one migration Mongo does not get for free.
//
// ⚠️ On all three SQL engines, `ALTER TABLE … ADD COLUMN family_id NOT NULL
// DEFAULT 'default'` fills existing rows as part of the statement. Mongo has no
// such rule: a document written before the field existed has no field, and
// `{"family_id": "default"}` does not match it. Without the explicit backfill
// in Migrate, every theme anybody made before this batch would VANISH from the
// list — not error, not warn, gone — on exactly one of the four backends.
//
// This test writes the pre-migration shape by hand, because that shape can no
// longer be produced through the repository. It is the only way to assert on a
// migration whose input is a schema that does not exist any more.
func TestFamilyIDBackfillFindsPreMigrationDocuments(t *testing.T) {
	dsn := os.Getenv("MONGO_TEST_DSN")
	if dsn == "" {
		t.Skip("set MONGO_TEST_DSN to run the migration test against MongoDB")
	}
	db := "daycore_backfill_" + time.Now().UTC().Format("150405000")
	s, err := Open(dsn + "/" + db)
	if err != nil {
		t.Fatalf("open mongo: %v", err)
	}
	ctx := context.Background()
	t.Cleanup(func() { _ = s.db.Drop(ctx); s.Close() })

	// A theme and a switch as they were written before families existed.
	for coll, doc := range map[string]bson.M{
		"custom_themes": {
			"_id": "old-theme", "session_id": "sid1", "name": "旧的",
			"base": "", "dark": false, "variables": bson.M{"--primary": "#f472b6"},
			"created_at": time.Now().UTC(), "updated_at": time.Now().UTC(),
		},
		"theme_switch_log": {
			"_id": "old-switch", "session_id": "sid1", "theme": "night",
			"switched_at": time.Now().UTC(),
		},
	} {
		if _, err := s.c(coll).InsertOne(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}

	// Before the backfill it is invisible — asserted so the test proves the
	// migration is what fixes it, not that the query was never scoped.
	if got, _ := s.Themes().List(ctx, "sid1", domain.FallbackFamilyID); len(got) != 0 {
		t.Fatalf("a document with no family_id matched a scoped query (%d) — this test is not testing what it claims", len(got))
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.Themes().List(ctx, "sid1", domain.FallbackFamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "old-theme" {
		t.Errorf("the pre-migration theme is still invisible after Migrate: %+v", got)
	}
	var sw bson.M
	if err := s.c("theme_switch_log").FindOne(ctx, bson.M{"_id": "old-switch"}).Decode(&sw); err != nil {
		t.Fatal(err)
	}
	if sw["family_id"] != domain.FallbackFamilyID {
		t.Errorf("the switch log row was not backfilled: %v", sw["family_id"])
	}

	// Idempotent: a second boot must not touch anything, and must not undo a
	// family somebody has since been assigned.
	if _, err := s.c("custom_themes").UpdateOne(ctx, bson.M{"_id": "old-theme"},
		bson.M{"$set": bson.M{"family_id": "liuli"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Themes().List(ctx, "sid1", "liuli"); len(got) != 1 {
		t.Error("a second Migrate moved a theme back to the fallback family")
	}
}
