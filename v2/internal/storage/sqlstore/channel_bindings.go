package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type channelBindingRepo struct{ *Store }

func (r channelBindingRepo) ListBySession(ctx context.Context, sessionID string) ([]domain.ChannelBinding, error) {
	rows, err := r.query(ctx,
		`SELECT id, session_id, channel, external_id, display_name, metadata, verified_at, created_at
		 FROM channel_bindings WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ChannelBinding
	for rows.Next() {
		var b domain.ChannelBinding
		var verifiedAt sql.NullInt64
		var createdAt int64
		if err := rows.Scan(&b.ID, &b.SessionID, &b.Channel, &b.ExternalID, &b.DisplayName, &b.Metadata, &verifiedAt, &createdAt); err != nil {
			return nil, err
		}
		if verifiedAt.Valid && verifiedAt.Int64 > 0 {
			t := time.UnixMilli(verifiedAt.Int64)
			b.VerifiedAt = &t
		}
		b.CreatedAt = time.UnixMilli(createdAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r channelBindingRepo) ListAllVerified(ctx context.Context) ([]domain.ChannelBinding, error) {
	rows, err := r.query(ctx,
		`SELECT id, session_id, channel, external_id, display_name, metadata, verified_at, created_at
		 FROM channel_bindings WHERE verified_at IS NOT NULL ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ChannelBinding
	for rows.Next() {
		var b domain.ChannelBinding
		var verifiedAt sql.NullInt64
		var createdAt int64
		if err := rows.Scan(&b.ID, &b.SessionID, &b.Channel, &b.ExternalID, &b.DisplayName, &b.Metadata, &verifiedAt, &createdAt); err != nil {
			return nil, err
		}
		if verifiedAt.Valid && verifiedAt.Int64 > 0 {
			t := time.UnixMilli(verifiedAt.Int64)
			b.VerifiedAt = &t
		}
		b.CreatedAt = time.UnixMilli(createdAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r channelBindingRepo) GetByChannelAndExternal(ctx context.Context, channel, externalID string) (*domain.ChannelBinding, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, channel, external_id, display_name, metadata, verified_at, created_at
		 FROM channel_bindings WHERE channel = ? AND external_id = ? AND verified_at IS NOT NULL`, channel, externalID)
	var b domain.ChannelBinding
	var verifiedAt sql.NullInt64
	var createdAt int64
	err := row.Scan(&b.ID, &b.SessionID, &b.Channel, &b.ExternalID, &b.DisplayName, &b.Metadata, &verifiedAt, &createdAt)
	if err != nil {
		return nil, err
	}
	if verifiedAt.Valid && verifiedAt.Int64 > 0 {
		t := time.UnixMilli(verifiedAt.Int64)
		b.VerifiedAt = &t
	}
	b.CreatedAt = time.UnixMilli(createdAt)
	return &b, nil
}

func (r channelBindingRepo) Create(ctx context.Context, b *domain.ChannelBinding) (*domain.ChannelBinding, error) {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	if b.Metadata == "" {
		b.Metadata = "{}"
	}
	now := time.Now().UnixMilli()
	_, err := r.exec(ctx,
		`INSERT INTO channel_bindings (id, session_id, channel, external_id, display_name, metadata, verified_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.SessionID, b.Channel, b.ExternalID, b.DisplayName, b.Metadata, nil, now)
	if err != nil {
		return nil, err
	}
	b.CreatedAt = time.UnixMilli(now)
	return b, nil
}

func (r channelBindingRepo) GetPendingByToken(ctx context.Context, channel, token string) (*domain.ChannelBinding, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, channel, external_id, display_name, metadata, verified_at, created_at
		 FROM channel_bindings WHERE channel = ? AND external_id = ? AND verified_at IS NULL`, channel, token)
	var b domain.ChannelBinding
	var verifiedAt sql.NullInt64
	var createdAt int64
	if err := row.Scan(&b.ID, &b.SessionID, &b.Channel, &b.ExternalID, &b.DisplayName, &b.Metadata, &verifiedAt, &createdAt); err != nil {
		return nil, err
	}
	_ = verifiedAt // always NULL for a pending row
	b.CreatedAt = time.UnixMilli(createdAt)
	return &b, nil
}

func (r channelBindingRepo) Promote(ctx context.Context, id, externalID, displayName string) error {
	now := time.Now().UnixMilli()
	_, err := r.exec(ctx,
		`UPDATE channel_bindings SET external_id = ?, verified_at = ?, display_name = ?
		 WHERE id = ? AND verified_at IS NULL`,
		externalID, now, displayName, id)
	return err
}

func (r channelBindingRepo) ExpirePending(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-10 * time.Minute).UnixMilli()
	res, err := r.exec(ctx, `DELETE FROM channel_bindings WHERE verified_at IS NULL AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r channelBindingRepo) Delete(ctx context.Context, sessionID, channel string) error {
	_, err := r.exec(ctx,
		`DELETE FROM channel_bindings WHERE session_id = ? AND channel = ?`, sessionID, channel)
	return err
}
