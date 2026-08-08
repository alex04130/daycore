// Package ics is a deliberately minimal iCalendar (RFC 5545) reader covering
// what schedule imports actually need: VEVENT with SUMMARY / LOCATION /
// DTSTART / DTEND and the common RRULE subset produced by Google Calendar and
// university registrars (FREQ=DAILY|WEEKLY|MONTHLY, INTERVAL, BYDAY, UNTIL,
// COUNT). Anything fancier degrades gracefully with a warning instead of
// failing the whole import. No third-party dependency.
package ics

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Event is one VEVENT.
type Event struct {
	Summary  string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
	RRule    *RRule // nil for single occurrences
}

// RRule is the supported recurrence subset.
type RRule struct {
	Freq     string // "DAILY" | "WEEKLY" | "MONTHLY"
	Interval int    // >= 1
	ByDay    []int  // 0=Sunday … 6=Saturday (WEEKLY)
	Until    *time.Time
	Count    int // 0 = unset
}

// Calendar is one parsed .ics file: its events plus what the file itself says
// about time.
//
// The two extra fields exist because "when is this class" has two different
// answers and only one of them is in the events:
//
//	Timezone  what the FILE claims, from X-WR-TIMEZONE. Google Calendar and
//	          most university exports write it; it is the closest thing an .ics
//	          has to "these wall-clock times belong to this city".
//	Floating  at least one event gave a time with no Z and no TZID. Those are
//	          the ones whose meaning depends entirely on the answer above, and
//	          they are the common case in university timetable exports.
//
// A file that is all-UTC or all-TZID needs no question asked; one with floating
// times and no X-WR-TIMEZONE is one where somebody has to say.
type Calendar struct {
	Events   []Event
	Timezone string
	Floating bool
}

// Parse reads an .ics payload. Per-event problems become warnings; only a
// payload with no VCALENDAR structure at all errors.
//
// defaultTZ is what floating times (no Z, no TZID) are interpreted in. It is a
// REQUIRED argument rather than a fallback to time.Local, which is what this
// used to do — and time.Local is the server's zone, which has nothing to do with
// either the user or the timetable. A container in UTC and a laptop in Shanghai
// parsed the same file into times eight hours apart, silently.
//
// X-WR-TIMEZONE, when present, overrides defaultTZ for floating times: the file
// saying which city its wall clocks belong to is better evidence than anything
// the caller can guess. The caller still gets Calendar.Timezone so it can ask.
func Parse(data string, defaultTZ *time.Location) (Calendar, []string, error) {
	if defaultTZ == nil {
		defaultTZ = time.UTC
	}
	lines := unfold(data)
	if len(lines) == 0 {
		return Calendar{}, nil, fmt.Errorf("empty ics payload")
	}

	var (
		cal      Calendar
		warnings []string
		cur      map[string]property
		inEvent  bool
		sawCal   bool
	)
	// Two passes over the header would be tidier; one pass is enough because
	// X-WR-TIMEZONE is a VCALENDAR property and therefore always precedes the
	// first VEVENT in any file that has it.
	for _, line := range lines {
		name, params, value := splitLine(line)
		switch {
		case name == "BEGIN" && strings.EqualFold(value, "VCALENDAR"):
			sawCal = true
		case !inEvent && strings.EqualFold(name, "X-WR-TIMEZONE"):
			if tz := strings.TrimSpace(value); tz != "" {
				if l, err := time.LoadLocation(tz); err == nil {
					cal.Timezone, defaultTZ = tz, l
				} else {
					warnings = append(warnings, fmt.Sprintf("calendar timezone %q is not a zone this build knows; falling back", tz))
				}
			}
		case name == "BEGIN" && strings.EqualFold(value, "VEVENT"):
			inEvent = true
			cur = map[string]property{}
		case name == "END" && strings.EqualFold(value, "VEVENT"):
			if inEvent {
				ev, floating, warns := buildEvent(cur, defaultTZ)
				warnings = append(warnings, warns...)
				if ev != nil {
					cal.Events = append(cal.Events, *ev)
					cal.Floating = cal.Floating || floating
				}
			}
			inEvent = false
		default:
			if inEvent {
				cur[name] = property{params: params, value: value}
			}
		}
	}
	if !sawCal {
		return Calendar{}, nil, fmt.Errorf("not an iCalendar file (no BEGIN:VCALENDAR)")
	}
	return cal, warnings, nil
}

type property struct {
	params map[string]string
	value  string
}

// unfold undoes RFC 5545 line folding (continuation lines start with a space
// or tab) and normalizes line endings.
func unfold(data string) []string {
	raw := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range raw {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += line[1:]
			continue
		}
		out = append(out, line)
	}
	return out
}

// splitLine parses `NAME;PARAM=V;PARAM2=V2:value` into its parts.
func splitLine(line string) (name string, params map[string]string, value string) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return strings.ToUpper(line), nil, ""
	}
	head, value := line[:colon], line[colon+1:]
	parts := strings.Split(head, ";")
	name = strings.ToUpper(parts[0])
	if len(parts) > 1 {
		params = map[string]string{}
		for _, p := range parts[1:] {
			if eq := strings.IndexByte(p, '='); eq > 0 {
				params[strings.ToUpper(p[:eq])] = strings.Trim(p[eq+1:], `"`)
			}
		}
	}
	return name, params, value
}

