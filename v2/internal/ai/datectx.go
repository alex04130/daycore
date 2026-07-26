package ai

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var weekdaysCN = []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
var weekdaysEN = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// dateStrs carries the locale-dependent labels used by BuildDateContext.
type dateStrs struct {
	tomorrow, dayAfter, dayAfter3 string // "明天"/"Tomorrow", etc.
	thisWeek, nextWeek            string // "本周"/"This", "下周"/"Next"
	weekdays                      []string
	shortFn                       func(w string) string
}

func dateStringTable(locale string) dateStrs {
	if strings.HasPrefix(locale, "zh") {
		return dateStrs{
			tomorrow:  "明天", dayAfter: "后天", dayAfter3: "大后天",
			thisWeek: "本周", nextWeek: "下周", weekdays: weekdaysCN,
			shortFn: func(w string) string { return strings.TrimPrefix(w, "星期") },
		}
	}
	return dateStrs{
		tomorrow:  "Tomorrow", dayAfter: "The day after", dayAfter3: "3 days later",
		thisWeek: "This", nextWeek: "Next", weekdays: weekdaysEN,
		shortFn: func(w string) string { return w },
	}
}

// DateContext is the fully-resolved date information injected into the planning
// prompts. All relative date terms are pre-computed server-side so the model
// never has to do calendar arithmetic (ported from v1 date-context.ts).
type DateContext struct {
	Date            string // YYYY-MM-DD the user is viewing/editing
	Weekday         string
	Time            string
	Timezone        string
	TargetDate      string // the day being planned (may differ from Date)
	TargetWeekday   string
	RelativeDateMap string // a ready-to-inject markdown table of relative terms
}

// BuildDateContext computes the relative-date table from the client's local date.
func BuildDateContext(clientDate, clientWeekday, clientTime, clientTimezone, locale string) DateContext {
	base := parseLocalDate(clientDate)
	lbl := dateStringTable(locale)

	tomorrow := base.AddDate(0, 0, 1)
	dayAfter := base.AddDate(0, 0, 2)
	dayAfter3 := base.AddDate(0, 0, 3)

	var rows []string
	rows = append(rows,
		fmt.Sprintf("| %s | %s（%s） |", lbl.tomorrow, fmtDate(tomorrow), lbl.weekdays[tomorrow.Weekday()]),
		fmt.Sprintf("| %s | %s（%s） |", lbl.dayAfter, fmtDate(dayAfter), lbl.weekdays[dayAfter.Weekday()]),
		fmt.Sprintf("| %s | %s（%s） |", lbl.dayAfter3, fmtDate(dayAfter3), lbl.weekdays[dayAfter3.Weekday()]),
	)
	for dow := 0; dow < 7; dow++ {
		label := lbl.weekdays[dow]
		short := lbl.shortFn(label)
		thisWeek := nextWeekdayDate(base, dow, false)
		nextWeek := nextWeekdayDate(base, dow, true)
		rows = append(rows,
			fmt.Sprintf("| %s%s | %s（%s） |", lbl.thisWeek, short, thisWeek, label),
			fmt.Sprintf("| %s%s | %s（%s） |", lbl.nextWeek, short, nextWeek, label),
		)
	}

	return DateContext{
		Date:            clientDate,
		Weekday:         clientWeekday,
		Time:            clientTime,
		Timezone:        clientTimezone,
		RelativeDateMap: strings.Join(rows, "\n"),
	}
}

func parseLocalDate(s string) time.Time {
	parts := strings.Split(s, "-")
	if len(parts) == 3 {
		y, e1 := strconv.Atoi(parts[0])
		m, e2 := strconv.Atoi(parts[1])
		d, e3 := strconv.Atoi(parts[2])
		if e1 == nil && e2 == nil && e3 == nil {
			// noon UTC avoids any DST/boundary surprises for pure date math
			return time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.UTC)
		}
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
}

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

// WeekdayName renders t's weekday for the locale ("星期日" / "Sunday").
func WeekdayName(t time.Time, locale string) string {
	if strings.HasPrefix(locale, "zh") {
		return weekdaysCN[t.Weekday()]
	}
	return t.Weekday().String()
}

// nextWeekdayDate resolves 本周X/下周X with Monday-start calendar weeks (the
// Chinese convention): 下周X is always next week's X, 本周X is this week's X —
// today counts as itself, and a weekday that already passed rolls forward to
// its next occurrence. The old v1 port used Sunday-relative offsets, which
// mis-resolved e.g. 下周日 said mid-week to *this* week's Sunday.
func nextWeekdayDate(base time.Time, targetDow int, nextWeek bool) string {
	baseIdx := (int(base.Weekday()) + 6) % 7 // Monday-start index
	targetIdx := (targetDow + 6) % 7
	diff := targetIdx - baseIdx
	if nextWeek {
		diff += 7
	} else if diff < 0 {
		diff += 7
	}
	return fmtDate(base.AddDate(0, 0, diff))
}
