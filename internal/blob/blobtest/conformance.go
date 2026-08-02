// Package blobtest is the behavioural suite every blob driver must pass.
//
// It exists before the second driver does, on purpose. The storage layer learned
// this the expensive way: two backends shipped, no test asserted they behaved
// the same, and an adversarial review later found about fifteen divergences —
// each of which had been reachable in production for months. Pairwise
// divergence grows with the square of the number of backends, and the list here
// is meant to be long: local disk, S3, OSS, COS, Qiniu, Upyun, OBS, KS3,
// OneDrive, a database column, and the http/exec bridges.
//
// Written against the blob.Store interface, so a driver runs it with three
// lines.
package blobtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"daycore/internal/blob"
)

// Factory builds a fresh, empty store for one case.
type Factory func(t *testing.T) blob.Store

// Run executes the suite.
func Run(t *testing.T, newStore Factory) {
	t.Helper()
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) { fn(t, newStore(t)) })
	}
}

var cases = map[string]func(*testing.T, blob.Store){
	"RoundTrip":                    roundTrip,
	"RefsAreUnique":                refsAreUnique,
	"MissingRefIsErrNotFound":      missingRef,
	"GarbageRefIsErrNotFound":      garbageRef,
	"StatMatchesGet":               statMatchesGet,
	"DeleteIsIdempotent":           deleteIdempotent,
	"DeletedBlobIsGone":            deletedIsGone,
	"EmptyBlobIsStorable":          emptyBlob,
	"MIMEIsRecordedNotSniffed":     mimeRecorded,
	"SHA256IsOfTheStoredBytes":     shaIsReal,
	"CancelledContextDoesNotStore": cancelledPut,
}

func roundTrip(t *testing.T, s blob.Store) {
	ctx := context.Background()
	body := []byte("明天下午三点和导师开会\x00\xff binary too")

	ref, meta, err := s.Put(ctx, "text/plain", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if ref == "" {
		t.Fatal("put returned an empty ref")
	}
	if meta.Size != int64(len(body)) {
		t.Errorf("size = %d, want %d", meta.Size, len(body))
	}

	rc, got, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	back, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(back, body) {
		// Byte-exactness, not string equality: a driver that round-trips text and
		// mangles a NUL or a lone 0xff is the one that will be handed a PDF.
		t.Errorf("bytes changed in storage: wrote %d bytes, read %d", len(body), len(back))
	}
	if got.MIME != "text/plain" {
		t.Errorf("mime = %q, want text/plain", got.MIME)
	}
}

func refsAreUnique(t *testing.T, s blob.Store) {
	ctx := context.Background()
	// Identical content twice. A driver that content-addresses would hand back the
	// same ref, and then one owner's Delete would silently take away another's
	// file. Dedup has to be opt-in, never a surprise.
	a, _, err := s.Put(ctx, "text/plain", strings.NewReader("same"))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := s.Put(ctx, "text/plain", strings.NewReader("same"))
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("identical content produced the same ref — deleting one would destroy the other")
	}
	if err := s.Delete(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(ctx, b); err != nil {
		t.Errorf("deleting one blob destroyed an unrelated one with the same content: %v", err)
	}
}

func missingRef(t *testing.T, s blob.Store) {
	ctx := context.Background()
	ref, _, err := s.Put(ctx, "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(ctx, ref); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("get of a deleted ref: %v, want ErrNotFound — callers branch on this to tell "+
			"'expired' from 'the backend is broken'", err)
	}
	if _, err := s.Stat(ctx, ref); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("stat of a deleted ref: %v, want ErrNotFound", err)
	}
}

