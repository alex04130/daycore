package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

func init() {
	registerRoutes("ai", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/ai/travel", s.handleAITravel)
	})
}

// POST /api/ai/travel — destination + dates → AI itinerary suggestion. The
// result is returned for display AND stored as an inbox draft, so confirming
// it is the same POST /api/inbox/commit flow as any quick capture (it lands as
// a travel material).
func (s *Server) handleAITravel(w http.ResponseWriter, r *http.Request) {
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
		Destination string `json:"destination"`
		StartDate   string `json:"startDate"`
		EndDate     string `json:"endDate"`
		Notes       string `json:"notes"`
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Destination) == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.aITravel.bad_request")
		return
	}
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	// The draft lands in the travel category, so it must be enabled.
	if !s.enabledMaterialCategories(r.Context(), sid)["travel"] {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_category", "err.aITravel.bad_category")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.runtime().AIRequestTimeout)
	defer cancel()
	locale := s.requestLocale(r)
	prompt, err := s.prompts.Render(ctx, ai.PromptTravelSuggest, locale, ai.TravelSuggestData{
		Destination: strings.TrimSpace(body.Destination),
		StartDate:   body.StartDate,
		EndDate:     body.EndDate,
		Notes:       body.Notes,
		Date:        time.Now().Format("2006-01-02"),
	})
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.aITravel.internal")
		return
	}
	provider := s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleUser, Content: prompt}},
		JSONMode:    true,
		Temperature: 0.6,
		MaxTokens:   4096,
	})
	s.logAICall(ctx, sid, epTravel, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		s.log.Error("ai travel", "err", err)
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "server_error", "err.aITravel.server_error")
		return
	}
	obj, ok2 := extractJSONObject(resp.Content)
	if !ok2 {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "parse_error", "err.aITravel.parse_error")
		return
	}

	title, _ := obj["title"].(string)
	summary, _ := obj["summary"].(string)
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSpace(body.Destination)
	}
	structured := map[string]any{"destination": strings.TrimSpace(body.Destination)}
	if body.StartDate != "" {
		structured["start_date"] = body.StartDate
	}
	if body.EndDate != "" {
		structured["end_date"] = body.EndDate
	}
	for _, k := range []string{"itinerary", "tips"} {
		if v, exists := obj[k]; exists {
			structured[k] = v
		}
	}
	cls := inboxClassification{
		Category: "travel", Confidence: 1, Title: title, Summary: summary,
		Structured:      structured,
		SuggestedAction: map[string]any{"action": "save_material", "entity": "travel"},
	}
	draftID := genTempID()
	payload, _ := json.Marshal(inboxDraft{
		Text:           strings.TrimSpace(strings.Join([]string{body.Destination, body.StartDate, body.EndDate, body.Notes}, " ")),
		Classification: cls,
	})
	_ = s.store.TempContexts().Set(r.Context(), &domain.TempContext{
		SessionID: sid, Key: inboxDraftKey(draftID), Payload: string(payload), TTL: time.Now().Add(time.Hour),
	})

	s.writeJSON(w, http.StatusOK, map[string]any{
		"title":     title,
		"summary":   summary,
		"itinerary": obj["itinerary"],
		"tips":      obj["tips"],
		"draftId":   draftID,
	})
}
