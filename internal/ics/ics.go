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

// Parse reads an .ics payload. Per-event problems become warnings; only a
// payload with no VCALENDAR structure at all errors.
func Parse(data string) ([]Event, []string, error) {
	lines := unfold(data)
	if len(lines) == 0 {
		return nil, nil, fmt.Errorf("empty ics payload")
	}

	var (
		events   []Event
		warnings []string
		cur      map[string]property
		inEvent  bool
		sawCal   bool
	)
	for _, line := range lines {
		name, params, value := splitLine(line)
		switch {
		case name == "BEGIN" && strings.EqualFold(value, "VCALENDAR"):
			sawCal = true
		case name == "BEGIN" && strings.EqualFold(value, "VEVENT"):
			inEvent = true
			cur = map[string]property{}
		case name == "END" && strings.EqualFold(value, "VEVENT"):
			if inEvent {
				ev, warns := buildEvent(cur)
				warnings = append(warnings, warns...)
				if ev != nil {
					events = append(events, *ev)
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
		return nil, nil, fmt.Errorf("not an iCalendar file (no BEGIN:VCALENDAR)")
	}
	return events, warnings, nil
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

func buildEvent(props map[string]property) (*Event, []string) {
	var warnings []string
	summaryProp, ok := props["SUMMARY"]
	summary := ""
	if ok {
		summary = unescape(summaryProp.value)
	}

	startProp, ok := props["DTSTART"]
	if !ok {
		return nil, []string{fmt.Sprintf("skipped event %q: no DTSTART", summary)}
	}
	start, allDay, err := parseDateTime(startProp)
	if err != nil {
		return nil, []string{fmt.Sprintf("skipped event %q: %v", summary, err)}
	}

	ev := &Event{Summary: summary, Start: start, AllDay: allDay}
	if loc, ok := props["LOCATION"]; ok {
		ev.Location = unescape(loc.value)
	}
	if endProp, ok := props["DTEND"]; ok {
		if end, _, err := parseDateTime(endProp); err == nil {
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
	return ev, warnings
}

// parseDateTime handles the three shapes DTSTART/DTEND come in:
// 20260907 (all-day), 20260907T101000Z (UTC), 20260907T101000 (+optional TZID).
func parseDateTime(p property) (time.Time, bool, error) {
	v := strings.TrimSpace(p.value)
	if p.params["VALUE"] == "DATE" || len(v) == 8 {
		t, err := time.Parse("20060102", v)
		return t, true, err
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return t, false, err
	}
	loc := time.Local
	if tzid := p.params["TZID"]; tzid != "" {
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		}
	}
	t, err := time.ParseInLocation("20060102T150405", v, loc)
	return t, false, err
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
