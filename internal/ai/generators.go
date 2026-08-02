package ai

import "context"

// Capabilities that live on their own endpoints.
//
// Image generation, speech synthesis, transcription and embeddings are not
// chat. Every vendor puts them on separate routes with separate request shapes,
// and a model that only draws has no Chat to implement. Adding them to
// AIProvider would force four no-op methods onto every wire format and make
// "does this thing speak?" unanswerable except by calling it.
//
// So each is an optional interface, discovered by assertion — the same shape as
// ToolStreamer and PartCarrier. A catalog entry may implement none, one, or
// several: the majors serve all of these from one API key, while a dedicated
// image endpoint implements exactly one.
//
// These are the *foundation*. Nothing here picks a vendor, a voice or a
// resolution; that is configuration, and configuration can wait. What cannot
// wait is the shape, because it decides whether adding a modality later is a
// package or a migration.

// GeneratedMedia is one produced artefact.
//
// Bytes rather than a blob.Ref: this package must not depend on storage, and the
// caller is the one that knows whether the result should be persisted, streamed
// straight to a channel, or dropped. OpenAI's image endpoint returns base64 only
// (the URL option is gone), so bytes are what actually arrives regardless.
type GeneratedMedia struct {
	MIME string
	Data []byte
	// Text is whatever the provider said about what it made — a revised prompt,
	// a transcript's language guess. Providers that say nothing leave it empty.
	Text  string
	Usage Usage
}

// ImageRequest asks for a picture.
type ImageRequest struct {
	Prompt string
	// Count is how many variants. Zero means one.
	Count int
	// Size is a vendor-specific hint like "1024x1024". Deliberately a string:
	// every vendor has its own allowed set, and an enum here would have to be
	// updated on their schedule rather than ours.
	Size string
	// Reference lets a request edit or vary an existing image. Empty means
	// generate from text alone.
	Reference *ContentPart
}

// ImageGenerator draws.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, req ImageRequest) ([]GeneratedMedia, error)
}

// SpeechRequest asks for audio of some text.
type SpeechRequest struct {
	Text string
	// Voice is a vendor-specific identifier. Same reasoning as ImageRequest.Size.
	Voice string
	// Format is the container wanted ("mp3", "opus", …). Empty lets the provider
	// choose, which matters because a chat channel usually accepts exactly one.
	Format string
	// Locale is a hint, not a command: a synthesiser that does not have the
	// language should speak it as best it can rather than refuse. A schedule
	// reminder in the wrong accent is still a reminder.
	Locale string
}

// SpeechSynthesizer speaks.
type SpeechSynthesizer interface {
	Synthesize(ctx context.Context, req SpeechRequest) (GeneratedMedia, error)
}

// TranscriptionRequest is audio to be turned into text.
type TranscriptionRequest struct {
	Audio ContentPart
	// Locale is a hint that improves accuracy when known; empty asks the provider
	// to detect.
	Locale string
}

// Transcript is recognised speech.
type Transcript struct {
	Text string
	// Locale the provider detected, when it says.
	Locale string
	Usage  Usage
}

// Transcriber listens.
type Transcriber interface {
	Transcribe(ctx context.Context, req TranscriptionRequest) (Transcript, error)
}

// Embedder turns text into vectors.
//
// Included in the foundation even though nothing needs it yet, because its
// absence is what forces every "find similar" question to be answered by
// full-text search or by asking a model — and both of those are decisions that
// get baked into callers and are expensive to undo.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, Usage, error)
}

// Discovery helpers. Callers assert through these rather than inline so the
// "not supported" answer is one shape everywhere and shows up in a grep.

func AsImageGenerator(p AIProvider) (ImageGenerator, bool) {
	g, ok := p.(ImageGenerator)
	return g, ok
}

func AsSpeechSynthesizer(p AIProvider) (SpeechSynthesizer, bool) {
	s, ok := p.(SpeechSynthesizer)
	return s, ok
}

func AsTranscriber(p AIProvider) (Transcriber, bool) {
	tr, ok := p.(Transcriber)
	return tr, ok
}

func AsEmbedder(p AIProvider) (Embedder, bool) {
	e, ok := p.(Embedder)
	return e, ok
}
