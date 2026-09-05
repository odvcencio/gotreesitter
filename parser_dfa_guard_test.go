package gotreesitter

import "testing"

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

func TestAllowRepeatedZeroWidthExternalGDScriptDedent(t *testing.T) {
	lang := &Language{
		Name:            "gdscript",
		SymbolNames:     []string{"end", "_dedent", "_other"},
		ExternalSymbols: []Symbol{1, 2},
	}
	d := &dfaTokenSource{language: lang}

	if !d.allowRepeatedZeroWidthExternalSymbol(1) {
		t.Fatal("expected gdscript _dedent to be repeatable")
	}
	if d.allowRepeatedZeroWidthExternalSymbol(2) {
		t.Fatal("expected non-dedent external symbol to remain guarded")
	}

	lang.Name = "other"
	if d.allowRepeatedZeroWidthExternalSymbol(1) {
		t.Fatal("expected _dedent to remain guarded for other languages")
	}
}

func TestAllowRepeatedZeroWidthExternalBendDedent(t *testing.T) {
	lang := &Language{
		Name:            "bend",
		SymbolNames:     []string{"end", "_dedent", "_other"},
		ExternalSymbols: []Symbol{1, 2},
	}
	d := &dfaTokenSource{language: lang}

	if !d.allowRepeatedZeroWidthExternalSymbol(1) {
		t.Fatal("expected Bend _dedent to be repeatable")
	}
	if d.allowRepeatedZeroWidthExternalSymbol(2) {
		t.Fatal("expected non-dedent external symbol to remain guarded")
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

func TestSuppressFortranPreprocDefineNewlineOnlyOnNonDefineLines(t *testing.T) {
	src := []byte("#ifdef X\n#define Y\n")
	lang := &Language{
		Name:        "fortran",
		SymbolNames: []string{"end", "_preproc_def_token1"},
	}
	d := &dfaTokenSource{
		language: lang,
		lexer:    NewLexer(nil, src),
	}

	if !d.shouldSuppressFortranPreprocDefineNewline(Token{
		Symbol:    1,
		StartByte: uint32(len("#ifdef X")),
		EndByte:   uint32(len("#ifdef X\n")),
	}) {
		t.Fatal("expected #ifdef line newline token to be suppressed")
	}

	defineStart := len("#ifdef X\n#define Y")
	if d.shouldSuppressFortranPreprocDefineNewline(Token{
		Symbol:    1,
		StartByte: uint32(defineStart),
		EndByte:   uint32(defineStart + 1),
	}) {
		t.Fatal("expected #define line newline token to be preserved")
	}
}

func TestExplicitLineBreakSymbolName(t *testing.T) {
	for _, name := range []string{"\n", "\r", "\r\n"} {
		if !isExplicitLineBreakSymbolName(name) {
			t.Fatalf("expected %q to be an explicit linebreak symbol", name)
		}
	}
	if isExplicitLineBreakSymbolName("_external_end_of_statement") {
		t.Fatal("external end-of-statement should not be treated as an explicit linebreak symbol")
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

func TestNextGLRUnionDFATokenPrefersPromotedKeywordOverCaptureToken(t *testing.T) {
	lang := &Language{
		SymbolNames:         []string{"end", "identifier", "kw_end"},
		SymbolCount:         3,
		TokenCount:          3,
		StateCount:          3,
		LargeStateCount:     3,
		InitialState:        1,
		KeywordCaptureToken: 1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 6}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 4}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 5}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 7}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 8}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		KeywordLexStates: []LexState{
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 1}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 2}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1, ReservedWordSetID: 1},
			{LexState: 2, ReservedWordSetID: 0},
		},
		ReservedWords:          []Symbol{0, 0, 2, 0},
		MaxReservedWordSetSize: 2,
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
			{0, 1, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("end")),
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
}

func TestNextGLRUnionDFATokenPrefersAnonymousVisibleKeywordOverNamedIdentifier(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "identifier", "kw_end"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "kw_end", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 6}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 4}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 5}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 7}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 8}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
			{0, 1, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("end")),
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
}

func TestNextGLRUnionDFATokenPrefersVisibleTokenOverHiddenFallback(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "identifier", "kw_end"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: false, Named: false},
			{Name: "kw_end", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 6}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 4}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 5}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 7}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 8}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
			{0, 1, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("end")),
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
}

