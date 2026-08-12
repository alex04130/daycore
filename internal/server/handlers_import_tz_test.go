package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/auth"
	"daycore/internal/ics"
)

// A timetable exported in one city and imported in another. The whole of
// adjudication #10 is that these are two different questions:
//
//	the session timezone   where the user is right now
//	the calendar timezone  whose wall clocks these times are
//
// "09:00" in a Shanghai timetable means 09:00 中国时间 whatever city the student
// is sitting in. Anchoring it to where they are shifts the entire term.
const shanghaiICS = "BEGIN:VCALENDAR\r\n" +
	"X-WR-TIMEZONE:Asia/Shanghai\r\n" +
	"BEGIN:VEVENT\r\n" +
	"DTSTART:20260907T090000\r\n" +
	"DTEND:20260907T104000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO\r\n" +
	"SUMMARY:线性代数\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// Floating times used to be parsed in time.Local — the SERVER's zone, which has
// nothing to do with either the user or the timetable. A container in UTC and a
// laptop in Shanghai turned the same file into times eight hours apart, and
// nothing anywhere said so.
func TestFloatingTimesFollowTheCalendarNotTheServer(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("no tzdata")
	}
	cal, _, err := ics.Parse(shanghaiICS, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Timezone != "Asia/Shanghai" {
		t.Fatalf("X-WR-TIMEZONE = %q, want Asia/Shanghai", cal.Timezone)
	}
	if !cal.Floating {
		t.Fatal("the file's times carry no Z and no TZID, so they are floating")
	}
	if len(cal.Events) != 1 {
		t.Fatalf("events = %d", len(cal.Events))
	}
	// X-WR-TIMEZONE wins over the caller's default: the file saying which city
	// its wall clocks belong to is better evidence than any guess.
	if got := cal.Events[0].Start.In(shanghai).Format("15:04"); got != "09:00" {
		t.Errorf("start = %s in Shanghai, want 09:00", got)
	}
	if got := cal.Events[0].Start.UTC().Format("15:04"); got != "01:00" {
		t.Errorf("start = %s UTC, want 01:00 — the calendar zone was ignored", got)
	}

	// And with no X-WR-TIMEZONE, the caller's default decides — which is the
	// argument that used to be time.Local.
	plain := strings.Replace(shanghaiICS, "X-WR-TIMEZONE:Asia/Shanghai\r\n", "", 1)
	cal, _, err = ics.Parse(plain, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	if got := cal.Events[0].Start.UTC().Format("15:04"); got != "01:00" {
		t.Errorf("with no X-WR-TIMEZONE the caller's default was not used: %s UTC", got)
	}
}

// A file whose times all carry their own Z or TZID has nothing floating to
// reinterpret, so there is nothing to ask about.
func TestAbsoluteTimesAreNotFloating(t *testing.T) {
	utcICS := "BEGIN:VCALENDAR\r\nX-WR-TIMEZONE:Asia/Shanghai\r\n" +
		"BEGIN:VEVENT\r\nDTSTART:20260907T010000Z\r\nSUMMARY:x\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, _, err := ics.Parse(utcICS, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Floating {
		t.Error("a UTC timestamp was reported as floating; the import would ask a question with no consequence")
	}
}

func importICS(t *testing.T, s *Server, sid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", versionPath("/api/import/ics"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withSessionID(req.Context(), sid))
	rec := httptest.NewRecorder()
	s.handleImportICS(rec, req)
	return rec
}

// Ask exactly once, and only when the answer could change something.
func TestICSImportAsksOnlyOnARealMismatch(t *testing.T) {
	s, sid := tzServer(t) // deployment default Asia/Shanghai
	ctx := context.Background()
	payload, _ := json.Marshal(map[string]any{"icsText": shanghaiICS})

	// Same zone: nothing to decide, import proceeds.
	if rec := importICS(t, s, sid, string(payload)); rec.Code != http.StatusOK {
		t.Fatalf("same-zone import asked or failed: %d %s", rec.Code, rec.Body.String())
	}

	// The user is somewhere else. Now it matters.
	s.noteClientTimezone(ctx, sid, "Europe/London")
	rec := importICS(t, s, sid, string(payload))
	if rec.Code != http.StatusConflict {
		t.Fatalf("a Shanghai timetable imported into a London session did not ask: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["calendarTimezone"] != "Asia/Shanghai" || got["sessionTimezone"] != "Europe/London" {
		t.Errorf("the question does not name both zones: %v", got)
	}

	// Answering it goes through.
	confirmed, _ := json.Marshal(map[string]any{"icsText": shanghaiICS, "tzConfirmed": true})
	if rec := importICS(t, s, sid, string(confirmed)); rec.Code != http.StatusOK {
		t.Fatalf("confirming the timezone still refused: %d %s", rec.Code, rec.Body.String())
	}
}

// Preview must never be blocked by the question: it is how a client shows the
// user what they are about to decide about.
func TestICSPreviewNeverAsks(t *testing.T) {
	s, sid := tzServer(t)
	s.noteClientTimezone(context.Background(), sid, "Europe/London")
	payload, _ := json.Marshal(map[string]any{"icsText": shanghaiICS, "preview": true})
	if rec := importICS(t, s, sid, string(payload)); rec.Code != http.StatusOK {
		t.Fatalf("preview was blocked by the timezone question: %d %s", rec.Code, rec.Body.String())
	}
}

// The materials page can correct an event's timezone, and cannot store one that
// does not exist.
func TestRuleTimezoneIsEditableAndValidated(t *testing.T) {
	s, sid := tzServer(t)
	s.cookies = auth.NewCookieSigner("rule-tz-test-secret")
	created := do(t, s, sid, http.MethodPost, "/api/rules",
		`{"title":"线性代数","type":"task","kind":"recurring","freq":"weekly","by_weekday":[1],"time":"09:00","timezone":"Asia/Shanghai"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var rule map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &rule); err != nil {
		t.Fatal(err)
	}
	id, _ := rule["id"].(string)

	ok := do(t, s, sid, http.MethodPatch, "/api/rules/"+id, `{"timezone":"Europe/London"}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("correcting the timezone failed: %d %s", ok.Code, ok.Body.String())
	}
	bad := do(t, s, sid, http.MethodPatch, "/api/rules/"+id, `{"timezone":"Not/AZone"}`)
	if bad.Code != http.StatusBadRequest {
		t.Errorf("a zone nothing can load was accepted: %d", bad.Code)
	}
}
