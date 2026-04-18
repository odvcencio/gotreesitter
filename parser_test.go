package gotreesitter

import "testing"

func TestNewParser(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	if parser == nil {
		t.Fatal("NewParser returned nil")
	}
	if parser.language != lang {
		t.Error("parser.language does not match the provided language")
	}
}

func TestParserSingleNumber(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("42"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root should be "expression".
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}
	if root.Type(lang) != "expression" {
		t.Errorf("root type = %q, want %q", root.Type(lang), "expression")
	}
	if !root.IsNamed() {
		t.Error("root IsNamed = false, want true")
	}

	// expression -> NUMBER: 1 child.
	if root.ChildCount() != 1 {
		t.Fatalf("root child count = %d, want 1", root.ChildCount())
	}

	child := root.Child(0)
	if child.Symbol() != 1 {
		t.Errorf("child symbol = %d, want 1 (NUMBER)", child.Symbol())
	}
	if child.Type(lang) != "NUMBER" {
		t.Errorf("child type = %q, want %q", child.Type(lang), "NUMBER")
	}
	if !child.IsNamed() {
		t.Error("NUMBER child IsNamed = false, want true")
	}

	// Verify the text span.
	if child.Text(tree.Source()) != "42" {
		t.Errorf("NUMBER text = %q, want %q", child.Text(tree.Source()), "42")
	}
	if child.StartByte() != 0 || child.EndByte() != 2 {
		t.Errorf("NUMBER bytes = [%d,%d), want [0,2)", child.StartByte(), child.EndByte())
	}
}

func TestParserSimpleExpression(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root should be "expression" with 3 children: expression, PLUS, NUMBER.
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	// Child 0: inner expression (expression -> NUMBER "1").
	inner := root.Child(0)
	if inner.Symbol() != 3 {
		t.Errorf("child 0 symbol = %d, want 3 (expression)", inner.Symbol())
	}
	if inner.ChildCount() != 1 {
		t.Fatalf("inner expression child count = %d, want 1", inner.ChildCount())
	}
	num1 := inner.Child(0)
	if num1.Text(tree.Source()) != "1" {
		t.Errorf("first NUMBER text = %q, want %q", num1.Text(tree.Source()), "1")
	}

	// Child 1: PLUS "+".
	plus := root.Child(1)
	if plus.Symbol() != 2 {
		t.Errorf("child 1 symbol = %d, want 2 (PLUS)", plus.Symbol())
	}
	if plus.IsNamed() {
		t.Error("PLUS IsNamed = true, want false")
	}
	if plus.Text(tree.Source()) != "+" {
		t.Errorf("PLUS text = %q, want %q", plus.Text(tree.Source()), "+")
	}

	// Child 2: NUMBER "2".
	num2 := root.Child(2)
	if num2.Symbol() != 1 {
		t.Errorf("child 2 symbol = %d, want 1 (NUMBER)", num2.Symbol())
	}
	if num2.Text(tree.Source()) != "2" {
		t.Errorf("second NUMBER text = %q, want %q", num2.Text(tree.Source()), "2")
	}
}

