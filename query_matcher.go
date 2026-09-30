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
	if stepIdx >= len(steps) {
		return false
	}

	step := &steps[stepIdx]

	if len(step.alternatives) > 0 {
		if !q.matchAlternationStep(step, node, parent, childIdx, lang, source, predicates, captures) {
			return false
		}
	} else {
		// Check if this node matches the current step.
		if !q.nodeMatchesStep(step, node, lang) {
			return false
		}
		q.appendCaptureIDs(step.captureIDs, node, captures)
		if !q.predicatesStillViable(predicates, *captures, source) {
			return false
		}
	}

	// Find child steps (steps at depth = step.depth + 1) that are direct
	// descendants of this step.
	childDepth := step.depth + 1
	childStart := stepIdx + 1

	// If there are no more steps, we matched successfully.
	if childStart >= len(steps) {
		return true
	}

	// If the next step is at the same depth or shallower, there are no
	// child constraints -- we matched.
	if steps[childStart].depth <= step.depth {
		return true
	}

	// Collect child step indices at childDepth (stop when we see a step
	// at a depth <= step.depth, meaning it belongs to a sibling/ancestor).
	var childStepsBuf [16]queryChildStepInfo
	childSteps := childStepsBuf[:0]
	for i := childStart; i < len(steps); i++ {
		if steps[i].depth <= step.depth {
			break
		}
		if steps[i].depth == childDepth {
			childSteps = append(childSteps, queryChildStepInfo{
				stepIdx: i,
				field:   steps[i].field,
			})
		}
	}
	return q.matchChildSteps(node, steps, childSteps, lang, source, predicates, captures)
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

func (q *Query) collectChildCandidateIndices(
	parent *Node,
	children []*Node,
	step *QueryStep,
	field FieldID,
	nextChildIdx int,
	lang *Language,
	dst []int,
) ([]int, bool) {
	fieldName, ok := queryFieldName(field, lang)
	if !ok {
		return dst, false
	}

	collector := childCandidateCollector{
		q:             q,
		parent:        parent,
		children:      children,
		step:          step,
		fieldName:     fieldName,
		nextChildIdx:  nextChildIdx,
		lang:          lang,
		dst:           dst,
		contiguousRun: stepUsesContiguousRun(step),
	}
	return collector.collect(), true
}

type childCandidateCollector struct {
	q             *Query
	parent        *Node
	children      []*Node
	step          *QueryStep
	fieldName     string
	nextChildIdx  int
	lang          *Language
	dst           []int
	contiguousRun bool
	startedRun    bool
}

func (c *childCandidateCollector) collect() []int {
	for i := c.nextChildIdx; i < len(c.children); i++ {
		switch c.candidateState(i) {
		case childCandidateMatch:
			c.dst = append(c.dst, i)
			c.startedRun = true
		case childCandidateStop:
			return c.dst
		}
	}
	return c.dst
}

type childCandidateState uint8

const (
	childCandidateSkip childCandidateState = iota
	childCandidateMatch
	childCandidateStop
)

func (c *childCandidateCollector) candidateState(childIdx int) childCandidateState {
	if c.stackEntryRejects(childIdx) {
		return c.skipOrStop()
	}
	if c.fieldMatches(childIdx) {
		return childCandidateMatch
	}
	return c.skipOrStop()
}

func (c *childCandidateCollector) stackEntryRejects(childIdx int) bool {
	entry, hasEntry := nodeChildEntryAtNoMaterialize(c.parent, childIdx)
	return !hasEntry || !c.q.stackEntryCanMatchStep(c.step, entry, c.lang)
}

func (c *childCandidateCollector) fieldMatches(childIdx int) bool {
	return c.fieldName == "" || c.parent.FieldNameForChild(childIdx, c.lang) == c.fieldName
}

func (c *childCandidateCollector) skipOrStop() childCandidateState {
	if c.contiguousRun && c.startedRun {
		return childCandidateStop
	}
	return childCandidateSkip
}

