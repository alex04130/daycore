package server

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/rapport"

	"github.com/google/uuid"
)

// 习惯扫描 → Rule 提案: the last of the three daemon producers.
//
// It notices that something keeps happening at the same time on the same
// weekday and offers to make it a standing rule, so the reader stops re-entering
// it every week.
//
// # ⚠️ It is a RULE, not an AI feeling
//
// STRATEGY §1.3 requires this to be 「可测可断言」, and there is a second reason
// beyond testability: the existing per-session model spend is two calls a day
// (the two briefs). A nightly scan that asked a model would be a 50% increase
// for every session that has ever opened the app, forever, to answer a question
// that counting can answer.
//
// So the predicate is arithmetic over the plan history and nothing else.
//
// # ⚠️ Why it is gated on rapport, and why that gate had no caller until now
//
// EXPERIENCE_CORE §8's first-cut rule: the system does not volunteer a STANDING
// change to somebody it has no standing with. A one-off suggestion is cheap to
// decline; a recurring rule is a claim about the shape of somebody's week, and
// getting that wrong early is how an assistant becomes something to be managed.
//
// internal/rapport exists to answer exactly this and had zero callers — see
// rapport_read.go. This is the caller.
//
// # Boundary: it proposes, it never writes a rule
//
// Accepting is what creates it, through the ordinary `rule_upsert` tool, so the
// rule is individually undoable like anything else. The scanner writing rules
// directly is the failure the whole proposal layer exists to prevent — and the
// specific version of it this repository already wrote down: 「从现有数据反推的
// 周期不是规则，不能拿去展开生成数据」.

const (
	// habitScanEvery runs it once a day, at the rhythm day cut, right after the
	// learner has finalised yesterday.
	//
	// ⚠️ Not more often. The input is "the last few weeks", which does not
	// change materially between breakfast and lunch, and a proposal about a
	// habit is not urgent by construction.
	habitLookbackWeeks = 4

	// habitMinOccurrences is how many times a thing must have happened, on the
	// same weekday, before it counts as a habit.
	//
	// ⚠️ THREE, and the number is load-bearing. Two is a coincidence — this
	// repository has already written down what happens when two occurrences are
	// treated as a period: 「2 次出现会被推成 by_weekday:[0,6] 然后铺满 28 天 ——
	// 一次开关凭空造出 7 节课」. Three is the smallest count that distinguishes a
	// pattern from a repeat.
	habitMinOccurrences = 3

	// habitSlackMinutes is how far apart two occurrences may start and still be
	// "the same time".
	//
	// People do not start things at exactly the same minute. Zero slack would
	// find almost no habits; an hour would call a morning and a lunchtime
	// occurrence the same thing.
	habitSlackMinutes = 30
)

var (
	keyHabitTitle = i18n.Reg("habit.title", i18n.Text{
		"zh-CN": "这件事好像每周都有",
		"en-US": "This looks like a weekly thing",
	})
	keyHabitBody = i18n.Reg("habit.body", i18n.Text{
		"zh-CN": "最近 %d 周里，「%s」有 %d 次都在%s %s 前后。要不要我把它记成每周的固定安排？以后就不用每次再加一遍了。",
		"en-US": "Over the last %d weeks, %s happened %d times around %s on %s. Shall I make it a standing weekly item, so you stop re-entering it?",
	})
	keyHabitYes = i18n.Reg("habit.opt.yes", i18n.Text{
		"zh-CN": "记成固定的",
		"en-US": "Make it standing",
	})
	keyHabitNo = i18n.Reg("habit.opt.no", i18n.Text{
		"zh-CN": "不用，我自己加",
		"en-US": "No, I will add it myself",
	})
	keyHabitWeekday = i18n.Reg("habit.weekday", i18n.Text{
		"zh-CN": "周日,周一,周二,周三,周四,周五,周六",
		"en-US": "Sunday,Monday,Tuesday,Wednesday,Thursday,Friday,Saturday",
	})
)

