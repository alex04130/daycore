package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/schedule"

	"github.com/google/uuid"
)

func init() {
	registerRoutes("proposals", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/proposals", s.handleProposalList)
		mux.HandleFunc("POST /api/proposals/{id}/respond", s.handleProposalRespond)
		mux.HandleFunc("POST /api/plan/conflict", s.handlePlanConflict)
	})
}

// The proposals table stops being dead weight here.
//
// It shipped with six repository methods, a behavioural suite across four
// backends, and no reader or writer anywhere in the server: decision cards
// lived in a process-local map that vanished on restart, and nothing else
// produced a proposal at all. Two representations of "the agent is asking
// something" is the divergence this file exists to avoid — the map stays, but
// only as the thing that wakes a blocked agent turn. The row is the truth.

// persistDecision writes a decision card to the table before it is shown.
//
// Failure to persist does NOT stop the card. The durable record is what makes
// the card survive a restart and appear in the ledger; the card itself is what
// the user is waiting on, and refusing to ask a question because the write
// failed would trade a small loss of history for a stuck conversation.
func (s *Server) persistDecision(ctx context.Context, sid, id, threadID, title, summary string, options []decisionOption, wait time.Duration) {
	if s.store == nil {
		return
	}
	rows := make([]domain.ProposalRow, 0, len(options))
	for _, o := range options {
		rows = append(rows, domain.ProposalRow{ID: o.ID, Label: o.Label, State: domain.ProposalPending})
	}
	p := &domain.Proposal{
		ID: id, SessionID: sid, ThreadID: threadID,
		State: domain.ProposalPending, Level: domain.LevelL2, Kind: domain.KindDecision,
		Title: title, Summary: summary, Rows: rows,
		// Ask-first: a decision card proposes something not yet done, so silence
		// voids it. Getting this backwards would let an unanswered card execute.
		TTLPolicy: domain.TTLSilenceRejects,
		ExpiresAt: time.Now().Add(wait),
		Origin:    domain.OriginAgent,
	}
	// It is on screen the moment it is created — a decision card is not queued.
	now := time.Now()
	p.DeliveredAt = &now
	if err := s.store.Proposals().Create(ctx, p); err != nil && s.log != nil {
		s.log.Warn("proposal: could not persist decision card", "id", id, "err", err)
	}
}

// settleDecision records how a card left pending. Best-effort for the same
// reason persistDecision is: the conversation has already moved on.
func (s *Server) settleDecision(ctx context.Context, sid, id, choice string) {
	if s.store == nil {
		return
	}
	p, err := s.store.Proposals().Get(ctx, sid, id)
	if err != nil || p.State != domain.ProposalPending {
		return
	}
	switch choice {
	case "timeout":
		p.State, p.Resolution = domain.ProposalExpired, domain.ResolutionSilence
	case "cancelled":
		// The user moved on rather than answered. Silence, not a no: consensus 15
		// keeps "nobody ever looked" distinguishable from "they said no", and a
		// cancelled card was never looked at.
		p.State, p.Resolution = domain.ProposalExpired, domain.ResolutionSilence
	default:
		p.State, p.Resolution = domain.ProposalAccepted, domain.ResolutionUser
		for i := range p.Rows {
			if p.Rows[i].ID == choice {
				p.Rows[i].State = domain.ProposalAccepted
			} else {
				p.Rows[i].State = domain.ProposalRejected
			}
		}
	}
	if err := s.store.Proposals().Update(ctx, p); err != nil && s.log != nil {
		s.log.Warn("proposal: could not settle decision card", "id", id, "err", err)
	}
}

type decisionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// GET /api/proposals — what is waiting on the user right now.
//
// Defaults to the stack rather than to everything: pending, already shown, and
// not yet lapsed. That conjunction is the whole point — delivered_at is a
// permanent stamp, so asking about it alone would return every card this
// session has ever been shown (see docs/specs/plan-semantics.md).
func (s *Server) handleProposalList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	f := domain.ProposalFilter{SessionID: sid}
	if q := r.URL.Query(); q.Get("all") == "1" {
		// The console view: everything this session ever had, newest first.
	} else {
		f.State = domain.ProposalPending
		f.Delivered = domain.PresenceSet
		f.DeliverableAt = time.Now()
	}
	if lvl := r.URL.Query().Get("level"); lvl != "" {
		f.Level = domain.ProposalLevel(lvl)
	}
	items, err := s.store.Proposals().List(r.Context(), f)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.proposalList.internal")
		return
	}
	if items == nil {
		items = []domain.Proposal{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"proposals": items})
}

