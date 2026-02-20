package gotreesitter

import "testing"

// buildIdentNumberWSDFA builds a DFA that recognizes:
//   - identifiers: [a-z]+  (Symbol 1)
//   - numbers:     [0-9]+  (Symbol 2)
//   - whitespace:  ' ' | '\n'  (Skip)
//
// States:
//
//	0: start state (dispatches to ident, number, or whitespace)
//	1: in identifier (accept Symbol 1)
//	2: in number (accept Symbol 2)
//	3: in whitespace (skip, accept)
func buildIdentNumberWSDFA() []LexState {
	return []LexState{
		// State 0: start — no accept
		{
			AcceptToken: 0,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 1},
				{Lo: '0', Hi: '9', NextState: 2},
				{Lo: ' ', Hi: ' ', NextState: 3},
				{Lo: '\n', Hi: '\n', NextState: 3},
			},
		},
		// State 1: identifier — accept Symbol 1
		{
			AcceptToken: 1,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 1},
			},
		},
		// State 2: number — accept Symbol 2
		{
			AcceptToken: 2,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: '0', Hi: '9', NextState: 2},
			},
		},
		// State 3: whitespace — skip
		{
			AcceptToken: 0,
			Skip:        true,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: ' ', Hi: ' ', NextState: 3},
				{Lo: '\n', Hi: '\n', NextState: 3},
			},
		},
	}
}

// TestBasicTokens verifies that the lexer recognizes identifiers, numbers,
// and automatically skips whitespace.
func TestBasicTokens(t *testing.T) {
	states := buildIdentNumberWSDFA()
	lex := NewLexer(states, []byte("hello 42 world"))

	// Token 1: "hello" (identifier, Symbol 1)
	tok := lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("token 1 Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "hello" {
		t.Errorf("token 1 Text = %q, want %q", tok.Text, "hello")
	}
	if tok.StartByte != 0 || tok.EndByte != 5 {
		t.Errorf("token 1 bytes = [%d,%d), want [0,5)", tok.StartByte, tok.EndByte)
	}

	// Token 2: "42" (number, Symbol 2)
	tok = lex.Next(0)
	if tok.Symbol != 2 {
		t.Errorf("token 2 Symbol = %d, want 2", tok.Symbol)
	}
	if tok.Text != "42" {
		t.Errorf("token 2 Text = %q, want %q", tok.Text, "42")
	}
	if tok.StartByte != 6 || tok.EndByte != 8 {
		t.Errorf("token 2 bytes = [%d,%d), want [6,8)", tok.StartByte, tok.EndByte)
	}

	// Token 3: "world" (identifier, Symbol 1)
	tok = lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("token 3 Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "world" {
		t.Errorf("token 3 Text = %q, want %q", tok.Text, "world")
	}
	if tok.StartByte != 9 || tok.EndByte != 14 {
		t.Errorf("token 3 bytes = [%d,%d), want [9,14)", tok.StartByte, tok.EndByte)
	}

	// Token 4: EOF
	tok = lex.Next(0)
	if tok.Symbol != 0 {
		t.Errorf("EOF token Symbol = %d, want 0", tok.Symbol)
	}
	if tok.StartByte != tok.EndByte {
		t.Errorf("EOF token StartByte(%d) != EndByte(%d)", tok.StartByte, tok.EndByte)
	}
}

// TestPositionTracking verifies that row/column positions are correctly
// tracked across newlines.
func TestPositionTracking(t *testing.T) {
	states := buildIdentNumberWSDFA()
	lex := NewLexer(states, []byte("ab\ncd"))

	// Token 1: "ab" at row 0, col 0..2
	tok := lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("token 1 Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "ab" {
		t.Errorf("token 1 Text = %q, want %q", tok.Text, "ab")
	}
	if tok.StartPoint.Row != 0 || tok.StartPoint.Column != 0 {
		t.Errorf("token 1 start = (%d,%d), want (0,0)", tok.StartPoint.Row, tok.StartPoint.Column)
	}
	if tok.EndPoint.Row != 0 || tok.EndPoint.Column != 2 {
		t.Errorf("token 1 end = (%d,%d), want (0,2)", tok.EndPoint.Row, tok.EndPoint.Column)
	}

	// Token 2: "cd" at row 1, col 0..2
	// The newline between "ab" and "cd" is whitespace and gets skipped.
	tok = lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("token 2 Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "cd" {
		t.Errorf("token 2 Text = %q, want %q", tok.Text, "cd")
	}
	if tok.StartPoint.Row != 1 || tok.StartPoint.Column != 0 {
		t.Errorf("token 2 start = (%d,%d), want (1,0)", tok.StartPoint.Row, tok.StartPoint.Column)
	}
	if tok.EndPoint.Row != 1 || tok.EndPoint.Column != 2 {
		t.Errorf("token 2 end = (%d,%d), want (1,2)", tok.EndPoint.Row, tok.EndPoint.Column)
	}
}

