package mongostore

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ── leases ──────────────────────────────────────────────────────────────────

type leaseDoc struct {
	Name       string    `bson:"_id"`
	Holder     string    `bson:"holder"`
	AcquiredAt time.Time `bson:"acquired_at"`
	ExpiresAt  time.Time `bson:"expires_at"`
	Fence      int64     `bson:"fence"`
}

type leaseRepo struct{ *Store }

// Acquire mirrors the SQL side: one conditional write, never read-then-write.
// FindOneAndUpdate with a filter that encodes the precondition is Mongo's
// version of the same guarantee — the match and the update are atomic on the
// document, so two instances cannot both come back holding it.
func (r leaseRepo) Acquire(ctx context.Context, name, holder string, ttl time.Duration, now time.Time) (*domain.Lease, bool, error) {
	// Same contract as the SQL side: a holder identifies one PROCESS. Two
	// instances sharing the string both match the renewal filter and both keep
	// coming back true forever, with the fence never moving.
	if holder == "" {
		return nil, false, errors.New("lease: holder must be a per-process identifier, not empty")
	}
	expires := now.Add(ttl)
	after := options.After

	// Renewal or take-over in a single statement: ours-and-live, or lapsed.
	// The fence rises only on a take-over, so a stalled holder can tell "still
	// mine" from "mine again after someone else had it" — which its own clock
	// cannot tell it.
	renew := r.c("leases").FindOneAndUpdate(ctx,
		bson.M{"_id": name, "holder": holder, "expires_at": bson.M{"$gt": now}},
		bson.M{"$set": bson.M{"expires_at": expires}},
		options.FindOneAndUpdate().SetReturnDocument(after))
	var d leaseDoc
	if err := renew.Decode(&d); err == nil {
		return leaseFromDoc(d), true, nil
	} else if !notFound(err) {
		return nil, false, err
	}

	takeover := r.c("leases").FindOneAndUpdate(ctx,
		bson.M{"_id": name, "expires_at": bson.M{"$lte": now}},
		bson.M{
			"$set": bson.M{"holder": holder, "acquired_at": now, "expires_at": expires},
			"$inc": bson.M{"fence": 1},
		},
		options.FindOneAndUpdate().SetReturnDocument(after))
	if err := takeover.Decode(&d); err == nil {
		return leaseFromDoc(d), true, nil
	} else if !notFound(err) {
		return nil, false, err
	}

	// Never claimed before. A concurrent insert loses on _id, which is the
	// right answer: it simply did not win.
	if _, err := r.c("leases").InsertOne(ctx, leaseDoc{
		Name: name, Holder: holder, AcquiredAt: now, ExpiresAt: expires, Fence: 1,
	}); err != nil {
		// Distinguish "somebody inserted first" from "the primary is
		// unreachable". Swallowing both as a lost election makes an outage look
		// like an ordinary hand-over: the worker goes quiet and nothing says why.
		// Reading the row back is driver-error-free — a row that now exists means
		// we simply lost, and the caller gets the row naming the real holder
		// rather than a nil the SQL side would never return.
		if l, gerr := r.Get(ctx, name); gerr == nil {
			return l, l.Held(holder, now), nil
		}
		return nil, false, err
	}
	return &domain.Lease{Name: name, Holder: holder, AcquiredAt: now, ExpiresAt: expires, Fence: 1}, true, nil
}

func (r leaseRepo) Release(ctx context.Context, name, holder string) error {
	// Expire rather than delete, so the fence survives a hand-back.
	_, err := r.c("leases").UpdateOne(ctx,
		bson.M{"_id": name, "holder": holder},
		bson.M{"$set": bson.M{"expires_at": time.Unix(0, 0).UTC()}})
	return err
}

