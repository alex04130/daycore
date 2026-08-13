package ics

import (
	"strings"
	"testing"
	"time"
)

func parseOne(t *testing.T, icsText string) (Calendar, []string, error) {
	t.Helper()
	return Parse(icsText, time.UTC)
}

func TestXWRTimezoneOverridesFloating(t *testing.T) {
	const icsText = "BEGIN:VCALENDAR\r\nX-WR-TIMEZONE:Asia/Shanghai\r\nBEGIN:VEVENT\r\nSUMMARY:数学课\r\nDTSTART:20260907T090000\r\nDTEND:20260907T103000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, warns, err := parseOne(t, icsText)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Timezone != "Asia/Shanghai" {
		t.Errorf("Timezone = %q", cal.Timezone)
	}
	if len(cal.Events) != 1 || !cal.Floating {
		t.Fatalf("events=%d floating=%v", len(cal.Events), cal.Floating)
	}
	// 09:00 Shanghai = 01:00 UTC.
	if got := cal.Events[0].Start.UTC().Format("15:04"); got != "01:00" {
		t.Errorf("the floating time must be resolved in X-WR-TIMEZONE, got %s", got)
	}
	_ = warns
}

func TestXWRTimezoneInvalidFallsBackWithWarning(t *testing.T) {
	const icsText = "BEGIN:VCALENDAR\r\nX-WR-TIMEZONE:Mars/Phobos\r\nBEGIN:VEVENT\r\nSUMMARY:X\r\nDTSTART:20260907T090000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, warns, err := parseOne(t, icsText)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "Mars/Phobos") {
		t.Errorf("an unknown zone must warn, got %v", warns)
	}
	if cal.Events[0].Start.UTC().Format("15:04") != "09:00" {
		t.Error("with the zone rejected the caller's defaultTZ (UTC) must apply")
	}
}

func TestUnknownTZIDSilentlyFloating(t *testing.T) {
	// A TZID this build cannot load falls back to the default zone while
	// STILL marking the time floating — locked behaviour, see parseDateTime.
	const icsText = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:X\r\nDTSTART;TZID=Mars/Phobos:20260907T090000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, _, err := parseOne(t, icsText)
	if err != nil {
		t.Fatal(err)
	}
	if !cal.Floating {
		t.Error("an unresolvable TZID leaves the time floating")
	}
}

func TestRRuleUntilShapes(t *testing.T) {
	cases := []struct{ val, wantUTC string }{
		{"UNTIL=20261001", "2026-10-01T00:00:00Z"},
		{"UNTIL=20261001T235959Z", "2026-10-01T23:59:59Z"},
		{"UNTIL=20261001T235959", "2026-10-01T23:59:59Z"},
	}
	for _, tc := range cases {
		r, warn := parseRRule("FREQ=DAILY;"+tc.val, time.UTC)
		if warn != "" || r == nil || r.Until == nil {
			t.Fatalf("parseRRule(%q) = (%v, %q)", tc.val, r, warn)
		}
		if got := r.Until.UTC().Format(time.RFC3339); got != tc.wantUTC {
			t.Errorf("Until(%s) = %s, want %s", tc.val, got, tc.wantUTC)
		}
	}
	// A malformed UNTIL is silently dropped — the rule just runs open-ended.
	r, warn := parseRRule("FREQ=DAILY;UNTIL=garbage", time.UTC)
	if warn != "" || r == nil || r.Until != nil {
		t.Errorf("a bad UNTIL is ignored (locked behaviour), got (%v, %q)", r, warn)
	}
}

func TestRRuleDegenerateValues(t *testing.T) {
	// INTERVAL garbage/zero/negative all collapse to the default 1.
	for _, v := range []string{"FREQ=DAILY;INTERVAL=abc", "FREQ=DAILY;INTERVAL=0", "FREQ=DAILY;INTERVAL=-2"} {
		r, warn := parseRRule(v, time.UTC)
		if warn != "" || r == nil || r.Interval != 1 {
			t.Errorf("parseRRule(%q) interval = %+v (%q)", v, r, warn)
		}
	}
	// COUNT garbage/zero are dropped; a positive count survives.
	r, _ := parseRRule("FREQ=WEEKLY;COUNT=0", time.UTC)
	if r == nil || r.Count != 0 {
		t.Errorf("COUNT=0 must be unset, got %+v", r)
	}
	r, _ = parseRRule("FREQ=WEEKLY;COUNT=5", time.UTC)
	if r == nil || r.Count != 5 {
		t.Errorf("COUNT=5 must parse, got %+v", r)
	}
	// A missing FREQ is unsupported → nil rule + warning, event becomes a
	// one-off rather than failing the import.
	if r, warn := parseRRule("INTERVAL=2", time.UTC); r != nil || warn == "" {
		t.Errorf("a rule without FREQ must downgrade with a warning, got (%v, %q)", r, warn)
	}
}

