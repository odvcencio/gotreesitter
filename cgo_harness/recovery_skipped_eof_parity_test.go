//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestRecoverySkippedEOFMatchesLockedC(t *testing.T) {
	type witness struct {
		text           string
		wantDivergence *DumpV1Divergence
	}
	for _, test := range []struct {
		name    string
		sources []witness
	}{
		{"awk", []witness{{text: "\\"}, {text: "\\\n"}, {text: "\\\r\n"}}},
		{"fsharp", []witness{
			{"|", &DumpV1Divergence{Path: "/file/ERROR[0]", Category: "shape", GoValue: "children=0", CValue: "children=1"}},
			{"|>", &DumpV1Divergence{Path: "/file/ERROR[0]", Category: "extra", GoValue: "false", CValue: "true"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			language := grammars.DetectLanguageByName(test.name).Language()
			cLanguage, err := COracleLanguage(test.name)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			for _, witness := range test.sources {
				source := witness.text
				t.Run(source, func(t *testing.T) {
					cTree := cParser.Parse([]byte(source), nil)
					if cTree == nil {
						t.Fatal("locked C returned no tree")
					}
					defer cTree.Close()
					for _, candidate := range []bool{false, true} {
						parser := gotreesitter.NewParser(language)
						parser.SetAdmissionCandidateRoute(candidate)
						routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
						tree, err := parser.Parse([]byte(source))
						if err != nil {
							t.Fatal(err)
						}
						defer tree.Release()
						routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
						t.Logf("candidate=%t routed=%d fallback=%d span=%d..%d", candidate, routedAfter-routedBefore, fallbackAfter-fallbackBefore, tree.RootNode().StartByte(), tree.RootNode().EndByte())
						if tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
							t.Fatalf("candidate=%t stop=%s", candidate, tree.ParseStopReason())
						}
						root, cRoot := tree.RootNode(), cTree.RootNode()
						if root.Type(language) != cRoot.Kind() || root.StartByte() != uint32(cRoot.StartByte()) || root.EndByte() != uint32(cRoot.EndByte()) || root.HasError() != cRoot.HasError() {
							t.Fatalf("candidate=%t root differs from locked C: %s %d..%d error=%t", candidate, root.Type(language), root.StartByte(), root.EndByte(), root.HasError())
						}
						diff := FirstDivergenceDumpV1(root, language, cRoot)
						if !sameRecoveryEOFDeviation(diff, witness.wantDivergence) {
							t.Fatalf("candidate=%t divergence=%+v, want %+v", candidate, diff, witness.wantDivergence)
						}
						unchanged, err := parser.ParseIncremental([]byte(source), tree)
						if err != nil {
							t.Fatal(err)
						}
						defer unchanged.Release()
						if diff := FirstDivergenceDumpV1(unchanged.RootNode(), language, cTree.RootNode()); !sameRecoveryEOFDeviation(diff, witness.wantDivergence) {
							t.Fatalf("candidate=%t no-edit divergence=%+v, want %+v", candidate, diff, witness.wantDivergence)
						}
					}
				})
			}
		})
	}
}

// F# now reaches EOF with C's root span and error verdict. Its recovered
// ERROR children and extra flags still differ; keep those differences exact.
func sameRecoveryEOFDeviation(got, want *DumpV1Divergence) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}
