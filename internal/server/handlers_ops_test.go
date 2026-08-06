package server

import (
	"context"
	"net/http"
	"testing"

	"daycore/internal/domain"
)

// Iron rule 3 says everything is visible and reversible, and this registry is
// where that promise is kept. A write with no registered inverse is an operation
// the ledger can show and cannot undo.
func TestEveryRegisteredRevertResolves(t *testing.T) {
	if len(revertHandlers) == 0 {
		t.Fatal("no revert handlers registered — init() did not run, or the map was replaced")
	}
	for action, h := range revertHandlers {
		if h == nil {
			t.Errorf("%s registered a nil handler", action)
		}
		if action == "" {
			t.Error("an empty action is registered")
		}
		// "revert" itself must never be reversible: undoing an undo is a new
		// forward operation, not a rollback (consensus 23 — the ledger is
		// append-only and regret is a new entry, not an eraser).
		if action == "revert" {
			t.Error("revert must not be registerable as its own inverse")
		}
	}
}

// The actions the plan-and-rules surface writes today all have inverses. This is
// the list to extend, deliberately, when a new write lands — not a switch to
// edit in passing.
func TestKnownWritesAreReversible(t *testing.T) {
	for _, action := range []string{
		"plan_add", "plan_update", "plan_remove", "plan_upsert", "plan_autoplan",
		"rule_create", "rule_update", "rule_delete", "rule_batch",
		"memory_add", "memory_delete", "memory_clear",
	} {
		if _, ok := revertHandlers[action]; !ok {
			t.Errorf("%s has no registered inverse", action)
		}
	}
}

// Registering twice would make the winner depend on link order, and the loser
// dead code nobody notices.
func TestDuplicateRevertRegistrationPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering the same action twice should panic")
		}
	}()
	registerRevert("plan_add", func(*Server, context.Context, http.ResponseWriter, string, string, *domain.OperationLog, revertDetail) {
	})
}
