package resources_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daycore/internal/resources"
)

// `-tags lite` has to mean something, and the last one did not.
//
// Its //go:embed directive lived in a file with no build tag, so the content
// shipped either way; the measured difference was 20 KiB out of 23.5 MB, and
// the only behaviour the tag changed was making the installer's extraction a
// no-op. This walks the source and fails if an embed directive appears anywhere
// a lite build would still compile it.
//
// Source-reading tests are usually a smell. Here it is the only place the
// property is visible: a lite binary that still carries the templates looks
// exactly like one that does not, from inside itself.
func TestLiteBuildEmbedsNothingOutsideTheFullFile(t *testing.T) {
	root := ".."
	found := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(src)
		// Real directives only: a line that BEGINS with //go:embed. Prose that
		// mentions the word (this package's own doc comment does) is not a
		// directive, and a gate that cannot tell the difference fires on the
		// documentation explaining why it exists.
		hasDirective := false
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//go:embed ") {
				hasDirective = true
				break
			}
		}
		if !hasDirective {
			return nil
		}
		found++
		base := filepath.Base(path)
		if base == "resources_full.go" {
			return nil
		}
		// Any other file must exclude itself from lite builds, or the tag is a
		// lie again.
		head := text
		if i := strings.Index(text, "package "); i > 0 {
			head = text[:i]
		}
		if !strings.Contains(head, "//go:build lite") && !strings.Contains(head, "!lite") {
			t.Errorf("%s has a //go:embed directive but no lite build constraint — "+
				"a lite build would still carry this content, which is exactly the bug the old -tags lite had", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("walked the tree and found no //go:embed at all — this gate is not looking at what it thinks it is")
	}
}

// The resources every build must be able to read. Named individually rather
// than counted, because a count passes while the one file that matters is
// missing.
func TestRequiredResourcesResolve(t *testing.T) {
	for _, name := range []string{
		"prompts/boundaries.json",
		"prompts/zh-CN/companion_agent.tmpl",
		"prompts/en-US/companion_agent.tmpl",
		"seed/models.yaml",
	} {
		b, err := resources.Read(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("%s resolved but is empty", name)
		}
	}
}

// In a lite build the error has to carry the recovery path, because "no such
// file" would send the reader into the source tree looking for something that
// was never there.
func TestMissingResourceErrorSaysWhereToLook(t *testing.T) {
	if !resources.Lite {
		t.Skip("full build: a missing resource means a broken binary, not a missing directory")
	}
	resources.SetDataDir(t.TempDir())
	_, err := resources.Read("prompts/boundaries.json")
	if err == nil {
		t.Fatal("reading from an empty data dir succeeded")
	}
	for _, want := range []string{resources.DataDirEnvKey, "-fetch"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q, so it does not say how to recover: %v", want, err)
		}
	}
}
