package gotreesitter

import "testing"

// ---------------------------------------------------------------------------
// #count? predicate tests
// ---------------------------------------------------------------------------

func TestParsePredicateCount(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#count? @name ">=" "2")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	preds, ok := q.PredicatesForPattern(0)
	if !ok || len(preds) != 1 {
		t.Fatalf("expected 1 predicate, got %d (ok=%v)", len(preds), ok)
	}
	if preds[0].kind != predicateCount {
		t.Fatalf("kind: got %d, want predicateCount", preds[0].kind)
	}
	if preds[0].leftCapture != "name" {
		t.Fatalf("leftCapture: got %q, want %q", preds[0].leftCapture, "name")
	}
	if preds[0].countOp != ">=" {
		t.Fatalf("countOp: got %q, want %q", preds[0].countOp, ">=")
	}
	if preds[0].countValue != 2 {
		t.Fatalf("countValue: got %d, want %d", preds[0].countValue, 2)
	}
}

func TestParsePredicateCountBareOperator(t *testing.T) {
	lang := queryTestLanguage()
	// Operator and value as bare atoms (not quoted strings)
	q, err := NewQuery(`(identifier) @name (#count? @name > 1)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	preds, ok := q.PredicatesForPattern(0)
	if !ok || len(preds) != 1 {
		t.Fatalf("expected 1 predicate, got %d (ok=%v)", len(preds), ok)
	}
	if preds[0].countOp != ">" {
		t.Fatalf("countOp: got %q, want %q", preds[0].countOp, ">")
	}
	if preds[0].countValue != 1 {
		t.Fatalf("countValue: got %d, want %d", preds[0].countValue, 1)
	}
}

func TestParsePredicateCountInvalidOp(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @name (#count? @name "~" "2")`, lang)
	if err == nil {
		t.Fatal("expected error for invalid operator, got nil")
	}
}

func TestParsePredicateCountInvalidValue(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @name (#count? @name ">=" "abc")`, lang)
	if err == nil {
		t.Fatal("expected error for non-integer value, got nil")
	}
}

func TestParsePredicateCountFirstArgNotCapture(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @name (#count? "name" ">=" "2")`, lang)
	if err == nil {
		t.Fatal("expected error when first arg is not a capture, got nil")
	}
}

// Test #count? evaluation: simple tree has 1 identifier ("main").
func TestMatchPredicateCountEqPass(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#count? @name "==" "1")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateCountEqFail(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#count? @name "==" "5")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchPredicateCountGT(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// 1 > 0 => true
	q, err := NewQuery(`(identifier) @name (#count? @name ">" "0")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}

	// 1 > 1 => false
	q2, err := NewQuery(`(identifier) @name (#count? @name ">" "1")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches2 := q2.Execute(tree)
	if len(matches2) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches2))
	}
}

func TestMatchPredicateCountLT(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// 1 < 2 => true
	q, err := NewQuery(`(identifier) @name (#count? @name "<" "2")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}

	// 1 < 1 => false
	q2, err := NewQuery(`(identifier) @name (#count? @name "<" "1")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches2 := q2.Execute(tree)
	if len(matches2) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches2))
	}
}

func TestMatchPredicateCountGTE(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// 1 >= 1 => true
	q, err := NewQuery(`(identifier) @name (#count? @name ">=" "1")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}

	// 1 >= 2 => false
	q2, err := NewQuery(`(identifier) @name (#count? @name ">=" "2")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches2 := q2.Execute(tree)
	if len(matches2) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches2))
	}
}

func TestMatchPredicateCountLTE(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// 1 <= 1 => true
	q, err := NewQuery(`(identifier) @name (#count? @name "<=" "1")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}

	// 1 <= 0 => false
	q2, err := NewQuery(`(identifier) @name (#count? @name "<=" "0")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches2 := q2.Execute(tree)
	if len(matches2) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches2))
	}
}

func TestMatchPredicateCountNotEq(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// 1 != 2 => true
	q, err := NewQuery(`(identifier) @name (#count? @name "!=" "2")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}

	// 1 != 1 => false
	q2, err := NewQuery(`(identifier) @name (#count? @name "!=" "1")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches2 := q2.Execute(tree)
	if len(matches2) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches2))
	}
}

// ---------------------------------------------------------------------------
// #is-exported? predicate tests
// ---------------------------------------------------------------------------

