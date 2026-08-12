package gotreesitter

// resolveOutlineOwner resolves the non-lexical Owner of one definition node
// against the rules registered for its NodeType.
//
// A NodeType with no rule leaves Owner empty and never touches
// report.OwnerRuleMisses: the rule set makes no claim about that shape, so
// there is nothing to miss. A NodeType WITH one or more rules tries each row
// in order and returns the first Owner a row resolves. When every row for a
// matched NodeType fails -- the field is absent, the unwrap chain runs out,
// or it does not end at exactly one accepted terminal node --
// report.OwnerRuleMisses counts the symbol once and Owner stays empty.
//
// rules, node, and lang may all be nil or empty; each case is a miss-free
// "no rule applies" outcome, exactly like a NodeType absent from rules. This
// keeps the call safe from buildOutlineForest, which always calls it, even
// when no WithOutlineOwnerRules call ever attached a table.
func resolveOutlineOwner(rules map[string][]OutlineOwnerRule, node *Node, nodeType string, lang *Language, source []byte, report *OutlineReport) string {
	if len(rules) == 0 {
		return ""
	}
	rows, ok := rules[nodeType]
	if !ok || len(rows) == 0 {
		return ""
	}
	if node != nil && lang != nil {
		for _, rule := range rows {
			if owner, resolved := resolveOutlineOwnerRule(rule, node, lang, source); resolved {
				return owner
			}
		}
	}
	report.OwnerRuleMisses++
	return ""
}

// resolveOutlineOwnerRule applies one rule to one definition node. It reads
// the node's OwnerField, then walks that field's subtree through
// collectOutlineOwnerNameNodes. The rule resolves an Owner only when the walk
// reaches EXACTLY ONE node whose type is in NameTypes; any other count --
// zero, or more than one -- is a failure. Owner is the matched node's raw
// text: no directive and no trimming rewrites it, unlike Name.
func resolveOutlineOwnerRule(rule OutlineOwnerRule, node *Node, lang *Language, source []byte) (string, bool) {
	field := node.ChildByFieldName(rule.OwnerField, lang)
	if field == nil {
		return "", false
	}
	matches := collectOutlineOwnerNameNodes(field, rule, lang)
	if len(matches) != 1 {
		return "", false
	}
	text := matches[0].Text(source)
	if text == "" {
		return "", false
	}
	return text, true
}

// collectOutlineOwnerNameNodes walks from start and returns every node the
// walk reaches whose type is in rule.NameTypes.
//
// The walk descends only through a node whose type is in rule.Unwrap, and it
// never descends past a NameTypes match: a matched node's own children are
// never visited. A node whose type is in neither list ends that branch of the
// walk without contributing a match. Together these two stopping rules mean a
// NameTypes node reachable only through a type the rule does not list in
// Unwrap is never reached and never counted -- for example, a generic
// receiver's type ARGUMENT sits inside a node type ("type_arguments" in the
// shipped Go rule) that Unwrap does not name, so it cannot be mistaken for
// the receiver's own type name.
func collectOutlineOwnerNameNodes(start *Node, rule OutlineOwnerRule, lang *Language) []*Node {
	var matches []*Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		t := n.Type(lang)
		if outlineOwnerTypeListHas(rule.NameTypes, t) {
			matches = append(matches, n)
			return
		}
		if !outlineOwnerTypeListHas(rule.Unwrap, t) {
			return
		}
		count := n.NamedChildCount()
		for i := 0; i < count; i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(start)
	return matches
}

// outlineOwnerTypeListHas reports whether nodeType appears in list. An
// OutlineOwnerRule's Unwrap and NameTypes lists hold a handful of entries, so
// a linear scan on every resolution attempt costs less than building and
// discarding a map.
func outlineOwnerTypeListHas(list []string, nodeType string) bool {
	for _, candidate := range list {
		if candidate == nodeType {
			return true
		}
	}
	return false
}
