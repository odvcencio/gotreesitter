package gotreesitter

import "testing"

type sameLineOutdentExternalScanner struct{}

func (sameLineOutdentExternalScanner) Create() any                           { return nil }
func (sameLineOutdentExternalScanner) Destroy(payload any)                   {}
func (sameLineOutdentExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (sameLineOutdentExternalScanner) Deserialize(payload any, buf []byte)   {}
func (sameLineOutdentExternalScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if len(validSymbols) == 0 || !validSymbols[0] || lexer.Lookahead() != ')' {
		return false
	}
	lexer.SetResultSymbol(2)
	return true
}

type newlineOutdentExternalScanner struct{}

func (newlineOutdentExternalScanner) Create() any                           { return nil }
func (newlineOutdentExternalScanner) Destroy(payload any)                   {}
func (newlineOutdentExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (newlineOutdentExternalScanner) Deserialize(payload any, buf []byte)   {}
func (newlineOutdentExternalScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\n' {
		lexer.Advance(true)
	}
	if len(validSymbols) == 0 || !validSymbols[0] || lexer.Lookahead() != ')' {
		return false
	}
	lexer.SetResultSymbol(2)
	return true
}

func buildLayoutExternalCompetitionLanguage(scanner ExternalScanner) *Language {
	return &Language{
		Name:        "layout_compete",
		SymbolNames: []string{"end", ")", "_outdent"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ")", Visible: true, Named: false},
			{Name: "_outdent", Visible: false, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{
				AcceptToken: 0,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 1, Skip: true},
					{Lo: '\n', Hi: '\n', NextState: 1, Skip: true},
					{Lo: ')', Hi: ')', NextState: 2},
				},
			},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 2},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
		ExternalScanner: scanner,
		ExternalSymbols: []Symbol{2},
	}
}

func buildSingleIdentifierLanguage() *Language {
	return &Language{
		Name:              "single_ident",
		SymbolCount:       3,
		TokenCount:        2,
		ProductionIDCount: 1,
		StateCount:        3,
		LargeStateCount:   3,
		InitialState:      0,
		SymbolNames:       []string{"end", "identifier", "source_file"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "source_file", Visible: true, Named: true},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 2, ChildCount: 1, ProductionID: 0}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			{0, 1, 2},
			{3, 0, 0},
			{4, 0, 0},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},
		LexStates: []LexState{
			{
				AcceptToken: 0,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'a', Hi: 'z', NextState: 1},
					{Lo: ' ', Hi: ' ', NextState: 2},
					{Lo: '\n', Hi: '\n', NextState: 2},
				},
			},
			{
				AcceptToken: 1,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 1}},
			},
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 2},
					{Lo: '\n', Hi: '\n', NextState: 2},
				},
			},
		},
	}
}

func TestParseWithoutDFALexerReturnsError(t *testing.T) {
	lang := &Language{Name: "no_dfa", InitialState: 1}
	parser := NewParser(lang)

	_, err := parser.Parse([]byte("anything"))
	if err == nil {
		t.Fatal("expected error for language without DFA lexer")
	}
}

func TestParseIncrementalWithoutDFALexerReturnsError(t *testing.T) {
	lang := &Language{Name: "no_dfa", InitialState: 1}
	parser := NewParser(lang)
	oldTree := NewTree(nil, []byte("old"), lang)

	_, err := parser.ParseIncremental([]byte("new"), oldTree)
	if err == nil {
		t.Fatal("expected error for language without DFA lexer")
	}
}

func TestParseWithIncompatibleLanguageVersionReturnsError(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LanguageVersion = RuntimeLanguageVersion + 1
	parser := NewParser(lang)

	_, err := parser.Parse([]byte("1+2"))
	if err == nil {
		t.Fatal("expected error for incompatible language version")
	}
}

