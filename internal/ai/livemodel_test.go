package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"daycore/internal/ai"
	_ "daycore/internal/ai/formats/openai"
)

// Live tool-calling conformance, one subtest per model.
//
// Everything else in this repo tests the agent loop against a fake provider,
// which proves our plumbing and nothing about whether a given model will
// actually drive it. The failure this catches is specific and common: a model
// that answers fluently in prose while never emitting a tool_call, or emitting
// one whose arguments do not satisfy our schema. Both look like "the AI is a bit
// dumb today" from the outside and are in fact a hard incompatibility — the
// companion cannot write to the plan at all.
//
// Gated on env vars and skipped by default, the same shape as the storage suites:
//
//	LIVE_MODEL_BASE_URL=https://ai.example.top/v1 \
//	LIVE_MODEL_API_KEY=sk-… \
//	LIVE_MODELS=deepseek-v4-flash,glm-5.2,kimi-k2.6 \
//	go test -count=1 -run TestLiveToolCalling -v ./internal/ai/
//
// It costs real money per run, so it is never in CI. `make test-models`.
func TestLiveToolCalling(t *testing.T) {
	base := os.Getenv("LIVE_MODEL_BASE_URL")
	key := os.Getenv("LIVE_MODEL_API_KEY")
	list := os.Getenv("LIVE_MODELS")
	if base == "" || key == "" || list == "" {
		t.Skip("LIVE_MODEL_BASE_URL / LIVE_MODEL_API_KEY / LIVE_MODELS not set — see `make test-models`")
	}

	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel() // the models are independent and each round trip is seconds
			runLiveToolCase(t, base, key, name)
		})
	}
}

// planAddTool is a trimmed copy of the real plan_add definition. Deliberately a
// copy rather than a reference to companionToolDefs: that lives in
// internal/server and importing it here would invert the dependency, and the
// point is the *shape* — required fields, an enum, a date string — not the exact
// wording, which changes.
var planAddTool = ai.ToolDef{
	Name:        "plan_add",
	Description: "在指定日期的计划里新增一个时间块。只影响这一天；长期/重复安排用 rule_upsert。",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"date":  map[string]any{"type": "string", "description": "目标日期 YYYY-MM-DD"},
			"title": map[string]any{"type": "string", "description": "标题"},
			"time":  map[string]any{"type": "string", "description": "开始时间 HH:MM；不确定可省略"},
			"type": map[string]any{
				"type": "string",
				"enum": []string{"task", "appointment", "break", "relax", "meal"},
			},
			"duration_min": map[string]any{"type": "integer"},
		},
		"required": []string{"date", "title"},
	},
}

type liveResult struct {
	toolCalled  bool
	name        string
	argsOK      bool
	date, title string
	blockType   string
	finish      string
	prose       string
	elapsed     time.Duration
	secondRound bool
}

