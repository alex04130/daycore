package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type attachmentRepo struct{ *Store }

const attachmentCols = `id, session_id, thread_id, message_id, ref, kind, mime, size, sha256, filename, created_at`

func (r attachmentRepo) Create(ctx context.Context, a *domain.Attachment) (*domain.Attachment, error) {
	if a.SessionID == "" {
		return nil, errors.New("attachment: session id required")
	}
	if a.Ref == "" {
		// A row with no ref is an owner record for nothing. It would pass every
		// later check and hand out a 404 at read time, which reads like a
		// missing file rather than a bug at write time.
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
	_, err := r.exec(ctx,
		`INSERT INTO attachments (`+attachmentCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.SessionID, a.ThreadID, a.MessageID, a.Ref, a.Kind, a.MIME, a.Size, a.SHA256, a.Filename, now)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r attachmentRepo) Get(ctx context.Context, sessionID, id string) (*domain.Attachment, error) {
	row := r.queryRow(ctx,
		`SELECT `+attachmentCols+` FROM attachments WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanAttachment(row.Scan)
}

// Bind is all-or-nothing without a transaction, which sqlstore does not have.
//
// The shape that gets there anyway: one conditional UPDATE per id, each carrying
// its own precondition (mine, and unbound), then a count. A partial success is
// still possible if the process dies mid-loop, and the consequence is bounded —
// an attachment bound to a message that was never written is swept by the same
// rule that reclaims uploads nobody sent, because the message it points at does
// not exist and the composer will never show it again.
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
		res, err := r.exec(ctx,
			`UPDATE attachments SET thread_id = ?, message_id = ?
			 WHERE id = ? AND session_id = ? AND message_id = ''`,
			threadID, messageID, id, sessionID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Either it does not exist, is another session's, or is already
			// bound. Distinguish the last one, because that is the only case a
			// client can act on — and only within its own session, so the
			// answer leaks nothing.
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
	ids := nonEmpty(messageIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, sessionID)
	for _, id := range ids {
		args = append(args, id)
	}
	q := `SELECT ` + attachmentCols + ` FROM attachments
		  WHERE session_id = ? AND message_id IN (` + placeholders(len(ids)) + `)
		  ORDER BY created_at ASC, id ASC`
	rows, err := r.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAttachments(rows)
}

func (r attachmentRepo) ListUnbound(ctx context.Context, sessionID string, limit int) ([]domain.Attachment, error) {
	limit = domain.ListLimit(limit, domain.AttachmentListDefault, domain.AttachmentListMax)
	rows, err := r.query(ctx,
		`SELECT `+attachmentCols+` FROM attachments
		 WHERE session_id = ? AND message_id = ''
		 ORDER BY created_at DESC, id ASC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectAttachments(rows)
}

func (r attachmentRepo) Delete(ctx context.Context, sessionID, id string) (*domain.Attachment, error) {
	a, err := r.Get(ctx, sessionID, id)
	if err != nil {
		return nil, err
	}
	if a.Bound() {
		return nil, domain.ErrAttachmentBound
	}
	res, err := r.exec(ctx,
		`DELETE FROM attachments WHERE id = ? AND session_id = ? AND message_id = ''`, id, sessionID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// It was bound between the read and the delete. Report the same refusal
		// the read would have, rather than a success that deleted nothing.
		return nil, domain.ErrAttachmentBound
	}
	return a, nil
}

func (r attachmentRepo) DeleteByThread(ctx context.Context, sessionID, threadID string) ([]domain.Attachment, error) {
	if threadID == "" {
		return nil, nil
	}
	rows, err := r.query(ctx,
		`SELECT `+attachmentCols+` FROM attachments WHERE session_id = ? AND thread_id = ?`,
		sessionID, threadID)
	if err != nil {
		return nil, err
	}
	out, err := collectAttachments(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if _, err := r.exec(ctx,
		`DELETE FROM attachments WHERE session_id = ? AND thread_id = ?`, sessionID, threadID); err != nil {
		return nil, err
	}
	return out, nil
}

func (r attachmentRepo) PruneUnbound(ctx context.Context, before time.Time) ([]domain.Attachment, error) {
	cutoff := toMillis(before)
	rows, err := r.query(ctx,
		`SELECT `+attachmentCols+` FROM attachments WHERE message_id = '' AND created_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	out, err := collectAttachments(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	// Re-state the predicate rather than deleting by the ids just read: an
	// attachment bound in between must survive, and `message_id = ''` says so
	// in one statement without a transaction.
	if _, err := r.exec(ctx,
		`DELETE FROM attachments WHERE message_id = '' AND created_at < ?`, cutoff); err != nil {
		return nil, err
	}
	return out, nil
}

func collectAttachments(rows *sql.Rows) ([]domain.Attachment, error) {
	var out []domain.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAttachment(scan func(...any) error) (*domain.Attachment, error) {
	var a domain.Attachment
	var ca int64
	err := scan(&a.ID, &a.SessionID, &a.ThreadID, &a.MessageID, &a.Ref,
		&a.Kind, &a.MIME, &a.Size, &a.SHA256, &a.Filename, &ca)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = fromMillis(ca)
	return &a, nil
}

// placeholders renders "?, ?, ?" for an IN clause. Rebind turns them into $n on
// Postgres, so this stays dialect-neutral.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func nonEmpty(in []string) []string {
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
