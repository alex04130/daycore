package auth

import (
	"errors"
	"strings"
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

// A console token's subject WAS "a label for the ledger, not an identity —
// there are no admin accounts". That stopped being true when permissions
// landed: a console session minted from a person's own login carries that
// person's id, and the authorisation path resolves their roles from it on every
// request.
//
// So the subject now names one of two things, and telling them apart has to be
// impossible to get wrong — it is the line between "passes everything" and
// "passes what their roles say".
//
//	"root"     the root credential (ADMIN_TOKEN). No identity, and none is
//	           needed: it passes everything by definition.
//	"u:<id>"   a person. Their permissions are read fresh from the database on
//	           every request, so revoking one takes effect on their NEXT
//	           request rather than when this token expires.
//
// # Why a prefix rather than a bare user id
//
// The tempting encoding is "empty subject means root, anything else is a user
// id" — and it is wrong twice. parse() refuses an empty subject (a degenerate
// token should not authenticate anything), and more importantly a bare id and a
// sentinel share one namespace: the day a user id can be the string "root",
// signing up with the right id is a privilege escalation. Prefixing every
// person makes the two sets disjoint by construction, which is the only form of
// "cannot collide" that does not depend on how ids happen to be generated
// today.
//
// ⚠️ Permissions are deliberately NOT in the token. userMW already reads the
// user row on every authenticated request (to check TokenVersion), so carrying
// them here would save no query and would buy a revocation window measured in
// the TTL. See docs/AUTH.md.
const (
	adminRootSubject = "root"
	adminUserPrefix  = "u:"
)

// IssueAdminRoot mints a console token for the root credential.
func (t *TokenIssuer) IssueAdminRoot() (string, error) {
	return t.issue(adminRootSubject, 0, ScopeAdmin, AdminTokenTTL)
}

// IssueAdminUser mints a console token for one person.
func (t *TokenIssuer) IssueAdminUser(userID string) (string, error) {
	if userID == "" {
		return "", ErrInvalidToken
	}
	return t.issue(adminUserPrefix+userID, 0, ScopeAdmin, AdminTokenTTL)
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

// ParseAdmin validates a console token and reports whose it is. A user session
// token is refused here even though it is signed with the same key and is
// otherwise valid — which is the whole reason the scope claim exists, since
// anybody can obtain a user token by signing up.
//
// Exactly one of the two returns is meaningful: root true means the root
// credential and userID is empty; root false means userID names a person. A
// subject in neither shape is refused rather than guessed at.
func (t *TokenIssuer) ParseAdmin(tokenString string) (userID string, root bool, err error) {
	claims, err := t.parse(tokenString)
	if err != nil {
		return "", false, err
	}
	if claims.Scope != ScopeAdmin {
		return "", false, ErrInvalidToken
	}
	if claims.Subject == adminRootSubject {
		return "", true, nil
	}
	id, ok := strings.CutPrefix(claims.Subject, adminUserPrefix)
	if !ok || id == "" {
		// Tokens minted before the two shapes existed land here, and refusing
		// them is right: the old subject was the literal "console", which names
		// neither a person nor the root credential. The TTL is thirty minutes,
		// so the whole population of them is gone within half an hour of a
		// deploy.
		return "", false, ErrInvalidToken
	}
	return id, false, nil
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