func TestParsePredicateIsExported(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#is-exported? @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	preds, ok := q.PredicatesForPattern(0)
	if !ok || len(preds) != 1 {
		t.Fatalf("expected 1 predicate, got %d (ok=%v)", len(preds), ok)
	}
	if preds[0].kind != predicateIsExported {
		t.Fatalf("kind: got %d, want predicateIsExported", preds[0].kind)
	}
	if preds[0].leftCapture != "name" {
		t.Fatalf("leftCapture: got %q, want %q", preds[0].leftCapture, "name")
	}
}

func TestParsePredicateIsExportedFirstArgNotCapture(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @name (#is-exported? "name")`, lang)
	if err == nil {
		t.Fatal("expected error when first arg is not a capture, got nil")
	}
}

func TestMatchPredicateIsExportedUppercase(t *testing.T) {
	lang := queryTestLanguage()
	// Build a tree where identifier text is "Main" (starts uppercase).
	source := []byte("func Main() { 42 }")

	funcKw := leaf(Symbol(8), false, 0, 4)    // "func"
	ident := leaf(Symbol(1), true, 5, 9)      // "Main"
	lparen := leaf(Symbol(11), false, 9, 10)  // "("
	rparen := leaf(Symbol(12), false, 10, 11) // ")"
	paramList := parent(Symbol(13), true,
		[]*Node{lparen, rparen},
		[]FieldID{0, 0})
	num := leaf(Symbol(2), true, 14, 16)
	block := parent(Symbol(14), true,
		[]*Node{num},
		[]FieldID{0})
	funcDecl := parent(Symbol(5), true,
		[]*Node{funcKw, ident, paramList, block},
		[]FieldID{0, FieldID(1), FieldID(5), FieldID(2)})
	program := parent(Symbol(7), true,
		[]*Node{funcDecl},
		[]FieldID{0})
	tree := NewTree(program, source, lang)

	q, err := NewQuery(`(identifier) @name (#is-exported? @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "Main" {
		t.Fatalf("capture text: got %q, want %q", got, "Main")
	}
}

func TestMatchPredicateIsExportedLowercase(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang) // identifier is "main" (lowercase)

	q, err := NewQuery(`(identifier) @name (#is-exported? @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (lowercase identifier should not be exported)", len(matches))
	}
}

func TestMatchPredicateIsExportedEmptyText(t *testing.T) {
	lang := queryTestLanguage()
	// Build a tree where identifier has zero-length text.
	source := []byte("func () { 42 }")

	funcKw := leaf(Symbol(8), false, 0, 4)  // "func"
	ident := leaf(Symbol(1), true, 5, 5)    // "" (empty)
	lparen := leaf(Symbol(11), false, 5, 6) // "("
	rparen := leaf(Symbol(12), false, 6, 7) // ")"
	paramList := parent(Symbol(13), true,
		[]*Node{lparen, rparen},
		[]FieldID{0, 0})
	num := leaf(Symbol(2), true, 10, 12)
	block := parent(Symbol(14), true,
		[]*Node{num},
		[]FieldID{0})
	funcDecl := parent(Symbol(5), true,
		[]*Node{funcKw, ident, paramList, block},
		[]FieldID{0, FieldID(1), FieldID(5), FieldID(2)})
	program := parent(Symbol(7), true,
		[]*Node{funcDecl},
		[]FieldID{0})
	tree := NewTree(program, source, lang)

	q, err := NewQuery(`(identifier) @name (#is-exported? @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (empty text should not be exported)", len(matches))
	}
}

func TestMatchPredicateIsExportedUnicode(t *testing.T) {
	lang := queryTestLanguage()
	// Test with a Unicode uppercase letter (e.g. German sharp S uppercase: U+00D6 = "O")
	source := []byte("func \xc3\x96ffnen() { 42 }")

	funcKw := leaf(Symbol(8), false, 0, 4)    // "func"
	ident := leaf(Symbol(1), true, 5, 12)     // "Offnen" (starts with O-umlaut, uppercase)
	lparen := leaf(Symbol(11), false, 12, 13) // "("
	rparen := leaf(Symbol(12), false, 13, 14) // ")"
	paramList := parent(Symbol(13), true,
		[]*Node{lparen, rparen},
		[]FieldID{0, 0})
	num := leaf(Symbol(2), true, 17, 19)
	block := parent(Symbol(14), true,
		[]*Node{num},
		[]FieldID{0})
	funcDecl := parent(Symbol(5), true,
		[]*Node{funcKw, ident, paramList, block},
		[]FieldID{0, FieldID(1), FieldID(5), FieldID(2)})
	program := parent(Symbol(7), true,
		[]*Node{funcDecl},
		[]FieldID{0})
	tree := NewTree(program, source, lang)

	q, err := NewQuery(`(identifier) @name (#is-exported? @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1 (Unicode uppercase letter should be exported)", len(matches))
	}
}
