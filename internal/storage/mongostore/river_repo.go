package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type riverRepo struct{ *Store }

// Days mirrors the SQL side exactly: the last `days` UTC days inclusive of
// today, oldest first, with each day's operation count and mood emoji. See
// sqlstore/river.go for why the day boundaries are computed in Go and why moods
// are read once and bucketed while operations are counted per day.
func (r riverRepo) Days(ctx context.Context, sessionID string, days int) ([]domain.RiverDay, error) {
	today := domain.UTCDay(time.Now())
	start, err := domain.StartOfUTCDay(today)
	if err != nil {
		return nil, err
	}
	from := start.AddDate(0, 0, -(days - 1))
	end := start.AddDate(0, 0, 1) // exclusive

	moods, err := r.moodsByDay(ctx, sessionID, from, end)
	if err != nil {
		return nil, err
	}

	out := make([]domain.RiverDay, 0, days)
	for d := from; d.Before(end); d = d.AddDate(0, 0, 1) {
		day := domain.UTCDay(d)
		row := domain.RiverDay{Date: day, Mood: moods[day]}
		n, err := r.c("operation_logs").CountDocuments(ctx, bson.M{
			"session_id": sessionID,
			"created_at": bson.M{"$gte": d, "$lt": d.AddDate(0, 0, 1)},
		})
		if err != nil {
			return nil, err
		}
		row.Count = n
		out = append(out, row)
	}
	return out, nil
}

// moodsByDay folds one window of check-ins into day → emoji, newest resolvable
// winning. Unresolvable ids are skipped, not stored — the same rule as SQL.
func (r riverRepo) moodsByDay(ctx context.Context, sessionID string, from, end time.Time) (map[string]string, error) {
	cur, err := r.c("mood_checkins").Find(ctx,
		bson.M{"session_id": sessionID, "created_at": bson.M{"$gte": from, "$lt": end}},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[string]string{}
	for cur.Next(ctx) {
		var d struct {
			Mood      string    `bson:"mood"`
			CreatedAt time.Time `bson:"created_at"`
		}
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		day := domain.UTCDay(d.CreatedAt)
		if _, set := out[day]; set {
			continue // a newer check-in already won this day
		}
		if kind, ok := domain.MoodKindByID(d.Mood); ok {
			out[day] = kind.Emoji
		}
	}
	return out, cur.Err()
}
