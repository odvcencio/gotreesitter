package gotreesitter

import (
	"reflect"
	"testing"
)

func TestSnapshotTokenSourceStateUnsupportedType(t *testing.T) {
	ts := &stubTokenSource{}
	restore, ok := snapshotTokenSourceState(ts)
	if ok {
		restore()
		t.Fatal("snapshotTokenSourceState unexpected success for non-wrapped DFA token source")
	}
}

func TestSnapshotTokenSourceStateRestoresDFATokenSource(t *testing.T) {
	original := &dfaTokenSource{
		state:                      42,
		glrStates:                  []StateID{1, 2},
		lexer:                      &Lexer{pos: 13, row: 7, col: 4},
		externalValid:              []bool{true, false},
		extZeroTried:               []bool{false, true},
		externalTokenStart:         []byte{0x11, 0x22},
		externalTokenEnd:           []byte{0x33},
		externalSnapshot:           []byte{0x44, 0x55},
		externalRetrySnap:          []byte{0x66},
		externalCompare:            []byte{0x77},
		externalLexer:              ExternalLexer{startPoint: Point{Row: 3, Column: 4}, endPoint: Point{Row: 3, Column: 5}},
		externalRetryLexer:         ExternalLexer{startPoint: Point{Row: 7, Column: 8}, endPoint: Point{Row: 7, Column: 9}},
		lastExternalTokenStartByte: 88,
		lastExternalTokenEndByte:   99,
		lastExternalTokenValid:     true,
		extZeroPos:                 11,
		extZeroState:               12,
		zeroWidthPos:               21,
		zeroWidthCount:             34,
	}
	restore, ok := snapshotTokenSourceState(original)
	if !ok {
		t.Fatal("snapshotTokenSourceState returned false for *dfaTokenSource")
	}

	original.state = 77
	original.glrStates = append(original.glrStates[:0], 9, 8, 7)
	original.lexer = &Lexer{pos: 99, row: 10, col: 11}
	original.externalValid = []bool{false, false, true}
	original.extZeroTried = []bool{true}
	original.externalTokenStart = []byte{0xee}
	original.externalTokenEnd = nil
	original.externalSnapshot = []byte{0xaa}
	original.externalRetrySnap = nil
	original.externalCompare = []byte{0xbb, 0xcc}
	original.externalLexer = ExternalLexer{startPoint: Point{Row: 9, Column: 9}, endPoint: Point{Row: 10, Column: 10}}
	original.externalRetryLexer = ExternalLexer{startPoint: Point{Row: 5, Column: 6}, endPoint: Point{Row: 6, Column: 7}}
	original.lastExternalTokenStartByte = 101
	original.lastExternalTokenEndByte = 202
	original.lastExternalTokenValid = false
	original.extZeroPos = 121
	original.extZeroState = 131
	original.zeroWidthPos = 141
	original.zeroWidthCount = 151

	restore()

	if original.state != 42 {
		t.Fatalf("state restored to %d, want %d", original.state, 42)
	}
	if !reflect.DeepEqual(original.glrStates, []StateID{1, 2}) {
		t.Fatalf("glrStates = %v, want %v", original.glrStates, []StateID{1, 2})
	}
	if original.lexer.pos != 13 || original.lexer.row != 7 || original.lexer.col != 4 {
		t.Fatalf("lexer = %#v, want pos=13 row=7 col=4", original.lexer)
	}
	if !reflect.DeepEqual(original.externalValid, []bool{true, false}) {
		t.Fatalf("externalValid = %v, want %v", original.externalValid, []bool{true, false})
	}
	if !reflect.DeepEqual(original.extZeroTried, []bool{false, true}) {
		t.Fatalf("extZeroTried = %v, want %v", original.extZeroTried, []bool{false, true})
	}
	if !reflect.DeepEqual(original.externalTokenStart, []byte{0x11, 0x22}) {
		t.Fatalf("externalTokenStart = %v, want %v", original.externalTokenStart, []byte{0x11, 0x22})
	}
	if !reflect.DeepEqual(original.externalTokenEnd, []byte{0x33}) {
		t.Fatalf("externalTokenEnd = %v, want %v", original.externalTokenEnd, []byte{0x33})
	}
	if !reflect.DeepEqual(original.externalSnapshot, []byte{0x44, 0x55}) {
		t.Fatalf("externalSnapshot = %v, want %v", original.externalSnapshot, []byte{0x44, 0x55})
	}
	if !reflect.DeepEqual(original.externalRetrySnap, []byte{0x66}) {
		t.Fatalf("externalRetrySnap = %v, want %v", original.externalRetrySnap, []byte{0x66})
	}
	if !reflect.DeepEqual(original.externalCompare, []byte{0x77}) {
		t.Fatalf("externalCompare = %v, want %v", original.externalCompare, []byte{0x77})
	}
	if got, want := original.externalLexer.startPoint, (Point{Row: 3, Column: 4}); got != want {
		t.Fatalf("externalLexer.startPoint = %v, want %v", got, want)
	}
	if got, want := original.externalRetryLexer.startPoint, (Point{Row: 7, Column: 8}); got != want {
		t.Fatalf("externalRetryLexer.startPoint = %v, want %v", got, want)
	}
	if original.lastExternalTokenStartByte != 88 || original.lastExternalTokenEndByte != 99 {
		t.Fatalf("external token bytes = [%d, %d], want [88, 99]", original.lastExternalTokenStartByte, original.lastExternalTokenEndByte)
	}
	if !original.lastExternalTokenValid {
		t.Fatal("lastExternalTokenValid false, want true")
	}
	if original.extZeroPos != 11 || original.extZeroState != 12 {
		t.Fatalf("extZeroPos = %d extZeroState = %d, want 11/12", original.extZeroPos, original.extZeroState)
	}
	if original.zeroWidthPos != 21 || original.zeroWidthCount != 34 {
		t.Fatalf("zeroWidthPos = %d zeroWidthCount = %d, want 21/34", original.zeroWidthPos, original.zeroWidthCount)
	}
}

