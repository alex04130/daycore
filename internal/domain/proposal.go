package domain

import (
	"context"
	"errors"
	"time"

	"daycore/internal/timeutil"
)

// A Proposal is everything the agent wants to do but has not done — or has done
// and is still offering to undo. Ghost blocks on a timeline, confirmation cards,
// decision cards, and approval messages on an integration channel are all the
// same object seen from different ends; the outbox is just this table filtered
// to state=pending. One resource, not four (EXPERIENCE_CORE §6, consensus 12).
//
// The JSON names of the first block of fields are copied verbatim from the
// design contract (01-core-contract.md §2) — `dur`, `btype`, `lockLevel` — even
// where they break backend naming. That interface is what the four frontends
// share; renaming it here would fork the thing it exists to unify.
type Proposal struct {
	// ── shared with the frontends, names verbatim from the design contract ──
	ID         string        `json:"id"`
	State      ProposalState `json:"state"`
	Level      ProposalLevel `json:"level"`
	Kind       ProposalKind  `json:"kind"`
	Title      string        `json:"title"`
	Summary    string        `json:"summary"`
	Reason     string        `json:"reason"`
	Evidence   string        `json:"evidence"`
	Date       string        `json:"date,omitempty"`
	Start      string        `json:"start,omitempty"` // HH:MM
	Dur        *int          `json:"dur"`
	BType      BlockType     `json:"btype,omitempty"`
	LockLevel  LockLevel     `json:"lockLevel,omitempty"`
	LockReason string        `json:"lockReason,omitempty"`
	Rows       []ProposalRow `json:"rows,omitempty"`

	// ── outbox: generation is unthrottled, delivery is the gate (consensus 15) ──
	SessionID    string     `json:"sessionId"`
	MergeKey     string     `json:"mergeKey,omitempty"`     // same key → only the newest undelivered survives
	DeliverAfter *time.Time `json:"deliverAfter,omitempty"` // backfill is not immediate
	DeliveredAt  *time.Time `json:"deliveredAt,omitempty"`  // nil = still in the pool, never shown
	PushedAt     *time.Time `json:"pushedAt,omitempty"`     // what the ≤3/day push budget counts

	// ── expiry ──
	TTLPolicy  ProposalTTL        `json:"ttlPolicy"`
	ExpiresAt  time.Time          `json:"expiresAt"`
	Resolution ProposalResolution `json:"resolution,omitempty"`

	// ── execution and traceability ──
	Origin        ProposalOrigin `json:"origin"`
	ThreadID      string         `json:"threadId,omitempty"`
	Ops           []ProposalOp   `json:"ops,omitempty"`           // what accepting will run
	AppliedOpIDs  []string       `json:"appliedOpIds,omitempty"`  // already run (act-first cards)
	AcceptOpIDs   []string       `json:"acceptOpIds,omitempty"`   // produced by accepting
	OwnerInstance string         `json:"ownerInstance,omitempty"` // for sweeping after a crash
	Rev           int            `json:"rev"`                     // optimistic concurrency
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

// ProposalRow is one line of a compound proposal, accepted or rejected on its
// own (EXPERIENCE_CORE §6: "复合提案在一张卡内分行列出，每行单独接受/拒绝").
type ProposalRow struct {
	ID    string        `json:"id"`
	Label string        `json:"label"`
	State ProposalState `json:"state"`
	Ops   []ProposalOp  `json:"ops,omitempty"`
}

// ProposalOp is a tool call to run on acceptance. Accepting does not get its own
// executor — it goes through the same agent tool path, which already writes the
// operation log and already has a registered inverse. That reuse is what makes
// "every write is undoable" hold for proposals without a second implementation.
type ProposalOp struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

type ProposalState string

const (
	ProposalPending  ProposalState = "pending"
	ProposalAccepted ProposalState = "accepted"
	ProposalRejected ProposalState = "rejected"
	// ProposalExpired is deliberately not folded into rejected. Consensus 15
	// says an expired offer is voided in place and the user never sees it;
	// consensus 3 says the ledger keeps everything. Both can only hold if a
	// silent expiry is distinguishable from a deliberate no. Expired proposals
	// are never delivered, which is why the frontend contract has three states
	// and the server has four.
	ProposalExpired ProposalState = "expired"
)

// ProposalLevel is the attention ladder (EXPERIENCE_CORE §4). L0 never becomes a
// proposal — silent actions only touch the ledger.
type ProposalLevel string

const (
	LevelL1 ProposalLevel = "L1" // in-view ghost, bound to a time slot
	LevelL2 ProposalLevel = "L2" // a card in the stack, max 3
	LevelL3 ProposalLevel = "L3" // a push, budget ≤3/day
)

// ProposalKind is how it wants to be shown. Rendering is the frontend's call —
// a timeline app draws a ghost, a card app draws a card, a channel sends text.
type ProposalKind string

const (
	KindTimed    ProposalKind = "timed"    // has a slot; can be drawn as a ghost
	KindCard     ProposalKind = "card"     // no slot
	KindDecision ProposalKind = "decision" // blocks an agent turn until answered
)

// ProposalTTL is the asymmetry from consensus 14: silence means yes for things
// already done, and no for things not yet done. Getting this backwards would
// either execute unattended work or silently discard an undo window.
type ProposalTTL string

const (
	TTLSilenceAccepts ProposalTTL = "silence_accepts" // acted first: silence closes the undo window
	TTLSilenceRejects ProposalTTL = "silence_rejects" // asked first: silence voids the offer
)

// ProposalResolution records how a proposal left pending, so the ledger can tell
// "the user said no" from "nobody ever looked".
type ProposalResolution string

const (
	ResolutionUser       ProposalResolution = "user"       // explicitly accepted or rejected
	ResolutionSilence    ProposalResolution = "silence"    // TTL ran out
	ResolutionSuperseded ProposalResolution = "superseded" // a newer proposal with the same mergeKey won
)

type ProposalOrigin string

const (
	OriginAgent     ProposalOrigin = "agent"     // propose_decision inside a turn
	OriginDaemon    ProposalOrigin = "daemon"    // self-initiated
	OriginUpload    ProposalOrigin = "upload"    // batch/cross-domain/low-confidence import
	OriginProtector ProposalOrigin = "protector" // the 20h care nudge
)

// ── TTL defaults ────────────────────────────────────────────────────────────
//
// Two of these are fixed by the experience core and are not tunable:
//   - an L1 placeholder dies when the slot it points at begins (§4: "死线 = 它
//     指向的时段本身")
//   - the morning brief dissolves at local noon (consensus 17: "过午消散")
//
// The rest are starting points, overridable per deployment. They are defaults
// rather than constants because the right number is a product feel, not a
// correctness property — and getting one wrong costs nothing but a re-read.
const (
	// DefaultCardTTL bounds an ask-first L2 card. Capped at local midnight by
	// ProposalExpiry so a card never outlives the day it is about — the 0 point
	// is the semantic day boundary (§5).
	DefaultCardTTL = 6 * time.Hour

	// DefaultUndoWindow is how long an act-first card stays in the stack before
	// silence confirms it. This is when the card leaves the stack, NOT when the
	// operation stops being undoable — the ledger keeps that open forever
	// (consensus 3: 账本随手可翻).
	DefaultUndoWindow = 2 * time.Hour

	// DefaultDecisionTTL bounds a decision card that is blocking an agent turn.
	DefaultDecisionTTL = 45 * time.Second

	// DefaultAsyncDecisionTTL is the same for an async turn, where nobody is
	// watching a stream and a slower answer costs nothing.
	DefaultAsyncDecisionTTL = 90 * time.Second
)

// ProposalExpiry computes the deadline for a proposal that has not set one.
// slotStart is the beginning of the slot an L1 placeholder points at.
func ProposalExpiry(p *Proposal, now time.Time, loc *time.Location, slotStart *time.Time) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	switch {
	case p.Kind == KindDecision:
		return now.Add(DefaultDecisionTTL)
	case p.Level == LevelL1 && slotStart != nil:
		// The slot itself is the deadline — an offer to fill 3pm is meaningless
		// at 3pm.
		return *slotStart
	case p.TTLPolicy == TTLSilenceAccepts:
		return now.Add(DefaultUndoWindow)
	default:
		// Never past the day boundary: an offer about today should not be
		// waiting tomorrow.
		midnight := startOfNextDay(now, loc)
		if deadline := now.Add(DefaultCardTTL); deadline.Before(midnight) {
			return deadline
		}
		return midnight
	}
}

