//go:build !grammar_subset || grammar_subset_rust

package grammarruntime

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

func TestRustCScannerStateAndRegistration(t *testing.T) {
	s := RustExternalScanner{}
	a := s.Create().(*rustScannerState)
	b := s.Create().(*rustScannerState)
	if a == b {
		t.Fatal("shared payload")
	}
	a.SetCFaithful(true)
	if !a.On() || b.On() {
		t.Fatal("mode shared")
	}
	a.openingHashCount = 255
	buf := make([]byte, 1)
	if s.Serialize(a, buf) != 1 || buf[0] != 255 {
		t.Fatal("serialization")
	}
	s.Deserialize(b, buf)
	if b.openingHashCount != 255 || !a.On() || b.On() {
		t.Fatal("state restore")
	}
	s.Deserialize(b, []byte{1, 2})
	if b.openingHashCount != 0 {
		t.Fatal("invalid length did not reset")
	}
	r, ok := scanmode.Lookup(s)
	if !ok || !r.NeedsMode {
		t.Fatal("C mode not registered")
	}
	s.Destroy(a)
	s.Destroy(b)
}

// These cases exercise differences between the pinned C scanner and the
// existing legacy mode: an embedded NUL is not EOF, the C locale classifies
// digits as ASCII, and line-doc content is scanned without a prefix check.
func TestRustCScannerLexicalBoundaries(t *testing.T) {
	scanner := RustExternalScanner{}
	valid := make([]bool, rustTokenCount)
	valid[rustTokStringContent] = true
	got := portScan(scanner, cFaithfulPayload(scanner), "a\x00b\"", valid)
	portWantScan(t, "NUL in string", got, true, rustDefaultSymTable[rustTokStringContent], 3)
	legacy := portScan(scanner, scanner.Create(), "a\x00b\"", valid)
	if legacy.ok {
		t.Fatal("legacy NUL behavior changed")
	}
	valid = make([]bool, rustTokenCount)
	valid[rustTokFloatLiteral] = true
	if got := portScan(scanner, cFaithfulPayload(scanner), "١.0", valid); got.ok {
		t.Fatal("non-ASCII digit accepted in C mode")
	}
	if got := portScan(scanner, scanner.Create(), "١.0", valid); !got.ok {
		t.Fatal("legacy Unicode digit behavior changed")
	}
	valid = make([]bool, rustTokenCount)
	valid[rustTokDocComment] = true
	if got := portScan(scanner, cFaithfulPayload(scanner), "body\n", valid); !got.ok {
		t.Fatal("C mode rejected line-doc body")
	}
	if got := portScan(scanner, scanner.Create(), "body\n", valid); got.ok {
		t.Fatal("legacy line-doc prefix behavior changed")
	}
}

// The C scanner stores the current lookahead in a char while scanning block
// comments, so U+012A has the same low byte as an ASCII asterisk.
func TestRustCScannerBlockCommentBoundaries(t *testing.T) {
	scanner := RustExternalScanner{}
	valid := make([]bool, rustTokenCount)
	valid[rustTokBlockCommentContent] = true
	for _, src := range []string{"abc*/", "abc\u012a/"} {
		got := portScan(scanner, cFaithfulPayload(scanner), src, valid)
		portWantScan(t, src, got, true, rustDefaultSymTable[rustTokBlockCommentContent], 3)
	}
	got := portScan(scanner, cFaithfulPayload(scanner), "", valid)
	portWantScan(t, "EOF", got, true, rustDefaultSymTable[rustTokBlockCommentContent], 0)
}
