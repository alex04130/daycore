package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type moodDoc struct {
	ID                string    `bson:"_id"`
	SessionID         string    `bson:"session_id"`
	Mood              string    `bson:"mood"`
	AIResponse        *string   `bson:"ai_response,omitempty"`
	ExerciseOffered   *string   `bson:"exercise_offered,omitempty"`
	ExerciseCompleted bool      `bson:"exercise_completed"`
	Theme             *string   `bson:"theme,omitempty"`
	Source            string    `bson:"source,omitempty"`
	Note              string    `bson:"note,omitempty"`
	CreatedAt         time.Time `bson:"created_at"`
}

type moodRepo struct{ *Store }

func (r moodRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.MoodCheckin, error) {
	limit = domain.ListLimit(limit, domain.MoodListDefault, domain.MoodListMax)
	cur, err := r.c("mood_checkins").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.MoodCheckin{}
	for cur.Next(ctx) {
		var d moodDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.MoodCheckin{
			ID: d.ID, SessionID: d.SessionID, Mood: d.Mood, AIResponse: d.AIResponse,
			ExerciseOffered: d.ExerciseOffered, ExerciseCompleted: d.ExerciseCompleted,
			Theme: d.Theme, Source: d.Source, Note: d.Note, CreatedAt: d.CreatedAt,
		})
	}
	return out, cur.Err()
}

func (r moodRepo) Create(ctx context.Context, m *domain.MoodCheckin) (*domain.MoodCheckin, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	m.CreatedAt = time.Now().UTC()
	_, err := r.c("mood_checkins").InsertOne(ctx, moodDoc{
		ID: m.ID, SessionID: m.SessionID, Mood: m.Mood, AIResponse: m.AIResponse,
		ExerciseOffered: m.ExerciseOffered, ExerciseCompleted: m.ExerciseCompleted,
		Theme: m.Theme, Source: m.Source, Note: m.Note, CreatedAt: m.CreatedAt,
	})
	return m, err
}

func (r moodRepo) MarkExerciseCompleted(ctx context.Context, sessionID, id string) error {
	_, err := r.c("mood_checkins").UpdateOne(ctx, bson.M{"_id": id, "session_id": sessionID}, bson.M{"$set": bson.M{"exercise_completed": true}})
	return err
}