func queryFieldName(field FieldID, lang *Language) (string, bool) {
	if field == 0 {
		return "", true
	}
	if int(field) <= 0 || int(field) >= len(lang.FieldNames) {
		return "", false
	}
	fieldName := lang.FieldNames[field]
	return fieldName, fieldName != ""
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

func (q *Query) matchChildSteps(
	parent *Node,
	steps []QueryStep,
	childSteps []queryChildStepInfo,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures *[]QueryCapture,
) bool {
	childCount := nodeChildCountNoMaterialize(parent)
	var childrenBuf [64]*Node
	var children []*Node
	if childCount <= len(childrenBuf) {
		children = childrenBuf[:childCount]
	} else {
		children = make([]*Node, childCount)
	}
	var namedPosByIndexBuf [64]int
	var namedPosByIndex []int
	if childCount <= len(namedPosByIndexBuf) {
		namedPosByIndex = namedPosByIndexBuf[:childCount]
	} else {
		namedPosByIndex = make([]int, childCount)
	}
	namedPos := 0
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(parent, i)
		if ok && stackEntryNodeIsNamed(entry) {
			namedPosByIndex[i] = namedPos
			namedPos++
		} else {
			namedPosByIndex[i] = -1
		}
	}
	parentLastNamedPos := namedPos - 1

	budget := newQueryMatchBudget(defaultQueryMatchWorkBudget)
	return q.matchChildStepsRecursive(
		parent, children, namedPosByIndex, parentLastNamedPos,
		steps, childSteps, 0, 0, childStepPrevMatch{lastIdx: -1},
		lang, source, predicates, captures, budget,
	)
}

func (q *Query) matchChildStepsRecursive(
	parent *Node,
	children []*Node,
	namedPosByIndex []int,
	parentLastNamedPos int,
	steps []QueryStep,
	childSteps []queryChildStepInfo,
	childPos int,
	nextChildIdx int,
	prev childStepPrevMatch,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures *[]QueryCapture,
	budget *queryMatchBudget,
) bool {
	if childPos >= len(childSteps) {
		return true
	}

	var candidateIndicesBuf [32]int
	matcher := childStepMatcher{
		q:                  q,
		parent:             parent,
		children:           children,
		namedPosByIndex:    namedPosByIndex,
		parentLastNamedPos: parentLastNamedPos,
		steps:              steps,
		childSteps:         childSteps,
		childPos:           childPos,
		nextChildIdx:       nextChildIdx,
		prev:               prev,
		lang:               lang,
		source:             source,
		predicates:         predicates,
		captures:           captures,
		candidateIndices:   candidateIndicesBuf[:0],
		budget:             budget,
	}
	if !matcher.prepare() {
		return false
	}
	return matcher.match()
}

