package server

import (
	"context"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/schedule"

	"github.com/google/uuid"
)

// 愿望池填缝: the first daemon producer.
//
// EXPERIENCE_CORE's daemon chain is 「习惯扫描 → Rule 提案、排程冲突 → 决策提案、
// 愿望池填缝」, and until now all three were 「机制已就位，缺的是生产者」 — the
// proposals table, the delivery pass, the expiry sweep and the ladder all
// existed with exactly two writers, both of them synchronous and both triggered
// by a person.
//
// This one is first because it is the cheapest honest producer: it touches no
// model, needs no rapport wiring, and its input (the wish pool with its
// EffortMin estimate) has had a writer since the capture tools shipped and a
// reader nowhere. `wish_add`'s own tool description says the estimate is 「用于
// 填缝匹配」 — this is the matching half.
//
// # ⚠️ It runs at PlanAt, which had no consumer either
//
// rhythm.Jobs.PlanAt is 「deep in the quiet window, shortly before they get
// up」. Two things follow. The card is written while they are asleep, so it must
// be QUEUED rather than delivered — see the note on DeliveredAt below. And the
// gap it points at is in the day about to start, not the one just ending.
//
// # Boundary: one card, one gap, one wish
//
// Deliberately not "fill every gap". A day with four openings does not want four
// cards — 「L2 同时最多堆叠 3 条」 is the ceiling for everything together, and a
// single producer that can spend the whole allowance on its own leaves nothing
// for the two that matter more. One suggestion is also the honest shape of the
// thing: it is an offer, and an offer repeated four times is a nag.

const (
	// wishGapMin is the smallest opening worth offering something for.
	//
	// ⚠️ 数值即产品. Below this the suggestion is not a kindness — a 20-minute
	// slot between two things is when somebody walks between rooms, and filling
	// it is how an assistant becomes the thing that never lets you sit down.
	wishGapMin = 45

	// wishSlack is how much of the gap stays empty around the suggestion.
	//
	// A wish that exactly fills its opening leaves no room to start late or run
	// over, so it produces a plan that is wrong the moment anything slips.
	wishSlack = 10
)

var (
	keyWishFillTitle = i18n.Reg("wishfill.title", i18n.Text{
		"zh-CN": "有段空着的时间",
		"en-US": "There is an opening",
	})
	keyWishFillBody = i18n.Reg("wishfill.body", i18n.Text{
		"zh-CN": "%s–%s 空着。要不要把「%s」放进去？不想也没关系。",
		"en-US": "%s–%s is free. Shall I put %s there? No is fine.",
	})
	keyWishFillYes = i18n.Reg("wishfill.opt.yes", i18n.Text{
		"zh-CN": "放进去",
		"en-US": "Put it there",
	})
	keyWishFillNo = i18n.Reg("wishfill.opt.no", i18n.Text{
		"zh-CN": "先不用",
		"en-US": "Not now",
	})
)

func init() {
	registerIrreversible("wishfill_offer",
		"the fact-track record that an opening was pointed out. Nothing was written to the plan — accepting the card is what does that, and the block it adds is individually undoable. Undoing the offer itself would mean claiming nobody was ever asked.")
}

