// Package server — background worker for proactive features: morning/evening
// briefs, deadline warnings, rolling replan checks, and auto-plan on first open.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"daycore/internal/ai"
	"daycore/internal/channels"
	"daycore/internal/domain"
	"daycore/internal/i18n"

	"github.com/robfig/cron/v3"
)

// Worker runs background scheduled tasks on a per-user-timezone cadence.
type Worker struct {
	cron *cron.Cron
	s    *Server
	log  *slog.Logger
	mu   sync.Mutex
	// jobs holds every cron entry a session owns, and sched remembers which
	// timezone they were built for.
	//
	// Keyed by session id alone, NOT by "sid:tz". The previous shape kept one
	// EntryID per (sid, tz, job) and looked up "sid:tz" to decide whether the
	// session was already scheduled — a key nothing ever wrote, so the guard was
	// dead and every admission through markAwake added four more entries. Keying
	// by session is also what makes a timezone change removable: the old
	// entries are found by the same key that replaces them.
	jobs     map[string][]cron.EntryID
	sched    map[string]string  // session_id → timezone its entries were built for
	channels *channels.Registry // nil when channels are not wired
}

// NewWorker creates a background worker. Call Start() to begin.
// If chReg is nil, channel-delivered messages will only be logged.
func NewWorker(s *Server, chReg *channels.Registry) *Worker {
	return &Worker{
		cron:     cron.New(cron.WithLocation(time.UTC)),
		s:        s,
		log:      s.log,
		jobs:     make(map[string][]cron.EntryID),
		sched:    make(map[string]string),
		channels: chReg,
	}
}

// Start begins the cron scheduler.
func (w *Worker) Start() {
	// A degraded process has no store, and every job this schedules reads rows.
	//
	// Nothing fires today, but only by accident: cron entries are added by
	// ScheduleUser, which runs off markAwake, which sits behind requireSession —
	// and degradedMW refuses every route that requires a session. So the worker
	// is idle because a middleware three layers away happens to keep requests
	// from reaching it.
	//
	// That accident has an expiry date written into docs/ROADMAP.md: enumerating
	// sessions at boot is a stated future want, and the day somebody adds it the
	// worker starts running jobs against a nil store in a degraded process.
	// Refusing here costs one branch and makes the property explicit instead of
	// emergent.
	if w.s != nil && w.s.Degraded() {
		w.s.log.Warn("degraded: proactive jobs are not scheduled (they all read rows); restart after fixing storage")
		return
	}
	w.cron.Start()
}

// Stop gracefully shuts down the scheduler and waits for running jobs to
// finish (cron.Stop returns a context that closes when they have).
func (w *Worker) Stop() {
	<-w.cron.Stop().Done()
}

// ScheduleUser runs the standard proactive jobs for a session at its timezone.
func (w *Worker) ScheduleUser(sid, tz string) {
	if sid == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if prev, ok := w.sched[sid]; ok {
		if prev == tz {
			return // already scheduled, at this timezone
		}
		// The timezone moved. Take the old entries out before adding new ones —
		// leaving them would fire the same brief twice, once per zone.
		w.unscheduleLocked(sid)
	}

	tz = w.resolveTZ(sid, tz)
	var ids []cron.EntryID
	add := func(name, spec string, fn func()) {
		id, err := w.cron.AddFunc(spec, fn)
		if err != nil {
			w.log.Error("schedule proactive job", "job", name, "sid", sid, "spec", spec, "err", err)
			return
		}
		ids = append(ids, id)
	}
	// The two daily times come from the learned rhythm (ζ-2). They used to be
	// the literals 07:30 and 21:00, which happen to be exactly what
	// Schedule(Cold()) produces — so "derived from the rhythm" was true of the
	// numbers and false of the mechanism: learning something else changed
	// nothing. This is where that stopped being true.
	//
	// Read under w.mu via a background context: ScheduleUser is called from a
	// request path and from the learn job, and neither should be able to make
	// this block on a caller's cancelled context.
	jobs := w.jobsFor(context.Background(), sid)
	// Morning brief: at the learned wake time, in the user's own timezone.
	add("morning", cronScheduleAtHM(jobs.BriefAt, tz), func() { w.runBrief(sid, tz, "morning") })
	// Evening review: 90 minutes before the learned bedtime.
	add("evening", cronScheduleAtHM(jobs.ReviewAt, tz), func() { w.runBrief(sid, tz, "evening") })
	// Rhythm learning: at the day cut, when yesterday's row is final.
	add("rhythm", cronScheduleAt(rhythmConfig().DayCutHour, 0, tz), func() { w.runRhythmLearn(sid, tz) })
	// The Protector. Half-hourly because the predicate is a threshold on a
	// continuously growing number: the interval only bounds how late the nudge
	// can be, half an hour against a twenty-hour stretch.
	add("protector", protectorEvery, func() { w.checkProtector(sid, tz) })
	// Deadline check: every 2 hours.
	add("deadline", "0 */2 * * *", func() { w.checkDeadlines(sid, tz) })
	// Rolling replan: every 30 minutes.
	add("replan", "*/30 * * * *", func() { w.checkRollingReplan(sid, tz) })

	if len(ids) == 0 {
		// Nothing was scheduled — recording the session as scheduled here would
		// make the failure permanent for the life of the process.
		return
	}
	w.jobs[sid] = ids
	w.sched[sid] = tz
	w.log.Info("scheduled proactive jobs", "sid", sid, "tz", tz, "entries", len(ids))
}

