package rapport

import "daycore/internal/domain"

// Gate decides whether the agent may raise an unprompted L2 card in a domain.
//
// What is gated is initiative, not capability (EXPERIENCE_CORE §8). From day
// one the agent routes, decomposes, schedules and files at full strength when
// asked; what it has to earn is the right to interrupt unasked. Common-sense L1
// hints are never gated either — they rest on how the world works, not on
// anything learned about this particular person.
type Gate struct {
	// MinEvidence is how many accepted proposals a domain needs before the
	// agent may raise one there on its own. Gating on evidence rather than on
	// elapsed days is deliberate: someone who connects Canvas on day one has
	// given the academic domain real facts immediately, and making them wait a
	// week would be treating imported truth as if it were a guess.
	MinEvidence map[string]int

	// FirstCutConfidence is the floor for a domain's *first* unprompted card.
	// §8 only says it must be the most confident candidate available, which on
	// its own would let a pool of weak candidates through on a technicality.
	// An absolute floor is what makes "宁可晚、不可错" mean something.
	FirstCutConfidence float64
}

// DefaultGate is the starting point. These numbers are tunable — none of them
// come from the design original, and getting one wrong costs a re-read, not
// data.
//
// Archive is 1 because a misfiled note is corrected by opening the folder;
// schedule and habit are 3 because a wrong card lands in someone's day; care is
// 2 in between — an unwanted check-in is intrusive but not destructive.
func DefaultGate() Gate {
	return Gate{
		MinEvidence: map[string]int{
			domain.OpDomainSchedule: 3,
			domain.OpDomainHabit:    3,
			domain.OpDomainArchive:  1,
			domain.OpDomainCare:     2,
		},
		FirstCutConfidence: 0.80,
	}
}

// GateResult explains a decision. The reason is not decoration: an apprentice
// card has to show its evidence on the face, and a domain that lost trust has
// to start explaining itself again — both need words, and inventing them at the
// call site would produce a different explanation in every caller.
type GateResult struct {
	Allowed bool
	// Reason is a stable machine-readable code, not display text. Copy is
	// finalised elsewhere; this is what the copy is chosen by.
	Reason string
}

// Reason codes.
const (
	ReasonOK              = "ok"
	ReasonNotScored       = "not_scored"         // system domain: never initiates
	ReasonNeedEvidence    = "need_evidence"      // hasn't earned the first card here yet
	ReasonFirstCutTooWeak = "first_cut_too_weak" // first card in a domain must be the strong one
)

// Allows reports whether an unprompted L2 may go out in this domain.
// confidence is the agent's own 0..1 estimate for this particular candidate.
func (g Gate) Allows(s Score, confidence float64) GateResult {
	if !scored(s.Domain) {
		return GateResult{Reason: ReasonNotScored}
	}
	need, ok := g.MinEvidence[s.Domain]
	if !ok {
		need = 3
	}
	if s.Evidence < need {
		return GateResult{Reason: ReasonNeedEvidence}
	}
	// The first cut: with no accepted card in this domain yet, only the
	// strongest candidate may go. Later cards ride on the standing already
	// earned.
	if s.Evidence == need && confidence < g.FirstCutConfidence {
		return GateResult{Reason: ReasonFirstCutTooWeak}
	}
	return GateResult{Allowed: true, Reason: ReasonOK}
}

// ShowEvidence reports whether a card should wear its reasoning on the face.
// Visibility runs opposite to standing (§8): an apprentice explains itself, a
// steward just acts — and a steward that gets undone repeatedly slides back
// below the line and starts explaining again, without anyone flipping a switch.
func (s Score) ShowEvidence() bool { return s.Phase() == PhaseApprentice }