func TestSnapshotTokenSourceStateSupportsIncludedRangeWrapper(t *testing.T) {
	underlying := &dfaTokenSource{
		state: 50,
		lexer: &Lexer{pos: 3},
	}
	parsed := &includedRangeTokenSource{
		base:   underlying,
		ranges: []Range{{StartByte: 1, EndByte: 2}},
		idx:    4,
	}

	restore, ok := snapshotTokenSourceState(parsed)
	if !ok {
		t.Fatal("snapshotTokenSourceState returned false for included range token source")
	}

	underlying.state = 51
	underlying.lexer = &Lexer{pos: 9}
	parsed.idx = 9

	restore()

	if parsed.idx != 4 {
		t.Fatalf("included range idx = %d, want %d", parsed.idx, 4)
	}
	if underlying.state != 50 {
		t.Fatalf("underlying state = %d, want %d", underlying.state, 50)
	}
	if underlying.lexer.pos != 3 {
		t.Fatalf("underlying lexer.pos = %d, want %d", underlying.lexer.pos, 3)
	}
}

func TestScanLeafTokenWithoutMutatingDFATokenSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	oldSource := []byte("1+2")
	newSource := []byte("1+3")
	tree := mustParse(t, parser, oldSource)
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	})
	leaf := tree.lastEditedLeaf
	if leaf == nil {
		t.Fatal("expected edited leaf to be tracked")
	}

	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, newSource), lang, parser.lookupActionIndex, parser.hasKeywordState, parser.externalValidByState)
	ts.state = 88
	ts.glrStates = []StateID{1, 3, 5}
	ts.zeroWidthPos = 7
	ts.zeroWidthCount = 9
	beforeLexer := *ts.lexer
	beforeGLR := append([]StateID(nil), ts.glrStates...)

	tok, ok := scanLeafTokenWithoutMutatingSource(ts, leaf)
	if !ok {
		t.Fatal("scanLeafTokenWithoutMutatingSource returned false for DFA token source")
	}
	if tok.Symbol != 1 || tok.StartByte != 2 || tok.EndByte != 3 {
		t.Fatalf("token = %+v, want NUMBER at [2,3)", tok)
	}
	if ts.state != 88 {
		t.Fatalf("source parser state mutated to %d, want 88", ts.state)
	}
	if !reflect.DeepEqual(ts.glrStates, beforeGLR) {
		t.Fatalf("source GLR states mutated to %v, want %v", ts.glrStates, beforeGLR)
	}
	if ts.zeroWidthPos != 7 || ts.zeroWidthCount != 9 {
		t.Fatalf("zero-width guard mutated to pos=%d count=%d, want 7/9", ts.zeroWidthPos, ts.zeroWidthCount)
	}
	if !reflect.DeepEqual(*ts.lexer, beforeLexer) {
		t.Fatalf("source lexer mutated to %#v, want %#v", *ts.lexer, beforeLexer)
	}
}

