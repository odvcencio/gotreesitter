//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// tree-sitter-blade@b5291d1b (PR #133, "support scoped slots") taught the
// external scanner an X_SLOT tag type: a bare closing "</x-slot>" now
// matches any open "<x-slot:name>" tag, not just an exact "<x-slot>" or
// "<x-slot:name>" pair. Before the port, blade's Go scanner treated
// "<x-slot:name>" and the bare "</x-slot>" as different custom tag names
// (the same rule ordinary custom tags already get), so the bare close
// missed and the parser recorded an erroneous_end_tag where the upstream
// corpus (test/corpus/components.txt, "Scoped Slots") expects a clean
// end_tag.
//
// Every case is wrapped in a <div>, not a Blade component tag such as
// <x-alert>, to keep this test isolated from an unrelated, pre-existing GLR
// gap on this runtime: a mismatched end tag directly inside a
// custom-tag-typed container does not always recover into a clean
// erroneous_end_tag/end_tag pair, independent of scoped slots.
func TestBladeScopedSlotEndTagMatching(t *testing.T) {
	language := BladeLanguage()

	tests := []struct {
		name          string
		fragment      string
		wantErroneous bool
	}{
		{"bare_open_bare_close", "<x-slot>a</x-slot>", false},
		{"named_open_bare_close", "<x-slot:title>b</x-slot>", false},
		{"named_open_matching_named_close", "<x-slot:contents>c</x-slot:contents>", false},
		{"named_open_mismatched_named_close", "<x-slot:title>d</x-slot:other>", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("<div>" + test.fragment + "</div>")
			tree, err := gotreesitter.NewParser(language).Parse(source)
			if err != nil {
				t.Fatalf("parse %q: %v", source, err)
			}
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if root == nil {
				t.Fatal("root is nil")
			}
			if root.HasError() {
				t.Fatalf("%s: unexpected parse error, tree = %s", test.fragment, root.SExpr(language))
			}

			erroneous := findRecoveryActionMaterializationNode(root, language, "erroneous_end_tag")
			if test.wantErroneous {
				if erroneous == nil {
					t.Fatalf("%s: want an erroneous_end_tag, tree = %s", test.fragment, root.SExpr(language))
				}
			} else if erroneous != nil {
				t.Fatalf("%s: bare/matching close should not be erroneous, tree = %s", test.fragment, root.SExpr(language))
			}
		})
	}
}
