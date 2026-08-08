package domain

import (
	"context"
	"errors"
	"mime"
	"strings"
	"time"
)

// Attachment is a row that says who owns some bytes on the file bus.
//
// The file bus itself (internal/blob) is deliberately dumb: it maps a ref to
// bytes and knows nothing about sessions. That split is only safe if something
// else remembers who put each blob there — this is that something. Every ref
// the server hands out or resolves goes through a row here first, which is what
// makes "a ref must only be handed to the session that put the bytes in" an
// enforceable rule rather than a comment.
//
// Two consequences worth stating out loud:
//
//   - Ref never leaves the process. Clients address attachments by ID and the
//     server maps ID → ref after checking the session. A ref in a JSON response
//     would be a bearer token for bytes, minted by a store that has no notion of
//     who is asking.
//   - Deleting the row and deleting the bytes are two operations, and the row is
//     the one that knows about both. Repository methods that remove rows return
//     what they removed so the caller can delete the bytes; a repository that
//     just dropped the rows would leak the file bus one upload at a time.
type Attachment struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	// ThreadID and MessageID are empty until the attachment is bound to a
	// message. Both are columns rather than a join because both are query keys:
	// hydrating a page of messages selects on MessageID, deleting a thread
	// selects on ThreadID.
	ThreadID  string `json:"threadId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	// Ref is the file-bus ref. json:"-" is load-bearing — see the type comment.
	Ref       string    `json:"-"`
	Kind      string    `json:"kind"`
	MIME      string    `json:"mime"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Bound reports whether the attachment belongs to a message yet. An unbound
// attachment is an upload the user has not sent: it counts against nobody's
// history and the sweeper is allowed to reclaim it.
func (a Attachment) Bound() bool { return a.MessageID != "" }

// Attachment kinds. This is what a client renders on and what the model
// pipeline branches on, so it is a small closed set rather than a MIME string:
// a frontend should not have to know that "image/avif" is an image.
const (
	AttachmentImage    = "image"
	AttachmentAudio    = "audio"
	AttachmentVideo    = "video"
	AttachmentDocument = "document"
	AttachmentFile     = "file" // anything else; still storable, just not special
)

// AttachmentKinds lists every kind, for contract fixtures and validation.
var AttachmentKinds = []string{
	AttachmentImage, AttachmentAudio, AttachmentVideo, AttachmentDocument, AttachmentFile,
}

// documentMIMEs are the non-obvious document types. Anything text/* is a
// document too, handled by prefix below.
var documentMIMEs = map[string]bool{
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/rtf":  true,
	"application/json": true,
	"application/xml":  true,
}

// AttachmentKindOf maps a MIME type onto a kind. Unknown types are AttachmentFile
// rather than an error: refusing to store a file because we cannot categorise it
// would make the file bus useless for exactly the cases it exists for.
func AttachmentKindOf(mimeType string) string {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch {
	case strings.HasPrefix(m, "image/"):
		return AttachmentImage
	case strings.HasPrefix(m, "audio/"):
		return AttachmentAudio
	case strings.HasPrefix(m, "video/"):
		return AttachmentVideo
	case strings.HasPrefix(m, "text/"), documentMIMEs[m]:
		return AttachmentDocument
	default:
		return AttachmentFile
	}
}

// ErrAttachmentBound is returned when a caller tries to bind or delete an
// attachment that already belongs to a message.
//
// It is a distinct error because the two ways to get here mean different
// things: binding twice is a client that resent a message (recoverable, and the
// client wants to know which attachment), while deleting a bound attachment is
// a client deleting history through the wrong door (the message owns it now).
var ErrAttachmentBound = errors.New("attachment already belongs to a message")

// ErrTooManyAttachments is a bind that exceeds AttachmentsPerMessage. Refused in
// the repository rather than only in the handler, because the agent pipeline
// writes messages too and a cap enforced at one door is not a cap.
var ErrTooManyAttachments = errors.New("too many attachments for one message")

// Attachment list ceilings. Same shape as every other list in the repo.
const (
	AttachmentListDefault = 100
	AttachmentListMax     = 500
	// AttachmentsPerMessage caps how many files one message may carry. The
	// number is a guard against a client looping, not a product opinion.
	AttachmentsPerMessage = 20
)