// TestEOF verifies EOF behavior for empty and single-character inputs.
func TestEOF(t *testing.T) {
	states := buildIdentNumberWSDFA()

	// Empty input: immediate EOF.
	lex := NewLexer(states, []byte(""))
	tok := lex.Next(0)
	if tok.Symbol != 0 {
		t.Errorf("empty EOF Symbol = %d, want 0", tok.Symbol)
	}
	if tok.StartByte != 0 || tok.EndByte != 0 {
		t.Errorf("empty EOF bytes = [%d,%d), want [0,0)", tok.StartByte, tok.EndByte)
	}

	// Single character: one token then EOF.
	lex = NewLexer(states, []byte("x"))
	tok = lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("single char Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "x" {
		t.Errorf("single char Text = %q, want %q", tok.Text, "x")
	}

	tok = lex.Next(0)
	if tok.Symbol != 0 {
		t.Errorf("after single char, EOF Symbol = %d, want 0", tok.Symbol)
	}
	if tok.StartByte != tok.EndByte {
		t.Errorf("EOF StartByte(%d) != EndByte(%d)", tok.StartByte, tok.EndByte)
	}
}

// TestWhitespaceSkipping verifies that leading and trailing whitespace
// is automatically skipped, yielding only the meaningful token.
func TestWhitespaceSkipping(t *testing.T) {
	states := buildIdentNumberWSDFA()
	lex := NewLexer(states, []byte("  hello  "))

	tok := lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "hello" {
		t.Errorf("Text = %q, want %q", tok.Text, "hello")
	}
	if tok.StartByte != 2 || tok.EndByte != 7 {
		t.Errorf("bytes = [%d,%d), want [2,7)", tok.StartByte, tok.EndByte)
	}

	// Next call should skip trailing spaces and return EOF.
	tok = lex.Next(0)
	if tok.Symbol != 0 {
		t.Errorf("after trailing spaces, Symbol = %d, want 0 (EOF)", tok.Symbol)
	}
}

// TestErrorRecovery verifies that unrecognized characters are skipped
// and the lexer continues to find valid tokens.
func TestErrorRecovery(t *testing.T) {
	states := buildIdentNumberWSDFA()
	// '@' and '#' are not in any transition — they should be skipped.
	lex := NewLexer(states, []byte("@#hello"))

	tok := lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("Symbol = %d, want 1", tok.Symbol)
	}
	if tok.Text != "hello" {
		t.Errorf("Text = %q, want %q", tok.Text, "hello")
	}
	if tok.StartByte != 2 || tok.EndByte != 7 {
		t.Errorf("bytes = [%d,%d), want [2,7)", tok.StartByte, tok.EndByte)
	}
}

// buildKeywordIdentDFA builds a DFA that can match:
//   - "if" as keyword (Symbol 1)
//   - identifiers [a-z]+ (Symbol 2)
//
// The DFA uses longest match, so "iffy" should match as identifier (Symbol 2),
// not "if" as keyword.
//
// States:
//
//	0: start
//	1: seen 'i' (no accept yet — could become "if" or longer ident)
//	2: seen 'if' — accept keyword Symbol 1, but continue on [a-z]
//	3: general identifier — accept Symbol 2
func buildKeywordIdentDFA() []LexState {
	return []LexState{
		// State 0: start
		{
			AcceptToken: 0,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'i', Hi: 'i', NextState: 1},
				{Lo: 'a', Hi: 'h', NextState: 3},
				{Lo: 'j', Hi: 'z', NextState: 3},
			},
		},
		// State 1: seen 'i'
		{
			AcceptToken: 2, // accept as ident if scanning stops here
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'f', Hi: 'f', NextState: 2},
				{Lo: 'a', Hi: 'e', NextState: 3},
				{Lo: 'g', Hi: 'z', NextState: 3},
			},
		},
		// State 2: seen "if" — accept keyword
		{
			AcceptToken: 1,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 3},
			},
		},
		// State 3: general identifier
		{
			AcceptToken: 2,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'z', NextState: 3},
			},
		},
	}
}

// TestLongestMatch verifies that the lexer uses maximal munch: "iffy" should
// be matched as a full identifier, not as keyword "if" + identifier "fy".
func TestLongestMatch(t *testing.T) {
	states := buildKeywordIdentDFA()

	// "iffy" should be a single identifier token, not "if" + "fy".
	lex := NewLexer(states, []byte("iffy"))
	tok := lex.Next(0)
	if tok.Symbol != 2 {
		t.Errorf("iffy: Symbol = %d, want 2 (identifier)", tok.Symbol)
	}
	if tok.Text != "iffy" {
		t.Errorf("iffy: Text = %q, want %q", tok.Text, "iffy")
	}
	if tok.StartByte != 0 || tok.EndByte != 4 {
		t.Errorf("iffy: bytes = [%d,%d), want [0,4)", tok.StartByte, tok.EndByte)
	}

	// "if" alone should be a keyword.
	lex = NewLexer(states, []byte("if"))
	tok = lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("if: Symbol = %d, want 1 (keyword)", tok.Symbol)
	}
	if tok.Text != "if" {
		t.Errorf("if: Text = %q, want %q", tok.Text, "if")
	}

	// EOF after keyword.
	tok = lex.Next(0)
	if tok.Symbol != 0 {
		t.Errorf("after if, EOF Symbol = %d, want 0", tok.Symbol)
	}
}

