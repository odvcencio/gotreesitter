//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestTokenInvariantUnicodeLookaheadContinuation(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		name := "flat"
		if grouped {
			name = "grouped"
		}
		t.Run(name, func(t *testing.T) {
			g := grammargen.NewGrammar("unicode_lookahead")
			g.Define("program", grammargen.Repeat1(grammargen.Sym("statement")))
			prefix := grammargen.Seq(grammargen.Str("("), grammargen.Sym("word"))
			if grouped {
				g.Define("group", prefix)
				prefix = grammargen.Sym("group")
			}
			g.Define("statement", grammargen.Seq(prefix, grammargen.Repeat(grammargen.Sym("letter")), grammargen.Str(";")))
			g.Define("word", grammargen.Pat("a|abbb😀"))
			g.Define("letter", grammargen.Pat("[b😀😁]"))
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
					before, _ := gts.AdmissionCandidateCounters()
					old, err := p.Parse([]byte("(abbb😁;"))
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					after, _ := gts.AdmissionCandidateCounters()
					if old.RootNode().HasError() || (after > before) != (route == "compact") || old.ParseRuntime().ForestFastPath != (route == "forest") {
						t.Fatal("initial fixture did not use the requested clean route")
					}
					wordOf := func(tree *gts.Tree) *gts.Node {
						statement := tree.RootNode().Child(0)
						if grouped {
							return statement.Child(0).Child(1)
						}
						return statement.Child(1)
					}
					if word := wordOf(old); word.Type(lang) != "word" || word.EndByte() != 2 {
						t.Fatal("initial fixture did not retain the short word")
					}
					// The failed word probe decodes bytes 5..9. Only its last
					// continuation byte changes; the later letter keeps its class.
					for _, source := range []string{"(abbb😀;", "(abbb😁;", "(abbb😀;"} {
						old.Edit(gts.InputEdit{StartByte: 8, OldEndByte: 9, NewEndByte: 9,
							StartPoint: gts.Point{Column: 8}, OldEndPoint: gts.Point{Column: 9}, NewEndPoint: gts.Point{Column: 9}})
						next, profile, err := p.ParseIncrementalProfiled([]byte(source), old)
						if err != nil {
							t.Fatal(err)
						}
						old.Release()
						old = next
						freshParser := gts.NewParser(lang)
						freshParser.SetAdmissionCandidateRoute(false)
						fresh, err := freshParser.Parse([]byte(source))
						if err != nil {
							t.Fatal(err)
						}
						func() {
							defer fresh.Release()
							wantEnd := uint32(2)
							if source[8] == 0x80 {
								wantEnd = 9
							}
							if fresh.RootNode().HasError() || wordOf(fresh).EndByte() != wantEnd {
								t.Fatal("fresh fixture selected the wrong word extent")
							}
							got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
							if err != nil {
								t.Fatal(err)
							}
							want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
							if err != nil {
								t.Fatal(err)
							}
							if got.SHA256 != want.SHA256 {
								t.Errorf("source=%q incremental=%s fresh=%s digest=%s want=%s reparse_ns=%d", source,
									next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang), got.SHA256, want.SHA256, profile.ReparseNanos)
							}
						}()
					}
				})
			}
		})
	}
}
