// Package timeutil provides time conversion utilities.
package timeutil

import (
	"fmt"
	"time"
)

// ToUTC parses a date string and a time string in the given IANA timezone
// (e.g. "Asia/Shanghai", "America/New_York"), interprets them as a wall-clock
// instant in that zone, and returns the equivalent UTC time formatted as RFC 3339.
//
// Expected formats:
//   - date:  "2006-01-02"
//   - timeStr: "15:04:05" or "15:04"
func ToUTC(date, timeStr, tz string) (string, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("timeutil.ToUTC: invalid timezone %q: %w", tz, err)
	}

	// Accept both "15:04:05" and "15:04".
	layout := "2006-01-02 15:04:05"
	combined := date + " " + timeStr
	if len(timeStr) == 5 {
		layout = "2006-01-02 15:04"
	}

	t, err := time.ParseInLocation(layout, combined, loc)
	if err != nil {
		return "", fmt.Errorf("timeutil.ToUTC: cannot parse date=%q time=%q: %w", date, timeStr, err)
	}

	return t.UTC().Format(time.RFC3339), nil
}

// FromUTC converts a UTC RFC 3339 timestamp into a wall-clock date and time
// in the target IANA timezone (e.g. "Asia/Shanghai", "America/New_York").
//
// Returned formats:
//   - date: "2006-01-02"
//   - timeStr: "15:04:05"
func FromUTC(utcRFC3339, targetTZ string) (date, timeStr string, err error) {
	t, err := time.Parse(time.RFC3339, utcRFC3339)
	if err != nil {
		return "", "", fmt.Errorf("timeutil.FromUTC: invalid RFC 3339 %q: %w", utcRFC3339, err)
	}

	loc, err := time.LoadLocation(targetTZ)
	if err != nil {
		return "", "", fmt.Errorf("timeutil.FromUTC: invalid timezone %q: %w", targetTZ, err)
	}

	local := t.In(loc)
	date = local.Format("2006-01-02")
	timeStr = local.Format("15:04:05")
	return date, timeStr, nil
}
