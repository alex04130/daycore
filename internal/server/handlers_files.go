package server

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/blob"
	"daycore/internal/domain"
)

func init() {
	registerRoutes("files", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/files", s.handleFileUpload)
		mux.HandleFunc("GET /api/files", s.handleFileListPending)
		mux.HandleFunc("GET /api/files/{id}", s.handleFileDownload)
		mux.HandleFunc("DELETE /api/files/{id}", s.handleFileDelete)
	})
}

// The upload body is raw bytes with the MIME type in Content-Type, not
// multipart and not base64 JSON.
//
// base64-in-JSON is what the repo did before the file bus existed, and it is
// what the file bus exists to stop: it inflates every byte by a third and
// requires the whole upload in memory on both sides. Multipart would work but
// buys nothing for a single file — it costs a parser here and a FormData dance
// in four frontends, when `fetch(url, {method:'POST', headers:{'Content-Type':
// file.type}, body: file})` already says everything.
//
// The filename travels in a query parameter rather than a header so it survives
// non-ASCII without RFC 5987 encoding on the client side.

// POST /api/files?filename=notes.pdf
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	locale := s.requestLocale(r)
	if s.blobs == nil {
		// A deployment without BLOB_STORE is supported; a feature that needs
		// bytes says so rather than failing in a way that looks like a bug.
		s.writeErrL(w, locale, http.StatusServiceUnavailable, "no_file_bus", "err.fileUpload.no_file_bus")
		return
	}
	mimeType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if mimeType == "" || strings.HasPrefix(mimeType, "application/x-www-form-urlencoded") {
		s.writeErrL(w, locale, http.StatusBadRequest, "bad_request", "err.fileUpload.bad_request")
		return
	}
	// Exactly the limit is allowed; MaxBytesReader errors on the byte after.
	body := http.MaxBytesReader(w, r.Body, s.runtime().MaxUploadBytes)
	defer body.Close()

	ref, meta, err := s.blobs.Put(r.Context(), mimeType, body)
	if err != nil {
		if errors.Is(err, blob.ErrTooLarge) || isMaxBytesError(err) {
			s.writeErrf(w, locale, http.StatusRequestEntityTooLarge, "too_large",
				"err.fileUpload.too_large", s.runtime().MaxUploadBytes/(1<<20))
			return
		}
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.fileUpload.internal")
		return
	}
	if meta.Size == 0 {
		// Zero bytes is always a client bug (an empty File object, an aborted
		// read) and never a file anyone meant to keep. Take the blob back out
		// rather than minting a row that resolves to nothing.
		_ = s.blobs.Delete(r.Context(), ref)
		s.writeErrL(w, locale, http.StatusBadRequest, "empty", "err.fileUpload.empty")
		return
	}

	a, err := s.store.Attachments().Create(r.Context(), &domain.Attachment{
		SessionID: sid,
		Ref:       string(ref),
		MIME:      meta.MIME,
		Kind:      domain.AttachmentKindOf(meta.MIME),
		Size:      meta.Size,
		SHA256:    meta.SHA256,
		Filename:  domain.SafeFilename(r.URL.Query().Get("filename")),
	})
	if err != nil {
		// The row is what remembers the ref. Without it the bytes are
		// unreachable and unreclaimable, so they go back out now.
		_ = s.blobs.Delete(r.Context(), ref)
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.fileUpload.internal2")
		return
	}
	s.writeJSON(w, http.StatusCreated, a)
}

// GET /api/files — this session's uploads that have not been sent yet, which is
// what a composer restores after a reload.
func (s *Server) handleFileListPending(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.store.Attachments().ListUnbound(r.Context(), sid, limit)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.fileList.internal")
		return
	}
	if list == nil {
		list = []domain.Attachment{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"files": list})
}

