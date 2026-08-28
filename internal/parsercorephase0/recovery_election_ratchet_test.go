package parsercorephase0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRecoveryElectionNoOutsideCallSitesRatchet is the compile-time,
// clean-path-zero-cost guardrail for recovery_election.go (campaign v7
// tranche B3 stage S4, first coherent sub-unit, design section 8 gates
// G2/G3): every symbol recovery_election.go declares at package scope must
// be referenced from nowhere else in this package's non-test source, proving
// by construction that ordinary condense, shift, reduce, and materialization
// code paths cannot reach the election scan. Same pattern and same rationale
// as TestRecoveryCostNoOutsideCallSitesRatchet (recovery_cost_ratchet_test.go)
// for stage S2's own inert landing, generalized here because the
// fork/materialization half of S4 is not yet built (recovery_election.go's
// own file doc comment). A later sub-unit that wires this in will make this
// ratchet fail on purpose -- that failure is the signal to update or retire
// it alongside the real call site landing.
func TestRecoveryElectionNoOutsideCallSitesRatchet(t *testing.T) {
	const homeFile = "recovery_election.go"

	fset := token.NewFileSet()
	home, err := parser.ParseFile(fset, homeFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", homeFile, err)
	}

	banned := map[string]bool{}
	for _, decl := range home.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				// A method: selected through a receiver, not a bare
				// package-scope identifier. Excluded (see recovery_cost_
				// ratchet_test.go's own doc comment for why).
				continue
			}
			banned[d.Name.Name] = true
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					banned[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.Name != "_" {
							banned[name.Name] = true
						}
					}
				}
			}
		}
	}
	if len(banned) == 0 {
		t.Fatalf("collected zero package-scope identifiers from %s; the ratchet would vacuously pass", homeFile)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	violations := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == homeFile {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok || !banned[ident.Name] {
				return true
			}
			violations++
			t.Errorf("%s references %s, a recovery_election.go (stage S4 sub-unit, inert) declaration -- "+
				"the clean path must not call into the election scan; if a later stage "+
				"intentionally wires this in, update this ratchet in the same change",
				fset.Position(ident.Pos()), ident.Name)
			return true
		})
	}
	if violations != 0 {
		t.Fatalf("%d reference(s) to recovery_election.go declarations found outside recovery_election.go and its own tests", violations)
	}
}
