package gotreesitter

import "slices"

type queryChildStepInfo struct {
	stepIdx int
	field   FieldID
}

func (q *Query) matchStepsWithPredicates(steps []QueryStep, stepIdx int, node *Node, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	return q.matchStepsWithParentPredicates(steps, stepIdx, node, nil, -1, lang, source, predicates, captures)
}

func (q *Query) matchStepsWithParentPredicates(steps []QueryStep, stepIdx int, node *Node, parent *Node, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	matched := false
	matchStepsAllWithReader(q, steps, stepIdx, node, parent, childIdx, lang, source, predicates, *captures, newQueryMatchBudget(defaultQueryMatchWorkBudget), publicQueryReader{}, func(next []QueryCapture) {
		if !matched {
			*captures = cloneQueryCaptures(next)
			matched = true
		}
	})
	return matched
}

// appendCaptureIDs appends every capture in ids, including ones a caller has
// disabled with DisableCapture. Predicates must see the full capture set, so
// disabled names are dropped only from a finished match's output; see
// filterDisabledCaptures.
func (q *Query) appendCaptureIDs(ids []int, node *Node, captures *[]QueryCapture) {
	if len(ids) == 0 {
		return
	}
	start := len(*captures)
	*captures = slices.Grow(*captures, len(ids))
	expanded := (*captures)[:start+len(ids)]
	for i, captureID := range ids {
		expanded[start+i] = QueryCapture{
			Name: q.captures[captureID],
			Node: node,
		}
	}
	*captures = expanded
}

// filterDisabledCaptures drops captures whose name DisableCapture removed
// from a finished match's captures, after predicates and directives have
// already evaluated the full set. DisableCapture's doc comment promises
// matching is unchanged; this keeps that promise by filtering only the
// returned output, not the captures predicates see while matching runs.
func (q *Query) filterDisabledCaptures(captures []QueryCapture) []QueryCapture {
	if len(q.disabledCaptureName) == 0 || len(captures) == 0 {
		return captures
	}
	out := captures[:0]
	for _, capture := range captures {
		if !q.isCaptureDisabled(capture.Name) {
			out = append(out, capture)
		}
	}
	return out
}

func cloneQueryCaptures(captures []QueryCapture) []QueryCapture {
	if len(captures) == 0 {
		return nil
	}
	out := make([]QueryCapture, len(captures))
	copy(out, captures)
	return out
}

// Both cursor and batch execution specialize the same reader matcher.
func (q *Query) matchPatternAll(pat *Pattern, node *Node, lang *Language, source []byte, budget *queryMatchBudget) [][]QueryCapture {
	return matchPatternAllWithReader(q, pat, node, lang, source, budget, publicQueryReader{})
}

func (q *Query) matchPatternPostorderAll(pat *Pattern, node *Node, parent *Node, childIdx int, lang *Language, source []byte, budget *queryMatchBudget) [][]QueryCapture {
	return matchPatternPostorderAllWithReader(q, pat, node, parent, childIdx, lang, source, budget, publicQueryReader{})
}

func (q *Query) alternativeFieldMatches(alt *alternativeSymbol, node *Node, parent *Node, childIdx int, lang *Language) bool {
	if node != nil && (parent == nil || childIdx < 0) {
		if linkedParent, linkedIdx, ok := nodeParentLink(node); linkedParent != nil && ok && linkedIdx >= 0 {
			parent, childIdx = linkedParent, linkedIdx
		}
	}
	return alternativeFieldMatchesWithReader(alt, node, parent, childIdx, lang, publicQueryReader{})
}

func stepUsesContiguousRun(step *QueryStep) bool {
	return step.quantifier == queryQuantifierZeroOrMore || step.quantifier == queryQuantifierOneOrMore
}

func quantifierBounds(quantifier queryQuantifier) (int, int, bool) {
	switch quantifier {
	case queryQuantifierOne:
		return 1, 1, true
	case queryQuantifierZeroOrOne:
		return 0, 1, true
	case queryQuantifierZeroOrMore:
		return 0, -1, true
	case queryQuantifierOneOrMore:
		return 1, -1, true
	default:
		return 0, 0, false
	}
}

