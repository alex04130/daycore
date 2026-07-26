package server

import (
	"context"
	"fmt"
	"strings"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

// Token estimation: ~4 chars = 1 token for English, ~2 chars = 1 token for CJK.
// We use a conservative 3:1 ratio so we never underestimate.
func estimateTokens(msgs []ai.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)
	}
	return n / 3
}

// compressHistory summarises the oldest half of messages using the flash model
// (or default chat if no flash is configured). Returns the summary string and
// extracted open loops as memory facts.
func (s *Server) compressHistory(ctx context.Context, msgs []ai.Message) (summary string, openLoops []string, _ error) {
	if len(msgs) < 6 {
		// Not enough to compress.
		return "", nil, nil
	}
	half := len(msgs) / 2
	toSummarise := msgs[:half]
	// Skip leading system messages: summarising the giant layered system prompt
	// wastes tokens and risks leaking its instructions into the summary.
	for len(toSummarise) > 0 && toSummarise[0].Role == ai.RoleSystem {
		toSummarise = toSummarise[1:]
	}

	// Build a compact text representation of the conversation segment.
	var sb strings.Builder
	for _, m := range toSummarise {
		sb.WriteString(string(m.Role))
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}

	sys := summariseSystemPrompt()
	provider := s.catalog.DefaultChat()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sys},
			{Role: ai.RoleUser, Content: sb.String()},
		},
		Temperature: 0.1,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", nil, fmt.Errorf("summarise: %w", err)
	}
	summary = strings.TrimSpace(resp.Content)
	// Extract open loops: lines starting with "[开环]" or "[open]".
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[开环]") || strings.HasPrefix(line, "[open]") {
			// TrimPrefix, not TrimLeft: TrimLeft treats the argument as a cutset
			// and would eat leading letters of the text (e.g. "[open] pack" → "ack").
			clean := strings.TrimPrefix(line, "[开环]")
			clean = strings.TrimPrefix(clean, "[open]")
			clean = strings.TrimSpace(clean)
			if clean != "" {
				openLoops = append(openLoops, clean)
			}
		}
	}
	return summary, openLoops, nil
}

// maybeCompress checks if the conversation has grown past the compression
// threshold and, if so, compresses the oldest half and saves the summary.
func (s *Server) maybeCompress(ctx context.Context, sid, threadID, locale, tz, name string, messages []ai.Message) []ai.Message {
	if threadID == "" {
		return messages // anonymous sessions don't have thread storage
	}
	tokens := estimateTokens(messages)
	threshold := s.catalog.DefaultChat().Capabilities().ContextWindow * 60 / 100
	if threshold <= 0 {
		threshold = 39000 // default for 65536 window
	}
	if tokens <= threshold {
		return messages
	}

	summary, openLoops, err := s.compressHistory(ctx, messages)
	if err != nil {
		s.log.Error("context compression", "err", err)
		return messages // don't break the conversation on compression failure
	}

	// Save the summary.
	_, _ = s.store.Chats().UpdateThread(ctx, sid, threadID, domain.ChatThreadUpdate{Summary: &summary})

	// Save open loops as auto-memory facts, de-duplicated against what's already
	// remembered so repeated compressions don't spam identical facts.
	existing, _ := s.store.Memory().ListFacts(ctx, sid)
	seenFacts := map[string]bool{}
	for _, f := range existing {
		seenFacts[f.Fact] = true
	}
	for _, loop := range openLoops {
		if seenFacts[loop] {
			continue
		}
		seenFacts[loop] = true
		_, _ = s.store.Memory().AddFact(ctx, &domain.MemoryFact{
			SessionID: sid,
			Fact:      loop,
			Source:    "auto",
			Type:      "open_loop",
		})
	}

	// Rebuild messages: system prompt + summary + recent half. Drop leading
	// non-user messages so the first turn after the system block is a user
	// message (strict providers reject otherwise) and no orphan tool message
	// (whose assistant tool_calls got split off) survives the cut.
	half := len(messages) / 2
	recent := messages[half:]
	for len(recent) > 0 && recent[0].Role != ai.RoleUser {
		recent = recent[1:]
	}
	sys, err := s.companionSystemPrompt(ctx, sid, locale, tz, name)
	if err != nil {
		return messages
	}
	compact := []ai.Message{
		{Role: ai.RoleSystem, Content: sys},
		{Role: ai.RoleSystem, Content: "[对话摘要]\n" + summary},
	}
	compact = append(compact, recent...)
	return compact
}

func summariseSystemPrompt() string {
	return `You are a conversation summariser. Summarise the following multi-turn agent conversation in Chinese. Output ONLY the summary, no extra text.

Format:
- 已确认事实：key facts the user confirmed
- 未完成事项：list any pending tasks or open questions. For each one, start the line with "[开环] " followed by a short statement
- 用户情绪基调：the user's emotional tone
- 已执行操作：brief list of operations performed (tool calls that changed state)

Keep it concise — under 500 characters total.`
}
