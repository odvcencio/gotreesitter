package gotreesitter

import "testing"

type stubTokenSource struct {
	tokens     []Token
	i          int
	state      StateID
	nextCalls  int
	skipCalls  int
	relexTok   Token
	relexOK    bool
	canRelex   bool
	relexSeen  Token
	relexCalls int
}

type nextOnlyTokenSource struct {
	tokens []Token
	i      int
}

func (s *nextOnlyTokenSource) Next() Token {
	if s.i >= len(s.tokens) {
		return Token{}
	}
	tok := s.tokens[s.i]
	s.i++
	return tok
}

func (s *stubTokenSource) Next() Token {
	s.nextCalls++
	if s.i >= len(s.tokens) {
		return Token{}
	}
	t := s.tokens[s.i]
	s.i++
	return t
}

func (s *stubTokenSource) SkipToByte(offset uint32) Token {
	s.skipCalls++
	for {
		t := s.Next()
		if t.Symbol == 0 || t.StartByte >= offset {
			return t
		}
	}
}

func (s *stubTokenSource) SkipToByteWithPoint(offset uint32, _ Point) Token {
	return s.SkipToByte(offset)
}

func (s *stubTokenSource) SetParserState(state StateID) {
	s.state = state
}

func (s *stubTokenSource) SetGLRStates(states []StateID) {
	// stub: no-op
}

func (s *stubTokenSource) CanRelexFromTokenStart(tok Token) bool {
	return s.canRelex
}

func (s *stubTokenSource) RelexFromTokenStart(tok Token) (Token, bool) {
	s.relexCalls++
	s.relexSeen = tok
	if !s.relexOK {
		return Token{}, false
	}
	return s.relexTok, true
}

func TestNormalizeIncludedRanges(t *testing.T) {
	in := []Range{
		{StartByte: 20, EndByte: 30},
		{StartByte: 10, EndByte: 15},
		{StartByte: 15, EndByte: 18},
		{StartByte: 18, EndByte: 18}, // empty, dropped
		{StartByte: 28, EndByte: 35}, // merge with 20-30
	}
	out := normalizeIncludedRanges(in)
	if len(out) != 2 {
		t.Fatalf("normalize len: got %d, want 2", len(out))
	}
	if out[0].StartByte != 10 || out[0].EndByte != 18 {
		t.Fatalf("range0: got %d-%d, want 10-18", out[0].StartByte, out[0].EndByte)
	}
	if out[1].StartByte != 20 || out[1].EndByte != 35 {
		t.Fatalf("range1: got %d-%d, want 20-35", out[1].StartByte, out[1].EndByte)
	}
}

func TestIncludedRangeTokenSourceDropsAllEmptyRanges(t *testing.T) {
	base := &nextOnlyTokenSource{tokens: []Token{{Symbol: 1, StartByte: 0, EndByte: 1}}}
	ts := newIncludedRangeTokenSource(base, []Range{
		{StartByte: 3, EndByte: 3},
		{StartByte: 8, EndByte: 4},
	})
	if ts != base {
		t.Fatalf("empty ranges returned %T, want the original token source", ts)
	}
}

func TestParserIncludedRangesUseDFAOnlyWithoutExternalScanner(t *testing.T) {
	p := NewParser(nil)
	p.SetIncludedRanges([]Range{{StartByte: 1, EndByte: 2}})

	internal := newIncludedRangeTestDFASource([]byte("xa"))
	if got := p.wrapIncludedRanges(internal); got != internal {
		t.Fatalf("internal DFA wrapper type = %T, want the producer source", got)
	}
	p.SetIncludedRanges(nil)
	if got := p.wrapIncludedRanges(internal); got != internal || len(internal.lexer.includedRanges) != 0 {
		t.Fatalf("cleared internal DFA source = %T with %d ranges, want the producer source with no ranges",
			got, len(internal.lexer.includedRanges))
	}
	p.SetIncludedRanges([]Range{{StartByte: 1, EndByte: 2}})

	external := newIncludedRangeTestDFASource([]byte("xa"))
	external.language.ExternalScanner = dualChoiceExternalScanner{}
	external.hasExternalScanner = true
	if _, ok := p.wrapIncludedRanges(external).(*includedRangeTokenSource); !ok {
		t.Fatal("external-scanner DFA did not use the fallback wrapper")
	}

	custom := &nextOnlyTokenSource{tokens: []Token{{Symbol: 1, StartByte: 1, EndByte: 2}}}
	if _, ok := p.wrapIncludedRanges(custom).(*includedRangeTokenSource); !ok {
		t.Fatal("custom source did not use the fallback wrapper")
	}
}

