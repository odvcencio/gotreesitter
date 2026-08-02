package gotreesitter

import "testing"

func TestDFATokenSourceSkipToByteClampsBeforeNarrowing(t *testing.T) {
	source := []byte("a\nβ")
	wantByte := uint32(len(source))
	wantPoint := Point{Row: 1, Column: 2}

	for _, tc := range []struct {
		name string
		skip func(*dfaTokenSource) Token
	}{
		{name: "byte", skip: func(d *dfaTokenSource) Token { return d.SkipToByte(^uint32(0)) }},
		{name: "byte_with_point", skip: func(d *dfaTokenSource) Token {
			return d.SkipToByteWithPoint(^uint32(0), Point{Row: 99, Column: 99})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := &Language{LexModes: []LexMode{{}}, LexStates: []LexState{{}}}
			d := &dfaTokenSource{lexer: NewLexer(lang.LexStates, source), language: lang}
			tok := tc.skip(d)
			if tok.StartByte != wantByte || tok.EndByte != wantByte {
				t.Fatalf("EOF bytes = [%d,%d), want [%d,%d)", tok.StartByte, tok.EndByte, wantByte, wantByte)
			}
			if tok.StartPoint != wantPoint || tok.EndPoint != wantPoint {
				t.Fatalf("EOF points = %v-%v, want %v", tok.StartPoint, tok.EndPoint, wantPoint)
			}
			if got := d.lexer.pos; got != len(source) {
				t.Fatalf("lexer position = %d, want %d", got, len(source))
			}
		})
	}
}

func TestDFATokenSourceSkipToByteNilSafety(t *testing.T) {
	var nilSource *dfaTokenSource
	if tok := nilSource.SkipToByte(^uint32(0)); tok != (Token{}) {
		t.Fatalf("nil SkipToByte = %+v, want zero token", tok)
	}
	if tok := nilSource.SkipToByteWithPoint(^uint32(0), Point{}); tok != (Token{}) {
		t.Fatalf("nil SkipToByteWithPoint = %+v, want zero token", tok)
	}
	withoutLexer := &dfaTokenSource{}
	if tok := withoutLexer.SkipToByte(1); tok != (Token{}) {
		t.Fatalf("lexerless SkipToByte = %+v, want zero token", tok)
	}
	if tok := withoutLexer.SkipToByteWithPoint(1, Point{}); tok != (Token{}) {
		t.Fatalf("lexerless SkipToByteWithPoint = %+v, want zero token", tok)
	}
}

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

type rawTextExternalScanner struct{}