func TestRRuleByDayOrdinalDowngrades(t *testing.T) {
	// Ordinal prefixes (2MO, -1FR) are beyond the subset: the whole rule
	// downgrades to a one-off WITH a warning, not a silent wrong schedule.
	r, warn := parseRRule("FREQ=WEEKLY;BYDAY=2MO", time.UTC)
	if r != nil || !strings.Contains(warn, "2MO") {
		t.Errorf("BYDAY=2MO must downgrade, got (%v, %q)", r, warn)
	}
	r, warn = parseRRule("FREQ=WEEKLY;BYDAY=XX", time.UTC)
	if r != nil || !strings.Contains(warn, "XX") {
		t.Errorf("an unknown weekday must downgrade, got (%v, %q)", r, warn)
	}
	r, warn = parseRRule("FREQ=WEEKLY;BYDAY=MO,WE", time.UTC)
	if r == nil || warn != "" || len(r.ByDay) != 2 {
		t.Errorf("BYDAY=MO,WE must parse, got (%+v, %q)", r, warn)
	}
}

func TestBadDateTimeHandling(t *testing.T) {
	// A bad DTSTART skips the whole event with a warning.
	const noStart = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:X\r\nDTSTART:garbage\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, warns, err := parseOne(t, noStart)
	if err != nil || len(cal.Events) != 0 || len(warns) != 1 {
		t.Fatalf("bad DTSTART: events=%d warns=%v err=%v", len(cal.Events), warns, err)
	}
	// A missing DTSTART is skipped too.
	const missing = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:X\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, warns, err = parseOne(t, missing)
	if err != nil || len(cal.Events) != 0 || len(warns) != 1 {
		t.Fatalf("missing DTSTART: events=%d warns=%v err=%v", len(cal.Events), warns, err)
	}
	// A bad DTEND warns and leaves End zero — the event itself survives.
	const badEnd = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:X\r\nDTSTART:20260907T090000Z\r\nDTEND:garbage\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, warns, err = parseOne(t, badEnd)
	if err != nil || len(cal.Events) != 1 || !cal.Events[0].End.IsZero() || len(warns) != 1 {
		t.Fatalf("bad DTEND: events=%+v warns=%v err=%v", cal.Events, warns, err)
	}
}

func TestUnterminatedVEVENT(t *testing.T) {
	// A VEVENT that never ends swallows the rest of the file silently —
	// locked as the current behaviour: no panic, no phantom event.
	const icsText = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:X\r\nDTSTART:20260907T090000Z\r\nEND:VCALENDAR\r\n"
	cal, _, err := parseOne(t, icsText)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Events) != 0 {
		t.Errorf("an unterminated VEVENT must not produce an event, got %+v", cal.Events)
	}
}

func TestUnescapeAllForms(t *testing.T) {
	cases := map[string]string{
		`a\,b`: "a,b",
		`a\;b`: "a;b",
		`a\nb`: "a\nb",
		`a\Nb`: "a\nb",
		`a\\b`: `a\b`,
	}
	for in, want := range cases {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnfoldTabContinuation(t *testing.T) {
	got := unfold("BEGIN:VCALENDAR\nX-A:\n\tcontinued\nEND:VCALENDAR\n")
	if len(got) != 3 || got[1] != "X-A:continued" {
		t.Errorf("tab continuation must fold, got %v", got)
	}
}

func TestSplitLineShapes(t *testing.T) {
	name, params, value := splitLine("DTSTART;TZID=Asia/Shanghai;VALUE=DATE-TIME:20260907T090000")
	if name != "DTSTART" || params["TZID"] != "Asia/Shanghai" || value != "20260907T090000" {
		t.Errorf("splitLine = (%q, %v, %q)", name, params, value)
	}
	name, params, value = splitLine("NOPARAMS")
	if name != "NOPARAMS" || params != nil || value != "" {
		t.Errorf("a line without colon: (%q, %v, %q)", name, params, value)
	}
	name, params, value = splitLine("P;X=\"a b\":v")
	if params["X"] != "a b" {
		t.Errorf("quoted params must be trimmed, got %v", params)
	}
}

func TestEmptyAndNonICS(t *testing.T) {
	if _, _, err := Parse("", time.UTC); err == nil {
		t.Error("an empty payload must error")
	}
	if _, _, err := Parse("just some text\n", time.UTC); err == nil || !strings.Contains(err.Error(), "VCALENDAR") {
		t.Errorf("a payload without VCALENDAR must be named, got %v", err)
	}
	// A VCALENDAR with zero events is valid — an empty calendar is not an error.
	cal, _, err := Parse("BEGIN:VCALENDAR\nEND:VCALENDAR\n", time.UTC)
	if err != nil || len(cal.Events) != 0 {
		t.Errorf("an empty calendar must parse, got (%+v, %v)", cal, err)
	}
}
