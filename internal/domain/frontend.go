package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

// Frontends: two layers of identity, because a theme belongs to neither one
// alone.
//
// # Why two
//
//	buildHash   one build's fingerprint. Derived from the manifest, not
//	            declarable, so it cannot lie about being a different build.
//	familyId    the theme-compatibility group ("liuli"). Declared by the build
//	            and OVERRIDABLE BY THE OPERATOR.
//
// Themes, per-frontend preferences and the theme-switch log all key on the
// FAMILY, because that is the unit a set of themes means anything for: 琉璃-web
// and 琉璃-app want the same themes; 琉璃 and 汀 do not.
//
// The build exists so the console can say "琉璃 has three builds connected:
// web 4.2, app 1.0, embedded 0.9" — and so an operator can move a new build
// into an existing family. Without it, a version bump would look like a new
// frontend and lose every theme.
//
// # ⚠️ A manifest is THIRD-PARTY DATA
//
// Third-party and multi-platform frontends are a first-class case, so what a
// build reports is treated the way any external input is: lengths bounded,
// newlines stripped, kinds checked against what this deployment can validate,
// and `rules` kept out of the model until an operator says otherwise.
//
// It is not "our own code" even when it is.

// FallbackFamilyID is the family a request that names no build is judged
// against, and the family every theme written before families existed belongs
// to.
//
// ⚠️ It lives in domain rather than in the server package because the STORAGE
// layer needs it too: the column that carries it defaults to this exact string,
// so an upgraded database and a fresh one agree without anybody remembering to
// keep two literals in step. There is no stored row for it — see
// server/themes.go for why a seeded family would be worse than a constant.
const FallbackFamilyID = "default"

// TokenSpec is one theme variable a frontend declares.
type TokenSpec struct {
	// Name is the CSS custom property, "--primary".
	Name string `json:"name"`
	// Kind is a kind expression the deployment can validate — a primitive, or a
	// combinator over one. See internal/theme.
	Kind string `json:"kind"`
	// Description is for the operator and for the model generating themes. It
	// is the one free-text field a manifest carries, so it is bounded and
	// stripped of newlines.
	Description string `json:"description,omitempty"`
}

// FrontendFamily is a theme-compatibility group and its token space.
type FrontendFamily struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	// Tokens is the UNION of every token any build in this family has ever
	// declared.
	//
	// ⚠️ A union, never an intersection, and no subset check. 琉璃-app using a
	// token 琉璃-web does not is not a conflict — the web build simply ignores
	// what it does not recognise, which the protocol makes its responsibility.
	// Intersecting would make adding a platform DELETE tokens from the other
	// one's themes.
	Tokens []TokenSpec `json:"tokens"`
	// Rules is the frontend's own "how to design a theme for this end" prompt
	// fragment, and RulesAccepted is whether an operator approved it.
	//
	// ⚠️ Unapproved client text never reaches the model. Until somebody
	// approves it the backend generates a mechanical fragment from the token
	// list instead, so a third-party frontend has the full feature on day one
	// and the injection surface is zero by default.
	Rules         string `json:"rules,omitempty"`
	RulesAccepted bool   `json:"rulesAccepted"`
	// Pinned freezes the token space: further builds may not extend it.
	//
	// The name-squatting problem the spec leaves open: the first build to claim
	// "liuli" defines it. Pinning is the operator saying "this is the one I
	// mean", after which a build claiming the same id joins as a member and
	// cannot widen it.
	Pinned bool `json:"pinned"`
	// BackfillRequestedAt is an operator saying "fill in the tokens my stored
	// themes are missing". Zero means nobody has asked.
	//
	// ⚠️ It is REQUESTED, never automatic, because it costs one model call per
	// theme — a family with two thousand themes is two thousand calls, and the
	// widening that made them incomplete is a routine deploy. A backend that
	// started spending on its own here would be spending because somebody
	// shipped a frontend.
	//
	// It is also PERSISTENT rather than a goroutine started by the request: the
	// work outlives any one process, and a restart halfway through has to
	// resume rather than leave half the themes filled with nothing recording
	// that.
	//
	// The leader clears it when a full pass finds nothing left to do. Widening
	// the family again does NOT re-arm it — that is another decision, with
	// another price.
	// ⚠️ A POINTER, and that is not a style choice: `json:",omitempty"` does
	// NOT omit a zero time.Time on this Go version (omitzero arrived in 1.24),
	// so a value field shipped `"0001-01-01T00:00:00Z"` on every family and the
	// console's "is a backfill running" test — a truthiness check on the field —
	// was true for every family, forever. Same shape as JobRun.EndedAt, which is
	// a pointer for the same reason.
	BackfillRequestedAt *time.Time `json:"backfillRequestedAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

// TokenSpaceHash identifies this family's TOKEN SPACE — the thing a backfill is
// filling against.
//
// ⚠️ Not UpdatedAt, and the difference is the whole point. The handshake calls
// UpsertFamily on EVERY connection, whether or not anything changed, and that
// writes updated_at = now. A run key built from it therefore rotated every time
// any frontend loaded a page: each rotation minted a fresh job_runs occurrence,
// which reset the attempts counter, which meant a permanently failing theme was
// paid for again and again forever — with JobMaxAttempts recorded as 1 each
// time, so nothing looked wrong.
//
// Keyed on names AND kinds because a token whose kind changed is a different
// value to generate, and sorted because the union is built from an unordered
// merge. Truncated to 16 hex characters: the only thing it has to do is differ
// when the space differs, and collisions here would just mean one theme is not
// re-filled after a widening.
func (f FrontendFamily) TokenSpaceHash() string {
	parts := make([]string, 0, len(f.Tokens))
	for _, t := range f.Tokens {
		parts = append(parts, t.Name+":"+t.Kind)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:8])
}

// MissingTokens reports which of this family's tokens a theme does not carry.
//
// ⚠️ Missing, never extra. A theme holding a token the family has since dropped
// keeps it: the family's token space is a union that only grows in practice, a
// build that no longer uses a variable simply ignores it, and deleting values
// somebody chose in order to tidy up a list is not a trade this makes.
func (f FrontendFamily) MissingTokens(vars map[string]string) []TokenSpec {
	var out []TokenSpec
	for _, t := range f.Tokens {
		if _, ok := vars[t.Name]; !ok {
			out = append(out, t)
		}
	}
	return out
}

// TokenByName finds a declared token.
func (f FrontendFamily) TokenByName(name string) (TokenSpec, bool) {
	for _, t := range f.Tokens {
		if t.Name == name {
			return t, true
		}
	}
	return TokenSpec{}, false
}

// FrontendBuild is one build of one family, seen connecting.
type FrontendBuild struct {
	// BuildHash is the identity and the primary key.
	BuildHash   string    `json:"buildHash"`
	FamilyID    string    `json:"familyId"`
	DisplayName string    `json:"displayName,omitempty"`
	Version     string    `json:"version,omitempty"`
	MinAPI      int       `json:"minApi,omitempty"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
	// LastSeenAt is written coarsely, like a pairing's — it answers "is this
	// build still out there" and must not cost a write per handshake.
	LastSeenAt time.Time `json:"lastSeenAt"`
}

