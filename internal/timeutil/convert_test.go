package timeutil

import "testing"

func TestToUTCFromUTCRoundtrip(t *testing.T) {
	// Shanghai is UTC+8 (no DST), so 09:00 local → 01:00 UTC.
	utc, err := ToUTC("2026-06-15", "09:00", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if utc != "2026-06-15T01:00:00Z" {
		t.Fatalf("ToUTC got %s", utc)
	}
	date, tm, err := FromUTC(utc, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if date != "2026-06-15" || tm != "09:00:00" {
		t.Fatalf("roundtrip got %s %s", date, tm)
	}
}

// The same wall-clock time maps to different UTC instants across a DST boundary:
// US Eastern is EDT (UTC-4) in summer and EST (UTC-5) in winter.
func TestToUTCHandlesDST(t *testing.T) {
	summer, err := ToUTC("2026-07-01", "12:00", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if summer != "2026-07-01T16:00:00Z" {
		t.Fatalf("EDT: got %s want 16:00Z", summer)
	}
	winter, err := ToUTC("2026-01-01", "12:00", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if winter != "2026-01-01T17:00:00Z" {
		t.Fatalf("EST: got %s want 17:00Z", winter)
	}
}

func TestToUTCRejectsBadTimezone(t *testing.T) {
	if _, err := ToUTC("2026-01-01", "12:00", "Mars/Phobos"); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}
