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

func (h mongoHarness) ForceProposalCreatedAt(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	if _, err := h.s.c("proposals").UpdateMany(context.Background(),
		bson.M{"session_id": sessionID},
		bson.M{"$set": bson.M{"created_at": at.UTC()}}); err != nil {
		t.Fatalf("force created_at: %v", err)
	}
}
