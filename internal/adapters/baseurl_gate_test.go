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

// Nothing outside internal/adapters may read Entry.BaseURL.
//
// Entry.BaseURL is what the FILE said. It stops being the truth the moment an
// operator edits the address from the console, and the override then lives in
// Source.effectiveBaseURL. A consumer that kept reading the entry would build
// its client against the old address — and the symptom is the worst kind: the
// console shows the new URL, the override row holds the new URL, the health
// check goes to the new URL, and the actual requests go somewhere else.
//
// Source.BaseURL() is the accessor. This walks the tree because the mistake is
// a field access that compiles, reads naturally, and no behavioural test would
// notice until somebody changed an address in production.
func TestNothingOutsideAdaptersReadsTheFileBaseURL(t *testing.T) {
	root := "../.."
	checked := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == "node_modules" || d.Name() == "design-ui" || d.Name() == ".git") {
				return filepath.SkipDir
			}
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// The adapters package owns the field and legitimately reads it.
		if strings.Contains(filepath.ToSlash(path), "internal/adapters/") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "BaseURL" {
				return true
			}
			// Entry.BaseURL is the file's value; anything else (Source.BaseURL(),
			// a config field, a local struct) is fine.
			inner, ok := sel.X.(*ast.SelectorExpr)
			if !ok || inner.Sel == nil || inner.Sel.Name != "Entry" {
				return true
			}
			t.Errorf("%s:%d reads Entry.BaseURL. That is what the FILE said — "+
				"it stops being true the moment an operator edits the address from the console. "+
				"Use Source.BaseURL(), which returns the override when there is one.",
				path, fset.Position(sel.Pos()).Line)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only walked %d files; this gate is not looking at what it thinks it is", checked)
	}
}
