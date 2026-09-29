package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRstValidLayoutMatchesC(t *testing.T) {
	entry := grammars.DetectLanguageByName("rst")
	if entry == nil || entry.Language() == nil {
		t.Skip("rst unavailable in this build")
	}
	lang := entry.Language()
	tests := []struct {
		source string
		want   string
	}{
		{" t: ", "(document (block_quote (paragraph)))"},
		{`"): `, "(document (paragraph))"},
	}
	for _, tt := range tests {
		tree, err := gotreesitter.NewParser(lang).Parse([]byte(tt.source))
		if err != nil {
			t.Fatalf("%q: parse: %v", tt.source, err)
		}
		root := tree.RootNode()
		if got := root.SExpr(lang); got != tt.want || root.HasError() {
			t.Errorf("%q: got %s (has error %v), want %s", tt.source, got, root.HasError(), tt.want)
		}
	}
}
