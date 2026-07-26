package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"daycore/internal/timeutil"
)

// petrifyVectors mirrors api/testdata/petrify-vectors.json. The Go side reads
// the shared fixture rather than restating the cases, so the rule the frontends
// follow and the rule the server enforces cannot drift apart silently.
type petrifyVectors struct {
	HorizonHours float64 `json:"horizonHours"`
	Scenarios    []struct {
		Name         string `json:"name"`
		Now          string `json:"now"`
		TZ           string `json:"tz"`
		ExpectedLine string `json:"expectedLine"`
		Cases        []struct {
			Date  string `json:"date"`
			Time  string `json:"time"`
			Dur   int    `json:"dur"`
			Phase string `json:"phase"`
			Why   string `json:"why"`
		} `json:"cases"`
	} `json:"scenarios"`
	DST struct {
		TZ            string `json:"tz"`
		SpringForward dstDay `json:"springForward"`
		FallBack      dstDay `json:"fallBack"`
		StartOfDay    []struct {
			Date string `json:"date"`
			Is   string `json:"is"`
			Why  string `json:"why"`
		} `json:"startOfDay"`
	} `json:"dst"`
}

type dstDay struct {
	Date  string `json:"date"`
	Note  string `json:"note"`
	Cases []struct {
		Time       string `json:"time"`
		ResolvesTo string `json:"resolvesTo"`
		Zone       string `json:"zone"`
	} `json:"cases"`
}

func loadPetrifyVectors(t *testing.T) petrifyVectors {
	t.Helper()
	path := filepath.Join("..", "..", "api", "testdata", "petrify-vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v petrifyVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return v
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad timestamp %q in fixture: %v", s, err)
	}
	return ts
}

// The whole shared table: line placement plus every phase classification.
func TestPetrifyVectors(t *testing.T) {
	v := loadPetrifyVectors(t)
	horizon := time.Duration(v.HorizonHours * float64(time.Hour))
	if horizon != timeutil.PetrifyHorizonDefault {
		t.Fatalf("fixture horizon %v differs from PetrifyHorizonDefault %v", horizon, timeutil.PetrifyHorizonDefault)
	}

	for _, sc := range v.Scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			loc, err := time.LoadLocation(sc.TZ)
			if err != nil {
				t.Fatalf("tz %q: %v", sc.TZ, err)
			}
			now := mustTime(t, sc.Now)

			gotLine := timeutil.PetrifyLine(now, loc, horizon)
			wantLine := mustTime(t, sc.ExpectedLine)
			if !gotLine.Equal(wantLine) {
				t.Errorf("PetrifyLine = %s, fixture says %s",
					gotLine.Format(time.RFC3339), wantLine.Format(time.RFC3339))
			}

			for _, c := range sc.Cases {
				dur := c.Dur
				hm := c.Time
				b := TimeBlock{Date: c.Date, Time: &hm, DurationMin: &dur}
				if got := b.PhaseIn("", now, loc, horizon); string(got) != c.Phase {
					start, end, _ := b.Span("", loc)
					t.Errorf("%s %s+%dmin: phase = %q, want %q — %s\n    span %s → %s, line %s",
						c.Date, c.Time, c.Dur, got, c.Phase, c.Why,
						start.Format(time.RFC3339), end.Format(time.RFC3339), gotLine.Format(time.RFC3339))
				}
			}
		})
	}
}

