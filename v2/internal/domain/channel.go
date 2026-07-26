package domain

import (
	"context"
	"time"
)

// ChannelBinding links a session to an external messaging platform account.
type ChannelBinding struct {
	ID          string     `json:"id"`
	SessionID   string     `json:"sessionId"`
	Channel     string     `json:"channel"`     // "onebot" | "wechat" | "feishu" | "telegram"
	ExternalID  string     `json:"externalId"`  // platform user id
	DisplayName string     `json:"displayName"` // platform nickname
	Metadata    string     `json:"metadata"`    // JSON blob (avatar URL etc.)
	VerifiedAt  *time.Time `json:"verifiedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// ChannelBindingRepository manages cross-platform account bindings.
type ChannelBindingRepository interface {
	ListBySession(ctx context.Context, sessionID string) ([]ChannelBinding, error)
	// ListAllVerified returns every verified binding across all sessions (used to
	// re-schedule proactive jobs on startup).
	ListAllVerified(ctx context.Context) ([]ChannelBinding, error)
	GetByChannelAndExternal(ctx context.Context, channel, externalID string) (*ChannelBinding, error)
	Create(ctx context.Context, b *ChannelBinding) (*ChannelBinding, error)
	// GetPendingByToken finds an unverified binding row by its one-time token
	// (the token is stored in external_id until the binding is promoted).
	GetPendingByToken(ctx context.Context, channel, token string) (*ChannelBinding, error)
	// Promote turns a pending token row into the real verified binding: it sets
	// the real external id, the display name, and verified_at.
	Promote(ctx context.Context, id, externalID, displayName string) error
	// ExpirePending deletes unverified token rows past the 10-minute TTL.
	ExpirePending(ctx context.Context) (int, error)
	Delete(ctx context.Context, sessionID, channel string) error
}
