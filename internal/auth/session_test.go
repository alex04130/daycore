package auth

import (
	"strings"
	"testing"
)

func TestCookieSignerRoundtripAndTampering(t *testing.T) {
	c := NewCookieSigner("secret-key")
	signed := c.Sign("session-value")
	if got, ok := c.Verify(signed); !ok || got != "session-value" {
		t.Errorf("Verify(Sign(x)) = (%q, %v)", got, ok)
	}
	// Any tampering must fail: value, tag, or both.
	cases := []string{
		"session-other." + signed[strings.LastIndexByte(signed, '.')+1:],
		signed[:len(signed)-2] + "xx",
		"no-dot-at-all",
		"",
	}
	for _, tc := range cases {
		if got, ok := c.Verify(tc); ok {
			t.Errorf("Verify(%q) must fail, got (%q, true)", tc, got)
		}
	}
	// A different key cannot verify.
	other := NewCookieSigner("different-key")
	if _, ok := other.Verify(signed); ok {
		t.Error("a different key must not verify")
	}
	// A value containing a dot splits at the LAST dot — the value part
	// keeps everything before it.
	v := "a.b.c"
	signed2 := c.Sign(v)
	if got, ok := c.Verify(signed2); !ok || got != v {
		t.Errorf("a dotted value must round-trip, got (%q, %v)", got, ok)
	}
}

func TestNewSessionIDFormat(t *testing.T) {
	a, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 {
		t.Errorf("32 bytes of URL-safe base64 = 43 chars, got %d", len(a))
	}
	if a == b {
		t.Error("two session ids collided")
	}
	for _, r := range a {
		if !(r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			t.Errorf("session ids must be URL-safe, saw %q in %q", r, a)
		}
	}
}
