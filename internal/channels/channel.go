// Package channels defines the pluggable messaging channel framework.
// Each platform (QQ/OneBot, WeChat, Feishu, Telegram) implements the Channel
// interface. The Registry manages their lifecycle.
package channels

import (
	"context"
	"log/slog"
	"sync"
)

// InboundMsg is a message received from any external channel.
type InboundMsg struct {
	Channel     string // "onebot" | "wechat" | "feishu" | "telegram"
	ExternalID  string // platform-specific user id
	DisplayName string // user's display name on that platform
	Text        string
	Attachments []Attachment
}

// Attachment represents a file or image sent through a channel.
type Attachment struct {
	URL      string
	MimeType string
	Name     string
}

// Outbound is a message going to a platform.
//
// A struct rather than a string because on a chat platform the picture *is* the
// interface: there is no view to render a timetable into, so an assistant that
// can only emit text is a crippled one there. The protocol has always declared
// `features: {attachments: true}` — this is the Go side finally being able to
// express it.
type Outbound struct {
	Text        string
	Attachments []OutAttachment
	// ReplyTo threads the message where the platform supports it.
	ReplyTo string
}

// OutAttachment is one file going out.
//
// Bytes or a URL, never a blob.Ref: an adapter may be a separate process (see
// docs/specs/transport.md), and a reference into Daycore's file bus means
// nothing on the other side of that boundary. Resolving a ref into whichever
// form a given channel wants is the caller's job — the same split that keeps
// internal/ai free of storage.
type OutAttachment struct {
	Kind string // "image" | "audio" | "file"
	MIME string
	// Data is the bytes. Preferred for images on platforms that accept uploads.
	Data []byte
	// URL is used when the platform fetches rather than receives, or when the
	// file bus can sign a URL and save Daycore from proxying the bytes twice.
	URL  string
	Name string
}

// Text builds a plain outbound message. Most call sites want exactly this and
// should not have to write a struct literal to say so.
func Text(s string) Outbound { return Outbound{Text: s} }

// Features is what a platform can actually receive.
//
// The spec's phrasing is the rule: a capability declaration is not a
// suggestion. A channel that reports markdown:false must not be sent markdown,
// and one that reports Images:false must be given words instead of a picture —
// silently dropping the attachment would leave the user with a reply that
// refers to something they cannot see.
type Features struct {
	Images   bool
	Audio    bool
	Files    bool
	Markdown bool
	// MaxTextRunes is 0 when the platform does not say. Callers that split long
	// replies need it; QQ in particular truncates rather than rejecting.
	MaxTextRunes int
}

// Channel is implemented by every messaging platform adapter.
type Channel interface {
	Name() string // e.g. "onebot"
	Send(ctx context.Context, externalID string, msg Outbound) error
	// Features reports what this platform accepts. An adapter that cannot answer
	// should report text-only rather than guess upward: sending an image that
	// silently vanishes is worse than sending a sentence.
	Features() Features
	Start(ctx context.Context, inbound chan<- InboundMsg) error
	Stop() error
}

// Registry manages the lifecycle of all channels.
type Registry struct {
	channels map[string]Channel
	inbound  chan InboundMsg
	mu       sync.RWMutex
	log      *slog.Logger
}

// NewRegistry creates an empty channel registry.
func NewRegistry(log *slog.Logger) *Registry {
	return &Registry{
		channels: make(map[string]Channel),
		inbound:  make(chan InboundMsg, 256),
		log:      log,
	}
}

// Register adds a channel to the registry.
func (r *Registry) Register(ch Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[ch.Name()] = ch
	r.log.Info("channel registered", "name", ch.Name())
}

// StartAll starts all registered channels.
func (r *Registry) StartAll(ctx context.Context) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, ch := range r.channels {
		go func(name string, ch Channel) {
			if err := ch.Start(ctx, r.inbound); err != nil {
				r.log.Error("channel start failed", "name", name, "err", err)
			}
		}(name, ch)
	}
}

// StopAll stops all registered channels.
func (r *Registry) StopAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, ch := range r.channels {
		if err := ch.Stop(); err != nil {
			r.log.Error("channel stop failed", "name", name, "err", err)
		}
	}
}

// Inbound returns the unified inbound message channel.
func (r *Registry) Inbound() <-chan InboundMsg { return r.inbound }

// Channel returns a registered channel by name, or nil.
func (r *Registry) Channel(name string) Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[name]
}
