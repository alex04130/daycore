package rapport

import (
	"math"
	"testing"
	"time"

	"daycore/internal/domain"
)

func op(id, action, dom, actor, target string) domain.OperationLog {
	return domain.OperationLog{
		ID: id, SessionID: "sid", Action: action, Domain: dom,
		Actor: actor, TargetID: target, CreatedAt: time.Now(),
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The asymmetry is the design: trust is slow to earn, quick to lose.
func TestDeltaAsymmetry(t *testing.T) {
	if !(DeltaAccept < -DeltaReject && -DeltaReject < -DeltaUndo) {
		t.Fatalf("accept(%v) must be smaller than a reject(%v), which must be smaller than an undo(%v)",
			DeltaAccept, DeltaReject, DeltaUndo)
	}
}

func TestReplayAcceptAndReject(t *testing.T) {
	start := Cold()[domain.OpDomainSchedule].Value
	got := Replay([]domain.OperationLog{
		op("1", ActionProposalAccept, domain.OpDomainSchedule, domain.ActorUser, ""),
		op("2", ActionProposalAccept, domain.OpDomainSchedule, domain.ActorUser, ""),
		op("3", ActionProposalReject, domain.OpDomainSchedule, domain.ActorUser, ""),
	})
	s := got.Get(domain.OpDomainSchedule)
	if want := start + 2*DeltaAccept + DeltaReject; !near(s.Value, want) {
		t.Errorf("value = %v, want %v", s.Value, want)
	}
	// A rejection lowers the score but teaches nothing — otherwise the gate
	// below could be unlocked purely by being turned down.
	if s.Evidence != 2 {
		t.Errorf("evidence = %d, want 2 (rejections must not count)", s.Evidence)
	}
}

// Undoing the agent's work is the strongest signal there is (iron rule 4), and
// it is scored against the domain of the thing undone, not the revert itself.
func TestReplayRevert(t *testing.T) {
	base := Cold()[domain.OpDomainArchive].Value
	got := Replay([]domain.OperationLog{
		op("a1", "material_create", domain.OpDomainArchive, domain.ActorAgent, ""),
		op("r1", ActionRevert, domain.OpDomainSystem, domain.ActorUser, "a1"),
	})
	if s := got.Get(domain.OpDomainArchive); !near(s.Value, base+DeltaUndo) {
		t.Errorf("archive = %v, want %v — the penalty belongs to the undone op's domain",
			s.Value, base+DeltaUndo)
	}
	if s := got.Get(domain.OpDomainSystem); s.Value != 0.5 {
		t.Errorf("system must never be scored, got %v", s.Value)
	}
}

func TestReplayRevertOfOwnWork(t *testing.T) {
	base := Cold()[domain.OpDomainSchedule].Value
	got := Replay([]domain.OperationLog{
		op("u1", "plan_add", domain.OpDomainSchedule, domain.ActorUser, ""),
		op("r1", ActionRevert, domain.OpDomainSchedule, domain.ActorUser, "u1"),
	})
	// Undoing your own edit says nothing about the agent.
	if s := got.Get(domain.OpDomainSchedule); !near(s.Value, base) {
		t.Errorf("value moved to %v; undoing one's own work must not penalise the agent", s.Value)
	}
}

// A revert whose original is outside the replay window is skipped rather than
// guessed at — attributing it to the revert's own domain would put the penalty
// in the wrong place.
func TestReplayRevertWithUnknownOriginal(t *testing.T) {
	base := Cold()[domain.OpDomainSchedule].Value
	got := Replay([]domain.OperationLog{
		op("r1", ActionRevert, domain.OpDomainSchedule, domain.ActorUser, "gone"),
	})
	if s := got.Get(domain.OpDomainSchedule); !near(s.Value, base) {
		t.Errorf("value = %v, want untouched %v", s.Value, base)
	}
}

func TestClamping(t *testing.T) {
	var ops []domain.OperationLog
	for i := 0; i < 200; i++ {
		ops = append(ops, op(string(rune('a'+i%26))+string(rune('0'+i/26)),
			ActionProposalAccept, domain.OpDomainCare, domain.ActorUser, ""))
	}
	if s := Replay(ops).Get(domain.OpDomainCare); s.Value > Ceiling {
		t.Errorf("value %v broke the ceiling", s.Value)
	}
	ops = nil
	for i := 0; i < 200; i++ {
		ops = append(ops, op(string(rune('a'+i%26))+string(rune('0'+i/26)),
			ActionProposalReject, domain.OpDomainCare, domain.ActorUser, ""))
	}
	if s := Replay(ops).Get(domain.OpDomainCare); s.Value < Floor {
		t.Errorf("value %v broke the floor", s.Value)
	}
}

// Incremental folding and a full replay must agree — the cache is only allowed
// to exist because it is reproducible from the ledger.
func TestIncrementalMatchesReplay(t *testing.T) {
	ops := []domain.OperationLog{
		op("1", ActionProposalAccept, domain.OpDomainSchedule, domain.ActorUser, ""),
		op("2", "plan_add", domain.OpDomainSchedule, domain.ActorAgent, ""),
		op("3", ActionRevert, domain.OpDomainSystem, domain.ActorUser, "2"),
		op("4", ActionProposalAccept, domain.OpDomainArchive, domain.ActorUser, ""),
		op("5", ActionProposalReject, domain.OpDomainCare, domain.ActorUser, ""),
	}
	full := Replay(ops)

	// Fold the first half, snapshot, resume from the snapshot for the rest —
	// which is exactly what a cached score catching up from a cursor does.
	f1 := NewFolder()
	for _, o := range ops[:3] {
		f1.Fold(o)
	}
	f2 := NewFolderFrom(f1.Scores())
	for _, o := range ops[3:] {
		f2.Fold(o)
	}
	for _, d := range Domains {
		a, b := full.Get(d), f2.Scores().Get(d)
		if !near(a.Value, b.Value) || a.Evidence != b.Evidence {
			t.Errorf("%s: replay %+v vs incremental %+v", d, a, b)
		}
	}
}

// A domain that has never been touched reports its cold-start value, not zero —
// "no evidence yet" is not the same as "trusted with nothing".
func TestGetNeverReturnsZero(t *testing.T) {
	s := Scores{}
	for _, d := range Domains {
		if got := s.Get(d); got.Value == 0 {
			t.Errorf("%s came back as 0", d)
		}
	}
	if got := s.Get(domain.OpDomainArchive).Value; !near(got, 0.30) {
		t.Errorf("archive should start higher than schedule (cheap mistakes), got %v", got)
	}
	if got := s.Get(domain.OpDomainSchedule).Value; !near(got, 0.10) {
		t.Errorf("schedule cold start = %v, want 0.10", got)
	}
}

func TestPhaseAndEvidenceVisibility(t *testing.T) {
	low := Score{Domain: domain.OpDomainSchedule, Value: 0.44}
	high := Score{Domain: domain.OpDomainSchedule, Value: 0.45}
	if low.Phase() != PhaseApprentice || !low.ShowEvidence() {
		t.Error("below the line the agent should be showing its working")
	}
	if high.Phase() != PhaseSteward || high.ShowEvidence() {
		t.Error("at the line it should stop narrating")
	}
}

func TestGate(t *testing.T) {
	g := DefaultGate()
	cases := []struct {
		name       string
		score      Score
		confidence float64
		want       string
	}{
		{"cold schedule", Score{Domain: domain.OpDomainSchedule, Evidence: 0}, 0.99, ReasonNeedEvidence},
		{"schedule one short", Score{Domain: domain.OpDomainSchedule, Evidence: 2}, 0.99, ReasonNeedEvidence},
		{"first cut, strong", Score{Domain: domain.OpDomainSchedule, Evidence: 3}, 0.85, ReasonOK},
		{"first cut, weak", Score{Domain: domain.OpDomainSchedule, Evidence: 3}, 0.60, ReasonFirstCutTooWeak},
		{"past the first cut", Score{Domain: domain.OpDomainSchedule, Evidence: 4}, 0.60, ReasonOK},
		{"archive needs only one", Score{Domain: domain.OpDomainArchive, Evidence: 1}, 0.85, ReasonOK},
		{"system never initiates", Score{Domain: domain.OpDomainSystem, Evidence: 99}, 0.99, ReasonNotScored},
	}
	for _, c := range cases {
		got := g.Allows(c.score, c.confidence)
		if got.Reason != c.want {
			t.Errorf("%s: reason = %s, want %s", c.name, got.Reason, c.want)
		}
		if got.Allowed != (c.want == ReasonOK) {
			t.Errorf("%s: allowed = %v for reason %s", c.name, got.Allowed, got.Reason)
		}
	}
}

func TestMoodKindRegistry(t *testing.T) {
	kinds := domain.MoodKinds()
	if len(kinds) != 12 {
		t.Fatalf("the design set has 12 moods, registry has %d", len(kinds))
	}
	seen := map[string]bool{}
	var pos, neg int
	for _, k := range kinds {
		if seen[k.ID] {
			t.Errorf("duplicate mood id %q", k.ID)
		}
		seen[k.ID] = true
		if k.Emoji == "" || k.NameZH == "" || k.NameEN == "" {
			t.Errorf("%s is missing emoji or a label", k.ID)
		}
		if k.Valence < -2 || k.Valence > 2 {
			t.Errorf("%s valence %d out of range", k.ID, k.Valence)
		}
		if k.Valence > 0 {
			pos++
		}
		if k.Valence < 0 {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		t.Error("a trend needs moods on both sides")
	}
	if _, ok := domain.MoodKindByID("nope"); ok {
		t.Error("unknown id should not resolve")
	}
	k, _ := domain.MoodKindByID("down")
	if k.MoodName("en-US") != "Down" || k.MoodName("zh-CN") != "低落" {
		t.Errorf("labels: %q / %q", k.MoodName("en-US"), k.MoodName("zh-CN"))
	}
	if k.MoodName("fr") != "低落" {
		t.Error("unknown locale should fall back to the design original, not English")
	}
}
