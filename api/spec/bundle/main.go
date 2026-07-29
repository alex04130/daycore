// Command bundle assembles api/openapi.yaml from the shards in api/spec/.
//
// The spec was one 925-line file that twenty-two work items all had to append
// to — the same contention that made server.go the worst file in the repo, with
// the added cruelty that a YAML merge conflict inside a nested mapping is
// resolvable only by someone who can hold the whole document in their head.
//
// So the paths live one file per tag, and this rebuilds the single file that
// downstream generators read. The bundled path does not change, so nothing that
// consumes the contract has to know any of this happened.
//
// Concatenation, not re-serialisation. Round-tripping through a YAML library
// would drop all 77 comments and rewrite every line's formatting, making the
// generated file both worse to read and impossible to review — the diff would
// be the whole file every time. Text assembly keeps the shards' bytes exactly
// as their authors wrote them.
//
//	go run ./api/spec/bundle           rebuild api/openapi.yaml
//	go run ./api/spec/bundle -check    verify it is current and the version rule holds
//	go run ./api/spec/bundle -lock     freeze the current surface at the current version
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"daycore/internal/version"
)

// Root is the repository-relative location of the spec. Overridable so the test
// can run from its own directory.
var Root = "api"

func main() {
	check := flag.Bool("check", false, "verify api/openapi.yaml matches the shards instead of writing it")
	lock := flag.Bool("lock", false, "rewrite contract-lock.json to the current surface and version (do this when freezing a contract version)")
	root := flag.String("root", Root, "directory holding openapi.yaml and spec/")
	flag.Parse()

	want, err := Bundle(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bundle:", err)
		os.Exit(1)
	}
	out := filepath.Join(*root, "openapi.yaml")

	if *lock {
		if err := WriteLock(*root, want); err != nil {
			fmt.Fprintln(os.Stderr, "bundle:", err)
			os.Exit(1)
		}
		fmt.Printf("locked contract v%d.%d with %d operations\n", version.APIVersion, version.APIMinor, len(mustSurface(want)))
		return
	}

	if *check {
		got, err := os.ReadFile(out)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bundle:", err)
			os.Exit(1)
		}
		if !bytes.Equal(got, want) {
			fmt.Fprintf(os.Stderr, "%s is stale — run `make api-bundle`\n", out)
			os.Exit(1)
		}
		lock, err := ReadLock(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bundle:", err)
			os.Exit(1)
		}
		if err := CheckVersion(lock, version.APIVersion, version.APIMinor, mustSurface(want)); err != nil {
			fmt.Fprintln(os.Stderr, "bundle:", err)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(out, want, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "bundle:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", out)
}

const banner = `# ─── GENERATED FILE — do not edit ─────────────────────────────────────────────
# Assembled from api/spec/head.yaml + api/spec/paths/<tag>.yaml + api/spec/components.yaml
# by ` + "`make api-bundle`" + `. Edit the shard for the tag you are changing; a path's
# shard is named after its tag, and the bundler enforces that. ` + "`go test ./...`" + ` fails
# when this file is stale, so a forgotten rebuild cannot reach CI green.
`

// Bundle assembles the spec and returns the bytes api/openapi.yaml should hold.
//
// It also enforces the two invariants that keep the sharding honest, because a
// build step that silently tolerates a misfiled path stops being a structure
// and becomes a convention:
//
//   - every shard file corresponds to a tag declared in head.yaml, and every
//     declared tag has a shard. The declared list is the table of contents and
//     the concatenation order — an undeclared shard would be silently dropped
//     from the output, which is the worst possible failure (the paths are in
//     git, the contract does not have them, and nothing complains).
//   - every operation inside paths/<tag>.yaml is tagged <tag>. Without this the
//     filename means nothing after the first person guesses wrong, and "which
//     file do I add my path to" is a judgement call again.
func Bundle(root string) ([]byte, error) {
	specDir := filepath.Join(root, "spec")

	headRaw, err := os.ReadFile(filepath.Join(specDir, "head.yaml"))
	if err != nil {
		return nil, err
	}
	componentsRaw, err := os.ReadFile(filepath.Join(specDir, "components.yaml"))
	if err != nil {
		return nil, err
	}

	declared, err := declaredTags(headRaw)
	if err != nil {
		return nil, err
	}

	shardFiles, err := filepath.Glob(filepath.Join(specDir, "paths", "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(shardFiles)

	shards := map[string][]byte{}
	for _, f := range shardFiles {
		tag := strings.TrimSuffix(filepath.Base(f), ".yaml")
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if err := checkShardTags(f, tag, b); err != nil {
			return nil, err
		}
		shards[tag] = b
	}

	if err := checkTagSets(declared, shards); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(banner)
	out.WriteString("\n")
	out.WriteString(strings.TrimRight(string(headRaw), "\n"))
	out.WriteString("\n\npaths:\n")
	for _, tag := range declared {
		out.WriteString(sectionHeader(tag))
		out.WriteString(indent(shards[tag]))
		out.WriteString("\n")
	}
	out.WriteString(strings.TrimRight(string(componentsRaw), "\n"))
	out.WriteString("\n")
	return out.Bytes(), nil
}

// declaredTags returns the tag names from head.yaml, in declared order. That
// order is meaningful — it is the order Swagger UI groups by, and the order the
// bundle reads in — so it is not derived from the filenames (which would sort
// admin first and bury session, the entry point, in the middle).
func declaredTags(head []byte) ([]string, error) {
	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
	}
	if err := yaml.Unmarshal(head, &doc); err != nil {
		return nil, fmt.Errorf("head.yaml: %w", err)
	}
	if len(doc.Tags) == 0 {
		return nil, fmt.Errorf("head.yaml declares no tags")
	}
	out := make([]string, 0, len(doc.Tags))
	for _, t := range doc.Tags {
		if t.Name == "" {
			return nil, fmt.Errorf("head.yaml has a tag with no name")
		}
		out = append(out, t.Name)
	}
	return out, nil
}

var httpVerbs = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true,
	"delete": true, "head": true, "options": true, "trace": true,
}

func checkShardTags(file, tag string, shard []byte) error {
	var paths map[string]map[string]struct {
		Tags []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal(shard, &paths); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("%s: no paths — delete the file or delete the tag from head.yaml", file)
	}
	for path, ops := range paths {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("%s: %q is not a path (shards hold path items at the top level, un-indented)", file, path)
		}
		for verb, op := range ops {
			if !httpVerbs[strings.ToLower(verb)] {
				continue // parameters, summary, servers, …
			}
			if len(op.Tags) != 1 || op.Tags[0] != tag {
				return fmt.Errorf("%s: %s %s is tagged %v but lives in the %q shard — one tag per operation, and it must match the filename",
					file, strings.ToUpper(verb), path, op.Tags, tag)
			}
		}
	}
	return nil
}

