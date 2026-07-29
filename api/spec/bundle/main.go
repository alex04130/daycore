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
//	go run ./api/spec/bundle -check    verify it is current (exit 1 if stale)
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Root is the repository-relative location of the spec. Overridable so the test
// can run from its own directory.
var Root = "api"

func main() {
	check := flag.Bool("check", false, "verify api/openapi.yaml matches the shards instead of writing it")
	root := flag.String("root", Root, "directory holding openapi.yaml and spec/")
	flag.Parse()

	want, err := Bundle(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bundle:", err)
		os.Exit(1)
	}
	out := filepath.Join(*root, "openapi.yaml")

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
