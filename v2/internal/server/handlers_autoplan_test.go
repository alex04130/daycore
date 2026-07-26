package server

import (
	"encoding/json"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// autoBlocksInput decodes a JSON array literal the same way the model's
// response reaches parseAutoBlocks (as []any of map[string]any).
func autoBlocksInput(t *testing.T, jsonArr string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(jsonArr), &v); err != nil {
		t.Fatalf("bad test JSON: %v", err)
	}
	return v
}

func TestParseAutoBlocks(t *testing.T) {
	input := autoBlocksInput(t, `[
		{"date":"2026-07-01","time":"09:00","title":"写作业","type":"appointment","time_mode":"fixed","timezone":"America/New_York","completed":true,"hidden":true,"rule_id":"r1"},
		{"date":"2026-07-02","title":"休息","type":"nonsense","time_mode":"whenever"},
		{"date":"2026-07-03","title":"越界","type":"task"},
		{"date":"2026-06-30","title":"太早","type":"task"},
		{"date":"07/02/2026","title":"坏日期","type":"task"},
		{"date":"2026-07-01","title":"   ","type":"task"}
	]`)

	byDate, warnings := parseAutoBlocks(input, "2026-07-01", "2026-07-02", "Asia/Shanghai")

	if len(byDate) != 2 || len(byDate["2026-07-01"]) != 1 || len(byDate["2026-07-02"]) != 1 {
		t.Fatalf("byDate = %#v", byDate)
	}

	// out-of-range / malformed dates warn; the blank title is dropped silently
	if len(warnings) != 3 {
		t.Fatalf("warnings = %v", warnings)
	}
	for i, title := range []string{"越界", "太早", "坏日期"} {
		if !strings.Contains(warnings[i], title) || !strings.Contains(warnings[i], "日期越界") {
			t.Errorf("warnings[%d] = %q", i, warnings[i])
		}
	}

	// valid block: id/origin stamped, model-controlled flags reset, rest preserved
	b := byDate["2026-07-01"][0]
	if !strings.HasPrefix(b.ID, "block-") {
		t.Errorf("ID = %q", b.ID)
	}
	if b.Origin != domain.OriginAuto || b.RuleID != "" || b.Hidden || b.Completed {
		t.Errorf("stamping failed: %+v", b)
	}
	if b.Type != domain.BlockAppointment || b.TimeMode != domain.TimeFixed || b.Timezone != "America/New_York" {
		t.Errorf("valid fields not preserved: %+v", b)
	}
	if b.Time == nil || *b.Time != "09:00" {
		t.Errorf("Time = %v", b.Time)
	}

	// invalid type/time_mode and missing timezone fall back to defaults
	f := byDate["2026-07-02"][0]
	if f.Type != domain.BlockTask || f.TimeMode != domain.TimeFloating || f.Timezone != "Asia/Shanghai" {
		t.Errorf("fallbacks failed: %+v", f)
	}
}

func TestParseAutoBlocksNonArray(t *testing.T) {
	for _, v := range []any{nil, "not an array", map[string]any{"date": "2026-07-01"}} {
		byDate, warnings := parseAutoBlocks(v, "2026-07-01", "2026-07-01", "UTC")
		if len(byDate) != 0 || len(warnings) != 0 {
			t.Errorf("input %v: byDate=%v warnings=%v", v, byDate, warnings)
		}
	}
}

func TestParseAutoBlocksUnparseable(t *testing.T) {
	input := autoBlocksInput(t, `[{"date":123,"title":"x"}]`)
	byDate, warnings := parseAutoBlocks(input, "2026-07-01", "2026-07-01", "UTC")
	if len(byDate) != 0 {
		t.Fatalf("byDate = %#v", byDate)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "无法解析") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestParseAutoBlocksUniqueIDs(t *testing.T) {
	input := autoBlocksInput(t, `[
		{"date":"2026-07-01","title":"A","type":"task"},
		{"date":"2026-07-01","title":"B","type":"task"}
	]`)
	byDate, _ := parseAutoBlocks(input, "2026-07-01", "2026-07-01", "UTC")
	got := byDate["2026-07-01"]
	if len(got) != 2 || got[0].ID == "" || got[0].ID == got[1].ID {
		t.Fatalf("blocks = %+v", got)
	}
}