func checkTagSets(declared []string, shards map[string][]byte) error {
	declaredSet := map[string]bool{}
	for _, t := range declared {
		declaredSet[t] = true
		if _, ok := shards[t]; !ok {
			return fmt.Errorf("head.yaml declares tag %q but api/spec/paths/%s.yaml does not exist", t, t)
		}
	}
	var undeclared []string
	for t := range shards {
		if !declaredSet[t] {
			undeclared = append(undeclared, t)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return fmt.Errorf("api/spec/paths/%v.yaml exist but head.yaml does not declare those tags — they would be silently left out of the bundle", undeclared)
	}
	return nil
}

// sectionHeader draws the same box-rule comment the single file used, so the
// bundled output still reads as a document with chapters.
func sectionHeader(tag string) string {
	const width = 78
	prefix := "  # ─── " + tag + " "
	// Rune count, not byte count: ─ is three bytes.
	pad := width - len([]rune(prefix))
	if pad < 3 {
		pad = 3
	}
	return prefix + strings.Repeat("─", pad) + "\n"
}

// indent shifts a shard under `paths:`. Uniform, so relative indentation — and
// therefore any block scalar inside — survives untouched. Blank lines stay
// blank rather than becoming two spaces of trailing whitespace.
func indent(b []byte) string {
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// ─── contract lock ───────────────────────────────────────────────────────────
//
// The plan has five separate work items each planning to bump APIMinor, which
// would produce three different "1.1"s and no way to tell which one a client
// meant. The rule is "one bump per batch, at the end" — but a rule written in
// prose is a rule nobody can be shown to have broken.
//
// So the lock records the contract surface as of the last freeze, and the test
// asserts the version moved *at least once* since then. That is exactly the
// batch rule: the first additive change in a batch bumps APIMinor, every later
// change in the same batch is already covered, and the lock is refreshed when
// the contract is frozen. A second bump is never required, so the 1.1/1.2/1.3
// collision cannot happen.

// Lock is api/spec/contract-lock.json.
type Lock struct {
	APIVersion int      `json:"apiVersion"`
	APIMinor   int      `json:"apiMinor"`
	Note       string   `json:"note"`
	Operations []string `json:"operations"`
}

const lockNote = "The contract surface as of the last freeze. Refresh with `make api-lock` when bumping " +
	"the contract version, never to silence a test. See api/spec/README.md."

// Surface is the set of operations a generated client can see, one line each.
//
// operationId is part of it because renaming one breaks every generated client
// exactly as removing it would — the method+path still answers, and the client's
// function is gone.
func Surface(bundled []byte) ([]string, error) {
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(bundled, &doc); err != nil {
		return nil, err
	}
	var out []string
	for path, ops := range doc.Paths {
		for verb, op := range ops {
			if !httpVerbs[strings.ToLower(verb)] {
				continue
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("%s %s has no operationId — generated clients name their functions after it", strings.ToUpper(verb), path)
			}
			out = append(out, fmt.Sprintf("%s %s %s", strings.ToUpper(verb), path, op.OperationID))
		}
	}
	sort.Strings(out)
	return out, nil
}

func mustSurface(bundled []byte) []string {
	s, err := Surface(bundled)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bundle:", err)
		os.Exit(1)
	}
	return s
}

func LockPath(root string) string { return filepath.Join(root, "spec", "contract-lock.json") }

func ReadLock(root string) (Lock, error) {
	var l Lock
	b, err := os.ReadFile(LockPath(root))
	if err != nil {
		return l, err
	}
	err = json.Unmarshal(b, &l)
	return l, err
}

func WriteLock(root string, bundled []byte) error {
	ops, err := Surface(bundled)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(Lock{
		APIVersion: version.APIVersion,
		APIMinor:   version.APIMinor,
		Note:       lockNote,
		Operations: ops,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(LockPath(root), append(b, '\n'), 0o644)
}

// CheckVersion applies the bump rule. It lives in the tool rather than only in
// the test so `-check` enforces it too — the rule is part of what the contract
// is, not a property someone remembered to test.
func CheckVersion(lock Lock, apiVersion, apiMinor int, current []string) error {
	added, removed := diffOps(lock.Operations, current)

	// A version that went backwards is always a mistake — usually a bad merge of
	// internal/version/version.go, which nothing else would notice.
	if apiVersion < lock.APIVersion || (apiVersion == lock.APIVersion && apiMinor < lock.APIMinor) {
		return fmt.Errorf("contract version went backwards: the lock says %d.%d, version.go says %d.%d",
			lock.APIVersion, lock.APIMinor, apiVersion, apiMinor)
	}

	switch {
	case len(removed) > 0:
		// Renames land here too: the method and path still answer, but the
		// generated client's function is gone, which a client cannot tell apart
		// from removal.
		if apiVersion <= lock.APIVersion {
			return fmt.Errorf("these operations disappeared since contract %d.%d, which is breaking — bump version.APIVersion to %d:\n  %s",
				lock.APIVersion, lock.APIMinor, lock.APIVersion+1, strings.Join(removed, "\n  "))
		}
	case len(added) > 0:
		if apiVersion == lock.APIVersion && apiMinor == lock.APIMinor {
			return fmt.Errorf("%d new operations since contract %d.%d — bump version.APIMinor to %d (once per batch, not once per change):\n  %s",
				len(added), lock.APIVersion, lock.APIMinor, lock.APIMinor+1, strings.Join(added, "\n  "))
		}
	}
	// Surface unchanged deliberately does NOT mean "no bump allowed": additive
	// changes include new fields on existing endpoints, which an operation set
	// cannot see. A bump with no new operation is legitimate.
	return nil
}

func diffOps(old, cur []string) (added, removed []string) {
	o, n := map[string]bool{}, map[string]bool{}
	for _, s := range old {
		o[s] = true
	}
	for _, s := range cur {
		n[s] = true
		if !o[s] {
			added = append(added, s)
		}
	}
	for _, s := range old {
		if !n[s] {
			removed = append(removed, s)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return
}
