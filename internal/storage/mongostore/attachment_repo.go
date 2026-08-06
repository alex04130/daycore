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

// Field names mirror the SQL columns, same rule as everywhere else in this
// package: two schemas that differ only in spelling cost more than the tidiness
// is worth when someone is debugging with a shell open.
//
// message_id and thread_id are stored as "" rather than omitted when unbound.
// omitempty would make `{"message_id": ""}` match nothing on Mongo while the
// identical predicate matches every unbound row on SQL — the exact class of
// silent divergence the conformance suite exists to catch, and one that would
// have made the sweeper a no-op on one backend only.
type attachmentDoc struct {
	ID        string `bson:"_id"`
	SessionID string `bson:"session_id"`
	ThreadID  string `bson:"thread_id"`
	MessageID string `bson:"message_id"`
	Ref       string `bson:"ref"`
	Kind      string `bson:"kind"`
	MIME      string `bson:"mime"`
	Size      int64  `bson:"size"`
	SHA256    string `bson:"sha256"`
	Filename  string `bson:"filename"`
	CreatedAt int64  `bson:"created_at"`
}

func (d attachmentDoc) toDomain() domain.Attachment {
	return domain.Attachment{
		ID: d.ID, SessionID: d.SessionID, ThreadID: d.ThreadID, MessageID: d.MessageID,
		Ref: d.Ref, Kind: d.Kind, MIME: d.MIME, Size: d.Size, SHA256: d.SHA256,
		Filename: d.Filename, CreatedAt: fromMillis(d.CreatedAt),
	}
}

type attachmentRepo struct{ *Store }

func (r attachmentRepo) coll() *mongo.Collection { return r.c("attachments") }

func (r attachmentRepo) Create(ctx context.Context, a *domain.Attachment) (*domain.Attachment, error) {
	if a.SessionID == "" {
		return nil, errors.New("attachment: session id required")
	}
	if a.Ref == "" {
		return nil, errors.New("attachment: ref required")
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.Kind == "" {
		a.Kind = domain.AttachmentKindOf(a.MIME)
	}
	now := nowMillis()
	a.CreatedAt = fromMillis(now)
	_, err := r.coll().InsertOne(ctx, attachmentDoc{
		ID: a.ID, SessionID: a.SessionID, ThreadID: a.ThreadID, MessageID: a.MessageID,
		Ref: a.Ref, Kind: a.Kind, MIME: a.MIME, Size: a.Size, SHA256: a.SHA256,
		Filename: a.Filename, CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r attachmentRepo) Get(ctx context.Context, sessionID, id string) (*domain.Attachment, error) {
	var d attachmentDoc
	err := r.coll().FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out := d.toDomain()
	return &out, nil
}

func (r attachmentRepo) Bind(ctx context.Context, sessionID, threadID, messageID string, ids []string) error {
	if messageID == "" {
		return errors.New("attachment: message id required")
	}
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > domain.AttachmentsPerMessage {
		return domain.ErrTooManyAttachments
	}
	for _, id := range ids {
		res, err := r.coll().UpdateOne(ctx,
			bson.M{"_id": id, "session_id": sessionID, "message_id": ""},
			bson.M{"$set": bson.M{"thread_id": threadID, "message_id": messageID}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			cur, gerr := r.Get(ctx, sessionID, id)
			if gerr == nil && cur.Bound() {
				if cur.MessageID == messageID {
					continue // idempotent retry of the same bind
				}
				return domain.ErrAttachmentBound
			}
			return domain.ErrNotFound
		}
	}
	return nil
}

func (r attachmentRepo) ListByMessages(ctx context.Context, sessionID string, messageIDs []string) ([]domain.Attachment, error) {
	ids := dedupeNonEmpty(messageIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	cur, err := r.coll().Find(ctx,
		bson.M{"session_id": sessionID, "message_id": bson.M{"$in": ids}},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return collectAttachments(ctx, cur)
}

func (r attachmentRepo) ListUnbound(ctx context.Context, sessionID string, limit int) ([]domain.Attachment, error) {
	limit = domain.ListLimit(limit, domain.AttachmentListDefault, domain.AttachmentListMax)
	cur, err := r.coll().Find(ctx,
		bson.M{"session_id": sessionID, "message_id": ""},
		options.Find().
			SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: 1}}).
			SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	return collectAttachments(ctx, cur)
}

func (r attachmentRepo) Delete(ctx context.Context, sessionID, id string) (*domain.Attachment, error) {
	a, err := r.Get(ctx, sessionID, id)
	if err != nil {
		return nil, err
	}
	if a.Bound() {
		return nil, domain.ErrAttachmentBound
	}
	res, err := r.coll().DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID, "message_id": ""})
	if err != nil {
		return nil, err
	}
	if res.DeletedCount == 0 {
		return nil, domain.ErrAttachmentBound
	}
	return a, nil
}

func (r attachmentRepo) DeleteByThread(ctx context.Context, sessionID, threadID string) ([]domain.Attachment, error) {
	if threadID == "" {
		return nil, nil
	}
	filter := bson.M{"session_id": sessionID, "thread_id": threadID}
	cur, err := r.coll().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	out, err := collectAttachments(ctx, cur)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if _, err := r.coll().DeleteMany(ctx, filter); err != nil {
		return nil, err
	}
	return out, nil
}

func (r attachmentRepo) PruneUnbound(ctx context.Context, before time.Time) ([]domain.Attachment, error) {
	filter := bson.M{"message_id": "", "created_at": bson.M{"$lt": before.UnixMilli()}}
	cur, err := r.coll().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	out, err := collectAttachments(ctx, cur)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	// Same predicate again rather than a delete by the ids just read, so an
	// attachment bound in between survives. See the sqlstore twin.
	if _, err := r.coll().DeleteMany(ctx, filter); err != nil {
		return nil, err
	}
	return out, nil
}

func collectAttachments(ctx context.Context, cur *mongo.Cursor) ([]domain.Attachment, error) {
	defer cur.Close(ctx)
	var out []domain.Attachment
	for cur.Next(ctx) {
		var d attachmentDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toDomain())
	}
	return out, cur.Err()
}

func dedupeNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