// proposalWriteRetries bounds the read-modify-write retry. Three is enough to
// ride out ordinary contention (a delivery pass, a second tab) and few enough
// that a write loop somewhere else surfaces as an error instead of a hang.
const proposalWriteRetries = 3

const (
	keyProposalGone     = "proposal.gone"
	keyProposalSettled  = "proposal.settled"
	keyConflictTitle    = "plan.conflict.title"
	keyConflictSummary  = "plan.conflict.summary"
	keyConflictOptMove  = "plan.conflict.option.moveOther"
	keyConflictOptSkip  = "plan.conflict.option.skipThis"
	keyConflictOptStand = "plan.conflict.option.leaveBoth"
	keyConflictNoClash  = "plan.conflict.noClash"
)

func init() {
	i18n.Register(keyProposalGone, i18n.Text{
		"zh-CN": "这张卡片已经不在了",
		"en-US": "That card is no longer here",
	})
	i18n.Register(keyProposalSettled, i18n.Text{
		"zh-CN": "这张卡片已经回答过了",
		"en-US": "That card has already been answered",
	})
	i18n.Register(keyConflictTitle, i18n.Text{
		"zh-CN": "两件事撞在一起了",
		"en-US": "Two things want the same time",
	})
	i18n.Register(keyConflictSummary, i18n.Text{
		"zh-CN": "「%s」和「%s」都要 %s–%s。想怎么办？",
		"en-US": "%s and %s both want %s–%s. What would you like to do?",
	})
	i18n.Register(keyConflictOptMove, i18n.Text{
		"zh-CN": "把另一件挪开",
		"en-US": "Move the other one",
	})
	i18n.Register(keyConflictOptSkip, i18n.Text{
		"zh-CN": "这次不去",
		"en-US": "Skip this one",
	})
	i18n.Register(keyConflictOptStand, i18n.Text{
		"zh-CN": "都留着，我自己看着办",
		"en-US": "Leave both — I will manage",
	})
	i18n.Register(keyConflictNoClash, i18n.Text{
		"zh-CN": "这个时段没有和别的事撞上",
		"en-US": "Nothing else wants that time",
	})
}

// POST /api/proposals/{id}/respond — answer a card.
//
// Distinct from POST /api/decisions/{id}/respond, which unblocks an agent turn
// that is waiting right now. This one settles a card nobody is blocked on: the
// conflict marker, and every daemon-produced card once those exist. Both write
// the same row.
func (s *Server) handleProposalRespond(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	locale := s.requestLocale(r)
	var body struct {
		Choice string `json:"choice"`
		Text   string `json:"text"`
	}
	_ = s.readJSON(r, &body)

	// Read-modify-write under optimistic concurrency, so it retries on a rev
	// conflict rather than reporting one.
	//
	// The interface says so in its own doc ("The caller re-reads and retries on
	// ErrConflict") and this handler did not: two tabs answering the same card,
	// or the delivery pass stamping deliveredAt on the very request that answers
	// it, both landed the user on a 500 for something that had in fact worked.
	// Bounded, because a conflict that keeps recurring is not contention any
	// more — it is something else writing in a loop, and retrying forever would
	// hide that.
	var p *domain.Proposal
	var err error
	for attempt := 0; ; attempt++ {
		p, err = s.store.Proposals().Get(r.Context(), sid, r.PathValue("id"))
		if err != nil {
			s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyProposalGone, locale))
			return
		}
		if p.State != domain.ProposalPending {
			// Not an error worth a 500, and not a silent success either: the
			// client asked about a card someone (or a timeout) already settled.
			s.writeErr(w, http.StatusConflict, "already_settled", i18n.T(keyProposalSettled, locale))
			return
		}
		p.State, p.Resolution = domain.ProposalAccepted, domain.ResolutionUser
		if body.Choice == "reject" || body.Choice == "" {
			p.State = domain.ProposalRejected
		}
		for i := range p.Rows {
			if p.Rows[i].ID == body.Choice {
				p.Rows[i].State = domain.ProposalAccepted
			} else {
				p.Rows[i].State = domain.ProposalRejected
			}
		}
		err = s.store.Proposals().Update(r.Context(), p)
		if err == nil {
			break
		}
		if !errors.Is(err, domain.ErrConflict) || attempt >= proposalWriteRetries {
			s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.proposalRespond.internal")
			return
		}
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorUser, Action: "proposal_" + string(p.State),
		TargetID: p.ID, Summary: p.Title,
		Detail: marshalCompact(map[string]any{"choice": body.Choice, "text": body.Text}),
	})
	s.writeJSON(w, http.StatusOK, p)
}

