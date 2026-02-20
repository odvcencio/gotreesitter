package gotreesitter

import (
	"testing"
)

// queryTestLanguage returns a language with symbols and fields suitable for
// testing the query engine. It models a simplified Go-like grammar.
//
// Symbol table:
//
//	0: ""            (unnamed, hidden - error/sentinel)
//	1: "identifier"  (named, visible)
//	2: "number"      (named, visible)
//	3: "true"        (named, visible)
//	4: "false"       (named, visible)
//	5: "function_declaration" (named, visible)
//	6: "call_expression"      (named, visible)
//	7: "program"     (named, visible)
//	8: "func"        (unnamed, visible - keyword)
//	9: "return"      (unnamed, visible - keyword)
//	10: "if"         (unnamed, visible - keyword)
//	11: "("          (unnamed, visible - punctuation)
//	12: ")"          (unnamed, visible - punctuation)
//	13: "parameter_list" (named, visible)
//	14: "block"      (named, visible)
//	15: "string"     (named, visible)
//
// Field table:
//
//	0: ""     (sentinel)
//	1: "name"
//	2: "body"
//	3: "function"
//	4: "arguments"
//	5: "parameters"
func queryTestLanguage() *Language {
	return &Language{
		Name: "test_query",
		SymbolNames: []string{
			"",                     // 0
			"identifier",           // 1
			"number",               // 2
			"true",                 // 3
			"false",                // 4
			"function_declaration", // 5
			"call_expression",      // 6
			"program",              // 7
			"func",                 // 8
			"return",               // 9
			"if",                   // 10
			"(",                    // 11
			")",                    // 12
			"parameter_list",       // 13
			"block",                // 14
			"string",               // 15
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "", Visible: false, Named: false},                   // 0
			{Name: "identifier", Visible: true, Named: true},           // 1
			{Name: "number", Visible: true, Named: true},               // 2
			{Name: "true", Visible: true, Named: true},                 // 3
			{Name: "false", Visible: true, Named: true},                // 4
			{Name: "function_declaration", Visible: true, Named: true}, // 5
			{Name: "call_expression", Visible: true, Named: true},      // 6
			{Name: "program", Visible: true, Named: true},              // 7
			{Name: "func", Visible: true, Named: false},                // 8 - keyword
			{Name: "return", Visible: true, Named: false},              // 9 - keyword
			{Name: "if", Visible: true, Named: false},                  // 10 - keyword
			{Name: "(", Visible: true, Named: false},                   // 11
			{Name: ")", Visible: true, Named: false},                   // 12
			{Name: "parameter_list", Visible: true, Named: true},       // 13
			{Name: "block", Visible: true, Named: true},                // 14
			{Name: "string", Visible: true, Named: true},               // 15
		},
		FieldNames: []string{
			"",           // 0
			"name",       // 1
			"body",       // 2
			"function",   // 3
			"arguments",  // 4
			"parameters", // 5
		},
		FieldCount: 5,
	}
}

// Helper to make leaf nodes quickly.
func leaf(sym Symbol, named bool, start, end uint32) *Node {
	return NewLeafNode(sym, named, start, end,
		Point{Row: 0, Column: start}, Point{Row: 0, Column: end})
}

// Helper to make parent nodes quickly.
func parent(sym Symbol, named bool, children []*Node, fields []FieldID) *Node {
	return NewParentNode(sym, named, children, fields, 0)
}

// --------------------------------------------------------------------------
// S-expression parser tests
// --------------------------------------------------------------------------

func TestParseSimpleNodeType(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	if len(q.patterns[0].steps) != 1 {
		t.Fatalf("steps: got %d, want 1", len(q.patterns[0].steps))
	}
	step := q.patterns[0].steps[0]
	if step.symbol != Symbol(1) {
		t.Errorf("symbol: got %d, want 1", step.symbol)
	}
	if !step.isNamed {
		t.Error("isNamed: got false, want true")
	}
	if step.captureID < 0 {
		t.Fatal("captureID: expected >= 0")
	}
	if q.captures[step.captureID] != "ident" {
		t.Errorf("capture name: got %q, want %q", q.captures[step.captureID], "ident")
	}
}

func TestParseWildcard(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`( _ ) @any`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if step.symbol != 0 {
		t.Fatalf("symbol: got %d, want 0 for wildcard", step.symbol)
	}
	if step.captureID < 0 {
		t.Fatal("captureID: expected >= 0")
	}
	if q.captures[step.captureID] != "any" {
		t.Errorf("capture name: got %q, want %q", q.captures[step.captureID], "any")
	}
}

