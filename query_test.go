package gotreesitter

import (
	"regexp"
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

func queryTestLanguageWithSupertypes() *Language {
	lang := queryTestLanguage()
	lang.SymbolNames = append(lang.SymbolNames, "declaration")
	lang.SymbolMetadata = append(lang.SymbolMetadata, SymbolMetadata{
		Name:      "declaration",
		Visible:   true,
		Named:     true,
		Supertype: true,
	})
	lang.SupertypeSymbols = []Symbol{16}
	lang.SupertypeMapSlices = make([][2]uint16, 17)
	lang.SupertypeMapSlices[16] = [2]uint16{0, 1}
	lang.SupertypeMapEntries = []Symbol{5}
	return lang
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
	if !step.isNamed {
		t.Fatalf("isNamed: got false, want true for parenthesized named wildcard")
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
	q, err := NewQuery(`(identifier) @name (#eq? @missing "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tree := buildSimpleTree(lang)
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (missing predicate capture should not match)", len(matches))
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

func TestParsePredicateNotMatch(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#not-match? @name "^z")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	if q.patterns[0].predicates[0].kind != predicateNotMatch {
		t.Fatalf("predicate kind: got %d, want %d", q.patterns[0].predicates[0].kind, predicateNotMatch)
	}
}

func TestParsePredicateNotAnyOf(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#not-any-of? @name "foo" "bar")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	if q.patterns[0].predicates[0].kind != predicateNotAnyOf {
		t.Fatalf("predicate kind: got %d, want %d", q.patterns[0].predicates[0].kind, predicateNotAnyOf)
	}
}

func TestParsePredicateLuaMatch(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#lua-match? @name "^[%l]+$")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	if q.patterns[0].predicates[0].kind != predicateLuaMatch {
		t.Fatalf("predicate kind: got %d, want %d", q.patterns[0].predicates[0].kind, predicateLuaMatch)
	}
}

func TestParsePredicateAncestorPredicates(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#has-ancestor? @name function_declaration)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	if q.patterns[0].predicates[0].kind != predicateHasAncestor {
		t.Fatalf("predicate kind: got %d, want %d", q.patterns[0].predicates[0].kind, predicateHasAncestor)
	}
}

func TestParsePredicateUnsupportedErrors(t *testing.T) {
	lang := queryTestLanguage()
	if _, err := NewQuery(`(identifier) @name (#does-not-exist? @name)`, lang); err == nil {
		t.Fatal("expected error for unsupported predicate")
	}
}

func TestParseParenthesizedStringPattern(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`("(") @punctuation.bracket`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if step.textMatch != "(" {
		t.Fatalf("textMatch: got %q, want %q", step.textMatch, "(")
	}
}

func TestParseGroupWrapperWithDirectivePredicate(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`((identifier) @name (#set! "priority" 100))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	if q.patterns[0].predicates[0].kind != predicateSet {
		t.Fatalf("predicate kind: got %d, want %d", q.patterns[0].predicates[0].kind, predicateSet)
	}
}

func TestParseTopLevelFieldShorthand(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`name: (identifier) @name`, lang)
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
	if steps[1].field != FieldID(1) {
		t.Fatalf("field: got %d, want 1", steps[1].field)
	}
}

func TestParseFieldWildcardShorthand(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(call_expression function: _ @fn)`, lang)
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
	if steps[1].symbol != 0 {
		t.Fatalf("symbol: got %d, want wildcard 0", steps[1].symbol)
	}
	if steps[1].captureID < 0 {
		t.Fatal("captureID: expected capture on wildcard child")
	}
}

func TestParseAlternationBranchCaptures(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`[(identifier) @name (number) @name]`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if len(step.alternatives) != 2 {
		t.Fatalf("alternatives: got %d, want 2", len(step.alternatives))
	}
	if step.alternatives[0].captureID < 0 || step.alternatives[1].captureID < 0 {
		t.Fatal("expected capture IDs on alternation branches")
	}
}

func TestParseAlternationComplexBranchPreserved(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`[(function_declaration name: (identifier) @fname) (number) @num]`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if len(step.alternatives) != 2 {
		t.Fatalf("alternatives: got %d, want 2", len(step.alternatives))
	}
	if len(step.alternatives[0].steps) == 0 {
		t.Fatal("expected first alternation branch to preserve nested steps")
	}
	if len(step.alternatives[0].steps) != 2 {
		t.Fatalf("branch steps: got %d, want 2", len(step.alternatives[0].steps))
	}
	if step.alternatives[1].captureID < 0 {
		t.Fatal("expected simple branch capture to be preserved")
	}
}

func TestParseAlternationFieldShorthandPreserved(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(function_declaration [name: (_) @x body: (_) @x])`, lang)
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
	step := steps[1]
	if len(step.alternatives) != 2 {
		t.Fatalf("alternatives: got %d, want 2", len(step.alternatives))
	}
	if step.alternatives[0].field == 0 {
		t.Fatal("first alternation branch lost field constraint")
	}
	if step.alternatives[1].field == 0 {
		t.Fatal("second alternation branch lost field constraint")
	}
}

func TestParseErrorPseudoNodeAllowed(t *testing.T) {
	lang := queryTestLanguage()
	if _, err := NewQuery(`(ERROR) @error`, lang); err != nil {
		t.Fatalf("parse error: %v", err)
	}
}

func TestParseUnknownIdentifierErrors(t *testing.T) {
	lang := queryTestLanguage()
	if _, err := NewQuery(`(m) @keyword`, lang); err == nil {
		t.Fatal("expected parse error for unknown node type")
	}
}

func TestParseTopLevelAnchorErrors(t *testing.T) {
	lang := queryTestLanguage()
	if _, err := NewQuery(`. (identifier) @id`, lang); err == nil {
		t.Fatal("expected parse error for top-level anchor")
	}
}

func TestParseFieldFallbackParentPrefixedName(t *testing.T) {
	lang := &Language{
		Name: "test_field_fallback",
		SymbolNames: []string{
			"",
			"option",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "", Visible: false, Named: false},
			{Name: "option", Visible: true, Named: true},
		},
		FieldNames: []string{
			"",
			"option_key",
		},
		FieldCount: 1,
	}

	q, err := NewQuery(`(option (_ key: _ @k))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	if len(steps) != 3 {
		t.Fatalf("steps: got %d, want 3", len(steps))
	}
	if steps[2].field != FieldID(1) {
		t.Fatalf("field: got %d, want %d", steps[2].field, FieldID(1))
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

func TestParseAnchorBeforeFirstChild(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(program . (identifier) @first)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	steps := q.patterns[0].steps
	if len(steps) != 2 {
		t.Fatalf("steps: got %d, want 2", len(steps))
	}
	if !steps[1].anchorBefore {
		t.Fatal("expected anchorBefore on first child step")
	}
}

func TestParseAnchorAfterChild(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(program (number) @num .)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	steps := q.patterns[0].steps
	if len(steps) != 2 {
		t.Fatalf("steps: got %d, want 2", len(steps))
	}
	if !steps[1].anchorAfter {
		t.Fatal("expected anchorAfter on child step")
	}
}

func TestParseAnchorBetweenChildren(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(program (identifier) @a . (number) @b)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	steps := q.patterns[0].steps
	if len(steps) != 3 {
		t.Fatalf("steps: got %d, want 3", len(steps))
	}
	if !steps[2].anchorBefore {
		t.Fatal("expected anchorBefore on second sibling")
	}
	if steps[1].anchorAfter {
		t.Fatal("did not expect anchorAfter on first sibling for between-child anchor")
	}
}

func TestParseFieldNegationConstraint(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(function_declaration !parameters name: (identifier) @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	steps := q.patterns[0].steps
	if len(steps) != 2 {
		t.Fatalf("steps: got %d, want 2", len(steps))
	}
	if len(steps[0].absentFields) != 1 {
		t.Fatalf("absentFields: got %d, want 1", len(steps[0].absentFields))
	}
	if steps[0].absentFields[0] != FieldID(5) {
		t.Fatalf("absentFields[0]: got %d, want 5 (parameters)", steps[0].absentFields[0])
	}
}

func TestParseFieldNegationUnknownFieldErrors(t *testing.T) {
	lang := queryTestLanguage()
	if _, err := NewQuery(`(function_declaration !does_not_exist)`, lang); err == nil {
		t.Fatal("expected parse error for unknown negated field")
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

func TestParseMultipleCapturesOnSingleStep(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @symbol @spell`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	step := q.patterns[0].steps[0]
	if len(step.captureIDs) != 2 {
		t.Fatalf("captureIDs: got %d, want 2", len(step.captureIDs))
	}
	if q.captures[step.captureIDs[0]] != "symbol" {
		t.Fatalf("capture[0]: got %q, want %q", q.captures[step.captureIDs[0]], "symbol")
	}
	if q.captures[step.captureIDs[1]] != "spell" {
		t.Fatalf("capture[1]: got %q, want %q", q.captures[step.captureIDs[1]], "spell")
	}
	if step.captureID != step.captureIDs[0] {
		t.Fatalf("captureID compatibility field: got %d, want %d", step.captureID, step.captureIDs[0])
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

func TestQueryMetadataAccessors(t *testing.T) {
	lang := queryTestLanguage()
	src := `(identifier) @id
(#eq? @id "main")
`
	q, err := NewQuery(src, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if got, want := q.CaptureCount(), uint32(1); got != want {
		t.Fatalf("CaptureCount: got %d want %d", got, want)
	}
	if name, ok := q.CaptureNameForID(0); !ok || name != "id" {
		t.Fatalf("CaptureNameForID(0): got (%q,%v), want (%q,true)", name, ok, "id")
	}
	if _, ok := q.CaptureNameForID(99); ok {
		t.Fatal("CaptureNameForID(99): ok=true, want false")
	}

	if got, want := q.StringCount(), uint32(1); got != want {
		t.Fatalf("StringCount: got %d want %d", got, want)
	}
	if s, ok := q.StringValueForID(0); !ok || s != "main" {
		t.Fatalf("StringValueForID(0): got (%q,%v), want (%q,true)", s, ok, "main")
	}
	if _, ok := q.StringValueForID(99); ok {
		t.Fatal("StringValueForID(99): ok=true, want false")
	}

	start, ok := q.StartByteForPattern(0)
	if !ok {
		t.Fatal("StartByteForPattern(0): ok=false")
	}
	end, ok := q.EndByteForPattern(0)
	if !ok {
		t.Fatal("EndByteForPattern(0): ok=false")
	}
	if end <= start {
		t.Fatalf("pattern byte range invalid: start=%d end=%d", start, end)
	}
	preds, ok := q.PredicatesForPattern(0)
	if !ok {
		t.Fatal("PredicatesForPattern(0): ok=false")
	}
	if len(preds) != 1 {
		t.Fatalf("PredicatesForPattern len: got %d want 1", len(preds))
	}
}

func TestQueryPatternMetadata(t *testing.T) {
	lang := queryTestLanguage()

	q, err := NewQuery(`(identifier) @id`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !q.IsPatternRooted(0) {
		t.Fatal("IsPatternRooted(0) = false, want true")
	}
	if q.IsPatternNonLocal(0) {
		t.Fatal("IsPatternNonLocal(0) = true, want false")
	}
	if !q.StepIsDefinite(0, 0) {
		t.Fatal("StepIsDefinite(0,0) = false, want true")
	}
	if !q.IsPatternGuaranteedAtStep(0, 0) {
		t.Fatal("IsPatternGuaranteedAtStep(0,0) = false, want true")
	}

	wild, err := NewQuery(`(_) @any`, lang)
	if err != nil {
		t.Fatalf("wildcard parse error: %v", err)
	}
	if wild.StepIsDefinite(0, 0) {
		t.Fatal("wildcard StepIsDefinite(0,0) = true, want false")
	}
	if wild.IsPatternGuaranteedAtStep(0, 0) {
		t.Fatal("wildcard IsPatternGuaranteedAtStep(0,0) = true, want false")
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

func TestMatchAlternationFieldShorthandRespectsField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration [name: (_) @x body: (_) @x])`, lang)
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

	got := map[string]bool{}
	for _, c := range matches[0].Captures {
		got[c.Node.Text(tree.Source())] = true
	}

	if !got["main"] {
		t.Fatalf("missing name capture 'main'; captures=%v", matches[0].Captures)
	}
	if !got["42"] {
		t.Fatalf("missing body capture '42'; captures=%v", matches[0].Captures)
	}
}

func TestMatchRootAlternationFieldShorthandRespectsParentField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`[name: (_) body: (_)] @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 2 {
		t.Fatalf("matches: got %d, want 2", len(matches))
	}

	got := map[string]bool{}
	for _, m := range matches {
		if len(m.Captures) != 1 {
			t.Fatalf("captures per match: got %d, want 1", len(m.Captures))
		}
		got[m.Captures[0].Node.Text(tree.Source())] = true
	}

	if !got["main"] {
		t.Fatalf("missing field capture 'main'; matches=%v", matches)
	}
	if !got["42"] {
		t.Fatalf("missing field capture '42'; matches=%v", matches)
	}
}

func TestMatchMultipleCapturesOnSingleNode(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @symbol @spell`, lang)
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
	if matches[0].Captures[0].Name != "symbol" {
		t.Fatalf("capture[0] name: got %q, want %q", matches[0].Captures[0].Name, "symbol")
	}
	if matches[0].Captures[1].Name != "spell" {
		t.Fatalf("capture[1] name: got %q, want %q", matches[0].Captures[1].Name, "spell")
	}
	if matches[0].Captures[0].Node != matches[0].Captures[1].Node {
		t.Fatal("captures should point to the same node")
	}
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture node text: got %q, want %q", got, "main")
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

func TestMatchPredicateNotMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#not-match? @name "^zz")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateNotAnyOf(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#not-any-of? @name "root" "entry")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateLuaMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#lua-match? @name "^[%l]+$")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateHasAncestor(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#has-ancestor? @name function_declaration)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateHasAncestorViaSupertype(t *testing.T) {
	lang := queryTestLanguageWithSupertypes()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#has-ancestor? @name declaration)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateNotHasAncestor(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#not-has-ancestor? @name function_declaration)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchPredicateNotHasParent(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @name (#not-has-parent? @name parameter_list)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
}

func TestMatchPredicateIsAndIsNot(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q1, err := NewQuery(`(identifier) @variable.parameter (#is? @variable.parameter parameter)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := len(q1.Execute(tree)); got != 1 {
		t.Fatalf("matches (#is?): got %d, want 1", got)
	}

	q2, err := NewQuery(`(identifier) @variable.parameter (#is-not? local)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := len(q2.Execute(tree)); got != 0 {
		t.Fatalf("matches (#is-not?): got %d, want 0", got)
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

func TestMatchAlternationComplexBranch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`[(function_declaration name: (identifier) @fname) (number) @num]`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 2 {
		t.Fatalf("matches: got %d, want 2", len(matches))
	}

	captureMap := make(map[string]string)
	for _, m := range matches {
		for _, c := range m.Captures {
			captureMap[c.Name] = c.Node.Text(tree.Source())
		}
	}
	if captureMap["fname"] != "main" {
		t.Fatalf("fname: got %q, want %q", captureMap["fname"], "main")
	}
	if captureMap["num"] != "42" {
		t.Fatalf("num: got %q, want %q", captureMap["num"], "42")
	}
}

func TestMatchAnchorBeforeFirstNamedChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration . (identifier) @name)`, lang)
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
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture text: got %q, want %q", got, "main")
	}
}

func TestMatchAnchorAfterLastNamedChild(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	q, err := NewQuery(`(program (number) @last .)`, lang)
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
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "1" {
		t.Fatalf("capture text: got %q, want %q", got, "1")
	}
}

func TestMatchAnchorBetweenSiblingsBacktracks(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	// First identifier ("a") is not adjacent to number; the matcher should
	// backtrack to "b" so . constraint can match "1".
	q, err := NewQuery(`(program (identifier) @left . (number) @right)`, lang)
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
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "b" {
		t.Fatalf("left capture: got %q, want %q", got, "b")
	}
	if got := matches[0].Captures[1].Node.Text(tree.Source()); got != "1" {
		t.Fatalf("right capture: got %q, want %q", got, "1")
	}
}

func TestMatchAnchorBetweenSiblingsNoMatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildProgramTreeWithIdentifiers(lang)

	q, err := NewQuery(`(program (number) @num . (identifier) @id)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchFieldNegationRejectsPresentField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration !parameters name: (identifier) @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0", len(matches))
	}
}

func TestMatchFieldNegationAllowsAbsentField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration !function name: (identifier) @name)`, lang)
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
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "main" {
		t.Fatalf("capture text: got %q, want %q", got, "main")
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

func TestMatchFieldScansAllSiblingsWithSameFieldName(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildRepeatedFieldTree(lang)

	q, err := NewQuery(`(program name: (identifier) @name)`, lang)
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
	if got := matches[0].Captures[0].Node.Text(tree.Source()); got != "a" {
		t.Fatalf("capture text: got %q, want %q", got, "a")
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

func TestMatchAnchorAfterAnonymousSibling(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration "func" . (identifier) @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 || len(matches[0].Captures) != 1 {
		t.Fatalf("captures: got %d matches / %d captures, want 1/1", len(matches), func() int {
			if len(matches) == 0 {
				return 0
			}
			return len(matches[0].Captures)
		}())
	}
	if got, want := matches[0].Captures[0].Node.Text(tree.Source()), "main"; got != want {
		t.Fatalf("anchor-after-anonymous capture = %q, want %q", got, want)
	}
}

func TestMatchNamedWildcardSkipsAnonymousNodes(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration (_) @child)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	matches := q.Execute(tree)
	if len(matches) != 1 || len(matches[0].Captures) == 0 {
		t.Fatalf("captures: got %d matches / %d captures, want 1/non-zero", len(matches), func() int {
			if len(matches) == 0 {
				return 0
			}
			return len(matches[0].Captures)
		}())
	}
	foundMain := false
	for _, c := range matches[0].Captures {
		if got := c.Node.Text(tree.Source()); got == "func" {
			t.Fatalf("named wildcard matched anonymous token %q", got)
		}
		if c.Node.Text(tree.Source()) == "main" {
			foundMain = true
		}
	}
	if !foundMain {
		t.Fatalf("named wildcard captures missing %q", "main")
	}
}

func TestParseBareWildcardChildRemainsUnnamed(t *testing.T) {
	lang := queryTestLanguage()

	q, err := NewQuery(`(function_declaration _ @child)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if got := len(q.patterns); got != 1 {
		t.Fatalf("patterns: got %d, want 1", got)
	}
	steps := q.patterns[0].steps
	if got := len(steps); got != 2 {
		t.Fatalf("steps: got %d, want 2", got)
	}
	child := steps[1]
	if child.symbol != 0 {
		t.Fatalf("child symbol: got %d, want 0", child.symbol)
	}
	if child.isNamed {
		t.Fatalf("child isNamed: got true, want false for bare wildcard")
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

func TestQueryCursorSetByteRange(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`[(identifier) (number)] @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	cursor.SetByteRange(0, 10) // includes "main", excludes "42"

	var got []string
	for {
		cap, ok := cursor.NextCapture()
		if !ok {
			break
		}
		got = append(got, cap.Node.Text(tree.Source()))
	}
	if len(got) != 1 || got[0] != "main" {
		t.Fatalf("captures in byte range: got %v, want [main]", got)
	}
}

func TestQueryCursorSetPointRange(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	q, err := NewQuery(`[(identifier) (number)] @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	cursor.SetPointRange(Point{Row: 0, Column: 5}, Point{Row: 0, Column: 10}) // includes "main", excludes "42"

	var got []string
	for {
		cap, ok := cursor.NextCapture()
		if !ok {
			break
		}
		got = append(got, cap.Node.Text(tree.Source()))
	}
	if len(got) != 1 || got[0] != "main" {
		t.Fatalf("captures in point range: got %v, want [main]", got)
	}
}

func TestQueryCursorSetMatchLimit(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`[(identifier) (number)] @x`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	cursor.SetMatchLimit(1)

	if _, ok := cursor.NextMatch(); !ok {
		t.Fatal("expected first match")
	}
	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("expected second call to stop at match limit")
	}
	if !cursor.DidExceedMatchLimit() {
		t.Fatal("expected DidExceedMatchLimit() to be true")
	}
}

func TestQueryCursorSetMaxStartDepth(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(identifier) @id`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// identifier is below depth 1 in buildSimpleTree, so depth 1 should yield none.
	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	cursor.SetMaxStartDepth(1)
	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("expected no matches at max start depth 1")
	}

	cursor = q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	cursor.SetMaxStartDepth(2)
	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected match at max start depth 2")
	}
	if got, want := match.Captures[0].Node.Text(tree.Source()), "main"; got != want {
		t.Fatalf("capture text: got %q want %q", got, want)
	}
}

func TestQueryDisableCapture(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery(`(function_declaration name: (identifier) @id body: (block (number) @num))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.DisableCapture("num")

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d want 1", len(matches))
	}
	if len(matches[0].Captures) != 1 {
		t.Fatalf("captures: got %d want 1", len(matches[0].Captures))
	}
	if got, want := matches[0].Captures[0].Name, "id"; got != want {
		t.Fatalf("capture name: got %q want %q", got, want)
	}
}

func TestQueryDisablePattern(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	q, err := NewQuery("(identifier) @x\n(number) @x", lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.DisablePattern(0)

	matches := q.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("matches: got %d want 1", len(matches))
	}
	if len(matches[0].Captures) != 1 {
		t.Fatalf("captures: got %d want 1", len(matches[0].Captures))
	}
	if got, want := matches[0].Captures[0].Node.Text(tree.Source()), "42"; got != want {
		t.Fatalf("capture text: got %q want %q", got, want)
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

// buildFieldedTree creates a tree with field annotations:
// program > function_declaration(name: identifier)
func buildFieldedTree(lang *Language) *Tree {
	source := []byte("func main 42")
	ident := leaf(Symbol(1), true, 5, 9)
	funcDecl := parent(Symbol(3), true,
		[]*Node{ident},
		[]FieldID{1}) // fieldID 1 = "name"
	program := parent(Symbol(7), true,
		[]*Node{funcDecl},
		[]FieldID{0})
	return NewTree(program, source, lang)
}

// buildRepeatedFieldTree creates a parent with two children using the same
// field ID, where only the second child is an identifier.
func buildRepeatedFieldTree(lang *Language) *Tree {
	source := []byte("1 a")
	num := leaf(Symbol(2), true, 0, 1)
	id := leaf(Symbol(1), true, 2, 3)
	program := parent(Symbol(7), true,
		[]*Node{num, id},
		[]FieldID{1, 1}) // both children are fieldID 1 = "name"
	return NewTree(program, source, lang)
}

// ---------------------------------------------------------------------------
// Quantified query predicates: #any-eq?, #any-not-eq?, #any-match?, #any-not-match?
// ---------------------------------------------------------------------------

// --- Parse tests ---

func TestParsePredicateAnyEq(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#any-eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateAnyEq {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateAnyEq)
	}
	if pred.leftCapture != "name" {
		t.Fatalf("leftCapture: got %q, want %q", pred.leftCapture, "name")
	}
	if pred.literal != "main" {
		t.Fatalf("literal: got %q, want %q", pred.literal, "main")
	}
}

func TestParsePredicateAnyEqCapture(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(block (identifier) @a (identifier) @b (#any-eq? @a @b))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateAnyEq {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateAnyEq)
	}
	if pred.rightCapture != "b" {
		t.Fatalf("rightCapture: got %q, want %q", pred.rightCapture, "b")
	}
}

func TestParsePredicateAnyNotEq(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#any-not-eq? @name "main")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateAnyNotEq {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateAnyNotEq)
	}
}

func TestParsePredicateAnyMatch(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#any-match? @name "^ma")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateAnyMatch {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateAnyMatch)
	}
	if pred.regex == nil {
		t.Fatal("regex should not be nil")
	}
}

func TestParsePredicateAnyNotMatch(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#any-not-match? @name "^z")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	pred := q.patterns[0].predicates[0]
	if pred.kind != predicateAnyNotMatch {
		t.Fatalf("predicate kind: got %d, want %d", pred.kind, predicateAnyNotMatch)
	}
	if pred.regex == nil {
		t.Fatal("regex should not be nil")
	}
}

func TestParsePredicateAnyMatchRejectsCapture(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(block (identifier) @a (identifier) @b (#any-match? @a @b))`, lang)
	if err == nil {
		t.Fatal("expected error: #any-match? second argument must be a string literal")
	}
}

func TestParsePredicateAnyNotMatchRejectsCapture(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewQuery(`(block (identifier) @a (identifier) @b (#any-not-match? @a @b))`, lang)
	if err == nil {
		t.Fatal("expected error: #any-not-match? second argument must be a string literal")
	}
}

// --- Evaluation tests using matchesPredicates directly ---

func TestAnyEqMatchesPredicates(t *testing.T) {
	source := []byte("foo bar baz")
	n1 := leaf(Symbol(1), true, 0, 3)  // "foo"
	n2 := leaf(Symbol(1), true, 4, 7)  // "bar"
	n3 := leaf(Symbol(1), true, 8, 11) // "baz"

	captures := []QueryCapture{
		{Name: "items", Node: n1},
		{Name: "items", Node: n2},
		{Name: "items", Node: n3},
	}

	q := &Query{}

	// Should match: "bar" is among the captured nodes.
	preds := []QueryPredicate{{
		kind:        predicateAnyEq,
		leftCapture: "items",
		literal:     "bar",
	}}
	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-eq? should match when one node equals 'bar'")
	}

	// Should not match: "xyz" is not among the captured nodes.
	preds[0].literal = "xyz"
	if q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-eq? should not match when no node equals 'xyz'")
	}
}

func TestAnyEqCaptureVsCapture(t *testing.T) {
	source := []byte("foo bar baz bar")
	n1 := leaf(Symbol(1), true, 0, 3)     // "foo"
	n2 := leaf(Symbol(1), true, 4, 7)     // "bar"
	n3 := leaf(Symbol(1), true, 8, 11)    // "baz"
	nRef := leaf(Symbol(1), true, 12, 15) // "bar"

	captures := []QueryCapture{
		{Name: "items", Node: n1},
		{Name: "items", Node: n2},
		{Name: "items", Node: n3},
		{Name: "ref", Node: nRef},
	}

	q := &Query{}

	// Should match: n2 ("bar") == nRef ("bar").
	preds := []QueryPredicate{{
		kind:         predicateAnyEq,
		leftCapture:  "items",
		rightCapture: "ref",
	}}
	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-eq? capture-vs-capture should match when one node matches")
	}

	// Change ref to "foo" — still should match (n1 == "foo").
	nRef2 := leaf(Symbol(1), true, 0, 3) // "foo"
	captures[3] = QueryCapture{Name: "ref", Node: nRef2}
	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-eq? capture-vs-capture should match when first node matches")
	}
}

