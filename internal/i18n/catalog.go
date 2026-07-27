package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Catalog resolves a message key to text, across three layers.
//
//	database   operator edits from the console; shared by every instance
//	files      a directory of <locale>.json dropped next to the binary
//	embedded   Go literals compiled in — zh-CN and en-US only
//
// The layering is the same shape as prompt_overrides, which is the one place
// this repo already got configuration right: a file seeds it, the database
// overrides it, changes take effect without a restart.
//
// Embedded is a floor, not the whole set. Adding a language must not require a
// rebuild — a translator drops ja-JP.json into the locales directory, or an
// operator pastes it into the console, and it is available. What is compiled in
// is only what has to work when there is no database and no files: enough to
// render a login page and say why nothing else loaded.
//
// Resolution runs per locale, not per layer: for each candidate locale in the
// fallback chain, the database is asked first, then files, then embedded. The
// alternative — pick the highest layer that knows the key at all, then run the
// chain inside it — would let a half-finished French override in the database
// hide a complete English translation underneath.
type Catalog struct {
	mu       sync.RWMutex
	embedded map[string]Text
	files    map[string]Text
	db       map[string]Text
}

// NewCatalog returns an empty catalog. Most code wants the package-level one
// (T, Register); this exists for tests, which must not scribble on a global.
func NewCatalog() *Catalog {
	return &Catalog{embedded: map[string]Text{}, files: map[string]Text{}, db: map[string]Text{}}
}

var std = NewCatalog()

// Std is the process-wide catalog.
func Std() *Catalog { return std }

// Reg registers embedded text and returns the key, so a package can declare
// both in one line next to the code that uses it:
//
//	var keyWrapUp = i18n.Reg("agent.wrapUpNudge", i18n.Text{...})
func Reg(key string, t Text) string { Register(key, t); return key }

// Register adds embedded text for a key. Packages call it from init, next to
// the code that uses the string, so a message never drifts away from its
// caller.
//
// Registering the same key twice panics: two packages silently fighting over
// one key produces text that changes with link order, which is close to
// impossible to debug from a bug report that just says "wrong words".
func Register(key string, t Text) { std.Register(key, t) }

// Register adds embedded text for a key.
func (c *Catalog) Register(key string, t Text) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, dup := c.embedded[key]; dup {
		panic(fmt.Sprintf("i18n: duplicate message key %q", key))
	}
	c.embedded[key] = t
}

// T resolves a key for a locale against the process catalog.
//
// An unregistered key returns the key itself. A missing message must be
// obviously missing on screen — returning "" produces a blank button that looks
// like a layout bug, and inventing text is worse.
func T(key, locale string) string { return std.T(key, locale) }

// T resolves a key for a locale.
func (c *Catalog) T(key, locale string) string {
	if v := Pick(c.Lookup(key), locale); v != "" {
		return v
	}
	return key
}

// Tf resolves a key whose text is a format string. It exists to mark the call
// sites where a translation carries verbs, so they can be found and checked.
func Tf(key, locale string, args ...any) string { return std.Tf(key, locale, args...) }

// Tf resolves a format-string key and applies it.
func (c *Catalog) Tf(key, locale string, args ...any) string {
	return fmt.Sprintf(c.T(key, locale), args...)
}

// Lookup returns the merged Text for a key: database over files over embedded,
// per locale. The result is a fresh map the caller may keep.
func (c *Catalog) Lookup(key string) Text {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := Text{}
	for _, layer := range []map[string]Text{c.embedded, c.files, c.db} {
		for l, v := range layer[key] {
			if v != "" {
				out[l] = v
			}
		}
	}
	return out
}

// LoadDir reads <dir>/<locale>.json, each a flat {"key": "text"} object, and
// replaces the file layer. A missing directory is not an error — running
// without one is the normal case.
//
// Replacing rather than merging means deleting a file actually removes its
// text on the next reload, instead of leaving a translation that no file
// accounts for.
func (c *Catalog) LoadDir(dir string) error {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("i18n: read %s: %w", dir, err)
	}
	next := map[string]Text{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(e.Name(), ".json")
		if Canonical(locale) == "" {
			return fmt.Errorf("i18n: %s is not a usable locale tag", e.Name())
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("i18n: read %s: %w", e.Name(), err)
		}
		var pack map[string]string
		if err := json.Unmarshal(raw, &pack); err != nil {
			return fmt.Errorf("i18n: %s: %w", e.Name(), err)
		}
		for k, v := range pack {
			if v == "" {
				continue
			}
			if next[k] == nil {
				next[k] = Text{}
			}
			next[k][Canonical(locale)] = v
		}
	}
	c.mu.Lock()
	c.files = next
	c.mu.Unlock()
	return nil
}

// SetOverrides replaces the database layer. The store owns the table; this
// package only holds what it was handed, so nothing here needs a database
// connection and the whole catalog stays testable without one.
func (c *Catalog) SetOverrides(m map[string]Text) {
	next := map[string]Text{}
	for k, t := range m {
		cp := Text{}
		for l, v := range t {
			if v != "" {
				cp[Canonical(l)] = v
			}
		}
		if len(cp) > 0 {
			next[k] = cp
		}
	}
	c.mu.Lock()
	c.db = next
	c.mu.Unlock()
}

// Available lists every locale this installation can render, across all three
// layers, sorted. This — not the embedded pair — is what a user picks their two
// languages from.
func (c *Catalog) Available() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Embedded is always in, even before any package has registered anything:
	// the binary can render those two by construction, and a bootstrap window
	// where Available() is empty would make Normalize reject every tag.
	seen := map[string]bool{}
	for _, l := range Embedded {
		seen[l] = true
	}
	for _, layer := range []map[string]Text{c.embedded, c.files, c.db} {
		for _, t := range layer {
			for l, v := range t {
				if v != "" {
					seen[l] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// Available lists the process catalog's locales.
func Available() []string { return std.Available() }

// Coverage reports how many of the catalog's keys a locale has text for. The
// console shows it next to each installed language: "ja-JP — 812/900" is the
// difference between a usable translation and one that will show raw keys.
func (c *Catalog) Coverage(locale string) (have, total int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := map[string]bool{}
	for _, layer := range []map[string]Text{c.embedded, c.files, c.db} {
		for k := range layer {
			keys[k] = true
		}
	}
	total = len(keys)
	for k := range keys {
		for _, layer := range []map[string]Text{c.db, c.files, c.embedded} {
			if layer[k][locale] != "" {
				have++
				break
			}
		}
	}
	return have, total
}

// Export dumps every key with its currently-resolved text for a locale, as the
// same flat shape LoadDir reads. It is how a translator starts: export en-US,
// translate the values, save as <locale>.json.
//
// Keys with no text in that locale are still present, holding whatever the
// fallback chain produced, so the file is a complete worklist rather than a
// sparse one.
func (c *Catalog) Export(locale string) map[string]string {
	c.mu.RLock()
	keys := make([]string, 0, len(c.embedded))
	seen := map[string]bool{}
	for _, layer := range []map[string]Text{c.embedded, c.files, c.db} {
		for k := range layer {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	c.mu.RUnlock()

	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = c.T(k, locale)
	}
	return out
}

// Keys lists every registered key, sorted.
func (c *Catalog) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	seen := map[string]bool{}
	for _, layer := range []map[string]Text{c.embedded, c.files, c.db} {
		for k := range layer {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
