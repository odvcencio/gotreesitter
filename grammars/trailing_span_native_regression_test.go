package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTrailingSpanNativeRetiredDispatchArms(t *testing.T) {
	for _, test := range trailingSpanRetirementCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := test.routedSource()
			tree, err := gotreesitter.NewParser(test.language).ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			test.assert(t, tree.RootNode(), test.language, source)
		})
	}
}

func TestTrailingSpanRetiredDispatchArmRoutes(t *testing.T) {
	for _, test := range trailingSpanRetirementCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := test.routedSource()
			for _, receipt := range retiredDispatchRouteReceipts(t, test.language, test.source) {
				t.Run(receipt.name, func(t *testing.T) {
					test.assert(t, receipt.tree.RootNode(), test.language, source)
					if receipt.name != "incremental" {
						return
					}
					profile := receipt.incrementalProfile
					if test.reuseUnsupportedReason != "" {
						if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != test.reuseUnsupportedReason {
							t.Fatalf("incremental reuse status = %+v", profile)
						}
						return
					}
					if test.expectDirtyRootInvalidation {
						if !profile.OldTreeReuseRoute || profile.ReuseUnsupported ||
							profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 ||
							profile.ReuseRejectDirty != 1 || profile.ReuseRejectAncestorDirtyBeforeEdit != 1 ||
							profile.ReuseRejectRootNonLeafChanged != 0 {
							t.Fatalf("incremental dirty-root invalidation status = %+v", profile)
						}
						return
					}
					if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
						t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
					}
				})
			}
		})
	}
}

type trailingSpanRetirementCase struct {
	name                        string
	source                      []byte
	language                    *gotreesitter.Language
	assert                      func(*testing.T, *gotreesitter.Node, *gotreesitter.Language, []byte)
	reuseUnsupportedReason      string
	expectDirtyRootInvalidation bool
}

func (test trailingSpanRetirementCase) routedSource() []byte {
	return append(append([]byte(nil), test.source...), '\n')
}

func trailingSpanRetirementCases() []trailingSpanRetirementCase {
	return []trailingSpanRetirementCase{
		{
			name:                   "caddy_sole_child_line_break",
			source:                 []byte(":8080 {\n}"),
			language:               CaddyLanguage(),
			assert:                 assertSoleChildOwnsTrailingLineBreak,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:                        "comment_hidden_trailing_extra",
			source:                      []byte("one line"),
			language:                    CommentLanguage(),
			assert:                      assertRootOwnsTrailingTriviaWithoutHiddenExtra,
			expectDirtyRootInvalidation: true,
		},
		{
			name:                   "fortran_statement_line_breaks",
			source:                 []byte("program hello\n  implicit none\nend program hello"),
			language:               FortranLanguage(),
			assert:                 assertFortranTrailingLineBreaks,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:                   "just_assignment_line_break",
			source:                 []byte("name := \"value\""),
			language:               JustLanguage(),
			assert:                 assertSoleChildOwnsTrailingLineBreak,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:                   "nginx_attribute_line_breaks",
			source:                 []byte("http {\n  server {\n    listen 80;\n    server_name example.com;\n  }\n}"),
			language:               NginxLanguage(),
			assert:                 assertNginxAttributeLineBreaks,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:                   "nim_child_excludes_trailing_line_break",
			source:                 []byte("echo \"hello\""),
			language:               NimLanguage(),
			assert:                 assertSoleChildExcludesTrailingLineBreak,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:     "pascal_program_excludes_trailing_line_break",
			source:   []byte("program hello;\nbegin\nend."),
			language: PascalLanguage(),
			assert:   assertSoleChildExcludesTrailingLineBreak,
		},
		{
			name:                   "pug_sole_child_line_break",
			source:                 []byte("p hello"),
			language:               PugLanguage(),
			assert:                 assertSoleChildOwnsTrailingLineBreak,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
		{
			name:                   "rst_section_excludes_trailing_line_break",
			source:                 []byte("Title\n====="),
			language:               RstLanguage(),
			assert:                 assertRootOwnsTrailingTriviaWithoutHiddenExtra,
			reuseUnsupportedReason: "external_scanner_unsupported",
		},
	}
}

func assertSoleChildOwnsTrailingLineBreak(
	t *testing.T,
	root *gotreesitter.Node,
	lang *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	if root == nil || root.HasError() || root.ChildCount() != 1 {
		t.Fatalf("unexpected root: %v", root)
	}
	child := root.Child(0)
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if got, want := child.EndByte(), root.EndByte(); got != want {
		t.Fatalf("%s end = %d, want %d", child.Type(lang), got, want)
	}
}

func assertSoleChildExcludesTrailingLineBreak(
	t *testing.T,
	root *gotreesitter.Node,
	lang *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	if root == nil || root.HasError() || root.ChildCount() != 1 {
		t.Fatalf("unexpected root: %v", root)
	}
	child := root.Child(0)
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	if got, want := child.EndByte(), uint32(len(source)-1); got != want {
		t.Fatalf("%s end = %d, want %d", child.Type(lang), got, want)
	}
}

func assertRootOwnsTrailingTriviaWithoutHiddenExtra(
	t *testing.T,
	root *gotreesitter.Node,
	lang *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	if root == nil || root.HasError() || root.ChildCount() == 0 {
		t.Fatalf("unexpected root: %v", root)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	last := root.Child(root.ChildCount() - 1)
	symbol := int(last.Symbol())
	if last.IsExtra() && symbol < len(lang.SymbolMetadata) && !lang.SymbolMetadata[symbol].Visible {
		t.Fatalf("root retained hidden trailing extra: %s", root.SExpr(lang))
	}
}

func assertFortranTrailingLineBreaks(
	t *testing.T,
	root *gotreesitter.Node,
	lang *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	assertSoleChildOwnsTrailingLineBreak(t, root, lang, source)
	program := root.Child(0)
	statement := program.Child(0)
	if statement == nil || statement.Type(lang) != "program_statement" {
		t.Fatalf("missing program_statement: %s", root.SExpr(lang))
	}
	if got, want := statement.EndByte(), uint32(14); got != want {
		t.Fatalf("program_statement end = %d, want %d", got, want)
	}
}

func assertNginxAttributeLineBreaks(
	t *testing.T,
	root *gotreesitter.Node,
	lang *gotreesitter.Language,
	source []byte,
) {
	t.Helper()
	assertSoleChildOwnsTrailingLineBreak(t, root, lang, source)
	wantEnds := []uint32{uint32(len(source)), 66, 33, 62}
	var gotEnds []uint32
	var visit func(*gotreesitter.Node)
	visit = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.Type(lang) == "attribute" {
			gotEnds = append(gotEnds, node.EndByte())
		}
		for index := 0; index < node.ChildCount(); index++ {
			visit(node.Child(index))
		}
	}
	visit(root)
	if len(gotEnds) != len(wantEnds) {
		t.Fatalf("attribute ends = %v, want %v", gotEnds, wantEnds)
	}
	for index := range wantEnds {
		if gotEnds[index] != wantEnds[index] {
			t.Fatalf("attribute ends = %v, want %v", gotEnds, wantEnds)
		}
	}
}