// GET /api/files/{id} — the bytes.
//
// Always proxied, never redirected to a signed URL. blob.Store may be able to
// sign (S3 and friends can, a directory cannot), but a client that has to
// handle both shapes will only ever be tested against whichever one the author
// deployed. Signing is an optimisation for later and this endpoint is the
// contract.
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	locale := s.requestLocale(r)
	a, err := s.store.Attachments().Get(r.Context(), sid, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, locale, http.StatusNotFound, "file_not_found", "err.fileDownload.file_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.fileDownload.internal")
		return
	}
	if s.blobs == nil {
		s.writeErrL(w, locale, http.StatusServiceUnavailable, "no_file_bus", "err.fileDownload.no_file_bus")
		return
	}
	rc, meta, err := s.blobs.Get(r.Context(), blob.Ref(a.Ref))
	if errors.Is(err, blob.ErrNotFound) {
		// The row outlived its bytes — a half-finished cleanup, or a store
		// pointed at a directory that was restored without its files. Say
		// "gone" rather than "not found": the difference is whether the client
		// should stop asking.
		s.writeErrL(w, locale, http.StatusGone, "bytes_missing", "err.fileDownload.bytes_missing")
		return
	}
	if err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.fileDownload.internal2")
		return
	}
	defer rc.Close()

	ct := meta.MIME
	if ct == "" {
		ct = a.MIME
	}
	w.Header().Set("Content-Type", ct)
	// Uploads are user-supplied bytes served from the app's own origin. Without
	// this an uploaded .html or .svg runs as a same-origin script the moment
	// someone opens it in a tab.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", contentDisposition(r, *a))
	if meta.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	}
	if a.SHA256 != "" {
		// Content-addressed, so the validator is exact and immutable.
		w.Header().Set("ETag", `"`+a.SHA256+`"`)
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, a.SHA256) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// DELETE /api/files/{id} — drop an upload that was never sent.
func (s *Server) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	locale := s.requestLocale(r)
	a, err := s.store.Attachments().Delete(r.Context(), sid, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, locale, http.StatusNotFound, "file_not_found", "err.fileDelete.file_not_found")
		return
	}
	if errors.Is(err, domain.ErrAttachmentBound) {
		s.writeErrL(w, locale, http.StatusConflict, "attachment_bound", "err.fileDelete.attachment_bound")
		return
	}
	if err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.fileDelete.internal")
		return
	}
	s.dropBlobs(r.Context(), []domain.Attachment{*a})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// dropBlobs deletes the bytes behind rows that are already gone.
//
// Best-effort and after the row, in that order: a blob whose row is gone is
// reclaimable garbage that a later sweep can find by other means, while a row
// pointing at deleted bytes is a broken attachment the user can see.
func (s *Server) dropBlobs(ctx context.Context, rows []domain.Attachment) {
	if s.blobs == nil {
		return
	}
	for _, a := range rows {
		if a.Ref == "" {
			continue
		}
		if err := s.blobs.Delete(ctx, blob.Ref(a.Ref)); err != nil {
			s.log.Warn("file bus delete failed; bytes are now orphaned",
				"attachment", a.ID, "err", err)
		}
	}
}

// contentDisposition builds the header, honouring ?download=1 to force a save
// dialog. Inline is the default because the common case is an image in a chat
// bubble, and a browser that downloads it instead is useless there.
func contentDisposition(r *http.Request, a domain.Attachment) string {
	kind := "inline"
	if r.URL.Query().Get("download") == "1" || a.Kind == domain.AttachmentFile {
		kind = "attachment"
	}
	name := domain.FilenameFor(a)
	// mime.FormatMediaType handles the RFC 5987 filename* encoding a non-ASCII
	// name needs; the plain filename= is the ASCII fallback for old clients.
	return kind + "; " + strings.TrimPrefix(
		mime.FormatMediaType("x", map[string]string{"filename": name}), "x; ")
}

// isMaxBytesError reports the error http.MaxBytesReader returns, which has no
// exported type to compare against.
func isMaxBytesError(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}
