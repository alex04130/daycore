package domain

import (
	"context"
	"time"
)

// The third tier: a validation rule that arrived as data rather than as a
// release.
//
// # The three tiers, and why only this one needs a person
//
//	primitives    compiled in (internal/theme/embedded.go). A floor — overridable
//	              by a later layer, never removable.
//	combinators   one-of[…] / list-of<…> / nullable<…>. NO approval, because they
//	              express nothing a primitive cannot: every leaf still goes
//	              through a primitive's validator.
//	patterns      this. A regular expression somebody wrote. It is the only tier
//	              that can say something new, so it is the only one that needs a
//	              person to agree.
//
// # ⚠️ What approval actually gates, and what it does not
//
// It does NOT gate safety. The character floor (internal/theme.CharacterFloor)
// runs before any kind is consulted and again on every leaf of a combinator, so
// a wide-open pattern is still a wide-open pattern *inside a safe alphabet* —
// there is a test that adds a `.*` kind and shows seven injection strings still
// refused.
//
// What it gates is MEANING: a pattern is a promise about what values a token may
// hold, stored themes are validated against it, and a promise nobody agreed to
// is not a promise. A frontend that could install its own validator could also
// install one that accepts anything, and then "validated" would mean nothing on
// that deployment.
type ThemeKind struct {
	// Name is the identifier a manifest writes, e.g. "spring".
	Name string `json:"name"`
	// Pattern is the regular expression. Anchored by the registry, not here —
	// see internal/theme.anchor for why that is central rather than trusted to
	// whoever wrote it.
	Pattern     string `json:"pattern"`
	Description string `json:"description,omitempty"`
	// Approved is the gate. ⚠️ Only approved kinds are merged into the running
	// registry; an unapproved row exists to be looked at and nothing else.
	Approved bool `json:"approved"`
	// ProposedBy is the frontend family whose manifest asked for it, or "" when
	// an operator wrote it themselves.
	//
	// Kept after approval on purpose: six months later "why does this
	// deployment have a `spring` kind" is a question whose answer is this
	// column.
	ProposedBy string    `json:"proposedBy,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Bounds on a proposed kind. Every one of them bounds THIRD-PARTY input.
const (
	// MaxKindName is generous next to any real kind name and small enough that
	// the column can be an indexed VARCHAR on MySQL.
	MaxKindName = 64
	//
	// ⚠️ The PATTERN's bound is deliberately not here. It lives in
	// internal/theme as MaxStoredPattern, because that package is the one that
	// compiles a pattern and therefore the only one that can refuse it — a
	// second constant here would be a number that could disagree with the number
	// actually enforced.
	//
	// MaxKindDescription is what an operator reads next to the approve button,
	// and what the model is told the shape means.
	MaxKindDescription = 200
	// MaxProposedKinds bounds how many UNAPPROVED rows may exist at once.
	//
	// ⚠️ The handshake is unauthenticated, so proposals arrive from anything
	// that can reach the server. Same answer as the family ceiling: a bound plus
	// a person, not a credential a first-contact frontend could not have.
	MaxProposedKinds = 64
)

// ThemeKindRepository stores the third tier.
//
// Deployment-wide, like frontend families: which validation rules exist is the
// operator's view of the world, not any one user's.
type ThemeKindRepository interface {
	// List returns every row, approved or not — the console needs both, because
	// the pending ones are the entire point of the screen.
	List(ctx context.Context) ([]ThemeKind, error)
	// Get resolves one by name, or ErrNotFound.
	Get(ctx context.Context, name string) (*ThemeKind, error)
	// Upsert creates or replaces one whole.
	//
	// ⚠️ Whole rather than per-field, and the caller must read before it writes.
	// A partial update would let "approve this" and "a frontend re-proposed it
	// with a different pattern" interleave into an approved row holding a
	// pattern nobody approved — which is exactly the thing approval exists to
	// prevent.
	Upsert(ctx context.Context, k ThemeKind) error
	// Delete removes one. Deleting an APPROVED kind is a real act: tokens
	// declaring it stop validating, so the caller decides — see the admin
	// handler, which says what it costs.
	Delete(ctx context.Context, name string) error
	// CountPending is the ceiling's counter. Separate from List so the
	// handshake — which runs on every frontend connection — does not read every
	// row to find out whether it may write one.
	CountPending(ctx context.Context) (int, error)
}