// runWishFill offers one wish for one opening in the day about to start.
//
// Order matches every other job and is load-bearing: suppression gates, then the
// predicate, then claim, then work. Claiming first would write a job_runs row
// every night for every session to record that there was nothing to suggest.
func (w *Worker) runWishFill(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	// ⚠️ GapSuggestions is the switch this feature is named after in the
	// preferences (「空出一段时间时给点建议」). DoNotDisturb is checked too even
	// though nothing is pushed: the card would still be sitting there in the
	// morning, and somebody who asked not to be disturbed did not mean
	// "disturb me later".
	if !prefs.GapSuggestions || prefs.DoNotDisturb {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now()
	// The day ABOUT TO START. PlanAt is before the wake time, so "today" in the
	// session's zone is already the right date — but only just, and that is
	// worth saying out loud: a job at 04:00 local reasons about the same
	// calendar day it fires on, while one at 23:00 would not.
	date := now.In(loc).Format("2006-01-02")

	wishes, err := w.s.store.Wishes().List(ctx, sid, domain.WishActive)
	if err != nil || len(wishes) == 0 {
		return
	}
	blocks := w.s.planBlocksForDate(ctx, sid, date)
	jobs := w.jobsFor(ctx, sid)
	dayStart, ok1 := atHM(date, jobs.BriefAt, loc)
	dayEnd, ok2 := atHM(date, jobs.ReviewAt, loc)
	if !ok1 || !ok2 {
		return
	}
	gap, found := schedule.LongestGap(schedule.Gaps(blocks, date, loc, dayStart, dayEnd), wishGapMin)
	if !found {
		return
	}
	wish, ok := pickWishForGap(wishes, gap.Minutes()-wishSlack)
	if !ok {
		return
	}

	// ⚠️ The occurrence key names the DAY and this producer, not the gap. A gap
	// that moves because the reader edited their morning is still the same day's
	// one offer — keying on the slot would let an edit at 09:00 produce a second
	// card for what is, to the reader, the same suggestion.
	runKey := "wishfill:" + date
	jr, ok := w.claim(ctx, sid, domain.JobWishFill, runKey)
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, jr, jobErr) }()

	locale := w.s.localePair(ctx, sid).Resolve("", "")
	start := gap.Start.In(loc).Format("15:04")
	end := gap.End.In(loc).Format("15:04")
	dur := wish.EffortMin
	if dur <= 0 || dur > gap.Minutes()-wishSlack {
		dur = gap.Minutes() - wishSlack
	}

	p := &domain.Proposal{
		ID: "pr_" + uuid.NewString(), SessionID: sid,
		State: domain.ProposalPending,
		// ⚠️ L1, and this is the level's first producer. §4: an L1 ghost occupies
		// the slot it is about and dies when that slot begins — it is a shape on
		// the canvas rather than a notification, which is exactly what "there is
		// room here" should be. Nothing is pushed.
		Level: domain.LevelL1, Kind: domain.KindTimed,
		Origin:  domain.OriginDaemon,
		Title:   i18n.T(keyWishFillTitle, locale),
		Summary: i18n.Tf(keyWishFillBody, locale, start, end, wish.Title),
		// The ghost's own coordinates. KindTimed requires both.
		Date: date, Start: start, Dur: &dur,
		TTLPolicy: domain.TTLSilenceRejects,
		MergeKey:  "wishfill:" + date,
		Rows: []domain.ProposalRow{
			{
				ID: "place", Label: i18n.T(keyWishFillYes, locale), State: domain.ProposalPending,
				Ops: []domain.ProposalOp{{
					Tool: "plan_add",
					Args: map[string]any{
						"date": date, "time": start, "title": wish.Title,
						"duration_min": dur, "type": "task",
					},
				}},
			},
			{ID: "skip", Label: i18n.T(keyWishFillNo, locale), State: domain.ProposalPending},
		},
	}
	p.ExpiresAt = domain.ProposalExpiry(p, now, loc, nil)
	// ⚠️ Queued, not delivered — the same reason the Protector's card is. This
	// runs before the reader is awake; stamping it delivered would make it a card
	// "shown" at four in the morning and read at eight. If the slot begins before
	// they open the app, it lapses having never been shown, which is honest: the
	// opening it was about has closed.
	if err := w.s.store.Proposals().Create(ctx, p); err != nil {
		jobErr = err
		w.log.Warn("wishfill: could not create the card", "sid", sid, "err", err)
		return
	}
	w.s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorSystem, Action: "wishfill_offer",
		TargetID: p.ID, Summary: wish.Title,
		Detail: marshalCompact(map[string]any{
			"date": date, "gapStart": start, "gapEnd": end,
			"gapMinutes": gap.Minutes(), "wishId": wish.ID,
		}),
	})
	w.log.Info("wishfill offer", "sid", sid, "date", date, "gap", gap.Minutes(), "wish", wish.Title)
}

// pickWishForGap chooses which wish to offer for an opening of maxMinutes.
//
// ⚠️ The BIGGEST wish that fits, not the oldest or the smallest.
//
// Smallest-first would spend a two-hour opening on a ten-minute errand and leave
// the thing somebody actually wants to do still waiting. Oldest-first sounds
// fair and is worse: the pool's oldest entry is often the one that has been
// declined implicitly for weeks, and offering it again every time a gap appears
// is how a suggestion becomes a reproach.
//
// ⚠️ A wish with no estimate is eligible and sorts last. EffortMin is written by
// the capture tool and is often absent; excluding those would make the pool's
// least-annotated half permanently invisible, and "I do not know how long it
// takes" is not "it does not fit".
func pickWishForGap(wishes []domain.Wish, maxMinutes int) (domain.Wish, bool) {
	if maxMinutes <= 0 {
		return domain.Wish{}, false
	}
	var best domain.Wish
	found := false
	for _, wi := range wishes {
		if wi.EffortMin > maxMinutes {
			continue
		}
		if !found || wi.EffortMin > best.EffortMin {
			best, found = wi, true
		}
	}
	return best, found
}

// atHM builds the instant at HH:MM of a plan date in a zone.
func atHM(date, hm string, loc *time.Location) (time.Time, bool) {
	h, m, ok := splitHM(hm)
	if !ok {
		return time.Time{}, false
	}
	d, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, false
	}
	return d.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute), true
}
