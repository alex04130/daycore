package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when a JWT is missing, expired, or has a bad signature.
var ErrInvalidToken = errors.New("invalid token")

// TokenIssuer signs and verifies short-lived session JWTs (HS256).
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenIssuer builds a TokenIssuer.
func NewTokenIssuer(secret string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: []byte(secret), ttl: ttl}
}

// Scope separates the kinds of thing a token may be. A token minted for one is
// never accepted for the other.
//
// Without it the two are the same string signed with the same key, so a user
// session JWT would authenticate against the admin API — and the user path is
// the one anybody can obtain by signing up.
type Scope string

const (
	// ScopeUser is a signed-in end user. Written as "" on the wire so that
	// tokens issued before scopes existed keep working; ParseScoped normalises.
	ScopeUser Scope = ""
	// ScopeAdmin is the operator console. It carries NO revocation mechanism —
	// there is no admin user row to bump a token version on — so its TTL is its
	// revocation, which is why AdminTokenTTL is short and must stay short.
	ScopeAdmin Scope = "admin"
)

// sessionClaims carries the user id (subject) plus a token version that the
// server matches against the user's current version — a mismatch (after logout
// bumps it) invalidates the token even though the JWT is otherwise valid.
type sessionClaims struct {
	TokenVersion int   `json:"tv"`
	Scope        Scope `json:"scp,omitempty"`
	jwt.RegisteredClaims
}

// AdminTokenTTL is how long a console login lasts.
//
// Short because it cannot be revoked: token-version revocation needs a database
// row and there is no admin user, and the planned degraded boot serves the
// console with no database at all. Thirty minutes is the compromise between
// "the operator re-authenticates constantly" and "a stolen cookie is useful all
// afternoon" — and it is the ONLY thing standing between those two, so raising
// it is a security change, not a convenience tweak.
const AdminTokenTTL = 30 * time.Minute

// Issue returns a signed user JWT whose subject is the user id and which carries
// the given token version.
func (t *TokenIssuer) Issue(userID string, version int) (string, error) {
	return t.issue(userID, version, ScopeUser, t.ttl)
}

// IssueAdmin returns a short-lived console token. subject is a label for the
// ledger, not an identity — there are no admin accounts.
func (t *TokenIssuer) IssueAdmin(subject string) (string, error) {
	return t.issue(subject, 0, ScopeAdmin, AdminTokenTTL)
}

func (t *TokenIssuer) issue(subject string, version int, scope Scope, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := sessionClaims{
		TokenVersion: version,
		Scope:        scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Parse validates a USER JWT and returns its subject and token version.
//
// It refuses any other scope. That refusal is the point: without it an admin
// token would also be a valid user session, and more importantly the reverse
// check (ParseAdmin) would be the only thing separating the two — one guard
// instead of two, on the boundary that matters most.
func (t *TokenIssuer) Parse(tokenString string) (string, int, error) {
	claims, err := t.parse(tokenString)
	if err != nil {
		return "", 0, err
	}
	if claims.Scope != ScopeUser {
		return "", 0, ErrInvalidToken
	}
	return claims.Subject, claims.TokenVersion, nil
}

// ParseAdmin validates a console token. A user session token is refused here
// even though it is signed with the same key and is otherwise valid — which is
// the whole reason the scope claim exists, since anybody can obtain a user
// token by signing up.
func (t *TokenIssuer) ParseAdmin(tokenString string) error {
	claims, err := t.parse(tokenString)
	if err != nil {
		return err
	}
	if claims.Scope != ScopeAdmin {
		return ErrInvalidToken
	}
	return nil
}

func (t *TokenIssuer) parse(tokenString string) (*sessionClaims, error) {
	claims := &sessionClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return t.secret, nil
	})
	if err != nil || !tok.Valid || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
