package grammargen

import (
	"fmt"
	"sort"
	"strings"
)

// GenerateHighlightQueries produces tree-sitter highlight queries for rules
// added by a grammar extension. It diffs base and extended to find new rules,
// then applies naming conventions to generate appropriate highlights.
//
// Conventions:
//   - New Str() tokens matching identifier pattern -> @keyword
//   - *_declaration with "name" field -> name: (identifier) @type.definition
//   - *_variant with "name" field -> name: (identifier) @constructor
//   - *_block with "description" field -> description: @string
//   - *_expression -> no default highlight (expressions are structural)
//   - *_statement -> no default highlight
//   - Field named "params"/"parameters" -> children (identifier) @variable.parameter
//   - let_declaration name -> @variable.definition
//   - New string tokens that are operators (non-alphanumeric) -> @operator
//   - New string tokens that are keywords (alphanumeric) -> @keyword
func GenerateHighlightQueries(base, extended *Grammar) string {
	// Find new rules
	newRules := make(map[string]*Rule)
	for name, rule := range extended.Rules {
		if _, exists := base.Rules[name]; !exists {
			newRules[name] = rule
		}
	}

	if len(newRules) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(";; Auto-generated highlight queries for grammar extension\n")
	fmt.Fprintf(&b, ";; Extension: %s (extends %s)\n\n", extended.Name, base.Name)

	// Collect new keywords and operators from Str() nodes in new rules
	keywords := highlightCollectNewKeywords(base, extended)
	operators := highlightCollectNewOperators(base, extended)

	// Emit keyword highlights
	if len(keywords) > 0 {
		b.WriteString(";; Keywords\n")
		for _, kw := range keywords {
			fmt.Fprintf(&b, "%q @keyword\n", kw)
		}
		b.WriteString("\n")
	}

	// Emit operator highlights
	if len(operators) > 0 {
		b.WriteString(";; Operators\n")
		for _, op := range operators {
			fmt.Fprintf(&b, "%q @operator\n", op)
		}
		b.WriteString("\n")
	}

	// Emit rule-specific highlights based on naming conventions
	// Process in rule order for deterministic output
	for _, name := range extended.RuleOrder {
		rule, isNew := newRules[name]
		if !isNew || strings.HasPrefix(name, "_") {
			continue // skip hidden rules and base rules
		}

		queries := highlightGenerateRuleHighlights(name, rule)
		if queries != "" {
			b.WriteString(queries)
		}
	}

	return b.String()
}

func highlightGenerateRuleHighlights(name string, rule *Rule) string {
	var b strings.Builder

	// Collect field names from the rule
	fields := highlightCollectFields(rule)

	switch {
	case name == "let_declaration":
		fmt.Fprintf(&b, ";; %s\n", name)
		if _, hasName := fields["name"]; hasName {
			fmt.Fprintf(&b, "(%s name: (identifier) @variable.definition)\n", name)
		}
		b.WriteString("\n")

	case strings.HasSuffix(name, "_declaration"):
		// Declarations: highlight the name field
		fmt.Fprintf(&b, ";; %s\n", name)
		if _, hasName := fields["name"]; hasName {
			fmt.Fprintf(&b, "(%s name: (identifier) @type.definition)\n", name)
		}
		// Check for parameter fields
		for _, pf := range []string{"params", "parameters"} {
			if _, has := fields[pf]; has {
				fmt.Fprintf(&b, "(%s %s: (parameter_list (parameter_declaration name: (identifier) @variable.parameter)))\n", name, pf)
			}
		}
		b.WriteString("\n")

	case strings.HasSuffix(name, "_variant"):
		fmt.Fprintf(&b, ";; %s\n", name)
		if _, hasName := fields["name"]; hasName {
			fmt.Fprintf(&b, "(%s name: (identifier) @constructor)\n", name)
		}
		b.WriteString("\n")

	case strings.HasSuffix(name, "_block") && !strings.HasSuffix(name, "_do_block"):
		fmt.Fprintf(&b, ";; %s\n", name)
		if _, hasDesc := fields["description"]; hasDesc {
			fmt.Fprintf(&b, "(%s description: (_) @string)\n", name)
		}
		if _, hasName := fields["name"]; hasName {
			fmt.Fprintf(&b, "(%s name: (_) @string)\n", name)
		}
		b.WriteString("\n")

	case strings.HasSuffix(name, "_expression"):
		// Most expressions don't need special highlights
		if name == "match_expression" {
			fmt.Fprintf(&b, ";; %s\n", name)
			if _, hasSub := fields["subject"]; hasSub {
				fmt.Fprintf(&b, "(%s subject: (identifier) @variable)\n", name)
			}
			b.WriteString("\n")
		}
		if name == "ternary_expression" {
			fmt.Fprintf(&b, ";; %s\n(%s \"?\" @operator \":\" @operator)\n\n", name, name)
		}
		if name == "lambda_expression" {
			fmt.Fprintf(&b, ";; %s\n", name)
			fmt.Fprintf(&b, "(%s (lambda_params (identifier) @variable.parameter))\n\n", name)
		}

	case strings.HasSuffix(name, "_arm"):
		fmt.Fprintf(&b, ";; %s\n", name)
		if _, hasPat := fields["pattern"]; hasPat {
			fmt.Fprintf(&b, "(%s pattern: (identifier) @constant)\n", name)
		}
		if _, hasGuard := fields["guard"]; hasGuard {
			fmt.Fprintf(&b, "(%s \"if\" @keyword)\n", name)
		}
		b.WriteString("\n")

	case strings.HasSuffix(name, "_method"):
		fmt.Fprintf(&b, ";; %s\n", name)
		if _, hasName := fields["name"]; hasName {
			fmt.Fprintf(&b, "(%s name: (identifier) @function.method)\n", name)
		}
		b.WriteString("\n")

	case strings.HasSuffix(name, "_directive"):
		// Directives like no_leaks, report_allocs -- the keyword itself is highlighted
		// No additional rule-level highlight needed

	case strings.HasSuffix(name, "_statement"):
		// Statements like expect_statement, verify_statement
		// The keyword is already highlighted; fields may need annotation
		if _, hasActual := fields["actual"]; hasActual {
			fmt.Fprintf(&b, ";; %s\n(%s actual: (identifier) @variable)\n\n", name, name)
		}
	}

	return b.String()
}

