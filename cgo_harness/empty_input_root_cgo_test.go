//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestParityEmptyInputRoot checks that Go returns the same root as tree-sitter
// C for empty and whitespace-only input. C always returns a root node. Before
// the fix, Go returned a Tree whose RootNode() was nil for these grammars and
// inputs (the zero-node accept path in buildResultFromNodes).
func TestParityEmptyInputRoot(t *testing.T) {
	// These pairs returned a nil root before the fix. Their whole root (shape,
	// span, points, error flag) must now equal C. Other pairs still log any
	// difference without failing: norg " " and xml " " differ from C for reasons
	// that are not about a missing root.
	formerNil := map[string]bool{
		"doxygen/": true, "doxygen/ ": true, "doxygen/\n": true,
		"tlaplus/": true, "tlaplus/ ": true, "tlaplus/\n": true,
		"markdown_inline/": true, "norg/": true, "xml/": true,
	}
	langs := []string{"doxygen", "markdown_inline", "norg", "tlaplus", "xml"}
	inputs := []string{"", " ", "\n"}
	for _, lang := range langs {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			cLang, err := ParityCLanguage(lang)
			if err != nil {
				t.Skipf("load C language %s: %v", lang, err)
			}
			entry, ok := parityEntriesByName[lang]
			if !ok {
				t.Fatalf("missing registry entry for %q", lang)
			}
			for _, input := range inputs {
				src := []byte(input)
				cp := sitter.NewParser()
				if err := cp.SetLanguage(cLang); err != nil {
					cp.Close()
					t.Fatalf("set C language %s: %v", lang, err)
				}
				cTree := cp.Parse(src, nil)
				cp.Close()
				cRoot := cTree.RootNode()
				cSexp := cRoot.ToSexp()
				cErr := cRoot.HasError()
				cStart, cEnd := cRoot.StartByte(), cRoot.EndByte()
				cStartPt, cEndPt := cRoot.StartPosition(), cRoot.EndPosition()
				cTree.Close()

				goLang := entry.Language()
				parser := gotreesitter.NewParser(goLang)
				var goTree *gotreesitter.Tree
				var perr error
				if entry.TokenSourceFactory != nil {
					goTree, perr = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, goLang))
				} else {
					goTree, perr = parser.Parse(src)
				}
				if perr != nil || goTree == nil {
					t.Fatalf("%s %q: go parse failed: tree=%v err=%v", lang, input, goTree, perr)
				}
				goRoot := goTree.RootNode()
				if goRoot == nil {
					goTree.Release()
					t.Errorf("%s %q: go root is nil, C root is %s", lang, input, cSexp)
					continue
				}
				goSexp := goRoot.SExpr(goLang)
				t.Logf("%s %q: C=%s [%d,%d] err=%t | Go=%s [%d,%d] err=%t", lang, input,
					cSexp, cStart, cEnd, cErr, goSexp, goRoot.StartByte(), goRoot.EndByte(), goRoot.HasError())
				strict := formerNil[lang+"/"+input]
				report := t.Errorf
				if !strict {
					report = t.Logf
				}
				if goSexp != cSexp {
					report("%s %q: sexp mismatch\n  go: %s\n  c:  %s", lang, input, goSexp, cSexp)
				}
				if goRoot.HasError() != cErr {
					report("%s %q: has_error go=%t c=%t", lang, input, goRoot.HasError(), cErr)
				}
				if goRoot.StartByte() != uint32(cStart) || goRoot.EndByte() != uint32(cEnd) {
					report("%s %q: span go=[%d,%d] c=[%d,%d]", lang, input, goRoot.StartByte(), goRoot.EndByte(), cStart, cEnd)
				}
				goStartPt, goEndPt := goRoot.StartPoint(), goRoot.EndPoint()
				if goStartPt.Row != uint32(cStartPt.Row) || goStartPt.Column != uint32(cStartPt.Column) ||
					goEndPt.Row != uint32(cEndPt.Row) || goEndPt.Column != uint32(cEndPt.Column) {
					report("%s %q: points go=%v..%v c=%v..%v", lang, input, goStartPt, goEndPt, cStartPt, cEndPt)
				}
				goTree.Release()
			}
		})
	}
}