func TestParseWithTokenSourceIncompatibleLanguageVersionReturnsError(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LanguageVersion = RuntimeLanguageVersion + 1
	parser := NewParser(lang)
	ts := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("1+2")),
		language:          lang,
		lookupActionIndex: parser.lookupActionIndex,
	}

	_, err := parser.ParseWithTokenSource([]byte("1+2"), ts)
	if err == nil {
		t.Fatal("expected error for incompatible language version")
	}
}

func TestParseIncrementalWithIncompatibleLanguageVersionReturnsError(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.LanguageVersion = RuntimeLanguageVersion + 1
	parser := NewParser(lang)
	oldTree := NewTree(nil, []byte("1+2"), lang)

	_, err := parser.ParseIncremental([]byte("1+3"), oldTree)
	if err == nil {
		t.Fatal("expected error for incompatible language version")
	}
}

func TestParseWithNilLanguageReturnsError(t *testing.T) {
	parser := &Parser{}

	_, err := parser.Parse([]byte("anything"))
	if err == nil {
		t.Fatal("expected error for nil language")
	}
	if err != ErrNoLanguage {
		t.Errorf("expected ErrNoLanguage, got: %v", err)
	}
}

func TestParseIncrementalWithNilLanguageReturnsError(t *testing.T) {
	parser := &Parser{}
	oldTree := NewTree(nil, []byte("old"), nil)

	_, err := parser.ParseIncremental([]byte("new"), oldTree)
	if err == nil {
		t.Fatal("expected error for nil language")
	}
	if err != ErrNoLanguage {
		t.Errorf("expected ErrNoLanguage, got: %v", err)
	}
}

func TestParseWithTokenSourceNilLanguageReturnsError(t *testing.T) {
	parser := &Parser{}

	_, err := parser.ParseWithTokenSource([]byte("anything"), nil)
	if err == nil {
		t.Fatal("expected error for nil language")
	}
	if err != ErrNoLanguage {
		t.Errorf("expected ErrNoLanguage, got: %v", err)
	}
}

func TestParseIncrementalWithTokenSourceNilLanguageReturnsError(t *testing.T) {
	parser := &Parser{}
	oldTree := NewTree(nil, []byte("old"), nil)

	_, err := parser.ParseIncrementalWithTokenSource([]byte("new"), oldTree, nil)
	if err == nil {
		t.Fatal("expected error for nil language")
	}
	if err != ErrNoLanguage {
		t.Errorf("expected ErrNoLanguage, got: %v", err)
	}
}

func TestAllowRepeatedZeroWidthExternalImplicitEndTag(t *testing.T) {
	lang := &Language{
		SymbolNames:     []string{"end", "_implicit_end_tag", "_other"},
		ExternalSymbols: []Symbol{1, 2},
	}
	d := &dfaTokenSource{language: lang}

	if !d.allowRepeatedZeroWidthExternalSymbol(1) {
		t.Fatal("expected _implicit_end_tag to be repeatable")
	}
	if d.allowRepeatedZeroWidthExternalSymbol(2) {
		t.Fatal("expected non-implicit external symbol to remain guarded")
	}
}

func TestTrackZeroWidthExternalRepeatableSymbolClearsLoopGuard(t *testing.T) {
	lang := &Language{
		SymbolNames:     []string{"end", "_implicit_end_tag"},
		ExternalSymbols: []Symbol{1},
	}
	d := &dfaTokenSource{
		language:     lang,
		lexer:        &Lexer{},
		state:        7,
		extZeroPos:   12,
		extZeroState: 7,
		extZeroTried: []bool{true},
	}

	d.trackZeroWidthExternalToken(Token{Symbol: 1, StartByte: 5, EndByte: 5})

	if got := d.extZeroPos; got != -1 {
		t.Fatalf("extZeroPos = %d, want -1", got)
	}
	if got := len(d.extZeroTried); got != 0 {
		t.Fatalf("len(extZeroTried) = %d, want 0", got)
	}
}

