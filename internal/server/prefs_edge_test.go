package server

import (
	"context"
	"reflect"
	"testing"
)

// "null", the empty string and whitespace all unmarshal into the zero value,
// and a zero SessionPrefs has every proactive toggle OFF. These are
// "never configured", not "configured with everything off" — one legacy row
// must not silently kill the briefs, deadline warnings and replan for an
// otherwise ordinary session.
func TestParsePrefsNullAndEmptyAreDefaults(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", " null "} {
		p := parsePrefs(raw)
		if !reflect.DeepEqual(p, DefaultPrefs()) {
			t.Errorf("parsePrefs(%q) = %+v, want the defaults %+v", raw, p, DefaultPrefs())
		}
	}
}

// A partial JSON object still wins for the fields it names: a settings patch
// that only touched one toggle keeps working, and the untouched toggles keep
// their defaults.
func TestParsePrefsPartialJSONKeepsDefaultsForTheRest(t *testing.T) {
	p := parsePrefs(`{"doNotDisturb":true}`)
	if !p.DoNotDisturb {
		t.Error("the named field must win")
	}
	if !p.MorningBrief || !p.DeadlineAlerts {
		t.Errorf("unnamed fields must keep their defaults, got %+v", p)
	}
	p = parsePrefs("not json at all")
	if !reflect.DeepEqual(p, DefaultPrefs()) {
		t.Errorf("unparseable prefs must fall back to defaults, got %+v", p)
	}
}

// " Asia/Shanghai " passes validTimezone (which trims for its screen) but was
// stored verbatim — the hint is accepted and then fails every LoadLocation
// forever. The stored value must be the value that was validated.
func TestNoteClientTimezoneTrimsBeforeStoring(t *testing.T) {
	s, sid := newAgentTestServer(t)
	s.cfg.WorkerDefaultTZ = "UTC"
	s.SetWorker(NewWorker(s, nil))

	s.noteClientTimezone(context.Background(), sid, " Asia/Shanghai ")
	if err := s.WaitBackground(context.Background()); err != nil {
		t.Fatal(err)
	}
	prefs := s.sessionPrefs(context.Background(), sid)
	if prefs.Timezone != "Asia/Shanghai" {
		t.Errorf("stored timezone = %q, want the trimmed %q", prefs.Timezone, "Asia/Shanghai")
	}
	if prefs.TimezoneSource != TZSourceDetected {
		t.Errorf("source = %q, want detected", prefs.TimezoneSource)
	}
}