func runLiveToolCase(t *testing.T, base, key, model string) {
	t.Helper()
	prov := liveProvider(t, base, key, model)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// A request that has exactly one correct action. If a model answers this in
	// prose it has failed, not been creative.
	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: "你是日程助手。用户要求安排日程时，必须调用 plan_add 工具，不要只用文字回答。" +
			"今天是 2026-07-30（星期四），明天是 2026-07-31（星期五）。"},
		{Role: ai.RoleUser, Content: "明天下午三点有个和导师的会，帮我加到计划里"},
	}

	start := time.Now()
	resp, err := chatWithRetry(ctx, t, prov, ai.ChatRequest{Messages: msgs, Tools: []ai.ToolDef{planAddTool}, MaxTokens: 1024})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	res := liveResult{finish: resp.FinishReason, prose: resp.Content, elapsed: time.Since(start)}

	if len(resp.ToolCalls) == 0 {
		t.Errorf("NO TOOL CALL — finish=%q, answered in prose instead: %.160q\n"+
			"    This model cannot drive the companion: every write would silently become chat.",
			resp.FinishReason, resp.Content)
		return
	}
	tc := resp.ToolCalls[0]
	res.toolCalled, res.name = true, tc.Name
	if tc.Name != "plan_add" {
		t.Errorf("called %q, not plan_add", tc.Name)
	}

	var args struct {
		Date        string `json:"date"`
		Title       string `json:"title"`
		Time        string `json:"time"`
		Type        string `json:"type"`
		DurationMin *int   `json:"duration_min"`
	}
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		t.Errorf("arguments are not valid JSON: %v\n    raw: %s", err, tc.Arguments)
		return
	}
	res.argsOK, res.date, res.title, res.blockType = true, args.Date, args.Title, args.Type

	// The schema's required fields must be present — a missing one means the tool
	// handler rejects the call and the user sees a failure.
	if args.Date == "" || args.Title == "" {
		t.Errorf("required field missing: date=%q title=%q (raw %s)", args.Date, args.Title, tc.Arguments)
	}
	if args.Date != "2026-07-31" {
		// Not fatal to the plumbing, but it is the single most common real
		// mistake: resolving "tomorrow" against the model's own idea of today
		// instead of the date the prompt supplied.
		t.Errorf("date is %q, want 2026-07-31 — the model ignored the supplied date context", args.Date)
	}
	if args.Type != "" && args.Type != "appointment" {
		t.Logf("type=%q (appointment would be the better read of \"和导师的会\")", args.Type)
	}
	if tc.ID == "" {
		t.Errorf("tool call has no id — the loop cannot correlate the result back")
	}

	// The other half of function calling: feeding the result back and getting a
	// natural-language confirmation rather than a second identical call.
	msgs = append(msgs,
		ai.Message{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{tc}},
		ai.Message{Role: ai.RoleTool, ToolCallID: tc.ID,
			Content: `{"ok":true,"date":"2026-07-31","block":{"time":"15:00","title":"导师会议"}}`},
	)
	second, err := prov.Chat(ctx, ai.ChatRequest{Messages: msgs, Tools: []ai.ToolDef{planAddTool}, MaxTokens: 1024})
	if err != nil {
		t.Errorf("second round (tool result fed back) failed: %v", err)
		return
	}
	res.secondRound = true
	if len(second.ToolCalls) > 0 {
		t.Errorf("called a tool again after being told it succeeded (%s) — this loops until AGENT_MAX_ROUNDS and writes the block repeatedly",
			second.ToolCalls[0].Name)
	}
	if strings.TrimSpace(second.Content) == "" {
		t.Error("second round produced no text — the user would see an empty reply after a successful write")
	}
	res.prose = second.Content

	t.Logf("OK  %s  →  plan_add(date=%s, title=%q, type=%s)  %.1fs\n    confirmation: %.100q",
		model, res.date, res.title, res.blockType, res.elapsed.Seconds(), strings.TrimSpace(res.prose))
}

// A model that declares streaming must also deliver tool calls incrementally,
// because that is the path the companion endpoint actually uses — Chat() is only
// used by the one-shot flows.
func TestLiveToolCallingStreamed(t *testing.T) {
	base := os.Getenv("LIVE_MODEL_BASE_URL")
	key := os.Getenv("LIVE_MODEL_API_KEY")
	list := os.Getenv("LIVE_MODELS")
	if base == "" || key == "" || list == "" {
		t.Skip("LIVE_MODEL_* not set")
	}
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			prov := liveProvider(t, base, key, name)
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			ch, err := prov.ChatStream(ctx, ai.ChatRequest{
				Messages: []ai.Message{
					{Role: ai.RoleSystem, Content: "你是日程助手。安排日程必须调用 plan_add，不要只用文字回答。今天 2026-07-30。"},
					{Role: ai.RoleUser, Content: "明天下午三点和导师开会，加到计划"},
				},
				Tools:     []ai.ToolDef{planAddTool},
				MaxTokens: 1024,
			})
			if err != nil {
				if unroutable(err) {
					t.Skipf("gateway will not route this model: %v", err)
				}
				t.Fatalf("stream failed: %v", err)
			}

			var (
				deltas, reasoning int
				argBuf            strings.Builder
				toolName, finish  string
				streamErr         error
			)
			for c := range ch {
				switch {
				case c.Err != nil:
					streamErr = c.Err
				case c.ToolCallDelta != nil:
					if c.ToolCallDelta.Name != "" {
						toolName = c.ToolCallDelta.Name
					}
					argBuf.WriteString(c.ToolCallDelta.ArgsDelta)
				case c.ContentDelta != "":
					deltas++
				case c.ReasoningDelta != "":
					reasoning++
				}
				if c.FinishReason != "" {
					finish = c.FinishReason
				}
			}
			if streamErr != nil {
				if unroutable(streamErr) {
					t.Skipf("gateway will not route this model: %v", streamErr)
				}
				t.Fatalf("stream error: %v", streamErr)
			}
			if toolName == "" {
				t.Errorf("streamed no tool call (finish=%q, %d text deltas, %d reasoning deltas) — the companion SSE path cannot write",
					finish, deltas, reasoning)
				return
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(argBuf.String()), &args); err != nil {
				t.Errorf("reassembled streamed arguments are not valid JSON: %v\n    %s\n"+
					"    (the format's per-index accumulation is the usual culprit)", err, argBuf.String())
				return
			}
			t.Logf("OK  %s  streamed %s(%v)  finish=%q  text-deltas=%d reasoning-deltas=%d",
				name, toolName, args, finish, deltas, reasoning)
			_ = fmt.Sprint()
		})
	}
}

