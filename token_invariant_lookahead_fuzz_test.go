//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// This target uses fresh Go parsing as its oracle, not the locked C parser.
func FuzzTokenInvariantLookaheadFlatFreshParity(f *testing.F) {
	g := grammargen.NewGrammar("fuzz_token_invariant_flat")
	g.Define("program", grammargen.Repeat1(grammargen.Sym("statement")))
	g.Define("statement", grammargen.Seq(grammargen.Str("("), grammargen.Sym("word"), grammargen.Repeat(grammargen.Sym("letter")), grammargen.Str(";")))
	g.Define("word", grammargen.Pat("a|ab+c|abbb😀"))
	g.Define("letter", grammargen.Pat("[bcd😀😁]"))
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []struct {
		source              string
		offset, replacement uint8
	}{
		{"(abbbd;", 5, 'c'},
		{"(abbbc;", 5, 'd'},
		{"(a bbbd;", 6, 'c'},
		{"\xef\xbb\xbf(abbbd;", 8, 'c'},
		{"\xc2\xa0(abbbd;", 7, 'c'},
		{"(abb\xffd;", 4, 'b'},
		{"\n(abbbd;", 6, 'c'},
		{"(a" + strings.Repeat("b", 60) + "d;", 62, 'c'},
		{"(abbb😁;", 8, 0x80},
		{"(abbb😀;", 8, 0x81},
	} {
		f.Add([]byte(seed.source), seed.offset, seed.replacement)
	}
	f.Fuzz(func(t *testing.T, source []byte, offset, replacement uint8) {
		if len(source) == 0 || len(source) > 128 || int(offset) >= len(source) {
			t.Skip("outside the bounded edit domain")
		}
		// Columns count bytes. Preserve newline status so this one-byte edit
		// has the same old and new end point, including invalid UTF-8 input.
		if (source[offset] == '\n') != (replacement == '\n') {
			t.Skip("the replacement changes the edit's line point")
		}
		edited := append([]byte(nil), source...)
		edited[offset] = replacement
		start := gts.Point{}
		for _, b := range source[:int(offset)] {
			if b == '\n' {
				start.Row++
				start.Column = 0
			} else {
				start.Column++
			}
		}
		end := start
		if source[offset] == '\n' {
			end.Row++
			end.Column = 0
		} else {
			end.Column++
		}
		edit := gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset) + 1, NewEndByte: uint32(offset) + 1, StartPoint: start, OldEndPoint: end, NewEndPoint: end}
		for _, compact := range []bool{false, true} {
			func() {
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(compact)
				old, err := parser.Parse(source)
				if err != nil {
					t.Fatalf("initial compact=%t: %v", compact, err)
				}
				defer old.Release()
				old.Edit(edit)
				next, err := parser.ParseIncremental(edited, old)
				if err != nil {
					t.Fatalf("incremental compact=%t: %v", compact, err)
				}
				if next != old {
					defer next.Release()
				}
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(false)
				fresh, err := freshParser.Parse(edited)
				if err != nil {
					t.Fatalf("fresh: %v", err)
				}
				defer fresh.Release()
				actual, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if actual.SHA256 != want.SHA256 {
					t.Fatalf("compact=%t source=%q offset=%d replacement=%#x incremental=%s fresh=%s", compact, source, offset, replacement, next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
				}
			}()
		}
	})
}
