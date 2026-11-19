package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestCppIncrementalTokenSourceRetryUsesFreshResult(t *testing.T) {
	lang := CppLanguage()
	factory := DetectLanguage("test.cpp").TokenSourceFactory
	for _, tc := range []struct {
		old, next string
		at        uint32
	}{
		{")&", "a&", 0},
		{"(x((", "lx((", 0},
		{";&;", ";&&", 2},
		{"{a<", "{}<", 1},
	} {
		t.Run(tc.next, func(t *testing.T) {
			oldSource, source := []byte(tc.old), []byte(tc.next)
			fresh, err := ts.NewParser(lang).ParseWithTokenSource(source, factory(source, lang))
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			for _, profiled := range []bool{false, true} {
				parser := ts.NewParser(lang)
				oldTree, err := parser.ParseWithTokenSource(oldSource, factory(oldSource, lang))
				if err != nil {
					t.Fatal(err)
				}
				defer oldTree.Release()
				oldTree.Edit(ts.InputEdit{
					StartByte: tc.at, OldEndByte: tc.at + 1, NewEndByte: tc.at + 1,
					StartPoint: ts.Point{Column: tc.at}, OldEndPoint: ts.Point{Column: tc.at + 1}, NewEndPoint: ts.Point{Column: tc.at + 1},
				})
				tokens := factory(source, lang)
				var got *ts.Tree
				if profiled {
					got, _, err = parser.ParseIncrementalWithTokenSourceProfiled(source, oldTree, tokens)
				} else {
					got, err = parser.ParseIncrementalWithTokenSource(source, oldTree, tokens)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer got.Release()
				inc, want := got.RootNode(), fresh.RootNode()
				if inc.SExpr(lang) != want.SExpr(lang) || inc.HasError() != want.HasError() ||
					inc.EndByte() != want.EndByte() || got.ParseStopReason() != fresh.ParseStopReason() {
					t.Errorf("profiled=%t incremental %s (%s), fresh %s (%s)",
						profiled, inc.SExpr(lang), got.ParseStopReason(), want.SExpr(lang), fresh.ParseStopReason())
				}
			}
		})
	}
}