func init() {
	registerIrreversible("habit_offer",
		"the fact-track record that a repetition was noticed. No rule was created — accepting the card is what does that, through the ordinary rule_upsert tool, and that IS undoable. Undoing the observation would mean claiming the pattern was never there.")
}

// habit is a repetition worth offering to formalise.
type habit struct {
	title   string
	weekday time.Weekday
	minutes int // the median start, in minutes from midnight
	count   int
	dur     int
}

// runHabitScan offers at most one standing rule.
func (w *Worker) runHabitScan(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	if prefs.DoNotDisturb {
		return
	}

	// ⚠️ The first cut, before anything is computed. A session with no standing
	// in the schedule domain gets no standing-change proposals at all — and
	// checking it first also means the scan costs nothing for the readers it
	// would never speak to.
	scores := w.s.rapportScores(ctx, sid)
	gate := rapport.DefaultGate()
	if !gate.Allows(scores.Get(domain.OpDomainSchedule), habitConfidence).Allowed {
		return
	}

	// ⚠️ Before the scan, not after: the ceiling is about how much is waiting,
	// so a full stack means there is nothing to compute.
	if !w.s.stackHasRoom(ctx, sid) {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now()
	found, ok := w.scanForHabit(ctx, sid, now, loc)
	if !ok {
		return
	}

	// ⚠️ Keyed on the habit, not on the day. Offering the same weekly thing
	// again tomorrow because the date changed is precisely the nag this is
	// supposed to remove.
	runKey := "habit:" + strings.ToLower(found.title) + ":" + found.weekday.String()
	jr, ok := w.claim(ctx, sid, domain.JobHabitScan, runKey)
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, jr, jobErr) }()

	locale := w.s.localePair(ctx, sid).Resolve("", "")
	at := hmOf(found.minutes)
	weekday := weekdayName(locale, found.weekday)

	p := &domain.Proposal{
		ID: "hb_" + uuid.NewString(), SessionID: sid,
		State: domain.ProposalPending, Level: domain.LevelL2, Kind: domain.KindCard,
		Origin: domain.OriginDaemon,
		Title:  i18n.T(keyHabitTitle, locale),
		Summary: i18n.Tf(keyHabitBody, locale,
			habitLookbackWeeks, found.title, found.count, weekday, at),
		// ⚠️ Evidence is filled because an apprentice-phase score requires the
		// card to show its working (rapport.Score.ShowEvidence). It is the
		// counting, not a rationale: a claim about somebody's week should be
		// checkable by them.
		Reason:    i18n.Tf(keyHabitBody, locale, habitLookbackWeeks, found.title, found.count, weekday, at),
		Evidence:  found.title + " ×" + strconv.Itoa(found.count),
		TTLPolicy: domain.TTLSilenceRejects,
		MergeKey:  "habit:" + runKey,
		Rows: []domain.ProposalRow{
			{
				ID: "make_rule", Label: i18n.T(keyHabitYes, locale), State: domain.ProposalPending,
				Ops: []domain.ProposalOp{{
					Tool: "rule_upsert",
					Args: map[string]any{
						"title": found.title, "type": "task",
						"time": at, "duration_min": found.dur,
						"kind": "recurring", "freq": "weekly",
						"by_weekday": []any{int(found.weekday)},
						// ⚠️ start_date is today, not the first occurrence seen.
						// A rule dated into the past would expand backwards over
						// days that already happened and have their own record —
						// the plan reader merges occurrences in, so history would
						// grow events nobody lived.
						"start_date": now.In(loc).Format("2006-01-02"),
					},
				}},
			},
			{ID: "no", Label: i18n.T(keyHabitNo, locale), State: domain.ProposalPending},
		},
	}
	p.ExpiresAt = domain.ProposalExpiry(p, now, loc, nil)
	// Queued: it runs at the day cut, in the middle of the night.
	if err := w.s.store.Proposals().Create(ctx, p); err != nil {
		jobErr = err
		w.log.Warn("habit scan: could not create the card", "sid", sid, "err", err)
		return
	}
	w.s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorSystem, Action: "habit_offer",
		TargetID: p.ID, Summary: found.title,
		Detail: marshalCompact(map[string]any{
			"title": found.title, "weekday": int(found.weekday),
			"at": at, "count": found.count, "weeks": habitLookbackWeeks,
		}),
	})
	w.log.Info("habit offer", "sid", sid, "title", found.title, "count", found.count)
}

