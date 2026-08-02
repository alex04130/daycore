// Package localfs stores blobs in a directory on the machine running Daycore.
//
// It is the simplest backend and the only one that needs a writable path, which
// is why it is also the one that introduces `DATA_DIR` — a concept the repo did
// not have. Every existing directory setting (STATIC_DIR, LOCALES_DIR,
// PROMPTS_DIR, MODELS_CONFIG) is a read-only input; nothing in internal/ called
// os.WriteFile at all before this.
//
// Registered as "local". Config string is the directory.
package localfs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"daycore/internal/blob"
)

func init() { blob.Register("local", open) }

// maxDefault caps a single blob. It is deliberately generous relative to the
// ceilings scattered around the HTTP layer today (8 MiB on plan-image, 1 MiB on
// what inbox actually keeps, 12 MiB on readJSON): those are request limits and
// belong there, while this one exists to stop a single object from filling the
// disk.
const maxDefault = 64 << 20

type store struct {
	root string
	max  int64
}

func open(cfg string) (blob.Store, error) {
	dir := strings.TrimSpace(cfg)
	if dir == "" {
		return nil, errors.New("localfs: no directory configured (set DATA_DIR)")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("localfs: resolve %q: %w", dir, err)
	}
	// 0o700: blobs are user content and the directory sits on a machine that may
	// be shared. Nothing else needs to read it — the bytes leave through Daycore.
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("localfs: create %q: %w", abs, err)
	}
	// Fail at open, not at the first upload. A read-only or full disk discovered
	// during a user's first upload looks like a bug in the upload; discovered at
	// boot it looks like what it is.
	probe := filepath.Join(abs, ".daycore-write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return nil, fmt.Errorf("localfs: %q is not writable: %w", abs, err)
	}
	_ = os.Remove(probe)
	return &store{root: abs, max: maxDefault}, nil
}

func (s *store) Name() string { return "local" }

// path fans blobs across two levels of hex so no directory holds more entries
// than a filesystem is comfortable with. It also refuses anything that is not a
// ref this store minted: a ref arrives from outside (a database column, a tool
// argument), and joining an attacker-chosen string onto a root is how a file
// bus becomes an arbitrary-file-read.
func (s *store) path(ref blob.Ref) (string, error) {
	id := string(ref)
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return "", blob.ErrNotFound
	}
	return filepath.Join(s.root, id[:2], id[2:4], id), nil
}

func (s *store) Put(ctx context.Context, mime string, r io.Reader) (blob.Ref, blob.Meta, error) {
	if err := ctx.Err(); err != nil {
		return "", blob.Meta{}, err
	}
	// A random id rather than the content hash: content-addressing would make two
	// users' identical uploads share storage, and then one user's delete would
	// take away the other's file — or, worse, a user could probe whether a
	// particular file already exists. Dedup is not worth that.
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", blob.Meta{}, fmt.Errorf("localfs: id: %w", err)
	}
	ref := blob.Ref(hex.EncodeToString(raw[:]))
	final, err := s.path(ref)
	if err != nil {
		return "", blob.Meta{}, err
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return "", blob.Meta{}, fmt.Errorf("localfs: mkdir: %w", err)
	}

	// Write to a temp file in the same directory, then rename. Rename within a
	// directory is atomic, so a reader never sees a half-written blob and a crash
	// mid-upload leaves a stray temp file rather than a truncated one that looks
	// like data.
	tmp, err := os.CreateTemp(filepath.Dir(final), ".tmp-*")
	if err != nil {
		return "", blob.Meta{}, fmt.Errorf("localfs: temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	sum := sha256.New()
	// LimitReader is max+1 so that hitting exactly the cap is not mistaken for
	// exceeding it, and exceeding it is detectable rather than silently truncated.
	n, err := io.Copy(io.MultiWriter(tmp, sum), io.LimitReader(r, s.max+1))
	if err != nil {
		cleanup()
		return "", blob.Meta{}, fmt.Errorf("localfs: write: %w", err)
	}
	if n > s.max {
		cleanup()
		return "", blob.Meta{}, blob.ErrTooLarge
	}
	// Sync before rename: without it a crash can leave a correctly-named file
	// with no contents, which is the one outcome the rename dance is meant to
	// prevent.
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", blob.Meta{}, fmt.Errorf("localfs: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", blob.Meta{}, fmt.Errorf("localfs: close: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return "", blob.Meta{}, fmt.Errorf("localfs: publish: %w", err)
	}

	meta := blob.Meta{Size: n, MIME: mime, SHA256: hex.EncodeToString(sum.Sum(nil)), CreatedAt: time.Now()}
	if err := s.writeMeta(final, meta); err != nil {
		// The bytes are already published; a missing sidecar degrades Stat, it
		// does not lose the blob. Report it rather than unpublishing.
		return ref, meta, fmt.Errorf("localfs: metadata for %s: %w", ref, err)
	}
	return ref, meta, nil
}