func TestNextTokenPrefersConsumingDFATokenOverSameLineZeroWidthLayoutExternal(t *testing.T) {
	lang := buildLayoutExternalCompetitionLanguage(sameLineOutdentExternalScanner{})
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte(")")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(1)

	tok := d.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextTokenKeepsZeroWidthLayoutExternalAfterSkippedNewline(t *testing.T) {
	lang := buildLayoutExternalCompetitionLanguage(newlineOutdentExternalScanner{})
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte("\n)")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(1)

	tok := d.Next()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(1); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}

	tok = d.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("second token symbol = %d, want %d", got, want)
	}
	if got, want := tok.StartByte, uint32(1); got != want {
		t.Fatalf("second token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("second token end = %d, want %d", got, want)
	}
}

func TestNextGLRUnionDFATokenPrefersVisibleTokenOnExactTie(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "]", "_special_character"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "]", Visible: true, Named: false},
			{Name: "_special_character", Visible: false, Named: true},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ']', Hi: ']', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ']', Hi: ']', NextState: 4}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 2},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 0, 1},
			{0, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("]")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok, ok := d.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned ok=false, want true")
	}
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
}

func TestNextGLRUnionDFATokenPrefersLongerTokenOverBroaderCoverage(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "_byte", "_pair"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "_byte", Visible: false, Named: false},
			{Name: "_pair", Visible: false, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '*', Hi: '*', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '/', Hi: '/', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '*', Hi: '*', NextState: 5}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 4},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 2},
			{0, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("*/")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok, ok := d.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned ok=false, want true")
	}
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenProbesAlternativeLexStateForLongerValidToken(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "_byte", "_pair"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "_byte", Visible: false, Named: false},
			{Name: "_pair", Visible: false, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 4}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'b', Hi: 'b', NextState: 5}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 3},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
			{0, 0, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte("ab")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenKeepsPrimaryTokenWhenAlternativeStartsEarlier(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "_byte", "_pair"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "_byte", Visible: false, Named: false},
			{Name: "_pair", Visible: false, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ' ', Hi: ' ', NextState: 2, Skip: true}, {Lo: 'a', Hi: 'a', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ' ', Hi: ' ', NextState: 2, Skip: true}, {Lo: 'a', Hi: 'a', NextState: 3}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ' ', Hi: ' ', NextState: 5}, {Lo: 'a', Hi: 'a', NextState: 6}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 6}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 4},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
			{0, 0, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte(" a")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(1); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenKeepsPrimaryTokenWhenAlternativeStartsLater(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", ":", "_identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ":", Visible: false, Named: false},
			{Name: "_identifier", Visible: false, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ':', Hi: ':', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{
				AcceptToken: 0,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ':', Hi: ':', NextState: 4, Skip: true},
					{Lo: 'a', Hi: 'z', NextState: 5},
				},
			},
			{
				AcceptToken: 0,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 5}},
			},
			{
				AcceptToken: 2,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 5}},
			},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 3},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 0, 1},
			{0, 0, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte(":ab")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenKeepsPrimaryInvisibleTokenOnExactTie(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "_narrow", "visible"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "_narrow", Visible: false, Named: false},
			{Name: "visible", Visible: true, Named: true},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 4}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 3},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
			{0, 0, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte("a")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(1)

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
}

func TestNextDFATokenDoesNotRecoverPastInvalidByte(t *testing.T) {
	lang := buildSingleIdentifierLanguage()
	parser := NewParser(lang)
	d := acquireDFATokenSource(NewLexer(lang.LexStates, []byte(":ab")), lang, parser.lookupActionIndex, parser.hasKeywordState)
	defer d.Close()
	d.SetParserState(lang.InitialState)

	tok := d.Next()
	if got, want := tok.Symbol, errorSymbol; got != want {
		t.Fatalf("token symbol = %d, want errorSymbol=%d", got, want)
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}

	tok = d.Next()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("second token symbol = %d, want %d", got, want)
	}
	if got, want := tok.StartByte, uint32(1); got != want {
		t.Fatalf("second token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(3); got != want {
		t.Fatalf("second token end = %d, want %d", got, want)
	}
}

