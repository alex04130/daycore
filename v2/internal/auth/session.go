package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// NewSessionID returns a cryptographically random, URL-safe session id. This
// replaces v1's client-generated UUID that traveled in a query param (guessable
// + no integrity), with a server-issued id carried in a signed httpOnly cookie.
func NewSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CookieSigner appends/verifies an HMAC tag on cookie values to detect tampering.
type CookieSigner struct {
	key []byte
}

// NewCookieSigner builds a CookieSigner from the configured secret.
func NewCookieSigner(secret string) *CookieSigner { return &CookieSigner{key: []byte(secret)} }

// Sign returns "value.<hmac>" — safe to store in a cookie.
func (c *CookieSigner) Sign(value string) string {
	return value + "." + c.tag(value)
}

// Verify checks the signature and returns the original value.
func (c *CookieSigner) Verify(signed string) (string, bool) {
	i := strings.LastIndexByte(signed, '.')
	if i < 0 {
		return "", false
	}
	value, sig := signed[:i], signed[i+1:]
	if subtle.ConstantTimeCompare([]byte(sig), []byte(c.tag(value))) != 1 {
		return "", false
	}
	return value, true
}

func (c *CookieSigner) tag(value string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
