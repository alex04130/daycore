package sqlstore

import (
	"context"
	"testing"
	"time"

	"daycore/internal/domain"
	"daycore/internal/storage/storagetest"
)

// The shared behavioural suite, run against SQLite. See internal/storage/
// storagetest for why it exists and why every backend has to pass the same one.
func TestConformance(t *testing.T) {
	storagetest.Run(t, func(t *testing.T) storagetest.Harness {
		return sqlHarness{newTestStore(t)}
	})
}

type sqlHarness struct{ s *Store }

func (h sqlHarness) Store() domain.Store { return h.s }

func (h sqlHarness) BackdateJobRuns(t *testing.T, sessionID string, d time.Duration) {
	t.Helper()
	if _, err := h.s.exec(context.Background(),
		`UPDATE job_runs SET started_at = started_at - ? WHERE session_id = ?`,
		d.Milliseconds(), sessionID); err != nil {
		t.Fatalf("backdate job runs: %v", err)
	}
}

func (h sqlHarness) BackdateProposals(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	if _, err := h.s.exec(context.Background(),
		`UPDATE proposals SET updated_at = ? WHERE session_id = ?`,
		toMillis(at), sessionID); err != nil {
		t.Fatalf("backdate proposals: %v", err)
	}
}

func (h sqlHarness) ForceAILogCreatedAt(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	if _, err := h.s.exec(context.Background(),
		`UPDATE ai_call_logs SET created_at = ? WHERE session_id = ?`,
		toMillis(at), sessionID); err != nil {
		t.Fatalf("force ai log created_at: %v", err)
	}
}

func (h sqlHarness) ForceProposalCreatedAt(t *testing.T, sessionID string, at time.Time) {
	t.Helper()
	if _, err := h.s.exec(context.Background(),
		`UPDATE proposals SET created_at = ? WHERE session_id = ?`,
		toMillis(at), sessionID); err != nil {
		t.Fatalf("force created_at: %v", err)
	}
}
