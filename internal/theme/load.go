package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The file layer: drop a JSON file, get a new kind.
//
// Same shape as LOCALES_DIR, and for the same reason — "adding one is a file,
// not a release" is only true if there is somewhere to put the file. Without
// this layer the three tiers collapse to "the six we compiled in", and the
// third tier (a frontend supplying its own pattern) would have nowhere to land
// after an operator approved it.
//
// ⚠️ The DATABASE layer is not wired yet. It arrives with the console screen
// that approves a pattern, because a stored kind with no way to store one is
// the kind of half-feature this repository has shipped seven times. Registry
// already accepts it; nothing calls Merge with OriginDB in production.

// FileKind is the on-disk shape. Deliberately the same field names as Kind
// minus the compiled regexp, so a file reads like the thing it becomes.
type FileKind struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
}

// LoadDir merges every *.json in dir into the registry.
//
// A MISSING DIRECTORY IS NOT AN ERROR: the default deployment has none, and
// refusing to start over an absent optional directory would make the optional
// thing mandatory. A file that is present and malformed IS reported — somebody
// put it there on purpose, and silently ignoring it would leave them with a
// kind that never works and no explanation.
func (r *Registry) LoadDir(dir string) error {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read theme kind dir %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	// Sorted, so two deployments with the same files get the same result when
	// two files define the same kind. Filesystem order is not an ordering.
	sort.Strings(names)

	var problems []string
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		var kinds []FileKind
		if err := json.Unmarshal(b, &kinds); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		out := make([]Kind, 0, len(kinds))
		for _, k := range kinds {
			out = append(out, Kind{Name: k.Name, Pattern: k.Pattern, Description: k.Description})
		}
		for _, p := range r.Merge(out, OriginFile) {
			problems = append(problems, fmt.Sprintf("%s: %v", name, p))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("theme kinds: %s", strings.Join(problems, "; "))
	}
	return nil
}