func TestParseDoesNotAcceptCleanlyAfterDFASkipsInvalidByte(t *testing.T) {
	lang := buildSingleIdentifierLanguage()
	parser := NewParser(lang)

	tree, err := parser.Parse([]byte(":ab"))
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("root = nil, want error tree")
	}
	if !root.HasError() {
		t.Fatalf("root.HasError() = false, want true; sexpr=%s", root.SExpr(lang))
	}
}

func TestLexStateForStateUsesLayoutFallbackOnlyForLayoutEntryExternals(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "_cmd_layout_start", "_cmd_texp_start"},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 5, ExternalLexState: 1},
			{LexState: 7, ExternalLexState: 2},
		},
		ExternalSymbols: []Symbol{1, 2},
		ExternalLexStates: [][]bool{
			{false, false},
			{true, false},
			{false, true},
		},
		LayoutFallbackLexState:    11,
		HasLayoutFallbackLexState: true,
	}
	d := &dfaTokenSource{language: lang}

	if got, want := d.lexStateForState(1), uint16(11); got != want {
		t.Fatalf("lexStateForState(1) = %d, want %d", got, want)
	}
	if got, want := d.lexStateForState(2), uint16(7); got != want {
		t.Fatalf("lexStateForState(2) = %d, want %d", got, want)
	}
}

func TestNextDFATokenUsesFallbackForEarlierActionableTokenInExternalState(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "(", ",", "_cond_context"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "(", Visible: true},
			{Name: ",", Visible: true},
			{Name: "_cond_context"},
		},
		SymbolCount:               4,
		TokenCount:                4,
		StateCount:                3,
		LargeStateCount:           3,
		InitialState:              1,
		LayoutFallbackLexState:    2,
		HasLayoutFallbackLexState: true,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ',', Hi: ',', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '(', Hi: '(', NextState: 4}, {Lo: ',', Hi: ',', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1, ExternalLexState: 1},
			{LexState: 1},
		},
		ExternalSymbols: []Symbol{3},
		ExternalLexStates: [][]bool{
			{false},
			{true},
		},
		ParseTable: [][]uint16{
			{0, 0, 0, 0},
			{0, 1, 1, 0},
			{0, 1, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("(x,")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
}

func TestNextDFATokenKeepsPrimaryTokenOutsideExternalState(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "(", ",", "_cond_context"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "(", Visible: true},
			{Name: ",", Visible: true},
			{Name: "_cond_context"},
		},
		SymbolCount:               4,
		TokenCount:                4,
		StateCount:                3,
		LargeStateCount:           3,
		InitialState:              1,
		LayoutFallbackLexState:    2,
		HasLayoutFallbackLexState: true,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ',', Hi: ',', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '(', Hi: '(', NextState: 4}, {Lo: ',', Hi: ',', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 1},
		},
		ExternalSymbols: []Symbol{3},
		ExternalLexStates: [][]bool{
			{false},
			{true},
		},
		ParseTable: [][]uint16{
			{0, 0, 0, 0},
			{0, 1, 1, 0},
			{0, 1, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("(x,")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, errorSymbol; got != want {
		t.Fatalf("token symbol = %d, want errorSymbol=%d", got, want)
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(1); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenUsesFallbackForLongerSameStartTokenInExternalState(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "#", "#)", "_cond_context"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "#", Visible: true},
			{Name: "#)", Visible: true},
			{Name: "_cond_context"},
		},
		SymbolCount:               4,
		TokenCount:                4,
		StateCount:                2,
		LargeStateCount:           2,
		InitialState:              1,
		LayoutFallbackLexState:    2,
		HasLayoutFallbackLexState: true,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '#', Hi: '#', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '#', Hi: '#', NextState: 4}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: ')', Hi: ')', NextState: 5}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1, ExternalLexState: 1},
		},
		ExternalSymbols: []Symbol{3},
		ExternalLexStates: [][]bool{
			{false},
			{true},
		},
		ParseTable: [][]uint16{
			{0, 0, 0, 0},
			{0, 1, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("#)")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.EndByte, uint32(2); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}
