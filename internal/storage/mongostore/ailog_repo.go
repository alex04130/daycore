package mongostore

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type aiCallLogDoc struct {
	ID           string `bson:"_id"`
	SessionID    string `bson:"session_id"`
	Endpoint     string `bson:"endpoint"`
	Model        string `bson:"model"`
	PromptTokens int    `bson:"prompt_tokens"`
	CompTokens   int    `bson:"comp_tokens"`
	DurationMs   int64  `bson:"duration_ms"`
	Status       string `bson:"status"`
	Error        string `bson:"error,omitempty"`
	RequestID    string `bson:"request_id,omitempty"`
	CreatedAt    int64  `bson:"created_at"`
}

type aiLogRepo struct{ *Store }

func (r aiLogRepo) Add(ctx context.Context, l *domain.AICallLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.c("ai_call_logs").InsertOne(ctx, aiCallLogDoc{l.ID, l.SessionID, l.Endpoint, l.Model, l.PromptTokens, l.CompTokens, l.DurationMs, l.Status, l.Error, l.RequestID, now})
	if err != nil {
		return err
	}
	l.CreatedAt = fromMillis(now)
	return nil
}

// aiLogFilterDoc renders the filter.
//
// ⚠️ Every narrowing value goes in as a VALUE, never spliced into a key or an
// expression. The SQL side's note applies here for the same reason: these
// arrive from a query string, and "the console only sends valid ones" is a
// property of one client rather than of this function.
func aiLogFilterDoc(f domain.AILogFilter) bson.M {
	q := bson.M{}
	if f.SessionID != "" {
		q["session_id"] = f.SessionID
	}
	if f.Endpoint != "" {
		q["endpoint"] = f.Endpoint
	}
	if f.Model != "" {
		q["model"] = f.Model
	}
	if f.Status != "" {
		q["status"] = f.Status
	}
	if !f.Since.IsZero() {
		q["created_at"] = bson.M{"$gte": f.Since.UnixMilli()}
	}
	if !f.Before.CreatedAt.IsZero() {
		ms := f.Before.CreatedAt.UnixMilli()
		// $or rather than folding into the created_at clause above, because both
		// bounds can be set at once and a second assignment to the same key would
		// silently drop Since. That is the shape of bug this back end has shipped
		// before: a filter that quietly stops filtering.
		q["$and"] = []bson.M{{"$or": []bson.M{
			{"created_at": bson.M{"$lt": ms}},
			{"created_at": ms, "_id": bson.M{"$lt": f.Before.ID}},
		}}}
	}
	return q
}

func (r aiLogRepo) List(ctx context.Context, f domain.AILogFilter, limit int) ([]domain.AICallLog, error) {
	limit = domain.ListLimit(limit, domain.AILogListDefault, domain.AILogListMax)
	cur, err := r.c("ai_call_logs").Find(ctx, aiLogFilterDoc(f),
		options.Find().
			SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).
			SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.AICallLog{}
	for cur.Next(ctx) {
		var d aiCallLogDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.AICallLog{
			ID: d.ID, SessionID: d.SessionID, Endpoint: d.Endpoint, Model: d.Model,
			PromptTokens: d.PromptTokens, CompTokens: d.CompTokens, DurationMs: d.DurationMs,
			Status: d.Status, Error: d.Error, RequestID: d.RequestID,
			CreatedAt: fromMillis(d.CreatedAt),
		})
	}
	return out, cur.Err()
}

