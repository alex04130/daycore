package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ── rapport ─────────────────────────────────────────────────────────────────

type rapportDoc struct {
	SessionID       string                         `bson:"_id"`
	Scores          map[string]domain.RapportScore `bson:"scores"`
	CursorCreatedAt time.Time                      `bson:"cursor_created_at,omitempty"`
	CursorID        string                         `bson:"cursor_id,omitempty"`
	FoldVersion     int                            `bson:"fold_version"`
	UpdatedAt       time.Time                      `bson:"updated_at"`
}

type rapportRepo struct{ *Store }

func (r rapportRepo) Get(ctx context.Context, sessionID string) (*domain.RapportState, error) {
	var d rapportDoc
	if err := r.c("rapport_states").FindOne(ctx, bson.M{"_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if d.Scores == nil {
		d.Scores = map[string]domain.RapportScore{}
	}
	return &domain.RapportState{
		SessionID: d.SessionID, Scores: d.Scores,
		Cursor:      domain.OpLogCursor{CreatedAt: d.CursorCreatedAt, ID: d.CursorID},
		FoldVersion: d.FoldVersion, UpdatedAt: d.UpdatedAt,
	}, nil
}

func (r rapportRepo) Save(ctx context.Context, s *domain.RapportState) error {
	if s.Scores == nil {
		s.Scores = map[string]domain.RapportScore{}
	}
	now := time.Now().UTC()
	_, err := r.c("rapport_states").UpdateOne(ctx, bson.M{"_id": s.SessionID}, bson.M{
		"$set": bson.M{
			"scores":            s.Scores,
			"cursor_created_at": s.Cursor.CreatedAt,
			"cursor_id":         s.Cursor.ID,
			"fold_version":      s.FoldVersion,
			"updated_at":        now,
		},
	}, options.Update().SetUpsert(true))
	if err == nil {
		s.UpdatedAt = now
	}
	return err
}

func (r rapportRepo) Reset(ctx context.Context, sessionID string) error {
	_, err := r.c("rapport_states").DeleteOne(ctx, bson.M{"_id": sessionID})
	return err
}

// ── rhythm ──────────────────────────────────────────────────────────────────

type rhythmProfileDoc struct {
	SessionID    string    `bson:"_id"`
	Wake         string    `bson:"wake_hm"`
	Sleep        string    `bson:"sleep_hm"`
	Source       string    `bson:"source"`
	Days         int       `bson:"learned_days"`
	RunSince     time.Time `bson:"run_since,omitempty"`
	LastSignalAt time.Time `bson:"last_signal_at,omitempty"`
	UpdatedAt    time.Time `bson:"updated_at"`
}

type rhythmDayDoc struct {
	ID        string `bson:"_id"` // "<session>:<day>"
	SessionID string `bson:"session_id"`
	Day       string `bson:"day"`
	FirstMin  int    `bson:"first_min"`
	LastMin   int    `bson:"last_min"`
	Signals   int    `bson:"signals"`
}

type rhythmRepo struct{ *Store }

func (r rhythmRepo) Get(ctx context.Context, sessionID string) (*domain.RhythmProfile, error) {
	var d rhythmProfileDoc
	if err := r.c("rhythm_profiles").FindOne(ctx, bson.M{"_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.RhythmProfile{
		SessionID: d.SessionID, Wake: d.Wake, Sleep: d.Sleep, Source: d.Source, Days: d.Days,
		RunSince: d.RunSince, LastSignalAt: d.LastSignalAt, UpdatedAt: d.UpdatedAt,
	}, nil
}

func (r rhythmRepo) Save(ctx context.Context, p *domain.RhythmProfile) error {
	now := time.Now().UTC()
	_, err := r.c("rhythm_profiles").UpdateOne(ctx, bson.M{"_id": p.SessionID}, bson.M{
		"$set": bson.M{
			"wake_hm": p.Wake, "sleep_hm": p.Sleep, "source": p.Source, "learned_days": p.Days,
			"run_since": p.RunSince, "last_signal_at": p.LastSignalAt, "updated_at": now,
		},
	}, options.Update().SetUpsert(true))
	if err == nil {
		p.UpdatedAt = now
	}
	return err
}

// Observe widens the day's bounds in the update itself rather than reading,
// comparing and writing back. This runs on every awake signal, and a read-then-
// write would lose an update whenever two tabs are open.
//
// $min and $max do exactly that, and on an upsert they seed the field from the
// value supplied — so the first signal of a day sets both ends rather than
// widening from a zero that would make every day look like it started at its cut.
func (r rhythmRepo) Observe(ctx context.Context, sessionID, day string, minute int) error {
	_, err := r.c("rhythm_days").UpdateOne(ctx, bson.M{"_id": sessionID + ":" + day}, bson.M{
		"$min": bson.M{"first_min": minute},
		"$max": bson.M{"last_min": minute},
		"$inc": bson.M{"signals": 1},
		"$setOnInsert": bson.M{
			"session_id": sessionID,
			"day":        day,
		},
	}, options.Update().SetUpsert(true))
	return err
}

func (r rhythmRepo) Days(ctx context.Context, sessionID string, limit int) ([]domain.RhythmDay, error) {
	if limit <= 0 {
		limit = 30
	}
	cur, err := r.c("rhythm_days").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "day", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.RhythmDay{}
	for cur.Next(ctx) {
		var d rhythmDayDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.RhythmDay{
			SessionID: d.SessionID, Day: d.Day,
			FirstMin: d.FirstMin, LastMin: d.LastMin, Signals: d.Signals,
		})
	}
	return out, cur.Err()
}

func (r rhythmRepo) PruneDays(ctx context.Context, sessionID, before string) (int, error) {
	res, err := r.c("rhythm_days").DeleteMany(ctx, bson.M{
		"session_id": sessionID,
		"day":        bson.M{"$lt": before},
	})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}

// ── locale overrides ────────────────────────────────────────────────────────

type localeDoc struct {
	ID        string    `bson:"_id"` // "<key>:<locale>"
	Key       string    `bson:"message_key"`
	Locale    string    `bson:"locale"`
	Content   string    `bson:"content"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type localeRepo struct{ *Store }

func (r localeRepo) All(ctx context.Context) ([]domain.LocaleOverride, error) {
	cur, err := r.c("locale_overrides").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "message_key", Value: 1}, {Key: "locale", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.LocaleOverride{}
	for cur.Next(ctx) {
		var d localeDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.LocaleOverride{
			Key: d.Key, Locale: d.Locale, Content: d.Content, UpdatedAt: d.UpdatedAt,
		})
	}
	return out, cur.Err()
}

func (r localeRepo) Set(ctx context.Context, key, locale, content string) error {
	_, err := r.c("locale_overrides").UpdateOne(ctx,
		bson.M{"message_key": key, "locale": locale},
		bson.M{
			"$set":         bson.M{"content": content, "updated_at": time.Now().UTC()},
			"$setOnInsert": bson.M{"_id": key + ":" + locale, "message_key": key, "locale": locale},
		}, options.Update().SetUpsert(true))
	return err
}

func (r localeRepo) Delete(ctx context.Context, key, locale string) error {
	_, err := r.c("locale_overrides").DeleteOne(ctx, bson.M{"message_key": key, "locale": locale})
	return err
}

func (r localeRepo) DeleteLocale(ctx context.Context, locale string) (int, error) {
	res, err := r.c("locale_overrides").DeleteMany(ctx, bson.M{"locale": locale})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}
