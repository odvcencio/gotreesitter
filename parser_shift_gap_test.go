package gotreesitter

import (
	"testing"
	"unsafe"
)

func TestRealShiftGapRejectsNonTriviaSource(t *testing.T) {
	source := []byte("call(arg1, arg8)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	if realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatalf("realShiftGapIsParserPadding = true, want false for gap %q", source[stack.byteOffset:tok.StartByte])
	}

	parser := &Parser{glrTrace: false}
	if parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap = true, want false")
	}
	if !stack.dead {
		t.Fatal("stack.dead = false, want true")
	}
}

func TestRealTokenAttachmentGapRejectsCommentSource(t *testing.T) {
	source := []byte("call(arg1/*c*/)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	if realTokenAttachmentGapIsParserPadding(source, &stack, tok, nil, 0) {
		t.Fatalf("realTokenAttachmentGapIsParserPadding = true, want false for comment gap %q", source[stack.byteOffset:tok.StartByte])
	}

	parser := &Parser{glrTrace: false}
	if parser.guardRealTokenAttachmentGap(source, &stack, tok, "test") {
		t.Fatal("guardRealTokenAttachmentGap = true, want false")
	}
	if !stack.dead {
		t.Fatal("stack.dead = false, want true")
	}
}

func TestRealShiftGapAllowsTriviaOnlySource(t *testing.T) {
	source := []byte("call(arg1   \n\t\f\v)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	if !realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatalf("realShiftGapIsParserPadding = false, want true for gap %q", source[stack.byteOffset:tok.StartByte])
	}

	parser := &Parser{glrTrace: false}
	if !parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap = false, want true")
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
}

func TestRealShiftGapAllowsEscapedNewlinePadding(t *testing.T) {
	source := []byte("call(arg1 \\\r\n  \\\n)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	if !realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatalf("realShiftGapIsParserPadding = false, want true for gap %q", source[stack.byteOffset:tok.StartByte])
	}

	parser := &Parser{glrTrace: false}
	if !parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap = false, want true")
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
}

// TestBytesAreParserPaddingLineContinuationEscapeComposes pins the composition
// the PowerShell backtick-continuation repair depends on: a language-declared
// continuation escape (Language.LineContinuationEscapeByte, threaded here as
// bytesAreParserPadding's continuationEscape parameter) accepts exactly that
// byte immediately followed by a newline as padding, the same unconditional
// treatment backslash+newline already gets, while leaving every other gap
// shape — including a DIFFERENT stray byte at the same position, and the
// same escape byte with no declaration at all (continuationEscape == 0, what
// every language gets by default) — exactly as rejected as before this
// parameter existed. Regression: PR #633 (materializeSkippedGapAsExtraError)
// must still materialize an ERROR for a genuine lexer-skipped stray; only a
// declared continuation escape may bypass that path.
func TestBytesAreParserPaddingLineContinuationEscapeComposes(t *testing.T) {
	source := []byte("a `\n   b")
	// [1:7) = " `\n   " -- backtick+LF then three spaces, mirroring the
	// PowerShell command_argument_sep shape from cgo_harness/corpus_real's
	// large__packaging.psm1 (C-oracle verified: 2 children, zero ERROR nodes).
	const start, end = 1, 7

	if !bytesAreParserPadding(source, start, end, '`') {
		t.Fatalf("bytesAreParserPadding(%q, escape='`') = false, want true", source[start:end])
	}
	if bytesAreParserPadding(source, start, end, 0) {
		t.Fatalf("bytesAreParserPadding(%q, escape=0) = true, want false: an undeclared escape must not become padding", source[start:end])
	}
	if bytesAreParserPadding(source, start, end, '@') {
		t.Fatalf("bytesAreParserPadding(%q, escape='@') = true, want false: declaring an unrelated escape byte must not match a different stray byte", source[start:end])
	}

	strayNonNewline := []byte("a `x   b") // backtick NOT followed by a newline
	if bytesAreParserPadding(strayNonNewline, start, end, '`') {
		t.Fatalf("bytesAreParserPadding(%q, escape='`') = true, want false: the escape byte alone (no following newline) is not padding", strayNonNewline[start:end])
	}
}