// UnscheduleUser removes a session's proactive jobs.
func (w *Worker) UnscheduleUser(sid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.unscheduleLocked(sid)
}

func (w *Worker) unscheduleLocked(sid string) {
	for _, id := range w.jobs[sid] {
		w.cron.Remove(id)
	}
	delete(w.jobs, sid)
	delete(w.sched, sid)
}

// EntryCount reports how many cron entries are registered. Exported for the
// test that keeps ScheduleUser idempotent — the property is invisible from
// outside otherwise, which is exactly how it broke.
func (w *Worker) EntryCount() int { return len(w.cron.Entries()) }

// resolveTZ maps a timezone onto one cron can actually parse.
//
// An unloadable zone used to be a partial failure that looked like a success:
// the two CRON_TZ-prefixed jobs (morning brief, evening review) failed to
// register while the two plain ones did, the session was recorded as scheduled,
// and no later call retried — so that user silently never got a brief again.
//
// Falling back is the same trade markAwake already makes for rhythm signals: a
// brief at a possibly-wrong hour beats no brief, and the wrong hour is visible
// while the absence is not.
func (w *Worker) resolveTZ(sid, tz string) string {
	if tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	fallback := "UTC"
	if w.s != nil && w.s.cfg != nil && w.s.runtime().WorkerDefaultTZ != "" {
		if _, err := time.LoadLocation(w.s.runtime().WorkerDefaultTZ); err == nil {
			fallback = w.s.runtime().WorkerDefaultTZ
		}
	}
	if tz != "" {
		w.log.Warn("unknown timezone; scheduling proactive jobs in the fallback zone",
			"sid", sid, "tz", tz, "fallback", fallback)
	}
	return fallback
}

// cronScheduleAt builds a 5-field cron spec that fires at hour:min in the given
// IANA timezone. The CRON_TZ prefix makes robfig/cron evaluate the spec in that
// zone (the scheduler itself runs in UTC). Empty tz falls back to UTC.
//
// This also fixes the prior 6-field spec: cron.New was created without
// WithSeconds(), so a 6-field spec failed to parse and the brief jobs were
// silently dropped.
func cronScheduleAt(hour, min int, tz string) string {
	if tz == "" {
		tz = "UTC"
	}
	return fmt.Sprintf("CRON_TZ=%s %d %d * * *", tz, min, hour)
}

// cronScheduleAtHM is cronScheduleAt for an "HH:MM" the rhythm produced. An
// unparseable value falls back to the cold-start time for that job rather than
// dropping the entry — a brief at the default hour beats no brief, the same
// trade resolveTZ makes.
func cronScheduleAtHM(hm, tz string) string {
	var h, m int
	if n, err := fmt.Sscanf(hm, "%d:%d", &h, &m); n != 2 || err != nil ||
		h < 0 || h > 23 || m < 0 || m > 59 {
		return cronScheduleAt(7, 30, tz)
	}
	return cronScheduleAt(h, m, tz)
}

// ─── leadership and occurrence ownership ─────────────────────────────────────

