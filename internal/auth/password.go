// Package auth provides password hashing (argon2id), JWT issuance, signed
// anonymous-session cookies, and a config-driven OAuth2 manager.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the cost parameters for one hash. They are encoded into the
// stored PHC string, so every user can carry different parameters and the
// verifier reads them back from the hash itself.
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32 // iterations
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

// baseArgon2 is the lower bound; Hash() jitters time/memory per user above it.
var baseArgon2 = Argon2Params{Memory: 64 * 1024, Time: 3, Threads: 4, KeyLen: 32, SaltLen: 16}

// maxConcurrentHashes caps how many argon2id computations run at once. Each uses
// 64–128 MiB, so an unauthenticated flood of login/register calls could exhaust
// memory; this bounds the peak (≤ ~512 MiB) at the cost of queueing.
const maxConcurrentHashes = 4

// ErrBadHashFormat is returned when a stored hash cannot be parsed.
var ErrBadHashFormat = errors.New("invalid argon2id hash format")

// Hasher hashes and verifies passwords with argon2id. An optional server-side
// pepper is folded in via HMAC so a DB leak alone cannot be brute-forced.
type Hasher struct {
	pepper []byte
	sem    chan struct{} // bounds concurrent argon2 computations
}

// NewHasher builds a Hasher. pepper may be empty.
func NewHasher(pepper string) *Hasher {
	return &Hasher{pepper: []byte(pepper), sem: make(chan struct{}, maxConcurrentHashes)}
}

// acquire blocks until a hashing slot is free; the returned func releases it.
func (h *Hasher) acquire() func() {
	if h.sem == nil {
		return func() {}
	}
	h.sem <- struct{}{}
	return func() { <-h.sem }
}

// Hash returns a PHC-encoded argon2id string with a fresh random salt and
// per-user randomized cost parameters.
func (h *Hasher) Hash(password string) (string, error) {
	defer h.acquire()()
	p := baseArgon2
	// Per-user cost jitter: iterations in [3,5], memory in {64,96,128} MiB.
	p.Time = baseArgon2.Time + randUint32(0, 2)
	p.Memory = []uint32{64 * 1024, 96 * 1024, 128 * 1024}[randUint32(0, 2)]

	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey(h.mix(password, salt), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return encodePHC(p, salt, key), nil
}

// Verify reports whether password matches the stored PHC-encoded hash, using a
// constant-time comparison.
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	defer h.acquire()()
	p, salt, want, err := decodePHC(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey(h.mix(password, salt), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return subtle.ConstantTimeCompare(want, got) == 1, nil
}

// mix folds the server pepper into the password input (HMAC-SHA256 keyed by the
// pepper, also bound to the per-user salt). With no pepper it returns the raw
// password bytes, so argon2id still applies its own per-user salt.
func (h *Hasher) mix(password string, salt []byte) []byte {
	if len(h.pepper) == 0 {
		return []byte(password)
	}
	mac := hmac.New(sha256.New, h.pepper)
	mac.Write(salt)
	mac.Write([]byte(password))
	return mac.Sum(nil)
}

func encodePHC(p Argon2Params, salt, key []byte) string {
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads, b64(salt), b64(key))
}

func decodePHC(encoded string) (Argon2Params, []byte, []byte, error) {
	var p Argon2Params
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, ErrBadHashFormat
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return p, nil, nil, ErrBadHashFormat
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return p, nil, nil, ErrBadHashFormat
	}
	// Degenerate cost parameters must be refused BEFORE argon2 sees them: with
	// m=0 or t=0 the reference implementation panics (it requires
	// memory >= 8*threads and at least one pass), and a panic here is a login
	// endpoint that any corrupted hash row can turn into a 500. The upper
	// bounds keep a hand-crafted hash from allocating absurd memory — the
	// hasher itself only ever writes 64–128 MiB, so anything far beyond that
	// is garbage rather than a cost we chose.
	if p.Memory == 0 || p.Time == 0 || p.Threads == 0 ||
		p.Memory < 8*uint32(p.Threads) ||
		p.Memory > 1<<20 || p.Time > 32 || p.Threads > 64 {
		return p, nil, nil, ErrBadHashFormat
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, ErrBadHashFormat
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return p, nil, nil, ErrBadHashFormat
	}
	p.SaltLen = uint32(len(salt))
	p.KeyLen = uint32(len(key))
	return p, salt, key, nil
}

// randUint32 returns a uniform-ish value in [min, max] using crypto/rand.
func randUint32(min, max uint32) uint32 {
	if max <= min {
		return min
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return min
	}
	return min + binary.BigEndian.Uint32(b[:])%(max-min+1)
}
