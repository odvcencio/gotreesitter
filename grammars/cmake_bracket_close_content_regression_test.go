package grammars

import "testing"

// TestCmakeBracketArgumentContentEndingInCloseBracket pins the fix ported
// from upstream tree-sitter-cmake commit 3725810 ("fix: handle bracketed
// strings with `]` at the end of its content"), part of the 58993af75218
// grammar bump.
//
// The bracket argument "[=[a]]=]" has level 1 (one "="). Its content is "a]":
// a literal "]" sits immediately before the real close "]=]". Before the
// fix, the scanner's failed-close-match path consumed one extra byte
// unconditionally instead of re-examining the current lookahead, so it
// swallowed the close's opening "]" as ordinary content and never found a
// matching close. That produced an ERROR node instead of a clean
// bracket_argument.
func TestCmakeBracketArgumentContentEndingInCloseBracket(t *testing.T) {
	const src = "message([=[a]]=])"
	tree, lang := pinAssertShape(t, "cmake", src,
		"(source_file (normal_command (identifier) (argument_list (argument (bracket_argument (bracket_argument_open) (bracket_argument_content) (bracket_argument_close))))))")
	defer tree.Release()

	if tree.RootNode().HasError() {
		t.Fatal("cmake: unexpected error node")
	}
	if got := tree.RootNode().EndByte(); int(got) != len(src) {
		t.Fatalf("cmake: root EndByte=%d, want %d (source: %q)", got, len(src), src)
	}

	source := []byte(src)
	open := pinFind(tree.RootNode(), lang, "bracket_argument_open")
	content := pinFind(tree.RootNode(), lang, "bracket_argument_content")
	close_ := pinFind(tree.RootNode(), lang, "bracket_argument_close")
	if open == nil || content == nil || close_ == nil {
		t.Fatalf("cmake: missing bracket_argument child (open=%v content=%v close=%v)", open, content, close_)
	}
	if got := open.Text(source); got != "[=[" {
		t.Fatalf("cmake: bracket_argument_open text = %q, want %q", got, "[=[")
	}
	if got := content.Text(source); got != "a]" {
		t.Fatalf("cmake: bracket_argument_content text = %q, want %q", got, "a]")
	}
	if got := close_.Text(source); got != "]=]" {
		t.Fatalf("cmake: bracket_argument_close text = %q, want %q", got, "]=]")
	}
	// No byte between the three tokens is dropped or double-counted.
	if content.StartByte() != open.EndByte() || close_.StartByte() != content.EndByte() {
		t.Fatalf("cmake: bracket_argument tokens are not contiguous: open=[%d,%d) content=[%d,%d) close=[%d,%d)",
			open.StartByte(), open.EndByte(), content.StartByte(), content.EndByte(), close_.StartByte(), close_.EndByte())
	}
}