// claim takes ownership of one occurrence of one job for one session, and
// reports whether this instance should do the work.
//
// # Where this call belongs
//
// AFTER the suppression gates (the user's toggles, Do Not Disturb) and after
// deciding there is actually something to do — never at the top of the job.
// domain.JobRunRepository.Claim says why: "Writing a row every half hour to
// record that nothing happened would bury the rows that mean something." The
// rolling replan fires 48 times a day per session and almost always decides to
// do nothing; claiming first would put 48 rows a day per session into the table
// whose only reader is a human asking "did my brief go out".
//
// # What it guarantees, and what it does not
//
// Guarantees: two instances that both reach here for the same occurrence — which
// the lease is supposed to prevent but clocks make possible — will not both come
// back true. That is a unique index doing the work, which is the only mutual
// exclusion all four backends share.
//
// Does not guarantee: that a failed occurrence is ever retried. Nothing
// re-drives it. See leader.go.
//
// A storage error returns false. Running unguarded when the guard is broken is
// exactly the duplicate this whole mechanism exists to prevent, and a proactive
// message that does not go out is cheaper than one that goes out twice.
func (w *Worker) claim(ctx context.Context, sid, job, runKey string) (*domain.JobRun, bool) {
	if !w.s.LeadsWorker() {
		return nil, false
	}
	run := &domain.JobRun{
		SessionID: sid, Job: job, RunKey: runKey, Instance: w.s.InstanceID(),
	}
	ok, err := w.s.store.JobRuns().Claim(ctx, run)
	if err != nil {
		w.log.Warn("could not claim a job occurrence; skipping it rather than risking a duplicate",
			"sid", sid, "job", job, "runKey", runKey, "err", err)
		return nil, false
	}
	if !ok {
		w.log.Debug("job occurrence already claimed", "sid", sid, "job", job, "runKey", runKey)
		return nil, false
	}
	return run, true
}

// finish closes an occurrence.
//
// context.WithoutCancel plus a fresh timeout: the job's own context is 15–30
// seconds and is very often the thing that just expired. Closing the row on a
// dead context would leave it "running" forever, which reads as a crash and
// blocks the occurrence for JobStaleAfter. Same reasoning as settleDecision in
// agent.go.
func (w *Worker) finish(ctx context.Context, run *domain.JobRun, jobErr error) {
	if run == nil {
		return
	}
	status, msg := domain.JobDone, ""
	if jobErr != nil {
		status, msg = domain.JobFailed, jobErr.Error()
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := w.s.store.JobRuns().Finish(fctx, run.ID, status, msg, time.Now()); err != nil {
		w.log.Warn("could not close a job occurrence; it will read as a crash",
			"sid", run.SessionID, "job", run.Job, "err", err)
	}
}

// dayKey names a once-a-day occurrence: the local date.
//
// Local, not UTC. A daily job is daily in the user's own day — an occurrence key
// in UTC would let a user near the date line get two morning briefs on one of
// their days and none on another.
func dayKey(now time.Time, loc *time.Location) string {
	return now.In(loc).Format("2006-01-02")
}

// slotKey names an occurrence of a job that repeats within a day: the local date
// plus the slot the firing falls in.
//
// Truncation is on the wall clock in the user's zone, and it is plain truncation
// with no grace window. robfig/cron fires from a timer that Go guarantees not to
// run early, so the body's own time.Now() is always at or after the slot
// boundary; a grace window would only serve to push a delayed tick into the NEXT
// slot, stealing an occurrence that has not happened yet and making the real
// firing find it taken.
func slotKey(now time.Time, loc *time.Location, slot time.Duration) string {
	t := now.In(loc)
	return t.Format("2006-01-02") + "T" + t.Truncate(slot).Format("1504")
}

// ─── Brief generation ────────────────────────────────────────────────────────

// runBrief generates and sends a morning or evening brief for a session.
func (w *Worker) runBrief(sid, tz, kind string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker runBrief: get session", "sid", sid, "err", err)
		return
	}

	prefs := parsePrefs(sess.Preferences)
	locale := w.s.localePair(ctx, sid).Resolve(sess.Language, "")
	name := sess.AssistantName
	if name == "" {
		name = "Daycore"
	}

	job := domain.JobMorningBrief
	switch kind {
	case "morning":
		if !prefs.MorningBrief || prefs.DoNotDisturb {
			return
		}
	case "evening":
		job = domain.JobEveningReview
		if !prefs.EveningReview || prefs.DoNotDisturb {
			return
		}
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	// Claim after the toggles, before the work. One occurrence per session per
	// local day per kind — so a second instance, or this instance after a
	// restart that re-fires the same cron minute, does not send it twice.
	run, ok := w.claim(ctx, sid, job, dayKey(now, loc))
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, run, jobErr) }()
	w.log.Info("running brief", "sid", sid, "kind", kind)

	// Gather context: weather, today's plan, upcoming deadlines.
	weatherSummary := w.lookupWeather(ctx, sid, locale)
	planSummary := w.loadPlanSummary(ctx, sid, today)

	// Build the prompt and call the agent.
	sysPrompt, err := w.s.prompts.Render(ctx, ai.PromptBrief, locale, map[string]any{
		"Name": name, "Date": today, "Clock": now.Format("15:04"), "TZ": tz,
		"Weather": weatherSummary, "PlanJSON": planSummary,
	})
	if err != nil {
		w.log.Error("worker: brief prompt", "err", err)
		jobErr = err
		return
	}
	if kind == "morning" && prefs.GapSuggestions {
		sysPrompt += gapSuggestionHint(locale)
	}

	var userMsg string
	if kind == "morning" {
		userMsg = morningUserPrompt(locale)
	} else {
		userMsg = eveningUserPrompt(locale)
	}

	provider := w.s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sysPrompt},
			{Role: ai.RoleUser, Content: userMsg},
		},
		Temperature: 0.7,
		MaxTokens:   1024,
	})
	w.s.logAICall(ctx, sid, epBrief, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		w.log.Error("worker runBrief: agent call", "sid", sid, "kind", kind, "err", err)
		jobErr = err
		return
	}

	text := strings.TrimSpace(resp.Content)
	if text == "" {
		// An empty answer is a done occurrence, not a failed one: the model was
		// asked and had nothing to say. Marking it failed would make the row read
		// like an outage.
		return
	}

	w.sendToChannels(ctx, sid, text)
}

