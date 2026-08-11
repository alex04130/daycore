// Package theme validates theme token values.
//
// # What this is for
//
// A theme is a map of CSS custom properties. The values reach a stylesheet, so
// "what may a value look like" is a security question before it is a design
// one — and it is also the type signature an AI needs in order to generate or
// backfill one.
//
// # The set of kinds is CLOSED at any instant, and is DATA rather than code
//
// Two things that were conflated in an earlier draft of the protocol and are
// deliberately separated here:
//
//   - **Only known kinds are accepted, ever.** That is the nail the value-level
//     injection surface hangs on.
//   - **Adding a kind should not need a release.** That was an implementation
//     detail masquerading as a security requirement.
//
// So kinds live in three layers, exactly like the message catalogue: database →
// `TOKEN_KINDS_DIR/*.json` → embedded. Adding one is a row or a file, not a
// deploy. The embedded six are the floor and can never be removed.
//
// # Three tiers, and only the last needs a person
//
//	primitives    color, length, number, ratio, duration, enum. Closed set,
//	              stored in the three layers above.
//	combinators   one-of[…], list-of<…>, nullable<…>. Parsed here, no approval:
//	              they CANNOT express anything a primitive cannot, because every
//	              leaf still goes through a primitive's validator.
//	patterns      a frontend supplying its own regex. Needs an operator's
//	              approval, after which it is simply a primitive.
//
// Most real requests for "a new kind" land in the middle tier, where nothing
// has to change at all.
//
// # ⚠️ The character floor is what the guarantee actually rests on
//
// EVERY value, whatever its kind, must also pass CharacterFloor. That is why
// the guarantee does not depend on the kind set being closed IN CODE: a
// third-tier pattern written too loosely is loose inside a safe alphabet rather
// than being a hole. An earlier draft hung the guarantee on the set being
// compiled in, and that is where "adding a kind needs a release" came from.
package theme

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Kind is a value type for a theme token.
type Kind struct {
	// Name is what a manifest writes: "color", "length", "clamp".
	Name string
	// Pattern is an RE2 regexp the whole value must match.
	//
	// ⚠️ Go's regexp is RE2 — linear time, no backtracking — which is the ONLY
	// reason a third-party-supplied pattern is affordable here. In a
	// backtracking engine an approved regex would be a denial-of-service
	// waiting for the right input. Do not port this design to one.
	Pattern string
	// Description is what the console shows an operator deciding whether to
	// approve one, and what an AI is told the shape is.
	Description string
	// Origin says which layer this came from, so the console can show whether a
	// kind is built in, dropped in as a file, or approved into the database.
	Origin string

	re *regexp.Regexp
}

// The three origins, in precedence order (later wins).
const (
	OriginEmbedded = "embedded"
	OriginFile     = "file"
	OriginDB       = "db"
)

// forbidden is the character floor: substrings no theme value may contain,
// whatever its kind.
//
// Each entry is here because of a specific thing it makes impossible, not as a
// general precaution:
//
//	url(   a value used in a property that resolves it is a beacon — it reports
//	       every render to a third party, from inside a page the user trusts.
//	; } {  ending the declaration and opening another is how one value becomes
//	       arbitrary CSS.
//	<      the value may be rendered into an inline style attribute.
//	\      CSS escapes can reconstruct any of the above.
//	/*     a comment can swallow the rest of a rule, changing what follows it.
//	\n \r  the same, and it breaks any line-oriented sanitiser downstream.
var forbidden = []string{"url(", ";", "}", "{", "<", ">", "\\", "/*", "*/", "\n", "\r"}

// CharacterFloor is the check every value passes regardless of kind.
//
// ⚠️ Applied to the RAW value, before any kind-specific parsing — which is why
// a combinator does NOT need to re-apply it per item: every item is a substring
// of a value this already searched.
func CharacterFloor(value string) error {
	for _, bad := range forbidden {
		if strings.Contains(value, bad) {
			return fmt.Errorf("value contains %q, which is never allowed in a theme value", bad)
		}
	}
	if len(value) > MaxValueLen {
		return fmt.Errorf("value is %d characters; the limit is %d", len(value), MaxValueLen)
	}
	return nil
}

// MaxValueLen bounds a single token value.
//
// Generous for anything real (`0 2px 8px rgba(0,0,0,0.1)` is 26) and small
// enough that a manifest cannot use theme values as a storage channel.
const MaxValueLen = 256