func TestParserChainedExpression(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// "1+2+3" should parse as left-associative: ((1)+2)+3
	tree := mustParse(t, parser, []byte("1+2+3"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root: expression -> expression PLUS NUMBER
	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3", root.Symbol())
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	// root.Child(2) should be NUMBER "3".
	num3 := root.Child(2)
	if num3.Text(tree.Source()) != "3" {
		t.Errorf("rightmost NUMBER text = %q, want %q", num3.Text(tree.Source()), "3")
	}

	// root.Child(0) should be an expression with 3 children (the "1+2" part).
	middle := root.Child(0)
	if middle.Symbol() != 3 {
		t.Errorf("middle expression symbol = %d, want 3", middle.Symbol())
	}
	if middle.ChildCount() != 3 {
		t.Fatalf("middle expression child count = %d, want 3", middle.ChildCount())
	}

	// middle.Child(0) is expression -> NUMBER "1".
	innerExpr := middle.Child(0)
	if innerExpr.Symbol() != 3 {
		t.Errorf("inner expression symbol = %d, want 3", innerExpr.Symbol())
	}
	if innerExpr.ChildCount() != 1 {
		t.Fatalf("inner expression child count = %d, want 1", innerExpr.ChildCount())
	}
	if innerExpr.Child(0).Text(tree.Source()) != "1" {
		t.Errorf("innermost NUMBER text = %q, want %q", innerExpr.Child(0).Text(tree.Source()), "1")
	}

	// middle.Child(2) is NUMBER "2".
	num2 := middle.Child(2)
	if num2.Text(tree.Source()) != "2" {
		t.Errorf("middle NUMBER text = %q, want %q", num2.Text(tree.Source()), "2")
	}
}

func TestParserEmptyInput(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte(""))

	// Empty input should produce a tree with nil root (nothing to parse).
	root := tree.RootNode()
	if root != nil {
		t.Errorf("expected nil root for empty input, got symbol %d with %d children",
			root.Symbol(), root.ChildCount())
	}
}

func TestParserWhitespace(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Whitespace between tokens should be handled correctly.
	tree := mustParse(t, parser, []byte("  1  +  2  "))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	if root.Symbol() != 3 {
		t.Errorf("root symbol = %d, want 3 (expression)", root.Symbol())
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	// Verify that the inner expression's NUMBER is "1" and the outer NUMBER is "2".
	inner := root.Child(0)
	if inner.ChildCount() < 1 {
		t.Fatal("inner expression has no children")
	}
	if inner.Child(0).Text(tree.Source()) != "1" {
		t.Errorf("first NUMBER text = %q, want %q", inner.Child(0).Text(tree.Source()), "1")
	}
	if root.Child(2).Text(tree.Source()) != "2" {
		t.Errorf("second NUMBER text = %q, want %q", root.Child(2).Text(tree.Source()), "2")
	}
}

func TestParserErrorRecovery(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// "+1" starts with PLUS which is invalid in state 0.
	// The parser should create an error node for "+" and then parse "1".
	tree := mustParse(t, parser, []byte("+1"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root for error input")
	}

	// The tree should have an error somewhere.
	if !root.HasError() {
		t.Error("expected HasError=true for invalid input")
	}
}

func TestParserPreservesPartialTreeOnNoStacksAlive(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+"))
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil tree/root")
	}
	root := tree.RootNode()
	switch got := tree.ParseStopReason(); got {
	case ParseStopNoStacksAlive:
		if gotText := root.Text(tree.Source()); gotText != "1+" {
			t.Fatalf("partial tree text = %q, want %q", gotText, "1+")
		}
	case ParseStopAccepted:
		if gotText := root.Text(tree.Source()); gotText != "1+" {
			t.Fatalf("accepted recovered text = %q, want %q", gotText, "1+")
		}
	default:
		t.Fatalf("ParseStopReason = %q, want %q or %q", got, ParseStopNoStacksAlive, ParseStopAccepted)
	}
	if got := root.Symbol(); got == errorSymbol {
		t.Fatalf("root symbol = %d, want partial preserved root", got)
	}
	if got := root.ChildCount(); got == 0 {
		t.Fatal("expected partial tree with children after no_stacks_alive")
	}
}

func TestCanFinalizeNoActionEOFRejectsFragmentStackWithInferredRoot(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	s.push(2, NewLeafNode(3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1}), nil, nil)
	s.push(3, NewLeafNode(2, false, 1, 2, Point{Row: 0, Column: 1}, Point{Row: 0, Column: 2}), nil, nil)

	if parser.canFinalizeNoActionEOF(&s) {
		t.Fatal("canFinalizeNoActionEOF() = true, want false for leftover fragments")
	}
}

func TestCanFinalizeNoActionEOFAcceptsSingleNonterminalWithExtras(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	extra := NewLeafNode(0, false, 0, 0, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 0})
	extra.isExtra = true
	s.push(0, extra, nil, nil)
	s.push(2, NewLeafNode(3, true, 0, 1, Point{Row: 0, Column: 0}, Point{Row: 0, Column: 1}), nil, nil)

	if !parser.canFinalizeNoActionEOF(&s) {
		t.Fatal("canFinalizeNoActionEOF() = false, want true for single nonterminal root")
	}
}

