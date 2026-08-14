package server

import (
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

func init() {
	// 逆操作与写入放在同一个文件 —— 改写入的人正好看得见它。
	registerRevert("mood_record", (*Server).revertMoodRecord_delete)

	registerRoutes("moods", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/mood/kinds", s.handleMoodKinds)
		mux.HandleFunc("GET /api/mood", s.handleMoodList)
		mux.HandleFunc("POST /api/mood", s.handleMoodCreate)
		mux.HandleFunc("PATCH /api/mood", s.handleMoodPatch)
	})
}

// GET /api/mood?limit=10 — recent check-ins (newest first).
func (s *Server) handleMoodList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.store.Moods().List(r.Context(), sid, limit)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.moodList.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, rows)
}

// POST /api/mood — record a check-in; bumps interaction count.
func (s *Server) handleMoodCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Mood            string  `json:"mood"`
		AIResponse      *string `json:"aiResponse"`
		ExerciseOffered *string `json:"exerciseOffered"`
		Theme           *string `json:"theme"`
		Note            string  `json:"note"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Mood == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.moodCreate.bad_request")
		return
	}
	// The stored value is a registry id, never a display string.
	//
	// The shipped frontend used to send `emoji + " " + localizedName` ("😊 开心"),
	// which `MoodKindByID` could not resolve — so `mood.Read` skipped every sample
	// and the companion was told "no check-ins" no matter how many times the user
	// checked in. The feature looked fine from the outside: the row saved, the
	// mood page answered. Only the thing that consumed it was empty.
	//
	// It was also localized, so the same feeling was a different string per UI
	// language — the exact reason ids and not labels are what gets stored.
	//
	// Rejecting here rather than accepting and hoping: a value the registry cannot
	// resolve has no valence, and a check-in with no valence is not a weaker
	// signal, it is no signal.
	kind, known := domain.MoodKindByID(body.Mood)
	if !known {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "unknown_mood",
			"err.moodCreate.unknown_mood")
		return
	}
	// Source is set here, never read from the body. A check-in that arrives on
	// this endpoint is the user pressing a button; the agent records its own
	// through its tool. Letting a client claim source=agent would let it write
	// check-ins that the mood window quietly discounts — or, worse, let a
	// frontend bug relabel real ones as inferences.
	ctx := r.Context()
	checkin, err := s.store.Moods().Create(ctx, &domain.MoodCheckin{
		SessionID: sid, Mood: body.Mood, AIResponse: body.AIResponse,
		ExerciseOffered: body.ExerciseOffered, Theme: body.Theme,
		Source: domain.MoodSourceUser, Note: strings.TrimSpace(body.Note),
	})
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.moodCreate.internal")
		return
	}
	_ = s.store.Sessions().IncrementInteraction(ctx, sid)
	// Same op the agent's mood_record tool writes, so the same revert handler
	// takes it back. ⚠️ Actor is left to default (user) rather than copied from
	// the tool path — the ledger's whole job is to say WHO did it, and this door
	// is the person pressing a button.
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Action: "mood_record", TargetID: checkin.ID,
		Summary: kind.MoodName(s.requestLocale(r)),
		Detail:  marshalCompact(map[string]any{"before": nil, "after": checkin}),
	})
	s.writeJSON(w, http.StatusOK, checkin)
}

// PATCH /api/mood — mark a check-in's exercise as completed.
func (s *Server) handleMoodPatch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := s.readJSON(r, &body); err != nil || body.ID == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.moodPatch.bad_request")
		return
	}
	if err := s.store.Moods().MarkExerciseCompleted(r.Context(), sid, body.ID); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.moodPatch.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/mood/kinds — the mood registry, localized.
//
// Frontends render from this instead of keeping their own list. The shipped one
// kept its own and the two drifted into different vocabularies: four ids named
// differently for the same emoji, two the frontend had and the backend did not
// (bored, lonely), two the backend had and the frontend did not (neutral,
// sleepless). With four frontends coming, "everyone keeps their own copy" is not
// a bug that gets fixed once — it is one that happens four more times.
//
// Valence is deliberately NOT exposed. The domain comment calls it coarse and
// says it is never shown to the user — putting it in the contract is how it ends
// up rendered as a score, and "your week was -4" is precisely the shame the
// product's底色 forbids.
//
// Shaped like GET /api/materials/categories, which solved the same problem.
func (s *Server) handleMoodKinds(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSession(w, r); !ok {
		return
	}
	locale := s.requestLocale(r)
	kinds := domain.MoodKinds()
	out := make([]map[string]any, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, map[string]any{
			// The label goes through the catalog, not through k.Names directly:
			// reading the struct would skip the database and file layers, which is
			// the whole point of having them (a language pack renames a mood
			// without a rebuild).
			"id": k.ID, "emoji": k.Emoji, "name": i18n.T(domain.MoodNameKey(k.ID), locale),
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"kinds": out})
}
