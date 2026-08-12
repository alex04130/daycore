package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/config"
)

func tzServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)
	s.cfg.WorkerDefaultTZ = "Asia/Shanghai"
	return s, sid
}

func patchPrefs(t *testing.T, s *Server, sid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("PATCH", versionPath("/api/session/preferences"), strings.NewReader(body))
	req = req.WithContext(withSessionID(req.Context(), sid))
	rec := httptest.NewRecorder()
	s.handleSessionPreferences(rec, req)
	return rec
}

// Before ζ-4 every clock in the server read one deployment-wide value, so a user
// in another zone had their petrify line, their rhythm day boundary and their
// morning brief all drawn in somebody else's day.
func TestSessionTimezoneFallsBackToTheDeployment(t *testing.T) {
	s, sid := tzServer(t)
	ctx := context.Background()

	if got := s.sessionTimezone(ctx, sid); got != "Asia/Shanghai" {
		t.Errorf("a session with no timezone = %q, want the deployment default", got)
	}
	// And with no deployment default either, UTC rather than nil.
	s.cfg = &config.Config{}
	if got := s.sessionTimezone(ctx, sid); got != "UTC" {
		t.Errorf("with no default configured = %q, want UTC", got)
	}
	if s.sessionLocation(ctx, sid) == nil {
		t.Fatal("sessionLocation returned nil; every caller does date arithmetic with it")
	}
}

