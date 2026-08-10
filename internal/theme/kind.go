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
// Safe for concurrent use: it is read on every theme write and rebuilt when the
// database layer changes.
type Registry struct {
	mu    sync.RWMutex
	kinds map[string]Kind
}

// NewRegistry returns a registry holding only the embedded floor.
func NewRegistry() *Registry {
	r := &Registry{kinds: map[string]Kind{}}
	r.Merge(EmbeddedKinds(), OriginEmbedded)
	return r
}

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
		if k.Name == "" || k.Pattern == "" {
			problems = append(problems, fmt.Errorf("kind %q has no name or no pattern", k.Name))
			continue
		}
		if isCombinator(k.Name) {
			// Combinators are parsed, not stored. Letting one be redefined here
			// would mean two ways to answer the same question, and the stored one
			// would silently win.
			problems = append(problems, fmt.Errorf("%q is a combinator and cannot be defined as a kind", k.Name))
			continue
		}
		re, err := regexp.Compile(anchor(k.Pattern))
		if err != nil {
			problems = append(problems, fmt.Errorf("kind %q: %w", k.Name, err))
			continue
		}
		k.re = re
		k.Origin = origin
		r.kinds[k.Name] = k
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

// Names returns every kind, sorted, for the console and for the AI's type list.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.kinds))
	for n := range r.kinds {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All returns every kind, sorted by name.
func (r *Registry) All() []Kind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Kind, 0, len(r.kinds))
	for _, k := range r.kinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
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
	k, ok := r.kinds[kind]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown kind %q", kind)
	}
	if !k.re.MatchString(value) {
		return fmt.Errorf("%q is not a valid %s", value, kind)
	}
	return nil
}

// MaxListItems bounds a list-of value. Four is a box-shadow; sixteen is
// somebody using a theme variable as a database.
const MaxListItems = 16

// resolve reports whether a kind expression can be validated at all, without a
// value to check. Returns nil when it can.
func (r *Registry) resolve(kind string) error {
	switch {
	case strings.HasPrefix(kind, "one-of[") && strings.HasSuffix(kind, "]"):
		if len(splitList(kind[len("one-of["):len(kind)-1])) == 0 {
			return fmt.Errorf("%s lists no values", kind)
		}
		return nil
	case strings.HasPrefix(kind, "list-of<") && strings.HasSuffix(kind, ">"):
		return r.resolve(kind[len("list-of<") : len(kind)-1])
	case strings.HasPrefix(kind, "nullable<") && strings.HasSuffix(kind, ">"):
		return r.resolve(kind[len("nullable<") : len(kind)-1])
	}
	r.mu.RLock()
	_, ok := r.kinds[kind]
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
