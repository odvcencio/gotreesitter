package grammargen

import (
	"testing"
)

// TestNestedInlineExpansion verifies that nested inline rules are fully expanded.
// This mimics JavaScript's pattern → _lhs_expression → _identifier chain.
func TestNestedInlineExpansion(t *testing.T) {
	g := &Grammar{
		Name: "test_nested_inline",
		Rules: map[string]*Rule{
			"document":          Sym("pattern"),
			"pattern":           Choice(Sym("_lhs_expression"), Sym("rest_pattern")),
			"rest_pattern":      Seq(Str("..."), Sym("_lhs_expression")),
			"_lhs_expression":   Choice(Sym("member_expression"), Sym("_identifier")),
			"_identifier":       Choice(Sym("undefined"), Sym("identifier")),
			"member_expression": Seq(Sym("identifier"), Str("."), Sym("identifier")),
			"identifier":        Pat(`[a-z]+`),
			"undefined":         Str("undefined"),
		},
		RuleOrder: []string{
			"document", "pattern", "rest_pattern", "_lhs_expression",
			"_identifier", "member_expression", "identifier", "undefined",
		},
		Inline: []string{"_lhs_expression", "_identifier"},
	}

	// After inline expansion, pattern should reference:
	// - member_expression (from _lhs_expression)
	// - undefined (from _lhs_expression → _identifier)
	// - identifier (from _lhs_expression → _identifier)
	// - rest_pattern
	// NOT: _identifier (should be expanded) or _lhs_expression (should be expanded)

	expanded := expandInlineRules(g)

	// Check that _lhs_expression and _identifier are removed
	for _, name := range expanded.RuleOrder {
		if name == "_lhs_expression" || name == "_identifier" {
			t.Errorf("inline rule %q still in RuleOrder after expansion", name)
		}
	}
	if _, ok := expanded.Rules["_lhs_expression"]; ok {
		t.Error("_lhs_expression still in Rules")
	}
	if _, ok := expanded.Rules["_identifier"]; ok {
		t.Error("_identifier still in Rules")
	}

	// Check pattern's rule tree
	patternRule := expanded.Rules["pattern"]
	if patternRule == nil {
		t.Fatal("pattern rule not found after expansion")
	}
	t.Logf("Expanded pattern rule: %s", ruleTreeString(patternRule, 0))

	// Check rest_pattern's rule tree
	restRule := expanded.Rules["rest_pattern"]
	if restRule == nil {
		t.Fatal("rest_pattern rule not found after expansion")
	}
	t.Logf("Expanded rest_pattern rule: %s", ruleTreeString(restRule, 0))

	// Now normalize and check for epsilon productions
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	symNameToID := make(map[string]int)
	for i, info := range ng.Symbols {
		symNameToID[info.Name] = i
	}

	// Check pattern productions
	patternID := symNameToID["pattern"]
	t.Logf("\nProductions for 'pattern' (id=%d):", patternID)
	hasEpsilon := false
	for _, p := range ng.Productions {
		if p.LHS == patternID {
			rhsNames := make([]string, len(p.RHS))
			for j, rid := range p.RHS {
				if rid >= 0 && rid < len(ng.Symbols) {
					rhsNames[j] = ng.Symbols[rid].Name
				}
			}
			t.Logf("  cc=%d → %v", len(p.RHS), rhsNames)
			if len(p.RHS) == 0 {
				hasEpsilon = true
			}
		}
	}
	if hasEpsilon {
		t.Error("BUG: pattern has an epsilon production (cc=0) — nested inline expansion failed")
	}

	// Check for any dangling references to _identifier or _lhs_expression
	for _, p := range ng.Productions {
		for _, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				name := ng.Symbols[rid].Name
				if name == "_identifier" || name == "_lhs_expression" {
					lhsName := "?"
					if p.LHS >= 0 && p.LHS < len(ng.Symbols) {
						lhsName = ng.Symbols[p.LHS].Name
					}
					t.Errorf("dangling inline ref: production for %s references %s", lhsName, name)
				}
			}
		}
	}
}

// TestDeepInlineChainNoEpsilon verifies that inline chains deeper than
// maxInlineSubstDepth don't create epsilon productions. Unexpanded inline
// rules should be preserved as hidden rules, not dangled.
func TestDeepInlineChainNoEpsilon(t *testing.T) {
	// Chain: _a → _b → _c → _d → leaf (4 levels deep, exceeds maxInlineSubstDepth=2)
	g := &Grammar{
		Name: "test_deep_inline",
		Rules: map[string]*Rule{
			"document": Sym("expr"),
			"expr":     Choice(Sym("_a"), Sym("literal")),
			"_a":       Choice(Sym("_b"), Sym("plus_expr")),
			"_b":       Choice(Sym("_c"), Sym("minus_expr")),
			"_c":       Choice(Sym("_d"), Sym("mul_expr")),
			"_d":       Choice(Sym("leaf"), Sym("div_expr")),
			"plus_expr":  Seq(Sym("expr"), Str("+"), Sym("expr")),
			"minus_expr": Seq(Sym("expr"), Str("-"), Sym("expr")),
			"mul_expr":   Seq(Sym("expr"), Str("*"), Sym("expr")),
			"div_expr":   Seq(Sym("expr"), Str("/"), Sym("expr")),
			"leaf":       Pat(`[a-z]+`),
			"literal":    Pat(`[0-9]+`),
		},
		RuleOrder: []string{
			"document", "expr", "_a", "_b", "_c", "_d",
			"plus_expr", "minus_expr", "mul_expr", "div_expr",
			"leaf", "literal",
		},
		Inline: []string{"_a", "_b", "_c", "_d"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	symNameToID := make(map[string]int)
	for i, info := range ng.Symbols {
		symNameToID[info.Name] = i
	}

	// Check for epsilon productions in expr
	exprID := symNameToID["expr"]
	t.Logf("Productions for 'expr' (id=%d):", exprID)
	for _, p := range ng.Productions {
		if p.LHS == exprID {
			rhsNames := make([]string, len(p.RHS))
			for j, rid := range p.RHS {
				if rid < len(ng.Symbols) {
					rhsNames[j] = ng.Symbols[rid].Name
				}
			}
			t.Logf("  cc=%d → %v", len(p.RHS), rhsNames)
			if len(p.RHS) == 0 {
				t.Error("BUG: expr has epsilon production — deep inline chain created dangling ref")
			}
		}
	}

	// Check that no symbol has an epsilon production (except augmented start maybe)
	for _, p := range ng.Productions {
		if len(p.RHS) == 0 && p.LHS != ng.AugmentProdID {
			lhsName := "?"
			if p.LHS < len(ng.Symbols) {
				lhsName = ng.Symbols[p.LHS].Name
			}
			t.Errorf("unexpected epsilon production for %q (id=%d)", lhsName, p.LHS)
		}
	}
}
