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
	epProtector     = "protector"
	epMoodReply     = "mood_reply"
	epAIPlan        = "ai_plan"
	epThemeGen      = "theme_gen"
	epTravel        = "travel"
	epInboxClassify = "inbox_classify"
	epSummarise     = "summarise"
	epReplan        = "replan"
)

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
}