func (rawTextExternalScanner) Create() any                           { return nil }
func (rawTextExternalScanner) Destroy(payload any)                   {}
func (rawTextExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (rawTextExternalScanner) Deserialize(payload any, buf []byte)   {}
func (rawTextExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	for i := 0; i < 3 && lexer.Lookahead() != 0; i++ {
		lexer.Advance(false)
	}
	lexer.SetResultSymbol(Symbol(2))
	return true
}

type checkpointByteExternalScanner struct{}

func (checkpointByteExternalScanner) Create() any         { state := byte(0); return &state }
func (checkpointByteExternalScanner) Destroy(payload any) {}
func (checkpointByteExternalScanner) Serialize(payload any, buf []byte) int {
	state := *payload.(*byte)
	if state == 0xff || len(buf) == 0 {
		return 0
	}
	buf[0] = state
	return 1
}
func (checkpointByteExternalScanner) Deserialize(payload any, buf []byte) {
	state := payload.(*byte)
	*state = 0
	if len(buf) > 0 {
		*state = buf[0]
	}
}
func (checkpointByteExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	return false
}
func (checkpointByteExternalScanner) UsesExternalScannerCheckpoints() bool { return true }

func TestCanRelexCheckpointScannerRequiresRepresentableStartAndLiveState(t *testing.T) {
	scanner := checkpointByteExternalScanner{}
	lang := &Language{ExternalScanner: scanner}
	state := scanner.Create()
	ts := &dfaTokenSource{
		lexer:                      NewLexer(nil, []byte("x")),
		language:                   lang,
		hasExternalScanner:         true,
		usesExternalCheckpoints:    true,
		externalPayload:            state,
		lastExternalTokenValid:     true,
		lastExternalTokenStartByte: 0,
		lastExternalTokenEndByte:   1,
	}
	tok := Token{StartByte: 0, EndByte: 1}

	if ts.CanRelexFromTokenStart(tok) {
		t.Fatal("relex accepted an absent start checkpoint")
	}
	ts.externalTokenStart = []byte{0}
	if !ts.CanRelexFromTokenStart(tok) {
		t.Fatal("relex rejected representable start and live states")
	}
	*state.(*byte) = 0xff
	if ts.CanRelexFromTokenStart(tok) {
		t.Fatal("relex accepted an unrepresentable live state")
	}
}

func TestExternalScannerReuseAuthenticatesLookaheadStartState(t *testing.T) {
	scanner := checkpointByteExternalScanner{}
	state := scanner.Create()
	*state.(*byte) = 9 // live payload is already at the lookahead token's end
	ts := &dfaTokenSource{
		language:                   &Language{ExternalScanner: scanner},
		externalPayload:            state,
		lastExternalTokenValid:     true,
		lastExternalTokenStartByte: 12,
		externalTokenStart:         []byte{3},
	}
	if !ts.externalScannerStateAtLookaheadStartMatches([]byte{3}, 12) {
		t.Fatal("reuse boundary compared against live lookahead-end state instead of recorded start state")
	}
	if ts.externalScannerStateAtLookaheadStartMatches([]byte{4}, 12) {
		t.Fatal("reuse boundary accepted a different recorded lookahead-start state")
	}
	if ts.externalScannerStateAtLookaheadStartMatches([]byte{3}, 13) {
		t.Fatal("reuse boundary used a start snapshot from a different token")
	}
}

type skipPrefixExternalScanner struct{}

func (skipPrefixExternalScanner) Create() any                           { return nil }
func (skipPrefixExternalScanner) Destroy(payload any)                   {}
func (skipPrefixExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (skipPrefixExternalScanner) Deserialize(payload any, buf []byte)   {}
func (skipPrefixExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	for lexer.Lookahead() == 'x' {
		lexer.Advance(true)
	}
	if lexer.Lookahead() != 'T' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(Symbol(1))
	return true
}

func TestAfterWhitespaceLexModeIgnoresWhitespaceInsideExternalContent(t *testing.T) {
	mode := LexMode{}
	mode.SetLexStateIndex(5)
	mode.SetAfterWhitespaceLexStateIndex(9)
	lang := &Language{
		LexModes: []LexMode{{}, mode},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
			{Actions: []ParseAction{{Type: ParseActionShift, Extra: true}}},
			{Actions: []ParseAction{{Type: ParseActionShift, ExtraChain: true}}},
		},
	}
	actionIndex := uint16(1)
	d := &dfaTokenSource{
		lexer:                       &Lexer{source: []byte(" "), pos: 1},
		language:                    lang,
		state:                       1,
		lookupActionIndex:           func(StateID, Symbol) uint16 { return actionIndex },
		lexModeStarts:               lang.LexModeStarts(),
		lastExternalTokenStartByte:  0,
		lastExternalTokenEndByte:    1,
		lastExternalTokenValid:      true,
		externalTokenEndSameAsStart: false,
	}

	d.lastExternalTokenWasExtra = d.tokenIsExtraInAllActiveStates(1)
	if d.lastExternalTokenWasExtra {
		t.Fatal("ordinary external content classified as extra")
	}
	if got := d.lexStateForState(1); got != 5 {
		t.Fatalf("lex state after external content = %d, want base immediate-capable state 5", got)
	}

	actionIndex = 2
	d.lastExternalTokenWasExtra = d.tokenIsExtraInAllActiveStates(1)
	if !d.lastExternalTokenWasExtra {
		t.Fatal("external extra shift not classified as extra")
	}
	if got := d.lexStateForState(1); got != 9 {
		t.Fatalf("lex state after external extra = %d, want after-whitespace state 9", got)
	}

	actionIndex = 3
	d.lastExternalTokenWasExtra = d.tokenIsExtraInAllActiveStates(1)
	if !d.lastExternalTokenWasExtra {
		t.Fatal("external extra-chain shift not classified as extra")
	}
	if got := d.lexStateForState(1); got != 9 {
		t.Fatalf("lex state after external extra chain = %d, want after-whitespace state 9", got)
	}

	snapshot := d.snapshotRelexState()
	d.lastExternalTokenWasExtra = false
	d.externalTokenEndSameAsStart = true
	snapshot.restore(d)
	if !d.lastExternalTokenWasExtra || d.externalTokenEndSameAsStart {
		t.Fatalf("relex snapshot lost external-token provenance: extra=%t same_as_start=%t",
			d.lastExternalTokenWasExtra, d.externalTokenEndSameAsStart)
	}
}

func TestRelexExternalScannerTokenPreservesSkippedGapProvenance(t *testing.T) {
	source := []byte("xxxT")
	lang := &Language{
		Name:            "external-skip-relex-test",
		SymbolNames:     []string{"EOF", "external"},
		ExternalScanner: skipPrefixExternalScanner{},
		ExternalSymbols: []Symbol{1},
		ExternalLexStates: [][]bool{
			{true},
		},
		LexModes: []LexMode{
			{ExternalLexState: 0},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		if state == 0 && sym == 1 {
			return 1
		}
		return 0
	}

	ts := acquireDFATokenSource(NewLexer(nil, source), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(0)

	tok := ts.Next()
	if !tok.ExternalScannerToken {
		t.Fatalf("token ExternalScannerToken = false; token=%+v", tok)
	}
	if got, want := tok.StartByte, uint32(3); got != want {
		t.Fatalf("token start = %d, want %d; token=%+v", got, want, tok)
	}
	if got, want := tok.ExternalScannerStartByte, uint32(0); got != want {
		t.Fatalf("token scanner start = %d, want %d; token=%+v", got, want, tok)
	}

	relexed, ok := ts.RelexFromTokenStart(tok)
	if !ok {
		t.Fatal("RelexFromTokenStart returned false")
	}
	if got, want := relexed.StartByte, tok.StartByte; got != want {
		t.Fatalf("relexed start = %d, want %d; token=%+v", got, want, relexed)
	}
	if got, want := relexed.EndByte, tok.EndByte; got != want {
		t.Fatalf("relexed end = %d, want %d; token=%+v", got, want, relexed)
	}
	if got, want := relexed.ExternalScannerStartByte, tok.ExternalScannerStartByte; got != want {
		t.Fatalf("relexed scanner start = %d, want preserved %d; token=%+v", got, want, relexed)
	}

	stack := newGLRStack(0)
	if !realShiftGapIsParserPadding(source, &stack, relexed, 0) {
		t.Fatalf("relexed scanner-owned gap rejected for %q; token=%+v", source[:relexed.StartByte], relexed)
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

	ts := acquireDFATokenSource(NewLexer(nil, []byte("x")), lang, lookup, nil, nil, nil)
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

func TestNextExternalTokenUsesExternalValidMaskByState(t *testing.T) {
	lang := &Language{
		Name:            "test",
		SymbolNames:     []string{"EOF", "first", "second"},
		ExternalScanner: dualChoiceExternalScanner{},
		ExternalSymbols: []Symbol{1, 2},
	}
	lookup := func(StateID, Symbol) uint16 { return 0 }
	externalValidByState := [][]uint16{nil, []uint16{1}}
	externalValidMaskByState := buildExternalValidMaskByState(externalValidByState, len(lang.ExternalSymbols))

	ts := acquireDFATokenSource(NewLexer(nil, []byte("x")), lang, lookup, nil, externalValidByState, externalValidMaskByState)
	defer ts.Close()
	ts.SetParserState(1)

	tok, ok := ts.nextExternalToken()
	if !ok {
		t.Fatal("expected external token")
	}
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("external token symbol = %d, want %d", got, want)
	}
}

func TestNextExternalTokenPrefersMoreSpecificExternalCandidateBeforePrimaryTie(t *testing.T) {
	lang := &Language{
		Name:            "external-specificity-test",
		SymbolNames:     []string{"EOF", "/", "raw_text"},
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

	ts := acquireDFATokenSource(NewLexer(nil, []byte("/raw")), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(2)
	ts.SetGLRStates([]StateID{2, 1})

	scored, ok := ts.nextGLRScoredExternalToken([]StateID{2, 1})
	if !ok {
		t.Fatal("expected scored external token")
	}
	if got, want := scored.Symbol, Symbol(1); got != want {
		t.Fatalf("scored external token symbol = %d, want more specific %d", got, want)
	}
}

func TestNextTokenLetsSpecificGLRDFATokenBeatExternalSubset(t *testing.T) {
	lang := &Language{
		Name:            "external-dfa-arbitration-test",
		SymbolNames:     []string{"EOF", "/", "raw_text"},
		ExternalScanner: rawTextExternalScanner{},
		ExternalSymbols: []Symbol{2},
		ExternalLexStates: [][]bool{
			{false},
			{true},
		},
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '/', Hi: '/', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{},
			{LexState: 0, ExternalLexState: 1},
			{LexState: 1, ExternalLexState: 0},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		switch {
		case state == 1 && sym == 2:
			return 1
		case state == 2 && sym == 1:
			return 1
		default:
			return 0
		}
	}

	ts := acquireDFATokenSource(NewLexer(lang.LexStates, []byte("/if")), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(1)
	ts.SetGLRStates([]StateID{1, 2})

	tok := ts.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextTokenLetsSpecificGLRDFATokenBeatHigherSupportExternalWhenStateAcceptsBoth(t *testing.T) {
	lang := &Language{
		Name:            "external-dfa-overlap-arbitration-test",
		SymbolNames:     []string{"EOF", "/", "raw_text"},
		ExternalScanner: rawTextExternalScanner{},
		ExternalSymbols: []Symbol{2},
		ExternalLexStates: [][]bool{
			{false},
			{true},
		},
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '/', Hi: '/', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{},
			{LexState: 1, ExternalLexState: 1},
			{LexState: 0, ExternalLexState: 1},
		},
	}
	lookup := func(state StateID, sym Symbol) uint16 {
		switch {
		case state == 1 && (sym == 1 || sym == 2):
			return 1
		case state == 2 && sym == 2:
			return 1
		default:
			return 0
		}
	}

	ts := acquireDFATokenSource(NewLexer(lang.LexStates, []byte("/each")), lang, lookup, nil, nil, nil)
	defer ts.Close()
	ts.SetParserState(1)
	ts.SetGLRStates([]StateID{1, 2})

	tok := ts.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
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

type preservingCountingExternalScanner struct {
	serializeCalls *int
}

func (s preservingCountingExternalScanner) Create() any { return nil }
func (s preservingCountingExternalScanner) Destroy(any) {}
func (s preservingCountingExternalScanner) Serialize(any, []byte) int {
	*s.serializeCalls++
	return 0
}
func (s preservingCountingExternalScanner) Deserialize(any, []byte) {}
func (s preservingCountingExternalScanner) PreservesStateOnScanFailure() bool {
	return true
}
func (s preservingCountingExternalScanner) Scan(any, *ExternalLexer, []bool) bool {
	return true
}

func TestRunExternalScannerWithRetryDefersSnapshotForFailurePreservingScanner(t *testing.T) {
	serializeCalls := 0
	lang := &Language{
		Name:            "test",
		ExternalScanner: preservingCountingExternalScanner{serializeCalls: &serializeCalls},
	}
	ts := acquireDFATokenSource(NewLexer(nil, nil), lang, nil, nil, nil, nil)
	defer ts.Close()

	el := &ExternalLexer{}
	el.reset(nil, 0, 0, 0)
	if !ts.runExternalScannerWithRetry(el, []bool{true}) {
		t.Fatal("runExternalScannerWithRetry = false, want true")
	}
	if serializeCalls != 0 {
		t.Fatalf("Serialize calls = %d, want 0 for first-pass success", serializeCalls)
	}
}

func TestCaptureExternalScannerStateUsesIndependentReusableBuffers(t *testing.T) {
	lang := &Language{
		Name:            "test",
		ExternalScanner: byteStateExternalScanner{},
	}
	ts := acquireDFATokenSource(NewLexer(nil, nil), lang, nil, nil, nil, nil)
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

func TestLastExternalScannerCheckpointCanUseStartAsEndWithoutAliasingScratch(t *testing.T) {
	ts := &dfaTokenSource{
		externalTokenStart:          []byte{7},
		externalTokenEnd:            make([]byte, 0, externalScannerSerializationBufferSize),
		lastExternalTokenStartByte:  12,
		lastExternalTokenEndByte:    34,
		lastExternalTokenValid:      true,
		externalTokenEndSameAsStart: true,
	}

	cp, startByte, endByte, ok := ts.lastExternalScannerCheckpoint()
	if !ok {
		t.Fatal("lastExternalScannerCheckpoint ok = false, want true")
	}
	if startByte != 12 || endByte != 34 {
		t.Fatalf("checkpoint bytes = (%d, %d), want (12, 34)", startByte, endByte)
	}
	if len(cp.start) != 1 || cp.start[0] != 7 || len(cp.end) != 1 || cp.end[0] != 7 {
		t.Fatalf("checkpoint = start %v end %v, want both [7]", cp.start, cp.end)
	}

	ts.externalTokenEnd = append(ts.externalTokenEnd, 99)
	if got, want := ts.externalTokenStart[0], byte(7); got != want {
		t.Fatalf("externalTokenEnd scratch aliases start: start[0] = %d, want %d", got, want)
	}
}

func TestLastExternalScannerCheckpointRequiresBothEndpoints(t *testing.T) {
	for name, ts := range map[string]*dfaTokenSource{
		"missing start": {
			externalTokenEnd:       []byte{1},
			lastExternalTokenValid: true,
		},
		"missing end": {
			externalTokenStart:     []byte{1},
			lastExternalTokenValid: true,
		},
		"missing shared start": {
			lastExternalTokenValid:      true,
			externalTokenEndSameAsStart: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := ts.lastExternalScannerCheckpoint(); ok {
				t.Fatal("lastExternalScannerCheckpoint accepted an incomplete boundary")
			}
		})
	}
}

func TestResetPooledDFATokenSourcePreservesScannerScratch(t *testing.T) {
	lookup := func(StateID, Symbol) uint16 { return 0 }
	ts := &dfaTokenSource{
		language:                    &Language{Name: "old"},
		lookupActionIndex:           lookup,
		externalValid:               make([]bool, 3, 8),
		externalTokenStart:          make([]byte, 2, externalScannerSerializationBufferSize),
		externalTokenEnd:            make([]byte, 3, externalScannerSerializationBufferSize),
		externalSnapshot:            make([]byte, 4, externalScannerSerializationBufferSize),
		externalRetrySnap:           make([]byte, 5, externalScannerSerializationBufferSize),
		externalCompare:             make([]byte, 6, externalScannerSerializationBufferSize),
		maskedScratch:               make([]bool, 7, 9),
		extZeroTried:                make([]bool, 4, 10),
		extZeroPos:                  99,
		zeroWidthPos:                77,
		lastExternalTokenValid:      true,
		externalTokenEndSameAsStart: true,
		lastExternalTokenStartByte:  12,
		lastExternalTokenEndByte:    34,
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
	if ts.extZeroPos != -1 || ts.zeroWidthPos != -1 || ts.lastExternalTokenValid || ts.externalTokenEndSameAsStart {
		t.Fatalf("reset did not clear scanner state: extZeroPos=%d zeroWidthPos=%d lastValid=%t endSameStart=%t", ts.extZeroPos, ts.zeroWidthPos, ts.lastExternalTokenValid, ts.externalTokenEndSameAsStart)
	}
}

func TestResetPooledDFATokenSourcePreservesClearedGLRUnionOverflow(t *testing.T) {
	ts := &dfaTokenSource{noPool: true}
	ts.glrUnionScanScratch = ts.glrUnionScanInline[:0]
	retainedText := string(make([]byte, 64))
	for state := StateID(0); state < 9; state++ {
		ts.glrUnionScanScratch = append(ts.glrUnionScanScratch, glrUnionDFAScan{
			state: state,
			tok:   Token{Text: retainedText},
		})
	}
	if cap(ts.glrUnionScanScratch) <= len(ts.glrUnionScanInline) {
		t.Fatalf("test did not force overflow: cap=%d inline=%d", cap(ts.glrUnionScanScratch), len(ts.glrUnionScanInline))
	}

	overflowCap := cap(ts.glrUnionScanScratch)
	overflow := ts.glrUnionScanScratch[:overflowCap]
	overflowBase := &overflow[0]
	ts.Close()
	for i := range overflow {
		if overflow[i].tok.Text != "" {
			t.Fatalf("Close retained source text in overflow entry %d", i)
		}
	}

	resetPooledDFATokenSource(ts)
	if got := cap(ts.glrUnionScanScratch); got != overflowCap {
		t.Fatalf("reset overflow cap=%d want=%d", got, overflowCap)
	}
	if len(ts.glrUnionScanScratch) != 0 {
		t.Fatalf("reset overflow len=%d want=0", len(ts.glrUnionScanScratch))
	}
	if got := &ts.glrUnionScanScratch[:overflowCap][0]; got != overflowBase {
		t.Fatal("reset replaced GLR union overflow backing")
	}
	for i := range ts.glrUnionScanScratch[:overflowCap] {
		if ts.glrUnionScanScratch[:overflowCap][i].tok.Text != "" {
			t.Fatalf("reset restored source text in overflow entry %d", i)
		}
	}
}

func TestDFATokenSourceResetClearsScannerAndLexerState(t *testing.T) {
	lang := &Language{
		Name:            "test",
		ExternalScanner: byteStateExternalScanner{},
	}
	ts := acquireDFATokenSource(NewLexer(nil, []byte("abc")), lang, nil, nil, nil, nil)
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

func TestNextDFATokenSynthesizesGeneratedNULSentinelLookahead(t *testing.T) {
	lang := &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\x00", "}"},
		TokenCount:            3,
		SymbolCount:           3,
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: '}', Hi: '}', NextState: 1},
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
			{LexState: 0},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce}}},
		},
	}
	lookup := func(_ StateID, sym Symbol) uint16 {
		if sym == 1 {
			return 1
		}
		return 0
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("}")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if tok.StartByte != 0 || tok.EndByte != 0 {
		t.Fatalf("token span = %d..%d, want zero-width at 0", tok.StartByte, tok.EndByte)
	}
}

func TestNextDFATokenDoesNotSynthesizeGeneratedNULSentinelOverValidToken(t *testing.T) {
	lang := &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\x00", "}"},
		TokenCount:            3,
		SymbolCount:           3,
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: '}', Hi: '}', NextState: 1},
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
			{LexState: 0},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
		},
	}
	lookup := func(_ StateID, sym Symbol) uint16 {
		switch sym {
		case 1:
			return 1
		case 2:
			return 2
		default:
			return 0
		}
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("}")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "}"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
}

func TestNextDFATokenDoesNotSynthesizeGeneratedNULSentinelBeforeIdentifier(t *testing.T) {
	lang := &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\x00", "identifier"},
		TokenCount:            3,
		SymbolCount:           3,
		LexStates: []LexState{
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: 'a', Hi: 'z', NextState: 1},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'a', Hi: 'z', NextState: 1},
				},
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce}}},
		},
	}
	lookup := func(_ StateID, sym Symbol) uint16 {
		if sym == 1 {
			return 1
		}
		return 0
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("name")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "name"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
}

