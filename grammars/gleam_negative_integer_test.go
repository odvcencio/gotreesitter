package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestGleamNegativeIntegerLiteralMatchesC(t *testing.T) {
	lang := grammars.DetectLanguageByName("gleam").Language()

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{name: "adjacent minus", src: "{-1}", want: "(source_file (block (integer)))"},
		{name: "spaced minus", src: "{- 1}", want: "(source_file (block (integer_negation (integer))))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("parse %q produced an error: %s", tc.src, root.SExpr(lang))
			}
			if root.StartByte() != 0 || root.EndByte() != uint32(len(tc.src)) {
				t.Fatalf("parse %q covers [%d:%d], want [0:%d]", tc.src, root.StartByte(), root.EndByte(), len(tc.src))
			}
			if got := root.SExpr(lang); got != tc.want {
				t.Fatalf("parse %q: got %s, want %s", tc.src, got, tc.want)
			}
		})
	}

	t.Run("incremental edits", func(t *testing.T) {
		parser := gotreesitter.NewParser(lang)
		current, err := parser.Parse([]byte("{-1}"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { current.Release() }()
		for _, step := range []struct {
			src  string
			want string
			edit gotreesitter.InputEdit
		}{
			{src: "{- 1}", want: "(source_file (block (integer_negation (integer))))", edit: gotreesitter.InputEdit{
				StartByte: 2, OldEndByte: 2, NewEndByte: 3,
				StartPoint: gotreesitter.Point{Column: 2}, OldEndPoint: gotreesitter.Point{Column: 2}, NewEndPoint: gotreesitter.Point{Column: 3},
			}},
			{src: "{-1}", want: "(source_file (block (integer)))", edit: gotreesitter.InputEdit{
				StartByte: 2, OldEndByte: 3, NewEndByte: 2,
				StartPoint: gotreesitter.Point{Column: 2}, OldEndPoint: gotreesitter.Point{Column: 3}, NewEndPoint: gotreesitter.Point{Column: 2},
			}},
		} {
			current.Edit(step.edit)
			incremental, err := parser.ParseIncremental([]byte(step.src), current)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := gotreesitter.NewParser(lang).Parse([]byte(step.src))
			if err != nil {
				incremental.Release()
				t.Fatal(err)
			}
			root := incremental.RootNode()
			freshRoot := fresh.RootNode()
			// Check each edited result against the expected derivation as well
			// as a fresh parse; matching fresh trees alone can hide a shared bug.
			if root.HasError() || freshRoot.HasError() ||
				root.StartByte() != 0 || root.EndByte() != uint32(len(step.src)) ||
				freshRoot.StartByte() != 0 || freshRoot.EndByte() != uint32(len(step.src)) ||
				root.SExpr(lang) != freshRoot.SExpr(lang) || root.SExpr(lang) != step.want {
				t.Errorf("incremental %q: got %s, fresh %s, want %s", step.src, root.SExpr(lang), freshRoot.SExpr(lang), step.want)
			}
			fresh.Release()
			current.Release()
			current = incremental
		}
	})
}