// stepAnchorsSatisfied applies the C query cursor's anchor rules to the
// children a step matched. A leading anchor requires that no named sibling
// sits between the previous matched child and the first child of this
// span; anonymous siblings never break an anchor, except after an unnamed
// wildcard step, which C treats as seeking an immediate match of any node.
// A trailing anchor requires that no named sibling follows the span's last
// child. Both rules apply whether the matched children are named or not,
// which is how C matches `(identifier) . "="`.
func (q *Query) stepAnchorsSatisfied(
	step *QueryStep,
	namedPosByIndex []int,
	span childStepNamedSpan,
	prev childStepPrevMatch,
	parentLastNamedPos int,
) bool {
	if step.anchorBefore && prev.namedBoundary >= 0 {
		if span.firstIdx < 0 {
			return false
		}
		if prev.unnamedWildcard && prev.lastIdx >= 0 {
			if span.firstIdx != prev.lastIdx+1 {
				return false
			}
		} else if namedCountBefore(namedPosByIndex, span.firstIdx) != prev.namedBoundary {
			return false
		}
	}
	if step.anchorAfter {
		if span.lastIdx < 0 {
			return false
		}
		if namedCountBefore(namedPosByIndex, span.lastIdx+1) != parentLastNamedPos+1 {
			return false
		}
	}
	return true
}

// childStepPrevMatch records where the previous child steps ended:
// namedBoundary is the number of named children consumed before the next
// step may start, lastIdx is the index of the last matched child (-1 when
// no child matched yet), and unnamedWildcard reports whether the step that
// matched lastIdx was an unnamed wildcard.
type childStepPrevMatch struct {
	namedBoundary   int
	lastIdx         int
	unnamedWildcard bool
}

// namedCountBefore returns the number of named children with an index
// below idx.
func namedCountBefore(namedPosByIndex []int, idx int) int {
	if idx > len(namedPosByIndex) {
		idx = len(namedPosByIndex)
	}
	for i := idx - 1; i >= 0; i-- {
		if pos := namedPosByIndex[i]; pos >= 0 {
			return pos + 1
		}
	}
	return 0
}

// advancePrevMatch returns the previous-match record after a step matched
// span (or nothing, when span is empty).
func advancePrevMatch(prev childStepPrevMatch, step *QueryStep, namedPosByIndex []int, span childStepNamedSpan) childStepPrevMatch {
	if span.lastIdx < 0 {
		// An anchor after an omitted first optional/repeated child does not
		// make its successor the first named child. A leading anchor on that
		// optional child still constrains the successor to the first child.
		if prev.lastIdx < 0 && !step.anchorBefore {
			prev.namedBoundary = -1
		}
		return prev
	}
	return childStepPrevMatch{
		namedBoundary:   namedCountBefore(namedPosByIndex, span.lastIdx+1),
		lastIdx:         span.lastIdx,
		unnamedWildcard: step.symbol == 0 && !step.isNamed && step.textMatch == "" && len(step.alternatives) == 0,
	}
}

type childStepNamedSpan struct {
	hasNamed bool
	first    int
	last     int
	// firstIdx and lastIdx are the child indices of the span's first and
	// last matched children, or -1 when the span is empty.
	firstIdx int
	lastIdx  int
}

func emptyChildStepNamedSpan() childStepNamedSpan {
	return childStepNamedSpan{first: -1, last: -1, firstIdx: -1, lastIdx: -1}
}

