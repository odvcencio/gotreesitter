package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// A reuse parse that ends in trailing-EOF recovery leaves its ERROR wrappers
// beside the root. The root must report them; a clean, childless root hides
// the error from the incremental fallback, and the published tree differs from
// a fresh parse of the same bytes.
func TestCppIncrementalErrorExtrasKeepRootError(t *testing.T) {
	lang := CppLanguage()
	entry := DetectLanguage("x.cpp")
	cases := []struct {
		old, new string
		at       uint32
		registry bool
	}{
		{"l((l", "l()l", 2, false},
		{"l((l", "l()l", 2, true},
		{"<&&", "<&{", 2, true},
	}
	parse := func(src []byte, old *ts.Tree, registry bool) *ts.Tree {
		t.Helper()
		parser := ts.NewParser(lang)
		parser.SetTimeoutMicros(2_000_000)
		var tree *ts.Tree
		var err error
		switch {
		case registry && old != nil:
			tree, err = parser.ParseIncrementalWithTokenSource(src, old, entry.TokenSourceFactory(src, lang))
		case registry:
			tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		case old != nil:
			tree, err = parser.ParseIncremental(src, old)
		default:
			tree, err = parser.Parse(src)
		}
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		return tree
	}
	for _, tc := range cases {
		oldTree := parse([]byte(tc.old), nil, tc.registry)
		oldTree.Edit(ts.InputEdit{
			StartByte: tc.at, OldEndByte: tc.at + 1, NewEndByte: tc.at + 1,
			StartPoint: ts.Point{Column: tc.at}, OldEndPoint: ts.Point{Column: tc.at + 1}, NewEndPoint: ts.Point{Column: tc.at + 1},
		})
		inc := parse([]byte(tc.new), oldTree, tc.registry).RootNode()
		fresh := parse([]byte(tc.new), nil, tc.registry).RootNode()
		if got, want := inc.SExpr(lang), fresh.SExpr(lang); got != want || inc.HasError() != fresh.HasError() {
			t.Errorf("%q -> %q (registry=%v):\n incremental: %s has_error=%v\n fresh:       %s has_error=%v",
				tc.old, tc.new, tc.registry, got, inc.HasError(), want, fresh.HasError())
		}
	}
}