// ─── Deadline warnings ───────────────────────────────────────────────────────

// checkDeadlines scans upcoming assignments and warns about imminent deadlines.
func (w *Worker) checkDeadlines(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	if !prefs.DeadlineAlerts || prefs.DoNotDisturb {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)

	assigns, err := w.s.store.Assignments().List(ctx, sid, domain.AssignmentFilter{})
	if err != nil {
		w.log.Warn("worker checkDeadlines: list assignments", "sid", sid, "err", err)
		return
	}

	// The ladder (STRATEGY §1.3). This used to be a single 48-hour window: every
	// two hours it re-listed everything due within two days and sent the same
	// message again. Nothing recorded that an item had already been warned about,
	// so a deadline three days out produced roughly two dozen identical messages
	// before it arrived — and the only way to stop them was DeadlineAlerts, which
	// turns the whole fact track off.
	//
	// Now: each item climbs 24h → 12h → 1h → overdue, and each rung fires at most
	// once for that item because the occurrence key is (assignment, rung). Three
	// or four messages over the life of a deadline, structurally, with no daily
	// budget needed — which is what lets the fact track stay out of the ≤3/day
	// suggestion budget honestly rather than as a loophole.
	var urgent []domain.Assignment
	rungs := map[string]time.Duration{}
	for _, a := range assigns {
		if a.DueAt == nil || a.RemindersOff {
			continue
		}
		if a.Status == domain.AssignmentDone || a.Status == domain.AssignmentDismissed {
			continue
		}
		rung, in := domain.DeadlineRungFor(*a.DueAt, now)
		if !in {
			continue
		}
		// Claim per (assignment, rung): whoever gets it sends, everybody else —
		// the next tick, the other instance, the process that just restarted —
		// finds it taken and stays quiet.
		if _, ok := w.claim(ctx, sid, domain.JobDeadlineWarn, deadlineRunKey(a.ID, rung)); !ok {
			continue
		}
		urgent = append(urgent, a)
		rungs[a.ID] = rung
	}

	if len(urgent) == 0 {
		return
	}

	// One occurrence row for the message itself, so the send is claimed too.
	run, ok := w.claim(ctx, sid, domain.JobDeadlineWarn, "batch:"+slotKey(now, loc, 2*time.Hour))
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, run, jobErr) }()

	// The fact track, and the only thing in the product that may escalate:
	// domain.BackingOf(Proposal{Origin: OriginDeadline}) is hard, which is what
	// licenses the 24h → 12h → 1h climb. Everything soft-backed gets one
	// delivery and never comes back louder — see domain/proposal.go.
	w.log.Info("deadline check", "sid", sid, "urgent", len(urgent), "rungs", rungs)

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker checkDeadlines: get session", "sid", sid, "err", err)
		jobErr = err
		return
	}
	locale := w.s.localePair(ctx, sid).Resolve(sess.Language, "")

	msg := formatDeadlineWarning(locale, urgent, now)
	w.sendToChannels(ctx, sid, msg)
}

