//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestReleaseTokenInvariantFallbackLongestMatch(t *testing.T) {
	for _, grouped := range []bool{true, false} {
		name := "flat"
		if grouped {
			name = "grouped"
		}
		t.Run(name, func(t *testing.T) {
			g := grammargen.NewGrammar("token_invariant_lookahead")
			g.Define("program", grammargen.Repeat1(grammargen.Sym("statement")))
			if grouped {
				g.Define("statement", grammargen.Seq(grammargen.Sym("group"), grammargen.Repeat(grammargen.Sym("letter")), grammargen.Str(";")))
				g.Define("group", grammargen.Seq(grammargen.Str("("), grammargen.Sym("word")))
			} else {
				g.Define("statement", grammargen.Seq(grammargen.Str("("), grammargen.Sym("word"), grammargen.Repeat(grammargen.Sym("letter")), grammargen.Str(";")))
			}
			g.Define("word", grammargen.Pat("a|ab+c"))
			g.Define("letter", grammargen.Pat("[bcd]"))
			lang, err := grammargen.GenerateLanguage(g)
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range []string{"legacy", "compact", "forest"} {
				t.Run(route, func(t *testing.T) {
					languageCopy := *lang
					languageCopy.WantsForest = route == "forest"
					p := gts.NewParser(&languageCopy)
					p.SetAdmissionCandidateRoute(route == "compact")
					routedBefore, _ := gts.AdmissionCandidateCounters()
					old, err := p.Parse([]byte("(abbbd;"))
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					routedAfter, _ := gts.AdmissionCandidateCounters()
					if (routedAfter > routedBefore) != (route == "compact") || old.ParseRuntime().ForestFastPath != (route == "forest") {
						t.Fatal("initial tree used the wrong engine")
					}
					if old.RootNode().HasError() {
						t.Fatal("initial fixture contains an error")
					}
					// The short word ends at byte 2. Its lexer examined byte 5.
					// Repeat both directions without replacing the incremental tree.
					for _, source := range []string{"(abbbc;", "(abbbd;", "(abbbc;"} {
						old.Edit(gts.InputEdit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartPoint: gts.Point{Column: 5}, OldEndPoint: gts.Point{Column: 6}, NewEndPoint: gts.Point{Column: 6}})
						next, profile, err := p.ParseIncrementalProfiled([]byte(source), old)
						if err != nil {
							t.Fatal(err)
						}
						if profile.ReparseNanos <= 0 {
							next.Release()
							t.Fatalf("real edit bypassed reparsing: %+v", profile)
						}
						old.Release()
						old = next
						freshParser := gts.NewParser(lang)
						freshParser.SetAdmissionCandidateRoute(false)
						fresh, err := freshParser.Parse([]byte(source))
						if err != nil {
							t.Fatal(err)
						}
						want := "a"
						if source[5] == 'c' {
							want = "abbbc"
						}
						word := fresh.RootNode().Child(0).Child(1)
						if grouped {
							word = fresh.RootNode().Child(0).Child(0).Child(1)
						}
						if fresh.RootNode().HasError() || word.Type(lang) != "word" || word.Text([]byte(source)) != want {
							fresh.Release()
							t.Fatal("fresh fixture did not select the expected word")
						}
						a, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
						if err != nil {
							fresh.Release()
							t.Fatal(err)
						}
						b, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
						if err != nil {
							fresh.Release()
							t.Fatal(err)
						}
						if a.SHA256 != b.SHA256 {
							t.Errorf("source=%q incremental=%s fresh=%s reparse_ns=%d new_nodes=%d", source, next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang), profile.ReparseNanos, profile.NewNodesAllocated)
						}
						requireForestFallbackNodeIdentity(t, "root", next.RootNode(), fresh.RootNode(), lang)
						fresh.Release()
					}
				})
			}
		})
	}
}
