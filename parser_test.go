package gotreesitter

import "testing"

// buildArithmeticLanguage constructs a hand-built LR grammar for simple
// arithmetic expressions:
//
//	expression -> NUMBER
//	expression -> expression PLUS NUMBER
//
// Symbols:
//
//	0: EOF
//	1: NUMBER (terminal, named)
//	2: PLUS "+" (terminal, anonymous)
//	3: expression (nonterminal, named)
//
// LR States:
//
//	State 0 (start):       NUMBER -> shift 1, expression -> goto 2
//	State 1 (saw NUMBER):  any -> reduce expression->NUMBER (1 child)
//	State 2 (saw expr):    PLUS -> shift 3, EOF -> accept
//	State 3 (saw expr +):  NUMBER -> shift 4
//	State 4 (saw e+N):     any -> reduce expression->expression PLUS NUMBER (3 children)
//
// Lexer DFA:
//
//	State 0: start (dispatches digits, '+', whitespace)
//	State 1: in number (accept Symbol 1)
//	State 2: saw '+' (accept Symbol 2)
//	State 3: whitespace (skip)
func buildArithmeticLanguage() *Language {
	return &Language{
		Name:              "arithmetic",
		SymbolCount:       4,
		TokenCount:        3,
		ExternalTokenCount: 0,
		StateCount:        5,
		LargeStateCount:   0,
		FieldCount:        0,
		ProductionIDCount: 2,

		SymbolNames: []string{"EOF", "NUMBER", "+", "expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "+", Visible: true, Named: false},
			{Name: "expression", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		// ParseActions indexed by the action index stored in the parse table.
		//
		// Index 0: no-op / error (empty actions)
		// Index 1: Shift to state 1 (NUMBER in state 0)
		// Index 2: Reduce expression -> NUMBER (1 child, symbol 3, production 0)
		// Index 3: Shift to state 2 (GOTO for expression from state 0)
		// Index 4: Shift to state 3 (PLUS in state 2)
		// Index 5: Accept (EOF in state 2)
		// Index 6: Shift to state 4 (NUMBER in state 3)
		// Index 7: Reduce expression -> expr PLUS NUMBER (3 children, symbol 3, production 1)
		ParseActions: []ParseActionEntry{
			// 0: error / no action
			{Actions: nil},
			// 1: shift to state 1
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
			// 2: reduce expression -> NUMBER
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0}}},
			// 3: shift/goto to state 2
			{Actions: []ParseAction{{Type: ParseActionShift, State: 2}}},
			// 4: shift to state 3
			{Actions: []ParseAction{{Type: ParseActionShift, State: 3}}},
			// 5: accept
			{Actions: []ParseAction{{Type: ParseActionAccept}}},
			// 6: shift to state 4
			{Actions: []ParseAction{{Type: ParseActionShift, State: 4}}},
			// 7: reduce expression -> expression PLUS NUMBER
			{Actions: []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 3, ProductionID: 1}}},
		},

		// Dense parse table: [state][symbol] -> action index
		// Columns: EOF(0), NUMBER(1), PLUS(2), expression(3)
		ParseTable: [][]uint16{
			// State 0: shift NUMBER->1, goto expression->2
			{0, 1, 0, 3},
			// State 1: reduce on any terminal
			{2, 2, 2, 0},
			// State 2: accept on EOF, shift PLUS->3
			{5, 0, 4, 0},
			// State 3: shift NUMBER->4
			{0, 6, 0, 0},
			// State 4: reduce on any terminal
			{7, 7, 7, 0},
		},

		// All 5 parser states use the same lex DFA start state (0).
		LexModes: []LexMode{
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
			{LexState: 0},
		},

		// Lexer DFA for: NUMBER ([0-9]+), PLUS ('+'), whitespace (skip)
		LexStates: []LexState{
			// State 0: start
			{
				AcceptToken: 0,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
					{Lo: '+', Hi: '+', NextState: 2},
					{Lo: ' ', Hi: ' ', NextState: 3},
					{Lo: '\t', Hi: '\t', NextState: 3},
					{Lo: '\n', Hi: '\n', NextState: 3},
				},
			},
			// State 1: in number (accept NUMBER = symbol 1)
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: '0', Hi: '9', NextState: 1},
				},
			},
			// State 2: saw '+' (accept PLUS = symbol 2)
			{
				AcceptToken: 2,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: nil,
			},
			// State 3: whitespace (skip)
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
		},
	}
}

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

	tree := parser.Parse([]byte("42"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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

	tree := parser.Parse([]byte("1+2"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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
	tree := parser.Parse([]byte("1+2+3"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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

	tree := parser.Parse([]byte(""))
	if tree == nil {
		t.Fatal("Parse returned nil tree for empty input")
	}

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
	tree := parser.Parse([]byte("  1  +  2  "))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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
	tree := parser.Parse([]byte("+1"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("tree has nil root for error input")
	}

	// The tree should have an error somewhere.
	if !root.HasError() {
		t.Error("expected HasError=true for invalid input")
	}
}

func TestParserMultiDigitNumbers(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	tree := parser.Parse([]byte("123+456"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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
	tree := parser.Parse([]byte("1+2+3+4+5"))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
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

	tree := parser.Parse([]byte("1+2"))
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

	tree := parser.Parse([]byte("1+2"))
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

	tree := parser.Parse([]byte("1+2"))
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
	tree := parser.Parse(source)

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

	tree := parser.Parse([]byte("1+2"))
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
	tree := parser.Parse([]byte("   "))
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
	root := tree.RootNode()
	if root != nil {
		t.Errorf("expected nil root for whitespace-only input, got symbol %d", root.Symbol())
	}
}
