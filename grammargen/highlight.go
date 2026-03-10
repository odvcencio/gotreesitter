package grammargen

import (
	"fmt"
	"strings"
)

// GenerateHighlightQuery infers a tree-sitter highlight query from grammar structure.
// It maps well-known rule names and patterns to standard capture names:
//
//   - comment → @comment
//   - string, string_content → @string
//   - number, integer, float → @number
//   - true, false → @boolean
//   - null, nil, none → @constant.builtin
//   - identifier → @variable
//   - type_identifier → @type
//   - function keywords → @keyword.function
//   - control flow keywords → @keyword.control
//   - other keyword-like string terminals → @keyword
//   - operators → @operator
func GenerateHighlightQuery(g *Grammar) string {
	var b strings.Builder

	// Collect named rules and string terminals.
	namedRules := make(map[string]bool)
	for _, name := range g.RuleOrder {
		namedRules[name] = true
	}

	// Collect all string terminals from the grammar.
	stringTerminals := collectStringTerminals(g)

	// Emit captures for well-known named rules.
	ruleCaptures := []struct {
		patterns []string
		capture  string
	}{
		{[]string{"comment", "line_comment", "block_comment"}, "comment"},
		{[]string{"string", "string_literal", "raw_string", "template_string"}, "string"},
		{[]string{"string_content"}, "string.content"},
		{[]string{"escape_sequence"}, "escape"},
		{[]string{"number", "integer", "float", "integer_literal", "float_literal"}, "number"},
		{[]string{"boolean", "true", "false"}, "boolean"},
		{[]string{"null", "nil", "none", "null_literal"}, "constant.builtin"},
		{[]string{"identifier"}, "variable"},
		{[]string{"type_identifier", "type_name"}, "type"},
		{[]string{"field_identifier", "property_identifier"}, "property"},
		{[]string{"function_name", "method_name"}, "function"},
		{[]string{"operator"}, "operator"},
	}

	for _, rc := range ruleCaptures {
		for _, pat := range rc.patterns {
			if namedRules[pat] {
				fmt.Fprintf(&b, "(%s) @%s\n", pat, rc.capture)
			}
		}
	}

	// Classify string terminals into keywords and operators.
	keywords, operators := classifyTerminals(stringTerminals)

	if len(keywords) > 0 {
		b.WriteString("\n; Keywords\n")
		for _, kw := range keywords {
			fmt.Fprintf(&b, "%q @keyword\n", kw)
		}
	}

	if len(operators) > 0 {
		b.WriteString("\n; Operators\n")
		for _, op := range operators {
			fmt.Fprintf(&b, "%q @operator\n", op)
		}
	}

	return b.String()
}

// collectStringTerminals extracts all unique string terminals from the grammar.
func collectStringTerminals(g *Grammar) []string {
	seen := make(map[string]bool)
	var result []string

	var walk func(r *Rule)
	walk = func(r *Rule) {
		if r == nil {
			return
		}
		if r.Kind == RuleString && r.Value != "" && !seen[r.Value] {
			seen[r.Value] = true
			result = append(result, r.Value)
		}
		for _, c := range r.Children {
			walk(c)
		}
	}

	for _, name := range g.RuleOrder {
		walk(g.Rules[name])
	}
	for _, e := range g.Extras {
		walk(e)
	}

	return result
}

// classifyTerminals separates string terminals into keywords and operators.
func classifyTerminals(terminals []string) (keywords, operators []string) {
	for _, t := range terminals {
		if isKeywordLike(t) {
			keywords = append(keywords, t)
		} else if isOperatorLike(t) {
			operators = append(operators, t)
		}
	}
	return
}

// isKeywordLike returns true if the string looks like a language keyword.
func isKeywordLike(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
			return false
		}
	}
	return true
}

// isOperatorLike returns true if the string looks like an operator.
func isOperatorLike(s string) bool {
	if len(s) == 0 || len(s) > 3 {
		return false
	}
	ops := "+-*/%=<>!&|^~?:."
	for _, c := range s {
		if !strings.ContainsRune(ops, c) {
			return false
		}
	}
	return true
}
