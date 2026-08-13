package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s must panic", name)
		}
	}()
	fn()
}

func TestRegisterRejectsInvalid(t *testing.T) {
	mustPanic(t, "empty name", func() { Register("", func(string) (Store, error) { return nil, nil }) })
	mustPanic(t, "nil opener", func() { Register("zz-nil", nil) })
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register("zz-dup", func(string) (Store, error) { return nil, nil })
	mustPanic(t, "duplicate", func() { Register("zz-dup", func(string) (Store, error) { return nil, nil }) })
}

func TestOpenEmptyNameIsNilStore(t *testing.T) {
	// Running without a file bus is a supported configuration — features that
	// need bytes must check and degrade, not make the process refuse to boot.
	s, err := Open("", "anything")
	if s != nil || err != nil {
		t.Fatalf("Open(\"\") = (%v, %v), want (nil, nil)", s, err)
	}
	s, err = Open("   ", "anything")
	if s != nil || err != nil {
		t.Fatalf("Open(whitespace) = (%v, %v), want (nil, nil)", s, err)
	}
}

func TestOpenUnknownStoreWrapsErrUnsupported(t *testing.T) {
	_, err := Open("no-such-store", "cfg")
	if !errors.Is(err, ErrUnsupportedStore) {
		t.Errorf("the error must wrap ErrUnsupportedStore, got %v", err)
	}
	if !strings.Contains(err.Error(), "no-such-store") {
		t.Errorf("the asked-for store must be named, got %v", err)
	}
}

func TestTypesSorted(t *testing.T) {
	got := Types()
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Types() not sorted: %v", got)
		}
	}
}

// ── SignedURL ─────────────────────────────────────────────────────────────

type memStore struct {
	signed string
	err    error
}

func (m *memStore) Put(context.Context, string, io.Reader) (Ref, Meta, error) {
	return "", Meta{}, nil
}
func (m *memStore) Get(context.Context, Ref) (io.ReadCloser, Meta, error) {
	return io.NopCloser(bytes.NewReader(nil)), Meta{}, nil
}
func (m *memStore) Stat(context.Context, Ref) (Meta, error) { return Meta{}, nil }
func (m *memStore) Delete(context.Context, Ref) error       { return nil }
func (m *memStore) Name() string                            { return "mem" }
func (m *memStore) SignedURL(context.Context, Ref, time.Duration) (string, error) {
	return m.signed, m.err
}

func TestSignedURLBranches(t *testing.T) {
	ctx := context.Background()
	// A store that cannot sign: the caller proxies the bytes itself.
	plain := struct{ Store }{&memStore{}}
	if u, ok := SignedURL(ctx, plain, "r", time.Minute); ok || u != "" {
		t.Errorf("a non-signing store must yield ok=false, got (%q, %v)", u, ok)
	}
	// A signing store that returns an error: treated the same as can't-sign,
	// never an error to the caller.
	s := &memStore{err: errors.New("boom")}
	if u, ok := SignedURL(ctx, s, "r", time.Minute); ok || u != "" {
		t.Errorf("a signing error must yield ok=false, got (%q, %v)", u, ok)
	}
	// An empty URL is also "proxy it yourself", not a broken answer.
	s = &memStore{signed: ""}
	if u, ok := SignedURL(ctx, s, "r", time.Minute); ok || u != "" {
		t.Errorf("an empty URL must yield ok=false, got (%q, %v)", u, ok)
	}
	// Success passes the ref and TTL through.
	s = &memStore{signed: "https://x/y"}
	if u, ok := SignedURL(ctx, s, Ref("r"), 5*time.Minute); !ok || u != "https://x/y" {
		t.Errorf("a signing store must pass its URL through, got (%q, %v)", u, ok)
	}
}