// TestRealTokenAttachmentGapIsParserPaddingAcceptsDeclaredEscape drives a
// non-zero continuationEscape through realTokenAttachmentGapIsParserPadding
// itself (every other direct call in this file passes 0, since those tests
// predate the continuation-escape parameter and pin unrelated gap shapes).
// This closes that gap in direct coverage: the function that
// tryMaterializeSkippedRealGap, guardRealTokenAttachmentGap, and
// tryReuseSubtree all actually call is exercised here with the same
// declared-escape argument production passes, not just its
// bytesAreParserPadding helper in isolation.
func TestRealTokenAttachmentGapIsParserPaddingAcceptsDeclaredEscape(t *testing.T) {
	source := []byte("a `\n   b") // same shape TestBytesAreParserPaddingLineContinuationEscapeComposes pins
	stack := newGLRStack(1)
	stack.byteOffset = 1
	tok := Token{
		Symbol:    1,
		StartByte: 7,
		EndByte:   8,
	}

	if !realTokenAttachmentGapIsParserPadding(source, &stack, tok, nil, '`') {
		t.Fatalf("realTokenAttachmentGapIsParserPadding(escape='`') = false, want true for gap %q", source[stack.byteOffset:tok.StartByte])
	}
	if realTokenAttachmentGapIsParserPadding(source, &stack, tok, nil, 0) {
		t.Fatalf("realTokenAttachmentGapIsParserPadding(escape=0) = true, want false: an undeclared escape must not become padding for gap %q", source[stack.byteOffset:tok.StartByte])
	}
}

func TestRealShiftGapAllowsNoLookaheadToken(t *testing.T) {
	source := []byte("call(arg1/*c*/)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:      1,
		StartByte:   uint32(len(source) - 1),
		EndByte:     uint32(len(source)),
		NoLookahead: true,
	}

	if !realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatal("realShiftGapIsParserPadding = false, want true for NoLookahead token")
	}

	parser := &Parser{glrTrace: false}
	if !parser.guardRealTokenAttachmentGap(source, &stack, tok, "test") {
		t.Fatal("guardRealTokenAttachmentGap = false, want true for NoLookahead token")
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
}

func TestRealShiftGapAllowsExternalScannerOwnedSkippedGap(t *testing.T) {
	source := []byte("aaaaaa identification division.")
	stack := newGLRStack(1)
	tok := Token{
		Symbol:                   1,
		StartByte:                6,
		EndByte:                  6,
		StartPoint:               Point{Column: 6},
		EndPoint:                 Point{Column: 6},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 0,
	}

	if !realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatalf("realShiftGapIsParserPadding = false, want true for scanner-owned gap %q", source[stack.byteOffset:tok.StartByte])
	}

	parser := &Parser{glrTrace: false}
	if !parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap = false, want true")
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
}

func TestRealShiftGapRejectsExternalScannerTokenFromDifferentStart(t *testing.T) {
	source := []byte("aaaaaa identification division.")
	stack := newGLRStack(1)
	tok := Token{
		Symbol:                   1,
		StartByte:                6,
		EndByte:                  6,
		StartPoint:               Point{Column: 6},
		EndPoint:                 Point{Column: 6},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 2,
	}

	if realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatalf("realShiftGapIsParserPadding = true, want false for mismatched scanner start gap %q", source[stack.byteOffset:tok.StartByte])
	}

	parser := &Parser{glrTrace: false}
	if parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap = true, want false")
	}
	if !stack.dead {
		t.Fatal("stack.dead = false, want true")
	}
}

func TestForestRealShiftGapRejectsNonTriviaSource(t *testing.T) {
	source := []byte("call(arg1, arg8)")
	node := &gssForestNode{byteOffset: uint32(len("call(arg1"))}
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	parser := &Parser{glrTrace: false}
	if parser.guardForestRealShiftGap(source, node, tok) {
		t.Fatal("guardForestRealShiftGap = true, want false")
	}
}

func TestForestRecoveryGapRejectsNonTriviaSource(t *testing.T) {
	source := []byte("call(arg1, arg8)")
	node := &gssForestNode{byteOffset: uint32(len("call(arg1"))}
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	var nextIndex gssForestIndex
	nextIndex.init(0)
	var nextFrontier []*gssForestNode
	parser := &Parser{glrTrace: false}
	if parser.guardForestRealShiftGap(source, node, tok) {
		leaf := &Node{}
		sh := coalesceForest(&nextIndex, &gssForestNodeSlab{}, node.state, tok.EndByte, node,
			stackEntry{node: unsafe.Pointer(leaf), state: node.state, kind: stackEntryKindNode},
			0, node.errorCost+int(tok.EndByte-tok.StartByte))
		nextFrontier = append(nextFrontier, sh)
	}
	if len(nextFrontier) != 0 {
		t.Fatalf("forest recovery accepted non-padding gap; next frontier len = %d", len(nextFrontier))
	}
}