func TestScanLeafTokenWithoutMutatingIncludedRangeTokenSource(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	oldSource := []byte("1+2")
	newSource := []byte("1+3")
	tree := mustParse(t, parser, oldSource)
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	})
	leaf := tree.lastEditedLeaf
	if leaf == nil {
		t.Fatal("expected edited leaf to be tracked")
	}

	base := newDFATokenSourceDirect(NewLexer(lang.LexStates, newSource), lang, parser.lookupActionIndex, parser.hasKeywordState, parser.externalValidByState)
	wrapped := &includedRangeTokenSource{
		base: base,
		ranges: []Range{
			{StartByte: 0, EndByte: 1},
			{StartByte: 2, EndByte: 3},
		},
	}
	beforeBaseLexer := *base.lexer

	tok, ok := scanLeafTokenWithoutMutatingSource(wrapped, leaf)
	if !ok {
		t.Fatal("scanLeafTokenWithoutMutatingSource returned false for included range token source")
	}
	if tok.Symbol != 1 || tok.StartByte != 2 || tok.EndByte != 3 {
		t.Fatalf("token = %+v, want NUMBER at [2,3)", tok)
	}
	if wrapped.idx != 0 {
		t.Fatalf("included range idx mutated to %d, want 0", wrapped.idx)
	}
	if !reflect.DeepEqual(*base.lexer, beforeBaseLexer) {
		t.Fatalf("base lexer mutated to %#v, want %#v", *base.lexer, beforeBaseLexer)
	}
}

func TestScanLeafTokenWithoutMutatingRejectsExternalScanner(t *testing.T) {
	lang := *buildArithmeticLanguage()
	lang.ExternalScanner = parserTestUnsafeExternalScanner{}
	parser := NewParser(&lang)
	oldSource := []byte("1+2")
	newSource := []byte("1+3")
	tree := mustParse(t, parser, oldSource)
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	})
	leaf := tree.lastEditedLeaf
	if leaf == nil {
		t.Fatal("expected edited leaf to be tracked")
	}

	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, newSource), &lang, parser.lookupActionIndex, parser.hasKeywordState, parser.externalValidByState)
	if tok, ok := scanLeafTokenWithoutMutatingSource(ts, leaf); ok {
		t.Fatalf("scanLeafTokenWithoutMutatingSource succeeded for external-scanner language: %+v", tok)
	}
}

func TestScanLeafTokenWithoutMutatingRejectsSyntheticExternalSymbols(t *testing.T) {
	lang := *buildArithmeticLanguage()
	lang.ExternalSymbols = []Symbol{1}
	parser := NewParser(&lang)
	oldSource := []byte("1+2")
	newSource := []byte("1+3")
	tree := mustParse(t, parser, oldSource)
	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	})
	leaf := tree.lastEditedLeaf
	if leaf == nil {
		t.Fatal("expected edited leaf to be tracked")
	}

	ts := newDFATokenSourceDirect(NewLexer(lang.LexStates, newSource), &lang, parser.lookupActionIndex, parser.hasKeywordState, parser.externalValidByState)
	if tok, ok := scanLeafTokenWithoutMutatingSource(ts, leaf); ok {
		t.Fatalf("scanLeafTokenWithoutMutatingSource succeeded for synthetic-external language: %+v", tok)
	}
}

func TestReuseTreeWithNewSourceKeepsPrimaryArena(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	oldSource := []byte("1+2")
	newSource := []byte("1+3")
	tree := mustParse(t, parser, oldSource)
	defer tree.Release()
	if tree.arena == nil {
		t.Fatal("initial parse did not attach a primary arena")
	}

	tree.Edit(InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	})
	if tree.lastEditedLeaf == nil {
		t.Fatal("expected edited leaf to be tracked")
	}

	arena := tree.arena
	refsBefore := arena.refs.Load()
	reused := reuseTreeWithNewSource(tree, newSource, tree.lastEditedLeaf)
	if reused == nil {
		t.Fatal("reuseTreeWithNewSource returned nil")
	}
	if reused.arena != arena {
		t.Fatalf("reused tree arena = %p, want primary arena %p", reused.arena, arena)
	}
	if len(reused.borrowedArena) != 0 {
		t.Fatalf("reused tree borrowed arenas = %d, want 0", len(reused.borrowedArena))
	}
	if got, want := arena.refs.Load(), refsBefore+1; got != want {
		t.Fatalf("arena refs after reuse = %d, want %d", got, want)
	}

	reused.Release()
	if got := arena.refs.Load(); got != refsBefore {
		t.Fatalf("arena refs after reused tree release = %d, want %d", got, refsBefore)
	}
}