// Registry is the set of kinds this deployment accepts.
//
// # ⚠️ Two maps, not one, and the reason is DELETION
//
//	base   embedded then file. Built once at boot, never replaced.
//	db     the third tier, replaced WHOLESALE by SetDBKinds.
//
// Merge is additive and cannot remove a kind — which is exactly right for a
// floor that must never disappear, and exactly wrong for a layer an operator
// edits. With one map, "the operator deleted a kind" could only take effect on
// restart, so a kind revoked because its pattern turned out to be too loose
// would keep validating writes until somebody redeployed.
//
// Splitting them also means a bad database layer cannot destroy the file layer:
// SetDBKinds replaces only `db`, and a row that will not compile is skipped
// while everything else lands.
//
// Same shape as i18n.Catalog (embedded / files / db) for the same reasons.
//
// Safe for concurrent use: read on every theme write, replaced when an operator
// approves or revokes.
type Registry struct {
	mu   sync.RWMutex
	base map[string]Kind
	db   map[string]Kind
}

// NewRegistry returns a registry holding only the embedded floor.
func NewRegistry() *Registry {
	r := &Registry{base: map[string]Kind{}, db: map[string]Kind{}}
	r.Merge(EmbeddedKinds(), OriginEmbedded)
	return r
}

// lookup resolves a name through the layers, newest first. Caller holds the
// read lock.
func (r *Registry) lookup(name string) (Kind, bool) {
	if k, ok := r.db[name]; ok {
		return k, true
	}
	k, ok := r.base[name]
	return k, ok
}

// SetDBKinds replaces the third tier wholesale.
//
// ⚠️ Wholesale, like i18n.Catalog.SetOverrides, so the caller MUST pass every
// approved row rather than a page — a partial read would silently revoke kinds,
// and a revoked kind makes every theme declaring it fail with "unknown kind",
// which reads as data loss.
//
// A row that will not compile is reported and skipped; the rest still land. The
// alternative — refusing the whole layer — would let one bad row an operator
// approved months ago block an unrelated one they need today.
func (r *Registry) SetDBKinds(kinds []Kind) []error {
	next := map[string]Kind{}
	var problems []error
	for _, k := range kinds {
		compiled, err := compileKind(k, OriginDB)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		next[compiled.Name] = compiled
	}
	r.mu.Lock()
	r.db = next
	r.mu.Unlock()
	return problems
}

// compileKind validates and compiles one kind for one layer.
func compileKind(k Kind, origin string) (Kind, error) {
	if k.Name == "" || k.Pattern == "" {
		return k, fmt.Errorf("kind %q has no name or no pattern", k.Name)
	}
	if isCombinator(k.Name) {
		// Combinators are parsed, not stored. Letting one be redefined here
		// would mean two ways to answer the same question, and the stored one
		// would silently win.
		return k, fmt.Errorf("%q is a combinator and cannot be defined as a kind", k.Name)
	}
	if len(k.Pattern) > MaxStoredPattern {
		return k, fmt.Errorf("kind %q: pattern is %d characters; the limit is %d",
			k.Name, len(k.Pattern), MaxStoredPattern)
	}
	re, err := regexp.Compile(anchor(k.Pattern))
	if err != nil {
		return k, fmt.Errorf("kind %q: %w", k.Name, err)
	}
	k.re = re
	k.Origin = origin
	return k, nil
}

// MaxStoredPattern bounds a pattern that arrived as data.
//
// ⚠️ Go's regexp is RE2, which does not backtrack — so a hostile pattern cannot
// blow up exponentially the way it could on a backtracking engine. That removes
// the catastrophic case, NOT the linear one: RE2 costs O(len(input) × program
// size), and this program runs against every theme value on every write. The
// bound is about that, and about a pattern nobody can read being a pattern
// nobody can review.
const MaxStoredPattern = 512

// Merge adds or replaces kinds from one layer.
//
// ⚠️ An embedded kind can be REDEFINED by a later layer but never REMOVED. An
// operator who breaks `color` with a bad override breaks colour picking; an
// operator who could delete it would make every existing theme unvalidatable
// and every theme write fail with "unknown kind", which reads as data loss.
func (r *Registry) Merge(kinds []Kind, origin string) []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var problems []error
	for _, k := range kinds {
		compiled, err := compileKind(k, origin)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		// ⚠️ Merge writes the BASE layer. The database layer is replaced
		// wholesale by SetDBKinds and never merged into, or a deleted row would
		// survive until the next restart.
		if origin == OriginDB {
			r.db[compiled.Name] = compiled
			continue
		}
		r.base[compiled.Name] = compiled
	}
	return problems
}

