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

// TestAcceptanceElectionNoOutsideCallSitesRatchet is the compile-time
// guardrail that keeps acceptance_election.go inert: every symbol it declares
// at package scope must be referenced from nowhere else in this package's
// non-test source. It is the same pattern as
// TestRecoveryCostNoOutsideCallSitesRatchet
// (recovery_cost_ratchet_test.go) and
// TestCondenseWithOutcomeAtomicProvenanceRatchet
// (condense_outcome_provenance_ratchet_test.go).
//
// WHY THIS FILE IS INERT is measured, not provisional -- see
// acceptance_election.go's own header for the eight C-oracle witnesses where
// wiring the order into the compact acceptance election published a different
// tree from the reference runtime. The prerequisite for wiring it in is a
// live-derivation set proven equal to the reference runtime's live-version
// set. A change that wires this file in without that proof will make this
// ratchet fail; that failure is the signal to produce the proof, not to
// delete the ratchet.
//
// Method names are excluded from the banned set for the same reason
// recovery_cost.go's ratchet excludes them: they are selected through a
// receiver, not referenced as bare package-scope identifiers.
func TestAcceptanceElectionNoOutsideCallSitesRatchet(t *testing.T) {
	const homeFile = "acceptance_election.go"

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
			t.Errorf("%s references %s, an acceptance_election.go (inert) declaration -- "+
				"the shipping acceptance election must not call the selection order until the "+
				"compact live-derivation set is proven equal to the reference runtime's "+
				"live-version set; see acceptance_election.go's header for the measured witnesses",
				fset.Position(ident.Pos()), ident.Name)
			return true
		})
	}
	if violations != 0 {
		t.Fatalf("%d reference(s) to acceptance_election.go declarations found outside acceptance_election.go and its own tests", violations)
	}
}