func TestParserIncludedRangesExternalSymbolsWithoutScannerUseWrapper(t *testing.T) {
	states := buildIdentNumberWSDFA()
	language := &Language{
		Name:            "scannerless-external-range-test",
		SymbolNames:     []string{"end", "identifier", "number", extNameAutomaticSemicolon},
		ExternalSymbols: []Symbol{3},
		ExternalLexStates: [][]bool{
			{true},
		},
		LexStates: states,
		LexModes: []LexMode{{
			LexState:         0,
			ExternalLexState: 0,
		}},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 0}}},
		},
	}
	lookup := func(state StateID, symbol Symbol) uint16 {
		if state == 0 && symbol == 3 {
			return 1
		}
		return 0
	}
	source := newDFATokenSourceDirect(NewLexer(states, []byte("\n++abc")), language, lookup, nil, nil, nil)
	parser := NewParser(nil)
	parser.SetIncludedRanges([]Range{
		{StartByte: 0, EndByte: 1, EndPoint: Point{Row: 1}},
		{StartByte: 3, EndByte: 6, StartPoint: Point{Row: 1, Column: 2}, EndPoint: Point{Row: 1, Column: 5}},
	})

	wrapped := parser.wrapIncludedRanges(source)
	filter, ok := wrapped.(*includedRangeTokenSource)
	if !ok {
		t.Fatalf("external-symbol source type = %T, want the fallback wrapper", wrapped)
	}
	synthetic := filter.SkipToByteWithPoint(0, Point{})
	if synthetic.Symbol != 3 || synthetic.StartByte != 0 || synthetic.EndByte != 1 {
		t.Fatalf("synthetic token = %+v, want symbol 3 at [0,1)", synthetic)
	}
	if synthetic.StartByte < 3 && synthetic.EndByte > 1 {
		t.Fatalf("synthetic token spans excluded bytes [1,3): %+v", synthetic)
	}
	selected := filter.Next()
	if selected.Symbol != 1 || selected.Text != "abc" || selected.StartByte != 3 || selected.EndByte != 6 {
		t.Fatalf("selected token = %+v, want abc at [3,6) after the excluded gap", selected)
	}
}

func TestParserIncludedRangesUseInternalDFACursor(t *testing.T) {
	lang := buildArithmeticLanguage()
	p := NewParser(lang)
	p.SetIncludedRanges([]Range{{
		StartByte:  1,
		EndByte:    4,
		StartPoint: Point{Column: 1},
		EndPoint:   Point{Column: 4},
	}})

	tree := mustParse(t, p, []byte("x1+2y"))
	root := tree.RootNode()
	if root == nil || root.Type(lang) != "expression" || root.HasError() {
		t.Fatalf("included-range root = %v, want a clean expression", root)
	}
	if root.StartByte() != 1 || root.EndByte() != 4 {
		t.Fatalf("included-range root bytes = [%d,%d), want [1,4)", root.StartByte(), root.EndByte())
	}
}

func TestIncludedRangeTokenSourceFiltersTokens(t *testing.T) {
	base := &stubTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 5},
			{Symbol: 2, StartByte: 12, EndByte: 15},
			{Symbol: 3, StartByte: 21, EndByte: 22},
			{},
		},
	}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 10, EndByte: 20}}).(*includedRangeTokenSource)

	tok := ts.Next()
	if tok.Symbol != 2 {
		t.Fatalf("first token: got %d, want 2", tok.Symbol)
	}
	tok = ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("second token should be EOF-like, got %d", tok.Symbol)
	}
}

func TestIncludedRangeTokenSourceReseeksOverlappingTokenAtRangeStart(t *testing.T) {
	base := &stubTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 5},
			{Symbol: 2, StartByte: 3, EndByte: 5},
			{Symbol: 3, StartByte: 12, EndByte: 15},
			{},
		},
	}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 3, EndByte: 20}}).(*includedRangeTokenSource)

	tok := ts.Next()
	if tok.Symbol != 2 {
		t.Fatalf("overlapping token: got %d, want 2", tok.Symbol)
	}
}

func TestIncludedRangeTokenSourcePreservesOverlapWithoutReseek(t *testing.T) {
	base := &nextOnlyTokenSource{tokens: []Token{
		{Symbol: 1, StartByte: 0, EndByte: 5},
		{Symbol: 2, StartByte: 8, EndByte: 10},
		{},
	}}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 3, EndByte: 12}}).(*includedRangeTokenSource)

	tok := ts.Next()
	if tok.Symbol != 1 || tok.StartByte != 0 || tok.EndByte != 5 {
		t.Fatalf("overlapping token without seek: got %+v, want the original token", tok)
	}
}