// habitConfidence is how sure the scanner claims to be.
//
// ⚠️ A constant rather than a computed number, and deliberately modest. The
// gate multiplies standing by confidence; inventing a confidence from the
// occurrence count would let a coincidence that happened five times outrank the
// reader's actual history with the assistant.
const habitConfidence = 0.6

// scanForHabit looks for one repetition in the plan history.
//
// ⚠️ It reads the STORED plans, not the merged view. A rule's own occurrences
// are merged in when a day is read, so scanning the merged view would find the
// rules that already exist and offer to create them again — every week, forever.
func (w *Worker) scanForHabit(ctx context.Context, sid string, now time.Time, loc *time.Location) (habit, bool) {
	to := now.In(loc)
	from := to.AddDate(0, 0, -7*habitLookbackWeeks)
	plans, err := w.s.store.DayPlans().Range(ctx, sid,
		from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil || len(plans) == 0 {
		return habit{}, false
	}

	type key struct {
		title   string
		weekday time.Weekday
	}
	starts := map[key][]int{}
	durs := map[key][]int{}
	for _, plan := range plans {
		day, err := time.ParseInLocation("2006-01-02", plan.Date, loc)
		if err != nil {
			continue
		}
		for _, b := range plan.Blocks {
			// ⚠️ Rule-sourced blocks are skipped. They are the expansion of a
			// rule that already exists; counting them would make every standing
			// item look like a habit waiting to be formalised.
			if b.Hidden || b.RuleID != "" || b.Time == nil {
				continue
			}
			h, m, ok := splitHM(*b.Time)
			if !ok {
				continue
			}
			k := key{title: strings.TrimSpace(b.Title), weekday: day.Weekday()}
			if k.title == "" {
				continue
			}
			starts[k] = append(starts[k], h*60+m)
			if b.DurationMin != nil {
				durs[k] = append(durs[k], *b.DurationMin)
			}
		}
	}

	best := habit{}
	for k, mins := range starts {
		if len(mins) < habitMinOccurrences {
			continue
		}
		sort.Ints(mins)
		mid := mins[len(mins)/2]
		// ⚠️ Counted against the MEDIAN rather than the mean: one occurrence at
		// a wildly different hour would drag a mean far enough to disqualify a
		// pattern that is otherwise obvious, and it is exactly the kind of
		// one-off this is meant to look past.
		n := 0
		for _, v := range mins {
			if abs(v-mid) <= habitSlackMinutes {
				n++
			}
		}
		if n < habitMinOccurrences {
			continue
		}
		// ⚠️ Ties broken toward the MORE frequent one, then alphabetically. Map
		// iteration order is random, and a card whose subject changes between
		// runs for the same input is one nobody can reason about.
		if n > best.count || (n == best.count && k.title < best.title) {
			best = habit{title: k.title, weekday: k.weekday, minutes: mid, count: n, dur: medianOf(durs[k])}
		}
	}
	if best.count < habitMinOccurrences {
		return habit{}, false
	}
	if best.dur == 0 {
		best.dur = 60
	}
	return best, true
}

func weekdayName(locale string, d time.Weekday) string {
	names := strings.Split(i18n.T(keyHabitWeekday, locale), ",")
	if int(d) < len(names) {
		return names[d]
	}
	return d.String()
}

func medianOf(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	sort.Ints(xs)
	return xs[len(xs)/2]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
