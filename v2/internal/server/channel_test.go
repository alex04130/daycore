package server

import (
	"context"
	"testing"

	"daycore/internal/domain"
)

// The full bind → verify flow was impossible before: verify looked up the token
// with a `verified_at IS NOT NULL` filter (never matches a fresh token) and
// never set the real external id. This exercises the fixed path end to end.
func TestChannelBindVerifyFlow(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()

	// bind: store an unverified token row (external_id = token).
	const token = "tok123abc456"
	if _, err := s.store.ChannelBindings().Create(ctx, &domain.ChannelBinding{
		SessionID: sid, Channel: "onebot", ExternalID: token,
	}); err != nil {
		t.Fatal(err)
	}

	// verify step 1: the token is found as a pending binding.
	pending, err := s.store.ChannelBindings().GetPendingByToken(ctx, "onebot", token)
	if err != nil {
		t.Fatalf("GetPendingByToken: %v", err)
	}
	if pending.SessionID != sid {
		t.Fatalf("pending session = %q, want %q", pending.SessionID, sid)
	}

	// verify step 2: promote to the real external id.
	if err := s.store.ChannelBindings().Promote(ctx, pending.ID, "qq-999", "Alice"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// the binding is now resolvable by the real external id and is verified.
	b, err := s.store.ChannelBindings().GetByChannelAndExternal(ctx, "onebot", "qq-999")
	if err != nil {
		t.Fatalf("GetByChannelAndExternal after promote: %v", err)
	}
	if b.SessionID != sid || b.VerifiedAt == nil {
		t.Fatalf("binding not verified/linked: session=%q verifiedAt=%v", b.SessionID, b.VerifiedAt)
	}

	// the raw token must no longer be a pending row.
	if _, err := s.store.ChannelBindings().GetPendingByToken(ctx, "onebot", token); err == nil {
		t.Fatal("token should not remain pending after promotion")
	}
}
