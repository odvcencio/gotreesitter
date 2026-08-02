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

// TestRecoveryCostNoOutsideCallSitesRatchet is the compile-time,
// bounded-call-site guardrail for recovery_cost.go (campaign v7 tranche
// B3 stage S2, design section 8 gates G2/G3): every symbol recovery_cost.go
// declares at package scope must be referenced from nowhere in this
// package's non-test source EXCEPT the one file listed in callers below.
// This is the same pattern as TestCondenseWithOutcomeAtomicProvenanceRatchet
// (condense_outcome_provenance_ratchet_test.go), generalized from one
// function name to a whole file's declared surface.
//
// Method names (Reset, Len, lookup, store, ...) are deliberately excluded
// from the banned set: they are selected through a receiver, not referenced
// as bare package-scope identifiers, and some are common enough (Core.Reset,
// for one) that banning them by name would false-positive on unrelated
// methods. What this ratchet protects is the set of package-scope
// declarations recovery_cost.go introduces: its exported API plus its two
// unexported helper functions/constant.
//
// callers is the authorized call-site list, and it is deliberately a list of
// exactly one file rather than an "off" switch. The one authorized caller is
// acceptance_election.go, which reads the cost model for rungs 1, 2, and 5 of
// ts_parser__select_tree and reuses RecoveryCostSource for the symbol and
// child-count view ts_subtree_compare needs.
//
// That caller is ITSELF inert (TestAcceptanceElectionNoOutsideCallSitesRatchet,
// acceptance_election_ratchet_test.go), so the clean-path-zero-cost property
// stage S2 established is unchanged: no condense, shift, reduce, or
// materialization path can reach the cost model, and now no shipping path
// can reach it through the election either.
func TestRecoveryCostNoOutsideCallSitesRatchet(t *testing.T) {
	const homeFile = "recovery_cost.go"
	callers := map[string]bool{"acceptance_election.go": true}

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
				// package-scope identifier. Excluded (see doc comment).
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
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			name == homeFile || callers[name] {
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
			t.Errorf("%s references %s, a recovery_cost.go (stage S2, inert) declaration -- "+
				"the clean path must not call into the recovery cost model; if a later stage "+
				"intentionally wires this in, update this ratchet in the same change",
				fset.Position(ident.Pos()), ident.Name)
			return true
		})
	}
	if violations != 0 {
		t.Fatalf("%d reference(s) to recovery_cost.go declarations found outside recovery_cost.go and its own tests", violations)
	}
}