// POST /api/plan/conflict — "this clashes with something".
//
// The third way out of a lock refusal. The other two let the user act (take
// leave, unlock); this one is for when neither is right — the class really is
// immovable AND something else really does need that time. The product rule is
// that a scheduling clash becomes a decision rather than being resolved
// quietly, because the system does not know which of the two matters more and
// guessing is how an assistant moves the thing the user cared about.
//
// It refuses to invent a clash: if nothing actually overlaps, there is nothing
// to decide, and a card that says "these two conflict" about two things that do
// not would teach the user to distrust the cards.
func (s *Server) handlePlanConflict(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	locale := s.requestLocale(r)
	var body struct {
		Date    string `json:"date"`
		BlockID string `json:"blockId"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Date == "" || body.BlockID == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.planConflict.bad_request")
		return
	}
	ctx := r.Context()
	blocks := s.planBlocksForDate(ctx, sid, body.Date)
	overlaps := schedule.Overlaps(blocks, body.Date, s.planLocation(ctx, sid))

	var hit *schedule.Overlap
	for i := range overlaps {
		if overlaps[i].A.ID == body.BlockID || overlaps[i].B.ID == body.BlockID {
			hit = &overlaps[i]
			break
		}
	}
	if hit == nil {
		s.writeErr(w, http.StatusUnprocessableEntity, "no_conflict", i18n.T(keyConflictNoClash, locale))
		return
	}

	mergeKey := "conflict:" + body.Date + ":" + hit.A.ID + ":" + hit.B.ID

	// Marking the same clash twice is one question, not two — but Supersede is
	// the wrong tool for saying so: it deliberately spares cards the user has
	// already seen (a card cannot be retired out from under someone looking at
	// it), and this one is delivered the moment it is created. So look first
	// and hand back what is already on screen.
	//
	// Scanning the stack in Go rather than adding a MergeKey filter dimension:
	// the stack is three cards by design, and a query dimension exists to keep
	// a LIMIT from cutting the wrong rows, which cannot happen at that size.
	if existing := s.liveCardWithMergeKey(ctx, sid, mergeKey); existing != nil {
		s.writeJSON(w, http.StatusOK, existing)
		return
	}

	p := &domain.Proposal{
		ID: "cf_" + uuid.NewString(), SessionID: sid,
		State: domain.ProposalPending, Level: domain.LevelL2, Kind: domain.KindCard,
		Title: i18n.T(keyConflictTitle, locale),
		Summary: i18n.Tf(keyConflictSummary, locale,
			hit.A.Title, hit.B.Title, hit.Start.Format("15:04"), hit.End.Format("15:04")),
		Date:      body.Date,
		TTLPolicy: domain.TTLSilenceRejects,
		Origin:    domain.OriginUser,
		MergeKey:  mergeKey,
		Rows: []domain.ProposalRow{
			{ID: "move_other", Label: i18n.T(keyConflictOptMove, locale), State: domain.ProposalPending},
			{ID: "skip_this", Label: i18n.T(keyConflictOptSkip, locale), State: domain.ProposalPending},
			{ID: "leave_both", Label: i18n.T(keyConflictOptStand, locale), State: domain.ProposalPending},
		},
	}
	p.ExpiresAt = domain.ProposalExpiry(p, time.Now(), s.planLocation(ctx, sid), nil)
	now := time.Now()
	p.DeliveredAt = &now
	if err := s.store.Proposals().Create(ctx, p); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.planConflict.internal")
		return
	}
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorUser, Action: "conflict_mark",
		TargetID: p.ID, Date: body.Date, Summary: p.Summary,
		Detail: marshalCompact(map[string]any{"a": hit.A.ID, "b": hit.B.ID}),
	})
	s.writeJSON(w, http.StatusOK, p)
}

// liveCardWithMergeKey returns the card already on screen for this merge key,
// or nil. "Live" is the stack conjunction — pending, delivered, not lapsed —
// because a settled or expired card is not an answer to the same question any
// more, and asking again should produce a fresh one.
func (s *Server) liveCardWithMergeKey(ctx context.Context, sid, mergeKey string) *domain.Proposal {
	items, err := s.store.Proposals().List(ctx, domain.ProposalFilter{
		SessionID: sid, State: domain.ProposalPending,
		Delivered: domain.PresenceSet, DeliverableAt: time.Now(),
	})
	if err != nil {
		return nil
	}
	for i := range items {
		if items[i].MergeKey == mergeKey {
			return &items[i]
		}
	}
	return nil
}