func garbageRef(t *testing.T, s blob.Store) {
	ctx := context.Background()
	// Refs arrive from outside — a database column, a tool argument, a URL. A
	// driver that joins one onto a root without checking turns the file bus into
	// an arbitrary-file read.
	for _, bad := range []blob.Ref{
		"", "../../etc/passwd", "..\\..\\windows\\system32", "not-hex-at-all",
		"/absolute/path", "a/b/c", blob.Ref(strings.Repeat("z", 64)),
	} {
		if _, _, err := s.Get(ctx, bad); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("Get(%q) = %v, want ErrNotFound", bad, err)
		}
		if err := s.Delete(ctx, bad); err != nil {
			t.Errorf("Delete(%q) = %v, want nil (deleting nonsense is already done)", bad, err)
		}
	}
}

func statMatchesGet(t *testing.T, s blob.Store) {
	ctx := context.Background()
	body := bytes.Repeat([]byte("x"), 5000)
	ref, put, err := s.Put(ctx, "application/pdf", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	stat, err := s.Stat(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size != put.Size || stat.MIME != put.MIME || stat.SHA256 != put.SHA256 {
		t.Errorf("Stat disagrees with Put:\n  put  %+v\n  stat %+v", put, stat)
	}
	rc, get, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	if get.Size != stat.Size || get.MIME != stat.MIME {
		t.Errorf("Get disagrees with Stat:\n  stat %+v\n  get  %+v", stat, get)
	}
}

func deleteIdempotent(t *testing.T, s blob.Store) {
	ctx := context.Background()
	ref, _, err := s.Put(ctx, "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, ref); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	// Cleanup runs on a schedule and will race with itself. A second delete that
	// errors turns a routine sweep into a log full of failures nobody can act on.
	if err := s.Delete(ctx, ref); err != nil {
		t.Errorf("second delete: %v, want nil", err)
	}
}

func deletedIsGone(t *testing.T, s blob.Store) {
	ctx := context.Background()
	ref, _, err := s.Put(ctx, "text/plain", strings.NewReader("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if rc, _, err := s.Get(ctx, ref); err == nil {
		rc.Close()
		t.Error("bytes are still readable after Delete — a user asked for this to be gone")
	}
}

func emptyBlob(t *testing.T, s blob.Store) {
	ctx := context.Background()
	// A zero-byte upload is a real thing (an empty file, a truncated stream). It
	// must be storable and distinguishable from a missing blob.
	ref, meta, err := s.Put(ctx, "application/octet-stream", strings.NewReader(""))
	if err != nil {
		t.Fatalf("put empty: %v", err)
	}
	if meta.Size != 0 {
		t.Errorf("size = %d, want 0", meta.Size)
	}
	rc, _, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatalf("an empty blob must exist, not read as missing: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if len(b) != 0 {
		t.Errorf("read %d bytes back from an empty blob", len(b))
	}
}

func mimeRecorded(t *testing.T, s blob.Store) {
	ctx := context.Background()
	// A PNG magic header declared as text/plain. The store records what it was
	// told: sniffing here would silently overrule the caller's own validation,
	// which is where the decision belongs.
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0}
	ref, _, err := s.Put(ctx, "text/plain", bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.Stat(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MIME != "text/plain" {
		t.Errorf("mime = %q — the store second-guessed its caller", meta.MIME)
	}
}

func shaIsReal(t *testing.T, s blob.Store) {
	ctx := context.Background()
	body := []byte("hash me exactly")
	want := sha256.Sum256(body)
	_, meta, err := s.Put(ctx, "text/plain", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if meta.SHA256 == "" {
		t.Fatal("no checksum — it is the only end-to-end evidence the bytes survived the trip")
	}
	if meta.SHA256 != hex.EncodeToString(want[:]) {
		t.Errorf("sha256 = %s, want %s", meta.SHA256, hex.EncodeToString(want[:]))
	}
}

func cancelledPut(t *testing.T, s blob.Store) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ref, _, err := s.Put(ctx, "text/plain", strings.NewReader("should not land"))
	if err == nil {
		// Not fatal to correctness, but a store that ignores cancellation will
		// keep writing after a client disconnects, and the object-store drivers
		// will each need to get this right.
		t.Errorf("Put on a cancelled context succeeded (ref %q); it must not start work", ref)
	}
}
