package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The gate that closes the undo loop.
//
// # The failure it prevents
//
// Delete the `registerRevert("wish_create", …)` line and everything stays green:
// it compiles (Go does not mind an unreferenced method), it vets, every test
// passes, CI is entirely happy. The only symptom is a user pressing undo and
// getting a 400. The registry is a map, so a missing entry is not a compile
// error, and until now nothing anywhere compared the set of actions we WRITE
// against the set we can UNDO.
//
// The old test (TestKnownWritesAreReversible) hard-coded twelve action names.
// By the time this replaced it there were sixteen — the four capture tools added
// in β0+ were never added to the list, so the list was checking a subset of
// itself and reported clean.
//
// # How it works
//
// Parse the package, find every `domain.OperationLog{…}` composite literal, and
// read its `Action:` field. Each action found must be declared exactly once,
// either as reversible (registerRevert) or as deliberately irreversible
// (registerIrreversible, with a reason). Nothing may be neither.
//
// # What it cannot see, stated so nobody assumes otherwise
//
// An Action built at runtime rather than written as a literal — a map lookup, a
// variable, a fmt.Sprintf. Those are listed by hand in dynamicActionSites below,
// and the test checks that list against reality in BOTH directions: an
// undeclared dynamic site fails, and a declared site that no longer exists fails
// too. That is the only way a hand-maintained list stays honest.

// dynamicActionSites names the places where Action is not a string literal, and
// which actions each can produce. file:line, so moving the code fails the test
// and makes somebody look.
var dynamicActionSites = map[string][]string{
	"plan_patch.go": {"plan_add", "plan_update", "plan_remove"},
	"proposals.go":  {"proposal_accepted", "proposal_rejected"},
}

func TestEveryLoggedActionIsDeclared(t *testing.T) {
	literal, dynamic := collectLoggedActions(t)

	// Every dynamic site must be declared, and every declared site must still
	// exist. A hand-written list only stays true if both directions are checked.
	for file := range dynamic {
		if _, ok := dynamicActionSites[file]; !ok {
			t.Errorf("%s builds an OperationLog.Action from a non-literal and is not in dynamicActionSites.\n"+
				"Add it with the actions it can produce, or the gate below cannot see them.", file)
		}
	}
	for file := range dynamicActionSites {
		if _, ok := dynamic[file]; !ok {
			t.Errorf("dynamicActionSites lists %s, but nothing there builds an Action dynamically any more — drop the entry", file)
		}
	}

	all := map[string]string{} // action → where it is written
	for action, site := range literal {
		all[action] = site
	}
	for file, actions := range dynamicActionSites {
		for _, a := range actions {
			if _, seen := all[a]; !seen {
				all[a] = file + " (dynamic)"
			}
		}
	}

	var undeclared []string
	for action, site := range all {
		_, canUndo := revertHandlers[action]
		_, declared := irreversibleActions[action]
		if !canUndo && !declared {
			undeclared = append(undeclared, action+"  (written at "+site+")")
		}
	}
	sort.Strings(undeclared)
	if len(undeclared) > 0 {
		t.Errorf("%d logged action(s) are neither reversible nor declared irreversible:\n\n%s\n\n"+
			"Every write lands in an append-only ledger with an undo button pointing at it.\n"+
			"Either registerRevert(action, …) next to the code that writes it, or\n"+
			"registerIrreversible(action, \"why there is nothing to undo\") — the reason is the\n"+
			"point: it is what lets the next reader tell a decision from an oversight.",
			len(undeclared), strings.Join(undeclared, "\n"))
	}

	// And the reverse: a registration nobody produces is the repo's recurring
	// "written, tested, no callers" shape, five times over by now.
	for action := range revertHandlers {
		if _, produced := all[action]; !produced {
			t.Errorf("revert registered for %q but nothing writes that action — dead code, or the writer was renamed", action)
		}
	}
	for action := range irreversibleActions {
		if _, produced := all[action]; !produced {
			t.Errorf("%q is declared irreversible but nothing writes it", action)
		}
	}
}

// Every reason must actually say something. "n/a" passes a check that only tests
// for emptiness, and then the field is decoration.
func TestIrreversibleReasonsAreReasons(t *testing.T) {
	for action, reason := range irreversibleActions {
		if len(strings.TrimSpace(reason)) < 20 {
			t.Errorf("%q: reason %q is too short to tell anyone why", action, reason)
		}
	}
}

// collectLoggedActions returns literal actions (action → file:line) and the set
// of files that build one dynamically.
func collectLoggedActions(t *testing.T) (map[string]string, map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	literal := map[string]string{}
	dynamic := map[string]bool{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			base := filepath.Base(path)
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok || !isOperationLog(lit.Type) {
					return true
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if id, ok := kv.Key.(*ast.Ident); !ok || id.Name != "Action" {
						continue
					}
					if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
						if v, err := strconv.Unquote(bl.Value); err == nil {
							literal[v] = base + ":" + strconv.Itoa(fset.Position(bl.Pos()).Line)
						}
						continue
					}
					dynamic[base] = true
				}
				return true
			})
		}
	}
	if len(literal) == 0 {
		t.Fatal("found no OperationLog literals at all — the parser is looking at the wrong thing, so this gate would pass on an empty set")
	}
	return literal, dynamic
}

// isOperationLog matches `domain.OperationLog` as a composite-literal type.
// Matching on the type rather than on the field name matters: other structs in
// this package have an Action field too.
func isOperationLog(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "OperationLog" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "domain"
}
