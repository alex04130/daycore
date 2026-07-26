// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// User is the authenticated account (optional — anonymous sessions have none).
type User struct {
	ID            string    `json:"id"`
	Email         *string   `json:"email,omitempty"`
	Name          *string   `json:"name,omitempty"`
	AvatarURL     *string   `json:"avatarUrl,omitempty"`
	IsAnonymous   bool      `json:"isAnonymous"`
	DataSessionID string    `json:"-"` // canonical data session; login and claim it
	TokenVersion  int       `json:"-"` // bumped on logout to revoke all issued JWTs
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Credential stores the password verifier for a user. PasswordHash is a PHC
// encoded string (e.g. "$argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>") so the
// per-user salt and per-user cost parameters live inside it — exactly the
// "different salt and iterations for every user, stored in the DB" requirement.
type Credential struct {
	UserID       string    `json:"userId"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// OAuthIdentity links a user to an external OAuth provider account.
type OAuthIdentity struct {
	ID             string    `json:"id"`
	UserID         string    `json:"userId"`
	Provider       string    `json:"provider"`       // e.g. "google", "github"
	ProviderUserID string    `json:"providerUserId"` // the subject id at the provider
	CreatedAt      time.Time `json:"createdAt"`
}
