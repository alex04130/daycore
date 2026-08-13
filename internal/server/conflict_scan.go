package server

import (
	"context"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/schedule"

	"github.com/google/uuid"
)

// 排程冲突 → 决策提案, the background half.
//
// # ⚠️ The foreground half already existed, and ROADMAP overstated the gap
//
// 「排程冲突 → 决策提案」 is listed as a chain 「缺生产者」, and that is not quite
// true: POST /api/plan/conflict has produced these cards since the lock work
// landed. What was missing is the version nobody has to ask for.
//
// # ⚠️ Which is why this one is NARROWER than the endpoint
//
// The endpoint reports any overlap the reader points at. A background scan that
// did the same would open a card for every clash a person can see perfectly
// well and is halfway through resolving — and handlePlanConflict's own comment
// names the cost: 「a card that says "these two conflict" about two things that
// do not would teach the user to distrust the cards」. Firing on clashes they
// already understand does the same damage more slowly.
//
// So the background scan speaks only when at least one side is LOCKED. A locked
// block is one the reader cannot move themselves — the timetable decides it —
// so a clash involving one is the case where "just drag it" is not available
// and an offer to decide is worth something.
//
// # ⚠️ Origin is daemon, and that is not cosmetic
//
// domain/proposal.go says the origin distinction counts toward rapport: a card
// the user summoned and one the system volunteered are judged differently when
// it comes to trust. Copying OriginUser here because the shape is the same
// would quietly credit the daemon's guesses to the reader's own request.
//
// It shares the endpoint's merge key on purpose, so the two halves cannot both
// open a card about the same pair.

// conflictScanEvery is how often the scan runs. Hourly: the input only changes
// when the plan does, and a clash that appears at 09:05 is not more urgent at
// 09:10 than at 10:00 — while a tighter interval multiplies the claim writes by
// the number of sessions for a predicate that is almost always false.
const conflictScanEvery = "7 * * * *"

var keyConflictScanSummary = i18n.Reg("plan.conflict.scan.summary", i18n.Text{
	"zh-CN": "「%s」和「%s」都要 %s–%s，而其中一件是课表定的、挪不动。想怎么办？",
	"en-US": "%s and %s both want %s–%s, and one of them is fixed by the timetable. What would you like to do?",
})

func init() {
	registerIrreversible("conflict_scan",
		"the fact-track record that a clash was noticed on its own. It writes nothing to the plan — the card it opens is answered like any other, and the moves an answer performs are individually undoable. Undoing the observation would mean claiming it was never seen.")
}

