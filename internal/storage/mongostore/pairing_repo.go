package mongostore

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type pairingDoc struct {
	ID          string   `bson:"_id"`
	Name        string   `bson:"name"`
	Description string   `bson:"description,omitempty"`
	SecretHash  string   `bson:"secret_hash"`
	Roles       []string `bson:"roles"`
	Full        bool     `bson:"full_access,omitempty"`
	LastSeenAt  int64    `bson:"last_seen_at"`
	CreatedAt   int64    `bson:"created_at"`
	UpdatedAt   int64    `bson:"updated_at"`
}

func (d pairingDoc) toDomain() domain.Pairing {
	roles := d.Roles
	if roles == nil {
		roles = []string{}
	}
	p := domain.Pairing{
		ID: d.ID, Name: d.Name, Description: d.Description,
		SecretHash: d.SecretHash, Roles: roles, Full: d.Full,
		CreatedAt: fromMillis(d.CreatedAt), UpdatedAt: fromMillis(d.UpdatedAt),
	}
	if d.LastSeenAt > 0 {
		p.LastSeenAt = fromMillis(d.LastSeenAt)
	}
	return p
}

type pairingRepo struct{ *Store }

func (r pairingRepo) List(ctx context.Context) ([]domain.Pairing, error) {
	cur, err := r.c("pairings").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Pairing{}
	for cur.Next(ctx) {
		var d pairingDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toDomain())
	}
	return out, cur.Err()
}

func (r pairingRepo) Get(ctx context.Context, id string) (*domain.Pairing, error) {
	if id == "" {
		return nil, domain.ErrNotFound
	}
	var d pairingDoc
	err := r.c("pairings").FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := d.toDomain()
	return &p, nil
}

func (r pairingRepo) Create(ctx context.Context, p *domain.Pairing) error {
	if p.ID == "" || p.SecretHash == "" {
		return domain.ErrMissingUpsertKey
	}
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	now := nowMillis()
	_, err := r.c("pairings").InsertOne(ctx, pairingDoc{
		ID: p.ID, Name: p.Name, Description: p.Description,
		SecretHash: p.SecretHash, Roles: roles, Full: p.Full,
		LastSeenAt: 0, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	p.CreatedAt, p.UpdatedAt = fromMillis(now), fromMillis(now)
	return nil
}

func (r pairingRepo) SetRoles(ctx context.Context, id string, roles []string) error {
	if roles == nil {
		roles = []string{}
	}
	res, err := r.c("pairings").UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"roles": roles, "updated_at": nowMillis()}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// TouchLastSeen writes only when the stored value is already stale — the
// staleness test is part of the FILTER, so this is one round trip and two
// instances cannot each write what the other just wrote. See the SQL side.
func (r pairingRepo) TouchLastSeen(ctx context.Context, id string, now time.Time) (bool, error) {
	cutoff := now.Add(-domain.PairingLastSeenGranularity).UnixMilli()
	res, err := r.c("pairings").UpdateOne(ctx,
		bson.M{"_id": id, "last_seen_at": bson.M{"$lte": cutoff}},
		bson.M{"$set": bson.M{"last_seen_at": now.UnixMilli()}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}

// SetFull marks or unmarks root-equivalence — see domain.Pairing.Full.
func (r pairingRepo) SetFull(ctx context.Context, id string, full bool) error {
	res, err := r.c("pairings").UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"full_access": full, "updated_at": nowMillis()}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r pairingRepo) Delete(ctx context.Context, id string) error {
	_, err := r.c("pairings").DeleteOne(ctx, bson.M{"_id": id})
	return err
}
