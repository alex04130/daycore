package domain

import (
	"strings"
	"testing"
	"time"
)

func tp(s string) *string { return &s }

// The boundary conventions come from the prototype and matter at the edges:
// end == line is NOT stone (strict <), end == now IS recon (<=),
// start == now IS now.
func TestPhaseAtBoundaryMatrix(t *testing.T) {
	line := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		start, end time.Time
		want       Phase
	}{
		{"ends exactly at the line is not stone", now.Add(-13 * time.Hour), line, PhaseRecon},
		{"ends just before the line is stone", now.Add(-13 * time.Hour), line.Add(-time.Nanosecond), PhaseStone},
		{"ends exactly now is recon", now.Add(-time.Hour), now, PhaseRecon},
		{"starts exactly now is now", now, now.Add(time.Hour), PhaseNow},
		{"running", now.Add(-time.Hour), now.Add(time.Hour), PhaseNow},
		{"future", now.Add(time.Hour), now.Add(2 * time.Hour), PhaseFuture},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PhaseAt(tc.start, tc.end, now, line); got != tc.want {
				t.Errorf("PhaseAt = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSpanEdgeCases(t *testing.T) {
	loc := time.UTC
	// A malformed UTCTime anchor falls through to the wall clock rather than
	// making the block un-phaseable.
	b := TimeBlock{UTCTime: tp("garbage"), Time: tp("09:00"), Date: "2026-06-15"}
	start, _, ok := b.Span("", loc)
	if !ok || start.Hour() != 9 {
		t.Errorf("a malformed anchor must fall through to the wall clock, got (%v, %v)", start, ok)
	}
	// A valid anchor wins over the wall clock.
	b = TimeBlock{UTCTime: tp("2026-06-15T01:00:00Z"), Time: tp("09:00"), Date: "2026-06-15"}
	start, _, ok = b.Span("", loc)
	if !ok || start.Hour() != 1 {
		t.Errorf("a valid anchor must win, got (%v, %v)", start, ok)
	}
	// No time at all → ok=false: an unscheduled item never freezes.
	if _, _, ok := (TimeBlock{Date: "2026-06-15"}).Span("", loc); ok {
		t.Error("a block with no time must report ok=false")
	}
	// No date anywhere → ok=false.
	if _, _, ok := (TimeBlock{Time: tp("09:00")}).Span("", loc); ok {
		t.Error("a block with no date anywhere must report ok=false")
	}
	// planDate covers blocks that inherit their date from the day plan.
	if start, _, ok := (TimeBlock{Time: tp("09:00")}).Span("2026-06-15", loc); !ok || start.Day() != 15 {
		t.Errorf("planDate must supply the date, got (%v, %v)", start, ok)
	}
	// A non-positive duration adds nothing.
	for _, d := range []int{-5, 0} {
		b := TimeBlock{Time: tp("09:00"), Date: "2026-06-15", DurationMin: &d}
		start, end, ok := b.Span("", loc)
		if !ok || !end.Equal(start) {
			t.Errorf("duration %d must add nothing, got %v..%v", d, start, end)
		}
	}
}

func TestMovableMatrix(t *testing.T) {
	cases := []struct {
		name      string
		actor     string
		level     LockLevel
		confirmed bool
		want      bool
	}{
		{"user vs hard", ActorUser, LockHard, false, false},
		{"user vs hard even confirmed", ActorUser, LockHard, true, false},
		{"user vs soft unconfirmed", ActorUser, LockSoft, false, false},
		{"user vs soft confirmed", ActorUser, LockSoft, true, true},
		{"user vs unlocked", ActorUser, LockNone, false, true},
		{"agent vs hard", ActorAgent, LockHard, false, true},
		{"system vs hard", ActorSystem, LockHard, false, true},
		{"unknown actor vs hard", "", LockHard, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := TimeBlock{LockLevel: tc.level}
			if got := b.Movable(tc.actor, tc.confirmed); got != tc.want {
				t.Errorf("Movable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDeriveLockMatrix(t *testing.T) {
	b := &TimeBlock{Type: BlockAppointment, Title: "高等数学（课）"}
	if !DeriveLock(b, "zh-CN") || b.LockLevel != LockHard || b.LockSource != LockSourceDerived {
		t.Errorf("a class appointment must derive hard: %+v", b)
	}
	if b.LockReason == "" {
		t.Error("a derived lock must carry its default reason")
	}
	// Idempotent: deriving again changes nothing.
	cp := *b
	if DeriveLock(b, "zh-CN") || b.LockLevel != cp.LockLevel {
		t.Error("DeriveLock must be idempotent")
	}
	// A user-set lock is never re-derived.
	b2 := &TimeBlock{Type: BlockAppointment, Title: "高等数学（课）", LockLevel: LockNone, LockSource: LockSourceUser}
	if DeriveLock(b2, "zh-CN") || b2.LockLevel != LockNone {
		t.Errorf("a user decision must survive derivation: %+v", b2)
	}
	// Rederive moves a derived lock when the type/title changes…
	b3 := &TimeBlock{Type: BlockAppointment, Title: "高等数学（课）", LockLevel: LockHard, LockSource: LockSourceDerived, LockReason: "x"}
	b3.Title = "随便聊聊"
	b3.Type = BlockTask
	if !RederiveLock(b3, "zh-CN") || b3.LockLevel != LockNone {
		t.Errorf("editing a derived lock's inputs must move it: %+v", b3)
	}
	// …but never a user or agent decision.
	b4 := &TimeBlock{Type: BlockTask, LockLevel: LockHard, LockSource: LockSourceUser}
	if RederiveLock(b4, "zh-CN") {
		t.Error("RederiveLock must not touch a user lock")
	}
	// nil pointers are safe.
	if DeriveLock(nil, "") || RederiveLock(nil, "") {
		t.Error("nil blocks must be safe no-ops")
	}
}

func TestIsClassTitleMatrix(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"高等数学课", true},
		{"高等数学", false}, // 课$ needs a trailing 课; 数学 ends in 学
		{"离散数学（课）", true},
		{"数据结构课程", true},
		{"操作系统导论", true},
		{"电路实验课", true},
		{"Lecture on RL", true},
		{"Lab 课", true},
		{"数学课 ", true},
		{"和同学讨论", false},
		{"跑步", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsClassTitle(tc.in); got != tc.want {
			t.Errorf("IsClassTitle(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// The ideographic space divergence: RE2's \s is ASCII-only, so the
	// Lab\s*课 alternative does not fire across U+3000 — recorded in
	// api/lock-rules.json as a deliberate divergence from the JS prototype.
	// (A title that ENDS in 课 still matches via 课$, so the divergence only
	// shows when the ideographic space sits mid-title.)
	if IsClassTitle("Lab　课答疑") {
		t.Error("ideographic-space Lab\u3000课答疑 must NOT match (locked divergence)")
	}
	if !IsClassTitle("Lab　课") {
		t.Error("a trailing 课 still matches via 课$ even with an ideographic space")
	}
}

func TestOpDomainOfPrefixMatrix(t *testing.T) {
	cases := []struct {
		action string
		want   string
	}{
		{"plan_add", OpDomainSchedule},
		{"plan_update", OpDomainSchedule},
		{"autoplan", OpDomainSchedule},
		{"autoplanner", OpDomainSchedule},
		{"plan", OpDomainSystem},
		{"rule_upsert", OpDomainHabit},
		{"rule", OpDomainSystem},
		{"memory_add", OpDomainArchive},
		{"material_add", OpDomainArchive},
		{"wish_add", OpDomainArchive},
		{"assignment_upsert", OpDomainArchive},
		{"mood_record", OpDomainCare},
		{"protector_nudge", OpDomainCare},
		{"", OpDomainSystem},
		{"unknown_thing", OpDomainSystem},
	}
	for _, tc := range cases {
		if got := OpDomainOf(tc.action); got != tc.want {
			t.Errorf("OpDomainOf(%q) = %v, want %v", tc.action, got, tc.want)
		}
	}
}

func TestDeadlineRungForBoundaries(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		due  time.Time
		want time.Duration
		in   bool
	}{
		{"exactly 24h", now.Add(24 * time.Hour), 24 * time.Hour, true},
		{"exactly 12h", now.Add(12 * time.Hour), 12 * time.Hour, true},
		{"exactly 1h", now.Add(time.Hour), time.Hour, true},
		{"due exactly now", now, time.Hour, true},
		{"just overdue", now.Add(-time.Nanosecond), 0, true},
		{"beyond the ladder", now.Add(25 * time.Hour), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, in := DeadlineRungFor(tc.due, now)
			if got != tc.want || in != tc.in {
				t.Errorf("DeadlineRungFor = (%v, %v), want (%v, %v)", got, in, tc.want, tc.in)
			}
		})
	}
}

func TestRefishableMatrix(t *testing.T) {
	cases := []struct {
		name string
		b    TimeBlock
		want bool
	}{
		{"plain task", TimeBlock{Type: BlockTask}, true},
		{"completed", TimeBlock{Type: BlockTask, Completed: true}, false},
		{"achievement", TimeBlock{Type: BlockTask, IsAchievement: true}, false},
		{"appointment", TimeBlock{Type: BlockAppointment}, false},
		{"at the cap", TimeBlock{Type: BlockTask, RescheduleCount: RescheduleCap}, false},
		{"one under the cap", TimeBlock{Type: BlockTask, RescheduleCount: RescheduleCap - 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.Refishable(); got != tc.want {
				t.Errorf("Refishable = %v, want %v", got, tc.want)
			}
		})
	}
	// The chain carries the ROOT id forward and counts server-side.
	root, count := RescheduleChain(TimeBlock{ID: "orig", RescheduledFrom: "", RescheduleCount: 0})
	if root != "orig" || count != 1 {
		t.Errorf("RescheduleChain(first) = (%q, %d)", root, count)
	}
	root, count = RescheduleChain(TimeBlock{ID: "second", RescheduledFrom: "orig", RescheduleCount: 2})
	if root != "orig" || count != 3 {
		t.Errorf("RescheduleChain(deep) = (%q, %d), want the root and 3", root, count)
	}
}

func TestClampOpLogSummaryBoundary(t *testing.T) {
	short := strings.Repeat("字", 512)
	if got := ClampOpLogSummary(short); got != short {
		t.Error("exactly 512 runes must pass through")
	}
	long := strings.Repeat("字", 513)
	got := ClampOpLogSummary(long)
	if len([]rune(got)) != 512 {
		t.Errorf("513 runes must clamp to 512, got %d", len([]rune(got)))
	}
}

func TestAttachmentKindOf(t *testing.T) {
	cases := []struct{ mime, want string }{
		{"image/png", AttachmentImage},
		{"IMAGE/PNG", AttachmentImage},
		{"image/png; charset=utf-8", AttachmentImage},
		{"audio/mpeg", AttachmentAudio},
		{"video/mp4", AttachmentVideo},
		{"text/plain", AttachmentDocument},
		{"application/pdf", AttachmentDocument},
		{"application/octet-stream", AttachmentFile},
		{"", AttachmentFile},
	}
	for _, tc := range cases {
		if got := AttachmentKindOf(tc.mime); got != tc.want {
			t.Errorf("AttachmentKindOf(%q) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

func TestSafeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a.png", "a.png"},
		{"/tmp/../a.png", "a.png"},
		{"C:\\Users\\me\\a.png", "a.png"},
		{"../../etc/passwd", "passwd"}, // only the LAST path element survives — that is the sanitisation
		{"a\r\nb.png", "ab.png"},
		{"say\"hi\".txt", "sayhi.txt"},
		{"..hidden", "hidden"},
		{"  spaced  ", "spaced"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := SafeFilename(tc.in); got != tc.want {
			t.Errorf("SafeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Over-long names are truncated to 200 bytes, not left to the header.
	long := strings.Repeat("a", 250)
	if got := SafeFilename(long); len(got) != 200 {
		t.Errorf("a 250-byte name must clamp to 200, got %d", len(got))
	}
}