// startOfNextDay delegates to timeutil so the day boundary has one definition.
// It used to compute midnight itself, which meant an ask-first card created
// after 23:00 on a zone that shifts its clocks AT midnight was capped at a
// boundary in the past — born expired. See timeutil.StartOfDay.
func startOfNextDay(now time.Time, loc *time.Location) time.Time {
	return timeutil.StartOfNextDay(now, loc)
}

// ErrActFirstNeedsUndo rejects a proposal that claims silence means yes without
// anything to undo.
var ErrActFirstNeedsUndo = errors.New("proposal: silence_accepts requires appliedOpIds")

// ErrNoExpiry rejects a proposal with no deadline. See Validate.
var ErrNoExpiry = errors.New("proposal: expiresAt required (see ProposalExpiry)")

// Validate enforces the invariants that cannot be allowed to fail at runtime.
//
// The load-bearing one is the act-first rule: a card whose silence counts as
// consent must already have logged operations to undo. "Acted first with nothing
// undoable" is precisely the failure the whole undo system exists to prevent, so
// it is caught when the card is built rather than when someone reaches for undo.
func (p *Proposal) Validate() error {
	if p.SessionID == "" {
		return errors.New("proposal: sessionId required")
	}
	if p.Title == "" {
		return errors.New("proposal: title required")
	}
	switch p.Level {
	case LevelL1, LevelL2, LevelL3:
	default:
		return errors.New("proposal: level must be L1, L2 or L3")
	}
	switch p.Kind {
	case KindTimed, KindCard, KindDecision:
	default:
		return errors.New("proposal: unknown kind")
	}
	switch p.TTLPolicy {
	case TTLSilenceAccepts:
		if len(p.AppliedOpIDs) == 0 {
			return ErrActFirstNeedsUndo
		}
	case TTLSilenceRejects:
	default:
		return errors.New("proposal: unknown ttlPolicy")
	}
	if p.Kind == KindTimed && (p.Date == "" || p.Start == "") {
		return errors.New("proposal: a timed proposal needs date and start")
	}
	// A card with no deadline is born expired: the very first sweep sees
	// expires_at at the zero time, decides it lapsed, and resolves it as
	// silence. Every proposal has a TTL (consensus 14) — ProposalExpiry exists
	// to compute one, so an absent deadline is a caller that forgot to call it,
	// not a proposal that lives forever.
	if p.ExpiresAt.IsZero() {
		return ErrNoExpiry
	}
	for i, r := range p.Rows {
		if r.ID == "" || r.Label == "" {
			return errors.New("proposal: every row needs an id and a label")
		}
		if i > 0 && r.ID == p.Rows[i-1].ID {
			return errors.New("proposal: duplicate row id")
		}
	}
	return nil
}

