package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/timeutil"
)

// The one gate every plan write passes, and the one refusal shape it produces.
//
// Two rules live here rather than in two places because they answer the same
// question — "may this block move?" — and a caller that had to ask twice would
// eventually ask once. They also have to sit inside applyPlanPatch rather than
// in the HTTP handler: the agent tools call applyPlanPatch directly, so a gate
// in the handler would guard the front door and leave the model's door open.
//
// Before this existed, neither rule was enforced anywhere. domain.Movable,
// TimeBlock.Frozen, PhaseIn and PetrifyLine were all written, all tested, and
// all had zero production callers — measured: a user could drag a hard-locked
// class from 09:00 to 15:00, and could rewrite or delete a block from three
// days ago, with no error from any layer.

// planBlocked is the refusal. One type for both rules so the HTTP envelope, the
// tool result and the log line cannot drift into two dialects of "no".
type planBlocked struct {
	// Code is "locked" or "petrified" — which rule refused.
	Code string
	// BlockID is what the caller tried to touch.
	BlockID string
	// LockLevel and LockReason are empty for a petrify refusal.
	LockLevel  domain.LockLevel
	LockReason string
	// Confirmable says the caller has a way through: re-send with confirm.
	// True only for soft locks. A hard lock and a petrified block have no way
	// through today, and saying otherwise would be an offer nothing honours.
	Confirmable bool
}

func (e *planBlocked) Error() string { return "plan write refused: " + e.Code }

const (
	keyPlanLockedHard = "plan.blocked.lockedHard"
	keyPlanLockedSoft = "plan.blocked.lockedSoft"
	keyPlanPetrified  = "plan.blocked.petrified"
)

func init() {
	i18n.Register(keyPlanLockedHard, i18n.Text{
		"zh-CN": "这个时间由课表决定，挪不动",
		"en-US": "This time is set by your timetable and cannot be moved",
	})
	i18n.Register(keyPlanLockedSoft, i18n.Text{
		"zh-CN": "这件事和别人约好了，确认一下再改",
		"en-US": "This one was agreed with someone else — confirm to change it",
	})
	i18n.Register(keyPlanPetrified, i18n.Text{
		"zh-CN": "这段已经翻篇了，是记录不是计划",
		"en-US": "That page has turned — it is a record now, not a plan",
	})
}

func (e *planBlocked) message(locale string) string {
	switch {
	case e.Code == "refish_capped":
		return i18n.T(keyPlanRefishCapped, locale)
	case e.Code == "petrified":
		return i18n.T(keyPlanPetrified, locale)
	case e.LockLevel == domain.LockSoft:
		return i18n.T(keyPlanLockedSoft, locale)
	default:
		return i18n.T(keyPlanLockedHard, locale)
	}
}

// writePlanBlocked renders the refusal as 409.
//
// 409 rather than 403: nothing about the caller's identity is wrong. The
// request conflicts with the state of the thing it names, and that state can
// change — a soft lock takes a confirmation, and tomorrow's block is not yet
// petrified. A 403 would tell a client to stop asking.
func (s *Server) writePlanBlocked(w http.ResponseWriter, locale string, e *planBlocked) {
	body := map[string]any{
		"error":       "blocked",
		"code":        e.Code,
		"message":     e.message(locale),
		"blockId":     e.BlockID,
		"confirmable": e.Confirmable,
	}
	if e.LockLevel != "" {
		body["lockLevel"] = string(e.LockLevel)
	}
	if e.LockReason != "" {
		body["lockReason"] = e.LockReason
	}
	s.writeJSON(w, http.StatusConflict, body)
}

// timeFields are the keys a lock actually guards.
//
// A lock is about WHEN, not about WHETHER or WHAT. Both default reasons say so
// in as many words — "课程时间由课表决定", "Time you agreed with someone else".
// Reading the lock as "this row is read-only" closes the only door the user
// has: taking leave from a class goes through remove (a rule occurrence
// becomes a tombstone and the rule survives), so a lock that blocked remove
// would turn a hard lock into a dead end with no exit at all — the opposite of
// what a lock is for.
//
// Measured while writing this: the first version of the gate did exactly that.
// It also stopped the user renaming "高数（课）" to "高等数学" and ticking the
// class off as done, neither of which moves anything.
var timeFields = map[string]bool{
	"time": true, "date": true, "duration_min": true,
	"utc_time": true, "time_mode": true, "timezone": true,
	"offset_min": true, "offset_ref": true,
}

