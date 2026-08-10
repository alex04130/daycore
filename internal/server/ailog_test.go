package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// Every endpoint that writes a ledger row can be filtered to in the console.
//
// The two lists — the ep* constants and aiEndpoints() — are the same strings
// written twice, and that is only tolerable with a gate holding them together.
// What it catches is an omission, not a mistake: somebody adds a twelfth
// endpoint, logs calls under it, and never thinks about the console. The screen
// keeps working and simply never offers that choice, so the rows are there and
// unreachable — which looks exactly like "that endpoint is not being called".
func TestEveryAIEndpointIsFilterable(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "ailog.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	declared := map[string]string{} // constant name → value
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		if !strings.HasPrefix(spec.Names[0].Name, "ep") {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err == nil {
			declared[spec.Names[0].Name] = v
		}
		return true
	})

	// Without this the walk can find nothing — a renamed prefix, a moved const
	// block — and the loop below asserts about an empty set.
	if len(declared) < 8 {
		t.Fatalf("only found %d ep* constants in ailog.go; this gate is not looking at what it thinks it is", len(declared))
	}

	offered := map[string]bool{}
	for _, e := range aiEndpoints() {
		if offered[e] {
			t.Errorf("aiEndpoints() lists %q twice — the console would render two identical chips", e)
		}
		offered[e] = true
	}
	for name, value := range declared {
		if !offered[value] {
			t.Errorf("%s = %q writes ledger rows but aiEndpoints() does not offer it.\n"+
				"  The console's filter is built from that list, so those rows are unreachable — which on\n"+
				"  screen is indistinguishable from that endpoint never being called.", name, value)
		}
	}
	// And the reverse: a chip for an endpoint nothing writes filters to an empty
	// table, which reads as a broken screen rather than as a stale list.
	byValue := map[string]bool{}
	for _, v := range declared {
		byValue[v] = true
	}
	for e := range offered {
		if !byValue[e] {
			t.Errorf("aiEndpoints() offers %q but no ep* constant has that value — the chip filters to nothing", e)
		}
	}
}
