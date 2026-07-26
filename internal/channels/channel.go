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

// Channel is implemented by every messaging platform adapter.
type Channel interface {
	Name() string // e.g. "onebot"
	Send(ctx context.Context, externalID string, msg string) error
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