func TestNextGLRUnionDFATokenPrefersHigherActionSpecificityOnSameLexeme(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "identifier", ">", "gt_template"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: ">", Visible: true, Named: false},
			{Name: ">", Visible: true, Named: false},
		},
		SymbolCount:     4,
		TokenCount:      4,
		StateCount:      5,
		LargeStateCount: 5,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 5}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 6}}},
			{AcceptToken: 0, Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 3, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
			{LexState: 0},
			{LexState: 0},
		},
		ParseTable: [][]uint16{
			{0, 0, 0, 0},
			{0, 0, 1, 0},
			{0, 0, 1, 2},
			{0, 0, 0, 0},
			{0, 0, 0, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{
				{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, DynamicPrecedence: 1},
				{Type: ParseActionReduce, Symbol: 1, ChildCount: 1},
			}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(">")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok, ok := d.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned ok=false, want true")
	}
	if got, want := tok.Symbol, Symbol(3); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
}

func TestNextDFATokenSplitsCompactCloseAnglesWhenOnlySingleCloseAngleHasAction(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"end", ">", ">>"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(">>x")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, ">"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if got, want := d.lexer.pos, 1; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
}

func TestNextDFATokenKeepsRightShiftWhenRightShiftHasAction(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"end", ">", ">>"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
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
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(">>x")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, ">>"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if got, want := d.lexer.pos, 2; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
}

func TestNextDFATokenSplitsCompactCloseAnglesForInternalRightShiftVariant(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"end", ">", "gt_gt_internal"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(">>")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, ">"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if got, want := d.lexer.pos, 1; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
}

// TestNextDFATokenKeepsRightShiftWhenDelimiterFollowsWithoutOpenAngle covers a
// case this test previously asserted the other way round.
//
// The old expectation was that `>>(` always narrows to `>`, on the reasoning
// that a delimiter after a close-angle run means generic closers. No unclosed
// '<' appears anywhere in the source here, so there is no generic argument list
// to close: the run is a signed right shift whose right operand is
// parenthesised. Splitting it produced `x = a >> (b)` parse failures across
// real TypeScript, which is how this was found — see
// grammars/typescript_shift_close_angle_regression_test.go.
//
// TestNextDFATokenSplitsCompactCloseAnglesWhenIdentifierFollowsAndRightShiftHasAction
// keeps the positive direction: its source carries `<T>` before the run, so the
// split still fires when a generic really is open.
func TestNextDFATokenKeepsRightShiftWhenDelimiterFollowsWithoutOpenAngle(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"end", ">", ">>"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
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
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(">>(x)")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, ">>"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if got, want := d.lexer.pos, 2; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
}

func TestNextDFATokenSplitsCompactCloseAnglesWhenIdentifierFollowsAndRightShiftHasAction(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"end", ">", ">>"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 1, ProductionID: 7}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("<T>>parse")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}
	d.lexer.pos = 2
	d.lexer.col = 2

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := tok.Text, ">"; got != want {
		t.Fatalf("token text = %q, want %q", got, want)
	}
	if got, want := d.lexer.pos, 3; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
}

func TestNextGLRUnionDFATokenPrefersLongerVisibleTokenOverShorterMorePopularPrefix(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "??", "?"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "??", Visible: true, Named: false},
			{Name: "?", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      4,
		LargeStateCount: 4,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '?', Hi: '?', NextState: 4}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '?', Hi: '?', NextState: 5}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '?', Hi: '?', NextState: 5}}},
			{AcceptToken: 2, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '?', Hi: '?', NextState: 6}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
			{LexState: 3},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
			{0, 0, 1},
			{0, 0, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("??")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2, 3},
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

func TestNextGLRUnionDFATokenPrefersStateLiteralOverIdentifierCapture(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "end", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "end", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 4}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 5}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 2},
			{0, 0, 2},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("end")),
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

func testCloseAngleFrontierLanguage() *Language {
	return &Language{
		Name: "cuda_probe",
		SymbolNames: []string{
			"end",
			">",
			">>",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      4,
		LargeStateCount: 4,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 4}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 5}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 2},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 0},
			{0, 1, 2},
			{0, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
}