func TestAnyNotEqMatchesPredicates(t *testing.T) {
	source := []byte("bar bar bar")
	n1 := leaf(Symbol(1), true, 0, 3)  // "bar"
	n2 := leaf(Symbol(1), true, 4, 7)  // "bar"
	n3 := leaf(Symbol(1), true, 8, 11) // "bar"

	captures := []QueryCapture{
		{Name: "items", Node: n1},
		{Name: "items", Node: n2},
		{Name: "items", Node: n3},
	}

	q := &Query{}

	// All nodes are "bar", so any-not-eq? "bar" should NOT match (no node != "bar").
	preds := []QueryPredicate{{
		kind:        predicateAnyNotEq,
		leftCapture: "items",
		literal:     "bar",
	}}
	if q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-not-eq? should not match when all nodes equal 'bar'")
	}

	// any-not-eq? "xyz" should match (all nodes != "xyz").
	preds[0].literal = "xyz"
	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-not-eq? should match when some node does not equal 'xyz'")
	}
}

func TestAnyMatchMatchesPredicates(t *testing.T) {
	source := []byte("foo bar baz")
	n1 := leaf(Symbol(1), true, 0, 3)  // "foo"
	n2 := leaf(Symbol(1), true, 4, 7)  // "bar"
	n3 := leaf(Symbol(1), true, 8, 11) // "baz"

	captures := []QueryCapture{
		{Name: "items", Node: n1},
		{Name: "items", Node: n2},
		{Name: "items", Node: n3},
	}

	q := &Query{}
	rx := mustCompileTestRegex(t, "^ba")

	// Should match: "bar" and "baz" both match ^ba.
	preds := []QueryPredicate{{
		kind:        predicateAnyMatch,
		leftCapture: "items",
		regex:       rx,
	}}
	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-match? should match when at least one node matches ^ba")
	}

	// Should not match: nothing matches ^xyz.
	rx2 := mustCompileTestRegex(t, "^xyz")
	preds[0].regex = rx2
	if q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-match? should not match when no node matches ^xyz")
	}
}

