// Package anthropic implements the Anthropic Messages API wire format
// (system as a top-level field, x-api-key auth, tool_use/tool_result blocks,
// base64 image blocks, content_block_delta SSE). Registered as format "anthropic".
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"daycore/internal/ai"
)

const apiVersion = "2023-06-01"

func init() { ai.RegisterFormat("anthropic", New) }

// New builds an Anthropic Messages provider.
func New(cfg ai.ModelConfig) (ai.AIProvider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	return &provider{
		cfg:      cfg,
		http:     &http.Client{},
		endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/v1/messages",
	}, nil
}

type provider struct {
	cfg      ai.ModelConfig
	http     *http.Client
	endpoint string
}

func (p *provider) Model() string                 { return p.cfg.Model }
func (p *provider) Capabilities() ai.Capabilities { return p.cfg.Caps }

// ─── wire types ──────────────────────────────────────────────────────────────

type msgReq struct {
	Model       string     `json:"model"`
	System      any        `json:"system,omitempty"`
	Messages    []wireMsg  `json:"messages"`
	MaxTokens   int        `json:"max_tokens"`
	Temperature float64    `json:"temperature,omitempty"`
	Stop        []string   `json:"stop_sequences,omitempty"`
	Tools       []wireTool `json:"tools,omitempty"`
	Stream      bool       `json:"stream,omitempty"`
}

// wireTool is either a client-side tool (name + input_schema) or a
// provider-executed one (type + max_uses, no schema).
//
// ⚠️ The two are the same array on the wire and different shapes inside it.
// A server tool with an input_schema is rejected; a client tool without one is
// useless. Hence the omitempty on both halves and the branch in buildReq.
type wireTool struct {
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	MaxUses     int            `json:"max_uses,omitempty"`
}

type wireMsg struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

