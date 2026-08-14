package server

import (
	"net/http"
	"strconv"

	"daycore/internal/i18n"
)

const (
	riverDaysDefault = 15
	riverDaysMin     = 1
	riverDaysMax     = 90
)

var keyRiverInternal = i18n.Reg("err.river.internal", i18n.Text{
	"zh-CN": "足迹河读不到",
	"en-US": "Could not read the activity river",
})

func init() {
	registerRoutes("river", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/river", s.handleRiver)
	})
}

// GET /api/river?days=15 — the activity river: one row per day.
//
// Each row is {date, count, mood}: count is how many operations were logged that
// UTC day, and mood is the emoji of the day's newest resolvable check-in ("" when
// there is none). The band is continuous — an empty day is still a row — and it
// runs oldest first so a client renders it left to right without re-sorting.
//
// Read-only: the river is a fold of the append-only ledger and the check-in
// table, so it writes nothing and therefore has nothing to undo.
func (s *Server) handleRiver(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	days := riverDaysDefault
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	if days < riverDaysMin {
		days = riverDaysMin
	}
	if days > riverDaysMax {
		days = riverDaysMax
	}
	rows, err := s.store.River().Days(r.Context(), sid, days)
	if err != nil {
		s.log.Error("river", "err", err)
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", keyRiverInternal)
		return
	}
	s.writeJSON(w, http.StatusOK, rows)
}