func TestTokenInvariantLeafEditAllowsExtraLeaf(t *testing.T) {
	lang := buildArithmeticExtraCommentLanguage()
	parser := NewParser(lang)
	oldSource := []byte("1#2012\n+2")
	newSource := []byte("1#2013\n+2")
	tree := mustParse(t, parser, oldSource)

	tree.Edit(InputEdit{
		StartByte:   5,
		OldEndByte:  6,
		NewEndByte:  6,
		StartPoint:  Point{Row: 0, Column: 5},
		OldEndPoint: Point{Row: 0, Column: 6},
		NewEndPoint: Point{Row: 0, Column: 6},
	})
	leaf := tree.lastEditedLeaf
	if leaf == nil {
		t.Fatal("expected edited leaf to be tracked")
	}
	if !leaf.isExtra() {
		t.Fatalf("edited leaf extra = false, want true; leaf=%s", leaf.Type(lang))
	}

	reused := mustParseIncremental(t, parser, newSource, tree)
	rt := reused.ParseRuntime()
	if rt.StopReason != ParseStopAccepted {
		t.Fatalf("incremental stop reason = %q, want %q (%s)", rt.StopReason, ParseStopAccepted, rt.Summary())
	}
	if rt.TokensConsumed != 1 {
		t.Fatalf("tokens consumed = %d, want token-invariant fast path to consume 1", rt.TokensConsumed)
	}
	if got, want := reused.RootNode().EndByte(), uint32(len(newSource)); got != want {
		t.Fatalf("root end byte = %d, want %d", got, want)
	}
}

func TestTokenInvariantLeafEditUsesFreshCustomTokenSource(t *testing.T) {
	lang := buildArithmeticExtraCommentLanguage()
	parser := NewParser(lang)
	oldSource := []byte("1#2012\n+2")
	newSource := []byte("1#2013\n+2")
	oldTree, err := parser.ParseWithTokenSource(oldSource, newArithmeticExtraCommentTokenSource(oldSource))
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}

	oldTree.Edit(InputEdit{
		StartByte:   5,
		OldEndByte:  6,
		NewEndByte:  6,
		StartPoint:  Point{Row: 0, Column: 5},
		OldEndPoint: Point{Row: 0, Column: 6},
		NewEndPoint: Point{Row: 0, Column: 6},
	})
	newTree, err := parser.ParseIncrementalWithTokenSource(newSource, oldTree, newArithmeticExtraCommentTokenSource(newSource))
	if err != nil {
		t.Fatalf("ParseIncrementalWithTokenSource failed: %v", err)
	}
	rt := newTree.ParseRuntime()
	if rt.StopReason != ParseStopAccepted {
		t.Fatalf("incremental stop reason = %q, want %q (%s)", rt.StopReason, ParseStopAccepted, rt.Summary())
	}
	if rt.TokensConsumed != 1 {
		t.Fatalf("tokens consumed = %d, want fresh custom-source fast path to consume 1", rt.TokensConsumed)
	}
}

type arithmeticExtraCommentTokenSource struct {
	src []byte
	pos int
	row uint32
	col uint32
}

func newArithmeticExtraCommentTokenSource(src []byte) *arithmeticExtraCommentTokenSource {
	return &arithmeticExtraCommentTokenSource{src: src}
}

func (ts *arithmeticExtraCommentTokenSource) RebuildTokenSource(src []byte, _ *Language) (TokenSource, error) {
	return newArithmeticExtraCommentTokenSource(src), nil
}

func (ts *arithmeticExtraCommentTokenSource) SetParserState(StateID) {}

func (ts *arithmeticExtraCommentTokenSource) SetGLRStates([]StateID) {}

