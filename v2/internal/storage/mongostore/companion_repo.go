package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type companionDoc struct {
	ID                  string           `bson:"_id"`
	SessionID           string           `bson:"session_id"`
	KeyFacts            []string         `bson:"key_facts"`
	ConversationHistory []domain.Message `bson:"conversation_history"`
	UpdatedAt           time.Time        `bson:"updated_at"`
}

type companionRepo struct{ *Store }

func (r companionRepo) Get(ctx context.Context, sessionID string) (*domain.CompanionMemory, error) {
	var d companionDoc
	if err := r.c("companion_memory").FindOne(ctx, bson.M{"session_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	kf := d.KeyFacts
	if kf == nil {
		kf = []string{}
	}
	hist := d.ConversationHistory
	if hist == nil {
		hist = []domain.Message{}
	}
	return &domain.CompanionMemory{
		ID: d.ID, SessionID: d.SessionID, KeyFacts: kf, ConversationHistory: hist, UpdatedAt: d.UpdatedAt,
	}, nil
}

func (r companionRepo) Upsert(ctx context.Context, sessionID string, history []domain.Message, keyFacts []string) error {
	if history == nil {
		history = []domain.Message{}
	}
	if keyFacts == nil {
		keyFacts = []string{}
	}
	now := time.Now().UTC()
	_, err := r.c("companion_memory").UpdateOne(ctx, bson.M{"session_id": sessionID}, bson.M{
		"$set":         bson.M{"conversation_history": history, "key_facts": keyFacts, "updated_at": now},
		"$setOnInsert": bson.M{"_id": uuid.NewString()},
	}, options.Update().SetUpsert(true))
	return err
}
