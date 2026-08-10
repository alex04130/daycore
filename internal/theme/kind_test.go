package theme

import (
	"strings"
	"testing"
)

// The character floor holds for EVERY kind, including ones an operator added.
//
// ⚠️ This is the assertion the whole design rests on. The kind set is data, so
// somebody can add a loose pattern — and the guarantee has to survive that. It
// does, because the floor runs before any kind is consulted and again on every
// leaf of a combinator.
//
// The failure it prevents is concrete: a value containing `url(` used in a
// property that resolves it reports every render to a third party, from inside
// a page the user trusts.
func TestTheCharacterFloorHoldsEvenForAWideOpenKind(t *testing.T) {
	r := NewRegistry()
	// The loosest kind anybody could add: anything at all.
	if problems := r.Merge([]Kind{{Name: "anything", Pattern: `.*`}}, OriginDB); len(problems) > 0 {
		t.Fatalf("could not add the kind: %v", problems)
	}

	for _, bad := range []string{
		`url(https://evil.example/beacon.png)`,
		`red; background: url(x)`,
		`}body{display:none`,
		`<script>`,
		`\75 rl(x)`, // a CSS escape reconstructing url(
		"red\nbackground: x",
		`/* */ red`,
	} {
		if err := r.Validate("anything", bad); err == nil {
			t.Errorf("a wide-open kind accepted %q — the floor is not being applied", bad)
		}
	}
	// …and it still accepts an ordinary value, or the floor would be a wall.
	if err := r.Validate("anything", "whatever-goes"); err != nil {
		t.Errorf("the floor rejected a harmless value: %v", err)
	}
}

// A combinator cannot smuggle a forbidden character through its separator.
func TestTheFloorAppliesToEveryItemOfAList(t *testing.T) {
	r := NewRegistry()
	if err := r.Validate("list-of<length>", "0 2px 8px"); err != nil {
		t.Fatalf("a plain shadow-shaped list was rejected: %v", err)
	}
	if err := r.Validate("list-of<length>", "0 2px url(x)"); err == nil {
		t.Error("a list accepted an item containing url(")
	}
	if err := r.Validate("list-of<length>", "0 2px 8px 1px 2px 3px 4px 5px 6px 7px 8px 9px 10px 11px 12px 13px 14px"); err == nil {
		t.Error("a list of seventeen items was accepted; the cap is not enforced")
	}
}

// Patterns match the WHOLE value.
//
// ⚠️ Go's MatchString is a SEARCH. Without anchoring, `#[0-9a-f]{6}` accepts
// "#ff0000 and then anything" — and "anything" is where the injection lives.
// The anchoring is done centrally rather than trusted to whoever wrote the
// pattern, because a third-party pattern is exactly the one that will forget.
func TestPatternsAreAnchoredForTheAuthor(t *testing.T) {
	r := NewRegistry()
	if problems := r.Merge([]Kind{{Name: "sloppy", Pattern: `[0-9]+`}}, OriginDB); len(problems) > 0 {
		t.Fatal(problems)
	}
	if err := r.Validate("sloppy", "123"); err != nil {
		t.Fatalf("an anchored pattern rejected an exact match: %v", err)
	}
	if err := r.Validate("sloppy", "123abc"); err == nil {
		t.Error("an unanchored pattern accepted trailing junk — anchoring is not being applied")
	}
	if err := r.Validate("sloppy", "abc123"); err == nil {
		t.Error("an unanchored pattern accepted leading junk")
	}
}

// The six primitives accept what a theme needs and refuse what it does not.
func TestThePrimitivesAcceptThemesAndRefuseCSS(t *testing.T) {
	r := NewRegistry()
	ok := map[string][]string{
		"color":    {"#fff", "#f472b6", "#ff000080", "rgba(255,255,255,0.72)", "hsl(210 40% 96%)", "transparent", "none"},
		"length":   {"12px", "1.5rem", "0", "100%", "-4px", "50vh"},
		"number":   {"1.6", "-2", "0"},
		"ratio":    {"0", "1", "0.75", "1.0"},
		"duration": {"200ms", "0.3s"},
		"enum":     {"blur", "solid-2"},
	}
	for kind, values := range ok {
		for _, v := range values {
			if err := r.Validate(kind, v); err != nil {
				t.Errorf("%s rejected %q: %v", kind, v, err)
			}
		}
	}
	bad := map[string][]string{
		// ⚠️ currentColor / inherit resolve against context, so the same theme
		// would render differently depending on where it landed — the one thing
		// a theme must not do.
		"color":    {"currentColor", "inherit", "red", "var(--x)", "#12345"},
		"length":   {"12", "12pt", "calc(1px + 2px)", "1e3px"},
		"number":   {"1px", "abc"},
		"ratio":    {"1.5", "2", "-0.5", "0.5.5"},
		"duration": {"200", "200 ms", "1min"},
		"enum":     {"9lives", "has space"},
	}
	for kind, values := range bad {
		for _, v := range values {
			if err := r.Validate(kind, v); err == nil {
				t.Errorf("%s accepted %q", kind, v)
			}
		}
	}
}

