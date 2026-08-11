package domain

import (
	"context"
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
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
