package ai

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"daycore/internal/i18n"
)

// The relative-date vocabulary lives in the message catalog, flat, one key per
// string — a translator edits a JSON file, not a Go struct. The keys are:
//
//	date.tomorrow  date.dayAfter  date.dayAfter3
//	date.thisWeek  date.nextWeek
//	date.weekday.0 … .6          full names, indexed by time.Weekday (Sunday = 0)
//	date.weekdayShort.0 … .6     the form used inside 本周三 / "This Monday"
//	date.row                     one table row: term, date, weekday
//	date.rel                     week qualifier joined to a short weekday
//
// weekday indices follow time.Weekday rather than the locale's first day of the
// week; which day a calendar starts on is nextWeekdayDate's business.
func init() {
	i18n.Register("date.tomorrow", i18n.Text{"zh-CN": "明天", "en-US": "Tomorrow"})
	i18n.Register("date.dayAfter", i18n.Text{"zh-CN": "后天", "en-US": "The day after"})
	i18n.Register("date.dayAfter3", i18n.Text{"zh-CN": "大后天", "en-US": "3 days later"})
	i18n.Register("date.thisWeek", i18n.Text{"zh-CN": "本周", "en-US": "This"})
	i18n.Register("date.nextWeek", i18n.Text{"zh-CN": "下周", "en-US": "Next"})

	full := [7]i18n.Text{
		{"zh-CN": "星期日", "en-US": "Sunday"},
		{"zh-CN": "星期一", "en-US": "Monday"},
		{"zh-CN": "星期二", "en-US": "Tuesday"},
		{"zh-CN": "星期三", "en-US": "Wednesday"},
		{"zh-CN": "星期四", "en-US": "Thursday"},
		{"zh-CN": "星期五", "en-US": "Friday"},
		{"zh-CN": "星期六", "en-US": "Saturday"},
	}
	// The short form used inside a relative phrase. Chinese drops the 星期
	// prefix (下周三); English has nothing to drop. Storing the短 forms as
	// their own keys rather than deriving them with a per-locale function is
	// what lets a translator add a language without writing Go.
	short := [7]i18n.Text{
		{"zh-CN": "日", "en-US": "Sunday"},
		{"zh-CN": "一", "en-US": "Monday"},
		{"zh-CN": "二", "en-US": "Tuesday"},
		{"zh-CN": "三", "en-US": "Wednesday"},
		{"zh-CN": "四", "en-US": "Thursday"},
		{"zh-CN": "五", "en-US": "Friday"},
		{"zh-CN": "六", "en-US": "Saturday"},
	}
	for i := 0; i < 7; i++ {
		i18n.Register(weekdayKey(i), full[i])
		i18n.Register(weekdayShortKey(i), short[i])
	}

	// Both carry format verbs. date.row is term, date, weekday — the brackets
	// are part of the translation, since full-width （） belongs in a Chinese
	// table and reads as a typo in an English one. date.rel joins a qualifier
	// to a short weekday: 下周 + 三 runs together, "Next" + "Monday" does not.
	i18n.Register("date.row", i18n.Text{"zh-CN": "| %s | %s（%s） |", "en-US": "| %s | %s (%s) |"})
	i18n.Register("date.rel", i18n.Text{"zh-CN": "%s%s", "en-US": "%s %s"})
}

func weekdayKey(i int) string      { return "date.weekday." + strconv.Itoa(i) }
func weekdayShortKey(i int) string { return "date.weekdayShort." + strconv.Itoa(i) }

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
	rowFmt := i18n.T("date.row", locale)
	relFmt := i18n.T("date.rel", locale)
	weekday := func(d time.Weekday) string { return i18n.T(weekdayKey(int(d)), locale) }

	tomorrow := base.AddDate(0, 0, 1)
	dayAfter := base.AddDate(0, 0, 2)
	dayAfter3 := base.AddDate(0, 0, 3)

	var rows []string
	rows = append(rows,
		fmt.Sprintf(rowFmt, i18n.T("date.tomorrow", locale), fmtDate(tomorrow), weekday(tomorrow.Weekday())),
		fmt.Sprintf(rowFmt, i18n.T("date.dayAfter", locale), fmtDate(dayAfter), weekday(dayAfter.Weekday())),
		fmt.Sprintf(rowFmt, i18n.T("date.dayAfter3", locale), fmtDate(dayAfter3), weekday(dayAfter3.Weekday())),
	)
	for dow := 0; dow < 7; dow++ {
		label := i18n.T(weekdayKey(dow), locale)
		short := i18n.T(weekdayShortKey(dow), locale)
		rows = append(rows,
			fmt.Sprintf(rowFmt, fmt.Sprintf(relFmt, i18n.T("date.thisWeek", locale), short),
				nextWeekdayDate(base, dow, false), label),
			fmt.Sprintf(rowFmt, fmt.Sprintf(relFmt, i18n.T("date.nextWeek", locale), short),
				nextWeekdayDate(base, dow, true), label),
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
	return i18n.T(weekdayKey(int(t.Weekday())), locale)
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