func (s childStepNamedSpan) withChild(childIdx int, namedPos int) childStepNamedSpan {
	if s.firstIdx < 0 {
		s.firstIdx = childIdx
	}
	s.lastIdx = childIdx
	if namedPos < 0 {
		return s
	}
	if !s.hasNamed {
		s.hasNamed = true
		s.first = namedPos
	}
	s.last = namedPos
	return s
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func queryStepHasNestedChildren(steps []QueryStep, stepIdx int) bool {
	if stepIdx < 0 || stepIdx+1 >= len(steps) {
		return false
	}
	return steps[stepIdx+1].depth > steps[stepIdx].depth
}

func (q *Query) stackEntryCanMatchStep(step *QueryStep, entry stackEntry, lang *Language) bool {
	if !stackEntryHasNode(entry) {
		return false
	}
	nodeNamed := stackEntryNodeIsNamed(entry)
	nodeSymbol := lang.PublicSymbolForNamedness(stackEntryNodeSymbol(entry), nodeNamed)
	if len(step.alternatives) > 0 {
		return stackEntryMatchesAlternatives(step, entry, lang, nodeSymbol, nodeNamed)
	}
	return stackEntryMatchesScalarStep(step, entry, lang, nodeSymbol, nodeNamed)
}

func queryStackEntryTypeName(entry stackEntry, lang *Language) string {
	if stackEntryNodeSymbol(entry) == errorSymbol {
		return "ERROR"
	}
	if lang == nil {
		return ""
	}
	symbol := stackEntryNodeSymbol(entry)
	if int(symbol) >= 0 && int(symbol) < len(lang.SymbolNames) {
		return unescapePunctuationSymbolName(lang.SymbolNames[symbol])
	}
	return ""
}

func alternativeMatchesStackEntry(alt alternativeSymbol, entry stackEntry, lang *Language, nodeSymbol Symbol, nodeNamed bool) bool {
	if alt.symbol == 0 && alt.textMatch == "" {
		if alt.isMissing {
			return stackEntryNodeIsMissing(entry)
		}
		return (!alt.isNamed || nodeNamed) && stackEntryNodeSymbol(entry) != errorSymbol && stackEntryMaySatisfySupertype(entry, lang, alt.supertype)
	}
	if alt.textMatch != "" {
		return !nodeNamed && queryStackEntryTypeName(entry, lang) == alt.textMatch &&
			(!alt.isMissing || stackEntryNodeIsMissing(entry))
	}
	return nodeNamed == alt.isNamed &&
		nodeSymbol == lang.PublicSymbolForNamedness(alt.symbol, alt.isNamed) &&
		(!alt.isMissing || stackEntryNodeIsMissing(entry)) &&
		stackEntryMaySatisfySupertype(entry, lang, alt.supertype)
}

// nodeMatchesStep checks if a single node matches a single step's type/symbol constraint.
func (q *Query) nodeMatchesStep(step *QueryStep, node *Node, lang *Language) bool {
	if len(step.alternatives) > 0 {
		return nodeMatchesAlternatives(step, node, lang)
	}
	return nodeMatchesScalarStep(step, node, lang)
}

func stackEntryMatchesAlternatives(step *QueryStep, entry stackEntry, lang *Language, nodeSymbol Symbol, nodeNamed bool) bool {
	if idx := step.altIndex; idx != nil {
		return indexedAlternativesMatchStackEntry(idx, entry, lang, nodeSymbol, nodeNamed)
	}
	for _, alt := range step.alternatives {
		if alternativeMatchesStackEntry(alt, entry, lang, nodeSymbol, nodeNamed) {
			return true
		}
	}
	return false
}

func indexedAlternativesMatchStackEntry(idx *queryAlternationIndex, entry stackEntry, lang *Language, nodeSymbol Symbol, nodeNamed bool) bool {
	if len(idx.wildcard) > 0 {
		return true
	}
	if len(idx.bySymbolNamed[alternationSymbolNamedKey(nodeSymbol, nodeNamed)]) > 0 {
		return true
	}
	if !nodeNamed && len(idx.byText) > 0 {
		return len(idx.byText[queryStackEntryTypeName(entry, lang)]) > 0
	}
	return false
}

func stackEntryMatchesScalarStep(step *QueryStep, entry stackEntry, lang *Language, nodeSymbol Symbol, nodeNamed bool) bool {
	if step.textMatch != "" {
		if !nodeNamed && queryStackEntryTypeName(entry, lang) == step.textMatch {
			return !step.isMissing || stackEntryNodeIsMissing(entry)
		}
		return false
	}
	if step.symbol == 0 {
		if step.isMissing {
			return stackEntryNodeIsMissing(entry)
		}
		return (!step.isNamed || nodeNamed) && stackEntryNodeSymbol(entry) != errorSymbol && stackEntryMaySatisfySupertype(entry, lang, step.supertype)
	}
	if nodeNamed != step.isNamed || nodeSymbol != lang.PublicSymbolForNamedness(step.symbol, step.isNamed) {
		return false
	}
	return (!step.isMissing || stackEntryNodeIsMissing(entry)) && stackEntryMaySatisfySupertype(entry, lang, step.supertype)
}

func nodeMatchesAlternatives(step *QueryStep, node *Node, lang *Language) bool {
	nodeNamed := node.IsNamed()
	nodeSymbol := lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed)
	if idx := step.altIndex; idx != nil {
		return indexedAlternativesMatchNode(idx, node, lang, nodeSymbol, nodeNamed)
	}

	var nodeType string
	nodeTypeLoaded := false
	for _, alt := range step.alternatives {
		if alternativeMatchesNodeCached(alt, node, lang, nodeSymbol, nodeNamed, &nodeType, &nodeTypeLoaded) {
			return true
		}
	}
	return false
}

