//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestProductionReplayErrorMatchesLockedC checks that the production route
// reports the same HasError() result as locked C on inputs where the legacy
// top-level replay used to re-attach popped nodes as extras and publish a tree
// with no ERROR node (issue 1519 and the HTML nightly fuzz witness). Tree shape
// is not asserted: production recovery still differs from C in shape on these
// grammars, which stay outside the C recovery port.
func TestProductionReplayErrorMatchesLockedC(t *testing.T) {
	for _, tc := range []struct {
		name, language, source string
	}{
		{"javascript_empty_fragment_then_number", "javascript", "<></>0"},
		{"javascript_nested_jsx_then_number", "javascript", "<A><A>AAAAA</A></A>0"},
		{"javascript_html_like_then_number", "javascript", "<html><body>Hello</body></html>0"},
		{"javascript_bang_between_empty_statements", "javascript", ";! ;A0"},
		{"html_stray_end_tag", "html", "<html><Y></0<html>"},
		{"html_stray_end_tag_text", "html", "<html><BodY>00000</0000<html>0"},
		{"cpp_extern_declaration", "cpp", "extern std::string foo();\nint main() { return 0; }\n"},
		{"javascript_valid_jsx", "javascript", "<A><A>AAAAA</A></A>;\n0;\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tc.language)
			if entry == nil {
				t.Fatalf("%s is not registered", tc.language)
			}
			lang := entry.Language()
			oracle, err := COracleLanguage(tc.language)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(oracle); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse([]byte(tc.source), nil)
			defer cTree.Close()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			got, want := tree.RootNode().HasError(), cTree.RootNode().HasError()
			t.Logf("locked C: has_error=%t %s", want, cTree.RootNode().ToSexp())
			t.Logf("production: has_error=%t %s", got, tree.RootNode().SExpr(lang))
			if got != want {
				t.Fatalf("production HasError() = %t, locked C = %t", got, want)
			}
		})
	}
}