func TestAnyNotMatchMatchesPredicates(t *testing.T) {
	source := []byte("bar baz")
	n1 := leaf(Symbol(1), true, 0, 3) // "bar"
	n2 := leaf(Symbol(1), true, 4, 7) // "baz"

	captures := []QueryCapture{
		{Name: "items", Node: n1},
		{Name: "items", Node: n2},
	}

	q := &Query{}

	// ^ba matches both "bar" and "baz", so any-not-match? ^ba should NOT match.
	rx := mustCompileTestRegex(t, "^ba")
	preds := []QueryPredicate{{
		kind:        predicateAnyNotMatch,
		leftCapture: "items",
		regex:       rx,
	}}
	if q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-not-match? should not match when all nodes match ^ba")
	}

	// ^bar matches "bar" but not "baz", so any-not-match? ^bar should match.
	rx2 := mustCompileTestRegex(t, "^bar$")
	preds[0].regex = rx2
	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-not-match? should match when at least one node does not match ^bar$")
	}
}

func TestAnyEqEmptyCaptures(t *testing.T) {
	source := []byte("foo")
	q := &Query{}

	preds := []QueryPredicate{{
		kind:        predicateAnyEq,
		leftCapture: "missing",
		literal:     "foo",
	}}
	if q.matchesPredicates(preds, nil, nil, source) {
		t.Fatal("any-eq? should return false for empty captures")
	}
}

