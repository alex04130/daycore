package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/ai"
)

// sseServer replies to every request with the given SSE lines (already
// including the "data: " prefix), one per line.
func sseServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n\n"))
		}
	}))
}

func streamAll(t *testing.T, baseURL string, req ai.ChatRequest) *ai.ChatResponse {
	t.Helper()
	p, err := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: baseURL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := p.ChatStream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var acc ai.StreamAccumulator
	for c := range stream {
		if c.Err != nil {
			t.Fatalf("stream err: %v", c.Err)
		}
		acc.Add(c)
	}
	return acc.Response()
}

func TestChatStreamFragmentedToolCall(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"content":"我查一下"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"plan_add","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"date\":\"20"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"26-07-13\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	resp := streamAll(t, srv.URL, ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if resp.Content != "我查一下" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc" || tc.Name != "plan_add" || tc.Arguments != `{"date":"2026-07-13"}` {
		t.Errorf("tool call = %+v", tc)
	}
}

func TestChatStreamWholeBlockToolCall(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"get_weather","arguments":"{\"location\":\"上海\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	resp := streamAll(t, srv.URL, ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Arguments != `{"location":"上海"}` {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
}

func TestChatStreamParallelToolCalls(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"get_weather","arguments":"{\"l"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"web_search","arguments":"{\"q"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":1}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\":2}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	resp := streamAll(t, srv.URL, ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
	if resp.ToolCalls[0].Name != "get_weather" || resp.ToolCalls[0].Arguments != `{"l":1}` {
		t.Errorf("call 0 = %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[1].Name != "web_search" || resp.ToolCalls[1].Arguments != `{"q":2}` {
		t.Errorf("call 1 = %+v", resp.ToolCalls[1])
	}
}

func TestChatStreamReasoningAndPlainText(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"reasoning_content":"先想想"}}]}`,
		`data: {"choices":[{"delta":{"content":"你好"}}]}`,
		`data: {"choices":[{"delta":{"content":"呀"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	p, err := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := p.ChatStream(context.Background(), ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var reasoning, content, finish string
	for c := range stream {
		reasoning += c.ReasoningDelta
		content += c.ContentDelta
		if c.FinishReason != "" {
			finish = c.FinishReason
		}
	}
	if reasoning != "先想想" || content != "你好呀" || finish != "stop" {
		t.Errorf("reasoning=%q content=%q finish=%q", reasoning, content, finish)
	}
}

func TestAccumulatorSynthesizesMissingID(t *testing.T) {
	var acc ai.StreamAccumulator
	acc.Add(ai.Chunk{ToolCallDelta: &ai.ToolCallDelta{Index: 0, Name: "plan_add", ArgsDelta: `{}`}})
	acc.Add(ai.Chunk{FinishReason: "tool_calls"})
	resp := acc.Response()
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_0" {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
}

func TestExtraBodyMerge(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p, err := New(ai.ModelConfig{
		ID: "m", Model: "real-model", BaseURL: srv.URL, APIKey: "k",
		ExtraBody: map[string]any{"thinking": map[string]any{"enabled": true}, "model": "evil-override"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Chat(context.Background(), ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	think, ok := got["thinking"].(map[string]any)
	if !ok || think["enabled"] != true {
		t.Errorf("thinking not merged: %v", got["thinking"])
	}
	if got["model"] != "real-model" {
		t.Errorf("extra_body overrode built key: model = %v", got["model"])
	}
}

func TestModelLevelMaxTokensDefault(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL, APIKey: "k", MaxTokens: 4096})
	_, err := p.Chat(context.Background(), ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want model default 4096", got["max_tokens"])
	}
	// An explicit request value wins over the model default.
	_, err = p.Chat(context.Background(), ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, MaxTokens: 300})
	if err != nil {
		t.Fatal(err)
	}
	if got["max_tokens"] != float64(300) {
		t.Errorf("max_tokens = %v, want request override 300", got["max_tokens"])
	}
}

func TestStreamViaChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"c9","type":"function","function":{"name":"memory_add","arguments":"{\"fact\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer srv.Close()

	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL, APIKey: "k"})
	stream, err := ai.StreamViaChat(context.Background(), p, ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var acc ai.StreamAccumulator
	sawDone := false
	for c := range stream {
		if c.Done {
			sawDone = true
		}
		acc.Add(c)
	}
	resp := acc.Response()
	if !sawDone || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "memory_add" || resp.FinishReason != "tool_calls" {
		t.Fatalf("done=%v resp=%+v", sawDone, resp)
	}
	if !strings.Contains(resp.ToolCalls[0].Arguments, "fact") {
		t.Errorf("arguments = %q", resp.ToolCalls[0].Arguments)
	}
}
