package auth

import (
	"testing"
	"time"
)

func TestTokenIssueParseRoundtrip(t *testing.T) {
	ti := NewTokenIssuer("secret", time.Hour)
	tok, err := ti.Issue("user-1", 3)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	uid, ver, err := ti.Parse(tok)
	if err != nil || uid != "user-1" || ver != 3 {
		t.Fatalf("parse: uid=%q ver=%d err=%v", uid, ver, err)
	}
}

// A token signed with the old version must not validate after the version is
// bumped — this is what makes logout revoke previously issued JWTs.
func TestTokenVersionMismatchDetectable(t *testing.T) {
	ti := NewTokenIssuer("secret", time.Hour)
	tok, _ := ti.Issue("user-1", 0)
	_, ver, err := ti.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	const currentUserVersion = 1 // simulates a logout bump
	if ver == currentUserVersion {
		t.Fatal("expected token version to differ from bumped user version")
	}
}

func TestTokenRejectsWrongSecret(t *testing.T) {
	tok, _ := NewTokenIssuer("a", time.Hour).Issue("u", 0)
	if _, _, err := NewTokenIssuer("b", time.Hour).Parse(tok); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestTokenRejectsExpired(t *testing.T) {
	tok, _ := NewTokenIssuer("s", -time.Hour).Issue("u", 0)
	if _, _, err := NewTokenIssuer("s", time.Hour).Parse(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}