// Deliverable reports whether a proposal may be shown now. Generation is
// unthrottled and delivery is the only gate (consensus 15) — an expired offer is
// voided in the pool, so the user never sees a suggestion that already lapsed.
func (p Proposal) Deliverable(now time.Time) bool {
	if p.State != ProposalPending || !p.ExpiresAt.After(now) {
		return false
	}
	if p.DeliverAfter != nil && p.DeliverAfter.After(now) {
		return false
	}
	return true
}

// ResolveExpiry returns the terminal state for a lapsed proposal. Act-first
// cards land accepted (the work was already done and is still in the ledger);
// ask-first cards land expired rather than rejected — nobody said no.
func (p Proposal) ResolveExpiry() (ProposalState, ProposalResolution) {
	if p.TTLPolicy == TTLSilenceAccepts {
		return ProposalAccepted, ResolutionSilence
	}
	return ProposalExpired, ResolutionSilence
}

// ── storage ─────────────────────────────────────────────────────────────────

// Presence is a three-way predicate over a nullable timestamp: don't care,
// stamped, unstamped.
//
// Two of this filter's dimensions ask about one — delivered_at and pushed_at —
// and the attention ladder's two budgets need all three answers between them.
// The push budget counts stamps inside a window; the candidate scan wants the
// stamp absent; the card stack wants it present. A bool says two of those,
// which is exactly why `Undelivered bool` could never express the stack budget.
// It is replaced here rather than joined by a second bool, because two bools
// have a fourth combination that means nothing.
type Presence string

