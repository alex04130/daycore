package mongostore

import (
	"context"
	"strconv"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type chatThreadDoc struct {
	ID        string `bson:"_id"`
	SessionID string `bson:"session_id"`
	Title     string `bson:"title"`
	Summary   string `bson:"summary,omitempty"`
	Archived  bool   `bson:"archived"`
	CreatedAt int64  `bson:"created_at"`
	UpdatedAt int64  `bson:"updated_at"`
}

type chatMsgDoc struct {
	ID         string `bson:"_id"`
	ThreadID   string `bson:"thread_id"`
	SessionID  string `bson:"session_id"`
	Role       string `bson:"role"`
	Content    string `bson:"content"`
	ToolEvents string `bson:"tool_events,omitempty"`
	Status     string `bson:"status,omitempty"`
	CreatedAt  int64  `bson:"created_at"`
}

func (d chatMsgDoc) toDomain() domain.ChatMessage {
	return domain.ChatMessage{
		ID: d.ID, ThreadID: d.ThreadID, SessionID: d.SessionID,
		Role: d.Role, Content: d.Content, ToolEvents: d.ToolEvents,
		Status: d.Status, CreatedAt: fromMillis(d.CreatedAt),
	}
}

type chatRepo struct{ *Store }

func (r chatRepo) ListThreads(ctx context.Context, sessionID string) ([]domain.ChatThread, error) {
	cur, err := r.c("chat_threads").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.ChatThread
	for cur.Next(ctx) {
		var d chatThreadDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.ChatThread{
			ID: d.ID, SessionID: d.SessionID, Title: d.Title, Summary: d.Summary,
			Archived: d.Archived, CreatedAt: fromMillis(d.CreatedAt), UpdatedAt: fromMillis(d.UpdatedAt),
		})
	}
	return out, cur.Err()
}

func (r chatRepo) CreateThread(ctx context.Context, t *domain.ChatThread) (*domain.ChatThread, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.c("chat_threads").InsertOne(ctx, chatThreadDoc{t.ID, t.SessionID, t.Title, t.Summary, t.Archived, now, now})
	if err != nil {
		return nil, err
	}
	t.CreatedAt = fromMillis(now)
	t.UpdatedAt = fromMillis(now)
	return t, nil
}

func (r chatRepo) UpdateThread(ctx context.Context, sessionID, id string, upd domain.ChatThreadUpdate) (*domain.ChatThread, error) {
	set := bson.M{"updated_at": nowMillis()}
	if upd.Title != nil {
		set["title"] = *upd.Title
	}
	if upd.Summary != nil {
		set["summary"] = *upd.Summary
	}
	if upd.Archived != nil {
		set["archived"] = *upd.Archived
	}
	res := r.c("chat_threads").FindOneAndUpdate(ctx, bson.M{"_id": id, "session_id": sessionID}, bson.M{"$set": set}, options.FindOneAndUpdate().SetReturnDocument(options.After))
	if notFound(res.Err()) {
		return nil, domain.ErrNotFound
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var d chatThreadDoc
	if err := res.Decode(&d); err != nil {
		return nil, err
	}
	return &domain.ChatThread{ID: d.ID, SessionID: d.SessionID, Title: d.Title, Summary: d.Summary, Archived: d.Archived, CreatedAt: fromMillis(d.CreatedAt), UpdatedAt: fromMillis(d.UpdatedAt)}, nil
}

func (r chatRepo) DeleteThread(ctx context.Context, sessionID, id string) error {
	_, err := r.c("chat_messages").DeleteMany(ctx, bson.M{"thread_id": id, "session_id": sessionID})
	if err != nil {
		return err
	}
	_, err = r.c("chat_threads").DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	return err
}

func (r chatRepo) DeleteThreadMessages(ctx context.Context, sessionID, threadID string) error {
	_, err := r.c("chat_messages").DeleteMany(ctx, bson.M{"thread_id": threadID, "session_id": sessionID})
	return err
}

func (r chatRepo) ListMessages(ctx context.Context, threadID, sessionID, before string, limit int) ([]domain.ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	filter := bson.M{"thread_id": threadID, "session_id": sessionID}
	if before != "" {
		var beforeMs int64
		if n, err := strconv.ParseInt(before, 10, 64); err == nil {
			beforeMs = n
		}
		if beforeMs > 0 {
			filter["created_at"] = bson.M{"$lt": beforeMs}
		}
	}
	cur, err := r.c("chat_messages").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.ChatMessage
	for cur.Next(ctx) {
		var d chatMsgDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toDomain())
	}
	return out, cur.Err()
}

func (r chatRepo) AppendMessages(ctx context.Context, msgs []domain.ChatMessage) error {
	now := nowMillis()
	docs := make([]interface{}, len(msgs))
	for i, m := range msgs {
		if m.ID == "" {
			m.ID = uuid.NewString()
		}
		// Distinct, increasing per-message timestamp so the `$lt before` cursor
		// can't skip a batch sharing one timestamp.
		docs[i] = chatMsgDoc{m.ID, m.ThreadID, m.SessionID, m.Role, m.Content, m.ToolEvents, m.Status, now + int64(i)}
	}
	_, err := r.c("chat_messages").InsertMany(ctx, docs)
	return err
}

func (r chatRepo) GetMessage(ctx context.Context, sessionID, id string) (*domain.ChatMessage, error) {
	res := r.c("chat_messages").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	if notFound(res.Err()) {
		return nil, domain.ErrNotFound
	}
	if res.Err() != nil {
		return nil, res.Err()
	}
	var d chatMsgDoc
	if err := res.Decode(&d); err != nil {
		return nil, err
	}
	m := d.toDomain()
	return &m, nil
}

func (r chatRepo) UpdateMessage(ctx context.Context, sessionID, id string, upd domain.ChatMessageUpdate) error {
	set := bson.M{}
	if upd.Content != nil {
		set["content"] = *upd.Content
	}
	if upd.ToolEvents != nil {
		set["tool_events"] = *upd.ToolEvents
	}
	if upd.Status != nil {
		set["status"] = *upd.Status
	}
	if len(set) == 0 {
		return nil
	}
	res, err := r.c("chat_messages").UpdateOne(ctx, bson.M{"_id": id, "session_id": sessionID}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r chatRepo) FailPendingMessages(ctx context.Context) (int64, error) {
	res, err := r.c("chat_messages").UpdateMany(ctx,
		bson.M{"status": domain.MsgStatusPending},
		bson.M{"$set": bson.M{"status": domain.MsgStatusError}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}
