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

// sessionClaims carries the user id (subject) plus a token version that the
// server matches against the user's current version — a mismatch (after logout
// bumps it) invalidates the token even though the JWT is otherwise valid.
type sessionClaims struct {
	TokenVersion int `json:"tv"`
	jwt.RegisteredClaims
}

// Issue returns a signed JWT whose subject is the user id and which carries the
// given token version.
func (t *TokenIssuer) Issue(userID string, version int) (string, error) {
	now := time.Now()
	claims := sessionClaims{
		TokenVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Parse validates a JWT and returns its subject (user id) and token version.
func (t *TokenIssuer) Parse(tokenString string) (string, int, error) {
	claims := &sessionClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return t.secret, nil
	})
	if err != nil || !tok.Valid || claims.Subject == "" {
		return "", 0, ErrInvalidToken
	}
	return claims.Subject, claims.TokenVersion, nil
}