const (
	PresenceAny   Presence = ""
	PresenceSet   Presence = "set"
	PresenceUnset Presence = "unset"
)

// ProposalFilter selects proposals. A zero filter matches every proposal in
// THAT SESSION — SessionID is always part of the predicate, never optional —
// which is the console's view of one user.
type ProposalFilter struct {
	SessionID string
	State     ProposalState // "" matches any
	Kind      ProposalKind  // "" matches any
	Level     ProposalLevel // "" matches any
	Date      string        // "" matches any; a day's L1 ghosts
	// Delivered asks whether the card has ever been shown. Unset is the pool —
	// generated but never shown. Consensus 15: generation is unthrottled and
	// delivery is the only gate, so this is the distinction the outbox rests on.
	//
	// ⚠️ delivered_at is a PERMANENT stamp; nothing clears it. So
	// `Delivered: PresenceSet` counts every card this session was ever shown,
	// including ones long since accepted, rejected or expired. It is NOT "how
	// many cards are in the stack right now" — that is the conjunction with
	// State pending and DeliverableAt. Using presence alone for the ≤3-card
	// stack budget would let a user see three cards in their life and then
	// never again.
	Delivered Presence
	// Pushed asks whether the proposal was ever pushed outside the app, which
	// is what the ≤3/day budget counts. Same permanence warning as Delivered:
	// pair it with the window below rather than asking about the stamp alone.
	Pushed Presence
	// PushedSince and PushedBefore bound pushed_at, half-open [since, before).
	// Absolute instants, never a day key: the day a push belongs to depends on
	// the user's timezone, and a stored day key freezes that answer at write
	// time and cannot be recomputed when they travel. Callers get the pair from
	// PushBudgetWindow.
	PushedSince  time.Time
	PushedBefore time.Time
	// DeliverableAt, when set, additionally requires that the proposal has not
	// lapsed and that its deliverAfter has arrived — the rest of what
	// Proposal.Deliverable checks.
	//
	// `Delivered: PresenceUnset` on its own is NOT the outbox: it returns cards
	// that expired in the pool and cards held back for later, both of which the
	// user must never see. Filtering those in Go after the fact would mean the
	// LIMIT is applied before the filter, so a pool full of lapsed cards could
	// return an empty page while deliverable ones waited behind it.
	DeliverableAt time.Time
	// OwnerInstance restricts to proposals claimed by one instance, which is
	// how a crash sweep finds what the dead process was holding. "" matches any.
	OwnerInstance string
	Limit         int
}

// PushBudgetWindow is the half-open span [start, end) that one day's push
// budget is counted over, for the day containing now in loc.
//
// The day boundary is the user's local midnight — the same line EXPERIENCE_CORE
// §5 gives "today/yesterday" and the same one ProposalExpiry caps ask-first
// cards at. Deliberately NOT rhythm's 04:00 day cut: that one is a tunable in
// rhythm.Config, and a budget whose size moves when a learning parameter is
// retuned is not a budget.
//
// It goes through timeutil because local midnight does not always exist — see
// timeutil.StartOfDay. Computing it here with time.Date would, in a zone that
// shifts its clocks at 00:00, produce a window that does not contain now, and
// every count taken during that hour would read zero: the budget would silently
// stop existing on exactly the days it was hardest to reason about.
func PushBudgetWindow(now time.Time, loc *time.Location) (start, end time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	return timeutil.StartOfDay(now, loc), timeutil.StartOfNextDay(now, loc)
}

