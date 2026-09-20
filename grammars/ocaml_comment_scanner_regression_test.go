package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// TestOCamlCommentCharLiteralInnerCloseMatchesUpstream proves the ported
// comment scanner (grammars/runtime/ocaml_scanner.go, ocamlScanComment) lets
// a `'` inside a comment probe forward for a character literal exactly as
// upstream's scan_character does, so an inner "*)" following an unclosed `'`
// ends the comment early instead of being skipped as an escaped character.
//
// tree-sitter-ocaml 3b2e14e0 (common/scanner.h) scans `'` as the start of a
// possible character literal. Since `'*)' ` never closes with a matching `'`,
// scan_character reports the probed `*` back to the caller for reprocessing,
// and the comment closes on the very next `)`. The C oracle for this input
// reports has_error=1 and a comment spanning only "(* c '*)".
func TestOCamlCommentCharLiteralInnerCloseMatchesUpstream(t *testing.T) {
	lang := OcamlLanguage()
	src := "(* c '*)' d *)\n"
	tree, err := ts.NewParser(lang).Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if !root.HasError() {
		t.Fatalf("expected has_error=1 (matching the C oracle), got a clean parse: %s", root.SExpr(lang))
	}
	comment := root.Child(0)
	if comment == nil || comment.Type(lang) != "comment" {
		t.Fatalf("expected a leading comment node: %s", root.SExpr(lang))
	}
	wantText := "(* c '*)"
	gotText := src[comment.StartByte():comment.EndByte()]
	if gotText != wantText {
		t.Fatalf("comment span = %q, want %q (upstream closes at the inner *)): %s", gotText, wantText, root.SExpr(lang))
	}
}

// TestOCamlCommentUnterminatedAtEOFMatchesUpstream proves the ported comment
// scanner closes an unterminated comment as a comment at EOF, matching
// upstream's `case '\0': if (eof(lexer)) return true;` (common/scanner.h,
// tree-sitter-ocaml 3b2e14e0), instead of misparsing the trailing text as an
// application expression.
//
// gotreesitter's ExternalLexer cannot distinguish a true embedded NUL byte
// from EOF (Lookahead returns 0 for both), so this port always treats
// lookahead==0 inside a comment as EOF, matching the C oracle's actual
// behavior for every real source file (a genuine embedded NUL mid-comment is
// not exercised by either oracle or this test).
func TestOCamlCommentUnterminatedAtEOFMatchesUpstream(t *testing.T) {
	lang := OcamlLanguage()
	src := "(* unterminated"
	tree, err := ts.NewParser(lang).Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected a clean parse (matching the C oracle), got has_error=1: %s", root.SExpr(lang))
	}
	if got, want := int(root.ChildCount()), 1; got != want {
		t.Fatalf("child count = %d, want %d: %s", got, want, root.SExpr(lang))
	}
	comment := root.Child(0)
	if comment == nil || comment.Type(lang) != "comment" {
		t.Fatalf("expected a single comment node spanning to EOF: %s", root.SExpr(lang))
	}
	gotText := src[comment.StartByte():comment.EndByte()]
	if gotText != src {
		t.Fatalf("comment span = %q, want the full source %q", gotText, src)
	}
}

// TestOCamlNestedCommentDepthDoesNotRecurse proves ocamlScanComment tracks
// nesting with a counter (upstream tree-sitter-ocaml#154) instead of Go call
// recursion, so a comment nested tens of thousands deep parses without
// growing the call stack. 20,000 matches the depth upstream's own test fixed
// a stack overflow at.
func TestOCamlNestedCommentDepthDoesNotRecurse(t *testing.T) {
	const depth = 20000
	src := make([]byte, 0, depth*2+8)
	for i := 0; i < depth; i++ {
		src = append(src, '(', '*')
	}
	src = append(src, ' ', 'x', ' ')
	for i := 0; i < depth; i++ {
		src = append(src, '*', ')')
	}

	lang := OcamlLanguage()
	tree, err := ts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("expected a clean parse for a balanced %d-deep nested comment", depth)
	}
	comment := root.Child(0)
	if comment == nil || comment.Type(lang) != "comment" {
		t.Fatalf("expected a single comment node: %s", root.SExpr(lang))
	}
	if got, want := int(comment.EndByte()-comment.StartByte()), len(src); got != want {
		t.Fatalf("comment span = %d bytes, want the full %d-byte source", got, want)
	}
}
