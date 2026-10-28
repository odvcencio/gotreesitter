package parse_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestTopLevelReplayDoesNotHideErrors pins the production route for languages
// outside the C recovery port. When a single stack dead-ends and only the
// degenerate top-level replay split fits, the parser used to re-attach the
// popped nodes as extras and publish a tree with no ERROR node, so HasError()
// was false where locked C reports an error. Each input below fails in locked
// C; cgo_harness/parity_replay_false_clean_test.go holds the C side.
func TestTopLevelReplayDoesNotHideErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang string
		src  string
	}{
		{"javascript_empty_fragment_then_number", "javascript", "<></>0"},
		{"javascript_nested_jsx_then_number", "javascript", "<A><A>AAAAA</A></A>0"},
		{"javascript_html_like_then_number", "javascript", "<html><body>Hello</body></html>0"},
		{"javascript_bang_between_empty_statements", "javascript", ";! ;A0"},
		{"html_stray_end_tag", "html", "<html><Y></0<html>"},
		{"html_stray_end_tag_text", "html", "<html><BodY>00000</0000<html>0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tc.lang)
			if entry == nil {
				t.Fatalf("%s is not registered", tc.lang)
			}
			lang := entry.Language()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if !root.HasError() {
				t.Fatalf("production tree has no error, locked C reports one: %s", root.SExpr(lang))
			}
			if !containsNodeTypeForTest(root, lang, "ERROR") {
				t.Fatalf("HasError() is true but no ERROR node exists: %s", root.SExpr(lang))
			}
		})
	}
}

// TestTopLevelReplayKeepsValidParsesClean checks that rejecting the degenerate
// replay split does not add errors to valid input, including the C++
// constructor-name-vs-declaration fork that the replay was written to rescue.
func TestTopLevelReplayKeepsValidParsesClean(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang string
		src  string
	}{
		{"cpp_extern_declaration", "cpp", "extern std::string foo();\nint main() { return 0; }\n"},
		{"cpp_class_with_semicolon", "cpp", "class C { int x; };\nint y;\n"},
		{"javascript_jsx_statement", "javascript", "<A><A>AAAAA</A></A>;\n0;\n"},
		{"javascript_comment_then_statement", "javascript", "// note\nlet x = 1;\n"},
		{"html_document", "html", "<html><body><p>text</p></body></html>\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tc.lang)
			if entry == nil {
				t.Fatalf("%s is not registered", tc.lang)
			}
			lang := entry.Language()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			defer tree.Release()
			if root := tree.RootNode(); root.HasError() {
				t.Fatalf("valid input reports an error: %s", root.SExpr(lang))
			}
		})
	}
}