// ProposalRepository persists the one resource that ghosts, cards, decision
// cards and channel approvals all are.
//
// It exists mostly to unblock horizontal scaling: decision cards live in a
// process-local map today (internal/server/agent.go), so a restart loses every
// pending card and a second instance cannot see the first one's. A table makes
// the card durable and readable from anywhere; waking the goroutine that is
// blocked on it is a separate problem, and not this layer's.
type ProposalRepository interface {
	Create(ctx context.Context, p *Proposal) error
	Get(ctx context.Context, sessionID, id string) (*Proposal, error)
	List(ctx context.Context, f ProposalFilter) ([]Proposal, error)

	// Update writes a proposal back, refusing the write when Rev has moved.
	//
	// Optimistic concurrency rather than a transaction, because sqlstore has
	// none: accepting one row of a compound card is a read-modify-write of the
	// rows JSON, and two accepts racing would otherwise lose one. The caller
	// re-reads and retries on ErrConflict.
	Update(ctx context.Context, p *Proposal) error

	// Expire moves lapsed pending proposals to their terminal state in one
	// statement per policy, and returns how many moved. Act-first cards land
	// accepted, ask-first ones expired — see ResolveExpiry.
	//
	// It is cross-session on purpose: whichever instance holds the worker lease
	// sweeps for everyone, rather than every session waiting for its owner to
	// wake up.
	Expire(ctx context.Context, now time.Time) (int, error)

	// Supersede retires undelivered proposals that share a merge key and are
	// OLDER than keepID, so only the newest survives to be shown. Returns how
	// many were retired.
	//
	// "Older than" rather than "not keepID" is load-bearing. Two daemons that
	// each produce a card for the same merge key and each call this with their
	// own id would otherwise retire each other, and the user would be shown
	// nothing at all — the one outcome consensus 15 does not allow, since
	// throttling delivery is not the same as cancelling it. Comparing creation
	// order makes the survivor the same card whichever daemon calls first.
	//
	// The order has to be TOTAL, and creation time alone is not: timestamps are
	// millisecond-resolution and two instances reacting to one trigger land in
	// the same millisecond routinely. On a tie neither card is older than the
	// other, so a strict comparison retires nothing and BOTH are delivered —
	// the same failure from the other direction. Ties break on id, which is
	// arbitrary but identical from either caller's side.
	Supersede(ctx context.Context, sessionID, mergeKey, keepID string) (int, error)

	// Count answers how many proposals match, which List cannot: List defaults
	// to a page and caps at a ceiling (see ListLimit), so len(List(f)) silently
	// saturates. Both attention budgets are counts, and a budget that stops
	// counting past 100 is not a budget.
	//
	// ⚠️ Count is a read. Deciding "under budget, therefore push" and then
	// stamping pushed_at is a read-modify-write with nothing holding it
	// together — this package has no transactions. Two pushers can both read
	// two and both push. The fix is not a lease: leases rest on clocks, and
	// DATA.md is explicit that correctness may not (job_runs is the mutual
	// exclusion mechanism, leases are throttling). Whatever ends up driving
	// delivery has to claim the right to push through the same insert-or-fail
	// primitive everything else uses.
	Count(ctx context.Context, f ProposalFilter) (int, error)

	// Prune deletes settled proposals older than before and reports how many
	// went. Only terminal states are eligible: a pending row older than the
	// cutoff is one Expire has not reached yet, and deleting it would drop a
	// card the user was still owed rather than tidy up after one they answered.
	// Same shape as JobRunRepository.Prune, and for the same reason — the table
	// grows forever otherwise.
	Prune(ctx context.Context, before time.Time) (int, error)
}