func TestRealShiftGapAllowsLeadingBOMPadding(t *testing.T) {
	for _, source := range [][]byte{
		[]byte("\xef\xbb\xbfa"),
		[]byte("\xef\xbb\xbf\n\ta"),
	} {
		stack := newGLRStack(1)
		tok := Token{
			Symbol:    1,
			StartByte: uint32(len(source) - 1),
			EndByte:   uint32(len(source)),
		}

		if !realShiftGapIsParserPadding(source, &stack, tok, 0) {
			t.Fatalf("realShiftGapIsParserPadding(%q) = false, want true", source[:tok.StartByte])
		}
	}
}

func TestRealShiftGapRejectsNonLeadingBOM(t *testing.T) {
	source := []byte("call(arg1\xef\xbb\xbf)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source)),
	}

	if realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatalf("realShiftGapIsParserPadding = true, want false for non-leading BOM gap %q", source[stack.byteOffset:tok.StartByte])
	}
}

func TestMaterializeSkippedRealGapExtendsTopError(t *testing.T) {
	source := []byte("1abc+")
	parser := NewParser(buildArithmeticLanguage())
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	arena := newNodeArena(arenaClassFull)
	stack := newGLRStackWithScratch(2, &entryScratch)
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 1, 2, Point{Column: 1}, Point{Column: 2})
	errNode.setHasError(true)
	errNode.parseState = 2
	parser.pushStackNode(&stack, 2, errNode, &entryScratch, &gssScratch)
	stack.byteOffset = 2
	nodeCount := 1
	trackChildErrors := false
	tok := Token{
		Symbol:     2,
		StartByte:  4,
		EndByte:    5,
		StartPoint: Point{Column: 4},
		EndPoint:   Point{Column: 5},
	}

	if !parser.tryMaterializeSkippedRealGap(source, &stack, 2, tok, &nodeCount, arena, &entryScratch, &gssScratch, &trackChildErrors) {
		t.Fatal("tryMaterializeSkippedRealGap = false, want true")
	}
	if got := stackEntryNode(stack.top()); got != errNode {
		t.Fatalf("top error node changed: got %p want %p", got, errNode)
	}
	if got, want := errNode.EndByte(), uint32(4); got != want {
		t.Fatalf("error end byte = %d, want %d", got, want)
	}
	if got, want := errNode.EndPoint(), (Point{Column: 4}); got != want {
		t.Fatalf("error end point = %+v, want %+v", got, want)
	}
	if got, want := stack.byteOffset, uint32(4); got != want {
		t.Fatalf("stack byte offset = %d, want %d", got, want)
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1 for extension", nodeCount)
	}
	if !trackChildErrors {
		t.Fatal("trackChildErrors = false, want true")
	}
	if !parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap after materialization = false, want true")
	}
}

