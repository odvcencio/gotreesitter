package gotreesitter

import "testing"

type dualChoiceExternalScanner struct{}

func (dualChoiceExternalScanner) Create() any                           { return nil }
func (dualChoiceExternalScanner) Destroy(payload any)                   {}
func (dualChoiceExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (dualChoiceExternalScanner) Deserialize(payload any, buf []byte)   {}
func (dualChoiceExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	switch {
	case len(valid) > 0 && valid[0]:
		lexer.SetResultSymbol(Symbol(1))
		return true
	case len(valid) > 1 && valid[1]:
		lexer.SetResultSymbol(Symbol(2))
		return true
	default:
		return false
	}
}

func TestNextExternalTokenPrefersCandidateUsableByPrimaryState(t *testing.T) {
	lang := &Language{
		Name:            "bash",
		SymbolNames:     []string{"EOF", "first", "second"},
		ExternalScanner: dualChoiceExternalScanner{},
		ExternalSymbols: []Symbol{1, 2},
		ExternalLexStates: [][]bool{
			{false, false},
			{true, false},
			{false, true},
		},
		LexModes: []LexMode{
			{},
			{ExternalLexState: 1},
			{ExternalLexState: 2},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		switch {
		case state == 1 && sym == 1:
			return 1
		case state == 2 && sym == 2:
			return 1
		default:
			return 0
		}
	}

	ts := acquireDFATokenSource(NewLexer(nil, []byte("x")), lang, lookup, nil, nil)
	defer ts.Close()
	ts.SetParserState(2)
	ts.SetGLRStates([]StateID{2, 1})

	scored, ok := ts.nextGLRScoredExternalToken([]StateID{2, 1})
	if !ok {
		t.Fatal("expected scored external token")
	}
	if got, want := scored.Symbol, Symbol(2); got != want {
		t.Fatalf("scored external token symbol = %d, want %d", got, want)
	}

	tok, ok := ts.nextExternalToken()
	if !ok {
		t.Fatal("expected external token")
	}
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("external token symbol = %d, want %d", got, want)
	}
}

func TestBashGeneratedShellOperatorsDoNotRequireArithmeticContext(t *testing.T) {
	shellOps := []string{"|", "|&", "||", "&&", "<", ">", "<<", "<<-", ">>", "<<<", "&>", "&>>", "<&", ">&", "<&-", ">&-", ">|", ";", ";;"}
	for _, op := range shellOps {
		if bashGeneratedOperatorRequiresArithmeticContext(op) {
			t.Fatalf("operator %q requires arithmetic context, want shell context allowed", op)
		}
	}
	arithmeticOps := []string{"+", "-", "*", "/", "%", "**", "++", "--", "+=", "<<=", ">>=", "?", ":", ","}
	for _, op := range arithmeticOps {
		if !bashGeneratedOperatorRequiresArithmeticContext(op) {
			t.Fatalf("operator %q does not require arithmetic context", op)
		}
	}
}

func TestBashGeneratedSyntheticExternalLiteralDoesNotConsumeHereStringPrefix(t *testing.T) {
	lang := &Language{
		Name:                  "bash",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "<<"},
		ExternalSymbols:       []Symbol{1},
	}
	ts := &dfaTokenSource{
		lexer:           NewLexer(nil, []byte("<<< word")),
		language:        lang,
		isBash:          true,
		isBashGenerated: true,
	}
	if tok, ok := ts.bashGeneratedSyntheticExternalLiteral([]bool{true}); ok {
		t.Fatalf("synthetic token = %+v, want DFA to handle here-string prefix", tok)
	}
}

func TestNormalizeBashNewlineTokenSplitsBySymbolName(t *testing.T) {
	lang := &Language{
		Name:                  "bash",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\\n"},
	}
	ts := &dfaTokenSource{
		lexer:           NewLexer(nil, []byte("\n\nsed")),
		language:        lang,
		isBash:          true,
		isBashGenerated: true,
	}
	tok := Token{
		Symbol:     1,
		StartByte:  0,
		EndByte:    2,
		StartPoint: Point{},
		EndPoint:   Point{Row: 2, Column: 0},
		Text:       "\n\n",
	}
	got, endPos, endRow, endCol := ts.normalizeDFAToken(tok, 2, 2, 0)
	if got.EndByte != 1 || endPos != 1 || endRow != 1 || endCol != 0 || got.Text != "\n" {
		t.Fatalf("split newline token = %+v end=(%d,%d,%d), want single newline", got, endPos, endRow, endCol)
	}
}