func (ts *arithmeticExtraCommentTokenSource) SkipToByte(offset uint32) Token {
	ts.pos = 0
	ts.row = 0
	ts.col = 0
	for ts.pos < len(ts.src) && ts.pos < int(offset) {
		ts.advance()
	}
	return ts.Next()
}

func (ts *arithmeticExtraCommentTokenSource) Next() Token {
	for ts.pos < len(ts.src) {
		switch ts.src[ts.pos] {
		case ' ', '\t', '\n':
			ts.advance()
			continue
		case '+':
			return ts.makeSingleByteToken(2)
		case '#':
			return ts.commentToken()
		default:
			if ts.src[ts.pos] >= '0' && ts.src[ts.pos] <= '9' {
				return ts.numberToken()
			}
			ts.advance()
		}
	}
	pt := Point{Row: ts.row, Column: ts.col}
	return Token{StartByte: uint32(ts.pos), EndByte: uint32(ts.pos), StartPoint: pt, EndPoint: pt}
}

func (ts *arithmeticExtraCommentTokenSource) makeSingleByteToken(sym Symbol) Token {
	start := ts.pos
	startPt := Point{Row: ts.row, Column: ts.col}
	ts.advance()
	return Token{
		Symbol:     sym,
		StartByte:  uint32(start),
		EndByte:    uint32(ts.pos),
		StartPoint: startPt,
		EndPoint:   Point{Row: ts.row, Column: ts.col},
		Text:       string(ts.src[start:ts.pos]),
	}
}

func (ts *arithmeticExtraCommentTokenSource) numberToken() Token {
	start := ts.pos
	startPt := Point{Row: ts.row, Column: ts.col}
	for ts.pos < len(ts.src) && ts.src[ts.pos] >= '0' && ts.src[ts.pos] <= '9' {
		ts.advance()
	}
	return Token{
		Symbol:     1,
		StartByte:  uint32(start),
		EndByte:    uint32(ts.pos),
		StartPoint: startPt,
		EndPoint:   Point{Row: ts.row, Column: ts.col},
		Text:       string(ts.src[start:ts.pos]),
	}
}

func (ts *arithmeticExtraCommentTokenSource) commentToken() Token {
	start := ts.pos
	startPt := Point{Row: ts.row, Column: ts.col}
	ts.advance()
	for ts.pos < len(ts.src) && ts.src[ts.pos] >= '0' && ts.src[ts.pos] <= '9' {
		ts.advance()
	}
	return Token{
		Symbol:     3,
		StartByte:  uint32(start),
		EndByte:    uint32(ts.pos),
		StartPoint: startPt,
		EndPoint:   Point{Row: ts.row, Column: ts.col},
		Text:       string(ts.src[start:ts.pos]),
	}
}

func (ts *arithmeticExtraCommentTokenSource) advance() {
	if ts.pos >= len(ts.src) {
		return
	}
	if ts.src[ts.pos] == '\n' {
		ts.row++
		ts.col = 0
		ts.pos++
		return
	}
	ts.pos++
	ts.col++
}

func buildArithmeticExtraCommentLanguage() *Language {
	return &Language{
		Name:               "arithmetic_extra_comment",
		SymbolCount:        5,
		TokenCount:         4,
		ExternalTokenCount: 0,
		StateCount:         5,
		LargeStateCount:    0,
		FieldCount:         0,
		ProductionIDCount:  2,

		SymbolNames: []string{"EOF", "NUMBER", "+", "COMMENT", "expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "+", Visible: true, Named: false},
			{Name: "COMMENT", Visible: true, Named: true},
			{Name: "expression", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 1, ProductionID: 0}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 4, ChildCount: 3, ProductionID: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, Extra: true}}},
		},

		// Columns: EOF(0), NUMBER(1), PLUS(2), COMMENT(3), expression(4)
		ParseTable: [][]uint16{
			{0, 1, 0, 8, 3},
			{2, 2, 2, 2, 0},
			{5, 0, 4, 8, 0},
			{0, 6, 0, 8, 0},
			{7, 7, 7, 7, 0},
		},

		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},

		LexStates: []LexState{
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
					{Lo: '+', Hi: '+', NextState: 2},
					{Lo: '#', Hi: '#', NextState: 4},
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
				},
			},
			{
				AcceptToken: 2,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: nil,
			},
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			{
				AcceptToken: 3,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 4},
				},
			},
		},
	}
}
