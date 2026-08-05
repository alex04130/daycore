// Package openai implements the OpenAI Chat Completions wire format. It covers
// OpenAI itself, DeepSeek, and any OpenAI-compatible endpoint (incl. Ollama's
// /v1). Registered as format "openai".
package openai

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

func init() { ai.RegisterFormat("openai", New) }

// New builds an OpenAI-compatible provider.
func New(cfg ai.ModelConfig) (ai.AIProvider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	return &provider{
		cfg:      cfg,
		http:     &http.Client{}, // no client timeout: streaming relies on ctx
		endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions",
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

type chatReq struct {
	Model          string      `json:"model"`
	Messages       []wireMsg   `json:"messages"`
	Temperature    float64     `json:"temperature,omitempty"`
	MaxTokens      int         `json:"max_tokens,omitempty"`
	Stream         bool        `json:"stream,omitempty"`
	Stop           []string    `json:"stop,omitempty"`
	Tools          []wireTool  `json:"tools,omitempty"`
	ResponseFormat *respFormat `json:"response_format,omitempty"`
	// prompt_cache_key is a top-level BODY field, not a header. It shipped as
	// `json:"-"` plus a Header.Set, which sends nothing OpenAI reads.
	//
	// Zero impact today — nothing in the repo sets ChatRequest.CacheKey, so the
	// wrong encoding never went out. That is worth saying plainly rather than
	// filing it as a leak: a field having a bug and a field having callers are
	// different things, and conflating them is how a three-line fix gets sold as
	// an incident.
	CacheKey string `json:"prompt_cache_key,omitempty"`

	// StreamOptions asks for the terminal usage chunk. Without it a streamed
	// call is unaccountable — the AICallLog would have to record zero for the
	// busiest call path in the product (the companion loop).
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// wireUsage is the OpenAI usage block, shared by Chat responses and the
// terminal stream chunk.
type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	PromptDetails    *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u *wireUsage) toAI() ai.Usage {
	if u == nil {
		return ai.Usage{}
	}
	out := ai.Usage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens}
	if u.PromptDetails != nil {
		out.CachedTokens = u.PromptDetails.CachedTokens
	}
	if u.CompletionDetails != nil {
		out.ReasoningTokens = u.CompletionDetails.ReasoningTokens
	}
	return out
}

type respFormat struct {
	Type string `json:"type"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function wireFn `json:"function"`
}

type wireFn struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type wireMsg struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type wireToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function wireToolCallFn `json:"function"`
}

type wireToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imagePart struct {
	Type     string   `json:"type"`
	ImageURL imageURL `json:"image_url"`
}

type imageURL struct {
	URL string `json:"url"`
}

func (p *provider) buildReq(req ai.ChatRequest, stream bool) chatReq {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.cfg.MaxTokens
	}
	out := chatReq{
		Model:       p.cfg.Model,
		Messages:    toWireMessages(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
		Stream:      stream,
		Stop:        req.Stop,
		CacheKey:    req.CacheKey,
	}
	if stream {
		out.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, wireTool{Type: "function", Function: wireFn{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	if req.JSONMode {
		out.ResponseFormat = &respFormat{Type: "json_object"}
	}
	return out
}

func toWireMessages(msgs []ai.Message) []wireMsg {
	out := make([]wireMsg, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMsg{Role: string(m.Role), ToolCallID: m.ToolCallID, Name: m.Name}
		switch {
		case len(m.Parts) > 0:
			parts := make([]any, 0, len(m.Parts))
			for _, pt := range m.Parts {
				if pt.Type == ai.PartImage {
					parts = append(parts, imagePart{Type: "image_url", ImageURL: imageURL{
						URL: fmt.Sprintf("data:%s;base64,%s", pt.MIME, pt.Data),
					}})
				} else {
					parts = append(parts, textPart{Type: "text", Text: pt.Text})
				}
			}
			wm.Content = parts
		case m.Content != "":
			wm.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID: tc.ID, Type: "function",
				Function: wireToolCallFn{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		out = append(out, wm)
	}
	return out
}

// ─── requests ────────────────────────────────────────────────────────────────

func (p *provider) post(ctx context.Context, body chatReq) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if len(p.cfg.ExtraBody) > 0 {
		if buf, err = mergeExtraBody(buf, p.cfg.ExtraBody); err != nil {
			return nil, err
		}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	return p.http.Do(httpReq)
}

// mergeExtraBody shallow-merges extra top-level keys into an encoded request
// without overriding keys the request already sets.
func mergeExtraBody(body []byte, extra map[string]any) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	for k, v := range extra {
		if _, exists := m[k]; exists {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		m[k] = raw
	}
	return json.Marshal(m)
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *wireUsage `json:"usage"`
}

func (p *provider) Chat(ctx context.Context, req ai.ChatRequest) (*ai.ChatResponse, error) {
	resp, err := p.post(ctx, p.buildReq(req, false))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai: http %d: %s", resp.StatusCode, truncate(data))
	}
	var cr chatResp
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("openai: decode: %w", err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("openai: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return nil, errors.New("openai: empty choices")
	}
	ch := cr.Choices[0]
	out := &ai.ChatResponse{Content: ch.Message.Content, FinishReason: ch.FinishReason}
	out.Usage = cr.Usage.toAI()
	for _, tc := range ch.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ai.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return out, nil
}

func (p *provider) ChatStream(ctx context.Context, req ai.ChatRequest) (<-chan ai.Chunk, error) {
	resp, err := p.post(ctx, p.buildReq(req, true))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: http %d: %s", resp.StatusCode, truncate(data))
	}

	out := make(chan ai.Chunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				select {
				case out <- ai.Chunk{Done: true}:
				case <-ctx.Done():
				}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content"`
						ToolCalls        []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage *wireUsage `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			// The terminal usage chunk (asked for via stream_options) arrives
			// with an empty choices array, so it must be handled before the
			// no-choices skip.
			if chunk.Usage != nil {
				u := chunk.Usage.toAI()
				select {
				case out <- ai.Chunk{Usage: &u}:
				case <-ctx.Done():
					return
				}
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			ch := chunk.Choices[0]
			var frames []ai.Chunk
			if ch.Delta.Content != "" {
				frames = append(frames, ai.Chunk{ContentDelta: ch.Delta.Content})
			}
			if ch.Delta.ReasoningContent != "" {
				frames = append(frames, ai.Chunk{ReasoningDelta: ch.Delta.ReasoningContent})
			}
			for _, tc := range ch.Delta.ToolCalls {
				frames = append(frames, ai.Chunk{ToolCallDelta: &ai.ToolCallDelta{
					Index: tc.Index, ID: tc.ID, Name: tc.Function.Name, ArgsDelta: tc.Function.Arguments,
				}})
			}
			if ch.FinishReason != "" { // may arrive on an otherwise-empty frame
				if len(frames) == 0 {
					frames = append(frames, ai.Chunk{})
				}
				frames[len(frames)-1].FinishReason = ch.FinishReason
			}
			for _, f := range frames {
				select {
				case out <- f:
				case <-ctx.Done():
					return
				}
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

// StreamsToolCalls reports true: this format parses delta.tool_calls and emits
// ToolCallDelta per index. It is the only one that does.
func (p *provider) StreamsToolCalls() bool { return true }
