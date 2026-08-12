package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/ai"
	"daycore/internal/blob"
	_ "daycore/internal/blob/localfs"
	"daycore/internal/domain"
)

// The file bus had zero production callers before this batch: blob.Store was
// built, tested against its own conformance suite, wired into Server — and
// nothing ever put a byte in it. These tests are the other half of fixing that,
// and they run against a real store and a real database rather than fakes,
// because the interesting failures here are all at the seam between the two.

func newFileTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)
	store, err := blob.Open("local", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.blobs = store
	s.cfg.MaxUploadBytes = 1 << 20
	s.cfg.MaxImageBytes = 1 << 20
	return s, sid
}

// upload posts bytes the way a browser would and returns the decoded row.
func upload(t *testing.T, s *Server, sid, filename, mime, body string) domain.Attachment {
	t.Helper()
	rec := doUpload(t, s, sid, filename, mime, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var a domain.Attachment
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func doUpload(t *testing.T, s *Server, sid, filename, mime, body string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/files"
	if filename != "" {
		url += "?filename=" + filename
	}
	req := httptest.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", mime)
	req = req.WithContext(withSessionID(req.Context(), sid))
	rec := httptest.NewRecorder()
	s.handleFileUpload(rec, req)
	return rec
}

func withSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, ctxSessionID, sid)
}

// The one property everything else depends on: a client is given an id, never a
// ref. A ref in a response would be a bearer handle for bytes, minted by a store
// that has no idea who is asking — and the JSON tag is the only thing standing
// between the two, which is exactly the kind of guarantee that survives one
// refactor and not two.
func TestFileRefNeverReachesTheClient(t *testing.T) {
	s, sid := newFileTestServer(t)
	a := upload(t, s, sid, "a.png", "image/png", "hello-bytes")
	if a.Ref != "" {
		t.Fatal("decoded row carries a ref — the response leaked it")
	}

	stored, err := s.store.Attachments().Get(context.Background(), sid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Ref == "" {
		t.Fatal("nothing stored a ref, so this test could not detect a leak")
	}

	// Every shape a ref could ride out on: the upload response, the pending
	// list, and a hydrated message.
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), stored.Ref) {
		t.Errorf("Attachment serialises its ref: %s", raw)
	}
	msg := domain.ChatMessage{ID: "m1", Attachments: []domain.Attachment{*stored}}
	raw, _ = json.Marshal(msg)
	if strings.Contains(string(raw), stored.Ref) {
		t.Errorf("ChatMessage serialises an attachment ref: %s", raw)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", versionPath("/api/files"), nil).WithContext(withSessionID(context.Background(), sid))
	s.handleFileListPending(rec, req)
	if strings.Contains(rec.Body.String(), stored.Ref) {
		t.Errorf("the pending list serialises refs: %s", rec.Body.String())
	}
}

