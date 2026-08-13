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

// PartType distinguishes multimodal content parts. It mirrors Modality one for
// one — the alias exists because the old two-value form is threaded through the
// vision pipeline and the three formats, and renaming it there buys nothing.
type PartType = Modality

const (
	PartText     = ModalityText
	PartImage    = ModalityImage
	PartAudio    = ModalityAudio
	PartDocument = ModalityDocument
	PartVideo    = ModalityVideo
)

// ContentPart is one piece of a (possibly multimodal) message.
//
// One struct for every modality rather than a type per kind: the formats all
// serialise these the same way (a discriminator plus bytes plus a MIME type),
// and a per-kind hierarchy would make "iterate the parts and encode each one"
// into a type switch in three places that each has to grow whenever a modality
// is added.
//
// Exactly one carriage field is set. Which one a caller may use depends on the
// provider — see CarriagesFor — because the difference is real: a 30 MB PDF
// cannot be inlined, and several gateways accept documents *only* as a
// pre-uploaded file id.
type ContentPart struct {
	Type PartType
	Text string // Type == PartText

	// MIME describes the bytes, whatever carries them ("image/png",
	// "application/pdf", "audio/ogg"). Required for every non-text part: the
	// formats need it, and guessing from magic bytes here would overrule the
	// caller's own validation.
	MIME string

	// Data is raw base64 with no `data:` prefix (CarriageInline).
	//
	// Named Data rather than the old ImageBase64 because it is no longer only
	// images. There were five references in the whole repo, so renaming outright
	// beat keeping a compatibility accessor that would let two names for one
	// value drift apart.
	Data string

	// URL the provider should fetch (CarriageURL). Also how an object-store file
	// bus hands over a signed URL instead of proxying bytes through Daycore.
	URL string

	// FileID is a handle to bytes already uploaded to the provider
	// (CarriageFileID).
	FileID string

	// Name is the original filename where there was one. Providers show it to
	// the model for documents, and it is often the only clue about what a file
	// is for.
	Name string
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
	Content   string
	ToolCalls []ToolCall
	// Parts carries non-text output — a drawn image, synthesised speech, or a
	// provider's structured content blocks. Content stays the plain-text
	// rendering so that every existing caller keeps working and only the ones
	// that care about pixels have to look further.
	Parts        []ContentPart
	FinishReason string
	// Usage is what the call cost. Every provider returns it and all three
	// formats parse it into here (streaming callers see it on the final Chunk
	// instead). Zero means the format did not report it, not that the call was
	// free.
	Usage Usage
}

// Usage is the token accounting a provider reports.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	// CachedTokens is the prompt portion served from the provider's cache. It is
	// the only direct evidence that prompt caching is working — the alternative
	// is inferring it from latency, which is noise.
	CachedTokens int
	// ReasoningTokens is billed but not shown, so a model that thinks a lot looks
	// cheap by output length and is not.
	ReasoningTokens int
}

// Chunk is one streamed delta. Done=true (or a closed channel) ends the stream.
type Chunk struct {
	ContentDelta   string
	ReasoningDelta string
	ToolCallDelta  *ToolCallDelta
	FinishReason   string // "stop" | "tool_calls" | "length"; non-empty only on the last data frame
	// Usage arrives at most once, on a terminal frame (openai sends it after
	// the last content chunk when asked, anthropic splits it across
	// message_start/message_delta, ollama puts it on the done line). Nil on
	// every other frame; nil at the end means the provider did not report it.
	Usage *Usage
	Err   error
	Done  bool
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
	// Vision is "this model reads images". Kept as its own bool because three
	// call sites branch on it and because it is what an operator writes in
	// models.yaml; In/Out below are the general form and Vision is folded into
	// them by LoadCatalog.
	Vision bool
	Tools  bool
	// ⚠️ There used to be Stream and Thinking here. Both were declared in
	// models.yaml and read by nothing — the same shape as DeepseekSearch below,
	// and removed for the same reason.
	//
	// Neither could have worked where it sat. Whether a format's ChatStream
	// carries TOOL CALLS is a property of our implementation, not of the model,
	// so it is the ToolStreamer optional interface (see below) — an operator
	// editing models.yaml has no way to know whether our anthropic parser handles
	// tool_use yet. And reasoning is turned on by `extra_body` (openai format's
	// mergeExtraBody) and read back through Usage.ReasoningTokens; a bool beside
	// it changed nothing either way.
	ContextWindow int

	// In and Out are the modalities this model accepts and produces.
	//
	// Two sets rather than one because they are genuinely independent: a model
	// that reads images may not draw them, one that speaks may not listen, and
	// the common case today — text in, text out, images in — is expressible only
	// if the directions are separate.
	//
	// Empty means "text only", so a models.yaml entry written before modalities
	// existed keeps meaning what it meant.
	In  Modalities
	Out Modalities
}

// Accepts reports whether the model takes this modality as input. Text is always
// accepted: a model that cannot read text is not one this repo can drive.
func (c Capabilities) Accepts(m Modality) bool {
	if m == ModalityText {
		return true
	}
	if m == ModalityImage && c.Vision {
		return true
	}
	return c.In.Has(m)
}

// Produces reports whether the model can emit this modality.
func (c Capabilities) Produces(m Modality) bool {
	if m == ModalityText {
		return true
	}
	return c.Out.Has(m)
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
