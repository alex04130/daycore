package auth

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h := NewHasher("server-pepper")
	enc, err := h.Hash("s3cret-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(enc, "$argon2id$") {
		t.Fatalf("expected PHC argon2id string, got %q", enc)
	}
	ok, err := h.Verify("s3cret-password", enc)
	if err != nil || !ok {
		t.Fatalf("correct password should verify (ok=%v err=%v)", ok, err)
	}
	bad, _ := h.Verify("wrong-password", enc)
	if bad {
		t.Fatal("wrong password must not verify")
	}
}

func TestPerUserSaltAndParams(t *testing.T) {
	h := NewHasher("")
	a, _ := h.Hash("same-password")
	b, _ := h.Hash("same-password")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random per-user salt)")
	}
}

func TestPepperChangesResult(t *testing.T) {
	enc, _ := NewHasher("pepper-1").Hash("pw")
	ok, _ := NewHasher("pepper-2").Verify("pw", enc)
	if ok {
		t.Fatal("verifying with a different pepper must fail")
	}
}
