package server

import (
	"context"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/mood"
	"daycore/internal/rhythm"

	"github.com/google/uuid"
)

// The Protector: the 20-hour care nudge.
//
// EXPERIENCE_CORE §5: 「连续活跃 ≈20 小时触发 Protector——『你快 20 小时没合眼
// 了，上午的安排我先帮你顺延，去睡一会？』值得花一条推送预算，语气是担心不是
// 训斥。顺延本身走提案。」 Consensus 28 adds that it is 「与任何日界线机制无关，
// 永久保留」 — it keys off how long this person has been up, and off nothing
// else. Not midnight, not the rhythm day cut, not the petrify line.
//
// # The occurrence key is the START OF THE RUN, and nothing else works
//
//	dayKey   a 30-hour stretch that crosses local midnight fires twice
//	slotKey  it fires every half hour
//	run start exactly once per stretch of being awake — which is what
//	         "the same long night" means
//
// Formatted in UTC. RunSince is an absolute instant; formatting it in the
// session's zone would give a stretch that spans a DST change a new key and fire
// again for the same night.
//
// # What it does NOT decide
//
// rhythm.Run.NeedsProtector is a pure predicate and says so in its own doc: the
// push budget, the attention ladder, and "did one already go out for this run"
// all belong to the caller. All three live here.

// protectorEvery is how often the check runs. The predicate is a threshold on a
// continuously growing number, so the interval only decides how late the nudge
// can be — half an hour against a twenty-hour run.
const protectorEvery = "*/30 * * * *"

var (
	keyProtectorTitle = i18n.Reg("protector.title", i18n.Text{
		"zh-CN": "要不要眯一会？",
		"en-US": "Maybe lie down for a bit?",
	})
	// Two tones, chosen by the same mood window six other features read. Not a
	// second window computed here — DATA.md names this as one of its consumers.
	keyProtectorBody = i18n.Reg("protector.body", i18n.Text{
		"zh-CN": "你快 %d 小时没合眼了。上午的安排我先帮你顺延，去睡一会，醒来再说。",
		"en-US": "You have been up for nearly %d hours. I will push this morning's plan back — get some sleep, the rest can wait.",
	})
	keyProtectorBodyGentle = i18n.Reg("protector.body.gentle", i18n.Text{
		"zh-CN": "已经 %d 小时了。今天上午的事我先往后挪，先去躺一会吧，剩下的醒了再说。",
		"en-US": "That is %d hours now. I am moving this morning's things back — go and lie down, the rest can wait until you are up.",
	})
	keyProtectorNoPlan = i18n.Reg("protector.body.noplan", i18n.Text{
		"zh-CN": "你快 %d 小时没合眼了。没什么非现在不可的事，去睡一会吧。",
		"en-US": "You have been up for nearly %d hours. Nothing here needs you right now — go and sleep.",
	})
	keyProtectorOptYes = i18n.Reg("protector.opt.postpone", i18n.Text{
		"zh-CN": "好，帮我挪",
		"en-US": "Yes, move them",
	})
	keyProtectorOptNo = i18n.Reg("protector.opt.leave", i18n.Text{
		"zh-CN": "不用管我",
		"en-US": "Leave it",
	})
)

func init() {
	registerIrreversible("protector_nudge",
		"the fact-track record that a care nudge went out. The postponement it offers is a proposal, and accepting that writes its own individually-undoable operations; undoing the nudge itself would mean claiming nobody was ever told.")
}

