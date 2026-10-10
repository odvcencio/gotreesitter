//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCompactLiquidBlockTextLockedC(t *testing.T) {
	lang := grammars.LiquidLanguage()

	cLang, err := ParityCLanguage("liquid")
	if err != nil {
		t.Fatal(err)
	}
	sourceFile, err := os.ReadFile("../internal/benchfixtures/testdata/real/liquid")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, source string }{
		{"if", "{% if x %}text{% endif %}"},
		{"if_else", "{% if x %}\n  yes\n{% else %}\n  no\n{% endif %}\n"},
		{"for", "{% for x in xs %}text{% endfor %}"},
		{"r4", string(sourceFile)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source
			cTree := compactT3ParseC(t, cLang, []byte(source))
			defer cTree.Close()
			for _, compact := range []bool{false, true} {
				route := "legacy"
				if compact {
					route = "compact"
				}
				t.Run(route, func(t *testing.T) {
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compact)
					beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
					tree, err := parser.Parse([]byte(source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					wantRouted := uint64(0)
					if compact {
						wantRouted = 1
					}
					if routed, fallback := gts.AdmissionCandidateCounters(); routed-beforeRouted != wantRouted || fallback != beforeFallback {
						t.Fatalf("compact route=%d fallback=%d: %s", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
					}
					if diffs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode()); len(diffs) != 0 {
						t.Fatalf("%s\nGo:\n%s\nC:\n%s", compactT3FormatDivergence(diffs[0]), dumpGoTree(tree.RootNode(), lang, 0), dumpCTree(cTree.RootNode(), 0))
					}
				})
			}
		})
	}
}