// Limits on what a manifest may declare.
//
// Every one of them is a bound on THIRD-PARTY input, not a style rule. A
// frontend that needs more than 512 theme tokens is not designing a theme.
const (
	MaxFamilyIDLen      = 64
	MaxTokenName        = 64
	MaxTokenDescription = 200
	MaxFamilyTokens     = 512
	MaxThemeRules       = 4000
	// MaxFamilies bounds what an UNAUTHENTICATED handshake can create. The
	// endpoint must be reachable before any credential exists, so the answer to
	// "anyone can make one" is a ceiling plus an operator's pin, not a
	// credential that a first-contact frontend could not have.
	MaxFamilies = 64
)

// FrontendRepository stores families and the builds that connect to them.
//
// Deployment-wide: which frontends exist is the operator's view of the world,
// not any one user's.
type FrontendRepository interface {
	ListFamilies(ctx context.Context) ([]FrontendFamily, error)
	GetFamily(ctx context.Context, id string) (*FrontendFamily, error)
	// UpsertFamily creates or replaces a family whole.
	//
	// Whole rather than per-field: the token space is computed as a union by the
	// caller, which needs the previous value anyway, and a partial update would
	// invite two writers each merging against a different starting point.
	UpsertFamily(ctx context.Context, f FrontendFamily) error

	// DeleteFamily removes a family. The caller decides whether that is safe —
	// see the admin handler, which refuses while builds still point at it.
	DeleteFamily(ctx context.Context, id string) error

	ListBuilds(ctx context.Context) ([]FrontendBuild, error)
	// GetBuild resolves one build, or ErrNotFound.
	//
	// ⚠️ On the theme read/write path (a request naming its build in a header),
	// so it is one indexed read. Resolving it by scanning ListBuilds would make
	// every theme request cost the whole table.
	GetBuild(ctx context.Context, buildHash string) (*FrontendBuild, error)
	// SeenBuild records a handshake: creates the build on first sight, and
	// afterwards only moves LastSeenAt — and only when it is already stale.
	//
	// ⚠️ It must NOT overwrite FamilyID on a repeat sighting. An operator who
	// moved a build into another family would otherwise have that undone by the
	// build's next startup, silently, and the themes would follow.
	SeenBuild(ctx context.Context, b FrontendBuild, staleBefore time.Time) error
	// SetBuildFamily is the operator moving a build between families.
	SetBuildFamily(ctx context.Context, buildHash, familyID string) error
}