func TestFileUploadAndDownload(t *testing.T) {
	s, sid := newFileTestServer(t)
	const body = "not really a png"
	a := upload(t, s, sid, "photo.png", "image/png", body)
	if a.Kind != domain.AttachmentImage || a.Size != int64(len(body)) || a.SHA256 == "" {
		t.Fatalf("upload metadata is wrong: %+v", a)
	}

	rec := download(t, s, sid, a.ID, "")
	if rec.Code != http.StatusOK || rec.Body.String() != body {
		t.Fatalf("download: %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		// Uploaded bytes served from the app's own origin. Without nosniff plus
		// the sandbox CSP, an uploaded .html or .svg runs as a same-origin
		// script the moment anyone opens it in a tab.
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Errorf("Content-Security-Policy = %q, want a sandbox", got)
	}
	if got := rec.Header().Get("ETag"); got != `"`+a.SHA256+`"` {
		t.Errorf("ETag = %q, want the content hash", got)
	}

	// If-None-Match is exact because the validator is the content hash.
	req := httptest.NewRequest("GET", versionPath("/api/files/")+a.ID, nil)
	req.SetPathValue("id", a.ID)
	req.Header.Set("If-None-Match", `"`+a.SHA256+`"`)
	req = req.WithContext(withSessionID(req.Context(), sid))
	rec = httptest.NewRecorder()
	s.handleFileDownload(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match returned %d, want 304", rec.Code)
	}
}

func download(t *testing.T, s *Server, sid, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", versionPath("/api/files/")+id+query, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withSessionID(req.Context(), sid))
	rec := httptest.NewRecorder()
	s.handleFileDownload(rec, req)
	return rec
}

// The id is a UUID a client sends. If the handler trusted it, guessing one would
// hand any authenticated caller anyone's uploads.
func TestFileDownloadIsSessionScoped(t *testing.T) {
	s, sid := newFileTestServer(t)
	a := upload(t, s, sid, "secret.png", "image/png", "private")

	if _, err := s.store.Sessions().GetOrCreate(context.Background(), "other-sid"); err != nil {
		t.Fatal(err)
	}
	rec := download(t, s, "other-sid", a.ID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another session downloaded the file: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private") {
		t.Fatal("the refusal still contained the bytes")
	}
}

func TestFileUploadRefusals(t *testing.T) {
	s, sid := newFileTestServer(t)

	if rec := doUpload(t, s, sid, "a.png", "image/png", ""); rec.Code != http.StatusBadRequest {
		// Zero bytes is always a client bug and never a file anyone meant to
		// keep; accepting it mints a row that resolves to nothing.
		t.Errorf("empty upload: %d, want 400", rec.Code)
	}
	if rec := doUpload(t, s, sid, "a.png", "", "x"); rec.Code != http.StatusBadRequest {
		t.Errorf("upload with no Content-Type: %d, want 400", rec.Code)
	}
	big := strings.Repeat("x", int(s.cfg.MaxUploadBytes)+10)
	if rec := doUpload(t, s, sid, "big.bin", "application/octet-stream", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("over-large upload: %d, want 413", rec.Code)
	}
	// And nothing was left behind by the refusals.
	list, err := s.store.Attachments().ListUnbound(context.Background(), sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("a refused upload left %d rows behind: %+v", len(list), list)
	}
}

// A deployment with no BLOB_STORE is supported. It must say so rather than
// failing in a way that reads like a bug — and /api/version must say so too, so
// a client does not offer an attach button that always 503s.
func TestFileEndpointsWithoutABus(t *testing.T) {
	s, sid := newFileTestServer(t)
	s.blobs = nil
	if rec := doUpload(t, s, sid, "a.png", "image/png", "x"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("upload without a bus: %d, want 503", rec.Code)
	}
	if f := s.capabilityFeatures(); f["files"] {
		t.Error(`features.files is true with no file bus configured`)
	}
	s, _ = newFileTestServer(t)
	if f := s.capabilityFeatures(); !f["files"] {
		t.Error(`features.files is false with a file bus configured`)
	}
}

// Deleting the row without the bytes leaks the disk one upload at a time;
// deleting a bound attachment breaks a message that is already on screen.
func TestFileDeleteTakesTheBytesAndSparesSentOnes(t *testing.T) {
	s, sid := newFileTestServer(t)
	ctx := context.Background()
	a := upload(t, s, sid, "a.png", "image/png", "bytes")
	stored, _ := s.store.Attachments().Get(ctx, sid, a.ID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", versionPath("/api/files/")+a.ID, nil)
	req.SetPathValue("id", a.ID)
	req = req.WithContext(withSessionID(req.Context(), sid))
	s.handleFileDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, _, err := s.blobs.Get(ctx, blob.Ref(stored.Ref)); err == nil {
		t.Error("the row is gone but the bytes are still on the bus")
	}

	sent := upload(t, s, sid, "b.png", "image/png", "bytes")
	if err := s.store.Attachments().Bind(ctx, sid, "t1", "m1", []string{sent.ID}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", versionPath("/api/files/")+sent.ID, nil)
	req.SetPathValue("id", sent.ID)
	req = req.WithContext(withSessionID(req.Context(), sid))
	s.handleFileDelete(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("deleting a sent attachment: %d, want 409", rec.Code)
	}
}

// Deleting a conversation must take its bytes. Nothing else can: once the
// messages are gone, no query can find which blobs belonged to them.
func TestDeletingAThreadReclaimsItsBytes(t *testing.T) {
	s, sid := newFileTestServer(t)
	ctx := context.Background()
	a := upload(t, s, sid, "a.png", "image/png", "bytes")
	stored, _ := s.store.Attachments().Get(ctx, sid, a.ID)
	if err := s.store.Attachments().Bind(ctx, sid, "t1", "m1", []string{a.ID}); err != nil {
		t.Fatal(err)
	}

	s.dropThreadAttachments(ctx, sid, "t1")

	if _, err := s.store.Attachments().Get(ctx, sid, a.ID); err == nil {
		t.Error("the attachment row survived its thread")
	}
	if _, _, err := s.blobs.Get(ctx, blob.Ref(stored.Ref)); err == nil {
		t.Error("the thread is gone but its bytes are still on the bus")
	}
}

// The sweeper is what keeps an abandoned upload from being permanent: its row
// keeps the blob referenced, so no other cleanup could ever reclaim it.
func TestAttachmentSweepReclaimsRowsAndBytes(t *testing.T) {
	s, sid := newFileTestServer(t)
	ctx := context.Background()
	loose := upload(t, s, sid, "loose.png", "image/png", "bytes")
	sent := upload(t, s, sid, "sent.png", "image/png", "bytes")
	looseRow, _ := s.store.Attachments().Get(ctx, sid, loose.ID)
	if err := s.store.Attachments().Bind(ctx, sid, "t1", "m1", []string{sent.ID}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.store.Attachments().PruneUnbound(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s.dropBlobs(ctx, rows)

	if _, _, err := s.blobs.Get(ctx, blob.Ref(looseRow.Ref)); err == nil {
		t.Error("the swept upload's bytes are still on the bus")
	}
	if _, err := s.store.Attachments().Get(ctx, sid, sent.ID); err != nil {
		t.Error("the sweep took an attachment that belongs to a message")
	}
}

// contentDisposition builds a response header out of a client-supplied string,
// which makes it the one place an upload can reach into the response itself.
func TestContentDispositionCannotBeInjected(t *testing.T) {
	req := httptest.NewRequest("GET", versionPath("/api/files/x"), nil)
	cases := []struct{ name, filename string }{
		{"crlf", "a\r\nX-Evil: 1.png"},
		{"quote", `a".png`},
		{"traversal", "../../etc/passwd"},
		{"windows path", `C:\Users\me\a.png`},
		{"dotfiles", "...hidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := domain.Attachment{ID: "id", Kind: domain.AttachmentImage, MIME: "image/png",
				Filename: domain.SafeFilename(tc.filename)}
			got := contentDisposition(req, a)
			for _, bad := range []string{"\r", "\n", `"a"`, "..", "/", `\`} {
				if strings.Contains(got, bad) {
					t.Errorf("Content-Disposition %q still contains %q", got, bad)
				}
			}
		})
	}
	// Non-ASCII survives, via RFC 5987 rather than by being mangled.
	a := domain.Attachment{ID: "id", Kind: domain.AttachmentImage, MIME: "image/png", Filename: "作业.png"}
	if got := contentDisposition(req, a); !strings.Contains(got, "filename*=") {
		t.Errorf("a non-ASCII filename was not encoded: %q", got)
	}
	// ?download=1 forces a save dialog; the default is inline so an image can
	// render in a chat bubble.
	dl := httptest.NewRequest("GET", versionPath("/api/files/x?download=1"), nil)
	if got := contentDisposition(dl, a); !strings.HasPrefix(got, "attachment;") {
		t.Errorf("download=1 gave %q", got)
	}
	if got := contentDisposition(req, a); !strings.HasPrefix(got, "inline;") {
		t.Errorf("default disposition = %q, want inline", got)
	}
	// Unknown bytes are never inline: the browser must not try to render them.
	blobby := domain.Attachment{ID: "id", Kind: domain.AttachmentFile, MIME: "application/x-thing"}
	if got := contentDisposition(req, blobby); !strings.HasPrefix(got, "attachment;") {
		t.Errorf("an unknown file type rendered inline: %q", got)
	}
}

// The sliding-window compressor can append a summary AFTER the user turn, and
// hanging a photo on that would send it as a system message — a shape some
// providers reject outright and others quietly ignore.
func TestAttachPartsFindsTheUserTurn(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: "rules"},
		{Role: ai.RoleUser, Content: "hi"},
		{Role: ai.RoleSystem, Content: "[summary]"},
	}
	attachPartsToLastUser(msgs, []ai.ContentPart{{Type: ai.PartImage, MIME: "image/png", Data: "x"}})
	if len(msgs[1].Parts) != 1 {
		t.Errorf("the photo did not land on the user turn: %+v", msgs)
	}
	if len(msgs[2].Parts) != 0 {
		t.Error("the photo landed on the trailing system message")
	}
	// No user turn at all: nothing to hang it on, and nothing may be invented.
	only := []ai.Message{{Role: ai.RoleSystem, Content: "rules"}}
	attachPartsToLastUser(only, []ai.ContentPart{{Type: ai.PartImage}})
	if len(only[0].Parts) != 0 {
		t.Error("a part was attached to a system message")
	}
}

// A model that cannot read a file must be told the file exists, by name.
// Dropping it silently produces the worst answer available: a confident reply to
// a question about a document nobody read.
func TestUnreadableAttachmentsAreDescribedNotDropped(t *testing.T) {
	s, sid := newFileTestServer(t)
	ctx := context.Background()
	img := upload(t, s, sid, "photo.png", "image/png", "bytes")
	doc := upload(t, s, sid, "notes.pdf", "application/pdf", "bytes")
	rows, err := s.store.Attachments().ListUnbound(ctx, sid, 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("seed: %d rows err=%v", len(rows), err)
	}

	// A text-only model: it accepts neither the image nor the PDF.
	parts := s.attachmentParts(ctx, textOnlyModel{}, "zh-CN", rows)
	if len(parts) != 1 || parts[0].Type != ai.PartText {
		t.Fatalf("text-only model got %+v, want one describing text part", parts)
	}
	for _, want := range []string{"photo.png", "notes.pdf"} {
		if !strings.Contains(parts[0].Text, want) {
			t.Errorf("the description does not name %q: %q", want, parts[0].Text)
		}
	}
	_ = img
	_ = doc

	// A vision model: the image goes in as bytes, the PDF is still described.
	parts = s.attachmentParts(ctx, visionModel{}, "zh-CN", rows)
	var images, texts int
	for _, p := range parts {
		switch p.Type {
		case ai.PartImage:
			images++
			if p.Data == "" || p.MIME != "image/png" {
				t.Errorf("image part is missing its bytes or mime: %+v", p)
			}
		case ai.PartText:
			texts++
		}
	}
	if images != 1 || texts != 1 {
		t.Errorf("vision model got %d image and %d text parts, want 1 and 1", images, texts)
	}
}

type textOnlyModel struct{ ai.AIProvider }

func (textOnlyModel) Capabilities() ai.Capabilities { return ai.Capabilities{} }

type visionModel struct{ ai.AIProvider }

func (visionModel) Capabilities() ai.Capabilities {
	return ai.Capabilities{Vision: true, In: ai.NewModalities(ai.ModalityText, ai.ModalityImage)}
}
