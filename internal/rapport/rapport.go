// Package rapport scores how much latitude the user has given the agent, per
// domain. It is the thing the experience core calls 默契 — what used to be
// described as the agent's "boldness".
//
// The score is not a stored fact. It is a reading of the operation log: accept
// a proposal and it rises, turn one down and it falls, undo something the agent
// did and it falls further. Any cache of it must be rebuildable by re-folding
// the ledger, which is why Replay and the incremental Folder share one code
// path — two implementations would eventually disagree, and then the cache
// would be authoritative over the ledger, which is exactly backwards.
//
// Everything here is a pure function of a slice of operations. No storage, no
// clock, no config.
package rapport

import (
	"sort"

	"daycore/internal/domain"
)

// Deltas, copied from the design prototype (daycore-core.js:186/187/222). The
// asymmetry is the point and should not be "tidied" into symmetry:
//
//	accept  +0.03   a small nod
//	reject  -0.05   turning something down says more than accepting one
//	undo    -0.07   the agent acted and the user took it back — the strongest
//	                signal available, and the whole of iron rule 4
//
// Trust is slow to earn and quick to lose, deliberately.
const (
	DeltaAccept = 0.03
	DeltaReject = -0.05
	DeltaUndo   = -0.07

	// Floor and Ceiling keep the score off the rails: never so low the agent
	// can never recover, never so high it stops listening.
	Floor   = 0.05
	Ceiling = 0.95

	// ApprenticeBelow splits the two phases. Below it the agent shows its
	// evidence on the card ("你前三天都在 8 点跑步，今天也排上？"); above it the
	// reasoning sinks into the card and can still be asked for. A domain that
	// gets undone repeatedly falls back here on its own — explaining yourself
	// again is what losing trust looks like.
	ApprenticeBelow = 0.45
)

// Domains that carry a score. system is plumbing and never scored.
//
// The experience core's §7 text lists three, but the design prototype's three
// seeds all carry a fourth — care — and the crisis-day seed has care-domain
// proposals in the ledger. Four it is: being trusted with the schedule says
// nothing about being trusted to check in on how someone is doing.
var Domains = []string{
	domain.OpDomainSchedule,
	domain.OpDomainHabit,
	domain.OpDomainArchive,
	domain.OpDomainCare,
}

// Phase is the visible half of the score.
type Phase string

const (
	PhaseApprentice Phase = "apprentice" // shows its working
	PhaseSteward    Phase = "steward"    // just does it, reasons on request
)

// Score is one domain's standing.
type Score struct {
	Domain   string  `json:"domain"`
	Value    float64 `json:"value"`
	Evidence int     `json:"evidence"`
}

// Phase derives the visible phase from the value.
func (s Score) Phase() Phase {
	if s.Value < ApprenticeBelow {
		return PhaseApprentice
	}
	return PhaseSteward
}

// Scores is the whole picture, keyed by domain.
type Scores map[string]Score

// Cold is where a brand-new session starts. Archive and care begin higher than
// schedule and habit because their mistakes are cheap: a wrongly filed note is
// fixed by opening the folder, whereas a wrongly moved class is a missed class
// (consensus 3's filing bias).
func Cold() Scores {
	return Scores{
		domain.OpDomainSchedule: {Domain: domain.OpDomainSchedule, Value: 0.10},
		domain.OpDomainHabit:    {Domain: domain.OpDomainHabit, Value: 0.10},
		domain.OpDomainArchive:  {Domain: domain.OpDomainArchive, Value: 0.30},
		domain.OpDomainCare:     {Domain: domain.OpDomainCare, Value: 0.30},
	}
}

// Get returns a domain's score, or its cold-start value if nothing has happened
// there yet. Never returns a zero Score — a value of 0 would read as "trusted
// with nothing", which is not the same as "no evidence yet".
func (s Scores) Get(d string) Score {
	if sc, ok := s[d]; ok {
		return sc
	}
	if cold, ok := Cold()[d]; ok {
		return cold
	}
	return Score{Domain: d, Value: 0.5}
}