// The device is the only thing that knows which zone it is in, so a hint is
// accepted — but only where the user has not already said otherwise.
func TestClientHintFillsInButNeverOverridesTheUser(t *testing.T) {
	s, sid := tzServer(t)
	ctx := context.Background()

	s.noteClientTimezone(ctx, sid, "America/Chicago")
	if got := s.sessionTimezone(ctx, sid); got != "America/Chicago" {
		t.Fatalf("the hint was not adopted: %q", got)
	}
	if got := s.sessionPrefs(ctx, sid).TimezoneSource; got != TZSourceDetected {
		t.Errorf("source = %q, want %q", got, TZSourceDetected)
	}
	// A later hint updates a detected value: it was only ever a guess.
	s.noteClientTimezone(ctx, sid, "Europe/Berlin")
	if got := s.sessionTimezone(ctx, sid); got != "Europe/Berlin" {
		t.Errorf("a second hint did not update a detected value: %q", got)
	}

	// The user chooses on the settings page.
	if rec := patchPrefs(t, s, sid, `{"timezone":"Asia/Tokyo"}`); rec.Code != http.StatusOK {
		t.Fatalf("PATCH timezone: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.sessionPrefs(ctx, sid).TimezoneSource; got != TZSourceUser {
		t.Errorf("source after an explicit set = %q, want %q", got, TZSourceUser)
	}

	// From here a device hint must NOT move it. Somebody who deliberately keeps
	// their schedule on home time would otherwise have it moved the first time
	// they opened the app from an airport.
	s.noteClientTimezone(ctx, sid, "America/Chicago")
	if got := s.sessionTimezone(ctx, sid); got != "Asia/Tokyo" {
		t.Errorf("a device hint overrode the user's own choice: %q", got)
	}

	// Clearing it goes back to following the device.
	if rec := patchPrefs(t, s, sid, `{"timezone":""}`); rec.Code != http.StatusOK {
		t.Fatalf("clearing: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.sessionTimezone(ctx, sid); got != "Asia/Shanghai" {
		t.Errorf("after clearing = %q, want the deployment default again", got)
	}
	s.noteClientTimezone(ctx, sid, "America/Chicago")
	if got := s.sessionTimezone(ctx, sid); got != "America/Chicago" {
		t.Errorf("after clearing, hints are ignored: %q", got)
	}
}

// Everything a client could send that is not a zone is refused.
//
// ⚠️ This asserts the OUTCOME, not the mechanism: time.LoadLocation already
// rejects every case below, so the pre-screen in validTimezone is a bound on
// what reaches the lookup rather than the thing that decides. An earlier version
// of this comment claimed the screen was the guard, and removing the screen left
// the test green — which is exactly the kind of assertion this repo keeps
// writing by accident.
func TestTimezoneValidation(t *testing.T) {
	for _, bad := range []string{
		"", "   ", "Not/AZone", "../../etc/passwd", "/etc/localtime",
		"Asia/Shanghai\x00", strings.Repeat("A", 65),
	} {
		if validTimezone(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, good := range []string{"UTC", "Asia/Shanghai", "America/Chicago", " Europe/Berlin "} {
		if !validTimezone(good) {
			t.Errorf("rejected %q", good)
		}
	}

	// And the endpoint refuses rather than storing garbage.
	s, sid := tzServer(t)
	if rec := patchPrefs(t, s, sid, `{"timezone":"Not/AZone"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH with a bad zone: %d, want 400", rec.Code)
	}
	if got := s.sessionPrefs(context.Background(), sid).Timezone; got != "" {
		t.Errorf("the refused value was stored anyway: %q", got)
	}
}

// A stored timezone the cron entries do not reflect is a value nobody reads:
// the entries carry CRON_TZ of the zone they were built with.
func TestTimezoneChangeRearmsTheCronEntries(t *testing.T) {
	s, sid := tzServer(t)
	w := NewWorker(s, nil)
	s.SetWorker(w)
	w.ScheduleUser(sid, "Asia/Shanghai")
	before := w.EntryCount()

	if rec := patchPrefs(t, s, sid, `{"timezone":"America/Chicago"}`); rec.Code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", rec.Code, rec.Body.String())
	}
	if got := w.EntryCount(); got != before {
		t.Errorf("after a timezone change: %d entries, want %d — the old zone's jobs are still armed", got, before)
	}
	if got := w.sched[sid]; got != "America/Chicago" {
		t.Errorf("the entries were rebuilt for %q, not the new zone", got)
	}

	// A no-op PATCH must not churn the entries.
	if rec := patchPrefs(t, s, sid, `{"timezone":"America/Chicago"}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if got := w.EntryCount(); got != before {
		t.Errorf("a no-op timezone PATCH changed the entry count to %d", got)
	}
}

// The petrify line is drawn in the session's own zone, which is the whole point
// of ζ-4 — before it, a user in another zone had their evening freeze at
// somebody else's hour.
func TestPlanLocationFollowsTheSession(t *testing.T) {
	s, sid := tzServer(t)
	ctx := context.Background()

	if got := s.planLocation(ctx, sid).String(); got != "Asia/Shanghai" {
		t.Errorf("planLocation = %q, want the deployment default", got)
	}
	s.noteClientTimezone(ctx, sid, "America/Chicago")
	if got := s.planLocation(ctx, sid).String(); got != "America/Chicago" {
		t.Errorf("planLocation = %q, want the session's own zone", got)
	}

	// Two sessions in different zones must not share one line.
	if _, err := s.store.Sessions().GetOrCreate(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	s.noteClientTimezone(ctx, "other", "Europe/Berlin")
	if a, b := s.planLocation(ctx, sid).String(), s.planLocation(ctx, "other").String(); a == b {
		t.Errorf("two sessions in different zones both got %q", a)
	}
}

// The stored preferences must round-trip through the JSON blob — it is the
// sessions.preferences column, not a typed one, so a missing tag is invisible
// until someone reads it back.
func TestTimezoneSurvivesThePreferencesBlob(t *testing.T) {
	prefs := DefaultPrefs()
	prefs.Timezone = "Asia/Tokyo"
	prefs.TimezoneSource = TZSourceUser
	raw, err := json.Marshal(prefs)
	if err != nil {
		t.Fatal(err)
	}
	var back SessionPrefs
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Timezone != "Asia/Tokyo" || back.TimezoneSource != TZSourceUser {
		t.Errorf("round trip lost the timezone: %+v", back)
	}
}
