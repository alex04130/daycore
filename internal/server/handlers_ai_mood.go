package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/i18n"
	"daycore/internal/mood"
)

func init() {
	registerRoutes("ai", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/ai/mood", s.handleAIMood)
	})
}

// POST /api/ai/mood — mood → warm text response.
func (s *Server) handleAIMood(w http.ResponseWriter, r *http.Request) {
	// Authenticate before parsing. These read the session further down anyway, so
	// the check was never missing — only late, which let an unauthenticated caller
	// probe body validation and spend the parser. Auth first is also what makes
	// "every non-public route answers 401" a checkable invariant
	// (auth_surface_test.go) rather than a per-handler habit.
	if _, ok := s.requireSession(w, r); !ok {
		return
	}
	if !s.requireAI(w, r) {
		return
	}
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Mood string `json:"mood"`
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Mood) == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.aIMood.bad_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.runtime().AIRequestTimeout)
	defer cancel()

	sys, err := s.prompts.Render(ctx, ai.PromptMood, s.requestLocale(r), ai.MoodData{
		Mood:        body.Mood,
		MemoryFacts: s.memoryFactsContext(ctx, sessionIDFrom(r.Context())),
	})
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.aIMood.internal")
		return
	}
	provider := s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleUser, Content: sys}},
		Temperature: 0.7, MaxTokens: 300,
	})
	s.logAICall(ctx, sessionIDFrom(r.Context()), epMoodReply, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		s.log.Error("ai mood", "err", err)
		// ⚠️ NOT writeErrL, and the difference is the key. This endpoint's reply
		// field is `response`, not `message` — a client reads the companion's
		// words from there, so the failure has to arrive in the same place the
		// success would. Swapping in the standard envelope would leave the
		// screen with an empty reply and an error field it does not read.
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":    "server_error",
			"response": i18n.T("err.aIMood.server_error", s.requestLocale(r)),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"response": resp.Content})
}

// moodWindowLimit is how many check-ins feed the window. Decay makes the tail
// weigh almost nothing, so this only has to be comfortably more than the
// trend comparison can use.
const moodWindowLimit = 30

// moodWindow derives this session's mood reading — trend, decayed score,
// staleness. Everything that wants to know how the user has been doing goes
// through here (EXPERIENCE_CORE §12.6): the companion's context, rapport's tone
// tier, the Protector's wording, the briefs, card tone, how full auto-plan
// makes a day. Six separate derivations would drift, and the user would meet a
// system that is gentle in one place and brisk in another about the same week.
func (s *Server) moodWindow(ctx context.Context, sid string) mood.Window {
	checkins, err := s.store.Moods().List(ctx, sid, moodWindowLimit)
	if err != nil {
		return mood.Window{Trend: mood.TrendUnknown}
	}
	return mood.Read(checkins, time.Now(), mood.DefaultConfig())
}

// moodHistoryContext renders the mood window for prompt injection.
//
// It is a window and not a list of the last five check-ins on purpose. A raw
// list invites the model to read whatever is on top as today's mood, which is
// how a single bad Tuesday three weeks ago ends up colouring a Thursday. The
// window states the direction, says how old the newest reading is, and says
// plainly when it does not know — so the template can branch on speakable
// instead of the model inferring a mood from a date it half-read.
func (s *Server) moodHistoryContext(ctx context.Context, sid string) string {
	w := s.moodWindow(ctx, sid)
	if !w.Known {
		if w.LastKind == "" {
			return `{"known":false,"why":"no check-ins"}`
		}
		return marshalCompact(map[string]any{
			"known": false, "why": "only faded check-ins",
			"lastKind": w.LastKind, "daysSince": daysSince(w.Staleness),
		})
	}
	out := map[string]any{
		"known":     true,
		"trend":     w.Trend,
		"tone":      w.Tone(),
		"speakable": w.Speakable(),
		"lastKind":  w.LastKind,
		"daysSince": daysSince(w.Staleness),
		"samples":   w.Samples,
	}
	if w.Stale {
		// The one thing the model must not do with an old reading is wear it as
		// today's mood, so say so in words rather than trusting it to infer the
		// rule from a number.
		out["note"] = "too old to assume; ask rather than presume"
	}
	return marshalCompact(out)
}

func daysSince(d time.Duration) int { return int(d.Hours() / 24) }
