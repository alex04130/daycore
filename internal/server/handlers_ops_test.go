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
// TestKnownWritesAreReversible used to live here with twelve action names typed
// out by hand. It is gone, replaced by TestEveryLoggedActionIsDeclared in
// oplog_actions_test.go, because the hand-written list did exactly what
// hand-written lists do: β0+ added four capture tools, nobody added them to the
// list, and a test that checked a subset of itself reported clean for months.
//
// Keeping it alongside the AST gate would be worse than deleting it — a second
// list to forget to update, with the credibility of a passing test.

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