// unescape reverses RFC 5545 TEXT escaping.
func unescape(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return r.Replace(s)
}

func buildEvent(props map[string]property, defaultTZ *time.Location) (*Event, bool, []string) {
	var warnings []string
	summaryProp, ok := props["SUMMARY"]
	summary := ""
	if ok {
		summary = unescape(summaryProp.value)
	}

	startProp, ok := props["DTSTART"]
	if !ok {
		return nil, false, []string{fmt.Sprintf("skipped event %q: no DTSTART", summary)}
	}
	start, allDay, floating, err := parseDateTime(startProp, defaultTZ)
	if err != nil {
		return nil, false, []string{fmt.Sprintf("skipped event %q: %v", summary, err)}
	}

	ev := &Event{Summary: summary, Start: start, AllDay: allDay}
	if loc, ok := props["LOCATION"]; ok {
		ev.Location = unescape(loc.value)
	}
	if endProp, ok := props["DTEND"]; ok {
		if end, _, _, err := parseDateTime(endProp, defaultTZ); err == nil {
			ev.End = end
		} else {
			warnings = append(warnings, fmt.Sprintf("event %q: bad DTEND ignored (%v)", summary, err))
		}
	}
	if rr, ok := props["RRULE"]; ok {
		rule, warn := parseRRule(rr.value, start.Location())
		if warn != "" {
			warnings = append(warnings, fmt.Sprintf("event %q: %s", summary, warn))
		}
		ev.RRule = rule // nil when the rule was unsupported → treated as one-off
	}
	return ev, floating, warnings
}

// parseDateTime handles the three shapes DTSTART/DTEND come in:
// 20260907 (all-day), 20260907T101000Z (UTC), 20260907T101000 (+optional TZID).
// parseDateTime handles the three shapes DTSTART/DTEND come in and reports
// whether the value was FLOATING — no Z, no TZID, so its meaning is entirely
// decided by defaultTZ.
//
// Floating is the interesting case and the common one in university exports:
// "09:00" with nothing else said. Which 09:00 depends on who is asking, and
// getting it wrong shifts a whole timetable by the offset between two cities.
func parseDateTime(p property, defaultTZ *time.Location) (t time.Time, allDay, floating bool, err error) {
	v := strings.TrimSpace(p.value)
	if p.params["VALUE"] == "DATE" || len(v) == 8 {
		t, err = time.Parse("20060102", v)
		return t, true, false, err
	}
	if strings.HasSuffix(v, "Z") {
		t, err = time.Parse("20060102T150405Z", v)
		return t, false, false, err
	}
	loc, isFloating := defaultTZ, true
	if tzid := p.params["TZID"]; tzid != "" {
		if l, lerr := time.LoadLocation(tzid); lerr == nil {
			loc, isFloating = l, false
		}
	}
	t, err = time.ParseInLocation("20060102T150405", v, loc)
	return t, false, isFloating, err
}

var icsWeekdays = map[string]int{
	"SU": 0, "MO": 1, "TU": 2, "WE": 3, "TH": 4, "FR": 5, "SA": 6,
}

// parseRRule reads the supported subset; unsupported rules return (nil, warning)
// so the event downgrades to a single occurrence.
func parseRRule(value string, loc *time.Location) (*RRule, string) {
	r := &RRule{Interval: 1}
	for _, part := range strings.Split(value, ";") {
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		key, val := strings.ToUpper(part[:eq]), part[eq+1:]
		switch key {
		case "FREQ":
			r.Freq = strings.ToUpper(val)
		case "INTERVAL":
			if n, err := strconv.Atoi(val); err == nil && n >= 1 {
				r.Interval = n
			}
		case "BYDAY":
			for _, d := range strings.Split(val, ",") {
				d = strings.ToUpper(strings.TrimSpace(d))
				// Ordinal prefixes like 2MO/-1FR are beyond the subset.
				if len(d) != 2 {
					return nil, fmt.Sprintf("unsupported BYDAY %q — imported as a one-off", d)
				}
				wd, ok := icsWeekdays[d]
				if !ok {
					return nil, fmt.Sprintf("unsupported BYDAY %q — imported as a one-off", d)
				}
				r.ByDay = append(r.ByDay, wd)
			}
		case "UNTIL":
			var t time.Time
			var err error
			switch {
			case len(val) == 8:
				t, err = time.ParseInLocation("20060102", val, loc)
			case strings.HasSuffix(val, "Z"):
				t, err = time.Parse("20060102T150405Z", val)
			default:
				t, err = time.ParseInLocation("20060102T150405", val, loc)
			}
			if err == nil {
				r.Until = &t
			}
		case "COUNT":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				r.Count = n
			}
		}
	}
	switch r.Freq {
	case "DAILY", "WEEKLY", "MONTHLY":
		return r, ""
	default:
		return nil, fmt.Sprintf("unsupported RRULE FREQ=%s — imported as a one-off", r.Freq)
	}
}
