package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type weeklyLetterDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	WeekStart string    `bson:"week_start"`
	WeekEnd   string    `bson:"week_end"`
	Body      string    `bson:"body"`
	Locale    string    `bson:"locale"`
	CreatedAt time.Time `bson:"created_at"`
}

func (d weeklyLetterDoc) toDomain() *domain.WeeklyLetter {
	return &domain.WeeklyLetter{
		ID: d.ID, SessionID: d.SessionID, WeekStart: d.WeekStart, WeekEnd: d.WeekEnd,
		Body: d.Body, Locale: d.Locale, CreatedAt: d.CreatedAt,
	}
}

type weeklyLetterRepo struct{ *Store }

func (r weeklyLetterRepo) Latest(ctx context.Context, sessionID string) (*domain.WeeklyLetter, error) {
	var d weeklyLetterDoc
	err := r.c("weekly_letters").FindOne(ctx,
		bson.M{"session_id": sessionID},
		options.FindOne().SetSort(bson.D{{Key: "week_start", Value: -1}, {Key: "created_at", Value: -1}}),
	).Decode(&d)
	if err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r weeklyLetterRepo) Get(ctx context.Context, sessionID, id string) (*domain.WeeklyLetter, error) {
	var d weeklyLetterDoc
	if err := r.c("weekly_letters").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r weeklyLetterRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.WeeklyLetter, error) {
	cur, err := r.c("weekly_letters").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "week_start", Value: -1}, {Key: "created_at", Value: -1}}).SetLimit(int64(domain.ListLimit(limit, domain.WeeklyLetterListDefault, domain.WeeklyLetterListMax))))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.WeeklyLetter{}
	for cur.Next(ctx) {
		var d weeklyLetterDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r weeklyLetterRepo) Create(ctx context.Context, l *domain.WeeklyLetter) (*domain.WeeklyLetter, error) {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.CreatedAt = time.Now().UTC()
	if _, err := r.c("weekly_letters").InsertOne(ctx, weeklyLetterDoc{
		ID: l.ID, SessionID: l.SessionID, WeekStart: l.WeekStart, WeekEnd: l.WeekEnd,
		Body: l.Body, Locale: l.Locale, CreatedAt: l.CreatedAt,
	}); err != nil {
		return nil, err
	}
	return r.Get(ctx, l.SessionID, l.ID)
}

func (r weeklyLetterRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.c("weekly_letters").DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