func TestMaterializeSkippedRealGapGroupsTrailingHiddenError(t *testing.T) {
	source := []byte(".\n@a")
	lang := &Language{
		Name:        "hidden_error_gap",
		TokenCount:  4,
		SymbolCount: 5,
		SymbolNames: []string{
			"EOF",
			".",
			"@",
			"a",
			"_hidden",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: ".", Visible: true},
			{Name: "@", Visible: true},
			{Name: "a", Visible: true},
			{Name: "_hidden"},
		},
	}
	parser := NewParser(lang)
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	arena := newNodeArena(arenaClassFull)
	stack := newGLRStackWithScratch(7, &entryScratch)
	errLeaf := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	errLeaf.setHasError(true)
	hidden := newParentNodeInArena(arena, 4, false, []*Node{errLeaf}, nil, 0)
	hidden.startByte = 0
	hidden.endByte = 1
	hidden.startPoint = Point{}
	hidden.endPoint = Point{Column: 1}
	hidden.setHasError(true)
	parser.pushStackNode(&stack, 7, hidden, &entryScratch, &gssScratch)
	stack.byteOffset = 1
	nodeCount := 1
	trackChildErrors := false
	tok := Token{
		Symbol:     3,
		StartByte:  3,
		EndByte:    4,
		StartPoint: Point{Row: 1, Column: 1},
		EndPoint:   Point{Row: 1, Column: 2},
	}

	if !parser.tryMaterializeSkippedRealGap(source, &stack, 7, tok, &nodeCount, arena, &entryScratch, &gssScratch, &trackChildErrors) {
		t.Fatal("tryMaterializeSkippedRealGap = false, want true")
	}
	if got := stackEntryNode(stack.top()); got != hidden {
		t.Fatalf("top node changed: got %p want hidden %p", got, hidden)
	}
	if got, want := stack.byteOffset, uint32(3); got != want {
		t.Fatalf("stack byte offset = %d, want %d", got, want)
	}
	if got, want := hidden.endByte, uint32(3); got != want {
		t.Fatalf("hidden end byte = %d, want %d", got, want)
	}
	grouped := hidden.children[0]
	if grouped == nil || grouped.symbol != errorSymbol {
		t.Fatalf("hidden child = %v, want grouped ERROR", grouped)
	}
	if got, want := grouped.startByte, uint32(0); got != want {
		t.Fatalf("grouped start byte = %d, want %d", got, want)
	}
	if got, want := grouped.endByte, uint32(3); got != want {
		t.Fatalf("grouped end byte = %d, want %d", got, want)
	}
	if len(grouped.children) != 2 {
		t.Fatalf("grouped child count = %d, want 2", len(grouped.children))
	}
	if got, want := grouped.children[0].symbol, Symbol(1); got != want {
		t.Fatalf("grouped child 0 symbol = %d, want %d", got, want)
	}
	if got, want := grouped.children[1].symbol, Symbol(2); got != want {
		t.Fatalf("grouped child 1 symbol = %d, want %d", got, want)
	}
	if nodeCount != 2 {
		t.Fatalf("nodeCount = %d, want 2", nodeCount)
	}
	if !trackChildErrors {
		t.Fatal("trackChildErrors = false, want true")
	}
	if !parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap after materialization = false, want true")
	}
}

func TestMaterializeSkippedRealGapCreatesErrorOnCleanStack(t *testing.T) {
	source := []byte("1abc+")
	parser := NewParser(buildArithmeticLanguage())
	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	arena := newNodeArena(arenaClassFull)
	stack := newGLRStackWithScratch(2, &entryScratch)
	stack.byteOffset = 2
	nodeCount := 0
	trackChildErrors := false
	tok := Token{
		Symbol:     2,
		StartByte:  4,
		EndByte:    5,
		StartPoint: Point{Column: 4},
		EndPoint:   Point{Column: 5},
	}

	if !parser.tryMaterializeSkippedRealGap(source, &stack, 2, tok, &nodeCount, arena, &entryScratch, &gssScratch, &trackChildErrors) {
		t.Fatal("tryMaterializeSkippedRealGap = false, want true for clean stack")
	}
	errNode := stackEntryNode(stack.top())
	if errNode == nil || errNode.symbol != errorSymbol {
		t.Fatalf("top node = %v, want ERROR", errNode)
	}
	if got, want := errNode.StartByte(), uint32(2); got != want {
		t.Fatalf("error start byte = %d, want %d", got, want)
	}
	if got, want := errNode.EndByte(), uint32(4); got != want {
		t.Fatalf("error end byte = %d, want %d", got, want)
	}
	if got, want := errNode.StartPoint(), (Point{}); got != want {
		t.Fatalf("error start point = %+v, want %+v", got, want)
	}
	if got, want := errNode.EndPoint(), (Point{Column: 4}); got != want {
		t.Fatalf("error end point = %+v, want %+v", got, want)
	}
	if got, want := errNode.parseState, StateID(2); got != want {
		t.Fatalf("error parse state = %d, want %d", got, want)
	}
	if got, want := stack.byteOffset, uint32(4); got != want {
		t.Fatalf("stack byte offset = %d, want %d", got, want)
	}
	if nodeCount != 1 {
		t.Fatalf("nodeCount = %d, want 1", nodeCount)
	}
	if !trackChildErrors {
		t.Fatal("trackChildErrors = false, want true")
	}
	if !parser.guardRealShiftGap(source, &stack, tok) {
		t.Fatal("guardRealShiftGap after materialization = false, want true")
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false")
	}
}

func TestRealShiftGapAllowsSyntheticMissingToken(t *testing.T) {
	source := []byte("call(arg1, arg8)")
	stack := newGLRStack(1)
	stack.byteOffset = uint32(len("call(arg1"))
	tok := Token{
		Symbol:    1,
		StartByte: uint32(len(source) - 1),
		EndByte:   uint32(len(source) - 1),
		Missing:   true,
	}

	if !realShiftGapIsParserPadding(source, &stack, tok, 0) {
		t.Fatal("realShiftGapIsParserPadding = false, want true for synthetic missing token")
	}
}

