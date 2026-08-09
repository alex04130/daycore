package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"daycore/internal/i18n"
	"daycore/internal/resources"
)

// BoundaryFile is the name the loader looks for under PROMPTS_DIR.
const BoundaryFile = "boundaries.json"

// ─── L1 hard boundaries ─────────────────────────────────────────────────────
//
// The L1 block is restated AFTER the L2 persona in every assembled system
// prompt, so that a persona saying "ignore previous instructions" cannot take
// the boundaries with it.
//
// It has exactly one editing surface: a JSON file on disk. That is a deliberate
// asymmetry with every other piece of prompt text in this repo:
//
//	prompt templates   DB (console) → PROMPTS_DIR/<locale>/<key>.tmpl → embedded
//	message catalog    DB (console) → LOCALES_DIR/<locale>.json       → embedded
//	hard boundaries                   PROMPTS_DIR/boundaries.json     → embedded
//
// There is no database layer here on purpose. A boundary that the console can
// rewrite is a boundary that anyone who reaches the console can delete, and the
// deletion would take effect on the next request rather than the next release.
// Requiring a file edit plus a restart puts the change in front of whoever
// operates the process, which is the audience that should be making it.
//
// This used to be a Go literal with a switch on the locale prefix, which bought
// the same immutability at two costs that turned out to be unnecessary: it was
// untranslatable, and adding or removing a single rule meant a release. Both are
// real — boundaries do get tuned, and tuning them is exactly the kind of edit
// that deserves a diff rather than a recompile.

// boundarySet is one locale's block: a heading, a lead-in line, the rules, and
// the sentence that resolves conflicts in the boundaries' favour.
type boundarySet struct {
	Heading string   `json:"heading"`
	Lead    string   `json:"lead"`
	Rules   []string `json:"rules"`
	Closing string   `json:"closing"`
}

// boundaryDoc is the on-disk shape. _readme is carried so that the file can
// document itself to the operator holding it; nothing reads it.
type boundaryDoc struct {
	Readme  []string               `json:"_readme"`
	Version int                    `json:"version"`
	Locales map[string]boundarySet `json:"locales"`
}

// Boundaries holds the L1 block per locale.
type Boundaries struct {
	mu       sync.RWMutex
	version  int
	embedVer int
	sets     map[string]boundarySet // canonical locale → block
}

// NewBoundaries parses the embedded boundaries file. It fails when either
// compiled-in locale is missing, the same double-locale rule prompt templates
// are held to and for a stronger reason: a locale with no boundary block would
// serve a persona with nothing after it.
func NewBoundaries() (*Boundaries, error) {
	raw, err := resources.Read("prompts/" + BoundaryFile)
	if err != nil {
		return nil, fmt.Errorf("read embedded %s: %w", BoundaryFile, err)
	}
	doc, err := parseBoundaryDoc(raw, "embedded "+BoundaryFile)
	if err != nil {
		return nil, err
	}
	b := &Boundaries{version: doc.Version, embedVer: doc.Version, sets: doc.Locales}
	for _, locale := range i18n.Embedded {
		if _, ok := b.sets[locale]; !ok {
			return nil, fmt.Errorf("embedded %s is missing locale %q", BoundaryFile, locale)
		}
	}
	return b, nil
}

// parseBoundaryDoc decodes and validates one boundaries document.
//
// Unknown fields are rejected. That looks fussy until you consider the failure
// it prevents: "rules" misspelled decodes cleanly into zero rules, and a block
// with no rules is precisely the outcome this file exists to make impossible.
// The same reasoning drives the emptiness checks below — a blank heading or an
// empty rule list is far more likely to be a half-written file than someone
// deciding to have no boundaries. Whoever genuinely wants fewer rules deletes
// rules; getting to zero requires saying so somewhere else.
func parseBoundaryDoc(raw []byte, source string) (*boundaryDoc, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc boundaryDoc
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", source, err)
	}
	if len(doc.Locales) == 0 {
		return nil, fmt.Errorf("parse %s: no locales", source)
	}
	out := make(map[string]boundarySet, len(doc.Locales))
	for tag, set := range doc.Locales {
		locale := i18n.Canonical(tag)
		if locale == "" {
			return nil, fmt.Errorf("parse %s: %q is not a language tag", source, tag)
		}
		if strings.TrimSpace(set.Heading) == "" {
			return nil, fmt.Errorf("parse %s: locale %s has no heading", source, locale)
		}
		if strings.TrimSpace(set.Closing) == "" {
			return nil, fmt.Errorf("parse %s: locale %s has no closing line", source, locale)
		}
		if len(set.Rules) == 0 {
			return nil, fmt.Errorf("parse %s: locale %s has no rules", source, locale)
		}
		for i, rule := range set.Rules {
			if strings.TrimSpace(rule) == "" {
				return nil, fmt.Errorf("parse %s: locale %s rule %d is empty", source, locale, i+1)
			}
		}
		out[locale] = set
	}
	doc.Locales = out
	return &doc, nil
}

