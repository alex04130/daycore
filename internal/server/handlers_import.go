package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/ics"
	"daycore/internal/schedule"
)

func init() {
	registerRoutes("imports", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/import/token", s.handleImportTokenGet)
		mux.HandleFunc("POST /api/import/token", s.handleImportTokenRotate)
		mux.HandleFunc("POST /api/import/ics", s.handleImportICS)
	})
}

// importSession resolves the acting session for import endpoints: the normal
// cookie session when present, else the X-Import-Token header (how the browser
// extension pushes without cookies). Writes the error response on failure.
func (s *Server) importSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	if sid := sessionIDFrom(r.Context()); sid != "" {
		return sid, true
	}
	token := strings.TrimSpace(r.Header.Get("X-Import-Token"))
	if token == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "no_session", "err.importSession.no_session")
		return "", false
	}
	sess, err := s.store.Sessions().GetByImportToken(r.Context(), token)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "invalid_import_token", "err.importSession.invalid_import_token")
		return "", false
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.importSession.internal")
		return "", false
	}
	return sess.ID, true
}

// GET /api/import/token — the session's current import token ("" when unset).
func (s *Server) handleImportTokenGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	sess, err := s.store.Sessions().Get(r.Context(), sid)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.importTokenGet.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"token": sess.ImportToken})
}

// POST /api/import/token — generate (or rotate) the import token.
func (s *Server) handleImportTokenRotate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.importTokenRotate.internal")
		return
	}
	token := "dcimp_" + hex.EncodeToString(buf)
	if _, err := s.store.Sessions().Update(r.Context(), sid, domain.SessionUpdate{ImportToken: &token}); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.importTokenRotate.internal2")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// POST /api/import/ics — parse a Google Calendar / registrar .ics export into
// schedule-rule candidates. Accepts raw text/calendar bodies or JSON
// {icsText, preview, timezone}. preview=true returns candidates without saving.
func (s *Server) handleImportICS(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.importSession(w, r)
	if !ok {
		return
	}
	var (
		icsText     string
		preview     bool
		defaultTZ   string
		tzConfirmed bool
	)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "text/calendar") {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.importICS.bad_request")
			return
		}
		icsText = string(raw)
		preview = r.URL.Query().Get("preview") == "1"
		defaultTZ = r.URL.Query().Get("timezone")
		tzConfirmed = r.URL.Query().Get("tzConfirmed") == "1"
	} else {
		var body struct {
			ICSText  string `json:"icsText"`
			Preview  bool   `json:"preview"`
			Timezone string `json:"timezone"`
			// TzConfirmed is the client saying "I saw the mismatch and this is
			// the zone I want". Without it the import stops and asks; with it,
			// `timezone` is taken as the answer.
			TzConfirmed bool `json:"tzConfirmed"`
		}
		if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.ICSText) == "" {
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.importICS.bad_request2")
			return
		}
		icsText, preview, defaultTZ = body.ICSText, body.Preview, body.Timezone
		tzConfirmed = body.TzConfirmed
	}

	// Two timezones, and the whole of adjudication #10 is about not confusing
	// them:
	//
	//	the session timezone   where the user is right now
	//	the calendar timezone  whose wall clocks these times are
	//
	// A student in London importing a Shanghai timetable means "09:00 中国时间"
	// — anchoring it to London shifts the entire term by eight hours. So a
	// floating time (no Z, no TZID) is interpreted in the CALENDAR's zone, and
	// the caller's `timezone` is only the fallback for a file that does not say.
	sessionTZ := s.sessionTimezone(r.Context(), sid)
	if defaultTZ == "" {
		defaultTZ = sessionTZ
	}
	cal, warnings, err := ics.Parse(icsText, resolveLocation(defaultTZ))
	if err != nil {
		s.writeErrf(w, s.requestLocale(r), http.StatusBadRequest, "invalid_ics", "err.fmt.invalidICS", err.Error())
		return
	}

	// Ask exactly once, and only when the answer could change something: the
	// file says which city it belongs to, that city is not where the user is,
	// and there are floating times whose meaning actually depends on it.
	//
	// Silent when they agree (nothing to decide) and silent when every time
	// carries its own Z or TZID (nothing floating to reinterpret). Asking anyway
	// would put a modal in front of the common case to protect the rare one,
	// which is how a prompt becomes something people click through.
	askTZ := cal.Timezone != "" && cal.Floating && !tzConfirmed &&
		!strings.EqualFold(cal.Timezone, sessionTZ)
	if askTZ && !preview {
		s.writeJSON(w, http.StatusConflict, map[string]any{
			"error":            "timezone_mismatch",
			"message":          i18n.Tf("err.importICS.timezone_mismatch", s.requestLocale(r), cal.Timezone, sessionTZ),
			"calendarTimezone": cal.Timezone,
			"sessionTimezone":  sessionTZ,
			"events":           len(cal.Events),
		})
		return
	}
	if cal.Timezone != "" {
		defaultTZ = cal.Timezone
	}

	events := cal.Events
	inputs := make([]ruleInput, 0, len(events))
	for _, ev := range events {
		in, warn := icsEventToRuleInput(ev, defaultTZ)
		warnings = append(warnings, warn...)
		if in != nil {
			inputs = append(inputs, *in)
		}
	}
	if preview {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"preview": true, "rules": inputs, "warnings": warnings,
		})
		return
	}

	created := []domain.ScheduleRule{}
	for i, in := range inputs {
		rule, err := in.toRule(sid)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("rules[%d] %q: %s，已跳过", i, in.Title, err))
			continue
		}
		c, err := s.store.Rules().Create(r.Context(), rule)
		if err != nil {
			s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.importICS.internal")
			return
		}
		created = append(created, *c)
	}
	s.recordImport(r.Context(), sid, "ics", len(created),
		fmt.Sprintf("%d rules from ICS", len(created)), icsText)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "rules": created, "created": len(created), "warnings": warnings,
	})
}

