package grammargen

// TSXGrammar returns the tsx grammar. It is the typescript grammar with four
// edits, so a typescript fix reaches tsx without a second copy of the rules.
// Rule and conflict order carry into the parse tables, so keep the edits
// in place.
func TSXGrammar() *Grammar {
	g := TypeScriptGrammar()
	g.Name = "tsx"

	// Angle-bracket type assertions clash with JSX, so tsx drops them and
	// parses JSX elements as expressions instead.
	expr := g.Rules["expression"]
	members := make([]*Rule, 0, len(expr.Children))
	for _, m := range expr.Children {
		if m.Kind == RuleSymbol && m.Value == "type_assertion" {
			continue
		}
		members = append(members, m)
		if m.Kind == RuleSymbol && m.Value == "primary_expression" {
			members = append(members, Sym("_jsx_element"))
		}
	}
	expr.Children = members

	for i, c := range g.Conflicts {
		if len(c) == 2 && c[0] == "primary_type" && c[1] == "type_parameter" {
			g.Conflicts[i] = []string{"jsx_opening_element", "type_parameter"}
		}
	}
	g.Conflicts = append(g.Conflicts, []string{"jsx_namespace_name", "primary_type"})

	return g
}
