package server

import (
	"testing"

	"daycore/internal/domain"
)

func TestFillBlockUTC(t *testing.T) {
	tm := "09:00"

	// fixed block is anchored to a UTC instant (Shanghai UTC+8 → 01:00Z).
	fixed := domain.TimeBlock{Time: &tm, Date: "2026-06-15", TimeMode: domain.TimeFixed, Timezone: "Asia/Shanghai"}
	fillBlockUTC(&fixed, "", "")
	if fixed.UTCTime == nil || *fixed.UTCTime != "2026-06-15T01:00:00Z" {
		t.Fatalf("fixed anchor: %v", fixed.UTCTime)
	}

	// floating block follows the wall clock and carries no anchor.
	floating := domain.TimeBlock{Time: &tm, Date: "2026-06-15", TimeMode: domain.TimeFloating, Timezone: "Asia/Shanghai"}
	fillBlockUTC(&floating, "", "")
	if floating.UTCTime != nil {
		t.Fatalf("floating should have no anchor, got %v", *floating.UTCTime)
	}

	// planDate is used as the date fallback when the block sets none.
	noDate := domain.TimeBlock{Time: &tm, TimeMode: domain.TimeLocal, Timezone: "UTC"}
	fillBlockUTC(&noDate, "2026-06-15", "")
	if noDate.UTCTime == nil || *noDate.UTCTime != "2026-06-15T09:00:00Z" {
		t.Fatalf("planDate fallback: %v", noDate.UTCTime)
	}
}
