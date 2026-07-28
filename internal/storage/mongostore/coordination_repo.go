package mongostore

import (
	"context"
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
		return nil, false, nil
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

type jobRunDoc struct {
	ID        string     `bson:"_id"`
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

type jobRunRepo struct{ *Store }

// Claim relies on the unique index over (session_id, job_name, run_key) exactly
// as the SQL side does: the insert IS the claim. Write first, work second — the
// other order leaves open the window this exists to close.
func (r jobRunRepo) Claim(ctx context.Context, run *domain.JobRun) (bool, error) {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	run.Status = domain.JobRunning
	run.StartedAt = now
	if run.Attempts == 0 {
		run.Attempts = 1
	}

	_, err := r.c("job_runs").InsertOne(ctx, jobRunDoc{
		ID: run.ID, SessionID: run.SessionID, Job: run.Job, RunKey: run.RunKey,
		Status: string(domain.JobRunning), Instance: run.Instance,
		StartedAt: now, Attempts: run.Attempts,
	})
	if err == nil {
		return true, nil
	}

	// Lost the race. Take over a claim stuck in "running" past when its owner
	// could still be working, or a "failed" one with retries left — the same two
	// cases the SQL side allows.
	//
	// The takeover must ROTATE the id, as the SQL UPDATE does, and _id is
	// immutable in Mongo — so this is a delete of the old document followed by
	// an insert of ours. Keeping the old _id and handing it back would let the
	// previous owner, waking up late, call Finish and stamp ITS verdict on OUR
	// run: a job that is running fine gets marked failed by a zombie. Losing the
	// delete race just means somebody else took over first, which is a refusal,
	// not an error.
	del := r.c("job_runs").FindOneAndDelete(ctx, bson.M{
		"session_id": run.SessionID, "job_name": run.Job, "run_key": run.RunKey,
		"$or": []bson.M{
			{"status": string(domain.JobRunning), "started_at": bson.M{"$lt": now.Add(-domain.JobStaleAfter)}},
			{"status": string(domain.JobFailed), "attempts": bson.M{"$lt": domain.JobMaxAttempts}},
		},
	})
	var old jobRunDoc
	if derr := del.Decode(&old); derr != nil {
		if notFound(derr) {
			return false, nil
		}
		return false, derr
	}
	run.Attempts = old.Attempts + 1
	if _, ierr := r.c("job_runs").InsertOne(ctx, jobRunDoc{
		ID: run.ID, SessionID: run.SessionID, Job: run.Job, RunKey: run.RunKey,
		Status: string(domain.JobRunning), Instance: run.Instance,
		StartedAt: now, Attempts: run.Attempts,
	}); ierr != nil {
		return false, ierr
	}
	return true, nil
}

func (r jobRunRepo) Finish(ctx context.Context, id string, status domain.JobStatus, errText string, at time.Time) error {
	_, err := r.c("job_runs").UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": string(status), "ended_at": at, "error_text": errText}})
	return err
}

func (r jobRunRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.JobRun, error) {
	if limit <= 0 {
		limit = 50
	}
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
			ID: d.ID, SessionID: d.SessionID, Job: d.Job, RunKey: d.RunKey,
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