func newCloseAngleFrontierTokenSource(lang *Language, states ...StateID) *dfaTokenSource {
	parser := NewParser(lang)
	return &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(">>")),
		language:          lang,
		state:             states[0],
		glrStates:         states,
		lookupActionIndex: parser.lookupActionIndex,
	}
}

func requireFrontierCandidate(t *testing.T, frontier tokenFrontier, sym Symbol) tokenCandidate {
	t.Helper()
	for _, cand := range frontier.Candidates {
		if cand.Tok.Symbol == sym {
			return cand
		}
	}
	t.Fatalf("frontier missing symbol %d; candidates=%#v", sym, frontier.Candidates)
	return tokenCandidate{}
}

func TestCollectGLRDFATokenFrontierKeepsCloseAngleAlternatives(t *testing.T) {
	lang := testCloseAngleFrontierLanguage()
	d := newCloseAngleFrontierTokenSource(lang, 1, 2)

	frontier, ok := d.PeekTokenFrontier(nil, nil)
	if !ok {
		t.Fatal("PeekTokenFrontier returned ok=false, want true")
	}
	if got, want := len(frontier.Candidates), 2; got != want {
		t.Fatalf("candidate count = %d, want %d: %#v", got, want, frontier.Candidates)
	}
	if got, want := frontier.StartByte, uint32(0); got != want {
		t.Fatalf("frontier start byte = %d, want %d", got, want)
	}
	if got, want := frontier.StartPoint, (Point{}); got != want {
		t.Fatalf("frontier start point = %#v, want %#v", got, want)
	}

	single := requireFrontierCandidate(t, frontier, 1)
	if got, want := single.Tok.EndByte, uint32(1); got != want {
		t.Fatalf("> end byte = %d, want %d", got, want)
	}
	if got, want := single.RouteMask, uint16(0b01); got != want {
		t.Fatalf("> route mask = %02b, want %02b", got, want)
	}

	double := requireFrontierCandidate(t, frontier, 2)
	if got, want := double.Tok.EndByte, uint32(2); got != want {
		t.Fatalf(">> end byte = %d, want %d", got, want)
	}
	if got, want := double.RouteMask, uint16(0b10); got != want {
		t.Fatalf(">> route mask = %02b, want %02b", got, want)
	}
	if got, want := d.lexer.pos, 0; got != want {
		t.Fatalf("lexer.pos after peek = %d, want %d", got, want)
	}
}

func TestCollectGLRDFATokenFrontierMergesEquivalentLexModes(t *testing.T) {
	lang := testCloseAngleFrontierLanguage()
	d := newCloseAngleFrontierTokenSource(lang, 1, 3, 2)

	frontier, ok := d.PeekTokenFrontier(nil, nil)
	if !ok {
		t.Fatal("PeekTokenFrontier returned ok=false, want true")
	}
	if got, want := len(frontier.Candidates), 2; got != want {
		t.Fatalf("candidate count = %d, want %d: %#v", got, want, frontier.Candidates)
	}

	single := requireFrontierCandidate(t, frontier, 1)
	if got, want := single.RouteMask, uint16(0b011); got != want {
		t.Fatalf("> route mask = %03b, want %03b", got, want)
	}
	if got, want := single.Origin, StateID(1); got != want {
		t.Fatalf("> origin = %d, want %d", got, want)
	}

	double := requireFrontierCandidate(t, frontier, 2)
	if got, want := double.RouteMask, uint16(0b100); got != want {
		t.Fatalf(">> route mask = %03b, want %03b", got, want)
	}
}

func TestCollectGLRDFATokenFrontierKeepsNoLookaheadEOFToken(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		SymbolCount:     2,
		TokenCount:      2,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexStateID: noLookaheadLexState},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0},
			{1, 0},
			{0, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("x")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	frontier, ok := d.PeekTokenFrontier(nil, nil)
	if !ok {
		t.Fatal("PeekTokenFrontier returned ok=false, want true")
	}
	eof := requireFrontierCandidate(t, frontier, 0)
	if !eof.Tok.NoLookahead {
		t.Fatalf("EOF candidate NoLookahead = false, want true: %#v", eof.Tok)
	}
	if got, want := eof.RouteMask, uint16(0b01); got != want {
		t.Fatalf("EOF route mask = %02b, want %02b", got, want)
	}
	if got, want := eof.Tok.EndByte, uint32(0); got != want {
		t.Fatalf("EOF end byte = %d, want %d", got, want)
	}

	ident := requireFrontierCandidate(t, frontier, 1)
	if got, want := ident.RouteMask, uint16(0b10); got != want {
		t.Fatalf("identifier route mask = %02b, want %02b", got, want)
	}
}

