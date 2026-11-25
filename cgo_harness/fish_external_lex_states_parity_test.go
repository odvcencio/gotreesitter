//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestFishExternalLexStatesLockedCParity locks three fish inputs to the trees
// that the pinned C parser gives. The fish blob that shipped until November
// 2026 had no ExternalLexStates table, so the C-recovery gate refused fish and
// every fish parse with an error recovered on the legacy path: each input
// below gave an ERROR root, or an ERROR with no children, where C gives a
// program. The blob regenerated from the same pinned parser.c carries the
// table's 8 rows. Issue #1529 reported the first witness: valid input without
// a final newline.
//
// Each witness runs on the default and the candidate route and must match C
// under runParityCase and under the stricter dump.v1 comparison, which also
// checks the extra flag and the error flag of every node.
func TestFishExternalLexStatesLockedCParity(t *testing.T) {
	rows := []witness{
		w("issue-1529-no-final-newline", "echo hi"),
		w("missing-terminator", "complete -x"),
		w("lone-redirect", "<"),
	}
	runWitnessSet(t, witnessSet{lang: "fish", label: "fish-external-lex-states", routes: true, rows: rows})

	cLang, err := ParityCLanguage("fish")
	if err != nil {
		t.Fatalf("load fish C parser: %v", err)
	}
	for _, row := range rows {
		for _, candidate := range []bool{false, true} {
			route := "default"
			if candidate {
				route = "candidate"
			}
			t.Run("dump.v1/"+row.name+"/"+route, func(t *testing.T) {
				src := []byte(row.source)
				goTree, goLang, err := parseWithGo(parityCase{name: "fish", candidateRoute: &candidate}, src, nil)
				if err != nil {
					t.Fatalf("Go parse: %v", err)
				}
				defer releaseGoTree(goTree)
				cParser := sitter.NewParser()
				defer cParser.Close()
				if err := cParser.SetLanguage(cLang); err != nil {
					t.Fatalf("C SetLanguage: %v", err)
				}
				cTree := cParser.Parse(src, nil)
				if cTree == nil || cTree.RootNode() == nil {
					t.Fatal("C parser returned no tree")
				}
				defer cTree.Close()
				if div := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()); div != nil {
					t.Fatalf("first divergence from C: %+v\n  go: %s\n  c:  %s", *div,
						goTree.RootNode().SExpr(goLang), cTree.RootNode().ToSexp())
				}
			})
		}
	}
}
