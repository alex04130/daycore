package ai

import (
	"strings"
	"testing"
)

func TestBuildDateContextRelativeDates(t *testing.T) {
	dc := BuildDateContext("2026-06-21", "星期日", "10:00", "Asia/Shanghai", "zh-CN")

	if dc.Date != "2026-06-21" {
		t.Fatalf("Date = %q", dc.Date)
	}
	for _, want := range []string{
		"明天 | 2026-06-22",  // +1
		"后天 | 2026-06-23",  // +2
		"大后天 | 2026-06-24", // +3
	} {
		if !strings.Contains(dc.RelativeDateMap, want) {
			t.Errorf("RelativeDateMap missing %q\n--- map ---\n%s", want, dc.RelativeDateMap)
		}
	}
}

// Full-table regression for nextWeekdayDate's Monday-start semantics: 下周X is
// always next calendar week's X; 本周X is this week's X (today counts as
// itself, already-passed weekdays roll forward to the next occurrence).
func TestBuildDateContextFullTable(t *testing.T) {
	cases := []struct {
		name string
		base string
		want []string
	}{
		{
			// dow=0, Monday-start index 6: today IS 本周日; the current week ends
			// today, so 下周X (X=Mon..Sat) all land within the coming 6 days and
			// coincide with the rolled-forward 本周X.
			name: "sunday base",
			base: "2026-06-21",
			want: []string{
				"| 明天 | 2026-06-22（星期一） |",
				"| 后天 | 2026-06-23（星期二） |",
				"| 大后天 | 2026-06-24（星期三） |",
				"| 本周日 | 2026-06-21（星期日） |",
				"| 下周日 | 2026-06-28（星期日） |",
				"| 本周一 | 2026-06-22（星期一） |",
				"| 下周一 | 2026-06-22（星期一） |",
				"| 本周二 | 2026-06-23（星期二） |",
				"| 下周二 | 2026-06-23（星期二） |",
				"| 本周三 | 2026-06-24（星期三） |",
				"| 下周三 | 2026-06-24（星期三） |",
				"| 本周四 | 2026-06-25（星期四） |",
				"| 下周四 | 2026-06-25（星期四） |",
				"| 本周五 | 2026-06-26（星期五） |",
				"| 下周五 | 2026-06-26（星期五） |",
				"| 本周六 | 2026-06-27（星期六） |",
				"| 下周六 | 2026-06-27（星期六） |",
			},
		},
		{
			name: "month end friday", // 2026-07-31 is a Friday; everything crosses into August
			base: "2026-07-31",
			want: []string{
				"| 明天 | 2026-08-01（星期六） |",
				"| 后天 | 2026-08-02（星期日） |",
				"| 大后天 | 2026-08-03（星期一） |",
				"| 本周日 | 2026-08-02（星期日） |",
				"| 下周日 | 2026-08-09（星期日） |",
				"| 本周一 | 2026-08-03（星期一） |",
				"| 下周一 | 2026-08-03（星期一） |",
				"| 本周二 | 2026-08-04（星期二） |",
				"| 下周二 | 2026-08-04（星期二） |",
				"| 本周三 | 2026-08-05（星期三） |",
				"| 下周三 | 2026-08-05（星期三） |",
				"| 本周四 | 2026-08-06（星期四） |",
				"| 下周四 | 2026-08-06（星期四） |",
				"| 本周五 | 2026-07-31（星期五） |",
				"| 下周五 | 2026-08-07（星期五） |",
				"| 本周六 | 2026-08-01（星期六） |",
				"| 下周六 | 2026-08-08（星期六） |",
			},
		},
		{
			name: "year end thursday", // 2026-12-31 is a Thursday; crosses into 2027
			base: "2026-12-31",
			want: []string{
				"| 明天 | 2027-01-01（星期五） |",
				"| 后天 | 2027-01-02（星期六） |",
				"| 大后天 | 2027-01-03（星期日） |",
				"| 本周日 | 2027-01-03（星期日） |",
				"| 下周日 | 2027-01-10（星期日） |",
				"| 本周一 | 2027-01-04（星期一） |",
				"| 下周一 | 2027-01-04（星期一） |",
				"| 本周二 | 2027-01-05（星期二） |",
				"| 下周二 | 2027-01-05（星期二） |",
				"| 本周三 | 2027-01-06（星期三） |",
				"| 下周三 | 2027-01-06（星期三） |",
				"| 本周四 | 2026-12-31（星期四） |",
				"| 下周四 | 2027-01-07（星期四） |",
				"| 本周五 | 2027-01-01（星期五） |",
				"| 下周五 | 2027-01-08（星期五） |",
				"| 本周六 | 2027-01-02（星期六） |",
				"| 下周六 | 2027-01-09（星期六） |",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dc := BuildDateContext(tc.base, "", "10:00", "Asia/Shanghai", "zh-CN")
			want := strings.Join(tc.want, "\n")
			if dc.RelativeDateMap != want {
				t.Errorf("RelativeDateMap mismatch\n--- got ---\n%s\n--- want ---\n%s", dc.RelativeDateMap, want)
			}
		})
	}
}