// checkProtector nudges a session that has been awake too long.
//
// Order is load-bearing and matches the other jobs: suppression gates first,
// then the predicate, then claim, then work. Claiming earlier would write a
// job_runs row every half hour for every session to record that nothing
// happened — which is exactly what JobRunRepository.Claim warns against.
func (w *Worker) checkProtector(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	if prefs.DoNotDisturb {
		// Do Not Disturb suppresses the PUSH, and here it suppresses the whole
		// nudge. A care card that appears silently in the app at 4am, unread
		// until morning, says "you were up too long" to somebody for whom that is
		// now just a fact about yesterday.
		return
	}

	prof, err := w.s.store.Rhythm().Get(ctx, sid)
	if err != nil || prof == nil {
		return // no row yet: nothing has ever marked this session awake
	}
	cfg := rhythmConfig()
	now := time.Now()
	// Live.Run, not CurrentRun: the O(1) path reads the two marks the store
	// keeps. CurrentRun wants a signal list, which storage deliberately does not
	// retain.
	run := rhythm.Live{RunSince: prof.RunSince, LastSignalAt: prof.LastSignalAt}.Run(now, cfg)
	if !run.NeedsProtector(cfg) {
		return
	}

	runKey := "run:" + run.Since.UTC().Format("2006-01-02T15:04")
	jr, ok := w.claim(ctx, sid, domain.JobProtector, runKey)
	if !ok {
		return // already nudged for this stretch, or somebody else is doing it
	}
	var jobErr error
	defer func() { w.finish(ctx, jr, jobErr) }()

	locale := w.s.localePair(ctx, sid).Resolve("", "")
	loc := resolveLocation(tz)
	hours := int(run.Continuous.Hours())
	movable := w.morningBlocksToPostpone(ctx, sid, now, loc)

	body := i18n.Tf(keyProtectorBody, locale, hours)
	switch {
	case len(movable) == 0:
		body = i18n.Tf(keyProtectorNoPlan, locale, hours)
	case w.s.moodWindow(ctx, sid).Tone() == mood.ToneGentle:
		// The same window six other features read (DATA.md lists this as one of
		// its consumers). Not a second computation — two windows that disagree
		// would have the assistant sound cheerful in one sentence and careful in
		// the next.
		body = i18n.Tf(keyProtectorBodyGentle, locale, hours)
	}

	p := &domain.Proposal{
		ID: "pr_" + uuid.NewString(), SessionID: sid,
		// L3: the ladder's own definition of L3 is "a push, budget ≤3/day", and
		// §5 says this nudge is worth spending one. It was L2 and pushed anyway,
		// which made the level a label rather than a statement.
		State: domain.ProposalPending, Level: domain.LevelL3, Kind: domain.KindCard,
		Origin: domain.OriginProtector,
		Title:  i18n.T(keyProtectorTitle, locale), Summary: body,
		// Ask first, and the card moves nothing until it is answered.
		//
		// The design mock is in the past tense ("我先帮你顺延了") = act-first. The
		// alternative considered and rejected was "move provisionally, revert if
		// unanswered", which sounds gentler and is not: the plan would change
		// twice with the user doing nothing, so what they saw at 09:00 is not
		// what is there at 10:00, and the ledger carries a move and a revert for
		// something they never touched. Ask-first keeps one promise instead —
		// the plan changes only after they press something.
		//
		// Flipping it is one constant plus a producer for AppliedOpIDs (an
		// act-first card must name what it already did, and the store enforces
		// that).
		TTLPolicy: domain.TTLSilenceRejects,
		// One card per stretch, so a second nudge cannot stack on the first.
		MergeKey: "protector:" + runKey,
		Rows: []domain.ProposalRow{
			{ID: "postpone", Label: i18n.T(keyProtectorOptYes, locale), State: domain.ProposalPending},
			{ID: "leave", Label: i18n.T(keyProtectorOptNo, locale), State: domain.ProposalPending},
		},
	}
	p.ExpiresAt = domain.ProposalExpiry(p, now, loc, nil)
	// Queued, NOT delivered. It fires at four in the morning because that is when
	// somebody has been up for twenty hours; stamping it delivered right then
	// makes it a card "shown" at 4am and read at noon, when it is a remark about
	// yesterday. It becomes visible on the user's next visit, and if it lapses
	// before that it is voided having never been shown — which is honest, because
	// the moment it was about has passed. See proposal_delivery.go.
	if err := w.s.store.Proposals().Create(ctx, p); err != nil {
		jobErr = err
		w.log.Warn("protector: could not create the card", "sid", sid, "err", err)
		return
	}

	// The fact track: what happened, not what was suggested.
	w.s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorSystem, Action: "protector_nudge",
		TargetID: p.ID, Summary: p.Title,
		Detail: marshalCompact(map[string]any{
			"hours": hours, "runSince": run.Since.UTC(), "movable": len(movable),
		}),
	})

	// Worth a push budget entry, says §5. Spending it is the caller's call, and
	// the budget is shared with everything else that pushes today.
	//
	// ⚠️ This card is SOFT-backed (BackingOf → soft: it is something the
	// assistant noticed, not a commitment anybody made), so it gets one delivery
	// and never comes back louder. It reaches L3 anyway because L3 is about
	// medium — "this is worth a push" — while backing is about whether it may
	// ESCALATE. The two are independent and this is the case that shows it.
	if w.spendPushBudget(ctx, sid, now, loc) {
		w.sendToChannels(ctx, sid, p.Title+"\n"+body)
		pushed := time.Now()
		p.PushedAt = &pushed
		if err := w.s.store.Proposals().Update(ctx, p); err != nil {
			w.log.Warn("protector: pushed but could not stamp the card", "sid", sid, "err", err)
		}
	}
	w.log.Info("protector nudge", "sid", sid, "hours", hours, "movable", len(movable))
}

// morningBlocksToPostpone lists what the card is offering to move.
//
// Only blocks that are movable AND rescheduleable: Refishable() carries the
// RescheduleCap that stops the same thing being pushed forward forever, which is
// the anti-shame limit, and a locked block is one the timetable decides. It is
// read-only — the card offers, accepting it performs the moves through the
// ordinary plan path so each one is individually undoable.
func (w *Worker) morningBlocksToPostpone(ctx context.Context, sid string, now time.Time, loc *time.Location) []domain.TimeBlock {
	date := now.In(loc).Format("2006-01-02")
	var out []domain.TimeBlock
	for _, b := range w.s.planBlocksForDate(ctx, sid, date) {
		if b.Completed || b.Hidden || b.Time == nil || !b.Refishable() {
			continue
		}
		if b.LockLevel == domain.LockHard || b.LockLevel == domain.LockSoft {
			continue
		}
		t, err := parseBlockTime(date, *b.Time, loc)
		if err != nil || !t.After(now) {
			continue
		}
		// "This morning" — anything before noon that has not happened yet.
		if t.In(loc).Hour() >= 12 {
			continue
		}
		out = append(out, b)
	}
	return out
}

// spendPushBudget reports whether there is a push left today.
//
// Counts what has already been pushed in this session's own day window rather
// than keeping a counter: the count is derivable from rows that exist for other
// reasons, and a counter would be a second source of truth to get out of sync.
func (w *Worker) spendPushBudget(ctx context.Context, sid string, now time.Time, loc *time.Location) bool {
	start, end := domain.PushBudgetWindow(now, loc)
	n, err := w.s.store.Proposals().Count(ctx, domain.ProposalFilter{
		SessionID: sid, Pushed: domain.PresenceSet,
		PushedSince: start, PushedBefore: end,
	})
	if err != nil {
		// Unknown budget: do not push. A care nudge that arrives on top of an
		// already-noisy day is the failure this budget exists to prevent, and the
		// card is in the app either way.
		w.log.Debug("protector: could not read the push budget", "sid", sid, "err", err)
		return false
	}
	return n < domain.PushBudgetPerDay
}
