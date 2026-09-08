package gotreesitter

import (
	"testing"
	"unsafe"
)

func recoveryLeafPolicyFixture(t *testing.T) (*Parser, Token) {
	t.Helper()
	language := &Language{
		SymbolMetadata: []SymbolMetadata{
			{},
			{Name: "named_visible", Visible: true, Named: true},
			{Name: "anonymous_visible", Visible: true},
			{Name: "named_hidden", Named: true},
		},
	}
	lexer := NewLexer([]LexState{
		{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'x', Hi: 'x', NextState: 1}}},
		{AcceptToken: 1, Default: -1, EOF: -1},
	}, []byte("x"))
	tok := lexer.Next(0)
	if !tok.lexerInternalDFALexed() {
		t.Fatal("fixture token lacks positive internal-DFA provenance")
	}
	return &Parser{language: language}, tok
}

func TestCRecoveryRegionClearsOrdinaryLeafErrors(t *testing.T) {
	parser, direct := recoveryLeafPolicyFixture(t)
	tests := []struct {
		name string
		tok  Token
		want bool
	}{
		{name: "direct named internal-DFA token", tok: direct, want: true},
		{name: "unproven token", tok: Token{Symbol: 1, StartByte: 0, EndByte: 1}},
		{name: "end-of-input symbol", tok: Token{Symbol: 0, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}},
		{name: "synthetic zero-width token", tok: Token{Symbol: 1, lexFlags: tokenFlagInternalDFALexed}},
		{name: "generated token", tok: Token{Symbol: 1, StartByte: 0, EndByte: 1}},
		{name: "anonymous internal-DFA token", tok: func() Token { tok := direct; tok.Symbol = 2; return tok }()},
		{name: "invisible named internal-DFA token", tok: func() Token { tok := direct; tok.Symbol = 3; return tok }()},
		{name: "skipped-prefix first token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagSkippedPrefix, true); return tok }()},
		{name: "external scanner token", tok: func() Token { tok := direct; tok.ExternalScannerToken = true; return tok }()},
		{name: "lexer error-mode token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagErrorModeLexed, true); return tok }()},
		{name: "missing token", tok: func() Token { tok := direct; tok.Missing = true; return tok }()},
		{name: "no-lookahead token", tok: func() Token { tok := direct; tok.NoLookahead = true; return tok }()},
		{name: "error token", tok: Token{Symbol: errorSymbol, StartByte: 0, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cRecoveryRegionClearsOrdinaryLeafErrors(parser, test.tok, true); got != test.want {
				t.Fatalf("predicate = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCRecoveryRegionRequiresParsedPrefix(t *testing.T) {
	parser, direct := recoveryLeafPolicyFixture(t)
	if cRecoveryRegionClearsOrdinaryLeafErrors(parser, direct, false) {
		t.Fatal("predicate accepted a recovery region without a parsed prefix")
	}
}

func TestCRecoveryEntriesHaveParsedPrefix(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	zeroWidth := newLeafNodeInArena(arena, 1, true, 2, 2, Point{}, Point{})
	sourceBearing := newLeafNodeInArena(arena, 1, true, 2, 3, Point{}, Point{})
	future := newLeafNodeInArena(arena, 1, true, 3, 5, Point{}, Point{})
	outOfOrder := newLeafNodeInArena(arena, 1, true, 4, 3, Point{}, Point{})
	missing := newLeafNodeInArena(arena, 1, true, 1, 2, Point{}, Point{})
	missing.setMissing(true)
	hasError := newLeafNodeInArena(arena, 1, true, 1, 2, Point{}, Point{})
	hasError.setHasError(true)
	dirty := newLeafNodeInArena(arena, 1, true, 1, 2, Point{}, Point{})
	dirty.setDirty(true)
	errorNode := newLeafNodeInArena(arena, errorSymbol, true, 1, 2, Point{}, Point{})
	pending := newPendingParentInArena(arena, 1, true, 0, nil, 1, 2, Point{}, Point{}, false)
	invalid := newStackEntryNode(2, sourceBearing)
	invalid.kind = 99
	for _, node := range []*Node{zeroWidth, sourceBearing, future, outOfOrder, missing, hasError, dirty, errorNode} {
		node.parseState = 2
	}
	stateMismatch := newStackEntryNode(3, sourceBearing)

	if cRecoveryEntriesHaveParsedPrefix([]stackEntry{{state: cErrorState}, {state: 1}}, 3) {
		t.Fatal("error discontinuity and base state supplied a parsed prefix")
	}
	if cRecoveryEntriesHaveParsedPrefix([]stackEntry{newStackEntryNode(2, zeroWidth)}, 3) {
		t.Fatal("zero-width generated node supplied a parsed prefix")
	}
	for name, entry := range map[string]stackEntry{
		"future":         newStackEntryNode(2, future),
		"out-of-order":   newStackEntryNode(2, outOfOrder),
		"missing":        newStackEntryNode(2, missing),
		"has-error":      newStackEntryNode(2, hasError),
		"dirty":          newStackEntryNode(2, dirty),
		"error-symbol":   newStackEntryNode(2, errorNode),
		"pending":        newStackEntryPendingParent(2, pending),
		"invalid":        invalid,
		"state-mismatch": stateMismatch,
	} {
		t.Run(name, func(t *testing.T) {
			if cRecoveryEntriesHaveParsedPrefix([]stackEntry{entry}, 3) {
				t.Fatal("invalid payload supplied a parsed prefix")
			}
		})
	}
	if !cRecoveryEntriesHaveParsedPrefix([]stackEntry{newStackEntryNode(2, sourceBearing)}, 3) {
		t.Fatal("source-bearing stack node did not supply a parsed prefix")
	}
}

type recoveryLeafPolicyTokenSource struct{}

func (*recoveryLeafPolicyTokenSource) Next() Token             { return Token{} }
func (*recoveryLeafPolicyTokenSource) SkipToByte(uint32) Token { return Token{} }

func recoveryLeafErrorModeFixture() (*Parser, []byte) {
	language := &Language{
		SymbolMetadata: []SymbolMetadata{{}, {Name: "visible", Visible: true, Named: true}, {Name: "shared", Visible: true, Named: true}},
		LexModes:       []LexMode{{LexState: 0}, {LexState: 2}},
		LexStates: []LexState{
			{Default: -1, EOF: -1, Transitions: []LexTransition{{Lo: 'x', Hi: 'x', NextState: 1}}},
			{AcceptToken: 1, Default: -1, EOF: -1},
			{Default: -1, EOF: -1},
		},
	}
	return &Parser{language: language}, []byte("x")
}

func TestCRecoveryManualResumeMarksVisibleErrorModeToken(t *testing.T) {
	parser, source := recoveryLeafErrorModeFixture()
	parser.cRecoverCustomSourceEligible = true
	stack := newGLRStack(1)
	tok, replaced := parser.cRecoverResumeLookahead(&recoveryLeafPolicyTokenSource{}, source, &stack, Token{Symbol: 2, EndByte: 1}, nil)
	if !replaced || !parser.cSymbolVisible(tok.Symbol) || !tok.lexerErrorModeLexed() {
		t.Fatalf("manual resume token = %+v, replaced = %t", tok, replaced)
	}
	if cRecoveryTokenCanClearOrdinaryLeafError(tok) {
		t.Fatal("manual error-mode token can clear an ordinary leaf error")
	}
}

func TestCRecoveryInternalErrorModeMarksVisibleToken(t *testing.T) {
	parser, source := recoveryLeafErrorModeFixture()
	stack := newGLRStack(1)
	stack.pushEntry(stackEntry{state: cErrorState}, nil, nil)
	stack.cRec = &cRecoverState{group: &cRecGroup{}}
	tok, ok := parser.cRecoverInternalErrorModeToken(&recoveryLeafPolicyTokenSource{}, []glrStack{stack}, source)
	if !ok || !parser.cSymbolVisible(tok.Symbol) || !tok.lexerErrorModeLexed() {
		t.Fatalf("internal error-mode token = %+v, ok = %t", tok, ok)
	}
	if cRecoveryTokenCanClearOrdinaryLeafError(tok) {
		t.Fatal("internal error-mode token can clear an ordinary leaf error")
	}
}

func TestCRecoveryTokenCanClearOrdinaryLeafError(t *testing.T) {
	_, direct := recoveryLeafPolicyFixture(t)
	tests := []struct {
		name string
		tok  Token
		want bool
	}{
		{name: "later direct internal-DFA token", tok: direct, want: true},
		{name: "later skipped-prefix internal-DFA token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagSkippedPrefix, true); return tok }(), want: true},
		{name: "later external scanner token", tok: func() Token { tok := direct; tok.ExternalScannerToken = true; return tok }()},
		{name: "later missing token", tok: func() Token { tok := direct; tok.Missing = true; return tok }()},
		{name: "later no-lookahead token", tok: func() Token { tok := direct; tok.NoLookahead = true; return tok }()},
		{name: "later lexer error-mode token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagErrorModeLexed, true); return tok }()},
		{name: "later error token", tok: Token{Symbol: errorSymbol, StartByte: 0, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}},
		{name: "later end-of-input token", tok: Token{Symbol: 0, StartByte: 1, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}},
		{name: "later synthetic zero-width token", tok: Token{Symbol: 1, lexFlags: tokenFlagInternalDFALexed}},
		{name: "later generated token", tok: Token{Symbol: 1, StartByte: 0, EndByte: 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cRecoveryTokenCanClearOrdinaryLeafError(test.tok); got != test.want {
				t.Fatalf("predicate = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCAbsorbOrdinaryLeafChecksEveryTokenProvenance(t *testing.T) {
	parser, direct := recoveryLeafPolicyFixture(t)
	tests := []struct {
		name           string
		tok            Token
		regionProof    bool
		wantChildError bool
	}{
		{name: "no region proof", tok: direct, wantChildError: true},
		{name: "direct internal-DFA token", tok: direct, regionProof: true},
		{name: "skipped-prefix internal-DFA token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagSkippedPrefix, true); return tok }(), regionProof: true},
		{name: "external scanner token", tok: func() Token { tok := direct; tok.ExternalScannerToken = true; return tok }(), regionProof: true, wantChildError: true},
		{name: "missing token", tok: func() Token { tok := direct; tok.Missing = true; return tok }(), regionProof: true, wantChildError: true},
		{name: "no-lookahead token", tok: func() Token { tok := direct; tok.NoLookahead = true; return tok }(), regionProof: true, wantChildError: true},
		{name: "lexer error-mode token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagErrorModeLexed, true); return tok }(), regionProof: true, wantChildError: true},
		{name: "error-symbol token", tok: Token{Symbol: errorSymbol, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}, regionProof: true, wantChildError: true},
		{name: "synthetic zero-width token", tok: Token{Symbol: 1, lexFlags: tokenFlagInternalDFALexed}, regionProof: true, wantChildError: true},
		{name: "generated token", tok: Token{Symbol: 1, EndByte: 1}, regionProof: true, wantChildError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			openErr := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
			openErr.setHasError(true)
			stack := newGLRStack(1)
			stack.pushEntry(newStackEntryNode(cErrorState, openErr), nil, nil)
			stack.cRec = &cRecoverState{
				group:      &cRecGroup{},
				openErr:    openErr,
				groupOrder: cPackRecoverGroupOrder(0, test.regionProof),
			}
			parser.cAbsorbTokenIntoError(&stack, test.tok, nil, arena, nil, nil, nil)
			if got, want := openErr.ChildCount(), 1; got != want {
				t.Fatalf("child count = %d, want %d", got, want)
			}
			if got := openErr.Child(0).HasError(); got != test.wantChildError {
				t.Fatalf("child HasError = %t, want %t", got, test.wantChildError)
			}
		})
	}
}

func TestCRecoveryRegionClearsOrdinaryLeafErrorsNilParser(t *testing.T) {
	if cRecoveryRegionClearsOrdinaryLeafErrors(nil, Token{Symbol: 1, StartByte: 0, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}, true) {
		t.Fatal("predicate accepted a nil parser")
	}
}

func TestCRecoverStateClonePreservesLeafPolicy(t *testing.T) {
	original := &cRecoverState{groupOrder: cPackRecoverGroupOrder(2, true)}
	clone := original.clone()
	if clone == nil || !clone.clearsOrdinaryLeafErrors() {
		t.Fatal("recovery-state clone lost the leaf policy")
	}
	if got, want := clone.groupOrderValue(), uint32(2); got != want {
		t.Fatalf("clone group order = %d, want %d", got, want)
	}
}

func TestCRecoverGroupOrderPolicyDoesNotChangeSorting(t *testing.T) {
	group := &cRecGroup{}
	stacks := []glrStack{
		{cRec: &cRecoverState{group: group, groupOrder: cPackRecoverGroupOrder(2, false)}},
		{cRec: &cRecoverState{group: group, groupOrder: cPackRecoverGroupOrder(0, true)}},
		{cRec: &cRecoverState{group: group, groupOrder: cPackRecoverGroupOrder(1, false)}},
	}
	members := []int{0, 1, 2}
	cSortRecoverMembersByGroupOrder(stacks, members)
	for i, want := range []int{1, 2, 0} {
		if members[i] != want {
			t.Fatalf("members[%d] = %d, want %d", i, members[i], want)
		}
	}
}

func TestCRecoverGroupOrderPolicyFailsClosedOnCollision(t *testing.T) {
	state := &cRecoverState{groupOrder: cPackRecoverGroupOrder(uint64(cRecoverGroupOrderLeafClearBit), true)}
	if state.clearsOrdinaryLeafErrors() {
		t.Fatal("colliding group order retained the leaf-clear policy")
	}
	if got := state.groupOrderValue(); got != cRecoverGroupOrderValueMask {
		t.Fatalf("colliding group order = %d, want saturated %d", got, cRecoverGroupOrderValueMask)
	}
}

func TestCRecoverStateLeafPolicyKeepsSizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(cRecoverState{}); got != 48 {
		t.Fatalf("cRecoverState size = %d, want 48", got)
	}
}

func TestCRecoveryRegionAuthenticatesSkippedWhitespace(t *testing.T) {
	parser, direct := recoveryLeafPolicyFixture(t)
	parser.language.SymbolNames = []string{"", "named_visible", "x", "hidden", "y"}
	parser.language.SymbolMetadata = append(parser.language.SymbolMetadata, SymbolMetadata{Visible: true})

	skipped := direct
	skipped.StartByte, skipped.EndByte = 1, 2
	skipped.lexerSkippedPrefixStart = 0
	skipped.setLexFlag(tokenFlagSkippedPrefix, true)
	tests := []struct {
		name   string
		source string
		tok    Token
		prefix bool
		want   bool
	}{
		{"space", " x", skipped, true, true},
		{"anonymous wrong symbol spelling", " x", func() Token { v := skipped; v.Symbol = 4; return v }(), true, false},
		{"anonymous invalid symbol", " x", func() Token { v := skipped; v.Symbol = 99; return v }(), true, false},
		{"anonymous missing", " x", func() Token { v := skipped; v.Symbol = 2; v.Missing = true; return v }(), true, false},
		{"anonymous EOF", " x", func() Token { v := skipped; v.Symbol = 0; return v }(), true, false},
		{"anonymous zero width", " x", func() Token { v := skipped; v.Symbol = 2; v.EndByte = v.StartByte; return v }(), true, false},
		{"anonymous invalid skipped prefix", "?x", func() Token { v := skipped; v.Symbol = 2; return v }(), true, false},
		{"anonymous source token", " x", func() Token { v := skipped; v.Symbol = 2; return v }(), true, true},
		{"anonymous text mismatch", " y", func() Token { v := skipped; v.Symbol = 2; return v }(), true, false},
		{"anonymous external token", " x", func() Token { v := skipped; v.Symbol = 2; v.ExternalScannerToken = true; return v }(), true, false},
		{"anonymous error-mode token", " x", func() Token { v := skipped; v.Symbol = 2; v.setLexFlag(tokenFlagErrorModeLexed, true); return v }(), true, false},
		{"anonymous absent prefix", " x", func() Token { v := skipped; v.Symbol = 2; return v }(), false, false},
		{"anonymous absent source", "", func() Token { v := skipped; v.Symbol = 2; return v }(), true, false},
		{"tab", "\tx", skipped, true, true},
		{"newline", "\nx", skipped, true, true},
		{"non-whitespace", "?x", skipped, true, false},
		{"mixed whitespace and invalid byte", " ?x", func() Token { v := skipped; v.StartByte = 2; v.EndByte = 3; return v }(), true, false},
		{"out-of-bounds span", " x", func() Token { v := skipped; v.StartByte = 3; v.EndByte = 4; return v }(), true, false},
		{"absent source", "", skipped, true, false},
		{"no parsed prefix", " x", skipped, false, false},
		{"empty skipped span", " x", func() Token { v := skipped; v.lexerSkippedPrefixStart = 1; return v }(), true, false},
		{"reversed skipped span", " x", func() Token { v := skipped; v.lexerSkippedPrefixStart = 2; return v }(), true, false},
		{"external token", " x", func() Token { v := skipped; v.ExternalScannerToken = true; return v }(), true, false},
		{"error-mode token", " x", func() Token { v := skipped; v.setLexFlag(tokenFlagErrorModeLexed, true); return v }(), true, false},
		{"unproven token", " x", func() Token { v := skipped; v.setLexFlag(tokenFlagInternalDFALexed, false); return v }(), true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.tok
			if got := cRecoveryRegionClearsOrdinaryLeafErrorsWithSource(parser, tc.tok, tc.prefix, []byte(tc.source)); got != tc.want {
				t.Fatalf("clear leaf error = %v, want %v", got, tc.want)
			}
			if tc.tok != before {
				t.Fatal("source check changed the published token")
			}
		})
	}
}
