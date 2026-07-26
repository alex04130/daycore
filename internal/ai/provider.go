// Package ai defines a provider-neutral chat/vision/tool-calling contract and a
// format registry. Concrete wire formats (OpenAI, Anthropic, Ollama, …) live in
// ./formats/* and self-register via RegisterFormat, so adding a new vendor API
// is: implement AIProvider in a new sub-package + one RegisterFormat call +
// recompile. Which *models* exist is data-driven (see models.go / config).
package ai

import "context"

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// PartType distinguishes multimodal content parts.
type PartType string

const (
	PartText  PartType = "text"
	PartImage PartType = "image"
)

// ContentPart is one piece of a (possibly multimodal) message.
type ContentPart struct {
	Type        PartType
	Text        string
	ImageBase64 string // raw base64 (no data: prefix)
	ImageMime   string // e.g. "image/png"
}

// ToolDef describes a function the model may call. Parameters is a JSON Schema
// for client-side tools. When ServerSide is non-empty (e.g. "web_search_20250305"),
// the tool is executed by the model provider and Parameters is ignored — the
// provider serialises it differently (a "type"-based tool block with max_uses).
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
	ServerSide  string // provider-executed tool type (e.g. "web_search_20250305"); empty = client-side
}

// ToolCall is a model's request to invoke a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // raw JSON arguments
}

// Message is one turn. Use Parts for multimodal input; otherwise Content.
type Message struct {
	Role       Role
	Content    string
	Parts      []ContentPart
	ToolCalls  []ToolCall // assistant → tool requests
	ToolCallID string     // role=tool → which call this result answers
	Name       string     // role=tool → tool name
}

// ChatRequest is a provider-neutral request.
type ChatRequest struct {
	Messages    []Message
	Tools       []ToolDef
	Temperature float64
	MaxTokens   int
	JSONMode    bool // ask for a strict JSON object (providers that can't, ignore)
	Stop        []string
	CacheKey    string // if non-empty, set "prompt-cache-key" header
}

// ChatResponse is a non-streaming reply.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// Chunk is one streamed delta. Done=true (or a closed channel) ends the stream.
type Chunk struct {
	ContentDelta   string
	ReasoningDelta string
	ToolCallDelta  *ToolCallDelta
	FinishReason   string // "stop" | "tool_calls" | "length"; non-empty only on the last data frame
	Err            error
	Done           bool
}

// ToolCallDelta is one streamed fragment of a tool call. The first fragment for
// an Index carries ID/Name; later fragments only append ArgsDelta.
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	ArgsDelta string
}

// Capabilities advertises what a model supports (declared in the catalog config).
type Capabilities struct {
	Vision         bool
	Tools          bool
	Stream         bool
	Thinking       bool
	ContextWindow  int
	DeepseekSearch bool // enable native web_search server-side tool (DeepSeek Anthropic endpoint)
}

// AIProvider is the contract every wire-format implementation satisfies.
type AIProvider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
	Capabilities() Capabilities
	Model() string // upstream model id, for logging
}