// runConflictScan opens at most one card for today's most pressing clash.
//
// ⚠️ One card, not one per clash. A day that has come apart has many overlaps
// and wants a conversation, not a stack — and 「L2 同时最多堆叠 3 条」 is the
// ceiling for every producer together.
func (w *Worker) runConflictScan(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	// RollingReplan is the preference this belongs under: 「计划被打乱时帮我
	// 重排」. A clash IS the plan coming apart, and somebody who turned that off
	// has said they would rather sort it out themselves.
	if !prefs.RollingReplan || prefs.DoNotDisturb {
		return
	}

	if !w.s.stackHasRoom(ctx, sid) {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now()
	date := now.In(loc).Format("2006-01-02")
	blocks := w.s.planBlocksForDate(ctx, sid, date)

	hit, ok := firstLockedClashAhead(schedule.Overlaps(blocks, date, loc), now)
	if !ok {
		return
	}
	mergeKey := "conflict:" + date + ":" + hit.A.ID + ":" + hit.B.ID
	// ⚠️ Checked BEFORE claiming, so a clash the reader already has on screen
	// does not burn an occurrence. The endpoint does the same thing for the same
	// reason, and sharing the key is what makes the two halves one question.
	if existing := w.s.liveCardWithMergeKey(ctx, sid, mergeKey); existing != nil {
		return
	}

	// ⚠️ The occurrence key is the PAIR, not the day. Two separate clashes on one
	// day are two questions; keying on the date would let the first one answered
	// silence the second forever.
	jr, ok := w.claim(ctx, sid, domain.JobConflictScan, mergeKey)
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, jr, jobErr) }()

	locale := w.s.localePair(ctx, sid).Resolve("", "")
	p := &domain.Proposal{
		ID: "cf_" + uuid.NewString(), SessionID: sid,
		State: domain.ProposalPending, Level: domain.LevelL2, Kind: domain.KindCard,
		Title: i18n.T(keyConflictTitle, locale),
		Summary: i18n.Tf(keyConflictScanSummary, locale,
			hit.A.Title, hit.B.Title, hit.Start.In(loc).Format("15:04"), hit.End.In(loc).Format("15:04")),
		Date:      date,
		TTLPolicy: domain.TTLSilenceRejects,
		Origin:    domain.OriginDaemon,
		MergeKey:  mergeKey,
		Rows: []domain.ProposalRow{
			// ⚠️ The movable side is the one offered for moving, and which side
			// that is has already been decided by the predicate: a locked block
			// cannot be it. Offering "move the other one" without knowing which
			// one can move is how a card promises something the plan gate will
			// then refuse with a 409.
			{
				ID: "move_other", Label: i18n.T(keyConflictOptMove, locale), State: domain.ProposalPending,
				Ops: []domain.ProposalOp{{
					Tool: "plan_update",
					Args: map[string]any{
						"date":    date,
						"match":   map[string]any{"id": movableSide(hit).ID},
						"changes": map[string]any{"time": hmOf(minutesOf(hit.End.In(loc)))},
					},
				}},
			},
			{ID: "skip_this", Label: i18n.T(keyConflictOptSkip, locale), State: domain.ProposalPending},
			{ID: "leave_both", Label: i18n.T(keyConflictOptStand, locale), State: domain.ProposalPending},
		},
	}
	p.ExpiresAt = domain.ProposalExpiry(p, now, loc, nil)
	// ⚠️ Queued rather than delivered, unlike the endpoint's card. That one is a
	// direct answer to something the reader just did, so it belongs on screen
	// now; this one was nobody's question and arrives when they next look.
	if err := w.s.store.Proposals().Create(ctx, p); err != nil {
		jobErr = err
		w.log.Warn("conflict scan: could not create the card", "sid", sid, "err", err)
		return
	}
	w.s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorSystem, Action: "conflict_scan",
		TargetID: p.ID, Date: date, Summary: p.Summary,
		Detail: marshalCompact(map[string]any{"a": hit.A.ID, "b": hit.B.ID}),
	})
	w.log.Info("conflict scan card", "sid", sid, "date", date, "a", hit.A.ID, "b", hit.B.ID)
}

// firstLockedClashAhead picks the earliest clash that is still ahead and has a
// side the reader cannot move.
//
// ⚠️ "Still ahead" matters as much as "locked". A clash whose contested minutes
// are already behind the reader is a fact about how the morning went, not a
// decision waiting to be made — and 「过去不改写」 means the answer to it is
// nothing. Offering to move something that has already happened is the kind of
// suggestion that reads as the assistant not having been paying attention.
func firstLockedClashAhead(overlaps []schedule.Overlap, now time.Time) (schedule.Overlap, bool) {
	best := schedule.Overlap{}
	found := false
	for _, o := range overlaps {
		if !o.End.After(now) {
			continue
		}
		aLocked := o.A.LockLevel == domain.LockHard || o.A.LockLevel == domain.LockSoft
		bLocked := o.B.LockLevel == domain.LockHard || o.B.LockLevel == domain.LockSoft
		// ⚠️ Exactly one side locked. Neither locked is a clash the reader can
		// resolve by dragging, and BOTH locked is one this card cannot help
		// with: there is nothing to offer to move, so the only honest row would
		// be "leave both", which is what happens anyway if nobody is asked.
		if aLocked == bLocked {
			continue
		}
		if !found || o.Start.Before(best.Start) {
			best, found = o, true
		}
	}
	return best, found
}

// movableSide is whichever block of a clash is not locked.
func movableSide(o schedule.Overlap) domain.TimeBlock {
	if o.A.LockLevel == domain.LockHard || o.A.LockLevel == domain.LockSoft {
		return o.B
	}
	return o.A
}

func minutesOf(t time.Time) int {
	m := t.Hour()*60 + t.Minute()
	// Same clamp as the Protector's, for the same reason: a move that would
	// spill past midnight lands at the end of the day rather than in the small
	// hours of a date this op does not name.
	if m > 23*60+30 {
		m = 23*60 + 30
	}
	return m
}
