package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The test runs in api/spec/bundle/, so api/ is two levels up.
const specRoot = "../.."

// The committed api/openapi.yaml must match the shards.
//
// This is in `go test ./...` rather than only in a make target because that is
// the command actually run before committing, and because a stale generated
// contract is the failure mode with no other symptom: the server is right, the
// shards are right, and every client reads a file that describes neither.
func TestBundledSpecIsCurrent(t *testing.T) {
	want, err := Bundle(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(specRoot, "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("api/openapi.yaml is stale — run `make api-bundle`.\ncommitted %d bytes, rebuilt %d bytes%s",
			len(got), len(want), firstDiff(got, want))
	}
}

// firstDiff points at the diverging line, since a byte count alone does not say
// what to look at in a 900-line file.
func firstDiff(a, b []byte) string {
	al, bl := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := range max(len(al), len(bl)) {
		var x, y []byte
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if !bytes.Equal(x, y) {
			return "\nfirst difference at line " + itoa(i+1) + ":\n  committed: " + string(x) + "\n  rebuilt:   " + string(y)
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// A check that cannot fire is worse than no check — it reads as a guarantee and
// provides none. These three build a broken spec on purpose and assert the
// bundler refuses it.
func TestBundlerRejectsMisfiledPaths(t *testing.T) {
	cases := []struct {
		name    string
		tags    []string
		shards  map[string]string
		wantErr string
	}{
		{
			name:    "operation tagged something other than its file",
			tags:    []string{"plan"},
			shards:  map[string]string{"plan": "/api/plan:\n  get: { tags: [ai], responses: {} }\n"},
			wantErr: "lives in the \"plan\" shard",
		},
		{
			name: "shard nobody declared",
			tags: []string{"plan"},
			shards: map[string]string{
				"plan":  "/api/plan:\n  get: { tags: [plan], responses: {} }\n",
				"stray": "/api/stray:\n  get: { tags: [stray], responses: {} }\n",
			},
			wantErr: "head.yaml does not declare",
		},
		{
			name:    "declared tag with no shard",
			tags:    []string{"plan", "ghost"},
			shards:  map[string]string{"plan": "/api/plan:\n  get: { tags: [plan], responses: {} }\n"},
			wantErr: "does not exist",
		},
		{
			name:    "path items left indented (i.e. pasted straight out of the bundle)",
			tags:    []string{"plan"},
			shards:  map[string]string{"plan": "paths:\n  /api/plan:\n    get: { tags: [plan], responses: {} }\n"},
			wantErr: "is not a path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeFixture(t, tc.tags, tc.shards)
			_, err := Bundle(root)
			if err == nil {
				t.Fatalf("bundled a broken spec without complaining")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.wantErr)) {
				t.Errorf("error does not say what is wrong:\n  got:  %v\n  want to contain: %q", err, tc.wantErr)
			}
		})
	}
}

func writeFixture(t *testing.T, tags []string, shards map[string]string) string {
	t.Helper()
	root := t.TempDir()
	paths := filepath.Join(root, "spec", "paths")
	if err := os.MkdirAll(paths, 0o755); err != nil {
		t.Fatal(err)
	}
	head := "openapi: 3.1.0\ntags:\n"
	for _, tag := range tags {
		head += "  - { name: " + tag + " }\n"
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "spec", "head.yaml"), head)
	write(filepath.Join(root, "spec", "components.yaml"), "components: {}\n")
	for tag, body := range shards {
		write(filepath.Join(paths, tag+".yaml"), body)
	}
	return root
}

// Round-trip: the assembled document must still parse, and every path from every
// shard must be present. Concatenation is only safe if it produces valid YAML —
// an indentation slip would make a syntactically fine but structurally wrong
// document, which is exactly the failure a text-assembling bundler risks.
func TestBundleParsesAndKeepsEveryPath(t *testing.T) {
	raw, err := Bundle(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components map[string]any            `yaml:"components"`
		Tags       []struct{ Name string }   `yaml:"tags"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the bundle does not parse: %v", err)
	}
	shards, err := filepath.Glob(filepath.Join(specRoot, "spec", "paths", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, f := range shards {
		var sp map[string]map[string]any
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(b, &sp); err != nil {
			t.Fatal(err)
		}
		for path, item := range sp {
			total++
			got, ok := doc.Paths[path]
			if !ok {
				t.Errorf("%s: %s is in the shard but not in the bundle", filepath.Base(f), path)
				continue
			}
			if len(got) != len(item) {
				t.Errorf("%s: %s has %d operations in the shard, %d in the bundle", filepath.Base(f), path, len(item), len(got))
			}
		}
	}
	if total != len(doc.Paths) {
		t.Errorf("shards hold %d paths, the bundle has %d — a path is defined in two shards and one silently won", total, len(doc.Paths))
	}
	if len(doc.Components) == 0 {
		t.Error("components did not make it into the bundle")
	}
	if len(doc.Tags) != len(shards) {
		t.Errorf("%d tags declared, %d shards on disk", len(doc.Tags), len(shards))
	}
}
