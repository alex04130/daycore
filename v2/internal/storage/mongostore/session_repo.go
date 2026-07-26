package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type sessionDoc struct {
	ID               string    `bson:"_id"`
	UserID           *string   `bson:"user_id,omitempty"`
	InteractionCount int       `bson:"interaction_count"`
	SignInPrompted   bool      `bson:"sign_in_prompted"`
	AssistantName    string    `bson:"assistant_name"`
	CurrentTheme     string    `bson:"current_theme"`
	Language         string    `bson:"language,omitempty"`
	PersonaPrompt    string    `bson:"persona_prompt,omitempty"`
	ImportToken      string    `bson:"import_token,omitempty"`
	Preferences      string    `bson:"preferences,omitempty"`
	CreatedAt        time.Time `bson:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at"`
}

func (d sessionDoc) toDomain() *domain.Session {
	return &domain.Session{
		ID: d.ID, UserID: d.UserID, InteractionCount: d.InteractionCount,
		SignInPrompted: d.SignInPrompted, AssistantName: d.AssistantName,
		CurrentTheme: d.CurrentTheme, Language: d.Language, PersonaPrompt: d.PersonaPrompt, ImportToken: d.ImportToken,
		Preferences: d.Preferences,
		CreatedAt:   d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type sessionRepo struct{ *Store }

func (r sessionRepo) GetOrCreate(ctx context.Context, id string) (*domain.Session, error) {
	now := time.Now().UTC()
	_, err := r.c("sessions").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$setOnInsert": bson.M{
		"interaction_count": 0, "sign_in_prompted": false,
		"assistant_name": "Leo", "current_theme": "sky",
		"created_at": now, "updated_at": now,
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r sessionRepo) Get(ctx context.Context, id string) (*domain.Session, error) {
	var d sessionDoc
	if err := r.c("sessions").FindOne(ctx, bson.M{"_id": id}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r sessionRepo) Update(ctx context.Context, id string, upd domain.SessionUpdate) (*domain.Session, error) {
	set := bson.M{"updated_at": time.Now().UTC()}
	if upd.UserID != nil {
		set["user_id"] = *upd.UserID
	}
	if upd.AssistantName != nil {
		set["assistant_name"] = *upd.AssistantName
	}
	if upd.CurrentTheme != nil {
		set["current_theme"] = *upd.CurrentTheme
	}
	if upd.SignInPrompted != nil {
		set["sign_in_prompted"] = *upd.SignInPrompted
	}
	if upd.Language != nil {
		set["language"] = *upd.Language
	}
	if upd.PersonaPrompt != nil {
		set["persona_prompt"] = *upd.PersonaPrompt
	}
	if upd.ImportToken != nil {
		set["import_token"] = *upd.ImportToken
	}
	if upd.Preferences != nil {
		set["preferences"] = *upd.Preferences
	}
	if _, err := r.c("sessions").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r sessionRepo) GetByImportToken(ctx context.Context, token string) (*domain.Session, error) {
	if token == "" {
		return nil, domain.ErrNotFound
	}
	var d sessionDoc
	if err := r.c("sessions").FindOne(ctx, bson.M{"import_token": token}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r sessionRepo) IncrementInteraction(ctx context.Context, id string) error {
	_, err := r.c("sessions").UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc": bson.M{"interaction_count": 1},
		"$set": bson.M{"updated_at": time.Now().UTC()},
	})
	return err
}