// icsEventToRuleInput maps one VEVENT onto the rule wire shape.
func icsEventToRuleInput(ev ics.Event, defaultTZ string) (*ruleInput, []string) {
	var warnings []string
	title := strings.TrimSpace(ev.Summary)
	if title == "" {
		return nil, []string{"跳过一条没有标题的事件"}
	}

	tz := ev.Start.Location().String()
	if tz == "Local" || tz == "" {
		tz = defaultTZ
	}
	in := &ruleInput{
		Title:    title,
		Type:     string(domain.BlockAppointment),
		Timezone: tz,
		TimeMode: string(domain.TimeFixed),
		Source:   "ics",
	}
	if ev.Location != "" {
		loc := ev.Location
		in.Note = &loc
	}
	if ev.AllDay {
		in.TimeMode = string(domain.TimeFloating)
	} else {
		t := ev.Start.Format("15:04")
		in.Time = &t
		if !ev.End.IsZero() && ev.End.After(ev.Start) {
			min := int(ev.End.Sub(ev.Start).Minutes())
			in.DurationMin = &min
		}
	}

	startDate := ev.Start.Format("2006-01-02")
	if ev.RRule == nil {
		in.Kind = domain.RuleOnce
		in.Date = &startDate
		return in, warnings
	}

	in.Kind = domain.RuleRecurring
	in.StartDate = startDate
	in.Interval = ev.RRule.Interval
	switch ev.RRule.Freq {
	case "WEEKLY":
		in.Freq = domain.FreqWeekly
		in.ByWeekday = ev.RRule.ByDay
	case "DAILY":
		if ev.RRule.Interval > 1 {
			in.Freq = domain.FreqEveryNDays
		} else {
			in.Freq = domain.FreqDaily
		}
	case "MONTHLY":
		in.Freq = domain.FreqMonthly
	}
	if ev.RRule.Until != nil {
		u := ev.RRule.Until.In(ev.Start.Location()).Format("2006-01-02")
		in.Until = &u
	} else if ev.RRule.Count > 0 {
		if u := untilFromCount(in, ev.RRule.Count); u != "" {
			in.Until = &u
		} else {
			warnings = append(warnings, fmt.Sprintf("事件 %q 的 COUNT 无法换算成结束日期，规则将无限重复", title))
		}
	}
	return in, warnings
}

// untilFromCount walks the recurrence to find the COUNT-th occurrence date
// (capped at 3 years), so RRULE COUNT semantics survive the conversion.
func untilFromCount(in *ruleInput, count int) string {
	rule := &domain.ScheduleRule{
		Title: in.Title, Kind: in.Kind, Freq: in.Freq, Interval: in.Interval,
		ByWeekday: in.ByWeekday, StartDate: in.StartDate, Active: true,
	}
	start, err := time.Parse("2006-01-02", in.StartDate)
	if err != nil {
		return ""
	}
	seen := 0
	for d, i := start, 0; i < 1096; d, i = d.AddDate(0, 0, 1), i+1 {
		if schedule.Occurs(rule, d) {
			seen++
			if seen == count {
				return d.Format("2006-01-02")
			}
		}
	}
	return ""
}