// anchor makes a pattern match the WHOLE value.
//
// ⚠️ Without this a pattern like `#[0-9a-f]{6}` would accept
// `#ff0000 anything-at-all`, because Go's MatchString is a SEARCH. Every kind
// pattern here is the shape of a complete value, so anchoring is done centrally
// rather than trusted to whoever wrote the pattern — and a third-party pattern
// is precisely the one that will forget.
//
// ⚠️⚠️ THE GROUP IS NOT COSMETIC. `^` and `$` bind tighter than `|`, so wrapping
// `a|b|c` as `^a|b|c$` anchors only the first and last alternatives — the
// middle ones match ANYWHERE in the value. The first version did exactly that,
// and `ratio` (whose pattern is four alternatives) accepted "1.5", "-0.5" and
// "0.5.5"; `color` accepted "#12345". Found by its own test, which is the only
// way it would have been found: every pattern still matched its own examples.
func anchor(p string) string {
	p = strings.TrimPrefix(p, "^")
	p = strings.TrimSuffix(p, "$")
	return "^(?:" + p + ")$"
}

// CompileCheck reports whether a pattern would compile as a kind — the same
// anchoring the registry applies, so a caller can refuse a pattern before it
// reaches an operator's screen rather than after they approve it.
func CompileCheck(pattern string) (*regexp.Regexp, error) {
	if len(pattern) > MaxStoredPattern {
		return nil, fmt.Errorf("pattern is %d characters; the limit is %d", len(pattern), MaxStoredPattern)
	}
	return regexp.Compile(anchor(pattern))
}

// PlainKindName reports whether a name is a well-formed kind name — a name a
// row could actually be stored under, as opposed to a combinator expression or
// something with punctuation in it.
//
// ⚠️ Narrower than the kind-EXPRESSION charset on purpose. An expression may
// contain `[`, `<`, `,` and spaces because combinators need them; a stored
// name may not, because a stored name that looked like a combinator would be
// unreachable — resolve() parses the combinator and never consults the map.
func PlainKindName(name string) bool {
	return name != "" && len(name) <= MaxKindNameLen && plainKindNameRe.MatchString(name)
}

// MaxKindNameLen keeps a stored name short enough to be an indexed VARCHAR on
// MySQL, and short enough to read on a screen.
const MaxKindNameLen = 64

var plainKindNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// OriginOf reports which layer a known kind came from, or "" if unknown.
//
// ⚠️ Exists so the console can say "this approved row is being shadowed" or
// "this approved row is what is actually in force". A screen that showed only
// the stored rows would describe the table rather than the deployment.
func (r *Registry) OriginOf(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.lookup(name)
	if !ok {
		return ""
	}
	return k.Origin
}

// BaseOrigin reports which layer would define this name if the database layer
// did not — "" when nothing below would.
//
// The console uses it to tell an operator that approving this row REDEFINES
// something, which is a different decision from adding something.
func (r *Registry) BaseOrigin(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.base[name]
	if !ok {
		return ""
	}
	return k.Origin
}

