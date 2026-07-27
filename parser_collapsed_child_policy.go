package gotreesitter

// collapsedChildSymbolPair is one exact public-wrapper/raw-child identity that
// C tree-sitter retains as a unary node. The policy is compiled once per Parser
// from the registered occurrence ledger after an exact runtime profile admits
// the artifact. Within that profile, exact named parent and raw-child metadata
// identities select the occurrence; display-name fallback is never sufficient.
type collapsedChildSymbolPair struct {
	parent Symbol
	child  Symbol
}

// collapsedChildOccurrenceRule records the exact public-wrapper/raw-child
// identity that C tree-sitter retains. It is ownership data for native
// materialization, not a recipe for reconstructing children after their
// identity is lost.
type collapsedChildOccurrenceRule struct {
	languageName string
	parentName   string
	childName    string
	childNamed   bool
}

var collapsedChildOccurrenceRules = []collapsedChildOccurrenceRule{
	{languageName: "ruby", parentName: "bare_string", childName: "string_content", childNamed: true},
	{languageName: "ruby", parentName: "bare_symbol", childName: "string_content", childNamed: true},
	{languageName: "apex", parentName: "after_delete", childName: "after_delete"},
	{languageName: "apex", parentName: "after_insert", childName: "after_insert"},
	{languageName: "apex", parentName: "after_undelete", childName: "after_undelete"},
	{languageName: "apex", parentName: "after_update", childName: "after_update"},
	{languageName: "apex", parentName: "before_delete", childName: "before_delete"},
	{languageName: "apex", parentName: "before_insert", childName: "before_insert"},
	{languageName: "apex", parentName: "before_update", childName: "before_update"},
	{languageName: "apex", parentName: "delete", childName: "delete"},
	{languageName: "apex", parentName: "insert", childName: "insert"},
	{languageName: "apex", parentName: "super", childName: "super"},
	{languageName: "apex", parentName: "system", childName: "system"},
	{languageName: "apex", parentName: "undelete", childName: "undelete"},
	{languageName: "apex", parentName: "upsert", childName: "upsert"},
	{languageName: "apex", parentName: "user", childName: "user"},
	{languageName: "kotlin", parentName: "identifier", childName: "simple_identifier", childNamed: true},
	{languageName: "hack", parentName: "true", childName: "true"},
	{languageName: "hack", parentName: "false", childName: "false"},
	{languageName: "hack", parentName: "null", childName: "null"},
	{languageName: "dart", parentName: "super", childName: "super"},
	{languageName: "dart", parentName: "this", childName: "this"},
	{languageName: "elixir", parentName: "nil", childName: "nil"},
	{languageName: "rust", parentName: "range_expression", childName: ".."},
}

func collapsedChildPairKey(parent, child Symbol) uint32 {
	return uint32(parent)<<16 | uint32(child)
}

func compileCollapsedChildOccurrencePolicy(lang *Language) ([]collapsedChildSymbolPair, map[uint32]struct{}) {
	if lang == nil || lang.NativeResultCompatibility&ResultCompatibilityNativeCollapsedChildren == 0 {
		return nil, nil
	}
	pairs := make([]collapsedChildSymbolPair, 0, 4)
	seen := make(map[uint32]struct{})
	for _, rule := range collapsedChildOccurrenceRules {
		if rule.languageName != lang.Name {
			continue
		}
		parent, ok := lang.symbolByNameAndNamed(rule.parentName, true)
		if !ok {
			continue
		}
		child, ok := lang.symbolByNameAndNamed(rule.childName, rule.childNamed)
		if !ok {
			continue
		}
		key := collapsedChildPairKey(parent, child)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		pairs = append(pairs, collapsedChildSymbolPair{parent: parent, child: child})
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	return pairs, seen
}

// retainsCollapsedChildOccurrence is the single native keep/wrap decision.
// Its registered (parent, raw-child) domain deliberately fails closed once raw
// child identity has been lost; no display-name-based reconstruction follows.
func (p *Parser) retainsCollapsedChildOccurrence(parent, rawChild Symbol) bool {
	if p == nil || len(p.collapsedChildOccurrenceSet) == 0 {
		return false
	}
	_, ok := p.collapsedChildOccurrenceSet[collapsedChildPairKey(parent, rawChild)]
	return ok
}
