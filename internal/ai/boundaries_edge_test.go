package ai

import (
	"strings"
	"testing"
)

const validBoundaryDoc = `{
  "version": 1,
  "locales": {
    "zh-CN": {"heading": "硬边界", "lead": "", "rules": ["规则一"], "closing": "结束"},
    "en-US": {"heading": "Boundaries", "lead": "", "rules": ["rule one"], "closing": "end"}
  }
}`

// Trailing garbage after the JSON document is silently ignored — the decoder
// reads the first value and stops. Locked as the current behaviour.
func TestParseBoundaryDocTrailingGarbageIgnored(t *testing.T) {
	doc, err := parseBoundaryDoc([]byte(validBoundaryDoc+" garbage"), "test")
	if err != nil {
		t.Errorf("trailing garbage is currently accepted, got %v", err)
	}
	if doc == nil || len(doc.Locales) != 2 {
		t.Errorf("doc must still parse, got %+v", doc)
	}
}

// The version field is typed: a string version is refused, and a negative
// one would read as stale rather than breaking anything.
func TestParseBoundaryDocVersionShapes(t *testing.T) {
	bad := strings.Replace(validBoundaryDoc, `"version": 1`, `"version": "1"`, 1)
	if _, err := parseBoundaryDoc([]byte(bad), "test"); err == nil {
		t.Error("a string version must be refused")
	}
	neg := strings.Replace(validBoundaryDoc, `"version": 1`, `"version": -1`, 1)
	doc, err := parseBoundaryDoc([]byte(neg), "test")
	if err != nil || doc.Version != -1 {
		t.Errorf("a negative version parses (and reads as stale later), got (%+v, %v)", doc, err)
	}
}

// _readme must be an array of strings: an operator writing a plain string
// (the likeliest shape for a readme) fails the WHOLE file rather than being
// silently accepted — an accepted readme is the one field whose corruption
// nothing downstream would notice.
func TestParseBoundaryDocReadmeMustBeArray(t *testing.T) {
	withStringReadme := strings.Replace(validBoundaryDoc, `"version": 1,`, `"version": 1, "_readme": "a note",`, 1)
	if _, err := parseBoundaryDoc([]byte(withStringReadme), "test"); err == nil {
		t.Error("a string _readme must be refused")
	}
	withArrayReadme := strings.Replace(validBoundaryDoc, `"version": 1,`, `"version": 1, "_readme": ["a", "b"],`, 1)
	if _, err := parseBoundaryDoc([]byte(withArrayReadme), "test"); err != nil {
		t.Errorf("an array _readme must be accepted, got %v", err)
	}
}

// Two locale keys that canonicalise to the same tag collapse into ONE entry
// without error — which one wins is map-iteration order and therefore NOT
// deterministic, so the pinned property is the collapse itself plus validity
// of both originals.
func TestParseBoundaryDocDuplicateCanonicalLocale(t *testing.T) {
	dup := strings.Replace(validBoundaryDoc, `"en-US": {"heading": "Boundaries"`, `"en_US": {"heading": "first", "lead": "", "rules": ["r"], "closing": "c"}, "en-US": {"heading": "second"`, 1)
	doc, err := parseBoundaryDoc([]byte(dup), "test")
	if err != nil {
		t.Fatalf("duplicate canonical tags are currently accepted, got %v", err)
	}
	if len(doc.Locales) != 2 { // zh-CN + one en entry
		t.Errorf("two canonical duplicates must collapse to one entry, got %d", len(doc.Locales))
	}
	en, ok := doc.Locales["en-US"]
	if !ok {
		t.Fatal("the canonical en-US entry is missing")
	}
	if en.Heading != "first" && en.Heading != "second" {
		t.Errorf("the surviving entry must be one of the originals, got %q", en.Heading)
	}
}

// BoundaryLoad.Stale is the one place a version lag gets spoken aloud.
func TestBoundaryLoadStale(t *testing.T) {
	if (BoundaryLoad{}).Stale() {
		t.Error("no file on disk is not stale")
	}
	if !(BoundaryLoad{Path: "/x", Version: 1, Embedded: 2}).Stale() {
		t.Error("a file older than the binary must read as stale")
	}
	if (BoundaryLoad{Path: "/x", Version: 2, Embedded: 2}).Stale() {
		t.Error("an equal version is not stale")
	}
	if (BoundaryLoad{Path: "/x", Version: 3, Embedded: 2}).Stale() {
		t.Error("a newer file is not stale")
	}
}