// Combinators express nothing a primitive cannot, which is why they need no
// approval — every leaf still goes through a primitive's validator.
func TestCombinatorsBottomOutInPrimitives(t *testing.T) {
	r := NewRegistry()
	cases := []struct {
		kind, value string
		want        bool
	}{
		{"one-of[blur, none, solid]", "blur", true},
		{"one-of[blur, none, solid]", "frosted", false},
		{"one-of[]", "anything", false},
		{"nullable<color>", "", true},
		{"nullable<color>", "#fff", true},
		{"nullable<color>", "not-a-color", false},
		{"list-of<color>", "#fff #000", true},
		{"list-of<color>", "#fff notacolor", false},
		{"nullable<list-of<length>>", "", true},
		{"nullable<list-of<length>>", "0 2px", true},
		{"list-of<one-of[a,b]>", "a b a", true},
		{"list-of<one-of[a,b]>", "a c", false},
		{"list-of<nonsense>", "x", false},
	}
	for _, c := range cases {
		err := r.Validate(c.kind, c.value)
		if (err == nil) != c.want {
			t.Errorf("%s(%q) = %v, want ok=%v", c.kind, c.value, err, c.want)
		}
	}
}

// An unknown kind is refused rather than waved through.
//
// ⚠️ The direction matters: a manifest declaring a kind this deployment has
// never heard of must not have its values accepted unchecked. Refusing means a
// frontend using a new kind sees a clear error and an operator adds it — a row,
// not a release.
func TestAnUnknownKindIsRefused(t *testing.T) {
	r := NewRegistry()
	if err := r.Validate("clamp", "clamp(1rem,2vw,2rem)"); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
	if r.Known("clamp") {
		t.Error("Known said yes to a kind that is not defined")
	}
	// …and adding it is a row.
	r.Merge([]Kind{{Name: "clamp", Pattern: `clamp\([0-9a-z.,%\s]+\)`, Description: "clamp()"}}, OriginDB)
	if err := r.Validate("clamp", "clamp(1rem,2vw,2rem)"); err != nil {
		t.Errorf("the newly added kind still rejects its own example: %v", err)
	}
	if !r.Known("clamp") {
		t.Error("Known still says no after the kind was added")
	}
}

// A later layer may redefine a primitive but the primitive never disappears.
//
// A deployment whose `color` vanished — a bad file, an emptied table — would
// reject every theme write with "unknown kind", which reads exactly like data
// loss. So the embedded set is a floor, not a default.
func TestTheEmbeddedFloorCannotBeRemoved(t *testing.T) {
	r := NewRegistry()
	// The file layer says nothing about color; it must still be there.
	r.Merge([]Kind{{Name: "length", Pattern: `[0-9]+px`}}, OriginFile)
	if !r.Known("color") {
		t.Fatal("merging a partial layer removed a primitive")
	}
	if err := r.Validate("color", "#fff"); err != nil {
		t.Errorf("color stopped working after an unrelated merge: %v", err)
	}
	// Redefining is allowed, and the last layer wins — so the NARROWER
	// definition is now in force.
	if err := r.Validate("length", "12px"); err != nil {
		t.Errorf("the redefinition rejects its own shape: %v", err)
	}
	if err := r.Validate("length", "1.5rem"); err == nil {
		t.Error("the redefinition did not take effect; the embedded pattern is still in force")
	}
	for _, k := range r.All() {
		if k.Name == "length" && k.Origin != OriginFile {
			t.Errorf("length reports origin %q after a file-layer redefinition", k.Origin)
		}
	}
}

// A combinator name cannot be stored as a kind.
//
// Two ways to answer the same question is one way too many, and the stored one
// would silently win — so a manifest writing `list-of<length>` would suddenly
// mean something an operator typed rather than what the protocol says.
func TestACombinatorCannotBeRedefinedAsAKind(t *testing.T) {
	r := NewRegistry()
	problems := r.Merge([]Kind{{Name: "list-of<length>", Pattern: `.*`}}, OriginDB)
	if len(problems) == 0 {
		t.Fatal("a combinator was accepted as a stored kind")
	}
	if !strings.Contains(problems[0].Error(), "combinator") {
		t.Errorf("the refusal does not say why: %v", problems[0])
	}
	// And the combinator still behaves as the protocol says.
	if err := r.Validate("list-of<length>", "1px url(x)"); err == nil {
		t.Error("the combinator was replaced by the wide-open definition anyway")
	}
}

// A kind with an uncompilable pattern is reported, not silently dropped or
// silently accepted.
func TestABadPatternIsReportedAndTheRegistryStaysUsable(t *testing.T) {
	r := NewRegistry()
	problems := r.Merge([]Kind{
		{Name: "broken", Pattern: `([a-z`},
		{Name: "fine", Pattern: `[a-z]+`},
	}, OriginFile)
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want exactly the broken one: %v", len(problems), problems)
	}
	if r.Known("broken") {
		t.Error("a kind with an uncompilable pattern was registered")
	}
	if !r.Known("fine") {
		t.Error("one bad kind took a good one down with it")
	}
}
