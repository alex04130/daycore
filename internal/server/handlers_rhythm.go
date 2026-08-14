package server

import (
	"errors"
	"net/http"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/rhythm"
)

func init() {
	i18n.Register("err.rhythm.internal", i18n.Text{
		"zh-CN": "读取节律失败",
		"en-US": "Could not read rhythm",
	})
	i18n.Register("err.rhythm.invalid", i18n.Text{
		"zh-CN": "节律时间格式不对，要用 HH:MM",
		"en-US": "Rhythm times must be HH:MM",
	})
	registerRoutes("rhythm", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/rhythm", s.handleRhythmGet)
		mux.HandleFunc("POST /api/rhythm/pin", s.handleRhythmPin)
	})
}

// GET /api/rhythm — the session's rhythm: learned when enough evidence, pinned
// when the user fixed it by hand, otherwise the cold default. The one line the
// UI shows ("约 23:40 睡 · 07:30 起") is these two fields; `source` says which
// of the three produced them, and `days` how much evidence stands behind it.
func (s *Server) handleRhythmGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	p, err := s.store.Rhythm().Get(r.Context(), sid)
	switch {
	case errors.Is(err, domain.ErrNotFound), (err == nil && p.Wake == ""):
		cold := rhythm.Cold()
		s.writeJSON(w, http.StatusOK, map[string]any{
			"wake": cold.Wake, "sleep": cold.Sleep,
			"source": cold.Source, "days": 0,
		})
		return
	case err != nil:
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.rhythm.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"wake": p.Wake, "sleep": p.Sleep,
		"source": p.Source, "days": p.Days,
	})
}

// POST /api/rhythm/pin — fix a rhythm by hand ("我就是夜猫子，别管我").
// A pinned profile is never overwritten by the nightly learn job (rhythm.Learn
// returns it unchanged), so the user's explicit choice wins over the median of
// observed days.
func (s *Server) handleRhythmPin(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Wake  string `json:"wake"`
		Sleep string `json:"sleep"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.rhythm.invalid")
		return
	}
	p, err := rhythm.Pin(body.Wake, body.Sleep)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_rhythm", "err.rhythm.invalid")
		return
	}
	profile := &domain.RhythmProfile{
		SessionID: sid, Wake: p.Wake, Sleep: p.Sleep,
		Source: string(p.Source), Days: 0,
	}
	if err := s.store.Rhythm().Save(r.Context(), profile); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.rhythm.internal")
		return
	}
	// The cron moments derive from the rhythm; re-arm so the new times take hold.
	if s.worker != nil {
		s.worker.ScheduleUser(sid, s.sessionTimezone(r.Context(), sid))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"wake": p.Wake, "sleep": p.Sleep, "source": string(p.Source), "days": 0,
	})
}
