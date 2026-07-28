package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The field names here match the SQL column names rather than the Go field
// names, so the two stores read alike when someone is debugging with a shell
// open — including rows_json and start_time, whose SQL spellings exist to dodge
// MySQL reserved words. Mongo does not care, but two schemas that differ only
// in spelling cost more than the tidiness is worth.
type proposalDoc struct {
	ID        string `bson:"_id"`
	SessionID string `bson:"session_id"`
	State     string `bson:"state"`
	Level     string `bson:"level"`
	Kind      string `bson:"kind"`

	Title      string               `bson:"title"`
	Summary    string               `bson:"summary,omitempty"`
	Reason     string               `bson:"reason,omitempty"`
	Evidence   string               `bson:"evidence,omitempty"`
	Date       string               `bson:"date,omitempty"`
	Start      string               `bson:"start_time,omitempty"`
	Dur        *int                 `bson:"duration_min,omitempty"`
	BType      string               `bson:"block_type,omitempty"`
	LockLevel  string               `bson:"lock_level,omitempty"`
	LockReason string               `bson:"lock_reason,omitempty"`
	Rows       []domain.ProposalRow `bson:"rows_json"`

	MergeKey     string     `bson:"merge_key,omitempty"`
	DeliverAfter *time.Time `bson:"deliver_after,omitempty"`
	DeliveredAt  *time.Time `bson:"delivered_at,omitempty"`
	PushedAt     *time.Time `bson:"pushed_at,omitempty"`

	TTLPolicy  string    `bson:"ttl_policy"`
	ExpiresAt  time.Time `bson:"expires_at"`
	Resolution string    `bson:"resolution,omitempty"`

	Origin        string              `bson:"origin,omitempty"`
	ThreadID      string              `bson:"thread_id,omitempty"`
	Ops           []domain.ProposalOp `bson:"ops_json"`
	AppliedOpIDs  []string            `bson:"applied_op_ids"`
	AcceptOpIDs   []string            `bson:"accept_op_ids"`
	OwnerInstance string              `bson:"owner_instance,omitempty"`
	Rev           int                 `bson:"rev"`
	CreatedAt     time.Time           `bson:"created_at"`
	UpdatedAt     time.Time           `bson:"updated_at"`
}

type proposalRepo struct{ *Store }

func (r proposalRepo) Create(ctx context.Context, p *domain.Proposal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.State == "" {
		p.State = domain.ProposalPending
	}
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	p.Rev = 1
	_, err := r.c("proposals").InsertOne(ctx, docFromProposal(*p))
	return err
}

func (r proposalRepo) Get(ctx context.Context, sessionID, id string) (*domain.Proposal, error) {
	var d proposalDoc
	if err := r.c("proposals").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p := proposalFromDoc(d)
	return &p, nil
}

func (r proposalRepo) List(ctx context.Context, f domain.ProposalFilter) ([]domain.Proposal, error) {
	filter := bson.M{"session_id": f.SessionID}
	if f.State != "" {
		filter["state"] = string(f.State)
	}
	if f.Kind != "" {
		filter["kind"] = string(f.Kind)
	}
	if f.Date != "" {
		filter["date"] = f.Date
	}
	if f.Undelivered {
		filter["delivered_at"] = bson.M{"$exists": false}
	}
	if !f.DeliverableAt.IsZero() {
		filter["expires_at"] = bson.M{"$gt": f.DeliverableAt}
		filter["$or"] = []bson.M{
			{"deliver_after": bson.M{"$exists": false}},
			{"deliver_after": bson.M{"$lte": f.DeliverableAt}},
		}
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	cur, err := r.c("proposals").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Proposal{}
	for cur.Next(ctx) {
		var d proposalDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, proposalFromDoc(d))
	}
	return out, cur.Err()
}

