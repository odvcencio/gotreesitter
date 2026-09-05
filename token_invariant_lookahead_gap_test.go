//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestTokenInvariantLookaheadSkippedExtraGap(t *testing.T) {
	g := grammargen.NewGrammar("token_invariant_gap")
	g.Define("program", grammargen.Seq(grammargen.Str("a"), grammargen.Sym("word")))
	g.Define("word", grammargen.Pat("[xy]"))
	g.SetExtras(grammargen.Pat(" +"), grammargen.Pat(" +x+"))
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		name := "legacy"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			before, _ := gts.AdmissionCandidateCounters()
			old, err := p.Parse([]byte("a y"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			after, _ := gts.AdmissionCandidateCounters()
			if (after > before) != compact || old.RootNode().HasError() {
				t.Fatal("initial fixture did not use the requested clean route")
			}
			word := old.RootNode().Child(1)
			if word == nil || word.Type(lang) != "word" || word.StartByte() != 2 || word.EndByte() != 3 {
				t.Fatal("initial fixture did not retain the word at bytes 2..3")
			}
			// Starting at byte 2 sees the same word class. Starting before the
			// skipped gap can consume the replacement as part of an extra.
			for _, source := range []string{"a x", "a y", "a x"} {
				old.Edit(gts.InputEdit{StartByte: 2, OldEndByte: 3, NewEndByte: 3,
					StartPoint: gts.Point{Column: 2}, OldEndPoint: gts.Point{Column: 3}, NewEndPoint: gts.Point{Column: 3}})
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
					if fresh.RootNode().HasError() != (source == "a x") {
						t.Fatalf("fresh gap fixture has unexpected error state: %q %s", source, fresh.RootNode().SExpr(lang))
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
}