// liveProvider builds one provider through the real config path — a temp
// models.yaml plus LoadCatalog — rather than reaching for an unexported
// constructor. It costs a file write and in exchange the test also covers the
// thing that actually breaks in production: whether a model entry written the
// way an operator writes it produces a working provider.
func liveProvider(t *testing.T, base, _, model string) ai.AIProvider {
	t.Helper()
	// The catalog resolves api_key_env from the process environment, and the key
	// is already there — that is how the test was invoked. Pointing the file at
	// that variable avoids t.Setenv, which panics in a parallel test, and keeps
	// the key out of the temp file either way.
	const env = "LIVE_MODEL_API_KEY"
	dir := t.TempDir()
	path := dir + "/models.yaml"
	yaml := fmt.Sprintf(`models:
  - id: %s
    format: openai
    base_url: %s
    model: %s
    api_key_env: %s
    tools: true
    stream: true
    context_window: 65536
    max_tokens: 1024
`, model, base, model, env)
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := ai.LoadCatalog(path, model, "", "")
	if err != nil {
		t.Fatalf("load catalog for %s: %v", model, err)
	}
	prov, ok := cat.Provider(model)
	if !ok {
		t.Fatalf("catalog has no provider %q", model)
	}
	return prov
}

// unroutable reports whether an error means the gateway would not route to this
// model at all, as opposed to the model behaving badly.
//
// This distinction is the whole point of the classification: a 503
// "No available channel" says nothing about whether the model can call tools,
// and reporting it as a failed tool-calling test is worse than not testing —
// it produces a compatibility matrix whose zeroes are lies. Observed live: a
// model that passed cleanly failed this way four minutes later, purely because
// the gateway stopped routing it.
func unroutable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, marker := range []string{
		"model_not_found",
		"No available channel",
		"可用渠道不存在",
		"get_channel_failed",
		"insufficient_user_quota",
		"quota",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// chatWithRetry retries once on a transient upstream failure, and skips — rather
// than fails — when the gateway simply has no channel for this model.
func chatWithRetry(ctx context.Context, t *testing.T, prov ai.AIProvider, req ai.ChatRequest) (*ai.ChatResponse, error) {
	t.Helper()
	resp, err := prov.Chat(ctx, req)
	if err == nil {
		return resp, nil
	}
	if unroutable(err) {
		t.Skipf("gateway will not route this model — nothing learned about its tool calling: %v", err)
	}
	// One retry: the gateways in front of these models return 5xx under load, and
	// a single flake should not be reported as a model incompatibility.
	if strings.Contains(err.Error(), "http 5") {
		t.Logf("transient upstream error, retrying once: %v", err)
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		resp, err2 := prov.Chat(ctx, req)
		if err2 == nil {
			return resp, nil
		}
		if unroutable(err2) {
			t.Skipf("gateway will not route this model: %v", err2)
		}
		return nil, err2
	}
	return nil, err
}
