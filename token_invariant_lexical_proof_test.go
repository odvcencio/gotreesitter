package gotreesitter

import (
	"bytes"
	"testing"
)

func TestTokenInvariantPrimitiveProofSharesEqualKeywordSlicesAcrossModes(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.KeywordCaptureToken = 1
	lang.LexModes = make([]LexMode, 400)
	lang.LexStates = make([]LexState, 401)
	for index := range lang.LexModes {
		lang.LexModes[index].SetLexStateIndex(uint32(index))
		lang.LexStates[index] = LexState{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 400}}}
	}
	lang.LexStates[400] = LexState{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 400}}}
	lang.KeywordLexStates = []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'i', Hi: 'i', NextState: 1}}},
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'f', Hi: 'f', NextState: 2}}},
		{Default: -1, EOF: -1, AcceptToken: 2},
	}
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("ab")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	edit := InputEdit{StartByte: 0, OldEndByte: 2, NewEndByte: 2, OldEndPoint: Point{Column: 2}, NewEndPoint: Point{Column: 2}}
	// Raw scans need 1,600 attempts. Repeating the same keyword slice
	// for every mode would exceed the unchanged 2,048-scan budget.
	if _, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("ab"), []byte("ac"), edit, 3); !ok {
		t.Fatal("duplicate keyword comparisons exhausted the primitive budget")
	}
}

func TestTokenInvariantPrimitiveProofUnreachableWideModeDoesNotInflateSpan(t *testing.T) {
	lang := primitiveProofDigitLanguage()
	lang.LexStates = append(lang.LexStates, LexState{AcceptToken: 1, Default: 2, EOF: -1})
	lang.LexModes = append(lang.LexModes, LexMode{LexState: 2})
	source := append([]byte("1"), bytes.Repeat([]byte(" "), 64)...)
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil, false)
	defer d.Close()
	edit := InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}}
	span := uint32(2)
	for _, replacement := range []byte{'2', '1', '2'} {
		edited := append([]byte(nil), source...)
		edited[0] = replacement
		nextSpan, ok := d.tokenInvariantPrimitiveEditsEquivalent(source, edited, edit, span)
		if !ok || nextSpan != span {
			t.Fatalf("unreachable mode inflated coverage: %d/%t", nextSpan, ok)
		}
		source = edited
	}
	budget := tokenInvariantPrimitiveBudget{bytes: 32768, scans: 2048}
	tok, _, _, ok := d.tokenInvariantProbeDFALimited(source, 0, Point{}, 2, &budget, 2)
	if !ok || tok.lexerLookaheadEndByte <= 2 {
		t.Fatal("proof cutoff did not exclude an impossible old scan")
	}
	budget = tokenInvariantPrimitiveBudget{bytes: 3, scans: 2048}
	if _, _, _, ok := d.tokenInvariantProbeDFALimited(source, 0, Point{}, 2, &budget, 2); ok {
		t.Fatal("work cutoff was mistaken for a complete proof cutoff")
	}
}

func TestTokenInvariantPrimitiveProofLeadingBOM(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates = []LexState{{AcceptToken: 1, Default: 0, EOF: -1}}
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("abcx")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	edit := InputEdit{StartByte: 0, OldEndByte: 3, NewEndByte: 3, OldEndPoint: Point{Column: 3}, NewEndPoint: Point{Column: 3}}
	if _, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("abcx"), []byte("\xef\xbb\xbfx"), edit, 5); ok {
		t.Fatal("equal broad DFA tokens hid a newly skipped BOM")
	}
}

func TestTokenInvariantPrimitiveProofWhitespacePrefixBoundary(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates = []LexState{{AcceptToken: 1, Default: 0, EOF: -1}}
	oldSource, newSource := []byte("\xc2\xa0x"), []byte("\xc2\xa1x")
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, oldSource), lang, nil, nil, nil, nil, false)
	defer d.Close()
	edit := InputEdit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartPoint: Point{Column: 1}, OldEndPoint: Point{Column: 2}, NewEndPoint: Point{Column: 2}}
	if !sourcePositionFollowsWhitespace(oldSource, 2) || sourcePositionFollowsWhitespace(newSource, 2) {
		t.Fatal("fixture lost its prefix whitespace distinction")
	}
	if _, ok := d.tokenInvariantPrimitiveEditsEquivalent(oldSource, newSource, edit, 4); ok {
		t.Fatal("changed whitespace gate beyond the edited origin was certified")
	}
	budget := tokenInvariantPrimitiveBudget{bytes: 32768, scans: 2048}
	if tokenInvariantWhitespaceGatesEquivalent(oldSource, newSource, edit, &budget) {
		t.Fatal("wrapper proof missed UTF-8 prefix lookbehind")
	}
}

func TestTokenInvariantPrimitiveProofAnonymousLiteralSignature(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.SymbolNames = []string{"end", "identifier", "foo"}
	lang.SymbolMetadata = []SymbolMetadata{{}, {Visible: true, Named: true}, {Visible: true}}
	lang.TokenCount, lang.SymbolCount = 3, 3
	lang.LexStates = []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 1}}},
		{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 1}}},
	}
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("foo")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	edit := InputEdit{StartByte: 0, OldEndByte: 3, NewEndByte: 3, OldEndPoint: Point{Column: 3}, NewEndPoint: Point{Column: 3}}
	if _, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("foo"), []byte("bar"), edit, 4); ok {
		t.Fatal("equal identifier DFA results hid a changed anonymous literal candidate")
	}
}

