package server

import (
	"context"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

// AI call ledger endpoints. The AICallLog table had three backend
// implementations and an admin reader for its aggregate, and not one writer:
// token spend, per-model latency and the pricing data the hosted tier will be
// built on were all unanswerable. Every model call — streaming or not — now
// lands one row here.
const (
	epCompanion     = "companion"
	epAutoPlan      = "autoplan"
	epBrief         = "brief"
	epWeeklyLetter  = "weekly_letter"
	epProtector     = "protector"
	epMoodReply     = "mood_reply"
	epAIPlan        = "ai_plan"
	epThemeGen      = "theme_gen"
	epThemeBackfill = "theme_backfill"
	epTravel        = "travel"
	epInboxClassify = "inbox_classify"
	epSummarise     = "summarise"
	epReplan        = "replan"
)

// aiEndpoints is the vocabulary the console's filter chips are built from.
//
// It is a second list of the same strings, which is a duplication worth having
// only because a gate keeps the two identical: TestEveryAIEndpointIsFilterable
// parses the const block above and fails if any value is missing here. Without
// that, adding a twelfth endpoint gives it a ledger row nobody can filter to —
// and the failure is invisible, because the screen still works and simply never
// offers that choice.
//
// Ordered by how often somebody looks for it rather than alphabetically: the
// question is nearly always "what did companion do", and a filter list that
// starts with ai_plan makes that a scan.
func aiEndpoints() []string {
	return []string{
		epCompanion, epBrief, epAutoPlan, epReplan, epProtector, epWeeklyLetter,
		epMoodReply, epAIPlan, epThemeGen, epThemeBackfill, epTravel, epInboxClassify, epSummarise,
	}
}

// usageOf tolerates a nil response (the call failed before one arrived).
func usageOf(resp *ai.ChatResponse) ai.Usage {
	if resp == nil {
		return ai.Usage{}
	}
	return resp.Usage
}

// logAICall records one provider call, best-effort like logOp: a lost ledger
// row must never fail the call it describes. A nil usage (provider did not
// report it) is recorded as zero — zero means "not reported", not "free".
func (s *Server) logAICall(ctx context.Context, sid, endpoint, model string, start time.Time, usage ai.Usage, callErr error) {
	s.recordAICall(ctx, sid, endpoint, model, start, usage, callErr, true)
}

// logAICallNotBilledToTheAccount records a call the ACCOUNT did not ask for.
//
// # ⚠️ The ledger row still carries the session; the account's counters do not
// move
//
// Those are two different questions and this batch is the first thing that can
// tell them apart:
//
//	ai_call_logs.session_id   "what was this spend FOR" — a theme belonging to
//	                          that person, so attributing it anywhere else (or
//	                          nowhere) would lose the only link between the cost
//	                          and the thing it bought.
//	sessions.usage_*          "what is this ACCOUNT doing" — read by the console
//	                          to find who is hammering the API, and windowed at
//	                          three hours precisely so it reflects behaviour.
//
// An operator pressing "backfill" on a family with two thousand themes would
// otherwise spike two thousand accounts' three-hour windows at once, and
// whoever went looking for the cause would find two thousand innocent users
// instead of the deploy that actually did it. The endpoint column
// (theme_backfill) is how the ledger says which it was.
func (s *Server) logAICallNotBilledToTheAccount(ctx context.Context, sid, endpoint, model string, start time.Time, usage ai.Usage, callErr error) {
	s.recordAICall(ctx, sid, endpoint, model, start, usage, callErr, false)
}

func (s *Server) recordAICall(ctx context.Context, sid, endpoint, model string, start time.Time, usage ai.Usage, callErr error, bill bool) {
	if s == nil || s.store == nil {
		return
	}
	l := &domain.AICallLog{
		SessionID:    sid,
		Endpoint:     endpoint,
		Model:        model,
		PromptTokens: usage.PromptTokens,
		CompTokens:   usage.CompletionTokens,
		DurationMs:   time.Since(start).Milliseconds(),
		Status:       "ok",
		RequestID:    requestIDFrom(ctx),
	}
	if callErr != nil {
		l.Status = "error"
		l.Error = truncate(callErr.Error(), 300)
	}
	if err := s.store.AILogs().Add(ctx, l); err != nil && s.log != nil {
		s.log.Debug("ai call log write failed", "endpoint", endpoint, "err", err)
	}
	// The per-account counters, on the session row (domain/session_usage.go).
	//
	// A SECOND write on this path, which the deployment-wide rollup deliberately
	// does not have — and the difference is not inconsistency. That rollup can be
	// recomputed from the ledger by a GROUP BY, so it is folded once a day. These
	// cannot: the lifetime total has to outlive the ledger's retention, and the
	// windows have to be readable in one row read rather than an aggregate over
	// the busiest table.
	//
	// Best-effort like the row above, and for the same reason: an account's
	// counter is not worth failing somebody's conversation over.
	if sid != "" && bill {
		if err := s.store.Sessions().AddUsage(ctx, sid, usage.PromptTokens, usage.CompletionTokens, time.Now()); err != nil && s.log != nil {
			s.log.Debug("session usage update failed", "session", sid, "err", err)
		}
	}
}