type childStepMatcher struct {
	q                  *Query
	parent             *Node
	children           []*Node
	namedPosByIndex    []int
	parentLastNamedPos int
	steps              []QueryStep
	childSteps         []queryChildStepInfo
	childPos           int
	nextChildIdx       int
	prev               childStepPrevMatch
	lang               *Language
	source             []byte
	predicates         []QueryPredicate
	captures           *[]QueryCapture
	budget             *queryMatchBudget

	cs               queryChildStepInfo
	step             *QueryStep
	minCount         int
	maxCount         int
	candidateIndices []int
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

func (m *childStepMatcher) prepare() bool {
	m.cs = m.childSteps[m.childPos]
	m.step = &m.steps[m.cs.stepIdx]
	minCount, maxCount, ok := quantifierBounds(m.step.quantifier)
	if !ok {
		return false
	}

	var candidatesOK bool
	m.candidateIndices, candidatesOK = m.q.collectChildCandidateIndices(
		m.parent, m.children, m.step, m.cs.field, m.nextChildIdx, m.lang, m.candidateIndices,
	)
	if !candidatesOK {
		return false
	}

	if maxCount < 0 || maxCount > len(m.candidateIndices) {
		maxCount = len(m.candidateIndices)
	}
	if minCount > len(m.candidateIndices) {
		return false
	}
	m.minCount = minCount
	m.maxCount = maxCount
	return true
}

func (m *childStepMatcher) match() bool {
	if m.canAggregateFinalStep() {
		return m.matchAggregatedFinalStep()
	}
	return m.matchQuantifierChoices()
}

func (m *childStepMatcher) canAggregateFinalStep() bool {
	return !predicatesCanRejectMatch(m.predicates) &&
		m.childPos == len(m.childSteps)-1 &&
		m.minCount == 1 &&
		m.maxCount == 1 &&
		!m.step.anchorBefore &&
		!m.step.anchorAfter
}

func (m *childStepMatcher) matchAggregatedFinalStep() bool {
	if m.canMatchAggregatedFinalStepWithoutMaterializing() {
		for _, childIdx := range m.candidateIndices {
			span := childStepNamedSpanForIndex(m.namedPosByIndex, childIdx)
			if m.anchorsSatisfied(span) {
				return true
			}
		}
		return false
	}

	any := false
	checkpoint := len(*m.captures)
	for _, childIdx := range m.candidateIndices {
		child := materializedQueryChild(m.parent, m.children, childIdx)
		if child == nil {
			continue
		}
		childCheckpoint := len(*m.captures)
		if !m.q.matchStepWithRollbackAtParentPredicates(
			m.steps, m.cs.stepIdx, child, m.parent, childIdx, m.lang, m.source, nil, m.captures,
		) {
			*m.captures = (*m.captures)[:childCheckpoint]
			continue
		}
		span := childStepNamedSpanForIndex(m.namedPosByIndex, childIdx)
		if !m.anchorsSatisfied(span) {
			*m.captures = (*m.captures)[:childCheckpoint]
			continue
		}
		any = true
	}
	if any {
		return true
	}
	*m.captures = (*m.captures)[:checkpoint]
	return false
}

func (m *childStepMatcher) matchQuantifierChoices() bool {
	for count := m.maxCount; count >= m.minCount; count-- {
		checkpoint := len(*m.captures)
		if m.matchChoiceCombinations(count, 0, 0, m.nextChildIdx, emptyChildStepNamedSpan()) {
			return true
		}

		*m.captures = (*m.captures)[:checkpoint]
		if m.budget.tripped() {
			return false
		}
	}
	return false
}

func (m *childStepMatcher) matchChoiceCombinations(count int, candidatePos int, chosen int, nextIdx int, span childStepNamedSpan) bool {
	if !m.budget.charge() {
		return false
	}
	if chosen == count {
		if !m.anchorsSatisfied(span) {
			return false
		}
		return m.q.matchChildStepsRecursive(
			m.parent, m.children, m.namedPosByIndex, m.parentLastNamedPos,
			m.steps, m.childSteps, m.childPos+1, nextIdx, advancePrevMatch(m.prev, m.step, m.namedPosByIndex, span),
			m.lang, m.source, m.predicates, m.captures, m.budget,
		)
	}

	remaining := count - chosen
	limit := len(m.candidateIndices) - remaining
	for i := candidatePos; i <= limit; i++ {
		childIdx := m.candidateIndices[i]
		child := materializedQueryChild(m.parent, m.children, childIdx)
		if child == nil {
			continue
		}

		childCheckpoint := len(*m.captures)
		if !m.q.matchStepWithRollbackAtParentPredicates(
			m.steps, m.cs.stepIdx, child, m.parent, childIdx, m.lang, m.source, m.predicates, m.captures,
		) {
			continue
		}

		nextIdxForChoice := maxInt(nextIdx, childIdx+1)
		spanForChoice := span.withChild(childIdx, m.namedPosByIndex[childIdx])
		if m.matchChoiceCombinations(count, i+1, chosen+1, nextIdxForChoice, spanForChoice) {
			return true
		}

		*m.captures = (*m.captures)[:childCheckpoint]
	}

	return false
}

func (m *childStepMatcher) anchorsSatisfied(span childStepNamedSpan) bool {
	return m.q.stepAnchorsSatisfied(m.step, m.namedPosByIndex, span, m.prev, m.parentLastNamedPos)
}

func childStepNamedSpanForIndex(namedPosByIndex []int, childIdx int) childStepNamedSpan {
	return emptyChildStepNamedSpan().withChild(childIdx, namedPosByIndex[childIdx])
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

func materializedQueryChild(parent *Node, children []*Node, childIdx int) *Node {
	if childIdx < 0 || childIdx >= len(children) {
		return nil
	}
	child := children[childIdx]
	if child == nil {
		child = nodeChildAtForReason(parent, childIdx, materializeForQuery)
		children[childIdx] = child
	}
	return child
}

func (m *childStepMatcher) canMatchAggregatedFinalStepWithoutMaterializing() bool {
	return len(m.step.captureIDs) == 0 &&
		len(m.step.alternatives) == 0 &&
		m.step.supertype == 0 &&
		!queryStepHasNestedChildren(m.steps, m.cs.stepIdx)
}

func queryStepHasNestedChildren(steps []QueryStep, stepIdx int) bool {
	if stepIdx < 0 || stepIdx+1 >= len(steps) {
		return false
	}
	return steps[stepIdx+1].depth > steps[stepIdx].depth
}

func (q *Query) matchAlternationStep(step *QueryStep, node *Node, parent *Node, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures *[]QueryCapture) bool {
	ctx := alternationMatchContext{
		q:               q,
		step:            step,
		node:            node,
		parent:          parent,
		childIdx:        childIdx,
		lang:            lang,
		source:          source,
		predicates:      predicates,
		captures:        captures,
		hasStepCaptures: len(step.captureIDs) > 0,
		nodeNamed:       node.IsNamed(),
	}
	ctx.nodeSymbol = lang.PublicSymbolForNamedness(node.Symbol(), ctx.nodeNamed)
	if idx := step.altIndex; idx != nil {
		return ctx.matchIndexedAlternation(idx)
	}
	return ctx.matchLinearAlternation()
}

type alternationMatchContext struct {
	q               *Query
	step            *QueryStep
	node            *Node
	parent          *Node
	childIdx        int
	lang            *Language
	source          []byte
	predicates      []QueryPredicate
	captures        *[]QueryCapture
	hasStepCaptures bool

	nodeNamed      bool
	nodeSymbol     Symbol
	nodeType       string
	nodeTypeLoaded bool
}

func (c *alternationMatchContext) matchIndexedAlternation(idx *queryAlternationIndex) bool {
	cursor := c.indexedAlternationCursor(idx)
	if !cursor.hasAny() {
		return false
	}
	for {
		nextAlt, sourceKind, ok := cursor.next()
		if !ok {
			return false
		}
		alt := &c.step.alternatives[nextAlt]
		if c.alternativeMatches(alt) {
			return true
		}
		cursor.advance(sourceKind)
	}
}

func (c *alternationMatchContext) matchLinearAlternation() bool {
	for _, alt := range c.step.alternatives {
		if !alternativeMatchesNodeCached(alt, c.node, c.lang, c.nodeSymbol, c.nodeNamed, &c.nodeType, &c.nodeTypeLoaded) {
			continue
		}
		if c.alternativeMatches(&alt) {
			return true
		}
	}
	return false
}

func (c *alternationMatchContext) alternativeMatches(alt *alternativeSymbol) bool {
	if alt.isMissing && !alternativeMatchesNodeCached(*alt, c.node, c.lang, c.nodeSymbol, c.nodeNamed, &c.nodeType, &c.nodeTypeLoaded) {
		return false
	}
	if !c.q.alternativeFieldMatches(alt, c.node, c.parent, c.childIdx, c.lang) {
		return false
	}
	return c.q.matchAlternationBranch(
		c.step, alt, c.node, c.lang, c.source, c.predicates, c.captures, c.hasStepCaptures,
	)
}

func (c *alternationMatchContext) indexedAlternationCursor(idx *queryAlternationIndex) indexedAlternationCursor {
	cursor := indexedAlternationCursor{
		symbolMatches:   idx.bySymbolNamed[alternationSymbolNamedKey(c.nodeSymbol, c.nodeNamed)],
		wildcardMatches: idx.wildcard,
	}
	if !c.nodeNamed && len(idx.byText) > 0 {
		if !c.nodeTypeLoaded {
			c.nodeType = c.node.Type(c.lang)
			c.nodeTypeLoaded = true
		}
		cursor.textMatches = idx.byText[c.nodeType]
	}
	return cursor
}

type indexedAlternativeSource uint8

const (
	indexedAlternativeNone indexedAlternativeSource = iota
	indexedAlternativeSymbol
	indexedAlternativeText
	indexedAlternativeWildcard
)

type indexedAlternationCursor struct {
	symbolMatches   []int
	textMatches     []int
	wildcardMatches []int
	iSym            int
	iText           int
	iWild           int
}

func (c *indexedAlternationCursor) hasAny() bool {
	return len(c.symbolMatches) > 0 || len(c.textMatches) > 0 || len(c.wildcardMatches) > 0
}

func (c *indexedAlternationCursor) next() (int, indexedAlternativeSource, bool) {
	nextAlt := -1
	source := indexedAlternativeNone
	if c.iSym < len(c.symbolMatches) {
		nextAlt = c.symbolMatches[c.iSym]
		source = indexedAlternativeSymbol
	}
	if c.iText < len(c.textMatches) && (nextAlt < 0 || c.textMatches[c.iText] < nextAlt) {
		nextAlt = c.textMatches[c.iText]
		source = indexedAlternativeText
	}
	if c.iWild < len(c.wildcardMatches) && (nextAlt < 0 || c.wildcardMatches[c.iWild] < nextAlt) {
		nextAlt = c.wildcardMatches[c.iWild]
		source = indexedAlternativeWildcard
	}
	return nextAlt, source, source != indexedAlternativeNone
}

func (c *indexedAlternationCursor) advance(source indexedAlternativeSource) {
	switch source {
	case indexedAlternativeSymbol:
		c.iSym++
	case indexedAlternativeText:
		c.iText++
	case indexedAlternativeWildcard:
		c.iWild++
	}
}

func (q *Query) alternativeFieldMatches(alt *alternativeSymbol, node *Node, parent *Node, childIdx int, lang *Language) bool {
	if alt == nil || alt.field == 0 {
		return true
	}
	if parent == nil || childIdx < 0 {
		// Root-level field-constrained patterns (for example `field: (node)`) are
		// matched against each candidate node and must resolve the real parent
		// relationship at match time.
		if linkedParent, linkedIdx, ok := nodeParentLink(node); linkedParent != nil && ok && linkedIdx >= 0 {
			parent = linkedParent
			childIdx = linkedIdx
		} else {
			parent = node.Parent()
			if parent == nil {
				return false
			}
			childIdx = -1
			childCount := nodeChildCountNoMaterialize(parent)
			for i := 0; i < childCount; i++ {
				child := nodeChildAtForReason(parent, i, materializeForQuery)
				if child == node {
					childIdx = i
					break
				}
			}
			if childIdx < 0 {
				return false
			}
		}
	}
	if int(alt.field) <= 0 || int(alt.field) >= len(lang.FieldNames) {
		return false
	}
	fieldName := lang.FieldNames[alt.field]
	if fieldName == "" {
		return false
	}
	return parent.FieldNameForChild(childIdx, lang) == fieldName
}

func (q *Query) matchAlternationBranch(
	step *QueryStep,
	alt *alternativeSymbol,
	node *Node,
	lang *Language,
	source []byte,
	predicates []QueryPredicate,
	captures *[]QueryCapture,
	hasStepCaptures bool,
) bool {
	if len(alt.steps) > 0 {
		// Fast path: no alternation-level captures and no branch predicates.
		// The predicate-aware rollback path protects captures from failed branches.
		if !hasStepCaptures && len(alt.predicates) == 0 {
			return q.matchStepWithRollbackPredicates(alt.steps, 0, node, lang, source, predicates, captures)
		}

		checkpoint := len(*captures)
		if hasStepCaptures {
			// Captures on the alternation itself apply regardless of chosen branch.
			q.appendCaptureIDs(step.captureIDs, node, captures)
			if !q.predicatesStillViable(predicates, *captures, source) {
				*captures = (*captures)[:checkpoint]
				return false
			}
		}
		if !q.matchStepWithRollbackPredicates(alt.steps, 0, node, lang, source, predicates, captures) {
			*captures = (*captures)[:checkpoint]
			return false
		}
		if len(alt.predicates) > 0 && !q.matchesPredicates(alt.predicates, *captures, lang, source) {
			*captures = (*captures)[:checkpoint]
			return false
		}
		return true
	}

	// Simple alternation branch captures (no nested structure).
	if !hasStepCaptures && len(alt.captureIDs) == 0 {
		return true
	}
	checkpoint := len(*captures)
	if hasStepCaptures {
		q.appendCaptureIDs(step.captureIDs, node, captures)
		if !q.predicatesStillViable(predicates, *captures, source) {
			*captures = (*captures)[:checkpoint]
			return false
		}
	}
	q.appendCaptureIDs(alt.captureIDs, node, captures)
	if !q.predicatesStillViable(predicates, *captures, source) {
		*captures = (*captures)[:checkpoint]
		return false
	}
	return true
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
