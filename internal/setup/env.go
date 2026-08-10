// Package setup is the interactive backend configuration flow behind
// `daycore install` and `daycore config`.
//
// # Why it is a package and not a file in cmd/daycore
//
// Two commands share it. `install` runs every section once on an empty
// directory; `config` re-runs one section against a deployment that already
// exists. Those are the same questions with different framing, and the moment
// they were two copies the answers would drift — `config db` would ask for
// something `install` no longer writes.
//
// It also makes the flow testable. The version of this code that lived in
// cmd/daycore read os.Stdin directly and was therefore never tested, which is
// how it shipped a command that wrote a directory the server could not start
// from.
//
// # Boundary: this package writes files and asks questions
//
// It does not connect to anything by default. A setup step that needs the
// network cannot configure an air-gapped deployment, and one that dials the
// database turns "I mistyped the DSN" into a failure that happens before there
// is anything able to explain it. Connectivity checks exist but are opt-in and
// always skippable — a check that cannot be skipped is a hard dependency
// wearing a friendly hat.
package setup

import (
	"os"
	"sort"
	"strings"
)

// Env is a .env file being edited.
//
// Ordered, because the generated file is meant to be read and hand-edited: a
// map-ordered dump reshuffles every key on every `daycore config` run and turns
// a one-line change into an unreviewable diff. Sections declare their own
// grouping and the order is the order they were first written.
type Env struct {
	order    []string
	values   map[string]string
	comments map[string]string // header comment printed above a key
	groups   map[string]string // section banner printed above a key
}

func NewEnv() *Env {
	return &Env{values: map[string]string{}, comments: map[string]string{}, groups: map[string]string{}}
}

// LoadEnv reads an existing .env. A missing file is not an error: that is
// exactly the `install` case, and treating it as one would mean two code paths
// for what is one flow.
//
// Unknown keys are preserved. An operator who added HTTP_PROXY by hand must not
// lose it because `daycore config models` rewrote the file — silently dropping
// somebody's edit is worse than any question this package asks.
func LoadEnv(path string) (*Env, error) {
	e := NewEnv()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		e.Set(strings.TrimSpace(k), v)
	}
	return e, nil
}

// Set writes a key, appending it to the order the first time it is seen.
func (e *Env) Set(key, value string) {
	if _, seen := e.values[key]; !seen {
		e.order = append(e.order, key)
	}
	e.values[key] = value
}

// SetIn writes a key under a named group, with an optional comment above it.
func (e *Env) SetIn(group, key, value, comment string) {
	e.Set(key, value)
	e.groups[key] = group
	if comment != "" {
		e.comments[key] = comment
	}
}

// Unset removes a key entirely.
//
// Distinct from Set(key, "") on purpose, and the distinction is load-bearing:
// config.getEnv treats a set-but-empty variable as unset, so `KEY=` in the file
// reads as configured while doing nothing. Anything that means "go back to the
// default" has to remove the line.
func (e *Env) Unset(key string) {
	delete(e.values, key)
	delete(e.comments, key)
	delete(e.groups, key)
	for i, k := range e.order {
		if k == key {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
}

func (e *Env) Get(key string) string   { return e.values[key] }
func (e *Env) Has(key string) bool     { _, ok := e.values[key]; return ok }
func (e *Env) Keys() []string          { out := append([]string(nil), e.order...); return out }
func (e *Env) Len() int                { return len(e.values) }
func (e *Env) Group(key string) string { return e.groups[key] }

// Render produces the file text, grouped and commented.
func (e *Env) Render(header string) string {
	var sb strings.Builder
	if header != "" {
		sb.WriteString("# " + header + "\n")
	}
	lastGroup := "\x00"
	for _, k := range e.order {
		v, ok := e.values[k]
		if !ok {
			continue
		}
		if g := e.groups[k]; g != lastGroup {
			sb.WriteString("\n# ─── " + g + " ───\n")
			lastGroup = g
		}
		if c := e.comments[k]; c != "" {
			for _, line := range strings.Split(c, "\n") {
				sb.WriteString("# " + line + "\n")
			}
		}
		sb.WriteString(k + "=" + v + "\n")
	}
	return sb.String()
}

// Save writes the file at mode 0600.
//
// Not 0644: this file holds JWT_SECRET, COOKIE_SECRET, ADMIN_TOKEN and every
// model provider key. On a shared host, world-readable signing keys mean any
// local user can mint a session for any account.
func (e *Env) Save(path, header string) error {
	return os.WriteFile(path, []byte(e.Render(header)), 0600)
}

// Diff reports which keys changed against another Env, for the summary a
// `daycore config` run prints. Sorted so the output is stable.
func (e *Env) Diff(old *Env) (added, changed, removed []string) {
	for _, k := range e.order {
		switch {
		case !old.Has(k):
			added = append(added, k)
		case old.Get(k) != e.Get(k):
			changed = append(changed, k)
		}
	}
	for _, k := range old.order {
		if !e.Has(k) {
			removed = append(removed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(changed)
	sort.Strings(removed)
	return
}