// AttachmentUnboundTTL is how long an upload nobody sent is kept.
//
// Long enough that composing a message with a slow upload, then going to lunch,
// then sending it, still works. Short enough that a client which uploads and
// abandons does not fill the disk. Bound attachments are never swept here —
// they die with their message.
const AttachmentUnboundTTL = 24 * time.Hour

// SafeFilename strips a user-supplied filename down to something that can be
// stored and echoed back without becoming a path, a header injection, or a
// surprise on a Windows client.
//
// The file bus never uses this for storage — refs are opaque and generated by
// the driver — so this is purely the label shown to the user and put in
// Content-Disposition. That is exactly why it needs sanitising: it is the one
// field that travels from an untrusted client into a response header.
func SafeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Take the last path element under both separators; a client on Windows
	// sends "C:\Users\me\a.png" and one on Unix sends "/tmp/a.png".
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f: // control characters, CR/LF included
			continue
		case r == '"' || r == '\\':
			continue
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.TrimLeft(out, ".") // ".." and dotfiles-by-accident
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

// FilenameFor returns the filename to report for an attachment, inventing a
// plausible one from the MIME type when the client did not send one.
func FilenameFor(a Attachment) string {
	if a.Filename != "" {
		return a.Filename
	}
	ext := ""
	if exts, err := mime.ExtensionsByType(a.MIME); err == nil && len(exts) > 0 {
		ext = exts[0]
	}
	return a.Kind + "-" + shortID(a.ID) + ext
}

func shortID(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// AttachmentRepository stores who owns which bytes.
//
// Every method is session-scoped, including the ones that take an ID. That is
// not defensive duplication: the ID is the only thing a client sends, so a
// method that trusted it would let any authenticated caller read any upload in
// the installation by guessing one UUID.
type AttachmentRepository interface {
	// Create stores a new, unbound attachment. Ref must already point at stored
	// bytes — this row is the record that they have an owner.
	Create(ctx context.Context, a *Attachment) (*Attachment, error)

	// Get returns one attachment, ErrNotFound when it does not exist OR belongs
	// to another session. The two are the same answer on purpose: distinguishing
	// them would turn this into an oracle for which IDs exist.
	Get(ctx context.Context, sessionID, id string) (*Attachment, error)

	// Bind attaches uploads to a message, all or nothing.
	//
	// Only unbound attachments in this session can be bound, and an ID that is
	// missing, foreign, or already bound fails the whole call — with
	// ErrAttachmentBound when the cause was prior ownership. One attachment
	// belonging to two messages would make "delete the message, delete its
	// bytes" ambiguous, and the ambiguity would surface as either a leak or a
	// message rendering a broken image.
	Bind(ctx context.Context, sessionID, threadID, messageID string, ids []string) error

	// ListByMessages hydrates a page of messages in one query. Ordered by
	// creation time so a message's attachments render in upload order.
	ListByMessages(ctx context.Context, sessionID string, messageIDs []string) ([]Attachment, error)

	// ListUnbound returns this session's pending uploads, newest first — what a
	// composer shows after a reload.
	ListUnbound(ctx context.Context, sessionID string, limit int) ([]Attachment, error)

	// Delete removes one attachment and returns the row so the caller can delete
	// the bytes. Bound attachments are refused with ErrAttachmentBound: they are
	// part of a message now, and the way to remove them is to remove the message.
	Delete(ctx context.Context, sessionID, id string) (*Attachment, error)

	// DeleteByThread removes every attachment belonging to a thread and returns
	// them, so deleting a conversation takes its bytes with it.
	DeleteByThread(ctx context.Context, sessionID, threadID string) ([]Attachment, error)

	// PruneUnbound removes uploads nobody sent, older than before, across all
	// sessions, and returns them so their bytes can go too.
	//
	// Returning the rows rather than a count is the whole design: a sweeper that
	// only deleted rows would turn every abandoned upload into a permanently
	// unreferenced blob, which is the failure mode you discover when the disk
	// fills and nothing in the database can explain why.
	PruneUnbound(ctx context.Context, before time.Time) ([]Attachment, error)
}
