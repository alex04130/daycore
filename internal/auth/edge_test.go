package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newIssuer(t *testing.T) *TokenIssuer {
	t.Helper()
	return NewTokenIssuer("test-secret", time.Hour)
}

// Tokens are HS256. Anything else — RS256, ES256, alg=none — must be refused,
// and an empty subject must never authenticate anything.
func TestParseRejectsNonHMACAndEmptySubject(t *testing.T) {
	i := newIssuer(t)
	// Forge an RS256 token and force the issuer to try its HS256 key.
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rs := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": "u1"})
	rsStr, err := rs.SignedString(rsaPriv)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := i.Parse(rsStr); err == nil {
		t.Error("a non-HMAC algorithm must be refused")
	}
	// HS512 signed with the same secret is ACCEPTED today — the keyfunc only
	// checks for any HMAC family. Locked as the current behaviour.
	hs512 := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{"sub": "u1", "tv": 0})
	hs512Str, err := hs512.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := i.Parse(hs512Str); err != nil {
		t.Errorf("HS512 with the same key is currently accepted (locked), got %v", err)
	}
	// An empty subject is refused even when the signature is valid.
	empty := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "", "tv": 0})
	emptyStr, _ := empty.SignedString([]byte("test-secret"))
	if _, _, err := i.Parse(emptyStr); err == nil {
		t.Error("an empty subject must be refused")
	}
}

func TestAdminScopeSeparation(t *testing.T) {
	i := newIssuer(t)
	// A user token never authenticates the admin API.
	userTok, err := i.Issue("u1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, root, err := i.ParseAdmin(userTok); err == nil || root {
		t.Errorf("a user token must be refused by ParseAdmin, got (%v, root=%v)", err, root)
	}
	// An admin token never authenticates the user API.
	adminTok, err := i.IssueAdminUser("u1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := i.Parse(adminTok); err == nil {
		t.Error("an admin token must be refused by Parse")
	}
	// The root admin token reports root.
	rootTok, err := i.IssueAdminRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, root, err := i.ParseAdmin(rootTok); err != nil || !root {
		t.Errorf("ParseAdmin(root) = (%v, %v)", err, root)
	}
	// IssueAdminUser with an empty id is refused.
	if _, err := i.IssueAdminUser(""); err == nil {
		t.Error("IssueAdminUser(empty) must be refused")
	}
}

func TestWrongTypeTokenVersion(t *testing.T) {
	// tv is an int claim; a string tv fails to decode and the token is invalid
	// rather than silently version-less.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "u1", "tv": "3"})
	str, _ := tok.SignedString([]byte("test-secret"))
	if _, _, err := newIssuer(t).Parse(str); err == nil {
		t.Error("a string tv must be refused")
	}
}

func TestDecodePHCRejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"not-a-phc",
		"$argon2id$v=19$m=65536,t=3,p=4$",
		"$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=x$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=xyz,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=4$!!!$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$!!!",
	}
	for _, in := range bad {
		if _, _, _, err := decodePHC(in); err == nil {
			t.Errorf("decodePHC(%q) must fail", in)
		}
	}
}

func TestVerifyMalformedHashNeverPanics(t *testing.T) {
	h := NewHasher("")
	for _, in := range []string{"garbage", "", "$argon2id$only-two$", "$argon2id$v=19$m=0,t=0,p=0$c2FsdA$aGFzaA"} {
		ok, err := h.Verify("pw", in)
		if ok {
			t.Errorf("Verify(%q) must not return ok", in)
		}
		if err == nil {
			t.Errorf("Verify(%q) must return the parse error", in)
		}
	}
}

func TestPepperIsRequiredToVerify(t *testing.T) {
	withPepper := NewHasher("pepper")
	hash, err := withPepper.Hash("pw")
	if err != nil {
		t.Fatal(err)
	}
	// The same password against a pepper-less hasher must fail — the pepper
	// is folded into the input BEFORE argon2, so removing it changes the key
	// material entirely.
	if ok, _ := NewHasher("").Verify("pw", hash); ok {
		t.Error("a pepper-less hasher must not verify a peppered hash")
	}
	if ok, err := withPepper.Verify("pw", hash); !ok || err != nil {
		t.Errorf("the right pepper must verify, got (%v, %v)", ok, err)
	}
}
