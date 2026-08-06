// Package ollama implements Ollama's native /api/chat wire format (base64
// images for vision via llava etc., newline-delimited JSON streaming, optional
// tools). Registered as format "ollama". For OpenAI-compatible Ollama usage,
// the "openai" format pointed at http://host:11434/v1 also works.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/ai"
)

func init() { ai.RegisterFormat("ollama", New) }

// New builds an Ollama native provider.
func New(cfg ai.ModelConfig) (ai.AIProvider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	return &provider{
		cfg:      cfg,
		http:     &http.Client{},
		endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/api/chat",
	}, nil
}

type provider struct {
	cfg      ai.ModelConfig
	http     *http.Client
	endpoint string
}

func (p *provider) Model() string                 { return p.cfg.Model }
func (p *provider) Capabilities() ai.Capabilities { return p.cfg.Caps }

type chatReq struct {
	Model    string     `json:"model"`
	Messages []wireMsg  `json:"messages"`
	Stream   bool       `json:"stream"`
	Format   string     `json:"format,omitempty"`
	Options  *options   `json:"options,omitempty"`
	Tools    []wireTool `json:"tools,omitempty"`
}

type options struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
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
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Images    []string       `json:"images,omitempty"`
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
}

type wireToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

func (p *provider) buildReq(req ai.ChatRequest, stream bool) chatReq {
	out := chatReq{Model: p.cfg.Model, Stream: stream}
	if req.JSONMode {
		out.Format = "json"
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.cfg.MaxTokens
	}
	if req.Temperature != 0 || maxTokens != 0 {
		out.Options = &options{Temperature: req.Temperature, NumPredict: maxTokens}
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, wireTool{Type: "function", Function: wireFn{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	for _, m := range req.Messages {
		wm := wireMsg{Role: string(m.Role)}
		if len(m.Parts) > 0 {
			var text strings.Builder
			for _, pt := range m.Parts {
				if pt.Type == ai.PartImage {
					wm.Images = append(wm.Images, pt.Data)
				} else {
					text.WriteString(pt.Text)
				}
			}
			wm.Content = text.String()
		} else {
			wm.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			wtc := wireToolCall{}
			wtc.Function.Name = tc.Name
			wtc.Function.Arguments = args
			wm.ToolCalls = append(wm.ToolCalls, wtc)
		}
		out.Messages = append(out.Messages, wm)
	}
	return out
}

func (p *provider) post(ctx context.Context, body chatReq) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
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

type chatResp struct {
	Message struct {
		Content   string         `json:"content"`
		ToolCalls []wireToolCall `json:"tool_calls"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
	// Ollama reports accounting only on the terminal frame of a stream and on
	// the non-streaming response; both land here.
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

func (r *chatResp) usage() ai.Usage {
	return ai.Usage{PromptTokens: r.PromptEvalCount, CompletionTokens: r.EvalCount}
}

func (p *provider) Chat(ctx context.Context, req ai.ChatRequest) (*ai.ChatResponse, error) {
	resp, err := p.post(ctx, p.buildReq(req, false))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama: http %d: %s", resp.StatusCode, truncate(data))
	}
	var cr chatResp
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("ollama: decode: %w", err)
	}
	if cr.Error != "" {
		return nil, fmt.Errorf("ollama: %s", cr.Error)
	}
	out := &ai.ChatResponse{Content: cr.Message.Content}
	out.Usage = cr.usage()
	for i, tc := range cr.Message.ToolCalls {
		args, _ := json.Marshal(tc.Function.Arguments)
		out.ToolCalls = append(out.ToolCalls, ai.ToolCall{
			ID: "call_" + strconv.Itoa(i), Name: tc.Function.Name, Arguments: string(args),
		})
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
		return nil, fmt.Errorf("ollama: http %d: %s", resp.StatusCode, truncate(data))
	}

	out := make(chan ai.Chunk)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var ev chatResp
			if json.Unmarshal([]byte(line), &ev) != nil {
				continue
			}
			if ev.Message.Content != "" {
				select {
				case out <- ai.Chunk{ContentDelta: ev.Message.Content}:
				case <-ctx.Done():
					return
				}
			}
			if ev.Done {
				u := ev.usage()
				select {
				case out <- ai.Chunk{Done: true, Usage: &u}:
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
