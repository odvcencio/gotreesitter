package gotreesitter

import (
	"testing"
	"unicode/utf8"
)

type retryingExternalScanner struct{}

func (retryingExternalScanner) Create() any               { return nil }
func (retryingExternalScanner) Destroy(any)               {}
func (retryingExternalScanner) Serialize(any, []byte) int { return 0 }
func (retryingExternalScanner) Deserialize(any, []byte)   {}

func (retryingExternalScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if len(validSymbols) > 0 && validSymbols[0] {
		lexer.SetResultSymbol(Symbol(2))
		return false
	}
	if len(validSymbols) > 1 && validSymbols[1] {
		lexer.SetResultSymbol(Symbol(3))
		lexer.Advance(false)
		lexer.MarkEnd()
		return true
	}
	return false
}

func TestNextExternalTokenRetriesScannerAfterFailedPreferredCandidate(t *testing.T) {
	lang := &Language{
		Name:               "test",
		SymbolNames:        []string{"end", "x", "first_ext", "second_ext"},
		SymbolMetadata:     []SymbolMetadata{{Name: "end"}, {Name: "x", Visible: true, Named: true}, {Name: "first_ext"}, {Name: "second_ext"}},
		SymbolCount:        4,
		TokenCount:         4,
		ExternalTokenCount: 2,
		StateCount:         2,
		LargeStateCount:    2,
		LexStates:          []LexState{{Default: -1, EOF: -1}},
		LexModes:           []LexMode{{LexState: 0}, {LexState: 0, ExternalLexState: 1}},
		ExternalSymbols:    []Symbol{2, 3},
		ExternalLexStates: [][]bool{
			{false, false},
			{true, true},
		},
		ExternalScanner: retryingExternalScanner{},
	}
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("x")),
		language:          lang,
		state:             1,
		lookupActionIndex: func(StateID, Symbol) uint16 { return 1 },
	}

	tok, ok := d.nextExternalToken()
	if !ok {
		t.Fatal("nextExternalToken returned ok=false, want true")
	}
	if got, want := tok.Symbol, Symbol(3); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("StartByte = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("EndByte = %d, want %d", got, want)
	}
}

type frontierRetryExternalScanner struct{}

func (frontierRetryExternalScanner) Create() any               { return nil }
func (frontierRetryExternalScanner) Destroy(any)               {}
func (frontierRetryExternalScanner) Serialize(any, []byte) int { return 0 }
func (frontierRetryExternalScanner) Deserialize(any, []byte)   {}
func (frontierRetryExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) > 0 && valid[0] {
		// Leave the cursor on an invalid byte. C still records its current
		// position plus the four-byte invalid-code-point lookahead.
		if lexer.Lookahead() == utf8.RuneError {
			lexer.SetResultSymbol(Symbol(2))
		}
		return false
	}
	if len(valid) > 1 && valid[1] {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(Symbol(3))
		return true
	}
	return false
}

func TestExternalScannerRetryPreservesMaximumInvalidUTF8Frontier(t *testing.T) {
	lang := &Language{
		Name:               "test",
		SymbolNames:        []string{"end", "x", "first_ext", "second_ext"},
		SymbolCount:        4,
		TokenCount:         4,
		ExternalTokenCount: 2,
		StateCount:         2,
		LexStates:          []LexState{{Default: -1, EOF: -1}},
		LexModes:           []LexMode{{LexState: 0}, {LexState: 0, ExternalLexState: 1}},
		ExternalSymbols:    []Symbol{2, 3},
		ExternalLexStates:  [][]bool{{false, false}, {true, true}},
		ExternalScanner:    frontierRetryExternalScanner{},
	}
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte{0xff}),
		language:          lang,
		state:             1,
		lookupActionIndex: func(StateID, Symbol) uint16 { return 1 },
	}

	tok, ok := d.nextExternalToken()
	if !ok {
		t.Fatal("nextExternalToken returned ok=false, want true")
	}
	if got, want := tok.Symbol, Symbol(3); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tokenLookaheadEndByte(tok), uint32(5); got != want {
		t.Fatalf("lookahead end = %d, want failed-attempt invalid UTF-8 frontier %d", got, want)
	}
}

type frontierFailingExternalScanner struct{}

func (frontierFailingExternalScanner) Create() any               { return nil }
func (frontierFailingExternalScanner) Destroy(any)               {}
func (frontierFailingExternalScanner) Serialize(any, []byte) int { return 0 }
func (frontierFailingExternalScanner) Deserialize(any, []byte)   {}
func (frontierFailingExternalScanner) Scan(_ any, lexer *ExternalLexer, _ []bool) bool {
	for lexer.Lookahead() != 0 {
		lexer.Advance(false)
	}
	return false
}

func TestInternalTokenPreservesFailedExternalScannerFrontier(t *testing.T) {
	lang := &Language{
		Name:               "test",
		SymbolNames:        []string{"end", "x", "external"},
		SymbolCount:        3,
		TokenCount:         2,
		ExternalTokenCount: 1,
		StateCount:         1,
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 1}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes:          []LexMode{{LexState: 0, ExternalLexState: 0}},
		ExternalSymbols:   []Symbol{2},
		ExternalLexStates: [][]bool{{true}},
		ExternalScanner:   frontierFailingExternalScanner{},
	}
	d := &dfaTokenSource{
		lexer:              NewLexer(lang.LexStates, []byte("aXXX")),
		language:           lang,
		state:              0,
		lookupActionIndex:  func(StateID, Symbol) uint16 { return 1 },
		hasExternalScanner: true,
		hasExternalSymbols: true,
	}

	tok := d.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tokenLookaheadEndByte(tok), uint32(5); got != want {
		t.Fatalf("lookahead end = %d, want failed external frontier %d", got, want)
	}
}

func TestSyntheticExternalTokenCarriesResultingCursorFrontier(t *testing.T) {
	lang := &Language{
		Name:              "test",
		SymbolNames:       []string{"end", "_line_break"},
		SymbolCount:       2,
		TokenCount:        2,
		ExternalSymbols:   []Symbol{1},
		ExternalLexStates: [][]bool{{true}},
		LexModes:          []LexMode{{LexState: 0, ExternalLexState: 0}},
		LexStates:         []LexState{{Default: -1, EOF: -1}},
	}
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("\nx")),
		language:          lang,
		lookupActionIndex: func(StateID, Symbol) uint16 { return 1 },
	}
	initDFATokenSourceWithCRecovery(d, d.lexer, lang, d.lookupActionIndex, nil, nil, nil, false)

	tok := d.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d, want %d", got, want)
	}
	if got, want := tokenLookaheadEndByte(tok), uint32(2); got != want {
		t.Fatalf("lookahead end = %d, want synthetic cursor frontier %d", got, want)
	}
}