func generatedNULWhitespaceChoiceLanguage() *Language {
	return &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\x00", "{", "}"},
		TokenCount:            4,
		SymbolCount:           4,
		ZeroWidthTokens:       []bool{false, true, false, false},
		LexStates: []LexState{
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
			},
			{
				Default: -1,
				EOF:     -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 1, Skip: true},
					{Lo: '{', Hi: '{', NextState: 2},
					{Lo: '}', Hi: '}', NextState: 3},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
			{
				AcceptToken: 3,
				Default:     -1,
				EOF:         -1,
			},
		},
		LexModes: []LexMode{
			{LexState: 0, AfterWhitespaceLexState: 1},
			{LexState: 0, AfterWhitespaceLexState: 1},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
		},
	}
}

func TestNextDFATokenDoesNotPreferGeneratedNULSentinelBeforeWhitespaceBrace(t *testing.T) {
	lang := generatedNULWhitespaceChoiceLanguage()
	lookup := func(_ StateID, sym Symbol) uint16 {
		if sym >= 1 && sym <= 3 {
			return uint16(sym)
		}
		return 0
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte(" {")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "{"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if tok.StartByte != 1 || tok.EndByte != 2 {
		t.Fatalf("token span = %d..%d, want 1..2", tok.StartByte, tok.EndByte)
	}
}

func TestNextDFATokenKeepsGeneratedNULSentinelBeforeWhitespaceCloser(t *testing.T) {
	lang := generatedNULWhitespaceChoiceLanguage()
	lookup := func(_ StateID, sym Symbol) uint16 {
		if sym >= 1 && sym <= 3 {
			return uint16(sym)
		}
		return 0
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte(" }")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if tok.StartByte != 0 || tok.EndByte != 0 {
		t.Fatalf("token span = %d..%d, want zero-width at 0", tok.StartByte, tok.EndByte)
	}
}

func TestNextDFATokenKeepsGeneratedNULSentinelBeforeLineBreakToken(t *testing.T) {
	lang := generatedNULWhitespaceChoiceLanguage()
	lookup := func(_ StateID, sym Symbol) uint16 {
		if sym >= 1 && sym <= 3 {
			return uint16(sym)
		}
		return 0
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte("\n{")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if tok.StartByte != 0 || tok.EndByte != 0 {
		t.Fatalf("token span = %d..%d, want zero-width at 0", tok.StartByte, tok.EndByte)
	}
}

func generatedNULSameLexWhitespaceLanguage() *Language {
	return &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "\x00", "{", "}"},
		TokenCount:            4,
		SymbolCount:           4,
		ZeroWidthTokens:       []bool{false, true, false, false},
		LexStates: []LexState{
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 0, Skip: true},
					{Lo: '{', Hi: '{', NextState: 1},
					{Lo: '}', Hi: '}', NextState: 2},
				},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
			},
			{
				AcceptToken: 3,
				Default:     -1,
				EOF:         -1,
			},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 0}},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionReduce}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
			{Actions: []ParseAction{{Type: ParseActionShift}}},
		},
	}
}

func TestNextDFATokenDoesNotPreferRawGeneratedNULSentinelBeforeWhitespaceBrace(t *testing.T) {
	lang := generatedNULSameLexWhitespaceLanguage()
	lookup := func(_ StateID, sym Symbol) uint16 {
		if sym >= 1 && sym <= 3 {
			return uint16(sym)
		}
		return 0
	}
	d := newDFATokenSourceDirect(NewLexer(lang.LexStates, []byte(" {")), lang, lookup, nil, nil, nil)
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, "{"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if tok.StartByte != 1 || tok.EndByte != 2 {
		t.Fatalf("token span = %d..%d, want 1..2", tok.StartByte, tok.EndByte)
	}
}
