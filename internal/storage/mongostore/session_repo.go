package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type sessionDoc struct {
	ID               string  `bson:"_id"`
	UserID           *string `bson:"user_id,omitempty"`
	InteractionCount int     `bson:"interaction_count"`
	SignInPrompted   bool    `bson:"sign_in_prompted"`
	AssistantName    string  `bson:"assistant_name"`
	CurrentTheme     string  `bson:"current_theme"`
	Language         string  `bson:"language,omitempty"`
	PersonaPrompt    string  `bson:"persona_prompt,omitempty"`
	ImportToken      string  `bson:"import_token,omitempty"`
	Preferences      string  `bson:"preferences,omitempty"`
	// Per-account usage counters — see domain/session_usage.go. Millisecond
	// stamps rather than BSON dates so the arithmetic $cond below compares
	// numbers; a date comparison inside $cond would work but would then be the
	// one field in this document stored differently from how the SQL side
	// stores it, for no gain.
	UsageFastCalls  int64     `bson:"usage_fast_calls,omitempty"`
	UsageFastTokens int64     `bson:"usage_fast_tokens,omitempty"`
	UsageFastStart  int64     `bson:"usage_fast_start,omitempty"`
	UsageSlowCalls  int64     `bson:"usage_slow_calls,omitempty"`
	UsageSlowTokens int64     `bson:"usage_slow_tokens,omitempty"`
	UsageSlowStart  int64     `bson:"usage_slow_start,omitempty"`
	UsageCalls      int64     `bson:"usage_calls,omitempty"`
	UsagePromptTok  int64     `bson:"usage_prompt_tokens,omitempty"`
	UsageCompTok    int64     `bson:"usage_comp_tokens,omitempty"`
	CreatedAt       time.Time `bson:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

func (d sessionDoc) toDomain() *domain.Session {
	return &domain.Session{
		ID: d.ID, UserID: d.UserID, InteractionCount: d.InteractionCount,
		SignInPrompted: d.SignInPrompted, AssistantName: d.AssistantName,
		CurrentTheme: d.CurrentTheme, Language: d.Language, PersonaPrompt: d.PersonaPrompt, ImportToken: d.ImportToken,
		Preferences: d.Preferences,
		Usage: domain.SessionUsage{
			FastCalls: d.UsageFastCalls, FastTokens: d.UsageFastTokens,
			FastWindowStart: fromMillisOrZero(d.UsageFastStart),
			SlowCalls:       d.UsageSlowCalls, SlowTokens: d.UsageSlowTokens,
			SlowWindowStart:   fromMillisOrZero(d.UsageSlowStart),
			TotalCalls:        d.UsageCalls,
			TotalPromptTokens: d.UsagePromptTok, TotalCompTokens: d.UsageCompTok,
		},
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
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

// fromMillisOrZero keeps "never opened" distinct from the epoch. A stored zero
// rendered as 1970 would make every fresh session look permanently expired and
// permanently ancient on the console.
func fromMillisOrZero(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return fromMillis(ms)
}

// AddUsage folds one call into the three counters, in one update.
//
// # $inc cannot express a window that rolls over, so this is a pipeline update
//
// Mongo 4.2+ accepts an aggregation pipeline in place of an update document,
// which is what makes the SQL side's CASE expressible here: $cond picks between
// "start a new window at this call" and "add to the current one", and the whole
// thing stays ONE atomic operation on the document.
//
// The alternative — read the doc, decide in Go, write it back — loses calls
// whenever two instances handle two calls for the same account at once. See the
// SQL side for why that particular loss is the expensive one.
func (r sessionRepo) AddUsage(ctx context.Context, sessionID string, promptTokens, compTokens int, now time.Time) error {
	if sessionID == "" {
		return domain.ErrMissingUpsertKey
	}
	tokens := int64(promptTokens + compTokens)
	nowMs := now.UnixMilli()
	fastCut := now.Add(-domain.UsageFastWindow).UnixMilli()
	slowCut := now.Add(-domain.UsageSlowWindow).UnixMilli()

	// expired(field, cut) is "this window has rolled over", and it has to treat a
	// MISSING field as expired — a document written before these fields existed
	// has none of them, and $lte against a missing field is false, which would
	// leave such a session's counters permanently un-started.
	expired := func(field string, cut int64) bson.M {
		return bson.M{"$lte": bson.A{bson.M{"$ifNull": bson.A{"$" + field, 0}}, cut}}
	}
	pick := func(field string, cut int64, fresh, carried any) bson.M {
		return bson.M{"$cond": bson.A{expired(field, cut), fresh, carried}}
	}
	add := func(field string, by any) bson.M {
		return bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$" + field, 0}}, by}}
	}

	_, err := r.c("sessions").UpdateOne(ctx, bson.M{"_id": sessionID}, mongo.Pipeline{
		{{Key: "$set", Value: bson.M{
			"usage_fast_calls":    pick("usage_fast_start", fastCut, 1, add("usage_fast_calls", 1)),
			"usage_fast_tokens":   pick("usage_fast_start", fastCut, tokens, add("usage_fast_tokens", tokens)),
			"usage_slow_calls":    pick("usage_slow_start", slowCut, 1, add("usage_slow_calls", 1)),
			"usage_slow_tokens":   pick("usage_slow_start", slowCut, tokens, add("usage_slow_tokens", tokens)),
			"usage_calls":         add("usage_calls", 1),
			"usage_prompt_tokens": add("usage_prompt_tokens", int64(promptTokens)),
			"usage_comp_tokens":   add("usage_comp_tokens", int64(compTokens)),
			"updated_at":          now.UTC(),
		}}},
		// ⚠️ A SECOND stage, not the same $set. Within one $set every expression
		// sees the document as it was BEFORE the stage, but writing the start
		// stamps beside the counters that read them is the kind of thing a later
		// reader has to check the semantics of. Doing it after removes the
		// question: by here the counters are already decided.
		{{Key: "$set", Value: bson.M{
			"usage_fast_start": pick("usage_fast_start", fastCut, nowMs, "$usage_fast_start"),
			"usage_slow_start": pick("usage_slow_start", slowCut, nowMs, "$usage_slow_start"),
		}}},
	})
	return err
}
