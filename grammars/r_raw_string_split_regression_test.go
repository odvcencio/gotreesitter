package grammars

import "testing"

// TestRRawStringContentEndingInClosingBracket pins the scanner split ported
// from upstream tree-sitter-r commit 3ee0e0a ("Split raw strings into
// open/content/close (#199)"), part of the 58a22794466c grammar bump.
//
// Before the split, the whole raw string body was one _raw_string_literal
// token. After it, the body is scanned in two passes: the external scanner
// first emits _raw_string_content up through a byte sequence that looks like
// the close, then a second call re-consumes that same sequence as
// _raw_string_close. `r"(a))"` has content "a)": a literal ")" sits right
// before the real close ")\"". A scanner that advanced past a failed close
// attempt unconditionally, instead of re-examining the lookahead, would
// swallow the close's opening ")" as content and never find a matching
// close.
func TestRRawStringContentEndingInClosingBracket(t *testing.T) {
	const src = `x <- r"(a))"`
	tree, lang := pinAssertShape(t, "r", src,
		"(program (binary_operator (identifier) (string (string_open) (string_content) (string_close))))")
	defer tree.Release()

	if tree.RootNode().HasError() {
		t.Fatal("r: unexpected error node")
	}
	if got := int(tree.RootNode().EndByte()); got != len(src) {
		t.Fatalf("r: root EndByte=%d, want %d (source: %q)", got, len(src), src)
	}

	source := []byte(src)
	open := pinFind(tree.RootNode(), lang, "string_open")
	content := pinFind(tree.RootNode(), lang, "string_content")
	close_ := pinFind(tree.RootNode(), lang, "string_close")
	if open == nil || content == nil || close_ == nil {
		t.Fatalf("r: missing string child (open=%v content=%v close=%v)", open, content, close_)
	}
	if got := open.Text(source); got != `r"(` {
		t.Fatalf("r: string_open text = %q, want %q", got, `r"(`)
	}
	if got := content.Text(source); got != "a)" {
		t.Fatalf("r: string_content text = %q, want %q", got, "a)")
	}
	if got := close_.Text(source); got != `)"` {
		t.Fatalf("r: string_close text = %q, want %q", got, `)"`)
	}
	// No byte between the three tokens is dropped or double-counted.
	if content.StartByte() != open.EndByte() || close_.StartByte() != content.EndByte() {
		t.Fatalf("r: string tokens are not contiguous: open=[%d,%d) content=[%d,%d) close=[%d,%d)",
			open.StartByte(), open.EndByte(), content.StartByte(), content.EndByte(), close_.StartByte(), close_.EndByte())
	}
}

// TestRRawStringEmptyBodyHasNoContentNode pins the empty-body case of the
// same split: `r"()"` has no content between its delimiters, and the
// scanner must not emit a zero-width string_content node, matching single-
// and double-quoted strings.
func TestRRawStringEmptyBodyHasNoContentNode(t *testing.T) {
	const src = `x <- r"()"`
	tree, lang := pinAssertShape(t, "r", src,
		"(program (binary_operator (identifier) (string (string_open) (string_close))))")
	defer tree.Release()

	if tree.RootNode().HasError() {
		t.Fatal("r: unexpected error node")
	}
	if content := pinFind(tree.RootNode(), lang, "string_content"); content != nil {
		t.Fatalf("r: unexpected string_content node for empty raw string body")
	}
}

// TestRElseKeywordNotConsumedFromLongerIdentifier pins the fix ported from
// upstream tree-sitter-r commit 40899e0 ("Don't consume `else` if it is part
// of a larger `identifier`" (#201)), part of the 58a22794466c grammar bump.
//
// Before the fix, the external scanner greedily matched the four bytes
// "else" as the ELSE token as soon as they appeared where an else clause
// could start, even when they were really the prefix of a longer identifier
// like "else_idx". That split "else_idx <- 1" into a stray ELSE token
// followed by an unparsable "_idx <- 1" tail. After the fix, the scanner
// declines to match when the next character continues an identifier, so
// "else_idx <- 1" lexes as an ordinary assignment.
func TestRElseKeywordNotConsumedFromLongerIdentifier(t *testing.T) {
	const src = "{\nif (TRUE) 1\nelse_idx <- 1\n}"
	tree, lang := pinAssertShape(t, "r", src,
		"(program (braced_expression (if_statement (true) (float)) (binary_operator (identifier) (float))))")
	defer tree.Release()

	if tree.RootNode().HasError() {
		t.Fatal("r: unexpected error node")
	}

	source := []byte(src)
	ident := pinFind(tree.RootNode(), lang, "identifier")
	if ident == nil {
		t.Fatal("r: missing identifier node")
	}
	if got := ident.Text(source); got != "else_idx" {
		t.Fatalf("r: identifier text = %q, want %q", got, "else_idx")
	}
}