// Names returns every kind, sorted, for the console and for the AI's type list.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.base)+len(r.db))
	seen := map[string]bool{}
	for _, m := range []map[string]Kind{r.db, r.base} {
		for n := range m {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// All returns every kind, sorted by name.
func (r *Registry) All() []Kind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Kind, 0, len(r.base)+len(r.db))
	seen := map[string]bool{}
	// db first, so a kind an operator approved shadows the file or embedded one
	// it redefines — which is what "the last layer wins" means, expressed as a
	// lookup order rather than as an overwrite.
	for _, m := range []map[string]Kind{r.db, r.base} {
		for n, k := range m {
			if !seen[n] {
				seen[n] = true
				out = append(out, k)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Describe renders one kind expression as a shape a person — or a model — can
// follow, e.g. "#rrggbb | rgba(…) | …" for color, or "one of: blur, none".
//
// ⚠️ It reads the REGISTRY, not a phrase written next to the template. A kind
// an operator added to a JSON file has to be described as accurately as a
// built-in one, or the whole "add a kind without a release" story stops at the
// point where somebody tries to use it: the model is told a name it cannot
// guess the shape of, produces something arbitrary, and validation drops it.
//
// Empty for an unknown kind. The caller is describing what it is ABOUT to
// validate against; a confident description of a kind nothing can check would
// be worse than none.
func (r *Registry) Describe(kind string) string {
	kind = strings.TrimSpace(kind)
	switch {
	case strings.HasPrefix(kind, "one-of[") && strings.HasSuffix(kind, "]"):
		items := splitList(kind[len("one-of[") : len(kind)-1])
		if len(items) == 0 {
			return ""
		}
		return "one of: " + strings.Join(items, ", ")
	case strings.HasPrefix(kind, "list-of<") && strings.HasSuffix(kind, ">"):
		inner := r.Describe(kind[len("list-of<") : len(kind)-1])
		if inner == "" {
			return ""
		}
		return fmt.Sprintf("up to %d, space-separated, each: %s", MaxListItems, inner)
	case strings.HasPrefix(kind, "nullable<") && strings.HasSuffix(kind, ">"):
		inner := r.Describe(kind[len("nullable<") : len(kind)-1])
		if inner == "" {
			return ""
		}
		return "empty, or: " + inner
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.lookup(kind)
	if !ok {
		return ""
	}
	if k.Description != "" {
		return k.Description
	}
	// No prose written for it — the pattern itself is the honest answer, and a
	// model reads a regex better than it reads a guess.
	return k.Pattern
}

// Known reports whether a kind expression is one this deployment can validate.
//
// Accepts combinators, so a manifest declaring `list-of<length>` is known even
// though nothing stores that string.
func (r *Registry) Known(kind string) bool { return r.resolve(kind) == nil }

// Validate checks one value against one kind expression.
//
// The character floor runs FIRST and unconditionally, so a malformed kind
// expression can never be a way past it.
func (r *Registry) Validate(kind, value string) error {
	if err := CharacterFloor(value); err != nil {
		return err
	}
	// ⚠️ resolve() FIRST, so a malformed or over-long kind expression cannot
	// validate anything. Without it the bounds live only on the storing path
	// and validate()'s own one-of branch happily matched against a member list
	// nothing had checked — a kind is only a promise if the thing that enforces
	// it agrees the kind is well formed.
	if err := r.resolve(kind); err != nil {
		return err
	}
	return r.validate(kind, strings.TrimSpace(value))
}

func (r *Registry) validate(kind, value string) error {
	switch {
	case strings.HasPrefix(kind, "one-of[") && strings.HasSuffix(kind, "]"):
		// one-of[blur, none] — the literal set. Spelled as a combinator because
		// `enum` needs the allowed values from somewhere, and a manifest saying
		// them inline is more readable than a side channel.
		for _, want := range splitList(kind[len("one-of[") : len(kind)-1]) {
			if value == want {
				return nil
			}
		}
		return fmt.Errorf("%q is not one of %s", value, kind)

	case strings.HasPrefix(kind, "list-of<") && strings.HasSuffix(kind, ">"):
		// "0 2px 8px" — every item validated as the inner kind. Whitespace is
		// the separator because that is what CSS uses for multi-value
		// properties.
		inner := kind[len("list-of<") : len(kind)-1]
		items := strings.Fields(value)
		if len(items) == 0 {
			return fmt.Errorf("a %s needs at least one item", kind)
		}
		if len(items) > MaxListItems {
			return fmt.Errorf("a %s has %d items; the limit is %d", kind, len(items), MaxListItems)
		}
		for _, item := range items {
			// ⚠️ NO per-item floor call here, and that is reasoned rather than
			// forgotten: every item is a SUBSTRING of the value the floor
			// already rejected, and the floor is a substring search. A second
			// call could not fail where the first passed.
			//
			// It was written that way at first ("a separator must not smuggle a
			// character past the whole-value check") and a mutation proved it
			// dead — removing it turned nothing red. Dead defensive code is
			// worse than none: it makes the next reader think the outer call is
			// optional.
			if err := r.validate(inner, item); err != nil {
				return fmt.Errorf("in %s: %w", kind, err)
			}
		}
		return nil

	case strings.HasPrefix(kind, "nullable<") && strings.HasSuffix(kind, ">"):
		// An explicit "do not set this". Distinct from an absent token: absent
		// means the frontend's own default applies, nullable means the theme
		// says to leave it alone.
		if value == "" {
			return nil
		}
		return r.validate(kind[len("nullable<"):len(kind)-1], value)
	}

	r.mu.RLock()
	k, ok := r.lookup(kind)
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown kind %q", kind)
	}
	if !k.re.MatchString(value) {
		return fmt.Errorf("%q is not a valid %s", value, kind)
	}
	return nil
}

// Bounds on a kind EXPRESSION — third-party text that reaches the model.
//
// Generous enough that no honest manifest notices, small enough that the thing
// cannot become a payload: a `one-of` naming more than 64 alternatives is not
// describing a design token, and a single alternative longer than 64 characters
// is not a CSS value.
const (
	MaxKindExpression = 512
	MaxOneOfMembers   = 64
	MaxOneOfMember    = 64
)

// kindExprRe is every character a kind expression may contain: primitive names,
// enum keywords, and the three combinators' punctuation. Nothing else.
var kindExprRe = regexp.MustCompile(`^[A-Za-z0-9 ,._%#\[\]<>+-]+$`)

// clip shortens a string for an error message without splitting a rune.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// MaxListItems bounds a list-of value. Four is a box-shadow; sixteen is
// somebody using a theme variable as a database.
const MaxListItems = 16

// resolve reports whether a kind expression can be validated at all, without a
// value to check. Returns nil when it can.
func (r *Registry) resolve(kind string) error {
	// ⚠️ A kind EXPRESSION is third-party text, and it is the one piece of a
	// manifest that reaches the model without an operator approving it: the
	// token list is rendered into every theme prompt so the model knows what
	// shapes to write. `rules` is gated behind RulesAccepted precisely to keep
	// unapproved frontend text out of there, and an unbounded kind expression
	// walks around that gate — `one-of[a, <newline> ignore everything above]`
	// was accepted, stored, and interpolated verbatim.
	//
	// Bounded HERE rather than only at the handshake because this is the
	// function that decides what "known" means, and every route that ever
	// stores a kind (the handshake today, the DB layer later) asks it.
	if len(kind) > MaxKindExpression {
		return fmt.Errorf("kind expression is %d characters; the limit is %d", len(kind), MaxKindExpression)
	}
	// ⚠️ An allowlist, not the value floor. The floor forbids `<` and `>`, which
	// `list-of<…>` legitimately contains — so a kind expression gets its own
	// charset, and it is deliberately narrower than the floor everywhere else:
	// a kind names primitives and enum keywords, so `(` alone already rules out
	// `url(`, and there is no honest expression that needs a brace, a
	// semicolon, a backslash or a newline.
	if !kindExprRe.MatchString(kind) {
		return fmt.Errorf("kind expression %q contains characters a kind cannot contain", clip(kind, 48))
	}
	switch {
	case strings.HasPrefix(kind, "one-of[") && strings.HasSuffix(kind, "]"):
		members := splitList(kind[len("one-of[") : len(kind)-1])
		if len(members) == 0 {
			return fmt.Errorf("%s lists no values", kind)
		}
		if len(members) > MaxOneOfMembers {
			return fmt.Errorf("%s lists %d values; the limit is %d", kind, len(members), MaxOneOfMembers)
		}
		for _, m := range members {
			if len(m) > MaxOneOfMember {
				return fmt.Errorf("in %s: %q is %d characters; the limit is %d",
					kind, clip(m, 32), len(m), MaxOneOfMember)
			}
		}
		return nil
	case strings.HasPrefix(kind, "list-of<") && strings.HasSuffix(kind, ">"):
		return r.resolve(kind[len("list-of<") : len(kind)-1])
	case strings.HasPrefix(kind, "nullable<") && strings.HasSuffix(kind, ">"):
		return r.resolve(kind[len("nullable<") : len(kind)-1])
	}
	r.mu.RLock()
	_, ok := r.lookup(kind)
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown kind %q", kind)
	}
	return nil
}

func isCombinator(name string) bool {
	return strings.HasPrefix(name, "one-of[") ||
		strings.HasPrefix(name, "list-of<") ||
		strings.HasPrefix(name, "nullable<")
}

func splitList(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