func TestAnyMatchNilRegex(t *testing.T) {
	source := []byte("foo")
	n := leaf(Symbol(1), true, 0, 3)
	captures := []QueryCapture{{Name: "x", Node: n}}
	q := &Query{}

	preds := []QueryPredicate{{
		kind:        predicateAnyMatch,
		leftCapture: "x",
		regex:       nil,
	}}
	if q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("any-match? with nil regex should return false")
	}
}

func mustCompileTestRegex(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	rx, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("bad test regex %q: %v", pattern, err)
	}
	return rx
}

// --------------------------------------------------------------------------
// #select-adjacent! directive tests
// --------------------------------------------------------------------------

func TestSelectAdjacentParse(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @items (#select-adjacent! @items @anchor)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(q.patterns))
	}
	preds := q.patterns[0].predicates
	if len(preds) != 1 {
		t.Fatalf("expected 1 predicate, got %d", len(preds))
	}
	if preds[0].kind != predicateSelectAdjacent {
		t.Errorf("kind: got %d, want predicateSelectAdjacent", preds[0].kind)
	}
	if preds[0].leftCapture != "items" {
		t.Errorf("leftCapture: got %q, want %q", preds[0].leftCapture, "items")
	}
	if preds[0].rightCapture != "anchor" {
		t.Errorf("rightCapture: got %q, want %q", preds[0].rightCapture, "anchor")
	}
}

