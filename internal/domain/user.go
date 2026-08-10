// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// User is the authenticated account (optional — anonymous sessions have none).
type User struct {
	ID            string  `json:"id"`
	Email         *string `json:"email,omitempty"`
	Name          *string `json:"name,omitempty"`
	AvatarURL     *string `json:"avatarUrl,omitempty"`
	IsAnonymous   bool    `json:"isAnonymous"`
	DataSessionID string  `json:"-"` // canonical data session; login and claim it
	TokenVersion  int     `json:"-"` // bumped on logout to revoke all issued JWTs
	// IsOwner is the super-administrator mark: the check function returns true
	// early and never consults a permission.
	//
	// # Why a mark and not an "all permissions" role
	//
	// The two behave OPPOSITELY when a new permission is added, and that is the
	// whole argument. An all-permissions role does not contain the new id unless
	// somebody remembers to edit it — so between the deploy and that edit,
	// **nobody on earth holds the new permission**, including the person whose
	// job it is to grant it. The console gains a section that 403s for everyone,
	// and the fix is a migration or a hand-written row. Doing it automatically
	// by migration is reinventing the mark, plus a migration that can fail. The
	// quiet variant is worse: rename a permission and the old id sits in the row
	// while the new one is absent — the grant is silently lost and nothing
	// anywhere reports it.
	//
	// A mark has none of that: define the permission and the owner has it, with
	// no data change at all.
	//
	// # The cost, which is real
	//
	// "Who can export the database?" is permanently "whoever holds db.export,
	// PLUS every owner", and the second clause can never be forgotten. Two cheap
	// mitigations: the mark is read in exactly one place (the check function, so
	// "plus owners" is one grep), and any "who can do X" screen must be built on
	// that function rather than querying the tables.
	//
	// ⚠️ **This field must never be written through Upsert.** UserRepository's
	// Upsert takes a whole *User and the OAuth callback constructs one from what
	// the provider returned — so a column list that includes is_owner turns the
	// login path into a privilege escalation write. It has a narrow setter, the
	// same arrangement TokenVersion and DataSessionID already use, and a
	// four-back-end conformance case pins it.
	IsOwner   bool      `json:"isOwner"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