func TestNormalizeBashGeneratedDFAOnlyNewlineToken(t *testing.T) {
	lang := &Language{
		Name:                  "bash",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\\n", "regex"},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		if sym == 1 {
			return 1
		}
		return 0
	}
	ts := &dfaTokenSource{
		lexer:             NewLexer(nil, []byte("\n\nsed")),
		language:          lang,
		lookupActionIndex: lookup,
		isBash:            true,
		isBashGenerated:   true,
	}
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    2,
		StartPoint: Point{},
		EndPoint:   Point{Row: 2, Column: 0},
		Text:       "\n\n",
	}
	got, endPos, endRow, endCol := ts.normalizeDFAToken(tok, 2, 2, 0)
	if got.Symbol != 1 || got.EndByte != 1 || endPos != 1 || endRow != 1 || endCol != 0 || got.Text != "\n" {
		t.Fatalf("normalized DFA newline = %+v end=(%d,%d,%d), want active newline", got, endPos, endRow, endCol)
	}
}

func TestAppendExternalLexStateForStateKeepsUniqueValidOrder(t *testing.T) {
	lang := &Language{
		ExternalLexStates: [][]bool{
			{false},
			{true},
			{false, true},
		},
		LexModes: []LexMode{
			{ExternalLexState: 1},
			{ExternalLexState: 2},
			{ExternalLexState: 1},
			{ExternalLexState: 99},
		},
	}

	var buf [4]int
	order := buf[:0]
	for _, st := range []StateID{0, 1, 2, 3, 4} {
		order = appendExternalLexStateForState(lang, order, st)
	}

	if got, want := len(order), 2; got != want {
		t.Fatalf("order len = %d, want %d: %v", got, want, order)
	}
	if order[0] != 1 || order[1] != 2 {
		t.Fatalf("order = %v, want [1 2]", order)
	}
}

type byteStateExternalScanner struct{}

func (byteStateExternalScanner) Create() any {
	state := byte(0)
	return &state
}

func (byteStateExternalScanner) Destroy(any) {}

func (byteStateExternalScanner) Serialize(payload any, buf []byte) int {
	if len(buf) == 0 {
		return 0
	}
	buf[0] = *payload.(*byte)
	return 1
}

func (byteStateExternalScanner) Deserialize(payload any, buf []byte) {
	state := payload.(*byte)
	if len(buf) == 0 {
		*state = 0
		return
	}
	*state = buf[0]
}

func (byteStateExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	return false
}

func TestCaptureExternalScannerStateUsesIndependentReusableBuffers(t *testing.T) {
	lang := &Language{
		Name:            "test",
		ExternalScanner: byteStateExternalScanner{},
	}
	ts := acquireDFATokenSource(NewLexer(nil, nil), lang, nil, nil, nil)
	defer ts.Close()

	state := ts.externalPayload.(*byte)
	*state = 7
	outer := ts.captureExternalScannerStateInto(&ts.externalSnapshot)
	if len(outer) != 1 || outer[0] != 7 {
		t.Fatalf("outer snapshot = %v, want [7]", outer)
	}

	*state = 9
	inner := ts.captureExternalScannerStateInto(&ts.externalRetrySnap)
	if len(inner) != 1 || inner[0] != 9 {
		t.Fatalf("inner snapshot = %v, want [9]", inner)
	}

	if len(outer) > 0 && len(inner) > 0 && &outer[0] == &inner[0] {
		t.Fatal("outer and inner snapshots share backing storage")
	}

	*state = 42
	ts.restoreExternalScannerState(outer)
	if got, want := *state, byte(7); got != want {
		t.Fatalf("restored outer state = %d, want %d", got, want)
	}
	ts.restoreExternalScannerState(inner)
	if got, want := *state, byte(9); got != want {
		t.Fatalf("restored inner state = %d, want %d", got, want)
	}
}

