package server

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/blob"
	"daycore/internal/domain"
	"daycore/internal/i18n"
)

// Attachments joining the two halves that already existed separately: rows that
// say who owns which bytes (domain.AttachmentRepository) and a bus that stores
// bytes without knowing about sessions (internal/blob).
//
// Everything here goes through the row first. That is the whole authorization
// story for the file bus — see domain/attachment.go.

// hydrateAttachments fills in Attachments for a page of messages, in one query
// rather than one per message.
//
// Best-effort: a failure here costs the attachment thumbnails, and returning an
// error instead would cost the conversation. The log line is what makes the
// degraded case visible.
func (s *Server) hydrateAttachments(ctx context.Context, sid string, msgs []domain.ChatMessage) {
	if len(msgs) == 0 {
		return
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	rows, err := s.store.Attachments().ListByMessages(ctx, sid, ids)
	if err != nil {
		s.log.Warn("could not hydrate attachments", "session", sid, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	byMsg := map[string][]domain.Attachment{}
	for _, a := range rows {
		byMsg[a.MessageID] = append(byMsg[a.MessageID], a)
	}
	for i := range msgs {
		if got := byMsg[msgs[i].ID]; len(got) > 0 {
			msgs[i].Attachments = got
		}
	}
}

// bindAttachments attaches uploads to a message that was just persisted.
//
// Called after AppendMessages, which fills in the message IDs. A bind that
// fails leaves the uploads unbound, so the sweeper reclaims them and the message
// renders without its files — visibly wrong, and recoverable, which is the right
// side of the trade against silently pointing a message at bytes that a later
// sweep will delete.
func (s *Server) bindAttachments(ctx context.Context, sid, threadID, messageID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.store.Attachments().Bind(ctx, sid, threadID, messageID, ids)
}

// attachmentParts turns attachments into model input.
//
// Two things it deliberately does not do:
//
//   - It does not upload to the provider or hand out URLs. Inline base64 is the
//     one carriage every format accepts (see ai.CarriagesFor), and a signed URL
//     needs a signing story the file bus does not have yet. Both are additive
//     later; getting them wrong now means a model that silently ignores the
//     user's file.
//   - It does not silently drop what the model cannot read. A model without
//     vision that is handed a photo should produce a sentence saying so, and
//     that sentence has to come from somewhere — so unsupported attachments
//     become a text part naming them.
func (s *Server) attachmentParts(ctx context.Context, model ai.AIProvider, locale string, atts []domain.Attachment) []ai.ContentPart {
	if len(atts) == 0 || s.blobs == nil {
		return nil
	}
	caps := model.Capabilities()
	var parts []ai.ContentPart
	var unreadable []string
	for _, a := range atts {
		modality := attachmentModality(a)
		if modality == ai.ModalityText || !caps.Accepts(modality) ||
			!ai.CarriagesFor(model, modality).Has(ai.CarriageInline) {
			unreadable = append(unreadable, domain.FilenameFor(a))
			continue
		}
		data, err := s.readBlobBase64(ctx, a)
		if err != nil {
			s.log.Warn("attachment unreadable, telling the model rather than dropping it",
				"attachment", a.ID, "err", err)
			unreadable = append(unreadable, domain.FilenameFor(a))
			continue
		}
		parts = append(parts, ai.ContentPart{
			Type: modality, MIME: a.MIME, Data: data, Name: domain.FilenameFor(a),
		})
	}
	if len(unreadable) > 0 {
		parts = append(parts, ai.ContentPart{
			Type: ai.PartText,
			Text: i18n.Tf(msgAttachmentsUnreadable, locale, strings.Join(unreadable, "、")),
		})
	}
	return parts
}

// msgAttachmentsUnreadable is prompt-adjacent rather than user-facing: it is
// read by the model, which then says something in its own words. It lives in the
// catalog anyway because the model answers in the user's language and a Chinese
// sentence in an English conversation is a nudge in the wrong direction.
var msgAttachmentsUnreadable = i18n.Reg("prompt.attachments.unreadable", i18n.Text{
	"zh-CN": "（用户附了这些文件，但当前模型读不了它们的内容：%s。如果需要，请说明你看不到内容，不要假装读过。）",
	"en-US": "(The user attached these files but this model cannot read their contents: %s. Say so if it matters; do not pretend to have read them.)",
})

// attachmentModality maps a stored attachment onto the modality vocabulary the
// AI layer speaks. Documents that are plain text are still documents here — the
// formats know how to carry them, and turning a file into an anonymous text blob
// loses the filename, which is often the only clue about what it is for.
func attachmentModality(a domain.Attachment) ai.Modality {
	switch a.Kind {
	case domain.AttachmentImage:
		return ai.ModalityImage
	case domain.AttachmentAudio:
		return ai.ModalityAudio
	case domain.AttachmentVideo:
		return ai.ModalityVideo
	case domain.AttachmentDocument:
		return ai.ModalityDocument
	default:
		// Unknown bytes. Claiming a modality would make a format serialise them
		// as something the provider will reject; ModalityText is the sentinel
		// attachmentParts reads as "describe, do not send".
		return ai.ModalityText
	}
}

// readBlobBase64 loads an attachment's bytes for inline carriage.
//
// Bounded by MaxImageBytes rather than MaxUploadBytes: this is the "fits in a
// model request" limit, and it is a different question from "fits on the disk".
// A 30 MiB PDF is a perfectly good upload and a terrible prompt.
func (s *Server) readBlobBase64(ctx context.Context, a domain.Attachment) (string, error) {
	if a.Size > s.cfg.MaxImageBytes {
		return "", errors.New("attachment is too large to inline into a model request")
	}
	rc, _, err := s.blobs.Get(ctx, blob.Ref(a.Ref))
	if err != nil {
		return "", err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, s.cfg.MaxImageBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(raw)) > s.cfg.MaxImageBytes {
		return "", errors.New("attachment is too large to inline into a model request")
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// resolveAttachments loads the attachments a client named, refusing anything
// that is not this session's unsent upload.
//
// The ids come from a request body, so this is the boundary where "an id the
// client sent" becomes "bytes the server will read".
func (s *Server) resolveAttachments(ctx context.Context, sid string, ids []string) ([]domain.Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > domain.AttachmentsPerMessage {
		return nil, domain.ErrTooManyAttachments
	}
	out := make([]domain.Attachment, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		a, err := s.store.Attachments().Get(ctx, sid, id)
		if err != nil {
			return nil, err
		}
		if a.Bound() {
			return nil, domain.ErrAttachmentBound
		}
		out = append(out, *a)
	}
	return out, nil
}

// dropThreadAttachments removes a thread's attachment rows and their bytes.
// Called when a conversation or its messages are deleted — after that point
// nothing references the blobs and nothing could ever find them again.
func (s *Server) dropThreadAttachments(ctx context.Context, sid, threadID string) {
	rows, err := s.store.Attachments().DeleteByThread(ctx, sid, threadID)
	if err != nil {
		s.log.Warn("could not remove a thread's attachments; its bytes are now orphaned",
			"session", sid, "thread", threadID, "err", err)
		return
	}
	s.dropBlobs(ctx, rows)
}

// attachPartsToLastUser hangs model input on the newest user turn.
//
// Searched from the end rather than assuming the last element: the sliding
// window compressor may append a summary after the user message, and attaching
// a photo to a summary would silently send it as a system turn.
func attachPartsToLastUser(messages []ai.Message, parts []ai.ContentPart) {
	if len(parts) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == ai.RoleUser {
			messages[i].Parts = append(messages[i].Parts, parts...)
			return
		}
	}
}

func idsOf(atts []domain.Attachment) []string {
	if len(atts) == 0 {
		return nil
	}
	out := make([]string, 0, len(atts))
	for _, a := range atts {
		out = append(out, a.ID)
	}
	return out
}

// writeAttachmentErr maps the three ways a client can name the wrong upload
// onto the three answers that tell it something different.
func (s *Server) writeAttachmentErr(w http.ResponseWriter, r *http.Request, handler string, err error) {
	locale := s.requestLocale(r)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.writeErrL(w, locale, http.StatusNotFound, "attachment_not_found", "err."+handler+".attachment_not_found")
	case errors.Is(err, domain.ErrAttachmentBound):
		s.writeErrL(w, locale, http.StatusConflict, "attachment_bound", "err."+handler+".attachment_bound")
	case errors.Is(err, domain.ErrTooManyAttachments):
		s.writeErrf(w, locale, http.StatusBadRequest, "too_many_attachments",
			"err."+handler+".too_many_attachments", domain.AttachmentsPerMessage)
	default:
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err."+handler+".attachment_internal")
	}
}

// StartAttachmentCleanup reclaims uploads nobody sent.
//
// Without it every abandoned upload is permanent: the row keeps the blob
// referenced so no other sweep can touch it, and nothing ever binds it. The
// interval is long because the TTL is long — this is disk hygiene, not a queue.
//
// Rows first, then bytes. A blob whose row is gone is garbage a future sweep
// can still find by enumerating the store; a row pointing at deleted bytes is a
// broken attachment a user can see.
func (s *Server) StartAttachmentCleanup(interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	s.everyTick("attachment sweep", interval, func(ctx context.Context) {
		rows, err := s.store.Attachments().PruneUnbound(ctx, time.Now().Add(-domain.AttachmentUnboundTTL))
		if err != nil {
			s.log.Warn("attachment sweep error", "err", err)
			return
		}
		if len(rows) == 0 {
			return
		}
		s.dropBlobs(ctx, rows)
		s.log.Info("reclaimed uploads that were never sent", "count", len(rows))
	})
}