func TestReuseSubtreeRejectsCommentGapWithoutKillingStack(t *testing.T) {
	source := []byte("a/*c*/#")
	lang := buildExtraShiftGapLanguage()
	parser := NewParser(lang)
	a := NewLeafNode(1, true, 0, 1, Point{Column: 0}, Point{Column: 1})
	hash := NewLeafNode(2, true, 6, 7, Point{Column: 6}, Point{Column: 7})
	root := NewParentNode(1, true, []*Node{a, hash}, nil, 0)
	oldTree := NewTree(root, source, lang)

	var reuseScratch reuseScratch
	reuse := (&reuseCursor{}).reset(oldTree, source, &reuseScratch)
	if reuse == nil {
		t.Fatal("reuse cursor reset returned nil")
	}

	var entryScratch glrEntryScratch
	var gssScratch gssScratch
	stack := newGLRStackWithScratch(1, &entryScratch)
	stack.byteOffset = 1
	gapStart := stack.byteOffset
	lookahead := Token{
		Symbol:     2,
		StartByte:  6,
		EndByte:    7,
		StartPoint: Point{Column: 6},
		EndPoint:   Point{Column: 7},
	}
	ts := &stubTokenSource{
		tokens: []Token{
			{Symbol: 0, StartByte: uint32(len(source)), EndByte: uint32(len(source))},
		},
	}

	nextTok, reusedBytes, ok := parser.tryReuseSubtree(&stack, lookahead, ts, reuse, &entryScratch, &gssScratch)
	if ok {
		t.Fatalf("tryReuseSubtree reused across non-padding gap %q: bytes=%d next=%+v", source[gapStart:lookahead.StartByte], reusedBytes, nextTok)
	}
	if stack.dead {
		t.Fatal("stack.dead = true, want false so ordinary lex/parse can decide")
	}
	if got := stackEntryNode(stack.top()); got != nil {
		t.Fatalf("stack top node = %v, want nil after rejected reuse", got)
	}
}

