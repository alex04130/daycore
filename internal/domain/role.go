package domain

import (
	"context"
	"time"
)

// Role is a named set of console permissions, and also a user group.
//
// # One table for two things, and why that is not a conflation
//
// A role with a NON-EMPTY permission set is an administrator role. A role with
// an EMPTY one is a plain group — which is what a commercial tier is. They are
// the same shape because the only difference between them is a field, and
// splitting them into two tables would mean two membership tables, two
// assignment endpoints, and a decision at every call site about which one is
// being talked about.
//
// It also makes the escalation boundary checkable rather than remembered:
// "assign a user to a group that carries no admin permission" is
// `len(Permissions) == 0`, evaluated at the moment of assignment against the
// role as it is right then. No stored flag to go stale.
//
// # Flat. Roles do not contain roles.
//
// Nesting buys deduplication, and at this scale — 18 permissions, a handful of
// people — there is no duplication worth removing. It sells three things: cycle
// detection, "who can export the database" turning from a query into a closure
// computation, and the failure that makes people regret it — **editing a leaf
// role silently changes what every role above it means.** The classic incident
// is adding one permission to a "read only" base that everybody inherits.
//
// Review threshold, so this is a decision and not an assumption: if roles pass
// about fifteen and their contents overlap heavily, the thing to add is
// one-level composition, not general nesting.
//
// # Grants only. There is no deny.
//
// A deny entry is only worth having when the model can OVER-grant — through
// inheritance, wildcards, or per-user overrides. All three are absent here by
// decision, so nothing needs subtracting. Keeping deny would cost an
// unanswerable precedence question: deny-wins means adding a role can make
// somebody able to do LESS, so no grant's effect is predictable without reading
// every role they hold; allow-wins means deny is decorative and somebody will
// rely on it anyway.
//
// "Suspend this person" is not a deny — it is removing them from their roles,
// or the account being disabled, which is a different mechanism.
type Role struct {
	// Name is the identifier and the label. One string rather than an id plus a
	// display name: a role's name is what an operator reasons about, and a
	// renameable label over a stable id means the audit log and the console can
	// disagree about what happened.
	Name string `json:"name"`
	// Description is for the person doing the granting. A role called "support"
	// with no explanation is a role nobody dares change in six months.
	Description string `json:"description,omitempty"`
	// Permissions is the set of ids from the server's registry. An id that is
	// not in the registry is refused at write time rather than stored — a stored
	// permission nothing matches is a role that quietly grants less than its
	// definition claims.
	//
	// ⚠️ Empty means this is a plain group (a commercial tier), and that is a
	// meaningful state rather than an unfinished one.
	Permissions []string `json:"permissions"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsAdminRole reports whether this role carries any console permission.
//
// Derived, never stored. A stored flag would be a second source of truth that
// goes stale the moment somebody edits the permission set — and it would go
// stale in the dangerous direction, because the check that uses it is the one
// keeping support staff from assigning people into a powerful group.
func (r Role) IsAdminRole() bool { return len(r.Permissions) > 0 }

// RoleRepository stores roles and their membership.
//
// Deployment-wide, like Setting and ProviderOverride: these are the operator's
// definitions, and there is one operator view.
type RoleRepository interface {
	// ListRoles returns every role, sorted by name.
	ListRoles(ctx context.Context) ([]Role, error)
	// GetRole returns one role, or ErrNotFound.
	GetRole(ctx context.Context, name string) (*Role, error)
	// UpsertRole creates or replaces a role's definition.
	UpsertRole(ctx context.Context, r Role) error
	// DeleteRole removes a role AND its membership.
	//
	// Both, in one call, because the alternative is orphaned membership rows
	// that grant nothing until somebody recreates a role with the same name —
	// at which point a set of people silently regain a permission set that was
	// deleted. Deleting a role that does not exist is not an error.
	DeleteRole(ctx context.Context, name string) error

	// RolesOf returns the role names a user belongs to, sorted.
	//
	// Called on the authorisation path, so it is one query by user id. It
	// returns names rather than Roles because the caller almost always needs the
	// permission union, and resolving that from the (small, cached-in-memory)
	// role list beats a join the four back ends would each implement
	// differently.
	RolesOf(ctx context.Context, userID string) ([]string, error)
	// MembersOf returns the user ids in a role, sorted.
	//
	// This is what makes "who can export the database" answerable, which is the
	// question the whole model exists to keep answerable.
	MembersOf(ctx context.Context, name string) ([]string, error)
	// AddMember puts a user in a role. Adding somebody who is already there is
	// not an error — two consoles clicking it is ordinary.
	AddMember(ctx context.Context, name, userID string) error
	// RemoveMember takes a user out. Removing somebody who is not there is not
	// an error, for the same reason.
	RemoveMember(ctx context.Context, name, userID string) error
}