// highlightCollectFields walks a rule tree and returns all field names.
func highlightCollectFields(rule *Rule) map[string]bool {
	fields := make(map[string]bool)
	var walk func(r *Rule)
	walk = func(r *Rule) {
		if r == nil {
			return
		}
		if r.Kind == RuleField {
			fields[r.Value] = true
		}
		for _, c := range r.Children {
			walk(c)
		}
	}
	walk(rule)
	return fields
}

// highlightCollectNewKeywords finds Str() nodes in new rules that look like keywords.
func highlightCollectNewKeywords(base, extended *Grammar) []string {
	baseStrings := highlightCollectAllStrings(base)
	extStrings := highlightCollectAllStrings(extended)

	var keywords []string
	seen := make(map[string]bool)
	for s := range extStrings {
		if baseStrings[s] || seen[s] {
			continue
		}
		if highlightIsIdentifierLike(s) && len(s) > 1 {
			keywords = append(keywords, s)
			seen[s] = true
		}
	}

	// Sort for deterministic output
	sort.Strings(keywords)
	return keywords
}

// highlightCollectNewOperators finds Str() nodes that look like operators.
func highlightCollectNewOperators(base, extended *Grammar) []string {
	baseStrings := highlightCollectAllStrings(base)
	extStrings := highlightCollectAllStrings(extended)

	var ops []string
	seen := make(map[string]bool)
	for s := range extStrings {
		if baseStrings[s] || seen[s] {
			continue
		}
		if isOperatorLike(s) && s != "(" && s != ")" && s != "{" && s != "}" && s != "," && s != ";" {
			ops = append(ops, s)
			seen[s] = true
		}
	}
	sort.Strings(ops)
	return ops
}

// highlightCollectAllStrings collects all unique string terminals from a grammar,
// including synthesized strings from Token/ImmToken wrapping Seq of Str nodes.
func highlightCollectAllStrings(g *Grammar) map[string]bool {
	strs := make(map[string]bool)
	for _, rule := range g.Rules {
		highlightWalkStrings(rule, strs)
	}
	return strs
}

// highlightWalkStrings walks a rule tree collecting string literals.
// For Token/ImmToken nodes wrapping a Seq of only Str children, it also
// synthesizes the concatenated string (e.g., Token(Seq(Str("?"), Str("?"))) -> "??").
func highlightWalkStrings(r *Rule, out map[string]bool) {
	if r == nil {
		return
	}
	if r.Kind == RuleString {
		out[r.Value] = true
	}
	// Detect Token/ImmToken wrapping a Seq of Str nodes -> synthesize combined string
	if (r.Kind == RuleToken || r.Kind == RuleImmToken) && len(r.Children) == 1 {
		child := r.Children[0]
		if child.Kind == RuleSeq && len(child.Children) > 0 {
			allStr := true
			var combined strings.Builder
			for _, sc := range child.Children {
				if sc.Kind != RuleString {
					allStr = false
					break
				}
				combined.WriteString(sc.Value)
			}
			if allStr {
				out[combined.String()] = true
			}
		}
	}
	for _, c := range r.Children {
		highlightWalkStrings(c, out)
	}
}

func highlightIsIdentifierLike(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