func indexedAlternativesMatchNode(idx *queryAlternationIndex, node *Node, lang *Language, nodeSymbol Symbol, nodeNamed bool) bool {
	if len(idx.wildcard) > 0 {
		return true
	}
	if len(idx.bySymbolNamed[alternationSymbolNamedKey(nodeSymbol, nodeNamed)]) > 0 {
		return true
	}
	if !nodeNamed && len(idx.byText) > 0 {
		return len(idx.byText[node.Type(lang)]) > 0
	}
	return false
}

func nodeMatchesScalarStep(step *QueryStep, node *Node, lang *Language) bool {
	if step.textMatch != "" {
		if !node.IsNamed() && node.Type(lang) == step.textMatch {
			return !step.isMissing || node.IsMissing()
		}
		return false
	}

	if step.symbol == 0 {
		if step.isMissing {
			return node.IsMissing()
		}
		// A wildcard never matches an ERROR node (ts_query_cursor__advance).
		if node.IsError() {
			return false
		}
		if step.supertype != 0 && !node.hasSupertype(lang, step.supertype) {
			return false
		}
		return !step.isNamed || node.IsNamed()
	}

	nodeNamed := node.IsNamed()
	if nodeNamed != step.isNamed {
		return false
	}

	if lang.PublicSymbolForNamedness(node.Symbol(), nodeNamed) != lang.PublicSymbolForNamedness(step.symbol, step.isNamed) {
		return false
	}
	if step.isMissing && !node.IsMissing() {
		return false
	}
	if step.supertype != 0 && !node.hasSupertype(lang, step.supertype) {
		return false
	}

	return nodeAbsentFieldsSatisfied(step, node, lang)
}

// stackEntryMaySatisfySupertype is the entry-level prefilter for a supertype
// step. An entry that owns a Node answers from the node's record; any other
// entry defers to the node check after materialization.
func stackEntryMaySatisfySupertype(entry stackEntry, lang *Language, supertype Symbol) bool {
	if supertype == 0 {
		return true
	}
	if n := stackEntryNode(entry); n != nil {
		return n.hasSupertype(lang, supertype)
	}
	return true
}

func nodeAbsentFieldsSatisfied(step *QueryStep, node *Node, lang *Language) bool {
	for _, fid := range step.absentFields {
		if int(fid) <= 0 || int(fid) >= len(lang.FieldNames) {
			return false
		}
		fieldName := lang.FieldNames[fid]
		if fieldName == "" {
			return false
		}
		if node.ChildByFieldName(fieldName, lang) != nil {
			return false
		}
	}

	return true
}

func alternativeMatchesNodeCached(
	alt alternativeSymbol,
	node *Node,
	lang *Language,
	nodeSymbol Symbol,
	nodeNamed bool,
	nodeType *string,
	nodeTypeLoaded *bool,
) bool {
	// Wildcard in alternation `( _ )` should match any node.
	if alt.symbol == 0 && alt.textMatch == "" {
		if alt.isMissing {
			return node.IsMissing()
		}
		if node.IsError() {
			return false
		}
		if alt.supertype != 0 && !node.hasSupertype(lang, alt.supertype) {
			return false
		}
		return !alt.isNamed || nodeNamed
	}

	if alt.textMatch != "" {
		// String match for anonymous nodes.
		if nodeNamed {
			return false
		}
		if !*nodeTypeLoaded {
			*nodeType = node.Type(lang)
			*nodeTypeLoaded = true
		}
		return *nodeType == alt.textMatch && (!alt.isMissing || node.IsMissing())
	}

	return nodeNamed == alt.isNamed &&
		nodeSymbol == lang.PublicSymbolForNamedness(alt.symbol, alt.isNamed) &&
		(!alt.isMissing || node.IsMissing()) &&
		(alt.supertype == 0 || node.hasSupertype(lang, alt.supertype))
}
