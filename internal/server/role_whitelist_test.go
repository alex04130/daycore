package server

import (
	"testing"

	"daycore/internal/ai"
)

// A client must not be able to put a `system` turn into its own LLM context.
//
// # ⚠️ The chain that made this reachable
//
//  1. POST /api/companion-history takes `[]domain.Message` with a free-form
//     `Role` and stores it verbatim (handlers_companion_history.go).
//  2. GET /api/chat/threads auto-imports that into chat_messages, again
//     verbatim (handlers_chat.go).
//  3. buildCompanionMessages read it back with `ai.Role(dbMsgs[i].Role)` — no
//     whitelist — straight into ChatRequest.Messages.
//
// So "the role in the database" is client input wearing a database's clothes.
// The ANONYMOUS path had defended against exactly this since it was written,
// with a comment saying why; the threaded path — the one every frontend uses —
// did not.
//
// A second consequence, in a different package: those turns arrive at the
// anthropic format as system blocks, and it used to put a cache breakpoint on
// every one. Anthropic allows four per request, so a thread with fifty injected
// system rows was a 400 on every request in it. Both halves are fixed; this
// tests the one that stops the bad row from being a system turn at all.
func TestOnlyAssistantSurvivesAsANonUserRole(t *testing.T) {
	for _, raw := range []string{
		"system",     // the instruction channel — the one that matters
		"tool",       // fabricating a tool result the model treats as ground truth
		"user",       // already a user turn
		"",           // legacy rows predate the column
		"SYSTEM",     // case is not a bypass
		"assistant ", // nor is whitespace
		"developer",
	} {
		if got := safeRole(raw); got != ai.RoleUser {
			t.Errorf("safeRole(%q) = %q, want %q — anything not exactly `assistant` "+
				"comes from a client and must land as a user turn", raw, got, ai.RoleUser)
		}
	}
	if got := safeRole(string(ai.RoleAssistant)); got != ai.RoleAssistant {
		t.Errorf("safeRole(%q) = %q — the one role that must survive did not",
			ai.RoleAssistant, got)
	}
}