func TestRecoverActionMaterializesCommentGap(t *testing.T) {
	source := []byte("1/*c*/*2")
	parser := NewParser(buildArithmeticRecoverLanguage())
	tree, err := parser.ParseWithTokenSource(source, &recoverCommentGapTokenSource{src: source})
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}

	if got, want := tree.ParseRuntime().StopReason, ParseStopAccepted; got != want {
		t.Fatalf("parse stop reason = %q, want %q; tree=%v", got, want, tree.RootNode())
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), errorSymbol, 1, 6); got != 1 {
		t.Fatalf("ERROR span 1..6 count = %d, want 1; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), errorSymbol, 6, 7); got != 1 {
		t.Fatalf("recovered lookahead ERROR span 6..7 count = %d, want 1; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), 1, 7, 8); got != 1 {
		t.Fatalf("NUMBER span 7..8 count = %d, want 1 after recovered lookahead is consumed; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), 3, 6, 7); got != 0 {
		t.Fatalf("STAR span 6..7 count = %d, want 0 because recovered lookahead must not be redispatched; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
}

func TestShiftActionMaterializesCommentGap(t *testing.T) {
	source := []byte("1/*c*/+2")
	parser := NewParser(buildArithmeticLanguage())
	tree, err := parser.ParseWithTokenSource(source, &recoverCommentGapTokenSource{src: source})
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}

	if got, want := tree.ParseRuntime().StopReason, ParseStopAccepted; got != want {
		t.Fatalf("parse stop reason = %q, want %q; tree=%v", got, want, tree.RootNode())
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), errorSymbol, 1, 6); got != 1 {
		t.Fatalf("ERROR span 1..6 count = %d, want 1; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
}

func TestExtraShiftActionMaterializesCommentGap(t *testing.T) {
	source := []byte("a/*c*/#")
	parser := NewParser(buildExtraShiftGapLanguage())
	tree, err := parser.ParseWithTokenSource(source, &extraShiftGapTokenSource{src: source})
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}

	if got, want := tree.ParseRuntime().StopReason, ParseStopAccepted; got != want {
		t.Fatalf("parse stop reason = %q, want %q; tree=%v", got, want, tree.RootNode())
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), errorSymbol, 1, 6); got != 1 {
		t.Fatalf("ERROR span 1..6 count = %d, want 1; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), 2, 6, 7); got != 1 {
		t.Fatalf("extra lookahead span 6..7 count = %d, want 1; tree=%s", got, tree.RootNode().SExpr(parser.language))
	}
}

func TestShiftActionExtendsExistingErrorAcrossSkippedRealGap(t *testing.T) {
	source := []byte("1abc+2")
	parser := NewParser(buildSkippedRealGapLanguage())
	tree, err := parser.ParseWithTokenSource(source, &skippedRealGapTokenSource{
		tokens: []Token{
			{Symbol: 1, StartByte: 0, EndByte: 1, StartPoint: Point{Column: 0}, EndPoint: Point{Column: 1}, Text: "1"},
			{Symbol: 3, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}, Text: "a"},
			{Symbol: 2, StartByte: 4, EndByte: 5, StartPoint: Point{Column: 4}, EndPoint: Point{Column: 5}, Text: "+"},
			{Symbol: 1, StartByte: 5, EndByte: 6, StartPoint: Point{Column: 5}, EndPoint: Point{Column: 6}, Text: "2"},
			{Symbol: 0, StartByte: 6, EndByte: 6, StartPoint: Point{Column: 6}, EndPoint: Point{Column: 6}},
		},
	})
	if err != nil {
		t.Fatalf("ParseWithTokenSource failed: %v", err)
	}
	if got, want := tree.ParseRuntime().StopReason, ParseStopAccepted; got != want {
		t.Fatalf("parse stop reason = %q, want %q; tree=%v", got, want, tree.RootNode())
	}
	if got := countGapTestNodesWithSymbolSpan(tree.RootNode(), errorSymbol, 1, 4); got != 1 {
		t.Fatalf("ERROR span 1..4 count = %d, want 1; tree=%v", got, tree.RootNode())
	}
}

func buildSkippedRealGapLanguage() *Language {
	lang := buildArithmeticRecoverLanguage()
	lang.Name = "skipped_real_gap"
	lang.SymbolNames = []string{"EOF", "NUMBER", "+", "BAD", "expression"}
	lang.SymbolMetadata[3] = SymbolMetadata{Name: "BAD", Visible: true, Named: true}
	// Keep BAD reducible after NUMBER, but illegal both at the start state and
	// after a completed expression. This forces ordinary error materialization
	// before the following PLUS lookahead skips additional real bytes.
	lang.ParseTable[0][3] = 0
	lang.ParseTable[1][3] = 2
	lang.ParseTable[2][3] = 0
	return lang
}

// TestSkippedRealGapContinuesSeparatedList directly exercises
// skippedRealGapContinuesSeparatedList (and, transitively,
// stateDeterministicNonExtraShift) against a synthetic language, isolating
// each guard clause in turn: only a sole stack whose top is an anonymous
// separator terminal with a real span, followed by a lookahead whose state
// has exactly one non-extra shift action, should report true.
func TestSkippedRealGapContinuesSeparatedList(t *testing.T) {
	const (
		symComma  Symbol = 1
		symNumber Symbol = 2
		symList   Symbol = 3
	)
	const (
		stateDeterministicShift StateID = 1
		stateExtraShiftOnly     StateID = 2
		stateAmbiguousActions   StateID = 3
	)

	lang := buildSkippedRealGapSeparatedListLanguage()
	parser := NewParser(lang)

	// The real lookahead is identical across every case; only the stack top
	// and the state used to look up its action vary.
	lookahead := Token{
		Symbol:     symNumber,
		StartByte:  4,
		EndByte:    5,
		StartPoint: Point{Column: 4},
		EndPoint:   Point{Column: 5},
	}

	push := func(state StateID, top *Node) *glrStack {
		var entryScratch glrEntryScratch
		var gssScratch gssScratch
		stack := newGLRStackWithScratch(state, &entryScratch)
		parser.pushStackNode(&stack, state, top, &entryScratch, &gssScratch)
		return &stack
	}

	t.Run("a) anonymous separator with deterministic shift continuation", func(t *testing.T) {
		comma := NewLeafNode(symComma, false, 1, 2, Point{Column: 1}, Point{Column: 2})
		stack := push(stateDeterministicShift, comma)
		if !parser.skippedRealGapContinuesSeparatedList(stack, stateDeterministicShift, lookahead) {
			t.Fatal("skippedRealGapContinuesSeparatedList = false, want true for anonymous separator with a deterministic non-extra shift lookahead")
		}
	})

	t.Run("b) named top leaf is excluded", func(t *testing.T) {
		number := NewLeafNode(symNumber, true, 1, 2, Point{Column: 1}, Point{Column: 2})
		stack := push(stateDeterministicShift, number)
		if parser.skippedRealGapContinuesSeparatedList(stack, stateDeterministicShift, lookahead) {
			t.Fatal("skippedRealGapContinuesSeparatedList = true, want false for a named top leaf")
		}
	})

	t.Run("c) reduced nonterminal top is excluded", func(t *testing.T) {
		child := NewLeafNode(symComma, false, 1, 2, Point{Column: 1}, Point{Column: 2})
		list := NewParentNode(symList, true, []*Node{child}, nil, 0)
		stack := push(stateDeterministicShift, list)
		if parser.skippedRealGapContinuesSeparatedList(stack, stateDeterministicShift, lookahead) {
			t.Fatal("skippedRealGapContinuesSeparatedList = true, want false for a reduced (non-leaf) top")
		}
	})

	t.Run("d) lookahead whose only action is an extra shift is excluded", func(t *testing.T) {
		comma := NewLeafNode(symComma, false, 1, 2, Point{Column: 1}, Point{Column: 2})
		stack := push(stateExtraShiftOnly, comma)
		if parser.skippedRealGapContinuesSeparatedList(stack, stateExtraShiftOnly, lookahead) {
			t.Fatal("skippedRealGapContinuesSeparatedList = true, want false when the only lookahead action is an extra shift")
		}
	})

	t.Run("e) lookahead with multiple actions is excluded", func(t *testing.T) {
		comma := NewLeafNode(symComma, false, 1, 2, Point{Column: 1}, Point{Column: 2})
		stack := push(stateAmbiguousActions, comma)
		if parser.skippedRealGapContinuesSeparatedList(stack, stateAmbiguousActions, lookahead) {
			t.Fatal("skippedRealGapContinuesSeparatedList = true, want false when the lookahead state has more than one action")
		}
	})
}

// buildSkippedRealGapSeparatedListLanguage is a minimal synthetic language for
// exercising skippedRealGapContinuesSeparatedList's guard clauses in
// isolation: symbol 1 (",") is an anonymous separator terminal, symbol 2
// (NUMBER) is a named terminal used both as a named-leaf fixture and as the
// shared lookahead symbol, and symbol 3 ("list") is a nonterminal used as a
// reduced-top fixture. States 1-3 give the lookahead symbol a single
// non-extra shift, a single extra shift, and two conflicting actions,
// respectively; state 0 is unused.
func buildSkippedRealGapSeparatedListLanguage() *Language {
	return &Language{
		Name:              "skipped_real_gap_separated_list",
		SymbolCount:       4,
		TokenCount:        3,
		StateCount:        4,
		ProductionIDCount: 1,
		SymbolNames:       []string{"EOF", ",", "NUMBER", "list"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ",", Visible: true, Named: false},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "list", Visible: true, Named: true},
		},
		FieldNames: []string{""},
		ParseActions: []ParseActionEntry{
			// 0: no action
			{Actions: nil},
			// 1: single, non-extra shift on NUMBER (deterministic continuation)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 0}}},
			// 2: single, EXTRA shift on NUMBER (trivia-style attachment only)
			{Actions: []ParseAction{{Type: ParseActionShift, State: 0, Extra: true}}},
			// 3: two actions on NUMBER (ambiguous/conflicted lookahead)
			{Actions: []ParseAction{
				{Type: ParseActionShift, State: 0},
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0},
			}},
		},
		// Columns: EOF(0), ","(1), NUMBER(2), list(3)
		ParseTable: [][]uint16{
			{0, 0, 0, 0}, // state 0: unused
			{0, 0, 1, 0}, // state 1: deterministic non-extra shift on NUMBER
			{0, 0, 2, 0}, // state 2: EXTRA-only shift on NUMBER
			{0, 0, 3, 0}, // state 3: ambiguous (2 actions) on NUMBER
		},
		LexModes: []LexMode{
			{LexState: 0}, {LexState: 0}, {LexState: 0}, {LexState: 0},
		},
		LexStates: []LexState{{Default: -1, EOF: -1}},
	}
}

