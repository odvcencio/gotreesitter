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
}

func TestCAbsorbOrdinaryLeafPreservesTokenErrors(t *testing.T) {
	parser, direct := recoveryLeafPolicyFixture(t)
	tests := []struct {
		name           string
		tok            Token
		wantChildError bool
	}{
		{name: "direct internal-DFA token", tok: direct},
		{name: "skipped-prefix internal-DFA token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagSkippedPrefix, true); return tok }()},
		{name: "external scanner token", tok: func() Token { tok := direct; tok.ExternalScannerToken = true; return tok }()},
		{name: "missing token", tok: func() Token { tok := direct; tok.Missing = true; return tok }(), wantChildError: true},
		{name: "no-lookahead token", tok: func() Token { tok := direct; tok.NoLookahead = true; return tok }(), wantChildError: true},
		{name: "lexer error-mode token", tok: func() Token { tok := direct; tok.setLexFlag(tokenFlagErrorModeLexed, true); return tok }()},
		{name: "error-symbol token", tok: Token{Symbol: errorSymbol, EndByte: 1, lexFlags: tokenFlagInternalDFALexed}, wantChildError: true},
		{name: "synthetic zero-width token", tok: Token{Symbol: 1, lexFlags: tokenFlagInternalDFALexed}},
		{name: "generated token", tok: Token{Symbol: 1, EndByte: 1}},
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
				group:   &cRecGroup{},
				openErr: openErr,
			}
			parser.cAbsorbTokenIntoError(&stack, test.tok, nil, arena, nil, nil, nil)
			if got, want := openErr.ChildCount(), 1; got != want {
				t.Fatalf("child count = %d, want %d", got, want)
			}
			child := openErr.Child(0)
			if !openErr.HasError() || child.IsMissing() != test.tok.Missing {
				t.Fatal("absorption lost parent or missing-token errors")
			}
			if child.StartByte() != test.tok.StartByte || child.EndByte() != test.tok.EndByte {
				t.Fatal("absorption changed token byte bounds")
			}
			if got := child.HasError(); got != test.wantChildError {
				t.Fatalf("child HasError = %t, want %t", got, test.wantChildError)
			}
		})
	}
}

func TestCRecoverStateClonePreservesGroupOrder(t *testing.T) {
	original := &cRecoverState{groupOrder: 1<<31 + 2}
	clone := original.clone()
	if clone == nil || clone == original || clone.groupOrderValue() != original.groupOrderValue() {
		t.Fatal("clone lost the independent recovery order")
	}
}

func TestCRecoverGroupOrderUsesFullWidth(t *testing.T) {
	group := &cRecGroup{}
	stacks := []glrStack{
		{cRec: &cRecoverState{group: group, groupOrder: 1<<31 + 2}},
		{cRec: &cRecoverState{group: group, groupOrder: 0}},
		{cRec: &cRecoverState{group: group, groupOrder: 1}},
	}
	members := []int{0, 1, 2}
	cSortRecoverMembersByGroupOrder(stacks, members)
	for i, want := range []int{1, 2, 0} {
		if members[i] != want {
			t.Fatalf("members[%d] = %d, want %d", i, members[i], want)
		}
	}
}

func TestCRecoverStateKeepsSizeBudget(t *testing.T) {
	if got := unsafe.Sizeof(cRecoverState{}); got != 48 {
		t.Fatalf("cRecoverState size = %d, want 48", got)
	}
}
