package parserresult_test

import "testing"

func TestRubyTopLevelModuleDoesNotAbsorbLeadingCommentOrTrailingNewline(t *testing.T) {
	const src = "# frozen_string_literal: true\n\nmodule Rails\n  module Command\n    class VersionCommand < Base # :nodoc:\n    end\n  end\nend\n"
	tree, lang := parseByLanguageName(t, "ruby", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected ruby parse error: %s", root.SExpr(lang))
	}
	if got, want := root.ChildCount(), 2; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	mod := root.Child(1)
	if mod == nil || mod.Type(lang) != "module" {
		t.Fatalf("top-level module missing: %s", root.SExpr(lang))
	}
	if got, want := mod.StartByte(), uint32(31); got != want {
		t.Fatalf("module.StartByte = %d, want %d", got, want)
	}
	if got, want := mod.EndByte(), uint32(len(src)-1); got != want {
		t.Fatalf("module.EndByte = %d, want %d", got, want)
	}
}
