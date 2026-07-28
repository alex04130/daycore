package domain

import (
	"errors"
	"testing"
	"time"
)

func validProposal() *Proposal {
	return &Proposal{
		SessionID: "sid", Title: "把复习挪到晚上",
		Level: LevelL2, Kind: KindCard, TTLPolicy: TTLSilenceRejects,
		// Every proposal has a deadline. Without one the first expiry sweep
		// resolves it as silence, so a card with no ExpiresAt is born dead —
		// which is why Validate refuses it.
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

// The act-first invariant is the load-bearing one: a card whose silence counts
// as consent must already have logged operations to undo. "Acted first with
// nothing undoable" is exactly the failure the undo system exists to prevent,
// so it is caught when the card is built, not when someone reaches for undo.
func TestProposalActFirstRequiresUndo(t *testing.T) {
	p := validProposal()
	p.TTLPolicy = TTLSilenceAccepts
	if err := p.Validate(); !errors.Is(err, ErrActFirstNeedsUndo) {
		t.Fatalf("act-first with no applied ops should be rejected, got %v", err)
	}
	p.AppliedOpIDs = []string{"op-1"}
	if err := p.Validate(); err != nil {
		t.Fatalf("act-first with a logged op should be valid, got %v", err)
	}
	// Ask-first has nothing to undo yet, by definition.
	ask := validProposal()
	if err := ask.Validate(); err != nil {
		t.Fatalf("ask-first needs no applied ops, got %v", err)
	}
}

func TestProposalValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Proposal)
		ok   bool
	}{
		{"baseline", func(*Proposal) {}, true},
		{"no session", func(p *Proposal) { p.SessionID = "" }, false},
		{"no title", func(p *Proposal) { p.Title = "" }, false},
		{"bad level", func(p *Proposal) { p.Level = "L9" }, false},
		{"bad kind", func(p *Proposal) { p.Kind = "banner" }, false},
		{"bad ttl policy", func(p *Proposal) { p.TTLPolicy = "whenever" }, false},
		{"timed without a slot", func(p *Proposal) { p.Kind = KindTimed }, false},
		{"timed with a slot", func(p *Proposal) {
			p.Kind, p.Date, p.Start = KindTimed, "2026-07-26", "15:00"
		}, true},
		{"row without a label", func(p *Proposal) {
			p.Rows = []ProposalRow{{ID: "r1"}}
		}, false},
		{"duplicate row ids", func(p *Proposal) {
			p.Rows = []ProposalRow{{ID: "r1", Label: "a"}, {ID: "r1", Label: "b"}}
		}, false},
		{"distinct rows", func(p *Proposal) {
			p.Rows = []ProposalRow{{ID: "r1", Label: "a"}, {ID: "r2", Label: "b"}}
		}, true},
		// A card with no deadline is born expired: the first sweep sees the zero
		// time, decides it lapsed, and resolves it as silence — so it is
		// rejected at construction instead. ProposalExpiry exists to compute one.
		{"no deadline", func(p *Proposal) { p.ExpiresAt = time.Time{} }, false},
	}
	for _, c := range cases {
		p := validProposal()
		c.mut(p)
		err := p.Validate()
		if c.ok && err != nil {
			t.Errorf("%s: want valid, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: want rejected, got valid", c.name)
		}
	}
}

// Silence resolves in opposite directions depending on whether the work was
// already done (consensus 14). Getting this backwards would either execute
// unattended work or silently drop an undo window.
func TestProposalResolveExpiry(t *testing.T) {
	act := Proposal{TTLPolicy: TTLSilenceAccepts, AppliedOpIDs: []string{"op-1"}}
	if state, res := act.ResolveExpiry(); state != ProposalAccepted || res != ResolutionSilence {
		t.Errorf("act-first silence should accept, got %s/%s", state, res)
	}
	ask := Proposal{TTLPolicy: TTLSilenceRejects}
	// Expired, not rejected: nobody said no. Consensus 15 voids it in place and
	// consensus 3 keeps it in the ledger — both need the distinction.
	if state, res := ask.ResolveExpiry(); state != ProposalExpired || res != ResolutionSilence {
		t.Errorf("ask-first silence should expire, got %s/%s", state, res)
	}
}

func TestProposalDeliverable(t *testing.T) {
	now := time.Date(2026, 7, 26, 15, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)

	base := func() Proposal {
		return Proposal{State: ProposalPending, ExpiresAt: later}
	}
	cases := []struct {
		name string
		mut  func(*Proposal)
		want bool
		why  string
	}{
		{"pending and fresh", func(*Proposal) {}, true, ""},
		{"already accepted", func(p *Proposal) { p.State = ProposalAccepted }, false, "settled"},
		{"lapsed", func(p *Proposal) { p.ExpiresAt = earlier }, false,
			"the user must never see an offer that already lapsed (consensus 15)"},
		{"expiring exactly now", func(p *Proposal) { p.ExpiresAt = now }, false, "not After(now)"},
		{"held back", func(p *Proposal) { p.DeliverAfter = &later }, false,
			"backfill is not immediate"},
		{"hold elapsed", func(p *Proposal) { p.DeliverAfter = &earlier }, true, ""},
	}
	for _, c := range cases {
		p := base()
		c.mut(&p)
		if got := p.Deliverable(now); got != c.want {
			t.Errorf("%s: Deliverable = %v, want %v — %s", c.name, got, c.want, c.why)
		}
	}
}

func TestProposalExpiry(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 7, 26, 15, 0, 0, 0, loc)

	// A decision card is blocking a turn — seconds, not hours.
	dec := &Proposal{Kind: KindDecision, Level: LevelL2, TTLPolicy: TTLSilenceRejects}
	if got := ProposalExpiry(dec, now, loc, nil); !got.Equal(now.Add(DefaultDecisionTTL)) {
		t.Errorf("decision TTL = %v", got.Sub(now))
	}

	// An L1 placeholder dies when the slot it points at begins — an offer to
	// fill 3pm is meaningless at 3pm.
	slot := time.Date(2026, 7, 26, 18, 0, 0, 0, loc)
	l1 := &Proposal{Kind: KindTimed, Level: LevelL1, TTLPolicy: TTLSilenceRejects}
	if got := ProposalExpiry(l1, now, loc, &slot); !got.Equal(slot) {
		t.Errorf("L1 should expire at its slot, got %v", got)
	}

	// Act-first: the undo window.
	act := &Proposal{Kind: KindCard, Level: LevelL2, TTLPolicy: TTLSilenceAccepts}
	if got := ProposalExpiry(act, now, loc, nil); !got.Equal(now.Add(DefaultUndoWindow)) {
		t.Errorf("undo window = %v", got.Sub(now))
	}

	// Ask-first at 3pm: 6h would run to 9pm, still today.
	ask := &Proposal{Kind: KindCard, Level: LevelL2, TTLPolicy: TTLSilenceRejects}
	if got := ProposalExpiry(ask, now, loc, nil); !got.Equal(now.Add(DefaultCardTTL)) {
		t.Errorf("card TTL = %v", got.Sub(now))
	}

	// Ask-first at 9pm: 6h would spill into tomorrow, so it is clamped to
	// midnight — an offer about today should not be waiting tomorrow.
	late := time.Date(2026, 7, 26, 21, 0, 0, 0, loc)
	midnight := time.Date(2026, 7, 27, 0, 0, 0, 0, loc)
	if got := ProposalExpiry(ask, late, loc, nil); !got.Equal(midnight) {
		t.Errorf("late card should clamp to midnight, got %v", got)
	}
}