func TestNextGLRUnionDFATokenPrefersComposableCloseAngle(t *testing.T) {
	lang := testCloseAngleFrontierLanguage()
	d := newCloseAngleFrontierTokenSource(lang, 1, 2)

	tok, ok := d.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned ok=false, want true")
	}
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
	if got, want := d.lexer.pos, 1; got != want {
		t.Fatalf("lexer.pos = %d, want %d", got, want)
	}
}

func TestCompareAngleTokenPreferenceCoversWideCloseRuns(t *testing.T) {
	lang := &Language{
		Name:        "typescript",
		SymbolNames: []string{"end", ">", ">>", ">>>", ">="},
	}
	d := &dfaTokenSource{language: lang}
	for _, wider := range []Symbol{2, 3} {
		if got := d.compareAngleTokenPreference(Token{Symbol: 1}, Token{Symbol: wider}); got != 1 {
			t.Fatalf("> versus %q preference = %d, want 1", lang.SymbolNames[wider], got)
		}
		if got := d.compareAngleTokenPreference(Token{Symbol: wider}, Token{Symbol: 1}); got != -1 {
			t.Fatalf("%q versus > preference = %d, want -1", lang.SymbolNames[wider], got)
		}
	}
	if got := d.compareAngleTokenPreference(Token{Symbol: 1}, Token{Symbol: 4}); got != 0 {
		t.Fatalf("> versus >= preference = %d, want 0", got)
	}
}

func TestContextualActionDefersUnsignedShiftLineageAfterSingleCloseElection(t *testing.T) {
	lang := &Language{
		Name:        "artifact_probe",
		SymbolNames: []string{"end", ">", ">>>"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 4}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
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
	}
	parser := NewParser(lang)
	tok := Token{
		Symbol:     1,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Column: 1},
	}
	if !parser.shouldDeferContextualCloseAngleAction([]byte(">>>"), 1, tok) {
		t.Fatal("unsigned-shift lineage accepted the elected single close angle")
	}
}

// TestTokenMaybeContextualCloseAngleShapeGate pins tokenMaybeContextualCloseAngle,
// the cheap, state-free shape pre-check extracted from
// deferContextualCloseAngleAction's own first guard (finding F9): a
// narrow, same-row, single ">" token passes; anything else fails.
func TestTokenMaybeContextualCloseAngleShapeGate(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ">", ">>"}}
	sameRowNarrowClose := Token{
		Symbol:     1,
		StartByte:  4,
		EndByte:    5,
		StartPoint: Point{Row: 2, Column: 1},
		EndPoint:   Point{Row: 2, Column: 2},
	}

	if !tokenMaybeContextualCloseAngle(lang, sameRowNarrowClose) {
		t.Fatal("a same-row, single-byte \">\" token must pass the shape gate")
	}
	if tokenMaybeContextualCloseAngle(nil, sameRowNarrowClose) {
		t.Fatal("a nil language must fail the shape gate")
	}
	if tokenMaybeContextualCloseAngle(lang, Token{Symbol: 99, StartByte: 0, EndByte: 1}) {
		t.Fatal("a symbol outside the language's SymbolNames must fail the shape gate")
	}
	wideClose := sameRowNarrowClose
	wideClose.Symbol = 2 // ">>"
	if tokenMaybeContextualCloseAngle(lang, wideClose) {
		t.Fatal("a token whose symbol name is not exactly \">\" must fail the shape gate")
	}
	wideSpan := sameRowNarrowClose
	wideSpan.EndByte = sameRowNarrowClose.StartByte + 2
	if tokenMaybeContextualCloseAngle(lang, wideSpan) {
		t.Fatal("a \">\" token wider than one byte must fail the shape gate")
	}
	crossRow := sameRowNarrowClose
	crossRow.EndPoint.Row++
	if tokenMaybeContextualCloseAngle(lang, crossRow) {
		t.Fatal("a \">\" token crossing a line must fail the shape gate")
	}
}