// TestNewLexerFields verifies that NewLexer initializes all fields correctly.
func TestNewLexerFields(t *testing.T) {
	states := buildIdentNumberWSDFA()
	source := []byte("test")
	lex := NewLexer(states, source)

	if len(lex.states) != len(states) {
		t.Errorf("states length = %d, want %d", len(lex.states), len(states))
	}
	if len(lex.source) != len(source) {
		t.Errorf("source length = %d, want %d", len(lex.source), len(source))
	}
	if lex.pos != 0 {
		t.Errorf("pos = %d, want 0", lex.pos)
	}
	if lex.row != 0 {
		t.Errorf("row = %d, want 0", lex.row)
	}
	if lex.col != 0 {
		t.Errorf("col = %d, want 0", lex.col)
	}
}

// TestMultipleNewlines verifies position tracking across multiple newlines.
func TestMultipleNewlines(t *testing.T) {
	states := buildIdentNumberWSDFA()
	lex := NewLexer(states, []byte("a\n\nb"))

	tok := lex.Next(0)
	if tok.Text != "a" {
		t.Errorf("token 1 Text = %q, want %q", tok.Text, "a")
	}
	if tok.StartPoint.Row != 0 {
		t.Errorf("token 1 row = %d, want 0", tok.StartPoint.Row)
	}

	tok = lex.Next(0)
	if tok.Text != "b" {
		t.Errorf("token 2 Text = %q, want %q", tok.Text, "b")
	}
	if tok.StartPoint.Row != 2 || tok.StartPoint.Column != 0 {
		t.Errorf("token 2 start = (%d,%d), want (2,0)", tok.StartPoint.Row, tok.StartPoint.Column)
	}
}

// TestDefaultTransition verifies that the Default field in LexState works
// as a fallback when no explicit transition matches.
func TestDefaultTransition(t *testing.T) {
	// DFA: state 0 transitions on 'a' to state 1, default to state 2.
	// State 1 accepts Symbol 1, state 2 accepts Symbol 2.
	states := []LexState{
		// State 0: start
		{
			AcceptToken: 0,
			Skip:        false,
			Default:     2,
			EOF:         -1,
			Transitions: []LexTransition{
				{Lo: 'a', Hi: 'a', NextState: 1},
			},
		},
		// State 1: accept Symbol 1
		{
			AcceptToken: 1,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: nil,
		},
		// State 2: accept Symbol 2
		{
			AcceptToken: 2,
			Skip:        false,
			Default:     -1,
			EOF:         -1,
			Transitions: nil,
		},
	}

	// 'a' should go through the explicit transition to state 1 (Symbol 1).
	lex := NewLexer(states, []byte("a"))
	tok := lex.Next(0)
	if tok.Symbol != 1 {
		t.Errorf("'a': Symbol = %d, want 1", tok.Symbol)
	}

	// 'x' should fall through to default state 2 (Symbol 2).
	lex = NewLexer(states, []byte("x"))
	tok = lex.Next(0)
	if tok.Symbol != 2 {
		t.Errorf("'x': Symbol = %d, want 2", tok.Symbol)
	}
}

// TestErrorInMiddle verifies that the lexer recovers from errors in the
// middle of input, not just at the beginning.
func TestErrorInMiddle(t *testing.T) {
	states := buildIdentNumberWSDFA()
	lex := NewLexer(states, []byte("ab@cd"))

	tok := lex.Next(0)
	if tok.Text != "ab" {
		t.Errorf("token 1 Text = %q, want %q", tok.Text, "ab")
	}

	tok = lex.Next(0)
	if tok.Text != "cd" {
		t.Errorf("token 2 Text = %q, want %q", tok.Text, "cd")
	}
}

// TestOnlyWhitespace verifies that input with only whitespace returns EOF.
func TestOnlyWhitespace(t *testing.T) {
	states := buildIdentNumberWSDFA()
	lex := NewLexer(states, []byte("   \n  \n"))

	tok := lex.Next(0)
	if tok.Symbol != 0 {
		t.Errorf("whitespace-only: Symbol = %d, want 0 (EOF)", tok.Symbol)
	}
	if tok.StartByte != tok.EndByte {
		t.Errorf("whitespace-only: StartByte(%d) != EndByte(%d)", tok.StartByte, tok.EndByte)
	}
}
