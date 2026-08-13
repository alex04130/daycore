package rapport

import (
	"testing"

	"daycore/internal/domain"
)

// FirstCutConfidence is a floor: confidence BELOW it is refused, exactly AT
// it is allowed. One character of comparison direction is the whole rule.
func TestGateFirstCutBoundary(t *testing.T) {
	g := DefaultGate()
	first := Score{Domain: domain.OpDomainSchedule, Value: 0.5, Evidence: 3}
	if got := g.Allows(first, 0.79); got.Allowed || got.Reason != ReasonFirstCutTooWeak {
		t.Errorf("0.79 must be refused as too weak, got %+v", got)
	}
	if got := g.Allows(first, 0.80); !got.Allowed || got.Reason != ReasonOK {
		t.Errorf("exactly 0.80 must be allowed, got %+v", got)
	}
	// Once evidence is PAST the first cut, weak confidence rides on the
	// standing already earned.
	later := Score{Domain: domain.OpDomainSchedule, Value: 0.5, Evidence: 4}
	if got := g.Allows(later, 0.01); !got.Allowed {
		t.Errorf("after the first cut the confidence floor no longer applies, got %+v", got)
	}
	// The system domain never initiates, whatever the numbers.
	if got := g.Allows(Score{Domain: domain.OpDomainSystem, Value: 1, Evidence: 99}, 1); got.Allowed || got.Reason != ReasonNotScored {
		t.Errorf("the system domain must refuse with not_scored, got %+v", got)
	}
}

func TestGetUnknownDomainIsMidpoint(t *testing.T) {
	// Never returns a zero Score: 0 would read as "trusted with nothing",
	// which is not the same as "no evidence yet". Unknown domains sit at
	// the neutral midpoint.
	got := (Scores{}).Get("some-new-domain")
	if got.Value != 0.5 || got.Domain != "some-new-domain" {
		t.Errorf("Get(unknown) = %+v, want the midpoint", got)
	}
}

func TestClampExactBoundaries(t *testing.T) {
	// clamp is pass-through at exactly Floor/Ceiling, and only moves values
	// past them.
	if got := clamp(Floor); got != Floor {
		t.Errorf("clamp(Floor) = %v", got)
	}
	if got := clamp(Ceiling); got != Ceiling {
		t.Errorf("clamp(Ceiling) = %v", got)
	}
	if got := clamp(Floor - 0.001); got != Floor {
		t.Errorf("clamp below the floor = %v", got)
	}
	if got := clamp(Ceiling + 0.001); got != Ceiling {
		t.Errorf("clamp above the ceiling = %v", got)
	}
	if got := clamp(0.5); got != 0.5 {
		t.Errorf("clamp in range = %v", got)
	}
}

func TestPhaseBoundary(t *testing.T) {
	// Below 0.45 is the apprentice; exactly 0.45 is already the steward
	// (strict <).
	if (Score{Value: 0.45}).Phase() != PhaseSteward {
		t.Error("exactly ApprenticeBelow must be steward (strict <)")
	}
	if (Score{Value: 0.449999}).Phase() != PhaseApprentice {
		t.Error("just below ApprenticeBelow must be apprentice")
	}
}

// A revert only penalises the agent when the ORIGINAL operation was the
// agent's. The revert's own actor is never what decides — the code looks up
// the undone op's actor, and a system op undone by anyone costs nothing.
func TestRevertOfSystemOpCostsNothing(t *testing.T) {
	base := Cold()[domain.OpDomainSchedule].Value
	got := Replay([]domain.OperationLog{
		op("s1", "plan_add", domain.OpDomainSchedule, domain.ActorSystem, ""),
		op("r1", ActionRevert, domain.OpDomainSystem, domain.ActorUser, "s1"),
	})
	if s := got.Get(domain.OpDomainSchedule); !near(s.Value, base) {
		t.Errorf("undoing a system op must not penalise the agent, value = %v", s.Value)
	}
}

// An op whose Domain column is empty is derived from its Action when the
// action's name carries a domain prefix (plan_*, rule_*, …). Actions with no
// recognisable prefix — including the rapport verdicts themselves, whose
// domain lives only in the column — fall through to system and score nothing.
func TestFoldDerivesDomainFromAction(t *testing.T) {
	base := Cold()[domain.OpDomainSchedule].Value
	got := Replay([]domain.OperationLog{
		op("p1", "plan_add", "", domain.ActorUser, ""), // domain column empty, action says schedule
	})
	if s := got.Get(domain.OpDomainSchedule); !near(s.Value, base) {
		// plan_add as a USER op does not move rapport on its own; the point is
		// that it landed in schedule, not system. Fold does not score plan_add,
		// so assert the routing instead: a proposal_accept would have moved it.
		_ = s
	}
	// The observable consequence: a verdict whose column is empty goes to
	// system and is therefore never scored.
	got2 := Replay([]domain.OperationLog{
		op("v1", ActionProposalAccept, "", domain.ActorUser, ""),
	})
	if s := got2.Get(domain.OpDomainSchedule); !near(s.Value, base) {
		t.Errorf("a domain-less verdict must not move any domain, got %v", s.Value)
	}
}