// retimes reports whether an update would move the block in time.
func retimes(changes map[string]any) bool {
	for k := range changes {
		if timeFields[k] {
			return true
		}
	}
	return false
}

// petrifyEdit classifies what an update does to a block that has already
// frozen.
//
// Reconciliation is not an edit. "翻篇" ends with someone marking last night
// done or not done, and the experience core is explicit that a petrified block
// still accepts that — refusing it would leave yesterday permanently half
// finished, which is the shape of debt this product exists not to create.
// Everything else about a stone block is history.
func petrifyEditAllowed(changes map[string]any) bool {
	if len(changes) == 0 {
		return true
	}
	for k := range changes {
		if k != "completed" {
			return false
		}
	}
	return true
}

// guardPlanWrite refuses a write that the lock rules or the petrify line
// forbid. blocks are the day's blocks as generic maps, mid-patch.
//
// actor decides a great deal:
//
//   - ActorSystem passes both gates unconditionally. Revert restores a frozen
//     block by definition, and regeneration rewrites yesterday's auto blocks;
//     gating those would mean undo stops working the moment it is most needed.
//   - ActorAgent passes the lock gate (domain.Movable says so: a hard lock
//     means "the timetable decides", not "the assistant may not help") but NOT
//     the petrify gate. The agent has no business rewriting what already
//     happened.
//   - ActorUser passes a soft lock only with confirm.
func (s *Server) guardPlanWrite(blocks []map[string]any, planDate string, action planAction, actor string, now time.Time, loc *time.Location) error {
	if actor == domain.ActorSystem {
		return nil
	}
	if action.Action != "update" && action.Action != "remove" {
		return nil // an add touches nothing that exists yet
	}
	horizon := timeutil.PetrifyHorizonDefault
	line := timeutil.PetrifyLine(now, loc, horizon)

	for _, raw := range blocks {
		if !matchesAll(raw, action.Match) {
			continue
		}
		b, err := blockFromMap(raw)
		if err != nil {
			continue // unparseable rows are the audit log's problem, not the gate's
		}
		// Petrify first: a frozen block is not a scheduling question at all, so
		// reporting it as a lock problem would send the user looking for an
		// unlock button that would not help.
		// planDate, not the block's own date: Span treats its argument as the
		// FALLBACK for rows that inherit the enclosing plan's date, which is most
		// of them. Passing the row's own (usually empty) date would make Span
		// return ok=false and the petrify gate would silently never fire.
		if start, end, ok := b.Span(planDate, loc); ok {
			if domain.PhaseAt(start, end, now, line) == domain.PhaseStone {
				if action.Action == "remove" || !petrifyEditAllowed(action.Changes) {
					return &planBlocked{Code: "petrified", BlockID: b.ID}
				}
			}
		}
		// Only a retime asks the lock's question. A remove is "this is not
		// happening" and a title edit is not a scheduling act at all.
		if action.Action == "update" && retimes(action.Changes) && !b.Movable(actor, action.Confirm) {
			return &planBlocked{
				Code: "locked", BlockID: b.ID,
				LockLevel: b.LockLevel, LockReason: b.LockReason,
				Confirmable: b.LockLevel == domain.LockSoft,
			}
		}
	}
	return nil
}

// blockFromMap round-trips one generic row back into a TimeBlock so the gate
// can use the domain predicates rather than re-reading fields by hand — a
// second reader of the same JSON is a second place for the two to disagree.
func blockFromMap(raw map[string]any) (domain.TimeBlock, error) {
	var b domain.TimeBlock
	j, err := json.Marshal(raw)
	if err != nil {
		return b, err
	}
	return b, json.Unmarshal(j, &b)
}

