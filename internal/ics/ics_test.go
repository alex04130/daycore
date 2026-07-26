package ics

import (
	"strings"
	"testing"
)

const sample = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Google Inc//Google Calendar 70.9054//EN\r\n" +
	"BEGIN:VEVENT\r\n" +
	"DTSTART;TZID=America/Chicago:20260908T101000\r\n" +
	"DTEND;TZID=America/Chicago:20260908T111500\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=TU,TH;UNTIL=20261215T000000Z\r\n" +
	"SUMMARY:CSCI 3081 Software Design\\, Sec 010\r\n" +
	"LOCATION:Keller Hall\r\n" +
	" 3-210\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"DTSTART:20260910T140000Z\r\n" +
	"DTEND:20260910T150000Z\r\n" +
	"SUMMARY:Advisor meeting\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"DTSTART;VALUE=DATE:20261001\r\n" +
	"SUMMARY:Career fair\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"DTSTART:20260901T090000Z\r\n" +
	"RRULE:FREQ=YEARLY\r\n" +
	"SUMMARY:Anniversary\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParse(t *testing.T) {
	events, warnings, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}

	// Weekly course with folded LOCATION and escaped comma.
	c := events[0]
	if c.Summary != "CSCI 3081 Software Design, Sec 010" {
		t.Fatalf("summary = %q", c.Summary)
	}
	if c.Location != "Keller Hall3-210" {
		t.Fatalf("location = %q", c.Location)
	}
	if c.RRule == nil || c.RRule.Freq != "WEEKLY" {
		t.Fatalf("rrule = %+v", c.RRule)
	}
	if len(c.RRule.ByDay) != 2 || c.RRule.ByDay[0] != 2 || c.RRule.ByDay[1] != 4 {
		t.Fatalf("byday = %v (want [2 4])", c.RRule.ByDay)
	}
	if c.RRule.Until == nil {
		t.Fatal("until missing")
	}
	if got := c.Start.Format("15:04"); got != "10:10" {
		t.Fatalf("start local time = %s", got)
	}
	if min := int(c.End.Sub(c.Start).Minutes()); min != 65 {
		t.Fatalf("duration = %d", min)
	}

	// One-off UTC event.
	if events[1].RRule != nil || events[1].AllDay {
		t.Fatalf("advisor meeting: %+v", events[1])
	}

	// All-day event.
	if !events[2].AllDay {
		t.Fatalf("career fair should be all-day")
	}

	// YEARLY downgrades to one-off with a warning.
	if events[3].RRule != nil {
		t.Fatalf("yearly must downgrade to one-off")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "FREQ=YEARLY") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a YEARLY warning, got %v", warnings)
	}
}

func TestParseRejectsNonICS(t *testing.T) {
	if _, _, err := Parse("hello world"); err == nil {
		t.Fatal("expected error for non-ics payload")
	}
}

func TestParseEventWithoutDTSTARTIsSkipped(t *testing.T) {
	payload := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:broken\nEND:VEVENT\nEND:VCALENDAR"
	events, warnings, err := Parse(payload)
	if err != nil || len(events) != 0 || len(warnings) != 1 {
		t.Fatalf("events=%d warnings=%v err=%v", len(events), warnings, err)
	}
}