func TestPushOrExtendErrorNodeCoalescesConsecutiveTokens(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	s := newGLRStack(lang.InitialState)
	nodeCount := 0
	trackChildErrors := false

	parser.pushOrExtendErrorNode(&s, lang.InitialState, Token{
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}, &nodeCount, arena, nil, nil, &trackChildErrors)
	if got, want := s.depth(), 2; got != want {
		t.Fatalf("stack depth after first error = %d, want %d", got, want)
	}

	parser.pushOrExtendErrorNode(&s, lang.InitialState, Token{
		StartByte:  1,
		EndByte:    2,
		StartPoint: Point{Row: 0, Column: 1},
		EndPoint:   Point{Row: 0, Column: 2},
	}, &nodeCount, arena, nil, nil, &trackChildErrors)

	if got, want := s.depth(), 2; got != want {
		t.Fatalf("stack depth after extending error = %d, want %d", got, want)
	}
	if got, want := nodeCount, 1; got != want {
		t.Fatalf("nodeCount = %d, want %d", got, want)
	}
	top := s.top().node
	if top == nil {
		t.Fatal("top node is nil")
	}
	if got, want := top.Symbol(), errorSymbol; got != want {
		t.Fatalf("top symbol = %d, want %d", got, want)
	}
	if got, want := top.StartByte(), uint32(0); got != want {
		t.Fatalf("top.StartByte = %d, want %d", got, want)
	}
	if got, want := top.EndByte(), uint32(2); got != want {
		t.Fatalf("top.EndByte = %d, want %d", got, want)
	}
	if !trackChildErrors {
		t.Fatal("expected trackChildErrors=true")
	}
}

func TestParserRecoverAction(t *testing.T) {
	lang := buildArithmeticLanguage()

	// In this custom grammar, NUMBER should trigger ParseActionRecover.
	lang.ParseTable = [][]uint16{
		{0, 1}, // EOF has no action, NUMBER -> recover action.
		{0, 0},
	}
	lang.ParseActions = []ParseActionEntry{
		{}, // index 0 is unused / error
		{Actions: []ParseAction{{Type: ParseActionRecover}}},
	}

	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("1"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree root is nil after recover action")
	}

	if root.Symbol() != errorSymbol {
		t.Errorf("root symbol = %d, want %d (error symbol)", root.Symbol(), errorSymbol)
	}
	if !root.HasError() {
		t.Error("expected recovered parse root to have HasError=true")
	}
}