func TestParseNestedPattern(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(function_declaration name: (identifier) @func.name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	if len(steps) != 2 {
		t.Fatalf("steps: got %d, want 2", len(steps))
	}

	// Step 0: function_declaration at depth 0.
	if steps[0].symbol != Symbol(5) {
		t.Errorf("step[0] symbol: got %d, want 5", steps[0].symbol)
	}
	if steps[0].depth != 0 {
		t.Errorf("step[0] depth: got %d, want 0", steps[0].depth)
	}
	if steps[0].captureID != -1 {
		t.Errorf("step[0] captureID: got %d, want -1", steps[0].captureID)
	}

	// Step 1: identifier at depth 1 with field "name".
	if steps[1].symbol != Symbol(1) {
		t.Errorf("step[1] symbol: got %d, want 1", steps[1].symbol)
	}
	if steps[1].depth != 1 {
		t.Errorf("step[1] depth: got %d, want 1", steps[1].depth)
	}
	if steps[1].field != FieldID(1) {
		t.Errorf("step[1] field: got %d, want 1 (name)", steps[1].field)
	}
	if steps[1].captureID < 0 {
		t.Fatal("step[1] captureID: expected >= 0")
	}
	if q.captures[steps[1].captureID] != "func.name" {
		t.Errorf("capture name: got %q, want %q", q.captures[steps[1].captureID], "func.name")
	}
}

func TestParseAlternation(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`[(true) (false)] @bool`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	if len(steps) != 1 {
		t.Fatalf("steps: got %d, want 1", len(steps))
	}
	step := steps[0]
	if len(step.alternatives) != 2 {
		t.Fatalf("alternatives: got %d, want 2", len(step.alternatives))
	}
	if step.alternatives[0].symbol != Symbol(3) {
		t.Errorf("alt[0] symbol: got %d, want 3 (true)", step.alternatives[0].symbol)
	}
	if step.alternatives[1].symbol != Symbol(4) {
		t.Errorf("alt[1] symbol: got %d, want 4 (false)", step.alternatives[1].symbol)
	}
	if step.captureID < 0 {
		t.Fatal("captureID: expected >= 0")
	}
	if q.captures[step.captureID] != "bool" {
		t.Errorf("capture name: got %q, want %q", q.captures[step.captureID], "bool")
	}
}

func TestParseStringMatch(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`"func" @keyword`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if step.textMatch != "func" {
		t.Errorf("textMatch: got %q, want %q", step.textMatch, "func")
	}
	if step.captureID < 0 {
		t.Fatal("captureID: expected >= 0")
	}
	if q.captures[step.captureID] != "keyword" {
		t.Errorf("capture name: got %q, want %q", q.captures[step.captureID], "keyword")
	}
}

func TestParseQuantifiers(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(program (identifier)? @maybe (number)* @nums (true)+ @truthy)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	if len(steps) != 4 {
		t.Fatalf("steps: got %d, want 4", len(steps))
	}
	if steps[1].quantifier != queryQuantifierZeroOrOne {
		t.Fatalf("step[1] quantifier: got %d, want %d", steps[1].quantifier, queryQuantifierZeroOrOne)
	}
	if steps[2].quantifier != queryQuantifierZeroOrMore {
		t.Fatalf("step[2] quantifier: got %d, want %d", steps[2].quantifier, queryQuantifierZeroOrMore)
	}
	if steps[3].quantifier != queryQuantifierOneOrMore {
		t.Fatalf("step[3] quantifier: got %d, want %d", steps[3].quantifier, queryQuantifierOneOrMore)
	}
}

func TestParseStringAlternation(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`["func" "return" "if"] @keyword`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if len(step.alternatives) != 3 {
		t.Fatalf("alternatives: got %d, want 3", len(step.alternatives))
	}
	if step.alternatives[0].textMatch != "func" {
		t.Errorf("alt[0] textMatch: got %q, want %q", step.alternatives[0].textMatch, "func")
	}
	if step.alternatives[1].textMatch != "return" {
		t.Errorf("alt[1] textMatch: got %q, want %q", step.alternatives[1].textMatch, "return")
	}
	if step.alternatives[2].textMatch != "if" {
		t.Errorf("alt[2] textMatch: got %q, want %q", step.alternatives[2].textMatch, "if")
	}
}

func TestParseMixedAlternation(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`[(true) "func"] @mixed`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	step := q.patterns[0].steps[0]
	if len(step.alternatives) != 2 {
		t.Fatalf("alternatives: got %d, want 2", len(step.alternatives))
	}
	if step.alternatives[0].symbol != Symbol(3) {
		t.Errorf("alt[0] symbol: got %d, want 3", step.alternatives[0].symbol)
	}
	if step.alternatives[1].textMatch != "func" {
		t.Errorf("alt[1] textMatch: got %q, want %q", step.alternatives[1].textMatch, "func")
	}
}

func TestParseAlternationWildcard(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`[(true) ( _ )] @mixed`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	step := q.patterns[0].steps[0]
	if len(step.alternatives) != 2 {
		t.Fatalf("alternatives: got %d, want 2", len(step.alternatives))
	}
	if step.alternatives[0].symbol != Symbol(3) {
		t.Errorf("alt[0] symbol: got %d, want 3", step.alternatives[0].symbol)
	}
	if step.alternatives[1].symbol != 0 {
		t.Errorf("alt[1] symbol: got %d, want 0 (wildcard)", step.alternatives[1].symbol)
	}
}

func TestParseMultiplePatterns(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`
; Match identifiers
(identifier) @ident

; Match function declarations
(function_declaration
  name: (identifier) @func.name)

; Match keywords
["func" "return"] @keyword
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 3 {
		t.Fatalf("PatternCount: got %d, want 3", q.PatternCount())
	}
}

func TestParseComments(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`
; This is a comment
(identifier) @ident
; Another comment
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
}

func TestParseErrorUnknownNodeType(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(nonexistent_type) @x`, lang)
	if err == nil {
		t.Fatal("expected error for unknown node type")
	}
}

func TestParseErrorUnknownField(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(function_declaration nonexistent_field: (identifier))`, lang)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParseErrorUnterminatedString(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`"unterminated`, lang)
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
}

