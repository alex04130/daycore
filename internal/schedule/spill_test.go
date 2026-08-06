package schedule

import (
	"testing"
	"time"

	"daycore/internal/domain"
)

func hm(v string) *string { return &v }
func mn(v int) *int       { return &v }

// Consensus 25: one row of data, rendered and counted on both days. The block
// that ends at 1 a.m. belongs to the evening it continued and to the day it
// landed in; picking one would either hide it from the day the user remembers
// or move it off the day it started.
func TestSpillsInto(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		name string
		b    domain.TimeBlock
		want bool
	}{
		{"23:00 起两小时，越过午夜", domain.TimeBlock{Time: hm("23:00"), DurationMin: mn(120)}, true},
		{"23:00 起一小时，正好停在午夜 —— 不算跨天", domain.TimeBlock{Time: hm("23:00"), DurationMin: mn(60)}, false},
		{"白天的块", domain.TimeBlock{Time: hm("09:00"), DurationMin: mn(60)}, false},
		{"没有时长", domain.TimeBlock{Time: hm("23:00")}, false},
		{"不定时的块无相位可言", domain.TimeBlock{DurationMin: mn(120)}, false},
		{"墓碑不外溢 —— 用户已经说了这次不算", domain.TimeBlock{Time: hm("23:00"), DurationMin: mn(120), Hidden: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SpillsInto(c.b, "2026-03-10", loc); got != c.want {
				t.Errorf("SpillsInto = %v, want %v", got, c.want)
			}
		})
	}
}

// The next-day boundary is the one that broke on zones shifting at midnight,
// so the spill rule is exercised there too: on 2026-03-08 in America/Havana the
// day begins at 01:00, so a block ending at 00:30 has NOT crossed into it.
func TestSpillsIntoAcrossAMidnightGap(t *testing.T) {
	loc, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	// The 8th begins at 01:00 CDT because 00:00 never happens, and 23:00 CST is
	// exactly sixty minutes of real time before it. So sixty minutes lands ON
	// the boundary and does not cross it; sixty-one does.
	//
	// An earlier draft of this test asserted that ninety minutes stays inside
	// the 7th, reasoning "23:00 + 90m = 00:30, which is before 01:00". That is
	// wall-clock arithmetic across a gap that does not exist: ninety minutes of
	// elapsed time lands at 01:30 CDT, well inside the 8th. The implementation
	// was right and the test was wrong — which is the entire reason the day
	// boundary lives in one tested function instead of in each caller's head.
	exact := domain.TimeBlock{Time: hm("23:00"), DurationMin: mn(60)}
	if SpillsInto(exact, "2026-03-07", loc) {
		t.Error("a block ending exactly when the next day begins has not crossed into it")
	}
	over := domain.TimeBlock{Time: hm("23:00"), DurationMin: mn(61)}
	if !SpillsInto(over, "2026-03-07", loc) {
		t.Error("one minute past the boundary is across it")
	}
}

// A spilled block is a VIEW of yesterday's row. It must carry its own date, or
// the day that displays it will resolve its time against the wrong day and move
// it twenty-four hours.
func TestSpillInsCarryTheirOwnDate(t *testing.T) {
	loc := time.UTC
	prev := []domain.TimeBlock{
		{ID: "a", Title: "赶论文", Time: hm("23:00"), DurationMin: mn(180)},
		{ID: "b", Title: "睡前读书", Time: hm("22:00"), DurationMin: mn(30)},
	}
	got := SpillIns(prev, "2026-03-10", loc)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("spill-ins = %+v, want just the one that crosses", got)
	}
	if got[0].Date != "2026-03-10" {
		t.Errorf("date = %q, want the day it belongs to", got[0].Date)
	}
	// The source must not have been touched: one row of data.
	if prev[0].Date != "" {
		t.Errorf("SpillIns mutated the block it was reading: %q", prev[0].Date)
	}
}