func TestIncludedRangeTokenSourceSkipsExcludedTokenWithoutReseek(t *testing.T) {
	base := &nextOnlyTokenSource{tokens: []Token{
		{Symbol: 1, StartByte: 0, EndByte: 2},
		{Symbol: 2, StartByte: 1, EndByte: 5},
		{Symbol: 3, StartByte: 8, EndByte: 10},
		{},
	}}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 3, EndByte: 12}}).(*includedRangeTokenSource)

	tok := ts.Next()
	if tok.Symbol != 2 || tok.StartByte != 1 || tok.EndByte != 5 {
		t.Fatalf("first selected token = %+v, want the preserved overlap", tok)
	}
	if base.i != 2 {
		t.Fatalf("base token index = %d, want 2 after skipping one excluded token", base.i)
	}
}

func TestInitialParseStackClampsIncludedRangeStartPastSource(t *testing.T) {
	var scratch parserScratch
	parser := &Parser{
		language: &Language{InitialState: 1},
		included: []Range{{StartByte: 100, EndByte: 200}},
	}

	stacks, _ := parser.newInitialParseStacks(&scratch, nil, nil, 12)
	if got, want := stacks[0].byteOffset, uint32(12); got != want {
		t.Fatalf("initial byte offset = %d, want clamped source end %d", got, want)
	}
}

func TestIncludedRangeTokenSourceDelegatesParserState(t *testing.T) {
	base := &stubTokenSource{
		tokens: []Token{{}},
	}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 0, EndByte: 1}}).(*includedRangeTokenSource)
	ts.SetParserState(42)
	if base.state != 42 {
		t.Fatalf("delegated parser state: got %d, want 42", base.state)
	}
}

// stubErrorModeTokenSource embeds stubTokenSource and additionally implements
// errorModeLexingTokenSource, mirroring a C-recovery-enabled dfaTokenSource.
type stubErrorModeTokenSource struct {
	stubTokenSource
	errorMode bool
}

func (s *stubErrorModeTokenSource) lexesErrorModeAtErrorState() bool {
	return s.errorMode
}

// TestIncludedRangeTokenSourceForwardsErrorModeLexingCapability pins the
// wave-1 amendment: an includedRangeTokenSource has no lexing of its own, so
// it must defer entirely to the base source's errorModeLexingTokenSource
// answer rather than always reporting false (which would route the C
// recovery port's engine-side error-mode substitution over the whole
// document, ignoring the active ranges) or always true.
func TestIncludedRangeTokenSourceForwardsErrorModeLexingCapability(t *testing.T) {
	plainBase := &stubTokenSource{tokens: []Token{{}}}
	plainTS := newIncludedRangeTokenSource(plainBase, []Range{{StartByte: 0, EndByte: 1}}).(*includedRangeTokenSource)
	if em, ok := TokenSource(plainTS).(errorModeLexingTokenSource); !ok || em.lexesErrorModeAtErrorState() {
		t.Fatalf("plain base: lexesErrorModeAtErrorState should forward to false (ok=%v)", ok)
	}

	errModeBase := &stubErrorModeTokenSource{stubTokenSource: stubTokenSource{tokens: []Token{{}}}, errorMode: true}
	errModeTS := newIncludedRangeTokenSource(errModeBase, []Range{{StartByte: 0, EndByte: 1}}).(*includedRangeTokenSource)
	em, ok := TokenSource(errModeTS).(errorModeLexingTokenSource)
	if !ok || !em.lexesErrorModeAtErrorState() {
		t.Fatalf("error-mode base: lexesErrorModeAtErrorState should forward to true (ok=%v)", ok)
	}
}

func TestIncludedRangeTokenSourceRelexRejectsOriginalOutsideRangeWithoutCallingBase(t *testing.T) {
	base := &stubTokenSource{
		canRelex: true,
		relexOK:  true,
		relexTok: Token{
			Symbol:     1,
			StartByte:  0,
			EndByte:    5,
			StartPoint: Point{},
			EndPoint:   Point{Column: 5},
		},
	}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 10, EndByte: 20}}).(*includedRangeTokenSource)

	tok := Token{
		Symbol:     1,
		StartByte:  0,
		EndByte:    5,
		StartPoint: Point{},
		EndPoint:   Point{Column: 5},
	}
	if got, ok := ts.RelexFromTokenStart(tok); ok {
		t.Fatalf("RelexFromTokenStart = (%+v, true), want false for original token outside included range", got)
	}
	if base.relexCalls != 0 {
		t.Fatalf("base RelexFromTokenStart calls = %d, want 0", base.relexCalls)
	}
	if base.nextCalls != 0 || base.skipCalls != 0 || base.i != 0 {
		t.Fatalf("rejected relex advanced base: next=%d skip=%d i=%d, want 0/0/0", base.nextCalls, base.skipCalls, base.i)
	}
	if ts.idx != 0 {
		t.Fatalf("rejected relex changed included range index to %d, want 0", ts.idx)
	}
}