// Update is a compare-and-set on rev, matching the SQL side.
//
// It is $set over an explicit field list rather than ReplaceOne, and the list is
// the same one the SQL UPDATE writes. A whole-document replace would also write
// the fields the SQL statement deliberately leaves alone — level, kind, origin,
// session_id, created_at — so a caller that mutated one of those would see it
// take effect on Mongo and be silently ignored on SQL. Those five are identity:
// a card does not change which ladder rung it is on or who produced it, and if
// that ever needs to change it is a new card.
func (r proposalRepo) Update(ctx context.Context, p *domain.Proposal) error {
	now := time.Now().UTC()
	d := docFromProposal(*p)
	set := bson.M{
		"state": d.State, "title": d.Title, "summary": d.Summary,
		"reason": d.Reason, "evidence": d.Evidence,
		"date": d.Date, "start_time": d.Start, "duration_min": d.Dur,
		"block_type": d.BType, "lock_level": d.LockLevel, "lock_reason": d.LockReason,
		"rows_json": d.Rows, "ops_json": d.Ops,
		"applied_op_ids": d.AppliedOpIDs, "accept_op_ids": d.AcceptOpIDs,
		"merge_key":  d.MergeKey,
		"ttl_policy": d.TTLPolicy, "expires_at": d.ExpiresAt, "resolution": d.Resolution,
		"owner_instance": d.OwnerInstance,
		"updated_at":     now,
	}
	// The three nullable timestamps are $set when present and $unset when not,
	// because the undelivered filter tests for absence — leaving a stale
	// delivered_at behind would take a card out of the pool for good.
	unset := bson.M{}
	for field, v := range map[string]*time.Time{
		"deliver_after": d.DeliverAfter, "delivered_at": d.DeliveredAt, "pushed_at": d.PushedAt,
	} {
		if v != nil {
			set[field] = *v
		} else {
			unset[field] = ""
		}
	}
	update := bson.M{"$set": set, "$inc": bson.M{"rev": 1}}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	res, err := r.c("proposals").UpdateOne(ctx,
		bson.M{"_id": p.ID, "session_id": p.SessionID, "rev": p.Rev}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrConflict
	}
	p.Rev++
	p.UpdatedAt = now
	return nil
}

func (r proposalRepo) Expire(ctx context.Context, now time.Time) (int, error) {
	total := 0
	for _, c := range []struct {
		policy domain.ProposalTTL
		state  domain.ProposalState
	}{
		{domain.TTLSilenceAccepts, domain.ProposalAccepted},
		{domain.TTLSilenceRejects, domain.ProposalExpired},
	} {
		res, err := r.c("proposals").UpdateMany(ctx,
			bson.M{
				"state":      string(domain.ProposalPending),
				"ttl_policy": string(c.policy),
				"expires_at": bson.M{"$lte": now},
			},
			bson.M{
				"$set": bson.M{"state": string(c.state), "resolution": string(domain.ResolutionSilence), "updated_at": now},
				"$inc": bson.M{"rev": 1},
			})
		if err != nil {
			return total, err
		}
		total += int(res.ModifiedCount)
	}
	return total, nil
}