type recoverCommentGapTokenSource struct {
	src []byte
	pos int
	row uint32
	col uint32
}

func (ts *recoverCommentGapTokenSource) Next() Token {
	for ts.pos < len(ts.src) {
		switch ts.src[ts.pos] {
		case ' ', '\t', '\n':
			ts.advance()
			continue
		case '/':
			if ts.pos+1 < len(ts.src) && ts.src[ts.pos+1] == '*' {
				ts.skipBlockComment()
				continue
			}
		case '+':
			return ts.singleByteToken(2)
		case '*':
			return ts.singleByteToken(3)
		}
		if ts.src[ts.pos] >= '0' && ts.src[ts.pos] <= '9' {
			return ts.numberToken()
		}
		ts.advance()
	}
	pt := Point{Row: ts.row, Column: ts.col}
	return Token{StartByte: uint32(ts.pos), EndByte: uint32(ts.pos), StartPoint: pt, EndPoint: pt}
}

func (ts *recoverCommentGapTokenSource) singleByteToken(sym Symbol) Token {
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

func (ts *recoverCommentGapTokenSource) numberToken() Token {
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

func (ts *recoverCommentGapTokenSource) skipBlockComment() {
	ts.advance()
	ts.advance()
	for ts.pos < len(ts.src) {
		if ts.src[ts.pos] == '*' && ts.pos+1 < len(ts.src) && ts.src[ts.pos+1] == '/' {
			ts.advance()
			ts.advance()
			return
		}
		ts.advance()
	}
}

func (ts *recoverCommentGapTokenSource) advance() {
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

func buildExtraShiftGapLanguage() *Language {
	return &Language{
		Name:              "extra_shift_gap",
		SymbolCount:       3,
		TokenCount:        3,
		StateCount:        2,
		ProductionIDCount: 1,
		SymbolNames:       []string{"EOF", "a", "#"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "a", Visible: true, Named: false},
			{Name: "#", Visible: true, Named: false},
		},
		FieldNames: []string{""},
		ParseActions: []ParseActionEntry{
			{Actions: nil},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			{Actions: []ParseAction{{Type: ParseActionShift, State: 0, Extra: true}}},
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0},
			{3, 0, 2},
		},
		LexModes: []LexMode{{LexState: 0}, {LexState: 0}},
		LexStates: []LexState{{
			Default: -1,
			EOF:     -1,
		}},
	}
}

type extraShiftGapTokenSource struct {
	src []byte
	pos int
	col uint32
}

func (ts *extraShiftGapTokenSource) Next() Token {
	for ts.pos < len(ts.src) {
		if ts.src[ts.pos] == '/' && ts.pos+1 < len(ts.src) && ts.src[ts.pos+1] == '*' {
			ts.skipBlockComment()
			continue
		}
		switch ts.src[ts.pos] {
		case 'a':
			return ts.singleByteToken(1)
		case '#':
			return ts.singleByteToken(2)
		default:
			ts.pos++
			ts.col++
		}
	}
	pt := Point{Column: ts.col}
	return Token{StartByte: uint32(ts.pos), EndByte: uint32(ts.pos), StartPoint: pt, EndPoint: pt}
}

func (ts *extraShiftGapTokenSource) singleByteToken(sym Symbol) Token {
	start := ts.pos
	startPt := Point{Column: ts.col}
	ts.pos++
	ts.col++
	return Token{
		Symbol:     sym,
		StartByte:  uint32(start),
		EndByte:    uint32(ts.pos),
		StartPoint: startPt,
		EndPoint:   Point{Column: ts.col},
		Text:       string(ts.src[start:ts.pos]),
	}
}

func (ts *extraShiftGapTokenSource) skipBlockComment() {
	ts.pos += 2
	ts.col += 2
	for ts.pos < len(ts.src) {
		if ts.src[ts.pos] == '*' && ts.pos+1 < len(ts.src) && ts.src[ts.pos+1] == '/' {
			ts.pos += 2
			ts.col += 2
			return
		}
		ts.pos++
		ts.col++
	}
}

type skippedRealGapTokenSource struct {
	tokens []Token
	idx    int
}

func (ts *skippedRealGapTokenSource) Next() Token {
	if ts.idx >= len(ts.tokens) {
		return Token{}
	}
	tok := ts.tokens[ts.idx]
	ts.idx++
	return tok
}

func countGapTestNodesWithSymbolSpan(n *Node, sym Symbol, start, end uint32) int {
	if n == nil {
		return 0
	}
	count := 0
	if n.symbol == sym && n.StartByte() == start && n.EndByte() == end {
		count++
	}
	for i := 0; i < n.ChildCount(); i++ {
		count += countGapTestNodesWithSymbolSpan(n.Child(i), sym, start, end)
	}
	return count
}
