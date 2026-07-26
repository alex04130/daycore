package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/ai"
	"daycore/internal/config"
	"daycore/internal/domain"
	"daycore/internal/storage"

	_ "daycore/internal/storage/sqlstore" // register the sqlite opener
)

// fakeProvider replays a scripted sequence of responses; ChatStream reuses
// StreamViaChat so the loop exercises the exact same chunk contract.
type fakeProvider struct {
	script []*ai.ChatResponse
	calls  int
	reqs   []ai.ChatRequest
}

func (f *fakeProvider) Chat(_ context.Context, req ai.ChatRequest) (*ai.ChatResponse, error) {
	f.reqs = append(f.reqs, req)
	i := f.calls
	if i >= len(f.script) {
		i = len(f.script) - 1
	}
	f.calls++
	return f.script[i], nil
}

func (f *fakeProvider) ChatStream(ctx context.Context, req ai.ChatRequest) (<-chan ai.Chunk, error) {
	return ai.StreamViaChat(ctx, f, req)
}

func (f *fakeProvider) Capabilities() ai.Capabilities {
	return ai.Capabilities{Tools: true, Stream: true}
}
func (f *fakeProvider) Model() string { return "fake" }

func newAgentTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	sid := "agent-test-sid"
	if _, err := store.Sessions().GetOrCreate(context.Background(), sid); err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Config: &config.Config{AgentMaxRounds: 3, RateLimitPerMin: 0},
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	})
	return s, sid
}

// runAgent drives runCompanionAgent against a recorder and returns the parsed
// SSE frames in order.
func runAgent(t *testing.T, s *Server, provider ai.AIProvider, sid, userMsg string) []map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ai/companion", nil)
	rc := http.NewResponseController(rec)
	messages := []ai.Message{
		{Role: ai.RoleSystem, Content: "system"},
		{Role: ai.RoleUser, Content: userMsg},
	}
	s.runCompanionAgent(context.Background(), sseSender{w: rec, rc: rc}, req, provider, sid, "zh-CN", "Asia/Shanghai", messages, true)

	var frames []map[string]any
	for _, block := range strings.Split(rec.Body.String(), "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m); err == nil {
				frames = append(frames, m)
			}
		}
	}
	return frames
}

func frameTypes(frames []map[string]any) []string {
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		if t, _ := f["type"].(string); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func findFrame(frames []map[string]any, typ string) map[string]any {
	for _, f := range frames {
		if f["type"] == typ {
			return f
		}
	}
	return nil
}

func toolCall(name, args string) ai.ToolCall {
	return ai.ToolCall{ID: "call_test", Name: name, Arguments: args}
}

func TestAgentLoopMemoryAddThenAnswer(t *testing.T) {
	s, sid := newAgentTestServer(t)
	p := &fakeProvider{script: []*ai.ChatResponse{
		{ToolCalls: []ai.ToolCall{toolCall("memory_add", `{"fact":"晚上11点后不安排任务"}`)}, FinishReason: "tool_calls"},
		{Content: "记住了", FinishReason: "stop"},
	}}
	frames := runAgent(t, s, p, sid, "记住我晚上11点后不干活")

	want := []string{"tool_start", "tool_result", "delta", "done"}
	if got := frameTypes(frames); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("frame sequence = %v, want %v", got, want)
	}
	tr := findFrame(frames, "tool_result")
	if tr["ok"] != true || tr["tool"] != "memory_add" || tr["opId"] == nil {
		t.Errorf("tool_result = %v", tr)
	}
	facts, _ := s.store.Memory().ListFacts(context.Background(), sid)
	if len(facts) != 1 || facts[0].Fact != "晚上11点后不安排任务" {
		t.Errorf("facts = %+v", facts)
	}
	logs, _ := s.store.OpLogs().List(context.Background(), sid, 10)
	if len(logs) != 1 || logs[0].Action != "memory_add" || logs[0].Actor != domain.ActorAgent {
		t.Errorf("op logs = %+v", logs)
	}
	// The tool round must offer tools; results are injected as role=tool messages.
	if len(p.reqs) != 2 || len(p.reqs[0].Tools) == 0 {
		t.Fatalf("reqs = %d, tools in round 1 = %d", len(p.reqs), len(p.reqs[0].Tools))
	}
	last := p.reqs[1].Messages[len(p.reqs[1].Messages)-1]
	if last.Role != ai.RoleTool || !strings.Contains(last.Content, `"ok":true`) {
		t.Errorf("tool message not injected: %+v", last)
	}
}

