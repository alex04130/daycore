// Package blob is the file bus: bytes that are too big, too binary, or too
// long-lived to travel inside a JSON body.
//
// Daycore had nowhere to put a byte. An upload of 1 MiB or less was base64'd
// into a temp_contexts row with a one-hour TTL and anything larger had its bytes
// dropped on the floor; images reached the model only as base64 inside the
// request that carried them; `Material.StorageRef` was a column three dialects
// created, Mongo persisted and the OpenAPI contract published, with no code
// anywhere that produced or resolved one. Four separate features were each
// waiting on the same missing thing — image upload, PDF, image generation, and
// handing artifacts to an adapter subprocess.
//
// The registry mirrors internal/storage: a driver name selects an
// implementation, drivers self-register from init(), and main.go imports them
// for their side effects. Adding a backend is a package plus a Register call —
// the same deal the database layer offers, for the same reason (nobody should
// have to fork this to point it at their own object store).
//
// # What a Store is not
//
// It is not authorization. A Store maps an id to bytes and knows nothing about
// sessions — the same split the reference MCP servers use, where the file bus is
// dumb and the orchestrator decides who may read. Whoever hands out a Ref is
// responsible for deciding who may use it; see the signing helpers in
// internal/auth for the shape that answer should take.
//
// It is also not a database. There is no listing, no query, no transaction. A
// backend that can only PUT and GET a key is a legitimate implementation, which
// is what makes "point it at S3, or a directory, or a column" a real option
// rather than a slogan.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Errors a caller is expected to handle. Drivers must return these rather than
// their own spellings — the whole point of the registry is that the layer above
// cannot tell which driver it is talking to.
var (
	// ErrNotFound is a ref that resolves to nothing. Callers distinguish it from
	// a backend failure: a missing blob is often expected (expired, cleaned up),
	// a broken backend never is.
	ErrNotFound = errors.New("blob: not found")

	// ErrTooLarge is a Put that exceeds the store's configured limit. It exists
	// so the limit can be enforced by the store rather than by every caller,
	// because the four call sites that will exist have historically each invented
	// their own ceiling (8 MiB, 1 MiB, 12 MiB and none).
	ErrTooLarge = errors.New("blob: too large")

	// ErrUnsupportedStore is an unknown driver name.
	ErrUnsupportedStore = errors.New("blob: unsupported store type")
)

// Ref identifies stored bytes. It is opaque to callers: a driver may encode a
// path, a key, a URL or an id, and nothing above this package may parse it.
//
// It is a distinct type rather than a string so that a ref cannot be passed
// where a filename, a session id or a URL is wanted. `Material.StorageRef` is a
// bare string today and that is exactly how it became a field nobody could say
// the meaning of.
type Ref string

// Meta is what a store knows about a blob without reading it.
type Meta struct {
	// Size in bytes. -1 when the driver genuinely cannot say without a read.
	Size int64
	// MIME as declared at Put. Stores record it; they do not sniff it, because a
	// store that second-guesses its caller makes the caller's validation
	// unreachable.
	MIME string
	// SHA256, lowercase hex, of the stored bytes. Computed on Put by the store,
	// because the store is the only place that necessarily sees every byte.
	SHA256 string
	// CreatedAt is when Put completed.
	CreatedAt time.Time
}

// Store is the file bus.
//
// Every method takes a context: a local-disk implementation will ignore it, but
// an object-store or subprocess one must not, and an interface that makes
// cancellation optional gets implementations that cannot be cancelled.
type Store interface {
	// Put stores r and returns a ref. mime is recorded, not verified. The
	// returned ref is valid until Delete or until the store's own retention
	// policy removes it.
	//
	// Implementations must not partially publish: a Put that fails leaves nothing
	// readable at the returned ref. A half-written blob is worse than a missing
	// one because it looks like data.
	Put(ctx context.Context, mime string, r io.Reader) (Ref, Meta, error)

	// Get opens the bytes. The caller closes.
	Get(ctx context.Context, ref Ref) (io.ReadCloser, Meta, error)

	// Stat reports what Get would report, without transferring the bytes.
	Stat(ctx context.Context, ref Ref) (Meta, error)

	// Delete removes the bytes. Deleting a ref that is already gone is not an
	// error — cleanup runs on a schedule and racing with itself must be boring.
	Delete(ctx context.Context, ref Ref) error

	// Name identifies the driver, for logs and for the console.
	Name() string
}

// URLSigner is implemented by stores that can hand out a time-limited URL the
// bytes can be fetched from directly.
//
// Optional, because most of the interesting backends can (S3, OSS, COS and the
// rest all sign) and the two that cannot — a local directory, a database column —
// are exactly the ones where proxying through Daycore is cheap anyway. Callers
// must therefore always have a proxy path and treat signing as an optimisation:
// a vision model that needs a URL gets one either way, from the object store
// directly or from Daycore's own endpoint.
type URLSigner interface {
	SignedURL(ctx context.Context, ref Ref, ttl time.Duration) (string, error)
}

// SignedURL returns a direct URL when the store can produce one, and ok=false
// when the caller should proxy the bytes itself.
func SignedURL(ctx context.Context, s Store, ref Ref, ttl time.Duration) (string, bool) {
	signer, is := s.(URLSigner)
	if !is {
		return "", false
	}
	u, err := signer.SignedURL(ctx, ref, ttl)
	if err != nil || u == "" {
		return "", false
	}
	return u, true
}

// Opener constructs a Store from a driver-specific config string.
//
// One string rather than a struct so a backend can be selected and configured
// entirely from the environment, the way DB_TYPE and DB_DSN already are.
type Opener func(cfg string) (Store, error)

var (
	mu      sync.RWMutex
	openers = map[string]Opener{}
)

// Register adds a blob driver. Panics on duplicate or empty — programmer error,
// and it happens during init() where returning an error has nowhere to go.
func Register(name string, o Opener) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" || o == nil {
		panic("blob: invalid Register arguments")
	}
	if _, dup := openers[name]; dup {
		panic("blob: store type already registered: " + name)
	}
	openers[name] = o
}

// Open builds a Store. An empty name yields a nil Store and no error: running
// without a file bus is a supported configuration — every feature that needs one
// is expected to check and to degrade to saying so, rather than the process
// refusing to boot over a capability most deployments will not use on day one.
func Open(name, cfg string) (Store, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	mu.RLock()
	o, ok := openers[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", ErrUnsupportedStore, name, Types())
	}
	return o(cfg)
}

// Types lists registered drivers, sorted.
func Types() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(openers))
	for t := range openers {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