func (r proposalRepo) Supersede(ctx context.Context, sessionID, mergeKey, keepID string) (int, error) {
	if mergeKey == "" {
		return 0, nil
	}
	// Read the keeper's creation time first so the retirement can be expressed
	// as "older than this one". Retiring everything that merely is not the
	// keeper lets two daemons annihilate each other's card and leave the user
	// with nothing — see the interface comment.
	var keeper proposalDoc
	if err := r.c("proposals").FindOne(ctx, bson.M{"_id": keepID, "session_id": sessionID}).Decode(&keeper); err != nil {
		if notFound(err) {
			// A keeper that no longer exists is nothing to merge against — the
			// same non-event as an empty merge key. The SQL side reaches the
			// same conclusion by way of a NULL comparison; both must answer
			// (0, nil) or a caller that aborts on ErrNotFound would abort on
			// one backend and proceed on the other for identical data.
			return 0, nil
		}
		return 0, err
	}
	now := time.Now().UTC()
	res, err := r.c("proposals").UpdateMany(ctx,
		bson.M{
			"session_id": sessionID, "merge_key": mergeKey,
			"_id":          bson.M{"$ne": keepID},
			"state":        string(domain.ProposalPending),
			"delivered_at": bson.M{"$exists": false},
			// A total order: older, or the same instant and a lower id. Strict
			// "older" alone leaves two same-millisecond cards both alive.
			"$or": []bson.M{
				{"created_at": bson.M{"$lt": keeper.CreatedAt}},
				{"created_at": keeper.CreatedAt, "_id": bson.M{"$lt": keepID}},
			},
		},
		bson.M{
			"$set": bson.M{
				"state":      string(domain.ProposalExpired),
				"resolution": string(domain.ResolutionSuperseded),
				"updated_at": now,
			},
			"$inc": bson.M{"rev": 1},
		})
	if err != nil {
		return 0, err
	}
	return int(res.ModifiedCount), nil
}

func docFromProposal(p domain.Proposal) proposalDoc {
	// nil slices become empty ones, so a round trip never turns [] into null —
	// the same normalisation the SQL side does before marshalling.
	rows := p.Rows
	if rows == nil {
		rows = []domain.ProposalRow{}
	}
	ops := p.Ops
	if ops == nil {
		ops = []domain.ProposalOp{}
	}
	applied := p.AppliedOpIDs
	if applied == nil {
		applied = []string{}
	}
	accept := p.AcceptOpIDs
	if accept == nil {
		accept = []string{}
	}
	return proposalDoc{
		ID: p.ID, SessionID: p.SessionID,
		State: string(p.State), Level: string(p.Level), Kind: string(p.Kind),
		Title: p.Title, Summary: p.Summary, Reason: p.Reason, Evidence: p.Evidence,
		Date: p.Date, Start: p.Start, Dur: p.Dur,
		BType: string(p.BType), LockLevel: string(p.LockLevel), LockReason: p.LockReason,
		Rows:     rows,
		MergeKey: p.MergeKey, DeliverAfter: p.DeliverAfter, DeliveredAt: p.DeliveredAt, PushedAt: p.PushedAt,
		TTLPolicy: string(p.TTLPolicy), ExpiresAt: p.ExpiresAt, Resolution: string(p.Resolution),
		Origin: string(p.Origin), ThreadID: p.ThreadID,
		Ops: ops, AppliedOpIDs: applied, AcceptOpIDs: accept,
		OwnerInstance: p.OwnerInstance, Rev: p.Rev,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func proposalFromDoc(d proposalDoc) domain.Proposal {
	return domain.Proposal{
		ID: d.ID, SessionID: d.SessionID,
		State: domain.ProposalState(d.State), Level: domain.ProposalLevel(d.Level), Kind: domain.ProposalKind(d.Kind),
		Title: d.Title, Summary: d.Summary, Reason: d.Reason, Evidence: d.Evidence,
		Date: d.Date, Start: d.Start, Dur: d.Dur,
		BType: domain.BlockType(d.BType), LockLevel: domain.LockLevel(d.LockLevel), LockReason: d.LockReason,
		Rows:     d.Rows,
		MergeKey: d.MergeKey, DeliverAfter: d.DeliverAfter, DeliveredAt: d.DeliveredAt, PushedAt: d.PushedAt,
		TTLPolicy: domain.ProposalTTL(d.TTLPolicy), ExpiresAt: d.ExpiresAt,
		Resolution: domain.ProposalResolution(d.Resolution),
		Origin:     domain.ProposalOrigin(d.Origin), ThreadID: d.ThreadID,
		Ops: d.Ops, AppliedOpIDs: d.AppliedOpIDs, AcceptOpIDs: d.AcceptOpIDs,
		OwnerInstance: d.OwnerInstance, Rev: d.Rev,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}