func TestIncludedRangeTokenSourceRelexRejectsBaseStartChangeWithoutFiltering(t *testing.T) {
	base := &stubTokenSource{
		tokens: []Token{
			{
				Symbol:     3,
				StartByte:  12,
				EndByte:    14,
				StartPoint: Point{Column: 12},
				EndPoint:   Point{Column: 14},
			},
		},
		canRelex: true,
		relexOK:  true,
		relexTok: Token{
			Symbol:     2,
			StartByte:  11,
			EndByte:    12,
			StartPoint: Point{Column: 11},
			EndPoint:   Point{Column: 12},
		},
	}
	ts := newIncludedRangeTokenSource(base, []Range{{StartByte: 10, EndByte: 20}}).(*includedRangeTokenSource)

	tok := Token{
		Symbol:     1,
		StartByte:  10,
		EndByte:    11,
		StartPoint: Point{Column: 10},
		EndPoint:   Point{Column: 11},
	}
	if got, ok := ts.RelexFromTokenStart(tok); ok {
		t.Fatalf("RelexFromTokenStart = (%+v, true), want false after base changes start", got)
	}
	if base.relexCalls != 1 {
		t.Fatalf("base RelexFromTokenStart calls = %d, want 1", base.relexCalls)
	}
	if base.relexSeen.StartByte != tok.StartByte || base.relexSeen.StartPoint != tok.StartPoint {
		t.Fatalf("base saw token %+v, want %+v", base.relexSeen, tok)
	}
	if base.nextCalls != 0 || base.skipCalls != 0 || base.i != 0 {
		t.Fatalf("rejected relex advanced base through filtering: next=%d skip=%d i=%d, want 0/0/0", base.nextCalls, base.skipCalls, base.i)
	}
	if ts.idx != 0 {
		t.Fatalf("rejected relex changed included range index to %d, want 0", ts.idx)
	}
}

func TestParserSetIncludedRangesRoundTrip(t *testing.T) {
	p := NewParser(nil)
	p.SetIncludedRanges([]Range{
		{StartByte: 5, EndByte: 8},
		{StartByte: 1, EndByte: 3},
	})
	got := p.IncludedRanges()
	if len(got) != 2 {
		t.Fatalf("IncludedRanges len: got %d, want 2", len(got))
	}
	if got[0].StartByte != 1 || got[1].StartByte != 5 {
		t.Fatalf("IncludedRanges not sorted: got %v", got)
	}
}

func TestParserSetIncludedRangesRepeated(t *testing.T) {
	p := NewParser(buildArithmeticLanguage())
	ranges := []Range{
		{StartByte: 2, EndByte: 4, StartPoint: Point{Column: 2}, EndPoint: Point{Column: 4}},
		{StartByte: 8, EndByte: 9, StartPoint: Point{Row: 1, Column: 3}, EndPoint: Point{Row: 1, Column: 4}},
	}
	p.SetIncludedRanges(ranges)
	if allocations := testing.AllocsPerRun(100, func() { p.SetIncludedRanges(ranges) }); allocations != 0 {
		t.Errorf("setting identical included ranges allocated %.1f objects", allocations)
	}

	previous := p.IncludedRanges()
	ranges[0].StartPoint.Column++
	if got := p.IncludedRanges()[0]; got != previous[0] {
		t.Fatalf("config changed after caller mutated input: got %+v, want %+v", got, previous[0])
	}
	p.SetIncludedRanges(ranges)
	if got := p.IncludedRanges()[0]; got != ranges[0] {
		t.Fatalf("equal byte offsets hid an updated point: got %+v, want %+v", got, ranges[0])
	}
	if core := p.ccoreIncludedRanges(); len(core) != len(ranges) || core[0].StartCol != ranges[0].StartPoint.Column {
		t.Fatalf("core range metadata did not update: %+v", core)
	}
	p.SetIncludedRanges(nil)
	if len(p.IncludedRanges()) != 0 || len(p.ccoreIncludedRanges()) != 0 {
		t.Fatal("clearing ranges left configured ranges behind")
	}
}
