//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/csv"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCsvEOFAcceptLockedC(t *testing.T) {
	cl, err := ParityCLanguage("csv")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"", "a", "a\n", "a,b,c\n1,2,3\n", "a,b,\n", "\n", "a\r\n"} {
		t.Run(source, func(t *testing.T) {
			oracle := cp.Parse([]byte(source), nil)
			if oracle == nil {
				t.Fatal("C tree is nil")
			}
			defer oracle.Close()
			for _, compact := range []bool{false, true} {
				name := "production"
				if compact {
					name = "compact"
				}
				t.Run(name, func(t *testing.T) {
					lang := csv.Language()
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compact)
					before, failed := gts.AdmissionCandidateCounters()
					tree, err := parser.Parse([]byte(source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertLockedCTreeExact(t, name, tree, lang, oracle)
					if compact {
						routed, fallback := gts.AdmissionCandidateCounters()
						if routed != before+1 || fallback != failed {
							t.Fatalf("compact parse fell back: %s", gts.AdmissionCandidateLastFallbackReason())
						}
					}
				})
			}
		})
	}
}