func (r leaseRepo) Get(ctx context.Context, name string) (*domain.Lease, error) {
	var d leaseDoc
	if err := r.c("leases").FindOne(ctx, bson.M{"_id": name}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return leaseFromDoc(d), nil
}

func leaseFromDoc(d leaseDoc) *domain.Lease {
	return &domain.Lease{
		Name: d.Name, Holder: d.Holder,
		AcquiredAt: d.AcquiredAt, ExpiresAt: d.ExpiresAt, Fence: d.Fence,
	}
}

// ── job runs ────────────────────────────────────────────────────────────────

// _id is the OCCURRENCE, not the claim: "<session>:<job>:<runKey>". That makes
// the insert-as-claim mechanism the primary key itself rather than a secondary
// unique index, and — the reason it matters here — it lets a takeover rotate the
// claim id inside one atomic update.
//
// The SQL side rotates the row's primary key in its takeover UPDATE, so a
// previous owner waking up late calls Finish with an id that matches nothing and
// its stale verdict is a no-op. Mongo cannot mutate _id, and delete-then-insert
// is not the same thing: between the two statements the occurrence does not
// exist, so a cancelled context there destroys the record of the attempts that
// came before — including the "running" row that Prune deliberately keeps as the
// only trace of a job that crashed and never came back.
type jobRunDoc struct {
	Key string `bson:"_id"` // session:job:runKey
	// ClaimID is what Finish matches on: the rotating half.
	ClaimID   string     `bson:"claim_id"`
	SessionID string     `bson:"session_id"`
	Job       string     `bson:"job_name"`
	RunKey    string     `bson:"run_key"`
	Status    string     `bson:"status"`
	Instance  string     `bson:"instance"`
	StartedAt time.Time  `bson:"started_at"`
	EndedAt   *time.Time `bson:"ended_at,omitempty"`
	Attempts  int        `bson:"attempts"`
	Error     string     `bson:"error_text,omitempty"`
}

func jobRunKey(sessionID, job, runKey string) string {
	return sessionID + ":" + job + ":" + runKey
}

type jobRunRepo struct{ *Store }

// Claim relies on _id being the occurrence: the insert IS the claim, and a
// duplicate-key failure means somebody else already owns it. Write first, work
// second — the other order leaves open the window this exists to close.
func (r jobRunRepo) Claim(ctx context.Context, run *domain.JobRun) (bool, error) {
	// Always fresh: the id is a claim token, not caller identity. See the SQL
	// side for what a reused struct would do.
	run.ID = uuid.NewString()
	now := time.Now().UTC()
	run.Status = domain.JobRunning
	run.StartedAt = now
	if run.Attempts == 0 {
		run.Attempts = 1
	}

	_, err := r.c("job_runs").InsertOne(ctx, jobRunDoc{
		Key: jobRunKey(run.SessionID, run.Job, run.RunKey), ClaimID: run.ID,
		SessionID: run.SessionID, Job: run.Job, RunKey: run.RunKey,
		Status: string(domain.JobRunning), Instance: run.Instance,
		StartedAt: now, Attempts: run.Attempts,
	})
	if err == nil {
		return true, nil
	}

	// Lost the race. Take over a claim stuck in "running" past when its owner
	// could still be working, or one that failed with retries left — the same two
	// branches, with the same two caps, as the SQL side.
	//
	// One atomic update, rotating claim_id. No delete: the occurrence row must
	// never stop existing, because it carries the attempt history and, when a job
	// dies for good, it is the only record that it ever ran.
	res := r.c("job_runs").FindOneAndUpdate(ctx,
		bson.M{
			"_id": jobRunKey(run.SessionID, run.Job, run.RunKey),
			"$or": []bson.M{
				{
					"status":     string(domain.JobRunning),
					"started_at": bson.M{"$lt": now.Add(-domain.JobStaleAfter)},
					"attempts":   bson.M{"$lt": domain.JobMaxCrashAttempts},
				},
				{
					"status":   string(domain.JobFailed),
					"attempts": bson.M{"$lt": domain.JobMaxAttempts},
				},
			},
		},
		bson.M{
			"$set": bson.M{
				"claim_id": run.ID, "instance": run.Instance,
				"started_at": now, "status": string(domain.JobRunning),
			},
			"$inc":   bson.M{"attempts": 1},
			"$unset": bson.M{"ended_at": "", "error_text": ""},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After))
	var d jobRunDoc
	if derr := res.Decode(&d); derr != nil {
		if notFound(derr) {
			return false, nil
		}
		return false, derr
	}
	run.Attempts = d.Attempts
	return true, nil
}

// Finish matches on claim_id, not _id: a previous owner waking up late holds a
// claim id that a takeover has since rotated away, so its stale verdict lands on
// nothing instead of marking a healthy run failed.
func (r jobRunRepo) Finish(ctx context.Context, id string, status domain.JobStatus, errText string, at time.Time) error {
	_, err := r.c("job_runs").UpdateOne(ctx, bson.M{"claim_id": id},
		bson.M{"$set": bson.M{"status": string(status), "ended_at": at, "error_text": errText}})
	return err
}

func (r jobRunRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.JobRun, error) {
	limit = domain.ListLimit(limit, domain.JobRunListDefault, domain.JobRunListMax)
	cur, err := r.c("job_runs").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.JobRun{}
	for cur.Next(ctx) {
		var d jobRunDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.JobRun{
			ID: d.ClaimID, SessionID: d.SessionID, Job: d.Job, RunKey: d.RunKey,
			Status: domain.JobStatus(d.Status), Instance: d.Instance,
			StartedAt: d.StartedAt, EndedAt: d.EndedAt, Attempts: d.Attempts, Error: d.Error,
		})
	}
	return out, cur.Err()
}

// Prune leaves "running" rows alone: one older than the cutoff is the only
// trace of a job that crashed and never came back.
func (r jobRunRepo) Prune(ctx context.Context, before time.Time) (int, error) {
	res, err := r.c("job_runs").DeleteMany(ctx, bson.M{
		"status":     bson.M{"$ne": string(domain.JobRunning)},
		"started_at": bson.M{"$lt": before},
	})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}