func TestParserAncestorRecoverActionPreservesLeftExpression(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+*2"))
	if tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	root := tree.RootNode()

	if root.Symbol() != 4 {
		t.Fatalf("root symbol = %d, want 4 (expression)", root.Symbol())
	}
	if !root.HasError() {
		t.Fatal("expected recovered tree to have HasError=true")
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	if got := root.Child(0).Symbol(); got != 4 {
		t.Fatalf("child[0] symbol = %d, want 4 (left expression preserved)", got)
	}
	if got := root.Child(1).Symbol(); got != errorSymbol {
		t.Fatalf("child[1] symbol = %d, want %d (error node)", got, errorSymbol)
	}
	if got := root.Child(2).Symbol(); got != 1 {
		t.Fatalf("child[2] symbol = %d, want 1 (NUMBER)", got)
	}
}
func TestParserMultiDigitNumbers(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("123+456"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}

	inner := root.Child(0)
	if inner.ChildCount() < 1 {
		t.Fatal("inner expression has no children")
	}
	if inner.Child(0).Text(tree.Source()) != "123" {
		t.Errorf("first NUMBER text = %q, want %q", inner.Child(0).Text(tree.Source()), "123")
	}
	if root.Child(2).Text(tree.Source()) != "456" {
		t.Errorf("second NUMBER text = %q, want %q", root.Child(2).Text(tree.Source()), "456")
	}
}
func TestParserLongChain(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// "1+2+3+4+5" — deeply left-nested.
	tree := mustParse(t, parser, []byte("1+2+3+4+5"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// The rightmost child should be NUMBER "5".
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}
	if root.Child(2).Text(tree.Source()) != "5" {
		t.Errorf("rightmost NUMBER text = %q, want %q", root.Child(2).Text(tree.Source()), "5")
	}

	// Walk down the left spine and count depth.
	depth := 0
	node := root
	for node.ChildCount() == 3 {
		node = node.Child(0)
		depth++
	}
	// "1+2+3+4+5" has 4 additions, so 4 levels of nesting.
	if depth != 4 {
		t.Errorf("left-nesting depth = %d, want 4", depth)
	}

	// The innermost expression should have 1 child (NUMBER "1").
	if node.ChildCount() != 1 {
		t.Errorf("innermost expression child count = %d, want 1", node.ChildCount())
	}
	if node.Child(0).Text(tree.Source()) != "1" {
		t.Errorf("innermost NUMBER text = %q, want %q", node.Child(0).Text(tree.Source()), "1")
	}
}

func TestParserByteSpans(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root expression should span the entire input [0, 3).
	if root.StartByte() != 0 {
		t.Errorf("root StartByte = %d, want 0", root.StartByte())
	}
	if root.EndByte() != 3 {
		t.Errorf("root EndByte = %d, want 3", root.EndByte())
	}

	// PLUS token at byte 1.
	plus := root.Child(1)
	if plus.StartByte() != 1 || plus.EndByte() != 2 {
		t.Errorf("PLUS bytes = [%d,%d), want [1,2)", plus.StartByte(), plus.EndByte())
	}
}

func TestParserPointPositions(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Check start/end points of the root.
	if root.StartPoint() != (Point{Row: 0, Column: 0}) {
		t.Errorf("root StartPoint = %v, want {0,0}", root.StartPoint())
	}
	if root.EndPoint() != (Point{Row: 0, Column: 3}) {
		t.Errorf("root EndPoint = %v, want {0,3}", root.EndPoint())
	}

	// NUMBER "2" starts at column 2.
	num2 := root.Child(2)
	if num2.StartPoint() != (Point{Row: 0, Column: 2}) {
		t.Errorf("NUMBER '2' StartPoint = %v, want {0,2}", num2.StartPoint())
	}
}

func TestParserParentPointers(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root has no parent.
	// (NewParentNode does not set the parent of the root itself.)

	// Each child should have the root as parent.
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child.Parent() != root {
			t.Errorf("child %d parent != root", i)
		}
	}

	// The inner expression's child should point to the inner expression.
	inner := root.Child(0)
	if inner.ChildCount() > 0 {
		if inner.Child(0).Parent() != inner {
			t.Error("inner expression's child has wrong parent")
		}
	}
}

func TestParserTreeMetadata(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	source := []byte("1+2")
	tree := mustParse(t, parser, source)

	if tree.Language() != lang {
		t.Error("tree.Language() does not match")
	}
	if string(tree.Source()) != "1+2" {
		t.Errorf("tree.Source() = %q, want %q", tree.Source(), "1+2")
	}
}

func TestParserNamedChildAccess(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := mustParse(t, parser, []byte("1+2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}

	// Root has 3 children: expression (named), PLUS (anonymous), NUMBER (named).
	// So NamedChildCount should be 2.
	if root.NamedChildCount() != 2 {
		t.Errorf("root NamedChildCount = %d, want 2", root.NamedChildCount())
	}

	// NamedChild(0) should be the expression.
	nc0 := root.NamedChild(0)
	if nc0 == nil || nc0.Symbol() != 3 {
		t.Errorf("NamedChild(0) symbol = %v, want 3 (expression)", nc0)
	}

	// NamedChild(1) should be the NUMBER "2".
	nc1 := root.NamedChild(1)
	if nc1 == nil || nc1.Symbol() != 1 {
		t.Errorf("NamedChild(1) symbol = %v, want 1 (NUMBER)", nc1)
	}
}

func TestParserLookupActionOutOfRange(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// State out of range.
	action := parser.lookupAction(StateID(999), Symbol(0))
	if action != nil {
		t.Error("expected nil for out-of-range state")
	}

	// Symbol out of range.
	action = parser.lookupAction(StateID(0), Symbol(999))
	if action != nil {
		t.Error("expected nil for out-of-range symbol")
	}
}

func TestParserIsNamedSymbol(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// EOF (0) is not named.
	if parser.isNamedSymbol(Symbol(0)) {
		t.Error("EOF should not be named")
	}
	// NUMBER (1) is named.
	if !parser.isNamedSymbol(Symbol(1)) {
		t.Error("NUMBER should be named")
	}
	// PLUS (2) is not named.
	if parser.isNamedSymbol(Symbol(2)) {
		t.Error("PLUS should not be named")
	}
	// expression (3) is named.
	if !parser.isNamedSymbol(Symbol(3)) {
		t.Error("expression should be named")
	}
	// Out of range symbol.
	if parser.isNamedSymbol(Symbol(999)) {
		t.Error("out-of-range symbol should not be named")
	}
}

func TestParserOnlyWhitespace(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	// Only whitespace — should produce empty tree like empty input.
	tree := mustParse(t, parser, []byte("   "))
	root := tree.RootNode()
	if root != nil {
		t.Errorf("expected nil root for whitespace-only input, got symbol %d", root.Symbol())
	}
}

type hashPlusExternalScanner struct{}

func (s *hashPlusExternalScanner) Create() any                           { return nil }
func (s *hashPlusExternalScanner) Destroy(payload any)                   {}
func (s *hashPlusExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (s *hashPlusExternalScanner) Deserialize(payload any, buf []byte)   {}
func (s *hashPlusExternalScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] {
		return false
	}
	if lexer.Lookahead() != '#' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(Symbol(2)) // PLUS
	return true
}

func TestParserExternalScannerToken(t *testing.T) {
	lang := buildArithmeticLanguage()
	lang.ExternalScanner = &hashPlusExternalScanner{}
	lang.ExternalSymbols = []Symbol{2} // PLUS token comes from external scanner

	parser := NewParser(lang)
	tree := mustParse(t, parser, []byte("1#2"))
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root")
	}
	if root.HasError() {
		t.Fatal("external scanner token path produced error tree")
	}
	if root.ChildCount() != 3 {
		t.Fatalf("root child count = %d, want 3", root.ChildCount())
	}
	if got := root.Child(1).Text(tree.Source()); got != "#" {
		t.Fatalf("operator text = %q, want %q", got, "#")
	}
}

// TestFieldIDsAlignAfterExtrasFold verifies that when buildResult folds
// extra nodes (e.g. leading comments) into a root's children, the fieldIDs
// slice is padded to maintain index alignment with children.
//
// Regression test for: prepending extras into realRoot.children without
func TestParserIncrementalArithmeticEditMatchesFreshParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	oldSrc := []byte("1+2")
	oldTree := mustParse(t, parser, oldSrc)

	newSrc := []byte("1+3")
	edit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	}
	oldTree.Edit(edit)

	incrTree := mustParseIncremental(t, parser, newSrc, oldTree)
	freshTree := mustParse(t, parser, newSrc)

	incrRoot := incrTree.RootNode()
	freshRoot := freshTree.RootNode()
	if incrRoot == nil || freshRoot == nil {
		t.Fatal("expected non-nil roots")
	}
	if got, want := incrRoot.SExpr(lang), freshRoot.SExpr(lang); got != want {
		t.Fatalf("incremental SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	if incrRoot.HasError() != freshRoot.HasError() {
		t.Fatalf("incremental HasError=%v, fresh HasError=%v", incrRoot.HasError(), freshRoot.HasError())
	}
}

func TestParserIncrementalArithmeticEditThenUndoMatchesFreshParse(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	originalSrc := []byte("1+2")
	tree := mustParse(t, parser, originalSrc)

	editedSrc := []byte("1+9")
	forwardEdit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	}
	tree.Edit(forwardEdit)
	tree = mustParseIncremental(t, parser, editedSrc, tree)

	undoEdit := InputEdit{
		StartByte:   2,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 2},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	}
	tree.Edit(undoEdit)
	incrUndo := mustParseIncremental(t, parser, originalSrc, tree)
	freshUndo := mustParse(t, parser, originalSrc)

	incrRoot := incrUndo.RootNode()
	freshRoot := freshUndo.RootNode()
	if incrRoot == nil || freshRoot == nil {
		t.Fatal("expected non-nil roots")
	}
	if got, want := incrRoot.SExpr(lang), freshRoot.SExpr(lang); got != want {
		t.Fatalf("incremental undo SExpr mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}