// planLocation is the timezone the petrify line is drawn in.
//
// The deployment default, because sessions do not carry a timezone yet — the
// same limitation awake.go records for the rhythm day key, and the same fix
// (per-session timezone, batch ζ) closes both. For a user whose real timezone
// differs, the line moves by that offset: their evening freezes early or late.
// That is a real defect and it is the reason ζ calls this correctness rather
// than polish; it is not a NEW defect, since every other clock in the server
// already reads from here.
func (s *Server) planLocation() *time.Location {
	if s == nil || s.cfg == nil {
		return time.UTC
	}
	return resolveLocation(s.cfg.WorkerDefaultTZ)
}

const keyPlanRefishCapped = "plan.refish.capped"

func init() {
	i18n.Register(keyPlanRefishCapped, i18n.Text{
		"zh-CN": "这件事已经改期够多次了 —— 也许它现在不重要，那也没关系",
		"en-US": "This has been moved enough times — maybe it is not the moment, and that is fine",
	})
}

// resolveReschedule fills in a retry's chain from the block it names.
//
// The caller says only WHAT it is retrying (`rescheduled_from`); the depth and
// the root come from the original row. Taking the count from the request would
// make RescheduleCap decorative — a client that reset its own counter could be
// offered the same undone thing forever, which is the debt spiral the cap
// exists to prevent.
//
// Refusing at the cap is a product decision, not a resource limit: after a few
// honest tries, "you still have not done this" stops being information and
// becomes an accusation. The refusal text says so.
func (s *Server) resolveReschedule(ctx context.Context, sid string, blocks []map[string]any, action *planAction) error {
	if action.Action != "add" || action.Block == nil {
		return nil
	}
	from, _ := action.Block["rescheduled_from"].(string)
	if from == "" {
		return nil
	}
	// The original is usually on another day — that is the whole point of a
	// retry — so look past today's blocks when it is not here.
	original, ok := findBlockByID(blocks, from)
	if !ok {
		var err error
		original, err = s.findBlockAcrossDays(ctx, sid, from)
		if err != nil || original.ID == "" {
			// An unknown origin is not worth failing the write over: the block
			// still wants to exist. Drop the claim rather than the block, so
			// nothing downstream reads a chain that points nowhere.
			delete(action.Block, "rescheduled_from")
			delete(action.Block, "reschedule_count")
			return nil
		}
	}
	if !original.Refishable() {
		return &planBlocked{Code: "refish_capped", BlockID: original.ID}
	}
	root, count := domain.RescheduleChain(original)
	action.Block["rescheduled_from"] = root
	action.Block["reschedule_count"] = count
	return nil
}

func findBlockByID(blocks []map[string]any, id string) (domain.TimeBlock, bool) {
	for _, raw := range blocks {
		if got, _ := raw["id"].(string); got != id {
			continue
		}
		b, err := blockFromMap(raw)
		return b, err == nil
	}
	return domain.TimeBlock{}, false
}

// RescheduleLookbackDays bounds the search for a retry's original.
//
// Re-fishing is about the recent past — the run you meant to do on Tuesday,
// not something from last term. A bounded window keeps this one range query
// instead of a scan, and the bound is not arbitrary: past the petrify horizon
// plus a couple of weeks, an undone block has stopped being a plan and become
// a record, and offering it a new slot would be asking about something the
// user has already moved on from.
const RescheduleLookbackDays = 21

// findBlockAcrossDays looks for a block in the recent past. Not finding it is
// an ordinary answer, not an error — the id may be stale, or from further back
// than the window.
func (s *Server) findBlockAcrossDays(ctx context.Context, sid, blockID string) (domain.TimeBlock, error) {
	loc := s.planLocation()
	now := time.Now().In(loc)
	from := now.AddDate(0, 0, -RescheduleLookbackDays).Format("2006-01-02")
	to := now.Format("2006-01-02")
	plans, err := s.store.DayPlans().Range(ctx, sid, from, to)
	if err != nil {
		return domain.TimeBlock{}, err
	}
	for _, p := range plans {
		for _, b := range p.Blocks {
			if b.ID == blockID {
				return b, nil
			}
		}
	}
	return domain.TimeBlock{}, nil
}
