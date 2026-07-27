package ai

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"daycore/internal/i18n"
)

// dateStrs carries the locale-dependent labels used by BuildDateContext.
//
// weekdays is indexed by time.Weekday, so Sunday is index 0 regardless of which
// day the locale considers the start of the week — that is a rendering
// question, and nextWeekdayDate handles it separately.
type dateStrs struct {
	tomorrow, dayAfter, dayAfter3 string // "明天"/"Tomorrow", etc.
	thisWeek, nextWeek            string // "本周"/"This", "下周"/"Next"
	weekdays                      []string
	// shortFn trims a weekday down to the form used inside a relative phrase.
	// Chinese drops the 星期 prefix ("星期三" → "三", giving 下周三); English has
	// nothing to drop. A new locale supplies its own rule rather than having
	// one guessed from the script.
	shortFn func(w string) string
	// rowFmt renders one row of the relative-date table: term, date, weekday.
	// The brackets are part of the translation — full-width （） belongs in a
	// Chinese table and reads as a typo in an English one.
	rowFmt string
	// relFmt joins a week qualifier to a short weekday: 下周 + 三 runs together,
	// "Next" + "Monday" needs a space between them.
	relFmt string
}

var dateTables = map[string]dateStrs{
	"zh-CN": {
		tomorrow: "明天", dayAfter: "后天", dayAfter3: "大后天",
		thisWeek: "本周", nextWeek: "下周",
		weekdays: []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"},
		shortFn:  func(w string) string { return strings.TrimPrefix(w, "星期") },
		rowFmt:   "| %s | %s（%s） |",
		relFmt:   "%s%s",
	},
	"en-US": {
		tomorrow: "Tomorrow", dayAfter: "The day after", dayAfter3: "3 days later",
		thisWeek: "This", nextWeek: "Next",
		weekdays: []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		shortFn:  func(w string) string { return w },
		rowFmt:   "| %s | %s (%s) |",
		relFmt:   "%s %s",
	},
}

func dateStringTable(locale string) dateStrs {
	t, ok := i18n.PickFrom(dateTables, locale)
	if !ok {
		return dateTables[i18n.Default]
	}
	return t
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
		fmt.Sprintf(lbl.rowFmt, lbl.tomorrow, fmtDate(tomorrow), lbl.weekdays[tomorrow.Weekday()]),
		fmt.Sprintf(lbl.rowFmt, lbl.dayAfter, fmtDate(dayAfter), lbl.weekdays[dayAfter.Weekday()]),
		fmt.Sprintf(lbl.rowFmt, lbl.dayAfter3, fmtDate(dayAfter3), lbl.weekdays[dayAfter3.Weekday()]),
	)
	for dow := 0; dow < 7; dow++ {
		label := lbl.weekdays[dow]
		short := lbl.shortFn(label)
		thisWeek := nextWeekdayDate(base, dow, false)
		nextWeek := nextWeekdayDate(base, dow, true)
		rows = append(rows,
			fmt.Sprintf(lbl.rowFmt, fmt.Sprintf(lbl.relFmt, lbl.thisWeek, short), thisWeek, label),
			fmt.Sprintf(lbl.rowFmt, fmt.Sprintf(lbl.relFmt, lbl.nextWeek, short), nextWeek, label),
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
	return dateStringTable(locale).weekdays[t.Weekday()]
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