// BoundaryLoad reports what a disk load did, so the caller can log it. An
// operator who edits this file should see evidence at boot that the edit landed.
type BoundaryLoad struct {
	Path     string   // file that was read ("" when none)
	Locales  []string // locales replaced, sorted
	Version  int      // version declared by the file
	Embedded int      // version compiled into this binary
}

// Stale reports that the file on disk predates the boundaries shipped with this
// binary, i.e. rules added upstream are being masked by the operator's copy.
// This is a warning rather than an error: their copy is authoritative by design,
// they just deserve to be told what it is holding back.
func (l BoundaryLoad) Stale() bool { return l.Path != "" && l.Version < l.Embedded }

// LoadDir overlays dir/boundaries.json onto the embedded blocks, locale by
// locale — the same per-item overlay the prompt templates and the message
// catalog use, and for the same reason: an all-or-nothing file would make
// "adjust one rule in Chinese" cost the maintenance of every other locale.
//
// A missing file is not an error. A present-but-broken one is: the operator put
// it there deliberately, and running the embedded boundaries while ignoring
// their file would be the quietest possible way to disregard a security edit.
func (b *Boundaries) LoadDir(dir string) (BoundaryLoad, error) {
	if dir == "" {
		return BoundaryLoad{Embedded: b.embedVer}, nil
	}
	path := filepath.Join(dir, BoundaryFile)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return BoundaryLoad{Embedded: b.embedVer}, nil
	}
	if err != nil {
		return BoundaryLoad{Embedded: b.embedVer}, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := parseBoundaryDoc(raw, path)
	if err != nil {
		return BoundaryLoad{Embedded: b.embedVer}, err
	}
	load := BoundaryLoad{Path: path, Version: doc.Version, Embedded: b.embedVer}
	b.mu.Lock()
	for locale, set := range doc.Locales {
		b.sets[locale] = set
		load.Locales = append(load.Locales, locale)
	}
	b.version = doc.Version
	b.mu.Unlock()
	sort.Strings(load.Locales)
	return load, nil
}

// Reminder renders the L1 block for a locale. An unknown locale falls back to
// another region of the same language, then to i18n.Default — the reader is a
// model, so answering in a language it was not asked in is a far better outcome
// than answering with no boundaries at all.
func (b *Boundaries) Reminder(locale string) string {
	b.mu.RLock()
	set := b.lookup(locale)
	b.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(set.Heading)
	sb.WriteString("\n\n")
	if strings.TrimSpace(set.Lead) != "" {
		sb.WriteString(set.Lead)
		sb.WriteString("\n")
	}
	for _, rule := range set.Rules {
		sb.WriteString("- ")
		sb.WriteString(rule)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(set.Closing)
	return sb.String()
}

// Locales lists the locales that have a boundary block, sorted.
func (b *Boundaries) Locales() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.sets))
	for locale := range b.sets {
		out = append(out, locale)
	}
	sort.Strings(out)
	return out
}

// lookup resolves against the blocks this file actually carries, not against
// the installed message catalog: a language can be installed as a translation
// file without anyone having written boundaries for it, and that must resolve
// to another language's boundaries rather than to nothing.
func (b *Boundaries) lookup(locale string) boundarySet {
	if c := i18n.Canonical(locale); c != "" {
		if set, ok := b.sets[c]; ok {
			return set
		}
		base := baseLocale(c)
		for _, l := range sortedKeys(b.sets) {
			if baseLocale(l) == base {
				return b.sets[l]
			}
		}
	}
	if set, ok := b.sets[i18n.Default]; ok {
		return set
	}
	// Unreachable: NewBoundaries requires both embedded locales and LoadDir only
	// ever replaces blocks, never removes them.
	for _, l := range sortedKeys(b.sets) {
		return b.sets[l]
	}
	return boundarySet{}
}

func sortedKeys(m map[string]boundarySet) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func baseLocale(l string) string {
	if i := strings.IndexByte(l, '-'); i > 0 {
		return strings.ToLower(l[:i])
	}
	return strings.ToLower(l)
}

// ─── process-wide instance ──────────────────────────────────────────────────

var (
	stdBoundariesOnce sync.Once
	stdBoundaries     *Boundaries
	stdBoundariesErr  error
)

// StdBoundaries returns the process-wide boundaries, parsed from the embedded
// file on first use. main loads any disk overlay onto this instance.
func StdBoundaries() (*Boundaries, error) {
	stdBoundariesOnce.Do(func() { stdBoundaries, stdBoundariesErr = NewBoundaries() })
	return stdBoundaries, stdBoundariesErr
}

// HardBoundaryReminder returns the L1 restatement block that sits AFTER L2 in
// the assembled system prompt.
//
// It panics if the embedded boundaries cannot be parsed. That case is caught by
// a test and by startup — main resolves StdBoundaries before serving — so the
// panic is unreachable in a running server. The alternative, returning an empty
// string, would let a process serve requests with no boundaries at all and no
// sign of it, which is the one failure mode worth crashing over.
func HardBoundaryReminder(locale string) string {
	b, err := StdBoundaries()
	if err != nil {
		panic("ai: hard boundaries unavailable: " + err.Error())
	}
	return b.Reminder(locale)
}