func (r aiLogRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.c("ai_call_logs").DeleteMany(ctx, bson.M{"created_at": bson.M{"$lt": before.UnixMilli()}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (r aiLogRepo) Stats(ctx context.Context) (*domain.AdminStats, error) {
	var s domain.AdminStats
	uc, _ := r.c("users").CountDocuments(ctx, bson.M{})
	sc, _ := r.c("sessions").CountDocuments(ctx, bson.M{})
	ac, _ := r.c("ai_call_logs").CountDocuments(ctx, bson.M{})
	s.Users = int(uc)
	s.Sessions = int(sc)
	s.AICalls = int(ac)
	// Sum comp_tokens via aggregation
	pipe := mongo.Pipeline{{
		{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "total", Value: bson.D{{Key: "$sum", Value: "$comp_tokens"}}}}},
	}}
	cur, err := r.c("ai_call_logs").Aggregate(ctx, pipe)
	if err == nil {
		defer cur.Close(ctx)
		if cur.Next(ctx) {
			var result struct{ Total int64 }
			cur.Decode(&result)
			s.TokenUsed = result.Total
		}
	}
	return &s, nil
}

func nowMillis() int64              { return time.Now().UnixMilli() }
func fromMillis(ms int64) time.Time { return time.UnixMilli(ms) }

// ── the daily rollup ────────────────────────────────────────────────────────

// RollUpUsage folds each closed day with an aggregation pipeline ending in
// $merge, so the documents never leave the database.
//
// $merge is Mongo's equivalent of `INSERT … SELECT`: the pipeline's output is
// written straight into another collection by the server. It is available on a
// self-hosted mongod from 4.2, which matters — the triggers that would have
// been the other way to do this server-side are an Atlas-only product, so a
// trigger-based design would have had no implementation here at all.
//
// The shape mirrors the SQL side exactly, including DELETE-then-write and the
// millisecond range instead of a date operator. See sqlstore/ailog.go and
// domain/ai_usage.go for why each of those is what it is.
func (r aiLogRepo) RollUpUsage(ctx context.Context, today string) (int, error) {
	// ⚠️ Start at the newest folded day, clamped to the ledger's oldest row.
	// The two failures this balances — a partially written day being skipped
	// forever, and a pruned day being overwritten with zero — are argued in
	// sqlstore/ailog.go. This back end is where the first one is REAL: $merge
	// writes documents one at a time, so a fold killed midway genuinely leaves a
	// day half-written.
	var oldest struct {
		CreatedAt int64 `bson:"created_at"`
	}
	lerr := r.c("ai_call_logs").FindOne(ctx, bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "created_at", Value: 1}})).Decode(&oldest)
	if errors.Is(lerr, mongo.ErrNoDocuments) {
		return 0, nil // nothing has ever been logged, or all of it is pruned
	}
	if lerr != nil {
		return 0, lerr
	}
	start := domain.UTCDay(fromMillis(oldest.CreatedAt))

	var newest struct {
		Day string `bson:"day"`
	}
	nerr := r.c("ai_usage_daily").FindOne(ctx, bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "day", Value: -1}})).Decode(&newest)
	switch {
	case nerr == nil:
		if newest.Day > start {
			start = newest.Day
		}
	case errors.Is(nerr, mongo.ErrNoDocuments):
		// Nothing folded yet; start where the ledger starts.
	default:
		return 0, nerr
	}

	from, err := domain.StartOfUTCDay(start)
	if err != nil {
		return 0, err
	}
	end, err := domain.StartOfUTCDay(today)
	if err != nil {
		return 0, err
	}

	written := 0
	for d := from; d.Before(end); d = d.AddDate(0, 0, 1) {
		day := domain.UTCDay(d)
		lo, hi := d.UnixMilli(), d.AddDate(0, 0, 1).UnixMilli()
		// Load-bearing now that the newest day is re-folded: $merge only
		// touches keys the new aggregate produces, so a group that has stopped
		// appearing (its ledger rows pruned) would otherwise linger and be
		// counted forever.
		if _, err := r.c("ai_usage_daily").DeleteMany(ctx, bson.M{"day": day}); err != nil {
			return written, err
		}
		pipe := mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"created_at": bson.M{"$gte": lo, "$lt": hi}}}},
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: bson.D{{Key: "model", Value: "$model"}, {Key: "endpoint", Value: "$endpoint"}}},
				{Key: "calls", Value: bson.M{"$sum": 1}},
				{Key: "errors", Value: bson.M{"$sum": bson.M{"$cond": bson.A{
					bson.M{"$eq": bson.A{"$status", domain.AICallStatusError}}, 1, 0,
				}}}},
				{Key: "prompt_tokens", Value: bson.M{"$sum": "$prompt_tokens"}},
				{Key: "comp_tokens", Value: bson.M{"$sum": "$comp_tokens"}},
			}}},
			// The _id is rebuilt as the composite key so $merge's `on` has
			// something to match, and so a re-run overwrites rather than
			// duplicates. The SQL side gets this from its PRIMARY KEY.
			{{Key: "$project", Value: bson.M{
				"_id":           bson.M{"$concat": bson.A{day, "|", "$_id.model", "|", "$_id.endpoint"}},
				"day":           day,
				"model":         "$_id.model",
				"endpoint":      "$_id.endpoint",
				"calls":         1,
				"errors":        1,
				"prompt_tokens": 1,
				"comp_tokens":   1,
				"updated_at":    nowMillis(),
			}}},
			{{Key: "$merge", Value: bson.M{
				"into":           "ai_usage_daily",
				"on":             "_id",
				"whenMatched":    "replace",
				"whenNotMatched": "insert",
			}}},
		}
		cur, err := r.c("ai_call_logs").Aggregate(ctx, pipe)
		if err != nil {
			return written, err
		}
		// $merge yields no documents, but the cursor still has to be drained
		// and closed or the pipeline may not have run to completion.
		for cur.Next(ctx) { //nolint:revive // draining is the point
		}
		cerr := cur.Err()
		cur.Close(ctx)
		if cerr != nil {
			return written, cerr
		}
		written++
	}
	return written, nil
}