// TestDeferContextualCloseAngleActionSharesShapeGateWithPreCheck pins the
// single-source-of-truth property finding F9 requires: every shape
// tokenMaybeContextualCloseAngle rejects must also make
// deferContextualCloseAngleAction's own (identical) first guard return
// false, so a caller that pre-checks with the extracted helper before
// resolving its own state (dispatchCorridor, parsercore_c4_vm.go) never
// disagrees with the full predicate it defers to.
//
// This uses the same "artifact_probe" language and state/source
// TestContextualActionDefersUnsignedShiftLineageAfterSingleCloseElection
// already proves triggers a real deferral (a well-formed narrow ">" token,
// state 1, source ">>>") specifically so the language has real LexModes and
// LexStates: with a language that has none (an earlier, weaker version of
// this test used a bare &Language{SymbolNames: ...}), every rejected-shape
// token also fails deferContextualCloseAngleAction's SEPARATE missing-lex-
// mode guard, so the test cannot tell "declined because of the shared shape
// gate" apart from "declined because of an unrelated missing-table guard" --
// a regression that deleted the internal tokenMaybeContextualCloseAngle
// call from deferContextualCloseAngleAction entirely could still pass it.
// Each malformed variant below shares the valid token's state, source, and
// start byte, so if the shared shape-gate call were ever removed, the
// downstream logic would run to completion and return true for every one of
// them (verified by mutation: deleting that call flips all three cases).
func TestDeferContextualCloseAngleActionSharesShapeGateWithPreCheck(t *testing.T) {
	lang := &Language{
		Name:        "artifact_probe",
		SymbolNames: []string{"end", ">", ">>>"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: ">", Visible: true, Named: false},
			{Name: ">>>", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 0, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 2}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 3}}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: '>', Hi: '>', NextState: 4}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
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
	}
	const state = StateID(1)
	source := []byte(">>>")
	probe := &Lexer{}

	// The valid, well-formed token this exact fixture defers for (sanity
	// control: proves the fixture and probe are wired correctly).
	valid := Token{Symbol: 1, StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Column: 1}}
	if !tokenMaybeContextualCloseAngle(lang, valid) {
		t.Fatal("the valid narrow \">\" token unexpectedly failed the shape gate")
	}
	if !deferContextualCloseAngleAction(lang, source, state, valid, nil, probe, nil) {
		t.Fatal("the valid narrow \">\" token unexpectedly did not defer")
	}

	rejectedShapes := []Token{
		// wrong symbol name: same span as valid, but tagged as ">>>" (the
		// wide token's own symbol), not ">".
		{Symbol: 2, StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Column: 1}},
		// width != 1: same start byte and symbol as valid, but spans two
		// bytes instead of one.
		{Symbol: 1, StartByte: 0, EndByte: 2, StartPoint: Point{}, EndPoint: Point{Column: 2}},
		// crosses a row: same span and symbol as valid, but EndPoint reports
		// a different row than StartPoint.
		{Symbol: 1, StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Row: 1}},
	}
	for _, tok := range rejectedShapes {
		if tokenMaybeContextualCloseAngle(lang, tok) {
			t.Fatalf("token %+v unexpectedly passed the shape gate", tok)
		}
		if deferContextualCloseAngleAction(lang, source, state, tok, nil, probe, nil) {
			t.Fatalf("token %+v unexpectedly deferred despite failing the shared shape gate", tok)
		}
	}
}

func TestPromoteActiveLiteralDoesNotUndoContextualKeywordDemotion(t *testing.T) {
	lang := &Language{
		Name:                "javascript",
		SymbolNames:         []string{"end", "identifier", "get"},
		KeywordCaptureToken: 1,
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
			{Name: "get", Visible: true, Named: false},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'g', Hi: 'g', NextState: 2}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 't', Hi: 't', NextState: 4}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		KeywordLexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'g', Hi: 'g', NextState: 1}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 2}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 't', Hi: 't', NextState: 3}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("get()")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok, _, _, _ := d.scanDFATokenForState(1, 1)
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
}

