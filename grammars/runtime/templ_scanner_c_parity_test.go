//go:build !grammar_subset || grammar_subset_templ

package grammarruntime

import "testing"

func templCFaithful(scanner TemplExternalScanner) *templState {
	st := scanner.Create().(*templState)
	st.SetCFaithful(true)
	return st
}

// TestTemplScanFallsThroughLikeC pins tree-sitter-templ src/scanner.c:317-345.
// C tries each valid token in turn (css property value, script block text,
// switch element text, element text) and goes on to the next when one returns
// false. A failed attempt may consume input, and the next attempt starts from
// there, as in C. The legacy mode keeps the base behaviour (the result of the
// first valid token), which the legacy parity suite depends on.
func TestTemplScanFallsThroughLikeC(t *testing.T) {
	scanner := TemplExternalScanner{}
	valid := make([]bool, templTokenCount)
	valid[templTokSwitchElemText] = true
	valid[templTokElemText] = true
	// In a switch, "default:" starts a statement, so the switch variant of the
	// element text declines after it has read the keyword. The plain variant
	// does not know that keyword; it goes on from the byte after it and takes
	// the rest of the text. The token still starts at byte 0, as in C.
	got := portScan(scanner, templCFaithful(scanner), "default:x", valid)
	if !got.ok || got.sym != templDefaultSymTable[templTokElemText] || got.start != 0 || got.end != 9 {
		t.Fatalf("Scan on \"default:x\" = %+v, want a plain element text token over bytes 0-9", got)
	}
	only := make([]bool, templTokenCount)
	only[templTokSwitchElemText] = true
	if got := portScan(scanner, templCFaithful(scanner), "default:x", only); got.ok {
		t.Fatalf("with only the switch variant valid the scan must decline, got %+v", got)
	}
	if got := portScan(scanner, scanner.Create(), "default:x", valid); got.ok {
		t.Fatalf("legacy mode: the first valid token's result is final, got %+v", got)
	}
}

// TestTemplKeywordHelperLeavesMismatchUnreadLikeC. With
// switch text and plain text valid, "i<x" gives switch text over "i" only
// (bytes 0-1) in C: the keyword helper compares before it consumes
// (scanner.c:62-71), so the "<" that does not continue the keyword "if " stays
// unread, ends the switch text and is a terminator. A helper that consumed the
// "<" into its buffer made the plain text run on to byte 3.
func TestTemplKeywordHelperLeavesMismatchUnreadLikeC(t *testing.T) {
	scanner := TemplExternalScanner{}
	valid := make([]bool, templTokenCount)
	valid[templTokSwitchElemText] = true
	valid[templTokElemText] = true
	got := portScan(scanner, templCFaithful(scanner), "i<x", valid)
	portWantScan(t, "i<x", got, true, templDefaultSymTable[templTokSwitchElemText], 1)
}

// TestTemplNulIsNotTheEndLikeC: C asks lexer->eof, so
// the NUL in "a\x00;" does not end the css value, which then ends at the ";"
// (scanner.c:124-130, the css value loop); the legacy mode stops at the NUL.
func TestTemplNulIsNotTheEndLikeC(t *testing.T) {
	scanner := TemplExternalScanner{}
	valid := make([]bool, templTokenCount)
	valid[templTokCssPropertyValue] = true
	got := portScan(scanner, templCFaithful(scanner), "a\x00;", valid)
	portWantScan(t, "css value over NUL", got, true, templDefaultSymTable[templTokCssPropertyValue], 2)
	if legacy := portScan(scanner, scanner.Create(), "a\x00;", valid); legacy.ok {
		t.Errorf("legacy mode: expected the NUL to end the scan, got %+v", legacy)
	}
}

// TestTemplWhitespaceIsCLocaleLikeC: the white space
// skipped before a token is the C locale's six characters. " x" gives
// text that starts at byte 0 in C (the no-break space is not white space).
func TestTemplWhitespaceIsCLocaleLikeC(t *testing.T) {
	scanner := TemplExternalScanner{}
	valid := make([]bool, templTokenCount)
	valid[templTokElemText] = true
	got := portScan(scanner, templCFaithful(scanner), " x", valid)
	if !got.ok || got.start != 0 {
		t.Fatalf("C-faithful: %+v, want text starting at byte 0", got)
	}
	legacy := portScan(scanner, scanner.Create(), " x", valid)
	if !legacy.ok || legacy.start != 2 {
		t.Fatalf("legacy: %+v, want the no-break space skipped (text from byte 2)", legacy)
	}
}
