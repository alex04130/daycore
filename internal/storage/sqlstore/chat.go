package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type chatRepo struct{ *Store }

func (r chatRepo) ListThreads(ctx context.Context, sessionID string) ([]domain.ChatThread, error) {
	rows, err := r.query(ctx,
		`SELECT id, session_id, title, summary, archived, created_at, updated_at
		 FROM chat_threads WHERE session_id = ? ORDER BY updated_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ChatThread
	for rows.Next() {
		var t domain.ChatThread
		var ca, ua int64
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Title, &t.Summary, &t.Archived, &ca, &ua); err != nil {
			return nil, err
		}
		t.CreatedAt = fromMillis(ca)
		t.UpdatedAt = fromMillis(ua)
		out = append(out, t)
	}
	return out, nil
}

func (r chatRepo) CreateThread(ctx context.Context, t *domain.ChatThread) (*domain.ChatThread, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO chat_threads (id, session_id, title, summary, archived, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.Title, t.Summary, boolToInt(t.Archived), now, now)
	if err != nil {
		return nil, err
	}
	t.CreatedAt = fromMillis(now)
	t.UpdatedAt = fromMillis(now)
	return t, nil
}

func (r chatRepo) UpdateThread(ctx context.Context, sessionID, id string, upd domain.ChatThreadUpdate) (*domain.ChatThread, error) {
	set := []string{}
	args := []any{}
	if upd.Title != nil {
		set = append(set, "title = ?")
		args = append(args, *upd.Title)
	}
	if upd.Summary != nil {
		set = append(set, "summary = ?")
		args = append(args, *upd.Summary)
	}
	if upd.Archived != nil {
		set = append(set, "archived = ?")
		args = append(args, boolToInt(*upd.Archived))
	}
	if len(set) == 0 {
		row := r.queryRow(ctx,
			`SELECT id, session_id, title, summary, archived, created_at, updated_at
			 FROM chat_threads WHERE id = ? AND session_id = ?`, id, sessionID)
		return scanChatThread(row)
	}
	set = append(set, "updated_at = ?")
	args = append(args, nowMillis())
	query := "UPDATE chat_threads SET " + joinComma(set) + " WHERE id = ? AND session_id = ?"
	args = append(args, id, sessionID)
	if _, err := r.exec(ctx, query, args...); err != nil {
		return nil, err
	}
	row := r.queryRow(ctx,
		`SELECT id, session_id, title, summary, archived, created_at, updated_at
		 FROM chat_threads WHERE id = ? AND session_id = ?`, id, sessionID)
	return scanChatThread(row)
}

func (r chatRepo) DeleteThread(ctx context.Context, sessionID, id string) error {
	_, err := r.exec(ctx, `DELETE FROM chat_messages WHERE thread_id = ? AND session_id = ?`, id, sessionID)
	if err != nil {
		return err
	}
	_, err = r.exec(ctx, `DELETE FROM chat_threads WHERE id = ? AND session_id = ?`, id, sessionID)
	return err
}

func (r chatRepo) ListMessages(ctx context.Context, threadID, sessionID, before string, limit int) ([]domain.ChatMessage, error) {
	limit = domain.ListLimit(limit, domain.ChatListDefault, domain.ChatListMax)
	var rows *sql.Rows
	var err error
	if before != "" {
		rows, err = r.query(ctx,
			`SELECT id, thread_id, session_id, role, content, tool_events, status, created_at
			 FROM chat_messages
			 WHERE thread_id = ? AND session_id = ? AND created_at < ?
			 ORDER BY created_at DESC LIMIT ?`,
			threadID, sessionID, before, limit)
	} else {
		rows, err = r.query(ctx,
			`SELECT id, thread_id, session_id, role, content, tool_events, status, created_at
			 FROM chat_messages
			 WHERE thread_id = ? AND session_id = ?
			 ORDER BY created_at DESC LIMIT ?`,
			threadID, sessionID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ChatMessage
	for rows.Next() {
		var m domain.ChatMessage
		var ca int64
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.SessionID, &m.Role, &m.Content, &m.ToolEvents, &m.Status, &ca); err != nil {
			return nil, err
		}
		m.CreatedAt = fromMillis(ca)
		out = append(out, m)
	}
	return out, nil
}

func (r chatRepo) DeleteThreadMessages(ctx context.Context, sessionID, threadID string) error {
	_, err := r.exec(ctx, `DELETE FROM chat_messages WHERE thread_id = ? AND session_id = ?`, threadID, sessionID)
	return err
}

func (r chatRepo) AppendMessages(ctx context.Context, msgs []domain.ChatMessage) error {
	now := nowMillis()
	for i := range msgs {
		if msgs[i].ID == "" {
			msgs[i].ID = uuid.NewString()
		}
		// Give each message in the batch a distinct, increasing timestamp so the
		// `created_at < before` pagination cursor can never skip a group sharing
		// one timestamp.
		ts := now + int64(i)
		_, err := r.exec(ctx,
			`INSERT INTO chat_messages (id, thread_id, session_id, role, content, tool_events, status, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			msgs[i].ID, msgs[i].ThreadID, msgs[i].SessionID, msgs[i].Role, msgs[i].Content, msgs[i].ToolEvents, msgs[i].Status, ts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r chatRepo) GetMessage(ctx context.Context, sessionID, id string) (*domain.ChatMessage, error) {
	row := r.queryRow(ctx,
		`SELECT id, thread_id, session_id, role, content, tool_events, status, created_at
		 FROM chat_messages WHERE id = ? AND session_id = ?`, id, sessionID)
	var m domain.ChatMessage
	var ca int64
	err := row.Scan(&m.ID, &m.ThreadID, &m.SessionID, &m.Role, &m.Content, &m.ToolEvents, &m.Status, &ca)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.CreatedAt = fromMillis(ca)
	return &m, nil
}

func (r chatRepo) UpdateMessage(ctx context.Context, sessionID, id string, upd domain.ChatMessageUpdate) error {
	set := []string{}
	args := []any{}
	if upd.Content != nil {
		set = append(set, "content = ?")
		args = append(args, *upd.Content)
	}
	if upd.ToolEvents != nil {
		set = append(set, "tool_events = ?")
		args = append(args, *upd.ToolEvents)
	}
	if upd.Status != nil {
		set = append(set, "status = ?")
		args = append(args, *upd.Status)
	}
	if len(set) == 0 {
		return nil
	}
	args = append(args, id, sessionID)
	res, err := r.exec(ctx, "UPDATE chat_messages SET "+joinComma(set)+" WHERE id = ? AND session_id = ?", args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r chatRepo) FailPendingMessages(ctx context.Context) (int64, error) {
	res, err := r.exec(ctx, `UPDATE chat_messages SET status = ? WHERE status = ?`,
		domain.MsgStatusError, domain.MsgStatusPending)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanChatThread(row *sql.Row) (*domain.ChatThread, error) {
	var t domain.ChatThread
	var ca, ua int64
	err := row.Scan(&t.ID, &t.SessionID, &t.Title, &t.Summary, &t.Archived, &ca, &ua)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = fromMillis(ca)
	t.UpdatedAt = fromMillis(ua)
	return &t, nil
}
