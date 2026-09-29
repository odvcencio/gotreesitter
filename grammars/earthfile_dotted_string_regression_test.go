package grammars_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestEarthfileDottedStringContinuation(t *testing.T) {
	lang := grammars.EarthfileLanguage()
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"DO..Y\n", "(source_file (block (do_command (string (unquoted_string)))))"},
		{"SAVE ARTIFACT.s .s\n", "(source_file (block (save_artifact_command (string (unquoted_string)) (string (unquoted_string)))))"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.HasError() || root.EndByte() != uint32(len(tc.source)) {
				t.Fatalf("incomplete or erroneous parse: %s", root.SExpr(lang))
			}
			if got := root.SExpr(lang); got != tc.want {
				t.Fatalf("tree = %s; want %s", got, tc.want)
			}
		})
	}
}
