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
			{text: "|>"},
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
			if test.name == "awk" {
				testRecoveryAWKSkippedEOFEditsMatchLockedC(t)
			}
		})
	}
}

// F# reaches EOF with C's root span and error verdict. The remaining
// recovered-child difference for a lone bar stays pinned exactly.
func sameRecoveryEOFDeviation(got, want *DumpV1Divergence) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

func TestRecoveryAWKSkippedEOFEditsMatchLockedC(t *testing.T) {
	testRecoveryAWKSkippedEOFEditsMatchLockedC(t)
}

func testRecoveryAWKSkippedEOFEditsMatchLockedC(t *testing.T) {
	language := grammars.DetectLanguageByName("awk").Language()
	cLanguage, err := COracleLanguage("awk")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	point := func(source []byte) gotreesitter.Point {
		var p gotreesitter.Point
		for _, c := range source {
			if c == '\n' {
				p.Row++
				p.Column = 0
			} else {
				p.Column++
			}
		}
		return p
	}
	for _, candidate := range []bool{false, true} {
		parser := gotreesitter.NewParser(language)
		parser.SetAdmissionCandidateRoute(candidate)
		source := []byte("\\")
		old, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		for step, text := range []string{"\\\n", "\\\r\n", "\\", "", "\n", "\\", "\\\n"} {
			nextSource := []byte(text)
			start := 0
			for start < len(source) && start < len(nextSource) && source[start] == nextSource[start] {
				start++
			}
			old.Edit(gotreesitter.InputEdit{
				StartByte: uint32(start), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(nextSource)),
				StartPoint: point(source[:start]), OldEndPoint: point(source), NewEndPoint: point(nextSource),
			})
			next, err := parser.ParseIncremental(nextSource, old)
			if err != nil {
				t.Fatal(err)
			}
			freshParser := gotreesitter.NewParser(language)
			freshParser.SetAdmissionCandidateRoute(candidate)
			fresh, err := freshParser.Parse(nextSource)
			if err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(nextSource, nil)
			if cTree == nil {
				t.Fatal("locked C returned no tree")
			}
			t.Logf("candidate=%t step=%d input=%q incremental=%d..%d fresh=%d..%d", candidate, step, text, next.RootNode().StartByte(), next.RootNode().EndByte(), fresh.RootNode().StartByte(), fresh.RootNode().EndByte())
			for _, tree := range []*gotreesitter.Tree{next, fresh} {
				if tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
					t.Fatalf("candidate=%t step=%d stop=%s", candidate, step, tree.ParseStopReason())
				}
				if diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()); diff != nil {
					t.Fatalf("candidate=%t step=%d source=%q divergence=%+v", candidate, step, text, diff)
				}
			}
			cTree.Close()
			fresh.Release()
			old.Release()
			old, source = next, nextSource
			if allocations := testing.AllocsPerRun(100, func() {
				unchanged, err := parser.ParseIncremental(source, old)
				if err != nil {
					panic(err)
				}
				unchanged.Release()
			}); allocations != 0 {
				t.Fatalf("candidate=%t step=%d no-edit allocated %.2f times", candidate, step, allocations)
			}
		}
		old.Release()
	}
}