// DST behaviour is Go's, not ours, and the fixture records what was measured.
// If a Go upgrade changes how a gap or an ambiguous hour resolves, this fails
// here rather than showing up as a block that froze on the server and not on
// the client.
func TestPetrifyVectorsDST(t *testing.T) {
	v := loadPetrifyVectors(t)
	loc, err := time.LoadLocation(v.DST.TZ)
	if err != nil {
		t.Fatalf("tz %q: %v", v.DST.TZ, err)
	}

	for _, day := range []dstDay{v.DST.SpringForward, v.DST.FallBack} {
		for _, c := range day.Cases {
			got, err := timeutil.ResolveWall(day.Date, c.Time, loc)
			if err != nil {
				t.Errorf("ResolveWall(%s %s): %v", day.Date, c.Time, err)
				continue
			}
			want := mustTime(t, c.ResolvesTo)
			if !got.Equal(want) {
				t.Errorf("%s %s → %s, fixture says %s (%s)",
					day.Date, c.Time, got.Format(time.RFC3339), c.ResolvesTo, day.Note)
			}
			if zone, _ := got.Zone(); zone != c.Zone {
				t.Errorf("%s %s resolved into zone %s, fixture says %s", day.Date, c.Time, zone, c.Zone)
			}
		}
	}

	for _, s := range v.DST.StartOfDay {
		noon, err := timeutil.ResolveWall(s.Date, "12:00", loc)
		if err != nil {
			t.Fatalf("ResolveWall: %v", err)
		}
		got := timeutil.StartOfDay(noon, loc)
		if want := mustTime(t, s.Is); !got.Equal(want) {
			t.Errorf("StartOfDay(%s) = %s, fixture says %s — %s",
				s.Date, got.Format(time.RFC3339), s.Is, s.Why)
		}
	}
}

// A block with no clock position never freezes — there is no past to lock.
func TestPhaseUnscheduled(t *testing.T) {
	now := time.Now()
	if got := (TimeBlock{Date: "2020-01-01"}).PhaseIn("", now, time.UTC, 0); got != PhaseFuture {
		t.Errorf("a block with no time should be future, got %q", got)
	}
	empty := ""
	if got := (TimeBlock{Date: "2020-01-01", Time: &empty}).PhaseIn("", now, time.UTC, 0); got != PhaseFuture {
		t.Errorf("empty time should be future, got %q", got)
	}
	// No date anywhere — not even from the enclosing plan.
	hm := "09:00"
	if got := (TimeBlock{Time: &hm}).PhaseIn("", now, time.UTC, 0); got != PhaseFuture {
		t.Errorf("a block with no date should be future, got %q", got)
	}
}

// A block inherits its date from the enclosing DayPlan when it carries none.
func TestPhaseInheritsPlanDate(t *testing.T) {
	loc := time.UTC
	now := mustTime(t, "2026-07-26T15:00:00Z")
	hm, dur := "09:00", 60
	b := TimeBlock{Time: &hm, DurationMin: &dur} // no Date
	if got := b.PhaseIn("2026-07-26", now, loc, timeutil.PetrifyHorizonDefault); got != PhaseRecon {
		t.Errorf("plan date should have been used, got %q", got)
	}
}

// An absolute anchor wins over the wall clock: a fixed block means the same
// instant no matter which zone the reader is in.
func TestPhaseUsesUTCAnchor(t *testing.T) {
	now := mustTime(t, "2026-07-26T15:00:00Z")
	hm, dur := "09:00", 60
	anchor := "2026-07-26T20:00:00Z" // wall clock says morning, anchor says evening
	b := TimeBlock{
		Date: "2026-07-26", Time: &hm, DurationMin: &dur,
		TimeMode: TimeFixed, UTCTime: &anchor,
	}
	if got := b.PhaseIn("", now, time.UTC, timeutil.PetrifyHorizonDefault); got != PhaseFuture {
		t.Errorf("the UTC anchor should decide, got %q", got)
	}

	// A malformed anchor must not make the block un-phaseable; fall back to
	// the wall clock rather than silently reporting future.
	bad := "not-a-timestamp"
	b.UTCTime = &bad
	if got := b.PhaseIn("", now, time.UTC, timeutil.PetrifyHorizonDefault); got != PhaseRecon {
		t.Errorf("a broken anchor should fall back to the wall clock, got %q", got)
	}
}

// Frozen is the one predicate the write paths gate on.
func TestFrozen(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := mustTime(t, "2026-07-26T15:00:00+08:00")
	h := timeutil.PetrifyHorizonDefault
	dur := 60

	yesterday := "22:00"
	old := TimeBlock{Date: "2026-07-25", Time: &yesterday, DurationMin: &dur}
	if !old.Frozen("", now, loc, h) {
		t.Error("yesterday evening should be frozen at 3pm today")
	}

	thisMorning := "09:00"
	fresh := TimeBlock{Date: "2026-07-26", Time: &thisMorning, DurationMin: &dur}
	if fresh.Frozen("", now, loc, h) {
		t.Error("this morning must stay editable — today never freezes while you live it")
	}
}
