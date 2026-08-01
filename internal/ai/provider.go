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
	Vision        bool
	Tools         bool
	Stream        bool
	Thinking      bool
	ContextWindow int
}

// There used to be a DeepseekSearch flag here, declared in models.yaml and read
// by nothing: no format consulted it, no tool was registered from it, and it
// still shipped as `deepseek_search: true` on two models in the default config.
// A capability that is advertised and unimplemented is worse than a missing one —
// it is a claim. Vendor-native search comes back in batch F2, where
// docs/specs/provider-protocol.md decides which of its two shapes it is (a
// server-side tool that returns results, versus a prompt fragment that rides
// with the model).

// ToolStreamer marks a provider whose ChatStream carries tool calls.
//
// It is a property of the wire format, not of the model, so it is an optional
// interface rather than a Capabilities field: a Capabilities field would have to
// be declared in models.yaml, where an operator has no way of knowing whether
// our anthropic implementation happens to parse tool_use blocks yet.
//
// Only the openai format emits ToolCallDelta today. anthropic's ChatStream
// handles content_block_delta and message_stop and drops tool_use entirely;
// ollama's reads Message.Content only. Callers that need tool calls must route
// those through StreamViaChat — see its comment, which was written for exactly
// this and then never called.
type ToolStreamer interface {
	StreamsToolCalls() bool
}

// StreamsToolCalls reports whether p delivers tool calls over ChatStream. A
// provider that does not say is assumed not to: the failure mode of guessing
// wrong in that direction is one non-streamed round, and in the other direction
// it is every tool call silently vanishing.
func StreamsToolCalls(p AIProvider) bool {
	ts, ok := p.(ToolStreamer)
	return ok && ts.StreamsToolCalls()
}

// AIProvider is the contract every wire-format implementation satisfies.
type AIProvider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
	Capabilities() Capabilities
	Model() string // upstream model id, for logging
}