// Sorted returns the scores in a stable order, for display.
func (s Scores) Sorted() []Score {
	out := make([]Score, 0, len(Domains))
	for _, d := range Domains {
		out = append(out, s.Get(d))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

// Op actions that move the score. Accepting and rejecting a proposal are
// explicit verdicts; a revert is an implicit one.
const (
	ActionProposalAccept = "proposal_accept"
	ActionProposalReject = "proposal_reject"
	ActionRevert         = "revert"
)

// Origin reports the actor and domain of an operation the folder has not seen
// for itself.
//
// A catch-up that resumes from a cursor starts with an empty seen map, so a
// revert whose original lies before that cursor has nothing to score against.
// Skipping it would make the incremental result disagree with a full replay —
// and the cache is only allowed to exist because it is reproducible from the
// ledger, so a disagreement means the cache has quietly become authoritative
// over the ledger. That is exactly backwards.
//
// The storage layer supplies this (OperationLogRepository.Get). Passing nil
// restores the old skip-and-hope behaviour and is only correct when every
// operation is already in the window, which is to say: for a full replay.
type Origin func(id string) (actor, domain string, ok bool)

// Folder folds operations into scores one at a time, so the same logic serves
// both a full replay and an incremental catch-up from a cursor.
//
// It remembers the actor and domain of operations it has seen, because a revert
// only says something about the agent when the thing reverted was the agent's
// doing — and it is scored against the domain of that original operation, not
// the revert's own.
type Folder struct {
	scores  Scores
	seen    map[string]seenOp
	resolve Origin
}

type seenOp struct {
	actor  string
	domain string
}

// NewFolder starts from the cold-start baseline. A full replay needs no Origin:
// every operation a revert can name has already passed through Fold.
func NewFolder() *Folder {
	return &Folder{scores: Cold(), seen: map[string]seenOp{}}
}

// NewFolderFrom resumes from a cached snapshot, for catching up rather than
// replaying from the beginning.
//
// resolve is not optional in practice. Without it this folder cannot score a
// revert of anything older than its window, and the cache it produces will
// differ from a replay — see Origin.
func NewFolderFrom(s Scores, resolve Origin) *Folder {
	cp := make(Scores, len(s))
	for k, v := range s {
		cp[k] = v
	}
	for d, cold := range Cold() {
		if _, ok := cp[d]; !ok {
			cp[d] = cold
		}
	}
	return &Folder{scores: cp, seen: map[string]seenOp{}, resolve: resolve}
}

// Fold applies one operation. Operations must arrive oldest-first; a revert
// whose original has not been seen is ignored rather than guessed at.
func (f *Folder) Fold(op domain.OperationLog) {
	d := op.Domain
	if d == "" {
		d = domain.OpDomainOf(op.Action)
	}
	f.seen[op.ID] = seenOp{actor: op.Actor, domain: d}

	switch op.Action {
	case ActionProposalAccept:
		f.bump(d, DeltaAccept, true)
	case ActionProposalReject:
		f.bump(d, DeltaReject, false)
	case ActionRevert:
		orig, ok := f.seen[op.TargetID]
		if !ok && f.resolve != nil {
			// The original is behind this folder's window. Look it up rather
			// than skip, or a catch-up would score fewer reverts than a replay
			// and the cache would stop matching the ledger. Cache the answer:
			// a card undone twice must not cost two queries.
			if actor, dom, found := f.resolve(op.TargetID); found {
				orig = seenOp{actor: actor, domain: dom}
				f.seen[op.TargetID] = orig
				ok = true
			}
		}
		if !ok {
			// Genuinely unknown — the original has been pruned, or no resolver
			// was supplied. Scoring it against the revert's own domain would
			// put the penalty in the wrong place, and guessing the actor would
			// penalise the agent for the user undoing their own edit. Skipping
			// is the honest option.
			return
		}
		if orig.actor != domain.ActorAgent {
			return // undoing your own work says nothing about the agent
		}
		f.bump(orig.domain, DeltaUndo, false)
	}
}

// bump moves a domain's score, clamped. Evidence counts only what builds
// standing — a rejection lowers the score but does not make the agent any more
// experienced, so the gate below cannot be unlocked by being turned down.
func (f *Folder) bump(d string, delta float64, evidence bool) {
	if !scored(d) {
		return
	}
	s := f.scores.Get(d)
	s.Domain = d
	s.Value = clamp(s.Value + delta)
	if evidence {
		s.Evidence++
	}
	f.scores[d] = s
}

// Scores returns the folded result. The returned map is the folder's own — copy
// it if the caller intends to keep it past further folding.
func (f *Folder) Scores() Scores { return f.scores }

// Replay folds an entire ledger, oldest first.
func Replay(ops []domain.OperationLog) Scores {
	f := NewFolder()
	for _, op := range ops {
		f.Fold(op)
	}
	return f.Scores()
}

func scored(d string) bool {
	for _, x := range Domains {
		if x == d {
			return true
		}
	}
	return false
}

func clamp(v float64) float64 {
	if v < Floor {
		return Floor
	}
	if v > Ceiling {
		return Ceiling
	}
	return v
}