func aiUsageWindowDoc(from, to string) bson.M {
	q := bson.M{}
	rng := bson.M{}
	if from != "" {
		rng["$gte"] = from
	}
	if to != "" {
		rng["$lte"] = to
	}
	if len(rng) > 0 {
		q["day"] = rng
	}
	return q
}

func (r aiLogRepo) UsageTotals(ctx context.Context, from, to string) (*domain.AIUsageTotals, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: aiUsageWindowDoc(from, to)}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "calls", Value: bson.M{"$sum": "$calls"}},
			{Key: "errors", Value: bson.M{"$sum": "$errors"}},
			{Key: "prompt_tokens", Value: bson.M{"$sum": "$prompt_tokens"}},
			{Key: "comp_tokens", Value: bson.M{"$sum": "$comp_tokens"}},
			{Key: "first_day", Value: bson.M{"$min": "$day"}},
		}}},
	}
	cur, err := r.c("ai_usage_daily").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := &domain.AIUsageTotals{}
	if cur.Next(ctx) {
		var d struct {
			Calls        int64  `bson:"calls"`
			Errors       int64  `bson:"errors"`
			PromptTokens int64  `bson:"prompt_tokens"`
			CompTokens   int64  `bson:"comp_tokens"`
			FirstDay     string `bson:"first_day"`
		}
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out.Calls, out.Errors = d.Calls, d.Errors
		out.PromptTokens, out.CompTokens = d.PromptTokens, d.CompTokens
		out.FirstDay = d.FirstDay
	}
	return out, cur.Err()
}

func (r aiLogRepo) UsageDays(ctx context.Context, from, to string, limit int) ([]domain.AIUsageDay, error) {
	limit = domain.ListLimit(limit, domain.UsageDaysDefault, domain.UsageDaysMax)
	return r.usageFold(ctx, from, to, "$day", bson.D{{Key: "_id", Value: -1}}, int64(limit), func(id string, d *domain.AIUsageDay) {
		d.Day = id
	})
}

func (r aiLogRepo) UsageByModel(ctx context.Context, from, to string) ([]domain.AIUsageDay, error) {
	return r.usageFold(ctx, from, to, "$model",
		bson.D{{Key: "tokens", Value: -1}, {Key: "_id", Value: 1}}, 0, func(id string, d *domain.AIUsageDay) {
			d.Model = id
		})
}

// usageFold is the one grouping pipeline both folds use.
//
// ⚠️ `by` is a field path chosen by THIS package from two literals, never by a
// caller. The same rule as the table browser: a group key is an identifier, and
// an identifier that came in from outside is the thing this codebase does not
// let happen.
func (r aiLogRepo) usageFold(ctx context.Context, from, to, by string, sort bson.D, limit int64,
	assign func(string, *domain.AIUsageDay)) ([]domain.AIUsageDay, error) {
	pipe := mongo.Pipeline{
		{{Key: "$match", Value: aiUsageWindowDoc(from, to)}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: by},
			{Key: "calls", Value: bson.M{"$sum": "$calls"}},
			{Key: "errors", Value: bson.M{"$sum": "$errors"}},
			{Key: "prompt_tokens", Value: bson.M{"$sum": "$prompt_tokens"}},
			{Key: "comp_tokens", Value: bson.M{"$sum": "$comp_tokens"}},
			{Key: "tokens", Value: bson.M{"$sum": bson.M{"$add": bson.A{"$prompt_tokens", "$comp_tokens"}}}},
		}}},
		{{Key: "$sort", Value: sort}},
	}
	if limit > 0 {
		pipe = append(pipe, bson.D{{Key: "$limit", Value: limit}})
	}
	cur, err := r.c("ai_usage_daily").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.AIUsageDay{}
	for cur.Next(ctx) {
		var d struct {
			ID           string `bson:"_id"`
			Calls        int64  `bson:"calls"`
			Errors       int64  `bson:"errors"`
			PromptTokens int64  `bson:"prompt_tokens"`
			CompTokens   int64  `bson:"comp_tokens"`
		}
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		row := domain.AIUsageDay{
			Calls: d.Calls, Errors: d.Errors,
			PromptTokens: d.PromptTokens, CompTokens: d.CompTokens,
		}
		assign(d.ID, &row)
		out = append(out, row)
	}
	return out, cur.Err()
}
