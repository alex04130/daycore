package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ruleDoc struct {
	ID          string           `bson:"_id"`
	SessionID   string           `bson:"session_id"`
	Title       string           `bson:"title"`
	Type        domain.BlockType `bson:"block_type"`
	Time        *string          `bson:"start_time,omitempty"`
	DurationMin *int             `bson:"duration_min,omitempty"`
	Timezone    string           `bson:"timezone"`
	TimeMode    domain.TimeMode  `bson:"time_mode"`
	Kind        string           `bson:"kind"`
	Date        *string          `bson:"date,omitempty"`
	Freq        string           `bson:"freq"`
	Interval    int              `bson:"interval_n"`
	ByWeekday   []int            `bson:"by_weekday"`
	StartDate   string           `bson:"start_date"`
	Until       *string          `bson:"until_date,omitempty"`
	Active      bool             `bson:"active"`
	Source      string           `bson:"source"`
	Note        *string          `bson:"note,omitempty"`
	CreatedAt   time.Time        `bson:"created_at"`
	UpdatedAt   time.Time        `bson:"updated_at"`
}

func (d ruleDoc) toDomain() *domain.ScheduleRule {
	return &domain.ScheduleRule{
		ID: d.ID, SessionID: d.SessionID, Title: d.Title, Type: d.Type, Time: d.Time,
		DurationMin: d.DurationMin, Timezone: d.Timezone, TimeMode: d.TimeMode, Kind: d.Kind,
		Date: d.Date, Freq: d.Freq, Interval: d.Interval, ByWeekday: d.ByWeekday,
		StartDate: d.StartDate, Until: d.Until, Active: d.Active, Source: d.Source, Note: d.Note,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func ruleToDoc(r *domain.ScheduleRule) ruleDoc {
	return ruleDoc{
		ID: r.ID, SessionID: r.SessionID, Title: r.Title, Type: r.Type, Time: r.Time,
		DurationMin: r.DurationMin, Timezone: r.Timezone, TimeMode: r.TimeMode, Kind: r.Kind,
		Date: r.Date, Freq: r.Freq, Interval: r.Interval, ByWeekday: r.ByWeekday,
		StartDate: r.StartDate, Until: r.Until, Active: r.Active, Source: r.Source, Note: r.Note,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type ruleRepo struct{ *Store }

func (r ruleRepo) Get(ctx context.Context, sessionID, id string) (*domain.ScheduleRule, error) {
	var d ruleDoc
	if err := r.c("schedule_rules").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r ruleRepo) List(ctx context.Context, sessionID string) ([]domain.ScheduleRule, error) {
	cur, err := r.c("schedule_rules").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.ScheduleRule{}
	for cur.Next(ctx) {
		var d ruleDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r ruleRepo) Create(ctx context.Context, rule *domain.ScheduleRule) (*domain.ScheduleRule, error) {
	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	if rule.Interval < 1 {
		rule.Interval = 1
	}
	now := time.Now().UTC()
	rule.CreatedAt, rule.UpdatedAt = now, now
	if _, err := r.c("schedule_rules").InsertOne(ctx, ruleToDoc(rule)); err != nil {
		return nil, err
	}
	return r.Get(ctx, rule.SessionID, rule.ID)
}

func (r ruleRepo) Update(ctx context.Context, sessionID, id string, upd domain.ScheduleRuleUpdate) (*domain.ScheduleRule, error) {
	set := bson.M{"updated_at": time.Now().UTC()}
	unset := bson.M{}
	if upd.Title != nil {
		set["title"] = *upd.Title
	}
	if upd.Type != nil {
		set["block_type"] = *upd.Type
	}
	if upd.HasTime {
		if upd.Time == nil || *upd.Time == "" {
			unset["start_time"] = ""
		} else {
			set["start_time"] = *upd.Time
		}
	}
	if upd.DurationMin != nil {
		set["duration_min"] = *upd.DurationMin
	}
	if upd.Timezone != nil {
		set["timezone"] = *upd.Timezone
	}
	if upd.TimeMode != nil {
		set["time_mode"] = *upd.TimeMode
	}
	if upd.Kind != nil {
		set["kind"] = *upd.Kind
	}
	if upd.Date != nil {
		set["date"] = *upd.Date
	}
	if upd.Freq != nil {
		set["freq"] = *upd.Freq
	}
	if upd.Interval != nil {
		n := *upd.Interval
		if n < 1 {
			n = 1
		}
		set["interval_n"] = n
	}
	if upd.ByWeekday != nil {
		set["by_weekday"] = *upd.ByWeekday
	}
	if upd.StartDate != nil {
		set["start_date"] = *upd.StartDate
	}
	if upd.HasUntil {
		if upd.Until == nil || *upd.Until == "" {
			unset["until_date"] = ""
		} else {
			set["until_date"] = *upd.Until
		}
	}
	if upd.Active != nil {
		set["active"] = *upd.Active
	}
	if upd.Note != nil {
		set["note"] = *upd.Note
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	res, err := r.c("schedule_rules").UpdateOne(ctx, bson.M{"_id": id, "session_id": sessionID}, update)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, domain.ErrNotFound
	}
	return r.Get(ctx, sessionID, id)
}

func (r ruleRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.c("schedule_rules").DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