// deadlineRunKey names one rung of one assignment.
//
// The rung is part of the key, so an item that crosses 24h, then 12h, then 1h
// produces three distinct occurrences and therefore three messages — and
// crossing the same rung again (a re-check two hours later) produces none.
//
// "overdue" rather than "0" for the past-due rung: run keys end up in a MySQL
// VARCHAR(64) and in a Mongo _id built by concatenation, so they are read by
// people as often as by code.
func deadlineRunKey(assignmentID string, rung time.Duration) string {
	name := "overdue"
	if rung > 0 {
		name = rung.String()
	}
	return "due:" + assignmentID + ":" + name
}

// ─── Rolling replan ──────────────────────────────────────────────────────────

// checkRollingReplan checks if any today blocks are overdue and proposes replan.
func (w *Worker) checkRollingReplan(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	if !prefs.RollingReplan || prefs.DoNotDisturb {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	dp, err := w.s.store.DayPlans().Get(ctx, sid, today)
	if err != nil || dp == nil {
		return
	}

	var overdue []domain.TimeBlock
	for _, b := range dp.Blocks {
		if b.Completed || b.Hidden || b.Time == nil {
			continue
		}
		bt, err := parseBlockTime(today, *b.Time, loc)
		if err != nil {
			continue
		}
		dur := 0 // default: 30 min buffer
		if b.DurationMin != nil {
			dur = *b.DurationMin
		}
		endTime := bt.Add(time.Duration(dur+15) * time.Minute)
		if now.After(endTime) {
			overdue = append(overdue, b)
		}
	}

	if len(overdue) == 0 {
		// The common case, 48 times a day per session. Claiming before this check
		// would make job_runs almost entirely rows that say "nothing happened".
		return
	}

	// Half-hour slots, matching the cron cadence.
	run, ok := w.claim(ctx, sid, domain.JobRollingReplan, slotKey(now, loc, 30*time.Minute))
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, run, jobErr) }()

	w.log.Info("rolling replan", "sid", sid, "overdue", len(overdue))

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker checkRollingReplan: get session", "sid", sid, "err", err)
		jobErr = err
		return
	}
	locale := w.s.localePair(ctx, sid).Resolve(sess.Language, "")

	// Call the agent to evaluate and suggest a replan.
	overdueJSON, _ := json.Marshal(overdue)
	sysPrompt, err := w.s.prompts.Render(ctx, ai.PromptReplan, locale, map[string]any{
		"Date": today, "Clock": now.Format("15:04"), "TZ": tz,
	})
	if err != nil {
		w.log.Error("worker: replan prompt", "err", err)
		jobErr = err
		return
	}
	userMsg := buildReplanUserPrompt(locale, string(overdueJSON))

	provider := w.s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sysPrompt},
			{Role: ai.RoleUser, Content: userMsg},
		},
		Temperature: 0.3,
		MaxTokens:   800,
	})
	w.s.logAICall(ctx, sid, epReplan, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		w.log.Error("worker checkRollingReplan: agent call", "sid", sid, "err", err)
		jobErr = err
		return
	}

	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return
	}

	w.sendToChannels(ctx, sid, text)
}

// ─── Channel dispatch ────────────────────────────────────────────────────────

// sendToChannels delivers a message through every channel bound to the session.
// When the channels registry is not wired, the message is only logged.
func (w *Worker) sendToChannels(ctx context.Context, sid, text string) {
	bindings, err := w.s.store.ChannelBindings().ListBySession(ctx, sid)
	if err != nil {
		w.log.Warn("worker: list channel bindings", "sid", sid, "err", err)
		return
	}
	if len(bindings) == 0 {
		w.log.Debug("worker: no channel bindings for session", "sid", sid)
		return
	}

	for _, b := range bindings {
		if w.channels == nil {
			w.log.Info("worker: would send to channel (registry not wired)",
				"sid", sid, "channel", b.Channel, "externalId", b.ExternalID,
				"len", len(text))
			continue
		}
		ch := w.channels.Channel(b.Channel)
		if ch == nil {
			w.log.Warn("worker: unknown channel", "channel", b.Channel)
			continue
		}
		if err := ch.Send(ctx, b.ExternalID, channels.Text(text)); err != nil {
			w.log.Warn("worker: channel send failed",
				"sid", sid, "channel", b.Channel, "externalId", b.ExternalID, "err", err)
		}
	}
	w.log.Info("worker: message dispatched",
		"sid", sid, "bindings", len(bindings), "len", len(text))
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// loadSessionPrefs fetches session prefs, falling back to defaults.
func (w *Worker) loadSessionPrefs(ctx context.Context, sid string) SessionPrefs {
	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		return DefaultPrefs()
	}
	return parsePrefs(sess.Preferences)
}

