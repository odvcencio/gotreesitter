package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// TestRubyTopLevelModuleBoundsNativeRetiredDispatchArm is the R2 retirement
// receipt for dispatch.ruby (https://github.com/odvcencio/gotreesitter/blob/9c955f3d29b126973da657124438f7c0aeddce5b/docs/root-normalization-retirement.md,
// testdata/result_compat_ownership_v1.json).
// normalizeRubyTopLevelModuleBounds (parser_result_ruby.go, now deleted)
// shrank a top-level `module` node's end span back from a trailing-trivia
// boundary it shared with the enclosing `program` root. Root finalization
// now keeps `module`'s own span tightly bound to its `end` keyword while
// only the outer program/root span absorbs trailing trivia, so the shared
// end-byte precondition the deleted normalizer looked for never occurs on a
// real parse, matching the dispatcher census's zero-rewrite receipt over the
// real Ruby corpus (both censused files carry real top-level `module`
// blocks).
func TestRubyTopLevelModuleBoundsNativeRetiredDispatchArm(t *testing.T) {
	lang := RubyLanguage()
	const moduleEnd = uint32(len("module Foo\nend"))

	cases := []struct {
		name      string
		src       string
		wantStart uint32
		wantEnd   uint32
	}{
		{"trailing_blank_lines", "module Foo\nend\n\n\n", 0, moduleEnd},
		{"trailing_comment", "module Foo\nend\n# trailing\n", 0, moduleEnd},
		{"trailing_spaces", "module Foo\nend   ", 0, moduleEnd},
		{"no_trailing_content", "module Foo\nend", 0, moduleEnd},
		{"trailing_second_module", "module Foo\nend\nmodule Bar\nend\n", 0, moduleEnd},
		{
			"nested_module_body",
			"module Foo\n  module Bar\n    X = 1\n  end\nend\n\n",
			0,
			uint32(len("module Foo\n  module Bar\n    X = 1\n  end\nend")),
		},
		{"trailing_crlf", "module Foo\r\nend\r\n\r\n", 0, uint32(len("module Foo\r\nend"))},
		{"one_liner_semicolon", "module Foo; end\n", 0, uint32(len("module Foo; end"))},
		{
			"leading_comment",
			"# comment\nmodule Foo\nend\n",
			uint32(len("# comment\n")),
			uint32(len("# comment\nmodule Foo\nend")),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			tree, err := ts.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			root := tree.RootNode()
			if root.HasError() {
				t.Fatalf("expected clean parse: %s", root.SExpr(lang))
			}
			mod := findFirstNamedDescendantWhere(root, lang, "module", func(*ts.Node) bool { return true })
			if mod == nil {
				t.Fatalf("missing module node: %s", root.SExpr(lang))
			}
			if got := mod.StartByte(); got != tc.wantStart {
				t.Fatalf("module StartByte = %d, want %d: %s", got, tc.wantStart, root.SExpr(lang))
			}
			if got := mod.EndByte(); got != tc.wantEnd {
				t.Fatalf("module EndByte = %d, want %d: %s", got, tc.wantEnd, root.SExpr(lang))
			}
		})
	}
}

func TestRubyTopLevelModuleBoundsRetiredDispatchArmRoutes(t *testing.T) {
	lang := RubyLanguage()
	const source = "module Foo\nend"
	receipts := retiredDispatchRouteReceipts(t, lang, []byte(source))
	for _, receipt := range receipts {
		root := receipt.tree.RootNode()
		mod := findFirstNamedDescendantWhere(root, lang, "module", func(*ts.Node) bool { return true })
		if mod == nil {
			t.Fatalf("%s route is missing module: %s", receipt.name, root.SExpr(lang))
		}
		if mod.StartByte() != 0 || mod.EndByte() != uint32(len(source)) {
			t.Fatalf(
				"%s module bounds = %d:%d, want 0:%d",
				receipt.name,
				mod.StartByte(),
				mod.EndByte(),
				len(source),
			)
		}
	}
}
