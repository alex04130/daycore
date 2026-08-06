package ai

import (
	"context"
	"strings"
	"testing"
)

func TestRenderCompanionAgentAndContext(t *testing.T) {
	s, err := NewPromptService(nil)
	if err != nil {
		t.Fatalf("NewPromptService: %v", err)
	}
	ctx := context.Background()

	agent := CompanionAgentData{}
	cctx := CompanionContextData{
		Date: "2026-07-11", Weekday: "星期六", Time: "10:00", Timezone: "Asia/Shanghai",
		RelativeDateMap: "| 明天 | 2026-07-12（星期日） |",
		WeatherSummary:  "晴，32°C",
		TodayPlan:       `[{"title":"团队会议"}]`,
	}

	cases := []struct {
		key, locale string
		data        any
		want        []string
	}{
		{PromptCompanionAgent, "zh-CN", agent, []string{
			"## 工具纪律",
			"400-161-9995",
			"## 安全底线",
			"## 何时不操作",
		}},
		{PromptCompanionAgent, "en-US", agent, []string{
			"## Tool discipline",
			"988",
			"## Safety baseline",
			"## When not to act",
		}},
		{PromptCompanionContext, "zh-CN", cctx, []string{
			"## 当前上下文",
			"今天 2026-07-11，星期六",
			"- 天气：晴，32°C",
			"| 今天 | 2026-07-11（星期六） |",
			"| 明天 | 2026-07-12（星期日） |",
			"- 明日计划：",
		}},
		{PromptCompanionContext, "en-US", cctx, []string{
			"## Current context",
			"today is 2026-07-11, 星期六",
			"- Weather: 晴，32°C",
			"| today | 2026-07-11 (星期六) |",
			"- Tomorrow's plan:",
		}},
	}
	for _, tc := range cases {
		out, err := s.Render(ctx, tc.key, tc.locale, tc.data)
		if err != nil {
			t.Fatalf("Render(%s, %s): %v", tc.key, tc.locale, err)
		}
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Errorf("Render(%s, %s) missing %q\n--- out ---\n%s", tc.key, tc.locale, want, out)
			}
		}
	}
}

// The L2 persona moved out of a Go switch and into the template system, so it
// gains what every other prompt already had: console overrides, a third
// language without a release, and the double-locale boot check. The assertions
// are unchanged — they are about the words, not about where they live.
func TestDefaultPersona(t *testing.T) {
	svc, err := NewPromptService(nil)
	if err != nil {
		t.Fatalf("NewPromptService: %v", err)
	}
	ctx := context.Background()
	for _, tc := range []struct{ locale, name, want string }{
		{"zh-CN", "小昼", "你是 小昼"},
		{"zh", "Leo", "你是 Leo"},
		{"en-US", "Leo", "You are Leo"},
		{"en", "小昼", "You are 小昼"},
	} {
		out, err := svc.Render(ctx, PromptPersona, tc.locale, map[string]any{"Name": tc.name})
		if err != nil {
			t.Fatalf("Render(persona, %s): %v", tc.locale, err)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("persona(%s, %s) missing %q\n--- out ---\n%s", tc.locale, tc.name, tc.want, out)
		}
		// Default persona must NOT contain hard boundary language.
		for _, bad := range []string{"绝对禁止", "Absolutely prohibited", "工具纪律", "Tool discipline"} {
			if strings.Contains(out, bad) {
				t.Errorf("persona(%s, %s) must not contain L1 boundary %q", tc.locale, tc.name, bad)
			}
		}
	}
}

func TestHardBoundaryReminder(t *testing.T) {
	for _, tc := range []struct{ locale, want string }{
		{"zh-CN", "硬约束重申"},
		{"zh", "硬约束重申"},
		{"en-US", "Hard boundaries"},
		{"en", "Hard boundaries"},
	} {
		out := HardBoundaryReminder(tc.locale)
		if !strings.Contains(out, tc.want) {
			t.Errorf("HardBoundaryReminder(%s) missing %q\n--- out ---\n%s", tc.locale, tc.want, out)
		}
		// Reminder must NOT contain personality/role language.
		for _, bad := range []string{"伙伴", "companion", "管家", "butler", "本能", "instinct", "角色", "Role", "你是", "You are"} {
			if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
				t.Errorf("HardBoundaryReminder(%s) must not contain personality word %q", tc.locale, bad)
			}
		}
	}
}

// TestPromptAssemblyOrder verifies that when L1, L3, L2, and L1_reminder are
// concatenated, the hard boundary reminder appears AFTER the persona (L2).
func TestPromptAssemblyOrder(t *testing.T) {
	l1 := "## 工具纪律\n1. 必须调用工具"
	l3 := "## 当前上下文\n今天 2026-07-13"
	l2 := "## 角色\n你是 Leo"
	reminder := "## 硬约束重申\n以这些约束为准"

	assembled := l1 + "\n\n" + l3 + l2 + "\n\n" + reminder

	l1Pos := strings.Index(assembled, "工具纪律")
	l2Pos := strings.Index(assembled, "你是 Leo")
	reminderPos := strings.Index(assembled, "硬约束重申")

	if l1Pos < 0 || l2Pos < 0 || reminderPos < 0 {
		t.Fatal("one of the layers is missing from the assembled prompt")
	}
	if reminderPos < l2Pos {
		t.Error("L1_reminder must appear AFTER L2 persona")
	}
	if l1Pos > l2Pos {
		t.Error("L1 must appear BEFORE L2 persona")
	}
}

func TestRenderCompanionContextHidesEmptyWeather(t *testing.T) {
	s, err := NewPromptService(nil)
	if err != nil {
		t.Fatalf("NewPromptService: %v", err)
	}
	data := CompanionContextData{Date: "2026-07-11", Weekday: "星期六", Time: "10:00", Timezone: "Asia/Shanghai"}
	for locale, marker := range map[string]string{"zh-CN": "天气：", "en-US": "Weather:"} {
		out, err := s.Render(context.Background(), PromptCompanionContext, locale, data)
		if err != nil {
			t.Fatalf("Render(%s): %v", locale, err)
		}
		if strings.Contains(out, marker) {
			t.Errorf("Render(%s) should hide the weather line when WeatherSummary is empty\n--- out ---\n%s", locale, out)
		}
	}
}
