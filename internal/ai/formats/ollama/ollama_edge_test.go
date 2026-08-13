package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/ai"
)

func ndjsonServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n"))
		}
	}))
}

func ollamaChunks(t *testing.T, baseURL string) []ai.Chunk {
	t.Helper()
	p, err := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := p.ChatStream(context.Background(), ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var out []ai.Chunk
	for c := range stream {
		out = append(out, c)
	}
	return out
}

func TestChatStreamContentAndUsage(t *testing.T) {
	srv := ndjsonServer(t, []string{
		`{"message":{"content":"你好"},"done":false}`,
		`{"message":{"content":"！"},"done":false}`,
		`{"message":{"content":""},"done":true,"prompt_eval_count":11,"eval_count":7}`,
	})
	defer srv.Close()
	chunks := ollamaChunks(t, srv.URL)
	if len(chunks) != 3 {
		t.Fatalf("want 2 content + 1 done, got %+v", chunks)
	}
	if chunks[0].ContentDelta != "你好" || chunks[1].ContentDelta != "！" {
		t.Errorf("content deltas wrong: %+v", chunks)
	}
	if !chunks[2].Done || chunks[2].Usage == nil || chunks[2].Usage.PromptTokens != 11 || chunks[2].Usage.CompletionTokens != 7 {
		t.Errorf("the terminal frame must carry Done+Usage, got %+v", chunks[2])
	}
}

// The stream ignores the error FIELD entirely: an error line flows past as
// an empty frame and the stream runs to EOF. Locked behaviour.
func TestChatStreamErrorLineIgnored(t *testing.T) {
	srv := ndjsonServer(t, []string{`{"error":"model not found"}`})
	defer srv.Close()
	chunks := ollamaChunks(t, srv.URL)
	if len(chunks) != 0 {
		t.Errorf("streamed error lines are currently ignored, got %+v", chunks)
	}
	// The non-streaming path DOES report it.
	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL})
	if _, err := p.Chat(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Errorf("non-streaming Chat must report the error field, got %v", err)
	}
}

// Invalid JSON lines are skipped; tool_calls on the terminal frame are
// dropped (the format only reads message.content). Both locked.
func TestChatStreamInvalidJSONAndToolCallsDropped(t *testing.T) {
	srv := ndjsonServer(t, []string{
		"not json",
		`{"message":{"content":"x","tool_calls":[{"function":{"name":"plan_add"}}]},"done":true}`,
	})
	defer srv.Close()
	chunks := ollamaChunks(t, srv.URL)
	if len(chunks) != 2 || chunks[0].ContentDelta != "x" || !chunks[1].Done {
		t.Errorf("got %+v", chunks)
	}
}

// Tool arguments must be a JSON object: an array fails the whole decode
// rather than degrading silently — the wire struct types arguments as a map,
// and the error names the field. Locked behaviour.
func TestNonObjectToolArgsFailTheDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"content":"","tool_calls":[{"function":{"name":"plan_add","arguments":[1,2,3]}}]},"done":true}`))
	}))
	defer srv.Close()
	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL})
	if _, err := p.Chat(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("array arguments must fail the decode, got %v", err)
	}
	// Object arguments round-trip with a synthesised id.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"content":"","tool_calls":[{"function":{"name":"plan_add","arguments":{"date":"2026-01-01"}}}]},"done":true}`))
	}))
	defer srv2.Close()
	p2, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv2.URL})
	resp, err := p2.Chat(context.Background(), ai.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_0" || resp.ToolCalls[0].Name != "plan_add" {
		t.Fatalf("tool call = %+v", resp.ToolCalls)
	}
	if !strings.Contains(resp.ToolCalls[0].Arguments, `"date":"2026-01-01"`) {
		t.Errorf("arguments = %q", resp.ToolCalls[0].Arguments)
	}
}

func TestChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL})
	if _, err := p.Chat(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("Chat must fail on HTTP errors, got %v", err)
	}
	if _, err := p.ChatStream(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("ChatStream must fail on HTTP errors, got %v", err)
	}
}
