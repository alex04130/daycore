package adapters_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The approval gate has a property that makes it uniquely fragile: **removing
// it is a one-line change that no behavioural test would catch.**
//
// Source.Description and Source.ManifestDescription hold "the description" from
// two different parties — an operator who was read, and an adapter on the
// network who was not. Somebody tidying up sees two fields holding the same
// shape, assigns one to the other, and every adapter anywhere gains the ability
// to write into the system prompt of every conversation. Nothing goes red,
// because every existing test still passes: descriptions still appear,
// approval still round-trips.
//
// So the gate is structural and checked in the source. Reading source in a test
// is normally a smell; here it is the only place the property is visible, the
// same reasoning as the runtime-snapshot and undo-registry gates elsewhere in
// this repository.
func TestNothingAssignsTheManifestDescriptionIntoTheInjectedOne(t *testing.T) {
	root := ".."
	checked := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil // not our business; the compiler will say so
		}
		ast.Inspect(f, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, lhs := range assign.Lhs {
				if i >= len(assign.Rhs) {
					break
				}
				if selects(lhs, "Description") && selects(assign.Rhs[i], "ManifestDescription") {
					t.Errorf("%s:%d assigns ManifestDescription into Description. "+
						"That is the approval gate: an adapter would be able to write into the system prompt. "+
						"If an operator wants to adopt an adapter's suggested text, they copy it in the console, "+
						"which records an approval against its hash.",
						path, fset.Position(assign.Pos()).Line)
				}
			}
			return true
		})
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Without this the gate passes on an empty walk and asserts nothing — the
	// exact failure mode of every source-reading check.
	if checked < 5 {
		t.Fatalf("only walked %d files; this gate is not looking at what it thinks it is", checked)
	}
}

// selects reports whether an expression ends in .<name>.
func selects(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel != nil && sel.Sel.Name == name
}

// PromptDescription is the single door between a source and a prompt. If a
// second accessor appears, the gate above has to know about it — and the person
// adding one is unlikely to come here. This fails when Source grows another
// method returning per-locale text, which is the moment to think.
func TestSourceHasExactlyOneDoorIntoAPrompt(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "source.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := []string{}
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Type.Params == nil {
			return true
		}
		// A method on Source taking a locale and returning a string is, by
		// shape, something that produces user- or model-facing text.
		if len(fn.Type.Params.List) != 1 || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
			return true
		}
		if ident, ok := fn.Type.Params.List[0].Type.(*ast.Ident); !ok || ident.Name != "string" {
			return true
		}
		if ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "string" {
			return true
		}
		found = append(found, fn.Name.Name)
		return true
	})
	want := map[string]bool{"PromptDescription": true, "mechanicalDescription": true}
	for _, name := range found {
		if !want[name] {
			t.Errorf("Source.%s(locale) string is a new way to produce text from a source. "+
				"If it can reach a prompt it needs the approval check that PromptDescription has; "+
				"if it cannot, say so here and add it to this list.", name)
		}
	}
	if len(found) < 2 {
		t.Fatalf("found %v; this gate is not matching what it thinks it is", found)
	}
}
