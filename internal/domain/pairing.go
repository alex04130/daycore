package domain

import (
	"context"
	"time"
)

// A pairing is an external console attached to this deployment.
//
// # What it is for
//
// Two things that turn out to be one thing:
//
//   - **Cluster management.** A console that watches several deployments has to
//     authenticate to each of them, and it cannot be given ADMIN_TOKEN — that is
//     the root password, it is in the environment, and handing it to a program
//     running somewhere else makes every deployment as compromised as the
//     weakest console.
//   - **Somebody else's console.** The author's own words: "有些人就可以写他们
//     自己喜欢的控制台了". The moment attaching is a defined act rather than a
//     shared secret, the console we embed stops being THE console and becomes
//     ONE of them — and the API is the contract between them.
//
// # The backend issues the ticket; the console does not guess a password
//
// The direction matters and it was the author's decision: "这个密钥是后端生成
// 的，让人自己搬过去". An operator creates a pairing here, is shown a key ONCE,
// and carries it to the other console by hand.
//
// The alternative — the external console presenting some shared secret it was
// configured with — has no moment at which anybody decided to attach it. There
// is no list of what is attached, no way to detach one thing without changing
// the secret for everything, and no name on any of it.
//
// # ⚠️ A pairing is not a tier of access. It holds ROLES.
//
// It resolves its permissions through exactly the same roles a person does, so
// there is one permission model rather than two. A pairing with no roles can
// authenticate and do nothing, which is a real and useful state: it proves the
// key works before anything is granted.
//
// The escalation caveat is the same one that applies to people and is written
// out in internal/server/permissions.go: a pairing given roles.edit can grant
// itself anything. That is a property of the model, not of pairings.
//
// # ⚠️ It is deliberately NOT a way into degraded mode
//
// Pairings live in the database. When storage is down they cannot be read, so a
// degraded process accepts only the root credential — which is the arrangement
// degraded boot was built around, and the reason ADMIN_TOKEN cannot be replaced
// by this.
type Pairing struct {
	// ID is the public half of the key and the lookup key. It travels in the
	// clear inside the presented credential, which is what makes verification
	// one row read rather than a hash against every pairing.
	ID string `json:"id"`
	// Name is what the operator recognises it by — "运营台", "cluster-eu".
	Name string `json:"name"`
	// Description is why it exists. A pairing called "temp" with no explanation
	// is a credential nobody dares revoke in six months.
	Description string `json:"description,omitempty"`
	// SecretHash is SHA-256 of the secret half.
	//
	// ⚠️ NOT argon2, and that is deliberate rather than lazy. Password hashing
	// is slow because passwords are LOW ENTROPY and the attack is guessing. This
	// secret is 256 bits from crypto/rand: there is nothing to guess, and a slow
	// hash would only mean burning CPU on every request an external console
	// makes. The rule is entropy, not habit — if this ever becomes something a
	// human chooses, it needs argon2 that same day.
	SecretHash string `json:"-"`
	// Roles are the groups this pairing belongs to, resolved through the same
	// union as a person's.
	Roles []string `json:"roles"`
	// LastSeenAt answers the only question anybody asks before revoking one:
	// is this still being used? Written coarsely — see the repository.
	LastSeenAt time.Time `json:"lastSeenAt,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// PairingKeyPrefix marks the credential so a leaked one is recognisable on
// sight — in a log, a screenshot, a paste. Secret scanners key off exactly this
// kind of prefix.
const PairingKeyPrefix = "dcp"

// PairingLastSeenGranularity is how stale LastSeenAt may be before it is
// rewritten.
//
// A write per request would put the busiest external console into the write
// path of every one of its own reads, to maintain a field nobody reads more
// than once a week. Five minutes answers "is this in use" exactly as well.
const PairingLastSeenGranularity = 5 * time.Minute

// PairingRepository stores external console attachments.
//
// Deployment-wide, like Setting and Role: these are the operator's decisions,
// and there is one operator view.
type PairingRepository interface {
	// List returns every pairing, oldest first. Secrets are never included —
	// the hash is not serialised and the plaintext was never stored.
	List(ctx context.Context) ([]Pairing, error)
	// Get returns one by id, or ErrNotFound.
	//
	// ⚠️ This is on the authorisation path, so it is one indexed read by id. It
	// must not become a scan: verifying a credential by hashing against every
	// pairing would make attaching a hundred consoles a hundred hashes per
	// request, and the shape that invites it is a Get that takes the secret.
	Get(ctx context.Context, id string) (*Pairing, error)
	// Create stores a new pairing. The caller has already generated the id and
	// hashed the secret.
	Create(ctx context.Context, p *Pairing) error
	// SetRoles replaces which groups a pairing belongs to.
	SetRoles(ctx context.Context, id string, roles []string) error
	// TouchLastSeen records that this pairing was used at `now`, but only if the
	// stored value is older than PairingLastSeenGranularity. It reports whether
	// it wrote.
	//
	// The throttle is in the STORE rather than in the caller because the caller
	// is the authorisation path, and a rule that lives there is a rule the next
	// authorisation path will not know about.
	TouchLastSeen(ctx context.Context, id string, now time.Time) (bool, error)
	// Delete detaches a console. Deleting one that is not there is not an error:
	// two operators revoking the same key is exactly the situation revocation
	// exists for.
	Delete(ctx context.Context, id string) error
}