func (s *store) Get(ctx context.Context, ref blob.Ref) (io.ReadCloser, blob.Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, blob.Meta{}, err
	}
	p, err := s.path(ref)
	if err != nil {
		return nil, blob.Meta{}, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, blob.Meta{}, blob.ErrNotFound
	}
	if err != nil {
		return nil, blob.Meta{}, fmt.Errorf("localfs: open: %w", err)
	}
	meta, _ := s.readMeta(p) // a lost sidecar must not make the bytes unreadable
	return f, meta, nil
}

func (s *store) Stat(ctx context.Context, ref blob.Ref) (blob.Meta, error) {
	if err := ctx.Err(); err != nil {
		return blob.Meta{}, err
	}
	p, err := s.path(ref)
	if err != nil {
		return blob.Meta{}, err
	}
	fi, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return blob.Meta{}, blob.ErrNotFound
	}
	if err != nil {
		return blob.Meta{}, fmt.Errorf("localfs: stat: %w", err)
	}
	meta, err := s.readMeta(p)
	if err != nil {
		// Fall back to what the filesystem knows. MIME is unknown rather than
		// guessed: sniffing here would contradict "stores record, they do not
		// verify".
		return blob.Meta{Size: fi.Size(), CreatedAt: fi.ModTime()}, nil
	}
	return meta, nil
}

func (s *store) Delete(ctx context.Context, ref blob.Ref) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := s.path(ref)
	if errors.Is(err, blob.ErrNotFound) {
		return nil // deleting something that was never a valid ref is already done
	}
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("localfs: delete: %w", err)
	}
	_ = os.Remove(p + ".meta")
	return nil
}

// Metadata lives in a sidecar file rather than in the blob or in a database.
//
// Not in the blob, because callers stream bytes straight to a model or a
// subprocess and a header would corrupt them. Not in a database, because that
// would make the file bus depend on the store being up — and the file bus is
// wanted precisely in the degraded and adapter cases where it may not be.
//
// Format is one line of JSON. A lost or corrupt sidecar degrades Stat and never
// makes the bytes unreadable.
func (s *store) writeMeta(blobPath string, m blob.Meta) error {
	b, err := json.Marshal(metaFile{Size: m.Size, MIME: m.MIME, SHA256: m.SHA256, CreatedAt: m.CreatedAt})
	if err != nil {
		return err
	}
	return os.WriteFile(blobPath+".meta", b, 0o600)
}

func (s *store) readMeta(blobPath string) (blob.Meta, error) {
	b, err := os.ReadFile(blobPath + ".meta")
	if err != nil {
		return blob.Meta{}, err
	}
	var f metaFile
	if err := json.Unmarshal(b, &f); err != nil {
		return blob.Meta{}, err
	}
	return blob.Meta{Size: f.Size, MIME: f.MIME, SHA256: f.SHA256, CreatedAt: f.CreatedAt}, nil
}

type metaFile struct {
	Size      int64     `json:"size"`
	MIME      string    `json:"mime"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
}