func TestResetPooledDFATokenSourcePreservesScannerScratch(t *testing.T) {
	lookup := func(StateID, Symbol) uint16 { return 0 }
	ts := &dfaTokenSource{
		language:                   &Language{Name: "old"},
		lookupActionIndex:          lookup,
		externalValid:              make([]bool, 3, 8),
		externalTokenStart:         make([]byte, 2, externalScannerSerializationBufferSize),
		externalTokenEnd:           make([]byte, 3, externalScannerSerializationBufferSize),
		externalSnapshot:           make([]byte, 4, externalScannerSerializationBufferSize),
		externalRetrySnap:          make([]byte, 5, externalScannerSerializationBufferSize),
		externalCompare:            make([]byte, 6, externalScannerSerializationBufferSize),
		maskedScratch:              make([]bool, 7, 9),
		extZeroTried:               make([]bool, 4, 10),
		extZeroPos:                 99,
		zeroWidthPos:               77,
		lastExternalTokenValid:     true,
		lastExternalTokenStartByte: 12,
		lastExternalTokenEndByte:   34,
	}

	resetPooledDFATokenSource(ts)

	if ts.language != nil || ts.lookupActionIndex != nil {
		t.Fatalf("reset retained parser wiring: lang=%v lookupSet=%t", ts.language, ts.lookupActionIndex != nil)
	}
	if got, want := cap(ts.externalValid), 8; got != want {
		t.Fatalf("externalValid cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.externalTokenStart), externalScannerSerializationBufferSize; got != want {
		t.Fatalf("externalTokenStart cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.externalTokenEnd), externalScannerSerializationBufferSize; got != want {
		t.Fatalf("externalTokenEnd cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.externalSnapshot), externalScannerSerializationBufferSize; got != want {
		t.Fatalf("externalSnapshot cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.externalRetrySnap), externalScannerSerializationBufferSize; got != want {
		t.Fatalf("externalRetrySnap cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.externalCompare), externalScannerSerializationBufferSize; got != want {
		t.Fatalf("externalCompare cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.maskedScratch), 9; got != want {
		t.Fatalf("maskedScratch cap = %d, want %d", got, want)
	}
	if got, want := cap(ts.extZeroTried), 10; got != want {
		t.Fatalf("extZeroTried cap = %d, want %d", got, want)
	}
	if len(ts.externalValid) != 0 || len(ts.externalTokenStart) != 0 || len(ts.externalTokenEnd) != 0 ||
		len(ts.externalSnapshot) != 0 || len(ts.externalRetrySnap) != 0 || len(ts.externalCompare) != 0 ||
		len(ts.maskedScratch) != 0 || len(ts.extZeroTried) != 0 {
		t.Fatalf("reset should keep scratch capacity with zero length: %+v", ts)
	}
	if ts.extZeroPos != -1 || ts.zeroWidthPos != -1 || ts.lastExternalTokenValid {
		t.Fatalf("reset did not clear scanner state: extZeroPos=%d zeroWidthPos=%d lastValid=%t", ts.extZeroPos, ts.zeroWidthPos, ts.lastExternalTokenValid)
	}
}

func TestDFATokenSourceResetClearsScannerAndLexerState(t *testing.T) {
	lang := &Language{
		Name:            "test",
		ExternalScanner: byteStateExternalScanner{},
	}
	ts := acquireDFATokenSource(NewLexer(nil, []byte("abc")), lang, nil, nil, nil)
	defer ts.Close()

	state := ts.externalPayload.(*byte)
	*state = 7
	ts.state = 12
	ts.glrStates = []StateID{1, 2}
	ts.externalValid = append(ts.externalValid, true, false)
	ts.extZeroTried = append(ts.extZeroTried, true)
	ts.extZeroPos = 9
	ts.extZeroState = 3
	ts.zeroWidthPos = 11
	ts.zeroWidthCount = 4
	ts.lexer.pos = 2
	ts.lexer.row = 3
	ts.lexer.col = 5

	ts.Reset([]byte("z"))

	if ts.lexer == nil {
		t.Fatal("Reset cleared lexer")
	}
	if got, want := ts.lexer.pos, 0; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
	if got, want := ts.lexer.row, uint32(0); got != want {
		t.Fatalf("lexer.row = %d, want %d", got, want)
	}
	if got, want := ts.lexer.col, uint32(0); got != want {
		t.Fatalf("lexer.col = %d, want %d", got, want)
	}
	if got, want := ts.lexer.source, []byte("z"); string(got) != string(want) {
		t.Fatalf("lexer.source = %q, want %q", got, want)
	}
	if got, want := ts.state, StateID(0); got != want {
		t.Fatalf("state = %d, want %d", got, want)
	}
	if got := len(ts.glrStates); got != 0 {
		t.Fatalf("len(glrStates) = %d, want 0", got)
	}
	if got := len(ts.externalValid); got != 0 {
		t.Fatalf("len(externalValid) = %d, want 0", got)
	}
	if got := len(ts.extZeroTried); got != 0 {
		t.Fatalf("len(extZeroTried) = %d, want 0", got)
	}
	if got, want := ts.extZeroPos, -1; got != want {
		t.Fatalf("extZeroPos = %d, want %d", got, want)
	}
	if got, want := ts.zeroWidthPos, -1; got != want {
		t.Fatalf("zeroWidthPos = %d, want %d", got, want)
	}
	if got, want := ts.zeroWidthCount, 0; got != want {
		t.Fatalf("zeroWidthCount = %d, want %d", got, want)
	}
	if got, want := *ts.externalPayload.(*byte), byte(0); got != want {
		t.Fatalf("externalPayload state = %d, want %d", got, want)
	}
}

func TestNextDFATokenUsesAfterWhitespaceLexState(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "base_word", "after_ws_word"},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
					{Lo: 'a', Hi: 'z', NextState: 1},
				},
			},
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 1}},
			},
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 2, Skip: true},
					{Lo: 'a', Hi: 'z', NextState: 3},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 3}},
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0, AfterWhitespaceLexState: 2},
		},
	}

	d := &dfaTokenSource{
		lexer:    NewLexer(lang.LexStates, []byte(" foo")),
		language: lang,
		state:    1,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol at whitespace = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "foo"; got != want {
		t.Fatalf("token text at whitespace = %q, want %q", got, want)
	}

	d.lexer = NewLexer(lang.LexStates, []byte(" foo"))
	d.lexer.pos = 1
	d.state = 1

	tok = d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol after whitespace = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "foo"; got != want {
		t.Fatalf("token text after whitespace = %q, want %q", got, want)
	}
}