func TestPromoteActiveLiteralConsidersSameLexModeGLRStates(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "end", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "end", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'e', Hi: 'e', NextState: 2}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 4}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 0, 1},
			{0, 2, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("end")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok := d.nextDFAToken()
	if got, want := tok.Symbol, Symbol(1); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
}

func TestPromoteActiveLiteralRejectsImmediateAfterWhitespace(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "end", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "end", Visible: true, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		SymbolCount:     3,
		TokenCount:      3,
		StateCount:      2,
		LargeStateCount: 2,
		InitialState:    1,
		ImmediateTokens: []bool{false, true, false},
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{Default: -1, EOF: -1, Transitions: []LexTransition{
				{Lo: ' ', Hi: ' ', NextState: 1, Skip: true},
				{Lo: 'e', Hi: 'e', NextState: 2},
			}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'n', Hi: 'n', NextState: 3}}},
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'd', Hi: 'd', NextState: 4}}},
			{AcceptToken: 2, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0, 0},
			{0, 1, 1},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte(" end")),
		language:          lang,
		state:             1,
		lookupActionIndex: parser.lookupActionIndex,
	}
	d.lexer.immediateTokens = lang.ImmediateTokens

	tok, _, _, _ := d.scanDFATokenForState(1, 1)
	if got, want := tok.Symbol, Symbol(2); got != want {
		t.Fatalf("token symbol = %d (%q), want %d (%q)", got, lang.SymbolNames[got], want, lang.SymbolNames[want])
	}
}

func TestNextGLRUnionDFATokenHandlesNoLookaheadLexState(t *testing.T) {
	lang := &Language{
		SymbolNames: []string{"end", "identifier"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Visible: false, Named: false},
			{Name: "identifier", Visible: true, Named: true},
		},
		SymbolCount:     2,
		TokenCount:      2,
		StateCount:      3,
		LargeStateCount: 3,
		InitialState:    1,
		LexStates: []LexState{
			{Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
		LexModes: []LexMode{
			{LexState: 0},
			{LexStateID: noLookaheadLexState},
			{LexState: 1},
		},
		ParseTable: [][]uint16{
			{0, 0},
			{1, 0},
			{1, 0},
		},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 1, ChildCount: 0}}},
		},
	}
	parser := NewParser(lang)
	d := &dfaTokenSource{
		lexer:             NewLexer(lang.LexStates, []byte("x")),
		language:          lang,
		state:             1,
		glrStates:         []StateID{1, 2},
		lookupActionIndex: parser.lookupActionIndex,
	}

	tok, ok := d.nextGLRUnionDFAToken()
	if !ok {
		t.Fatal("nextGLRUnionDFAToken returned ok=false, want true")
	}
	if !tok.NoLookahead {
		t.Fatalf("token NoLookahead = false, want true: %#v", tok)
	}
	if got, want := tok.StartByte, uint32(0); got != want {
		t.Fatalf("token start = %d, want %d", got, want)
	}
	if got, want := tok.EndByte, uint32(0); got != want {
		t.Fatalf("token end = %d, want %d", got, want)
	}
}

func TestNextDFATokenUsesWideLexStateIndex(t *testing.T) {
	const wideLexState = 70000
	lexStates := make([]LexState, wideLexState+2)
	lexStates[wideLexState] = LexState{
		Default: -1,
		EOF:     -1,
		Transitions: []LexTransition{
			{Lo: 'x', Hi: 'x', NextState: wideLexState + 1},
		},
	}
	lexStates[wideLexState+1] = LexState{
		AcceptToken: 1,
		Default:     -1,
		EOF:         -1,
	}
	lang := &Language{
		SymbolNames: []string{"end", "x"},
		LexStates:   lexStates,
		LexModes:    []LexMode{{}},
	}
	lang.LexModes[0].SetLexStateIndex(wideLexState)

	d := &dfaTokenSource{
		lexer:    NewLexer(lang.LexStates, []byte("x")),
		language: lang,
	}
	d.lexer.zeroWidthTokens = []bool{false, false}

	tok := d.nextDFAToken()
	if tok.Symbol != 1 || tok.StartByte != 0 || tok.EndByte != 1 {
		t.Fatalf("token = sym=%d %d-%d, want x 0-1", tok.Symbol, tok.StartByte, tok.EndByte)
	}
}