// parsePrefs decodes the JSON preferences string or returns defaults on any error.
func parsePrefs(raw string) SessionPrefs {
	var p SessionPrefs
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return DefaultPrefs()
	}
	return p
}

// resolveLocation parses tz into a *time.Location, falling back to UTC.
func resolveLocation(tz string) *time.Location {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// parseBlockTime parses a "HH:MM" time on the given date in a location.
func parseBlockTime(dateStr, timeStr string, loc *time.Location) (time.Time, error) {
	var h, m int
	if _, err := fmt.Sscanf(timeStr, "%d:%d", &h, &m); err != nil {
		return time.Time{}, err
	}
	// dateStr used to be accepted and then ignored — the date came from
	// time.Now() regardless. Both existing callers happened to pass today, so
	// the parameter was a lie nothing could catch; the first caller to pass any
	// other date would silently get a time on the wrong day.
	y, mo, d := time.Now().In(loc).Date()
	if t, err := time.ParseInLocation("2006-01-02", dateStr, loc); err == nil {
		y, mo, d = t.Date()
	}
	return time.Date(y, mo, d, h, m, 0, 0, loc), nil
}

// lookupWeather gets a 2-day forecast for the brief.
//
// # This path never picks a source and never probes
//
// It passes an empty id, which takes the first source that is currently usable,
// and it is the ONE weather caller that cannot ask the model to choose: the
// brief is a Chat with no Tools attached, so the model never sees a tool band
// at all. Any "let the model decide" story is simply false here, and inventing
// a fallback chain for it would re-introduce exactly what this batch deleted.
//
// It must also never act as a half-open probe. A deployment nobody is chatting
// with makes exactly two weather calls a day; paying the cost of discovering
// that a dead source recovered out of those two means a probe timeout delays a
// brief that was otherwise ready to send. Recovery is discovered by somebody's
// conversation — see adapters.Health.ShouldProbe.
//
// No source available means the brief simply has no weather line. That has
// always been the behaviour and it is the right one: a missing sentence beats a
// brief that arrives late or not at all.
//
// The location comes from the ladder in session_location.go, whose last rung is
// silence. It used to be hardcoded to 北京 for every user anywhere, with
// `// future: session setting` beside it — a confidently wrong forecast every
// morning, which is worse than none, because "17°C and raining" is a sentence
// somebody dresses by.
func (w *Worker) lookupWeather(ctx context.Context, sid, locale string) string {
	if w.s.weather == nil {
		return ""
	}
	place, _ := w.s.SessionLocation(ctx, sid)
	if place == "" {
		// No line rather than a guess. The brief has four other things to say.
		return ""
	}
	fc, err := w.s.weather.Lookup(ctx, "", domain.WeatherQuery{Location: place, Days: 2, Locale: locale})
	if err != nil {
		return ""
	}
	return fc.Summary(locale)
}

// loadPlanSummary loads today's visible plan blocks as a compact summary.
func (w *Worker) loadPlanSummary(ctx context.Context, sid, date string) string {
	dp, err := w.s.store.DayPlans().Get(ctx, sid, date)
	if err != nil || dp == nil || len(dp.Blocks) == 0 {
		return ""
	}
	vp := visiblePlan(dp)
	if len(vp.Blocks) == 0 {
		return ""
	}
	b, err := json.Marshal(vp.Blocks)
	if err != nil {
		return ""
	}
	return string(b)
}

// ─── Prompt builders ─────────────────────────────────────────────────────────

// gapSuggestionHint appends the GapSuggestions behavior to the morning brief so
// the toggle actually does something (a full standalone gap-scan job is a later
// optimization).
var (
	gapSuggestionText = i18n.Reg("worker.gapSuggestion", i18n.Text{
		"zh-CN": "\n另外：如果今天日程里有较长的空档，可以顺带建议用户安排一件小事（休息、复习或愿望池里的事），一句话即可。",
		"en-US": "\nAlso: if today's schedule has a long open gap, suggest one small thing to slot in (a break, review, or a wish) — one sentence.",
	})
	morningUserText = i18n.Reg("worker.morningUser", i18n.Text{
		"zh-CN": "请给我生成一条早安简报，总结今天的日程和天气。",
		"en-US": "Please generate a friendly morning brief summarizing today's schedule and weather.",
	})
	eveningUserText = i18n.Reg("worker.eveningUser", i18n.Text{
		"zh-CN": "请回顾一下今天的完成情况，并给出明天优先事项的建议。",
		"en-US": "Please review today's accomplishments and suggest tomorrow's priorities.",
	})
	// %s is the overdue-block JSON.
	replanUserText = i18n.Reg("worker.replanUser", i18n.Text{
		"zh-CN": "以下是我今天超时未完成的时间块（JSON）：%s\n请帮我提出重排建议。",
		"en-US": "Here are my overdue time blocks for today (JSON): %s\nPlease suggest a replan.",
	})
)

func gapSuggestionHint(locale string) string { return i18n.T(gapSuggestionText, locale) }
func morningUserPrompt(locale string) string { return i18n.T(morningUserText, locale) }
func eveningUserPrompt(locale string) string { return i18n.T(eveningUserText, locale) }

func buildReplanUserPrompt(locale, overdueJSON string) string {
	return i18n.Tf(replanUserText, locale, overdueJSON)
}

// ─── Deadline formatting ─────────────────────────────────────────────────────

// The deadline-warning vocabulary is split this finely because the two
// languages assemble the sentence differently — Chinese runs the clauses
// together with 、and ：where English needs "and" and a colon — and one format
// string per language could not express both without one reading as a
// translation. Flat keys also mean a translator edits JSON, not Go.
//
// Counts interpolate as %d. Note that neither language pluralises properly:
// Chinese has no plural and the English writes "assignment(s)". That is the
// existing copy, kept verbatim; a language with real plural rules will need
// more than a format string, and this is where that lands.
const (
	keyDeadlinePrefix     = "worker.deadline.prefix"
	keyDeadlineOverdue    = "worker.deadline.overdue"
	keyDeadlineAlsoSoon   = "worker.deadline.alsoSoon"
	keyDeadlineOnlySoon   = "worker.deadline.onlySoon"
	keyDeadlineColon      = "worker.deadline.colon"
	keyDeadlineNoDue      = "worker.deadline.noDue"
	keyDeadlineOverdueTag = "worker.deadline.overdueTag"
	keyDeadlineItem       = "worker.deadline.item"
)

func init() {
	i18n.Register(keyDeadlinePrefix, i18n.Text{"zh-CN": "提醒：", "en-US": "Reminder: "})
	i18n.Register(keyDeadlineOverdue, i18n.Text{
		"zh-CN": "你有 %d 个已逾期的任务",
		"en-US": "you have %d overdue assignment(s)",
	})
	i18n.Register(keyDeadlineAlsoSoon, i18n.Text{
		"zh-CN": "，还有 %d 个在 48 小时内截止",
		"en-US": " and %d due within 48 hours",
	})
	i18n.Register(keyDeadlineOnlySoon, i18n.Text{
		"zh-CN": "你有 %d 个任务在 48 小时内截止",
		"en-US": "you have %d assignment(s) due within 48 hours",
	})
	i18n.Register(keyDeadlineColon, i18n.Text{"zh-CN": "：\n", "en-US": ":\n"})
	i18n.Register(keyDeadlineNoDue, i18n.Text{"zh-CN": "无截止日期", "en-US": "No due date"})
	i18n.Register(keyDeadlineOverdueTag, i18n.Text{"zh-CN": "%s（已逾期）", "en-US": "%s (overdue)"})
	i18n.Register(keyDeadlineItem, i18n.Text{"zh-CN": "%d. %s — %s\n", "en-US": "%d. %s — %s\n"})
}

// formatDeadlineWarning builds a human-readable deadline warning message.
func formatDeadlineWarning(locale string, urgent []domain.Assignment, now time.Time) string {
	overdueCount, upcomingCount := partitionDeadlines(urgent, now)

	var b strings.Builder
	b.WriteString(i18n.T(keyDeadlinePrefix, locale))
	if overdueCount > 0 {
		b.WriteString(i18n.Tf(keyDeadlineOverdue, locale, overdueCount))
		if upcomingCount > 0 {
			b.WriteString(i18n.Tf(keyDeadlineAlsoSoon, locale, upcomingCount))
		}
	} else {
		b.WriteString(i18n.Tf(keyDeadlineOnlySoon, locale, upcomingCount))
	}
	b.WriteString(i18n.T(keyDeadlineColon, locale))

	for i, a := range urgent {
		due := i18n.T(keyDeadlineNoDue, locale)
		if a.DueAt != nil {
			due = a.DueAt.Format("01-02 15:04")
			if a.DueAt.Before(now) {
				due = i18n.Tf(keyDeadlineOverdueTag, locale, due)
			}
		}
		b.WriteString(i18n.Tf(keyDeadlineItem, locale, i+1, a.Title, due))
	}
	return b.String()
}

// partitionDeadlines counts overdue vs upcoming assignments against now.
func partitionDeadlines(urgent []domain.Assignment, now time.Time) (overdue, upcoming int) {
	for _, a := range urgent {
		if a.DueAt != nil && a.DueAt.Before(now) {
			overdue++
		} else {
			upcoming++
		}
	}
	return
}

// ─── Session preferences ─────────────────────────────────────────────────────

// SessionPrefs holds user-controlled toggles for proactive features.
type SessionPrefs struct {
	MorningBrief   bool `json:"morningBrief"`
	EveningReview  bool `json:"eveningReview"`
	DeadlineAlerts bool `json:"deadlineAlerts"`
	RollingReplan  bool `json:"rollingReplan"`
	GapSuggestions bool `json:"gapSuggestions"`
	DoNotDisturb   bool `json:"doNotDisturb"`
	AutoPlan       bool `json:"autoPlan"`

	// ThemeByFamily is the current theme per FRONTEND FAMILY, for every family
	// except the fallback one — that one stays in sessions.current_theme, where
	// it has always been. See theme_current.go for why the split, and why the
	// lost-update race it carries is accepted.
	ThemeByFamily map[string]string `json:"themeByFamily,omitempty"`

	// MaterialCategories holds per-category enablement overrides (category id →
	// on/off). A missing entry falls back to the registry's DefaultOn; "note"
	// can never be turned off. See domain.MaterialCategories.
	MaterialCategories map[string]bool `json:"materialCategories,omitempty"`

	// PrimaryLocale and SecondaryLocale are the two languages this user's
	// switch toggles between, chosen on the settings page from whatever the
	// installation has available. Empty means "use the deployment default"
	// (config.DefaultLocales) — the common case, since most people never open
	// the language settings.
	//
	// These live in preferences rather than as columns on purpose: the sessions
	// table already has a language column, and it holds a different thing —
	// which of the two they are reading in *right now*, flipped by the home
	// page switch. A pair is a preference; the current one is state.
	PrimaryLocale   string `json:"primaryLocale,omitempty"`
	SecondaryLocale string `json:"secondaryLocale,omitempty"`

	// Timezone is the IANA zone this user's own day is measured in — the
	// petrify line, the rhythm day key, and when the morning brief fires.
	// Empty means "use the deployment default" (WORKER_DEFAULT_TZ).
	//
	// TimezoneSource says who decided it: TZSourceUser (settings page) or
	// TZSourceDetected (a client hint). The distinction is load-bearing — a
	// device hint may fill in or update a detected value but must never
	// overwrite a user's own choice, or somebody who deliberately keeps their
	// schedule on home time has it moved the first time they open the app from
	// an airport. See timezone.go.
	Timezone       string `json:"timezone,omitempty"`
	TimezoneSource string `json:"timezoneSource,omitempty"`

	// Location is where this user is, as free text a weather source can resolve
	// ("北京", "Cambridge, MA"). It exists for ONE caller: the morning and
	// evening briefs, which query the weather without a model in the loop and
	// therefore cannot ask anybody where to look.
	//
	// LocationSource mirrors TimezoneSource and carries the same rule: a client
	// hint may fill in or update a detected value but must never overwrite what
	// the user typed. Somebody who set their home city on purpose should not
	// have it rewritten the first time they open the app on a train.
	//
	// ⚠️ Not a coordinate pair, and not derived from the timezone. Free text
	// because every weather source in this project takes free text and resolves
	// it itself — turning "Cambridge, MA" into a lat/lon here would mean this
	// process owning a geocoder, and getting a different answer than the source
	// would have. A timezone is not a location either: Asia/Shanghai covers a
	// country.
	Location       string `json:"location,omitempty"`
	LocationSource string `json:"locationSource,omitempty"`
}

// DefaultPrefs returns the default (all-on) preferences.
func DefaultPrefs() SessionPrefs {
	return SessionPrefs{
		MorningBrief:   true,
		EveningReview:  true,
		DeadlineAlerts: true,
		RollingReplan:  true,
		GapSuggestions: true,
		DoNotDisturb:   false,
		AutoPlan:       true,
	}
}