func TestSelectAdjacentParseErrors(t *testing.T) {
	lang := queryTestLanguage()
	// First argument must be a capture
	_, err := NewQuery(`(identifier) @x (#select-adjacent! "foo" @y)`, lang)
	if err == nil {
		t.Fatal("expected error for non-capture first argument")
	}
	// Second argument must be a capture
	_, err = NewQuery(`(identifier) @x (#select-adjacent! @x "bar")`, lang)
	if err == nil {
		t.Fatal("expected error for non-capture second argument")
	}
}

func TestSelectAdjacentFiltering(t *testing.T) {
	// Source: "abcdef"
	//   node A: bytes [0,3)  "abc"
	//   node B: bytes [3,6)  "def"  -- adjacent to A (A.end == B.start)
	//   node C: bytes [7,10) "ghi"  -- NOT adjacent to A
	source := []byte("abcdef ghi")
	nodeA := leaf(Symbol(1), true, 0, 3)  // "abc"
	nodeB := leaf(Symbol(1), true, 3, 6)  // "def"
	nodeC := leaf(Symbol(1), true, 7, 10) // "ghi"

	captures := []QueryCapture{
		{Name: "items", Node: nodeA},
		{Name: "items", Node: nodeB},
		{Name: "items", Node: nodeC},
		{Name: "anchor", Node: nodeA},
	}

	pred := QueryPredicate{
		kind:         predicateSelectAdjacent,
		leftCapture:  "items",
		rightCapture: "anchor",
	}

	result := applySelectAdjacent(pred, captures)

	// Should keep nodeB (adjacent: A.end==3 == B.start==3)
	// Should NOT keep nodeA (A.end==3 != A.start==0 from anchor, A.start==0 != A.end==3 from anchor ... wait)
	// Actually nodeA IS the anchor, so: nodeA.end==3 == anchor.start==0? No. nodeA.start==0 == anchor.end==3? No.
	// So nodeA should NOT be adjacent (it is the anchor itself, but the adjacency check only looks at
	// whether end==start or start==end between items and anchors).
	// nodeB: B.start==3 == anchor.end==3 → YES
	// nodeC: C.start==7 != 3, C.end==10 != 0 → NO

	var itemNames []string
	for _, c := range result {
		if c.Name == "items" {
			itemNames = append(itemNames, c.Node.Text(source))
		}
	}

	if len(itemNames) != 1 || itemNames[0] != "def" {
		t.Fatalf("expected items=[def], got %v", itemNames)
	}

	// Anchor should still be present in the result.
	hasAnchor := false
	for _, c := range result {
		if c.Name == "anchor" {
			hasAnchor = true
		}
	}
	if !hasAnchor {
		t.Fatal("anchor capture should still be in the result")
	}
}

