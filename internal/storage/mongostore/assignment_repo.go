package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type assignmentDoc struct {
	ID             string     `bson:"_id"`
	SessionID      string     `bson:"session_id"`
	CourseID       string     `bson:"course_id"`
	CanvasID       string     `bson:"canvas_id"`
	Title          string     `bson:"title"`
	DueAt          *time.Time `bson:"due_at,omitempty"`
	PointsPossible *float64   `bson:"points_possible,omitempty"`
	Submitted      bool       `bson:"submitted"`
	Graded         bool       `bson:"graded"`
	Score          *float64   `bson:"score,omitempty"`
	HTMLURL        string     `bson:"html_url"`
	Source         string     `bson:"source"`
	Status         string     `bson:"status"`
	RemindersOff   bool       `bson:"reminders_off"`
	CreatedAt      time.Time  `bson:"created_at"`
	UpdatedAt      time.Time  `bson:"updated_at"`
}

func (d assignmentDoc) toDomain() *domain.Assignment {
	return &domain.Assignment{
		ID: d.ID, SessionID: d.SessionID, CourseID: d.CourseID, CanvasID: d.CanvasID,
		Title: d.Title, DueAt: d.DueAt, PointsPossible: d.PointsPossible,
		Submitted: d.Submitted, Graded: d.Graded, Score: d.Score, HTMLURL: d.HTMLURL,
		Source: d.Source, Status: d.Status, RemindersOff: d.RemindersOff,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type assignmentRepo struct{ *Store }

func (r assignmentRepo) Get(ctx context.Context, sessionID, id string) (*domain.Assignment, error) {
	var d assignmentDoc
	if err := r.c("assignments").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r assignmentRepo) List(ctx context.Context, sessionID string, f domain.AssignmentFilter) ([]domain.Assignment, error) {
	filter := bson.M{"session_id": sessionID}
	due := bson.M{}
	if f.DueFrom != nil {
		due["$gte"] = *f.DueFrom
	}
	if f.DueTo != nil {
		due["$lte"] = *f.DueTo
	}
	if len(due) > 0 {
		filter["due_at"] = due
	}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	cur, err := r.c("assignments").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "due_at", Value: 1}, {Key: "title", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Assignment{}
	for cur.Next(ctx) {
		var d assignmentDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r assignmentRepo) UpsertByCanvasID(ctx context.Context, a *domain.Assignment) (*domain.Assignment, error) {
	if a.CanvasID == "" {
		return nil, domain.ErrMissingUpsertKey
	}
	now := time.Now().UTC()
	status := a.Status
	if status == "" {
		status = domain.AssignmentPending
	}
	filter := bson.M{"session_id": a.SessionID, "canvas_id": a.CanvasID}
	// Status is deliberately NOT refreshed: it tracks the local planner workflow.
	_, err := r.c("assignments").UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"course_id": a.CourseID, "title": a.Title, "due_at": a.DueAt,
			"points_possible": a.PointsPossible, "submitted": a.Submitted, "graded": a.Graded,
			"score": a.Score, "html_url": a.HTMLURL, "source": a.Source, "updated_at": now,
		},
		"$setOnInsert": bson.M{"_id": uuid.NewString(), "status": status, "created_at": now},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return nil, err
	}
	var d assignmentDoc
	if err := r.c("assignments").FindOne(ctx, filter).Decode(&d); err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r assignmentRepo) SetStatus(ctx context.Context, sessionID, id, status string) error {
	res, err := r.c("assignments").UpdateOne(ctx, bson.M{"_id": id, "session_id": sessionID},
		bson.M{"$set": bson.M{"status": status, "updated_at": time.Now().UTC()}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetReminders silences or restores the deadline ladder for one item. Separate
// from SetStatus because they mean different things: status is the planner
// workflow, this is whether the fact track may speak.
func (r assignmentRepo) SetReminders(ctx context.Context, sessionID, id string, on bool) error {
	res, err := r.c("assignments").UpdateOne(ctx, bson.M{"_id": id, "session_id": sessionID},
		bson.M{"$set": bson.M{"reminders_off": !on, "updated_at": time.Now().UTC()}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r assignmentRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.c("assignments").DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