func primitiveProofDigitLanguage() *Language {
	lang := readSpanTestLanguage()
	lang.LexStates = []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '0', Hi: '9', NextState: 1}}},
		{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '0', Hi: '9', NextState: 1}}},
	}
	return lang
}

type primitiveProofSubsetScanner struct{ empty bool }

func (primitiveProofSubsetScanner) Create() any                      { return nil }
func (primitiveProofSubsetScanner) Destroy(any)                      {}
func (primitiveProofSubsetScanner) Serialize(any, []byte) int        { return 0 }
func (primitiveProofSubsetScanner) Deserialize(any, []byte)          {}
func (primitiveProofSubsetScanner) ExternalScannerIsStateless() bool { return true }
func (s primitiveProofSubsetScanner) Scan(_ any, l *ExternalLexer, valid []bool) bool {
	if len(valid) != 2 || valid[0] || valid[1] == s.empty {
		return false
	}
	if l.Lookahead() != '2' {
		return false
	}
	l.Advance(false)
	l.SetResultSymbol(2)
	return true
}

func TestTokenInvariantPrimitiveProofChecksRetrySubsetsAndEmptyMask(t *testing.T) {
	for _, empty := range []bool{false, true} {
		lang := primitiveProofDigitLanguage()
		lang.ExternalSymbols = []Symbol{2, 3}
		lang.ExternalLexStates = [][]bool{{true, true}}
		lang.ExternalScanner = primitiveProofSubsetScanner{empty: empty}
		d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("1")), lang, nil, nil, nil, nil, false)
		edit := InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}}
		_, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("1"), []byte("2"), edit, 2)
		d.Close()
		if ok {
			t.Fatalf("undeclared external mask difference accepted: empty=%t", empty)
		}
	}
}

func TestTokenInvariantPrimitiveProofKeywordPrefilterBoundary(t *testing.T) {
	lang := primitiveProofDigitLanguage()
	lang.LexStates[0].Transitions = []LexTransition{{Lo: '0', Hi: 'z', NextState: 1}}
	lang.LexStates[1].Transitions = []LexTransition{{Lo: '0', Hi: 'z', NextState: 1}}
	lang.KeywordCaptureToken = 1
	lang.KeywordLexStates = []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '1', Hi: '1', NextState: 1}}},
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}}},
		{Default: -1, EOF: -1, AcceptToken: 2},
	}
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("1b")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	if !lang.keywordLexCouldMatch([]byte("1b"), 0, 2) || lang.keywordLexCouldMatch([]byte("2b"), 0, 2) {
		t.Fatal("fixture lost its prefilter distinction")
	}
	if _, ok := d.lexKeywordSource([]byte("1b")); ok {
		t.Fatal("old bounded keyword unexpectedly accepted")
	}
	if _, ok := d.lexKeywordSource([]byte("2b")); ok {
		t.Fatal("new bounded keyword unexpectedly accepted")
	}
	edit := InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}}
	if _, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("1b"), []byte("2b"), edit, 3); ok {
		t.Fatal("equal failed keyword scans hid a changed prefilter")
	}
}

func TestTokenInvariantPrimitiveProofExternalMaskBudget(t *testing.T) {
	for _, count := range []int{8, 9} {
		lang := primitiveProofDigitLanguage()
		lang.ExternalSymbols = make([]Symbol, count)
		lang.ExternalLexStates = [][]bool{make([]bool, count)}
		lang.ExternalScanner = primitiveProofSubsetScanner{}
		d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("11111")), lang, nil, nil, nil, nil, false)
		// Every origin intersects the edit, so eight symbols require at
		// least five origins times 256 masks times two source versions.
		edit := InputEdit{StartByte: 0, OldEndByte: 5, NewEndByte: 5, OldEndPoint: Point{Column: 5}, NewEndPoint: Point{Column: 5}}
		_, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("11111"), []byte("22222"), edit, 6)
		d.Close()
		if ok {
			t.Fatalf("external mask work limit did not decline: symbols=%d", count)
		}
	}
}

func TestTokenInvariantPrimitiveProofRejectsEarlierLongerToken(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates[2].Transitions = []LexTransition{{Lo: 'c', Hi: 'c', NextState: 3}}
	lang.LexStates = append(lang.LexStates, LexState{AcceptToken: 2, Default: -1, EOF: -1})
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("=ix")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	d.lexer.Next(0)
	beforePos, beforeSpan := d.lexer.pos, d.tokenInvariantReadSpan()
	edit := InputEdit{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartPoint: Point{Column: 2}, OldEndPoint: Point{Column: 3}, NewEndPoint: Point{Column: 3}}
	if _, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("=ix"), []byte("=ic"), edit, beforeSpan); ok {
		t.Fatal("earlier token growth was certified")
	}
	if d.lexer.pos != beforePos || d.tokenInvariantReadSpan() != beforeSpan {
		t.Fatal("proof changed the active lexer")
	}
}

func TestTokenInvariantPrimitiveProofDigitControlAndBudget(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates = []LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '0', Hi: '9', NextState: 1}}},
		{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '0', Hi: '9', NextState: 1}}},
	}
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("1")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	edit := InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}}
	if span, ok := d.tokenInvariantPrimitiveEditsEquivalent([]byte("1"), []byte("2"), edit, 2); !ok || span != 2 {
		t.Fatalf("digit control=%d/%t", span, ok)
	}
	budget := tokenInvariantPrimitiveBudget{bytes: 1, scans: 1}
	if _, _, _, ok := d.tokenInvariantProbeDFA([]byte("111"), 0, Point{}, 0, &budget); ok {
		t.Fatal("artificial EOF certified an incomplete scan")
	}
}
