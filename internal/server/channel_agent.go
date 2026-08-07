package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/channels"
)

// discardSink drops every agent frame; channel replies only need the final
// text runCompanionAgent returns.
type discardSink struct{}

func (discardSink) send(map[string]any) {}
func (discardSink) ping()               {}
func (discardSink) fail(string, string) {}

// HandleInbound processes one inbound channel message: resolve the bound
// session, run the companion agent, and reply through the same channel. Unbound
// senders are ignored (the adapter's ValidateBinding should already filter them;
// this is defense in depth).
func (s *Server) HandleInbound(ctx context.Context, reg *channels.Registry, msg channels.InboundMsg) {
	binding, err := s.store.ChannelBindings().GetByChannelAndExternal(ctx, msg.Channel, msg.ExternalID)
	if err != nil || binding == nil {
		return
	}
	answer := s.runChannelMessage(ctx, binding.SessionID, strings.TrimSpace(msg.Text))
	if strings.TrimSpace(answer) == "" {
		return
	}
	if ch := reg.Channel(msg.Channel); ch != nil {
		if err := ch.Send(ctx, msg.ExternalID, channels.Text(answer)); err != nil {
			s.log.Warn("channel reply failed", "channel", msg.Channel, "err", err)
		}
	}
}

// runChannelMessage runs the companion agent for a channel message and returns
// the assistant's final text, reusing runCompanionAgent with a discard SSE
// writer so the tool loop, server-side context, and safety prompts stay identical.
func (s *Server) runChannelMessage(ctx context.Context, sid, text string) string {
	if text == "" {
		return ""
	}
	sess, err := s.store.Sessions().Get(ctx, sid)
	if err != nil {
		return ""
	}
	locale := s.localePair(ctx, sid).Resolve(sess.Language, "")
	name := sess.AssistantName
	if name == "" {
		name = "Leo"
	}
	tz := s.sessionTimezone(ctx, sid)
	sys, err := s.companionSystemPrompt(ctx, sid, locale, tz, name)
	if err != nil {
		return ""
	}
	messages := []ai.Message{
		{Role: ai.RoleSystem, Content: sys},
		{Role: ai.RoleUser, Content: text},
	}
	// Minimal request: runCompanionAgent only reads it for the rate-limit key;
	// RemoteAddr=sid gives each session its own bucket.
	req := (&http.Request{RemoteAddr: sid, Header: http.Header{}}).WithContext(ctx)
	// interactive=false: a channel reply has no UI to answer a decision card
	// (previously the card frame was silently discarded and the agent waited
	// out the full timeout for an answer that could never arrive).
	return s.runCompanionAgent(ctx, discardSink{}, req, s.catalog.DefaultChat(), sid, locale, tz, messages, false)
}

// StartChannelBindingCleanup periodically deletes expired unverified binding
// tokens so stale rows don't accumulate. interval defaults to 10 minutes.
func (s *Server) StartChannelBindingCleanup(interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	s.everyTick("channel-binding expire", interval, func(ctx context.Context) {
		n, err := s.store.ChannelBindings().ExpirePending(ctx)
		if err != nil {
			// Was swallowed: `err == nil && n > 0` logged only successes, so a
			// cleanup that had been failing for weeks looked exactly like one with
			// nothing to do.
			s.log.Warn("channel-binding expire error", "err", err)
			return
		}
		if n > 0 {
			s.log.Info("expired pending channel tokens", "count", n)
		}
	})
}