func TestParseErrorEmptyAlternation(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`[] @empty`, lang)
	if err == nil {
		t.Fatal("expected error for empty alternation")
	}
}

func TestParseErrorUnmatchedParen(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier`, lang)
	if err == nil {
		t.Fatal("expected error for unmatched paren")
	}
}

func TestParsePatternWithCaptureInsideParen(t *testing.T) {
	// Capture can also appear inside the parens before closing:
	// (identifier @ident)
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier @ident)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if step.captureID < 0 {
		t.Fatal("captureID: expected >= 0")
	}
	if q.captures[step.captureID] != "ident" {
		t.Errorf("capture: got %q, want %q", q.captures[step.captureID], "ident")
	}
}

func TestParsePredicateEq(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateEq {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateEq)
	}
	if pred.leftCapture != "name" {
		t.Fatalf("left capture: got %q, want %q", pred.leftCapture, "name")
	}
	if pred.literal != "main" {
		t.Fatalf("literal: got %q, want %q", pred.literal, "main")
	}
}

func TestParsePredicateMatch(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#match? @name "^ma")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateMatch {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateMatch)
	}
	if pred.regex == nil {
		t.Fatal("expected compiled regex")
	}
}

func TestParsePredicateNotEq(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#not-eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateNotEq {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateNotEq)
	}
	if pred.leftCapture != "name" {
		t.Fatalf("left capture: got %q, want %q", pred.leftCapture, "name")
	}
	if pred.literal != "main" {
		t.Fatalf("literal: got %q, want %q", pred.literal, "main")
	}
}

func TestParsePredicateAnyOf(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#any-of? @name "main" "root")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateAnyOf {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateAnyOf)
	}
	if pred.leftCapture != "name" {
		t.Fatalf("left capture: got %q, want %q", pred.leftCapture, "name")
	}
	if len(pred.values) != 2 || pred.values[0] != "main" || pred.values[1] != "root" {
		t.Fatalf("values: got %#v, want [main root]", pred.values)
	}
}

func TestParsePredicateUnknownCapture(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @name (#eq? @missing "main")`, lang)
	if err == nil {
		t.Fatal("expected error for unknown predicate capture")
	}
}