func TestSelectAdjacentBothDirections(t *testing.T) {
	// Test adjacency in both directions:
	// anchor at [5,8), item at [3,5) → item.end==5 == anchor.start==5 → adjacent
	// anchor at [5,8), item at [8,11) → item.start==8 == anchor.end==8 → adjacent
	itemBefore := leaf(Symbol(1), true, 3, 5) // "ab"
	anchor := leaf(Symbol(1), true, 5, 8)     // "anc" (conceptually)
	itemAfter := leaf(Symbol(1), true, 8, 11) // "hor" (conceptually)
	itemFar := leaf(Symbol(1), true, 12, 14)  // far away

	captures := []QueryCapture{
		{Name: "items", Node: itemBefore},
		{Name: "items", Node: itemAfter},
		{Name: "items", Node: itemFar},
		{Name: "anchor", Node: anchor},
	}

	pred := QueryPredicate{
		kind:         predicateSelectAdjacent,
		leftCapture:  "items",
		rightCapture: "anchor",
	}

	result := applySelectAdjacent(pred, captures)

	var kept []*Node
	for _, c := range result {
		if c.Name == "items" {
			kept = append(kept, c.Node)
		}
	}

	if len(kept) != 2 {
		t.Fatalf("expected 2 adjacent items, got %d", len(kept))
	}
	if kept[0] != itemBefore {
		t.Errorf("first kept item should be itemBefore")
	}
	if kept[1] != itemAfter {
		t.Errorf("second kept item should be itemAfter")
	}
}