func TestAgentLoopPlanAddLandsOnDate(t *testing.T) {
	s, sid := newAgentTestServer(t)
	p := &fakeProvider{script: []*ai.ChatResponse{
		{ToolCalls: []ai.ToolCall{toolCall("plan_add", `{"date":"2026-07-13","title":"开会","time":"15:00","duration_min":60}`)}, FinishReason: "tool_calls"},
		{Content: "排好了", FinishReason: "stop"},
	}}
	frames := runAgent(t, s, p, sid, "明天下午三点开会")

	tr := findFrame(frames, "tool_result")
	if tr == nil || tr["ok"] != true {
		t.Fatalf("tool_result = %v", tr)
	}
	data, _ := tr["data"].(map[string]any)
	if data["date"] != "2026-07-13" {
		t.Errorf("data.date = %v", data["date"])
	}
	plan, err := s.store.DayPlans().Get(context.Background(), sid, "2026-07-13")
	if err != nil || len(plan.Blocks) != 1 || plan.Blocks[0].Title != "开会" {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
	if plan.Blocks[0].Time == nil || *plan.Blocks[0].Time != "15:00" {
		t.Errorf("time = %v", plan.Blocks[0].Time)
	}
}

func TestAgentLoopToolFailureKeepsGoing(t *testing.T) {
	s, sid := newAgentTestServer(t)
	p := &fakeProvider{script: []*ai.ChatResponse{
		{ToolCalls: []ai.ToolCall{toolCall("plan_update", `{"date":"2026-07-13","match":{"title":"不存在"},"changes":{"time":"16:00"}}`)}, FinishReason: "tool_calls"},
		{Content: "找不到那个安排，你看下标题对不对", FinishReason: "stop"},
	}}
	frames := runAgent(t, s, p, sid, "把会改到四点")

	tr := findFrame(frames, "tool_result")
	if tr == nil || tr["ok"] != false {
		t.Fatalf("tool_result = %v", tr)
	}
	if findFrame(frames, "done") == nil || findFrame(frames, "delta") == nil {
		t.Errorf("loop did not continue after failure: %v", frameTypes(frames))
	}
}

func TestAgentLoopRoundsExhaustedForcesWrapUp(t *testing.T) {
	s, sid := newAgentTestServer(t)
	// Every scripted round asks for another tool — the loop must cut it off
	// after AgentMaxRounds and force a tool-free wrap-up round.
	p := &fakeProvider{script: []*ai.ChatResponse{
		{ToolCalls: []ai.ToolCall{toolCall("memory_add", `{"fact":"循环"}`)}, FinishReason: "tool_calls"},
	}}
	frames := runAgent(t, s, p, sid, "test")

	if findFrame(frames, "done") == nil {
		t.Fatalf("no done frame: %v", frameTypes(frames))
	}
	// 3 tool rounds + 1 wrap-up round.
	if p.calls != 4 {
		t.Errorf("model calls = %d, want 4", p.calls)
	}
	lastReq := p.reqs[len(p.reqs)-1]
	if len(lastReq.Tools) != 0 {
		t.Errorf("wrap-up round still offered tools")
	}
	lastMsg := lastReq.Messages[len(lastReq.Messages)-1]
	if lastMsg.Role != ai.RoleUser || !strings.Contains(lastMsg.Content, "工具轮次已用尽") {
		t.Errorf("wrap-up nudge missing: %+v", lastMsg)
	}
}

func TestDecisionRegistryOneCardPerSession(t *testing.T) {
	r := newDecisionRegistry()
	ch1 := r.register("sid", "dc_1")
	_ = r.register("sid", "dc_2") // supersedes dc_1
	select {
	case ans := <-ch1:
		if ans.Choice != "cancelled" {
			t.Errorf("first card answer = %+v", ans)
		}
	default:
		t.Fatal("first card was not cancelled")
	}
	if r.resolve("dc_1", "sid", decisionAnswer{Choice: "a"}) {
		t.Error("stale card resolved")
	}
	if !r.resolve("dc_2", "sid", decisionAnswer{Choice: "b"}) {
		t.Error("live card not resolved")
	}
	if r.resolve("dc_2", "sid", decisionAnswer{Choice: "b"}) {
		t.Error("double resolve")
	}
}