func TestNextDFATokenAtWhitespacePrefersEarlierBaseLexStateToken(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "base_word", "after_ws_quote"},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
					{Lo: '"', Hi: '"', NextState: 1},
					{Lo: 'a', Hi: 'z', NextState: 2},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 2}},
			},
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 3, Skip: true},
					{Lo: '"', Hi: '"', NextState: 4},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0, AfterWhitespaceLexState: 3},
		},
	}

	d := &dfaTokenSource{
		lexer:    NewLexer(lang.LexStates, []byte(" from \"x\"")),
		language: lang,
		state:    1,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol at whitespace = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "from"; got != want {
		t.Fatalf("token text at whitespace = %q, want %q", got, want)
	}
}

func TestNextDFATokenAfterWhitespacePrefersEarlierBaseLexStateToken(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "base_word", "after_ws_quote"},
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
					{Lo: '"', Hi: '"', NextState: 1},
					{Lo: 'a', Hi: 'z', NextState: 2},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 2}},
			},
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 3, Skip: true},
					{Lo: '"', Hi: '"', NextState: 4},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0, AfterWhitespaceLexState: 3},
		},
	}

	d := &dfaTokenSource{
		lexer:    NewLexer(lang.LexStates, []byte(" from \"x\"")),
		language: lang,
		state:    1,
	}
	d.lexer.pos = 1

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol after whitespace = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "from"; got != want {
		t.Fatalf("token text after whitespace = %q, want %q", got, want)
	}
}

func TestNextDFATokenPrefersParserValidZeroWidthBaseToken(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "text", "newline"},
		ZeroWidthTokens: []bool{
			false,
			true,
			false,
		},
		LexStates: []LexState{
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: ' ', Hi: ' ', NextState: 1}},
			},
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: ' ', Hi: ' ', NextState: 1}},
			},
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0, AfterWhitespaceLexState: 2},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
		},
	}

	d := &dfaTokenSource{
		lexer:                   NewLexer(lang.LexStates, []byte(";\n")),
		language:                lang,
		state:                   1,
		hasZeroWidthTokens:      true,
		hasZeroWidthStartAccept: true,
		lookupActionIndex: func(_ StateID, sym Symbol) uint16 {
			if sym == 1 || sym == 2 {
				return 1
			}
			return 0
		},
	}
	d.lexer.zeroWidthTokens = lang.ZeroWidthTokens
	d.lexer.pos = 1

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol at whitespace boundary = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(1); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenPrefersParserValidZeroWidthStartAccept(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "text", "newline"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "text", Visible: true, Named: true},
			{Name: "newline"},
		},
		ZeroWidthTokens: []bool{
			false,
			true,
			false,
		},
		LexStates: []LexState{
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: '\n', Hi: '\n', NextState: 1}},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
		},
	}

	d := &dfaTokenSource{
		lexer:                   NewLexer(lang.LexStates, []byte("\n")),
		language:                lang,
		state:                   1,
		hasZeroWidthTokens:      true,
		hasZeroWidthStartAccept: true,
		lookupActionIndex: func(_ StateID, sym Symbol) uint16 {
			if sym == 1 || sym == 2 {
				return 1
			}
			return 0
		},
	}
	d.lexer.zeroWidthTokens = lang.ZeroWidthTokens

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol at zero-width start accept = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(0); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}
