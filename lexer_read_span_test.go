package gotreesitter

import "testing"

func readSpanTestLanguage() *Language {
	return &Language{
		Name: "read_span", SymbolNames: []string{"end", "short"}, TokenCount: 2, SymbolCount: 2,
		LexModes: []LexMode{{LexState: 0}},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '=', Hi: '=', NextState: 1}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'i', Hi: 'i', NextState: 2}}},
			{Default: -1, EOF: -1},
		},
	}
}

func TestLexerReadSpanSurvivesSnapshotRollback(t *testing.T) {
	lang := readSpanTestLanguage()
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("=i")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	snapshot, ok := snapshotDFATokenSourceState(d)
	if !ok {
		t.Fatal("snapshot failed")
	}
	tok := d.lexer.Next(0)
	if tok.EndByte != 1 || d.tokenInvariantReadSpan() != 3 {
		t.Fatalf("short token=%+v span=%d", tok, d.tokenInvariantReadSpan())
	}
	restoreDFATokenSourceState(d, snapshot)
	if d.lexer.pos != 0 || d.tokenInvariantReadSpan() != 3 {
		t.Fatal("rollback erased a discarded scan's read span")
	}
	copyLexer := *d.lexer
	d.tokenInvariantMaxReadSpan = 0
	copyLexer.Next(0)
	if d.tokenInvariantReadSpan() != 3 {
		t.Fatal("copied lexer did not share the source aggregate")
	}
}

func TestLexerReadSpanFailedPrimitiveAndKeyword(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates[1].AcceptToken = 0
	lang.KeywordLexStates = lang.LexStates
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("=i")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	if _, ok := d.lexer.scan(0, 0, 0, 0); ok {
		t.Fatal("failed fixture accepted")
	}
	if d.tokenInvariantReadSpan() != 3 {
		t.Fatal("failed primitive lost its read span")
	}
	d.Reset([]byte("="))
	if d.tokenInvariantReadSpan() != 0 {
		t.Fatal("Reset retained the previous source's span")
	}
	if _, ok := d.lexKeywordSource([]byte("=i")); ok {
		t.Fatal("failed keyword fixture accepted")
	}
	if d.tokenInvariantReadSpan() != 3 {
		t.Fatal("failed bounded keyword scan lost its span")
	}
}

func TestLexerReadSpanSourceCloseAndPoolReset(t *testing.T) {
	lang := readSpanTestLanguage()
	lexer := NewLexer(lang.LexStates, []byte("=i"))
	d := newDFATokenSourceDirectWithCRecovery(lexer, lang, nil, nil, nil, nil, false)
	lexer.Next(0)
	d.Close()
	if lexer.tokenInvariantReadSpanMax != nil || d.tokenInvariantReadSpan() != 0 {
		t.Fatal("Close retained an aggregate owner")
	}
	resetPooledDFATokenSource(d)
	initDFATokenSourceWithCRecovery(d, lexer, lang, nil, nil, nil, nil, false)
	d.noPool = true
	defer d.Close()
	d.Reset(nil)
	d.lexer.Next(0)
	if d.tokenInvariantReadSpan() != 1 {
		t.Fatalf("fresh EOF span=%d", d.tokenInvariantReadSpan())
	}
}

type readSpanFailingScanner struct{}

func (readSpanFailingScanner) Create() any                      { return nil }
func (readSpanFailingScanner) Destroy(any)                      {}
func (readSpanFailingScanner) Serialize(any, []byte) int        { return 0 }
func (readSpanFailingScanner) Deserialize(any, []byte)          {}
func (readSpanFailingScanner) ExternalScannerIsStateless() bool { return true }
func (readSpanFailingScanner) Scan(_ any, l *ExternalLexer, _ []bool) bool {
	l.Advance(true)
	l.Advance(true)
	l.Lookahead()
	return false
}

func TestLexerReadSpanFailedExternalSkip(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.ExternalScanner = readSpanFailingScanner{}
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte(" ab")), lang, nil, nil, nil, nil, false)
	defer d.Close()
	el := newExternalLexer(d.lexer.source, 0, 0, 0)
	if d.runExternalScannerWithRetry(el, []bool{true}) {
		t.Fatal("failed scanner accepted")
	}
	if d.tokenInvariantReadSpan() != 3 {
		t.Fatalf("failed scanner span=%d, want 3 from its original start", d.tokenInvariantReadSpan())
	}
}

func TestLexerReadSpanFailedLegacyRelex(t *testing.T) {
	lang := readSpanTestLanguage()
	lang.LexStates[1].AcceptToken = 0
	p := &Parser{language: lang}
	var maximum uint32
	shared := Token{Symbol: 1, EndByte: 1, EndPoint: Point{Column: 1}}
	if _, ok := p.relexTokenForStackLexState([]byte("=i"), 0, shared, &maximum); ok {
		t.Fatal("failed relex unexpectedly selected a token")
	}
	if maximum != 3 || p.relexProbeLexer.tokenInvariantReadSpanMax != nil {
		t.Fatal("failed relex lost coverage or retained its aggregate owner")
	}
}