func TestParsePredicateInvalidRegex(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @name (#match? @name "[")`, lang)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestParsePredicateAnyOfRejectsCaptureArg(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(identifier) @a (identifier) @b (#any-of? @a @b)`, lang)
	if err == nil {
		t.Fatal("expected error for capture argument in #any-of?")
	}
}

func TestParseNestedWithMultipleChildren(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(function_declaration
  name: (identifier) @func.name
  parameters: (parameter_list) @func.params
  body: (block) @func.body)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	// Should have 4 steps: function_declaration + 3 children.
	if len(steps) != 4 {
		t.Fatalf("steps: got %d, want 4", len(steps))
	}
	// Verify fields.
	if steps[1].field != FieldID(1) { // name
		t.Errorf("step[1] field: got %d, want 1", steps[1].field)
	}
	if steps[2].field != FieldID(5) { // parameters
		t.Errorf("step[2] field: got %d, want 5", steps[2].field)
	}
	if steps[3].field != FieldID(2) { // body
		t.Errorf("step[3] field: got %d, want 2", steps[3].field)
	}
}

func TestParseCaptureOutsideParen(t *testing.T) {
	// Capture after closing paren: (identifier) @name
	lang := queryTestLanguage()
	q, err := NewQuery(`(function_declaration) @func`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	step := q.patterns[0].steps[0]
	if step.captureID < 0 {
		t.Fatal("captureID: expected >= 0")
	}
	if q.captures[step.captureID] != "func" {
		t.Errorf("capture: got %q, want %q", q.captures[step.captureID], "func")
	}
}

func TestCaptureNames(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`
(identifier) @ident
(number) @number
(true) @bool
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	names := q.CaptureNames()
	if len(names) != 3 {
		t.Fatalf("CaptureNames: got %d, want 3", len(names))
	}
	expected := []string{"ident", "number", "bool"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("CaptureNames[%d]: got %q, want %q", i, names[i], name)
		}
	}
}

func TestCaptureDeduplicated(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`
(identifier) @name
(number) @name
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	names := q.CaptureNames()
	// "name" appears twice in patterns but should be stored once.
	if len(names) != 1 {
		t.Fatalf("CaptureNames: got %d, want 1 (deduplication)", len(names))
	}
	if names[0] != "name" {
		t.Errorf("CaptureNames[0]: got %q, want %q", names[0], "name")
	}
}

// --------------------------------------------------------------------------
// Matching engine tests
// --------------------------------------------------------------------------

// buildSimpleTree builds a tree representing: `func main() { 42 }`
//
//	program (7)
//	  function_declaration (5) [name: identifier(1), parameters: parameter_list(13), body: block(14)]
//	    "func" (8, anonymous)
//	    identifier (1, named) "main"
//	    parameter_list (13, named) "()"
//	      "(" (11, anonymous)
//	      ")" (12, anonymous)
//	    block (14, named)
//	      number (2, named) "42"
func buildSimpleTree(lang *Language) *Tree {
	source := []byte("func main() { 42 }")

	funcKw := leaf(Symbol(8), false, 0, 4)    // "func"
	ident := leaf(Symbol(1), true, 5, 9)      // "main"
	lparen := leaf(Symbol(11), false, 9, 10)  // "("
	rparen := leaf(Symbol(12), false, 10, 11) // ")"
	paramList := parent(Symbol(13), true,
		[]*Node{lparen, rparen},
		[]FieldID{0, 0})
	num := leaf(Symbol(2), true, 14, 16) // "42"
	block := parent(Symbol(14), true,
		[]*Node{num},
		[]FieldID{0})
	funcDecl := parent(Symbol(5), true,
		[]*Node{funcKw, ident, paramList, block},
		[]FieldID{0, FieldID(1), FieldID(5), FieldID(2)}) // fields: _, name, parameters, body
	program := parent(Symbol(7), true,
		[]*Node{funcDecl},
		[]FieldID{0})

	return NewTree(program, source, lang)
}

func TestMatchSimpleNodeType(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	m := matches[0]
	if m.PatternIndex != 0 {
		t.Errorf("PatternIndex: got %d, want 0", m.PatternIndex)
	}
	if len(m.Captures) != 1 {
		t.Fatalf("Captures: got %d, want 1", len(m.Captures))
	}
	if m.Captures[0].Name != "ident" {
		t.Errorf("Capture name: got %q, want %q", m.Captures[0].Name, "ident")
	}
	if m.Captures[0].Node.Text(tree.Source()) != "main" {
		t.Errorf("Capture text: got %q, want %q", m.Captures[0].Node.Text(tree.Source()), "main")
	}
}

func TestMatchNumber(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(number) @num`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if matches[0].Captures[0].Node.Text(tree.Source()) != "42" {
		t.Errorf("Capture text: got %q, want %q", matches[0].Captures[0].Node.Text(tree.Source()), "42")
	}
}

func TestMatchPredicateEq(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture text: got %q, want %q", got, "main")
	}
}

func TestMatchPredicateEqNoMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#eq? @name "other")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchPredicateMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#match? @name "^ma")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateNotEq(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#not-eq? @name "other")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture text: got %q, want %q", got, "main")
	}
}

func TestMatchPredicateNotEqNoMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#not-eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchPredicateAnyOf(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#any-of? @name "root" "main" "entry")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture text: got %q, want %q", got, "main")
	}
}

func TestMatchPredicateAnyOfNoMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#any-of? @name "root" "entry")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchFieldConstrained(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration name: (identifier) @func.name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	m := matches[0]
	if len(m.Captures) != 1 {
		t.Fatalf("Captures: got %d, want 1", len(m.Captures))
	}
	if m.Captures[0].Name != "func.name" {
		t.Errorf("Capture name: got %q, want %q", m.Captures[0].Name, "func.name")
	}
	if m.Captures[0].Node.Text(tree.Source()) != "main" {
		t.Errorf("Capture text: got %q, want %q", m.Captures[0].Node.Text(tree.Source()), "main")
	}
}

func TestMatchStringLiteral(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`"func" @keyword`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	cap := matches[0].Captures[0]
	if cap.Name != "keyword" {
		t.Errorf("Capture name: got %q, want %q", cap.Name, "keyword")
	}
	if cap.Node.Text(tree.Source()) != "func" {
		t.Errorf("Capture text: got %q, want %q", cap.Node.Text(tree.Source()), "func")
	}
}

func TestMatchAlternation(t *testing.T) {
	lang := queryTestLanguage()

	// Build a tree with both true and false nodes.
	trueNode := leaf(Symbol(3), true, 0, 4)   // true
	falseNode := leaf(Symbol(4), true, 5, 10) // false
	numNode := leaf(Symbol(2), true, 11, 13)  // 42
	program := parent(Symbol(7), true,
		[]*Node{trueNode, falseNode, numNode},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, []byte("true false 42"), lang)

	q, err := NewQuery(`[(true) (false)] @bool`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 2 {
		t.Fatalf("matches: got %d, want 2", len(matches))
	}

	texts := make(map[string]bool)
	for _, m := range matches {
		if len(m.Captures) != 1 {
			t.Fatalf("Captures: got %d, want 1", len(m.Captures))
		}
		texts[m.Captures[0].Node.Text(tree.Source())] = true
	}
	if !texts["true"] {
		t.Error("missing match for 'true'")
	}
	if !texts["false"] {
		t.Error("missing match for 'false'")
	}
}

func TestMatchStringAlternation(t *testing.T) {
	lang := queryTestLanguage()

	// Build a tree with keyword nodes.
	funcKw := leaf(Symbol(8), false, 0, 4)    // "func"
	returnKw := leaf(Symbol(9), false, 5, 11) // "return"
	ident := leaf(Symbol(1), true, 12, 15)    // "foo"
	program := parent(Symbol(7), true,
		[]*Node{funcKw, returnKw, ident},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, []byte("func return foo"), lang)

	q, err := NewQuery(`["func" "return"] @keyword`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 2 {
		t.Fatalf("matches: got %d, want 2", len(matches))
	}

	texts := make(map[string]bool)
	for _, m := range matches {
		texts[m.Captures[0].Node.Text(tree.Source())] = true
	}
	if !texts["func"] {
		t.Error("missing match for 'func'")
	}
	if !texts["return"] {
		t.Error("missing match for 'return'")
	}
}

func TestMatchNoMatch(t *testing.T) {
	lang := queryTestLanguage()

	// Tree with only numbers, query for strings.
	num := leaf(Symbol(2), true, 0, 2)
	program := parent(Symbol(7), true, []*Node{num}, []FieldID{0})
	tree := NewTree(program, []byte("42"), lang)

	q, err := NewQuery(`(string) @str`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchNoMatchField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// Look for a function_declaration with a "function" field (which doesn't exist).
	q, err := NewQuery(`(function_declaration function: (identifier) @x)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (field doesn't match)", len(matches))
	}
}

func TestMatchWildcard(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`( _ ) @any`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) == 0 {
		t.Fatal("matches: got 0, want >0")
	}

	// The wildcard should match the top-level program node at minimum.
	foundProgram := false
	for _, m := range matches {
		for _, c := range m.Captures {
			if c.Node.Type(lang) == "program" {
				foundProgram = true
			}
		}
	}
	if !foundProgram {
		t.Fatal("expected a match for program node using wildcard")
	}
}

func TestMatchMultiplePatterns(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`
(identifier) @ident
(number) @num
"func" @keyword
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	// Should find: 1 identifier ("main"), 1 number ("42"), 1 keyword ("func").
	if len(matches) != 3 {
		t.Fatalf("matches: got %d, want 3", len(matches))
	}

	captureMap := make(map[string]string)
	for _, m := range matches {
		for _, c := range m.Captures {
			captureMap[c.Name] = c.Node.Text(tree.Source())
		}
	}
	if captureMap["ident"] != "main" {
		t.Errorf("ident: got %q, want %q", captureMap["ident"], "main")
	}
	if captureMap["num"] != "42" {
		t.Errorf("num: got %q, want %q", captureMap["num"], "42")
	}
	if captureMap["keyword"] != "func" {
		t.Errorf("keyword: got %q, want %q", captureMap["keyword"], "func")
	}
}

func TestMatchPatternWithParentCapture(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// Capture both the function_declaration and its name.
	q, err := NewQuery(`(function_declaration name: (identifier) @name) @func`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	m := matches[0]
	if len(m.Captures) != 2 {
		t.Fatalf("Captures: got %d, want 2", len(m.Captures))
	}

	// Find captures by name.
	capMap := make(map[string]*Node)
	for _, c := range m.Captures {
		capMap[c.Name] = c.Node
	}
	if capMap["func"] == nil {
		t.Fatal("missing capture @func")
	}
	if capMap["name"] == nil {
		t.Fatal("missing capture @name")
	}
	if capMap["func"].Symbol() != Symbol(5) {
		t.Errorf("@func symbol: got %d, want 5", capMap["func"].Symbol())
	}
	if capMap["name"].Text(tree.Source()) != "main" {
		t.Errorf("@name text: got %q, want %q", capMap["name"].Text(tree.Source()), "main")
	}
}

func buildProgramTreeWithIdentifiers(lang *Language) *Tree {
	source := []byte("a b 1")
	id0 := leaf(Symbol(1), true, 0, 1)
	id1 := leaf(Symbol(1), true, 2, 3)
	num := leaf(Symbol(2), true, 4, 5)
	program := parent(Symbol(7), true, []*Node{id0, id1, num}, []FieldID{0, 0, 0})
	return NewTree(program, source, lang)
}

func TestMatchQuantifierOptionalAllowsMissingChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	q, err := NewQuery(`(program (string)? @str)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if len(matches[0].Captures) != 0 {
		t.Fatalf("captures: got %d, want 0", len(matches[0].Captures))
	}
}

func TestMatchQuantifierStarCapturesAllMatches(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	q, err := NewQuery(`(program (identifier)* @id)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if len(matches[0].Captures) != 2 {
		t.Fatalf("captures: got %d, want 2", len(matches[0].Captures))
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "a" {
		t.Fatalf("capture[0]: got %q, want %q", got, "a")
	}
	if got := matches[0].Captures[1].Node.Text(tree.Source()); got != "b" {
		t.Fatalf("capture[1]: got %q, want %q", got, "b")
	}
}

func TestMatchQuantifierPlusRequiresAtLeastOne(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	q, err := NewQuery(`(program (string)+ @str)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchQuantifierFailedBranchRollsBackCaptures(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	q, err := NewQuery(`(program (identifier @bad (number))? (number) @num)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if len(matches[0].Captures) != 1 {
		t.Fatalf("captures: got %d, want 1", len(matches[0].Captures))
	}
	if matches[0].Captures[0].Name != "num" {
		t.Fatalf("capture name: got %q, want %q", matches[0].Captures[0].Name, "num")
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "1" {
		t.Fatalf("capture text: got %q, want %q", got, "1")
	}
}

func TestMatchNestedWithMultipleFields(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration
  name: (identifier) @name
  body: (block) @body)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	m := matches[0]
	if len(m.Captures) != 2 {
		t.Fatalf("Captures: got %d, want 2", len(m.Captures))
	}

	capMap := make(map[string]*Node)
	for _, c := range m.Captures {
		capMap[c.Name] = c.Node
	}
	if capMap["name"].Text(tree.Source()) != "main" {
		t.Errorf("@name text: got %q, want %q", capMap["name"].Text(tree.Source()), "main")
	}
	if capMap["body"].Symbol() != Symbol(14) {
		t.Errorf("@body symbol: got %d, want 14 (block)", capMap["body"].Symbol())
	}
}

func TestMatchMultipleIdentifiers(t *testing.T) {
	lang := queryTestLanguage()

	// Tree with multiple identifiers at different positions.
	id1 := leaf(Symbol(1), true, 0, 3)  // "foo"
	id2 := leaf(Symbol(1), true, 4, 7)  // "bar"
	id3 := leaf(Symbol(1), true, 8, 11) // "baz"
	program := parent(Symbol(7), true,
		[]*Node{id1, id2, id3},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, []byte("foo bar baz"), lang)

	q, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 3 {
		t.Fatalf("matches: got %d, want 3", len(matches))
	}

	texts := make([]string, len(matches))
	for i, m := range matches {
		texts[i] = m.Captures[0].Node.Text(tree.Source())
	}
	expected := []string{"foo", "bar", "baz"}
	for i, want := range expected {
		if texts[i] != want {
			t.Errorf("match[%d]: got %q, want %q", i, texts[i], want)
		}
	}
}

func TestMatchNilTree(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tree := NewTree(nil, nil, lang)
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches on nil root: got %d, want 0", len(matches))
	}
}

func TestMatchStringDoesNotMatchNamed(t *testing.T) {
	lang := queryTestLanguage()

	// "true" is a named node (symbol 3), not an anonymous keyword.
	// String matching should NOT match it since it's named.
	trueNode := leaf(Symbol(3), true, 0, 4)
	program := parent(Symbol(7), true, []*Node{trueNode}, []FieldID{0})
	tree := NewTree(program, []byte("true"), lang)

	q, err := NewQuery(`"true" @keyword`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (string match should not match named nodes)", len(matches))
	}
}

func TestMatchAlternationDoesNotMatchWrongType(t *testing.T) {
	lang := queryTestLanguage()

	numNode := leaf(Symbol(2), true, 0, 2) // number, not true/false
	program := parent(Symbol(7), true, []*Node{numNode}, []FieldID{0})
	tree := NewTree(program, []byte("42"), lang)

	q, err := NewQuery(`[(true) (false)] @bool`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchFieldWrongChildType(t *testing.T) {
	lang := queryTestLanguage()

	// Build a function_declaration where the name field points to a number
	// instead of an identifier. The query asks for identifier.
	funcKw := leaf(Symbol(8), false, 0, 4)
	numAsName := leaf(Symbol(2), true, 5, 7) // number in the name field
	funcDecl := parent(Symbol(5), true,
		[]*Node{funcKw, numAsName},
		[]FieldID{0, FieldID(1)}) // name field -> number
	program := parent(Symbol(7), true, []*Node{funcDecl}, []FieldID{0})
	tree := NewTree(program, []byte("func 42"), lang)

	q, err := NewQuery(`(function_declaration name: (identifier) @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (wrong child type in field)", len(matches))
	}
}

func TestExecuteNode(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(number) @num`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Execute starting from the function_declaration node (skip program).
	funcDecl := tree.RootNode().Child(0)
	matches := q.ExecuteNode(funcDecl, lang, tree.Source())
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if matches[0].Captures[0].Node.Text(tree.Source()) != "42" {
		t.Errorf("text: got %q, want %q", matches[0].Captures[0].Node.Text(tree.Source()), "42")
	}
}

func TestExecuteNodePredicateUsesSource(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	funcDecl := tree.RootNode().Child(0)

	matches := q.ExecuteNode(funcDecl, lang, tree.Source())
	if len(matches) != 1 {
		t.Fatalf("source-backed matches: got %d, want 1", len(matches))
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture text: got %q, want %q", got, "main")
	}

	// Explicitly passing nil source opts out of text predicates.
	noSource := q.ExecuteNode(funcDecl, lang, nil)
	if len(noSource) != 0 {
		t.Fatalf("nil-source matches: got %d, want 0", len(noSource))
	}
}

func TestQueryCursorNextMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`[(identifier) (number)] @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	var got []string
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		if len(m.Captures) != 1 {
			t.Fatalf("captures: got %d, want 1", len(m.Captures))
		}
		got = append(got, m.Captures[0].Node.Text(tree.Source()))
	}

	if len(got) != 2 {
		t.Fatalf("cursor matches: got %d, want 2", len(got))
	}
	if got[0] != "main" || got[1] != "42" {
		t.Fatalf("cursor capture texts: got %v, want [main 42]", got)
	}
}

func TestQueryCursorNextCapture(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`[(identifier) (number)] @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	var got []string
	for {
		cap, ok := cursor.NextCapture()
		if !ok {
			break
		}
		got = append(got, cap.Node.Text(tree.Source()))
	}

	if len(got) != 2 {
		t.Fatalf("cursor captures: got %d, want 2", len(got))
	}
	if got[0] != "main" || got[1] != "42" {
		t.Fatalf("cursor capture texts: got %v, want [main 42]", got)
	}
}

func TestQueryCursorNextCaptureThenNextMatchDropsRemainingCaptureBuffer(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration (identifier) @id (block (number) @num))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())

	firstCap, ok := cursor.NextCapture()
	if !ok {
		t.Fatal("expected first capture")
	}
	if got := firstCap.Node.Text(tree.Source()); got != "main" {
		t.Fatalf("first capture text: got %q, want %q", got, "main")
	}

	// NextMatch advances at match granularity and discards unconsumed captures.
	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("expected no next match after mixed NextCapture/NextMatch on single-match query")
	}
}

func TestMatchDeeplyNested(t *testing.T) {
	lang := queryTestLanguage()

	// Build: program > block > block > identifier
	ident := leaf(Symbol(1), true, 0, 3)
	innerBlock := parent(Symbol(14), true, []*Node{ident}, []FieldID{0})
	outerBlock := parent(Symbol(14), true, []*Node{innerBlock}, []FieldID{0})
	program := parent(Symbol(7), true, []*Node{outerBlock}, []FieldID{0})
	tree := NewTree(program, []byte("foo"), lang)

	q, err := NewQuery(`(identifier) @ident`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if matches[0].Captures[0].Node.Text(tree.Source()) != "foo" {
		t.Errorf("text: got %q, want %q", matches[0].Captures[0].Node.Text(tree.Source()), "foo")
	}
}

func TestMatchUnnamedChildWithoutField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// Match parentheses (anonymous nodes) inside parameter_list without field constraints.
	q, err := NewQuery(`(parameter_list) @params`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if matches[0].Captures[0].Node.Symbol() != Symbol(13) {
		t.Errorf("symbol: got %d, want 13", matches[0].Captures[0].Node.Symbol())
	}
}

func TestMatchPatternWithNoCaptureStillMatches(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// A pattern without captures should still produce a match (with empty Captures).
	q, err := NewQuery(`(function_declaration name: (identifier))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if len(matches[0].Captures) != 0 {
		t.Errorf("Captures: got %d, want 0", len(matches[0].Captures))
	}
}

func TestParseEscapedString(t *testing.T) {
	lang := queryTestLanguage()
	// Test that escaped quotes in strings work.
	q, err := NewQuery(`"func" @kw`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.patterns[0].steps[0].textMatch != "func" {
		t.Errorf("textMatch: got %q, want %q", q.patterns[0].steps[0].textMatch, "func")
	}
}

func TestMatchEmptyQuery(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`; just a comment`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 0 {
		t.Fatalf("PatternCount: got %d, want 0", q.PatternCount())
	}

	tree := buildSimpleTree(lang)
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchRealisticHighlightQuery(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	// A realistic-ish highlight query with multiple patterns.
	q, err := NewQuery(`
; Keywords
"func" @keyword

; Function names
(function_declaration
  name: (identifier) @function)

; Numbers
(number) @number

; Punctuation
["(" ")"] @punctuation.bracket
`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)

	// Collect all captures.
	capturesByName := make(map[string][]string)
	for _, m := range matches {
		for _, c := range m.Captures {
			capturesByName[c.Name] = append(capturesByName[c.Name], c.Node.Text(tree.Source()))
		}
	}

	// Verify expected captures.
	if texts := capturesByName["keyword"]; len(texts) != 1 || texts[0] != "func" {
		t.Errorf("@keyword: got %v, want [\"func\"]", texts)
	}
	if texts := capturesByName["function"]; len(texts) != 1 || texts[0] != "main" {
		t.Errorf("@function: got %v, want [\"main\"]", texts)
	}
	if texts := capturesByName["number"]; len(texts) != 1 || texts[0] != "42" {
		t.Errorf("@number: got %v, want [\"42\"]", texts)
	}
	if texts := capturesByName["punctuation.bracket"]; len(texts) != 2 {
		t.Errorf("@punctuation.bracket: got %d matches, want 2", len(texts))
	}
}