func TestSelectAdjacentNoAnchors(t *testing.T) {
	// When there are no anchor captures, all items should be removed.
	source := []byte("abc")
	n := leaf(Symbol(1), true, 0, 3)
	_ = source

	captures := []QueryCapture{
		{Name: "items", Node: n},
	}

	pred := QueryPredicate{
		kind:         predicateSelectAdjacent,
		leftCapture:  "items",
		rightCapture: "anchor",
	}

	result := applySelectAdjacent(pred, captures)
	for _, c := range result {
		if c.Name == "items" {
			t.Fatal("no items should remain when there are no anchors")
		}
	}
}

func TestSelectAdjacentMultipleAnchors(t *testing.T) {
	// Multiple anchors: item is adjacent if adjacent to ANY anchor.
	// anchor1 at [0,3), anchor2 at [6,9)
	// item at [3,6) → adjacent to anchor1 (item.start==3==anchor1.end) AND anchor2 (item.end==6==anchor2.start)
	// item at [10,13) → not adjacent to either
	n1 := leaf(Symbol(1), true, 3, 6)
	n2 := leaf(Symbol(1), true, 10, 13)
	anchor1 := leaf(Symbol(1), true, 0, 3)
	anchor2 := leaf(Symbol(1), true, 6, 9)

	captures := []QueryCapture{
		{Name: "items", Node: n1},
		{Name: "items", Node: n2},
		{Name: "anchor", Node: anchor1},
		{Name: "anchor", Node: anchor2},
	}

	pred := QueryPredicate{
		kind:         predicateSelectAdjacent,
		leftCapture:  "items",
		rightCapture: "anchor",
	}

	result := applySelectAdjacent(pred, captures)

	var kept []*Node
	for _, c := range result {
		if c.Name == "items" {
			kept = append(kept, c.Node)
		}
	}
	if len(kept) != 1 {
		t.Fatalf("expected 1 adjacent item, got %d", len(kept))
	}
	if kept[0] != n1 {
		t.Error("expected n1 to be kept (adjacent to both anchors)")
	}
}

// --------------------------------------------------------------------------
// #strip! directive tests
// --------------------------------------------------------------------------

func TestStripParse(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(identifier) @name (#strip! @name "^_+")`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(q.patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(q.patterns))
	}
	preds := q.patterns[0].predicates
	if len(preds) != 1 {
		t.Fatalf("expected 1 predicate, got %d", len(preds))
	}
	if preds[0].kind != predicateStrip {
		t.Errorf("kind: got %d, want predicateStrip", preds[0].kind)
	}
	if preds[0].leftCapture != "name" {
		t.Errorf("leftCapture: got %q, want %q", preds[0].leftCapture, "name")
	}
	if preds[0].literal != "^_+" {
		t.Errorf("literal: got %q, want %q", preds[0].literal, "^_+")
	}
	if preds[0].regex == nil {
		t.Fatal("regex should be compiled")
	}
}

func TestStripParseErrors(t *testing.T) {
	lang := queryTestLanguage()
	// First argument must be a capture
	_, err := NewQuery(`(identifier) @x (#strip! "foo" "bar")`, lang)
	if err == nil {
		t.Fatal("expected error for non-capture first argument")
	}
	// Second argument must be a string, not a capture
	_, err = NewQuery(`(identifier) @x (#strip! @x @y)`, lang)
	if err == nil {
		t.Fatal("expected error for capture second argument")
	}
	// Invalid regex
	_, err = NewQuery(`(identifier) @x (#strip! @x "[invalid")`, lang)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestStripApply(t *testing.T) {
	source := []byte("__hello__world")
	n := leaf(Symbol(1), true, 0, 14)

	captures := []QueryCapture{
		{Name: "name", Node: n},
	}

	rx := mustCompileTestRegex(t, "_+")
	pred := QueryPredicate{
		kind:        predicateStrip,
		leftCapture: "name",
		regex:       rx,
	}

	result := applyStrip(pred, captures, source)

	if len(result) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(result))
	}
	if result[0].TextOverride != "helloworld" {
		t.Errorf("TextOverride: got %q, want %q", result[0].TextOverride, "helloworld")
	}
}

func TestStripNoChange(t *testing.T) {
	// When the regex doesn't match, TextOverride should remain empty.
	source := []byte("hello")
	n := leaf(Symbol(1), true, 0, 5)

	captures := []QueryCapture{
		{Name: "name", Node: n},
	}

	rx := mustCompileTestRegex(t, "_+")
	pred := QueryPredicate{
		kind:        predicateStrip,
		leftCapture: "name",
		regex:       rx,
	}

	result := applyStrip(pred, captures, source)

	if result[0].TextOverride != "" {
		t.Errorf("TextOverride should be empty when regex doesn't match, got %q", result[0].TextOverride)
	}
}

func TestStripOnlyAffectsNamedCapture(t *testing.T) {
	source := []byte("__foo__bar")
	n1 := leaf(Symbol(1), true, 0, 5)  // "__foo"
	n2 := leaf(Symbol(1), true, 5, 10) // "__bar"

	captures := []QueryCapture{
		{Name: "target", Node: n1},
		{Name: "other", Node: n2},
	}

	rx := mustCompileTestRegex(t, "^_+")
	pred := QueryPredicate{
		kind:        predicateStrip,
		leftCapture: "target",
		regex:       rx,
	}

	result := applyStrip(pred, captures, source)

	if result[0].TextOverride != "foo" {
		t.Errorf("target TextOverride: got %q, want %q", result[0].TextOverride, "foo")
	}
	if result[1].TextOverride != "" {
		t.Errorf("other TextOverride should be empty, got %q", result[1].TextOverride)
	}
}

func TestStripNilRegex(t *testing.T) {
	source := []byte("hello")
	n := leaf(Symbol(1), true, 0, 5)

	captures := []QueryCapture{
		{Name: "name", Node: n},
	}

	pred := QueryPredicate{
		kind:        predicateStrip,
		leftCapture: "name",
		regex:       nil,
	}

	result := applyStrip(pred, captures, source)
	if result[0].TextOverride != "" {
		t.Errorf("nil regex should not set TextOverride, got %q", result[0].TextOverride)
	}
}

