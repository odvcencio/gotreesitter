//go:build !grammar_subset

package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestAngularEmptyQuotedStringMultiline covers tree-sitter-angular@6a31043
// ("fix: empty quoted attribute values on multiline no longer brakes
// parsing"), part of the 38a8014ed545 grammar bump: a property binding
// written as `[property]=""` on its own line parses cleanly instead of
// producing an ERROR node, matching the upstream fix's own corpus case
// ("Empty attribute value multiline").
func TestAngularEmptyQuotedStringMultiline(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantSExpr string
	}{
		{
			name:      "multiline empty property binding",
			source:    "<span\n  [property]=\"\"\n></span>",
			wantSExpr: "(document (element (start_tag (tag_name) (attribute (property_binding (binding_name (identifier))))) (end_tag (tag_name))))",
		},
		{
			name:      "single-line empty property binding",
			source:    `<span [property]=""></span>`,
			wantSExpr: "(document (element (start_tag (tag_name) (attribute (property_binding (binding_name (identifier))))) (end_tag (tag_name))))",
		},
		{
			name:      "non-empty property binding still parses",
			source:    `<span [property]="value"></span>`,
			wantSExpr: "(document (element (start_tag (tag_name) (attribute (property_binding (binding_name (identifier)) (expression (identifier))))) (end_tag (tag_name))))",
		},
	}

	language := AngularLanguage()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(language).Parse([]byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)

			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("unexpected ERROR node: %s", root.SExpr(language))
			}
			if got := root.SExpr(language); got != test.wantSExpr {
				t.Fatalf("SExpr = %q, want %q", got, test.wantSExpr)
			}
		})
	}
}