// block is a flexible content block (text / image / tool_use / tool_result).
type block struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// image
	Source *imgSource `json:"source,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type imgSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

func (p *provider) buildReq(req ai.ChatRequest, stream bool) msgReq {
	out := msgReq{
		Model:       p.cfg.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stop:        req.Stop,
		Stream:      stream,
	}
	if out.MaxTokens == 0 {
		out.MaxTokens = p.cfg.MaxTokens
	}
	if out.MaxTokens == 0 {
		out.MaxTokens = 1024 // Anthropic requires max_tokens
	}
	for _, t := range req.Tools {
		if t.ServerSide != "" {
			// ⚠️ No input_schema and no description: the provider owns both. Sending
			// a schema for a tool we do not implement is a 400.
			out.Tools = append(out.Tools, wireTool{Type: t.ServerSide, Name: t.Name, MaxUses: t.MaxUses})
			continue
		}
		out.Tools = append(out.Tools, wireTool{Name: t.Name, Description: t.Description, InputSchema: t.Parameters})
	}

	var systems []string
	for _, m := range req.Messages {
		switch m.Role {
		case ai.RoleSystem:
			systems = append(systems, m.Content)
		case ai.RoleTool:
			out.Messages = append(out.Messages, wireMsg{Role: "user", Content: []block{{
				Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content,
			}}})
		default:
			out.Messages = append(out.Messages, wireMsg{Role: string(m.Role), Content: toBlocks(m)})
		}
	}
	if len(systems) > 0 {
		out.System = systemBlocks(systems)
	}
	return out
}

func toBlocks(m ai.Message) []block {
	var blocks []block
	if len(m.Parts) > 0 {
		for _, pt := range m.Parts {
			if pt.Type == ai.PartImage {
				blocks = append(blocks, block{Type: "image", Source: &imgSource{
					Type: "base64", MediaType: pt.MIME, Data: pt.Data,
				}})
			} else {
				blocks = append(blocks, block{Type: "text", Text: pt.Text})
			}
		}
	} else if m.Content != "" {
		blocks = append(blocks, block{Type: "text", Text: m.Content})
	}
	for _, tc := range m.ToolCalls {
		args := tc.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		blocks = append(blocks, block{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: json.RawMessage(args)})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, block{Type: "text", Text: ""})
	}
	return blocks
}

func (p *provider) post(ctx context.Context, body msgReq) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", apiVersion)
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	}
	return p.http.Do(httpReq)
}

type msgResp struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		// cache_read_input_tokens is the prompt portion served from Anthropic's
		// cache — the only direct evidence prompt caching is working.
		CacheRead int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (p *provider) Chat(ctx context.Context, req ai.ChatRequest) (*ai.ChatResponse, error) {
	resp, err := p.post(ctx, p.buildReq(req, false))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic: http %d: %s", resp.StatusCode, truncate(data))
	}
	var mr msgResp
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("anthropic: decode: %w", err)
	}
	if mr.Error != nil {
		return nil, fmt.Errorf("anthropic: %s", mr.Error.Message)
	}
	out := &ai.ChatResponse{FinishReason: mr.StopReason}
	if mr.Usage != nil {
		out.Usage = ai.Usage{
			PromptTokens:     mr.Usage.InputTokens,
			CompletionTokens: mr.Usage.OutputTokens,
			CachedTokens:     mr.Usage.CacheRead,
		}
	}
	out.Content, out.ToolCalls = foldContent(mr)
	return out, nil
}

// foldContent turns the response blocks into text plus CLIENT tool calls.
//
// ⚠️ A named function rather than a loop inside Chat, so a test can drive the
// real thing. The first version of the test for this reimplemented the switch
// inline and therefore proved nothing about the code — a mutation that turned
// server_tool_use into a client tool call passed it.
//
// ⚠️ server_tool_use and its result are deliberately DROPPED. The provider
// already ran the tool and already used the answer — the text blocks in this
// same response cite it. Surfacing them as ToolCalls would send the agent loop
// off to execute a tool it does not implement, fail, and report that failure
// back to the model as if the search had gone wrong.
//
// They are NAMED here rather than left to the default, because a silent drop and
// an unhandled block look identical until the day a new block type needs
// handling.
func foldContent(mr msgResp) (string, []ai.ToolCall) {
	var sb strings.Builder
	var calls []ai.ToolCall
	for _, b := range mr.Content {
		switch b.Type {
		case "text":
			sb.WriteString(b.Text)
		case "tool_use":
			calls = append(calls, ai.ToolCall{ID: b.ID, Name: b.Name, Arguments: string(b.Input)})
		case "server_tool_use", "web_search_tool_result":
		}
	}
	return sb.String(), calls
}

func (p *provider) ChatStream(ctx context.Context, req ai.ChatRequest) (<-chan ai.Chunk, error) {
	resp, err := p.post(ctx, p.buildReq(req, true))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: http %d: %s", resp.StatusCode, truncate(data))
	}

	out := make(chan ai.Chunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var inUsage ai.Usage // message_start carries the prompt half
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var ev struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
						CacheRead   int `json:"cache_read_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &ev) != nil {
				continue
			}
			switch ev.Type {
			case "message_start":
				inUsage.PromptTokens = ev.Message.Usage.InputTokens
				inUsage.CachedTokens = ev.Message.Usage.CacheRead
			case "content_block_delta":
				if ev.Delta.Text != "" {
					select {
					case out <- ai.Chunk{ContentDelta: ev.Delta.Text}:
					case <-ctx.Done():
						return
					}
				}
			case "message_delta":
				// message_delta carries the completion half of the accounting;
				// pair it with the prompt half from message_start.
				if ev.Usage.OutputTokens > 0 {
					inUsage.CompletionTokens = ev.Usage.OutputTokens
					u := inUsage
					select {
					case out <- ai.Chunk{Usage: &u}:
					case <-ctx.Done():
						return
					}
				}
			case "message_stop":
				select {
				case out <- ai.Chunk{Done: true}:
				case <-ctx.Done():
				}
				return
			}
		}
		if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case out <- ai.Chunk{Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

func truncate(b []byte) string {
	const max = 400
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// maxCacheBreakpoints is Anthropic's hard limit per request. Exceeding it is a
// 400, not a degradation.
const maxCacheBreakpoints = 4

// systemBlocks renders the system turns, marking at most two of them cacheable.
//
// # ⚠️ Which ones, and why the obvious answer is backwards
//
// A breakpoint caches THE WHOLE PREFIX UP TO ITSELF, not the block it sits on.
// So the instinct — "keep the first N" — throws away the only breakpoint that
// covers everything: the LAST system block's prefix includes the tools and every
// system turn before it. Keeping the first four and dropping the rest would raise
// the cap and make caching WORSE, silently, visible only as
// `usage.cache_read_input_tokens` drifting down.
//
// So: first and last, nothing in between.
//
//   - last  — the big one. Its prefix is tools + all system turns.
//   - first — the fallback read point. The rolling summary is injected as its own
//     system turn and changes every time the window compresses, so a single
//     breakpoint at the end misses on every compression; the first block still
//     covers the L1 boundaries and the persona, which never move.
//
// Middle blocks earn nothing: a shorter prefix, and each breakpoint costs a
// 1.25× write of its own.
//
// # ⚠️ It used to mark EVERY block, and the ceiling was not two
//
// The old code put a breakpoint on each system turn. That reads as "at most two"
// from the call sites this repository controls, and it was not: a client could
// push `role: "system"` rows into a thread (see the note on safeRole in
// internal/server/handlers_ai_companion.go), and they arrived here as system
// turns. Fifty of them was a 400 on every request in that thread — with the
// number of breakpoints chosen by the caller.
//
// Both halves are fixed; this is the half that holds even if the other regresses.
func systemBlocks(systems []string) []map[string]any {
	mark := map[int]bool{0: true, len(systems) - 1: true}
	blocks := make([]map[string]any, 0, len(systems))
	used := 0
	for i, s := range systems {
		b := map[string]any{"type": "text", "text": s}
		if mark[i] && used < maxCacheBreakpoints {
			b["cache_control"] = map[string]any{"type": "ephemeral"}
			used++
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// RunsServerTool reports the provider-executed tools this format handles.
//
// ⚠️ A whitelist of tool TYPES, not a blanket yes. Each one needs two things
// here — a serialisation shape in buildReq and its result frames dropped in the
// response parser — and "we handle web search" does not imply "we handle code
// execution". A blanket true would turn the next unsupported tool type into a
// 400 on every request in that deployment.
//
// ⚠️ Streaming is the reason there is only one entry today: ChatStream handles
// content_block_delta / message_stop and would surface nothing for a server
// tool's frames. That is fine for web search — the text deltas carry the answer
// — and would not be for a tool whose output the reader needs to see.
func (p *provider) RunsServerTool(toolType string) bool {
	return toolType == "web_search_20250305"
}
