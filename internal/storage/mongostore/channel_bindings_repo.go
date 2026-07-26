package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type channelBindingDoc struct {
	ID          string     `bson:"_id"`
	SessionID   string     `bson:"session_id"`
	Channel     string     `bson:"channel"`
	ExternalID  string     `bson:"external_id"`
	DisplayName string     `bson:"display_name"`
	Metadata    string     `bson:"metadata"`
	VerifiedAt  *time.Time `bson:"verified_at,omitempty"`
	CreatedAt   time.Time  `bson:"created_at"`
}

type channelBindingRepo struct{ *Store }

func (r channelBindingRepo) ListBySession(ctx context.Context, sessionID string) ([]domain.ChannelBinding, error) {
	cur, err := r.c("channel_bindings").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.ChannelBinding
	for cur.Next(ctx) {
		var d channelBindingDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.ChannelBinding{
			ID: d.ID, SessionID: d.SessionID, Channel: d.Channel,
			ExternalID: d.ExternalID, DisplayName: d.DisplayName, Metadata: d.Metadata,
			VerifiedAt: d.VerifiedAt, CreatedAt: d.CreatedAt,
		})
	}
	return out, cur.Err()
}

func (r channelBindingRepo) ListAllVerified(ctx context.Context) ([]domain.ChannelBinding, error) {
	cur, err := r.c("channel_bindings").Find(ctx, bson.M{"verified_at": bson.M{"$ne": nil}},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.ChannelBinding
	for cur.Next(ctx) {
		var d channelBindingDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.ChannelBinding{
			ID: d.ID, SessionID: d.SessionID, Channel: d.Channel,
			ExternalID: d.ExternalID, DisplayName: d.DisplayName, Metadata: d.Metadata,
			VerifiedAt: d.VerifiedAt, CreatedAt: d.CreatedAt,
		})
	}
	return out, cur.Err()
}

func (r channelBindingRepo) GetByChannelAndExternal(ctx context.Context, channel, externalID string) (*domain.ChannelBinding, error) {
	var d channelBindingDoc
	err := r.c("channel_bindings").FindOne(ctx,
		bson.M{"channel": channel, "external_id": externalID, "verified_at": bson.M{"$ne": nil}}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return &domain.ChannelBinding{
		ID: d.ID, SessionID: d.SessionID, Channel: d.Channel,
		ExternalID: d.ExternalID, DisplayName: d.DisplayName, Metadata: d.Metadata,
		VerifiedAt: d.VerifiedAt, CreatedAt: d.CreatedAt,
	}, nil
}

func (r channelBindingRepo) Create(ctx context.Context, b *domain.ChannelBinding) (*domain.ChannelBinding, error) {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	if b.Metadata == "" {
		b.Metadata = "{}"
	}
	b.CreatedAt = time.Now().UTC()
	_, err := r.c("channel_bindings").InsertOne(ctx, channelBindingDoc{
		ID: b.ID, SessionID: b.SessionID, Channel: b.Channel, ExternalID: b.ExternalID,
		DisplayName: b.DisplayName, Metadata: b.Metadata, CreatedAt: b.CreatedAt,
	})
	return b, err
}

func (r channelBindingRepo) GetPendingByToken(ctx context.Context, channel, token string) (*domain.ChannelBinding, error) {
	var d channelBindingDoc
	err := r.c("channel_bindings").FindOne(ctx,
		bson.M{"channel": channel, "external_id": token, "verified_at": nil}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return &domain.ChannelBinding{
		ID: d.ID, SessionID: d.SessionID, Channel: d.Channel,
		ExternalID: d.ExternalID, DisplayName: d.DisplayName, Metadata: d.Metadata,
		VerifiedAt: d.VerifiedAt, CreatedAt: d.CreatedAt,
	}, nil
}

func (r channelBindingRepo) Promote(ctx context.Context, id, externalID, displayName string) error {
	now := time.Now().UTC()
	_, err := r.c("channel_bindings").UpdateOne(ctx,
		bson.M{"_id": id, "verified_at": nil},
		bson.M{"$set": bson.M{"external_id": externalID, "verified_at": now, "display_name": displayName}})
	return err
}

func (r channelBindingRepo) ExpirePending(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-10 * time.Minute).UTC()
	res, err := r.c("channel_bindings").DeleteMany(ctx,
		bson.M{"verified_at": nil, "created_at": bson.M{"$lt": cutoff}})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}

func (r channelBindingRepo) Delete(ctx context.Context, sessionID, channel string) error {
	_, err := r.c("channel_bindings").DeleteOne(ctx,
		bson.M{"session_id": sessionID, "channel": channel})
	return err
}