func TestQueryCaptureText(t *testing.T) {
	source := []byte("hello world")
	n := leaf(Symbol(1), true, 0, 5) // "hello"

	// Without TextOverride, Text() returns node text.
	c := QueryCapture{Name: "x", Node: n}
	if got := c.Text(source); got != "hello" {
		t.Errorf("Text() without override: got %q, want %q", got, "hello")
	}

	// With TextOverride, Text() returns the override.
	c.TextOverride = "stripped"
	if got := c.Text(source); got != "stripped" {
		t.Errorf("Text() with override: got %q, want %q", got, "stripped")
	}

	// Nil node with no override returns empty.
	c2 := QueryCapture{Name: "y", Node: nil}
	if got := c2.Text(source); got != "" {
		t.Errorf("Text() nil node: got %q, want %q", got, "")
	}
}

func TestStripDoesNotFilterMatch(t *testing.T) {
	// #strip! is a directive — it should not cause the match to be rejected.
	// Verify that matchesPredicates returns true when #strip! is present.
	q := &Query{}
	source := []byte("__hello")
	n := leaf(Symbol(1), true, 0, 7)

	captures := []QueryCapture{
		{Name: "name", Node: n},
	}

	rx := mustCompileTestRegex(t, "^_+")
	preds := []QueryPredicate{{
		kind:        predicateStrip,
		leftCapture: "name",
		regex:       rx,
	}}

	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("#strip! should not cause match rejection")
	}
}

func TestSelectAdjacentDoesNotFilterMatch(t *testing.T) {
	// #select-adjacent! is a directive — it should not cause the match to be rejected.
	q := &Query{}
	source := []byte("abc")
	n := leaf(Symbol(1), true, 0, 3)

	captures := []QueryCapture{
		{Name: "items", Node: n},
	}

	preds := []QueryPredicate{{
		kind:         predicateSelectAdjacent,
		leftCapture:  "items",
		rightCapture: "anchor",
	}}

	if !q.matchesPredicates(preds, captures, nil, source) {
		t.Fatal("#select-adjacent! should not cause match rejection")
	}
}

// --------------------------------------------------------------------------
// Multi-sibling grouping pattern tests
// --------------------------------------------------------------------------

func TestGroupingSingleElementUnchanged(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`((identifier) @name)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	// Single-element group should NOT insert wildcard root.
	if len(steps) != 1 {
		t.Fatalf("steps: got %d, want 1 (no wildcard root for single element)", len(steps))
	}
	if steps[0].symbol != Symbol(1) {
		t.Fatalf("symbol: got %d, want 1 (identifier)", steps[0].symbol)
	}
	if steps[0].depth != 0 {
		t.Fatalf("depth: got %d, want 0", steps[0].depth)
	}
}

func TestGroupingMultiSiblingInsertsWildcard(t *testing.T) {
	lang := queryTestLanguage()
	// Two siblings in a group — should insert wildcard root.
	q, err := NewQuery(`((identifier) (number))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	if len(steps) != 3 {
		t.Fatalf("steps: got %d, want 3 (wildcard + identifier + number)", len(steps))
	}
	// Step 0: wildcard root at depth 0.
	if steps[0].symbol != 0 || steps[0].isNamed {
		t.Fatalf("step 0: got symbol=%d isNamed=%v, want wildcard (0, false)", steps[0].symbol, steps[0].isNamed)
	}
	if steps[0].depth != 0 {
		t.Fatalf("step 0 depth: got %d, want 0", steps[0].depth)
	}
	// Step 1: identifier at depth 1.
	if steps[1].symbol != Symbol(1) {
		t.Fatalf("step 1 symbol: got %d, want 1 (identifier)", steps[1].symbol)
	}
	if steps[1].depth != 1 {
		t.Fatalf("step 1 depth: got %d, want 1", steps[1].depth)
	}
	// Step 2: number at depth 1.
	if steps[2].symbol != Symbol(2) {
		t.Fatalf("step 2 symbol: got %d, want 2 (number)", steps[2].symbol)
	}
	if steps[2].depth != 1 {
		t.Fatalf("step 2 depth: got %d, want 1", steps[2].depth)
	}
}

func TestGroupingMultiSiblingWithCaptures(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`((identifier) @id (number) @num)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	steps := q.patterns[0].steps
	if len(steps) != 3 {
		t.Fatalf("steps: got %d, want 3", len(steps))
	}
	// Verify wildcard root.
	if steps[0].symbol != 0 {
		t.Fatalf("step 0 symbol: got %d, want 0 (wildcard)", steps[0].symbol)
	}
	// Verify captures.
	if steps[1].captureID < 0 {
		t.Fatal("step 1: expected capture on identifier")
	}
	if steps[2].captureID < 0 {
		t.Fatal("step 2: expected capture on number")
	}
}

func TestGroupingTripleParens(t *testing.T) {
	lang := queryTestLanguage()
	// Triple parens like git_rebase uses: (((a) @cap (b) @cap2) (#set! ...))
	q, err := NewQuery(`(((identifier) @id (number) @num) (#set! "priority" 100))`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.PatternCount() != 1 {
		t.Fatalf("PatternCount: got %d, want 1", q.PatternCount())
	}
	steps := q.patterns[0].steps
	if len(steps) != 3 {
		t.Fatalf("steps: got %d, want 3", len(steps))
	}
	// Wildcard root at depth 0.
	if steps[0].symbol != 0 {
		t.Fatalf("step 0 symbol: got %d, want 0 (wildcard)", steps[0].symbol)
	}
	if steps[0].depth != 0 {
		t.Fatalf("step 0 depth: got %d, want 0", steps[0].depth)
	}
	// Captures present.
	if steps[1].captureID < 0 || steps[2].captureID < 0 {
		t.Fatal("expected captures on both children")
	}
	// Predicate present.
	if len(q.patterns[0].predicates) != 1 {
		t.Fatalf("predicates: got %d, want 1", len(q.patterns[0].predicates))
	}
}
