package gotreesitter

import (
	"cmp"
	"slices"
)

func cloneQueryCapturesWithReader[C any](captures []C) []C {
	if len(captures) == 0 {
		return nil
	}
	return slices.Clone(captures)
}

// appendCaptureIDsWithReader appends every capture in ids, including ones a
// caller has disabled with DisableCapture. Predicates must see the full
// capture set, so disabled names are dropped only from a finished match's
// output; see filterDisabledCapturesWithReader.
func appendCaptureIDsWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, ids []int, node N, captures *[]C, reader R) {
	if len(ids) == 0 {
		return
	}
	start := len(*captures)
	*captures = slices.Grow(*captures, len(ids))
	expanded := (*captures)[:start+len(ids)]
	for i, captureID := range ids {
		expanded[start+i] = reader.NewCapture(q.captures[captureID], node)
	}
	*captures = expanded
}

// filterDisabledCapturesWithReader drops captures whose name DisableCapture
// removed from a finished match's captures, after predicates and directives
// have already evaluated the full set. See (*Query).filterDisabledCaptures.
func filterDisabledCapturesWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, captures []C, reader R) []C {
	if len(q.disabledCaptureName) == 0 || len(captures) == 0 {
		return captures
	}
	out := captures[:0]
	for _, capture := range captures {
		if !q.isCaptureDisabled(reader.CaptureName(capture)) {
			out = append(out, capture)
		}
	}
	return out
}

// matchPatternAllWithReader returns the matches of pat rooted at node, in the
// order the C query cursor returns them (see orderQueryCompletions), and
// scratch, which ordering allocates on first use. scratch may be nil.
func matchPatternAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pat *Pattern, node N, lang *Language, source []byte, budget *queryMatchBudget, scratch *queryOrderScratch[N, C], reader R) ([][]C, *queryOrderScratch[N, C]) {
	if len(pat.steps) == 0 || reader.IsNil(node) {
		return nil, scratch
	}
	if pat.steps[0].quantifier == queryQuantifierZeroOrMore {
		if matchesPredicatesWithReader(q, pat.predicates, []C(nil), lang, source, reader) {
			captures := applyDirectivesWithReader(q, pat.predicates, []C(nil), source, reader)
			return [][]C{cloneQueryCapturesWithReader(captures)}, scratch
		}
		return nil, scratch
	}
	if pat.steps[0].quantifier == queryQuantifierOneOrMore {
		return nil, scratch
	}

	var matches [][]C
	matchStepsAllWithReader(q, pat.steps, 0, node, *new(N), -1, lang, source, pat.predicates, nil, budget, reader, func(captures []C, finish queryFinish[N]) {
		if !matchesPredicatesWithReader(q, pat.predicates, captures, lang, source, reader) {
			return
		}
		captures = applyDirectivesWithReader(q, pat.predicates, captures, source, reader)
		captures = filterDisabledCapturesWithReader(q, captures, reader)
		if len(captures) == 0 && !queryStepsHaveCaptures(pat.steps) {
			// C merges matches without captures into one, so where they
			// finish does not matter.
			matches = append(matches, nil)
			return
		}
		matches = append(matches, queryCapturesWithFinish(captures, finish, reader))
	})
	return orderQueryCompletions(matches, scratch, reader)
}

func matchPatternPostorderAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pat *Pattern, node, parent N, childIdx int, lang *Language, source []byte, budget *queryMatchBudget, reader R) [][]C {
	if len(pat.steps) == 0 || reader.IsNil(node) {
		return nil
	}
	switch pat.steps[0].quantifier {
	case queryQuantifierZeroOrMore, queryQuantifierOneOrMore:
	default:
		return nil
	}
	parent, childIdx = queryPatternSiblingContextWithReader(node, parent, childIdx, reader)
	if len(matchPatternOnceAllWithReader(q, pat, node, parent, childIdx, lang, source, nil, budget, reader)) == 0 {
		return nil
	}
	if next, nextParent, nextChildIdx, ok := queryAdjacentSiblingWithReader(node, parent, childIdx, 1, reader); ok {
		if len(matchPatternOnceAllWithReader(q, pat, next, nextParent, nextChildIdx, lang, source, nil, budget, reader)) > 0 {
			return nil
		}
	}

	runStart, runParent, runChildIdx := node, parent, childIdx
	for {
		prev, prevParent, prevChildIdx, ok := queryAdjacentSiblingWithReader(runStart, runParent, runChildIdx, -1, reader)
		if !ok || len(matchPatternOnceAllWithReader(q, pat, prev, prevParent, prevChildIdx, lang, source, nil, budget, reader)) == 0 {
			break
		}
		runStart, runParent, runChildIdx = prev, prevParent, prevChildIdx
	}

	partials := [][]C{nil}
	current, currentParent, currentChildIdx := runStart, runParent, runChildIdx
	for !reader.IsNil(current) {
		var nextPartials [][]C
		for _, captures := range partials {
			nextPartials = append(nextPartials, matchPatternOnceAllWithReader(q, pat, current, currentParent, currentChildIdx, lang, source, captures, budget, reader)...)
		}
		if len(nextPartials) == 0 {
			break
		}
		partials = nextPartials
		if current == node {
			break
		}
		var ok bool
		current, currentParent, currentChildIdx, ok = queryAdjacentSiblingWithReader(current, currentParent, currentChildIdx, 1, reader)
		if !ok {
			break
		}
	}

	var matches [][]C
	for _, captures := range partials {
		if matchesPredicatesWithReader(q, pat.predicates, captures, lang, source, reader) {
			captures = applyDirectivesWithReader(q, pat.predicates, captures, source, reader)
			captures = filterDisabledCapturesWithReader(q, captures, reader)
			matches = append(matches, cloneQueryCapturesWithReader(captures))
		}
	}
	return matches
}

func queryPatternSiblingContextWithReader[N comparable, C any, R queryNodeReader[N, C]](node, parent N, childIdx int, reader R) (N, int) {
	if reader.IsNil(node) {
		var zero N
		return zero, -1
	}
	if !reader.IsNil(parent) && childIdx >= 0 {
		return parent, childIdx
	}
	parent, ok := reader.Parent(node)
	if !ok {
		var zero N
		return zero, -1
	}
	for i := 0; i < reader.ChildCount(parent); i++ {
		if child, ok := reader.Child(parent, i); ok && child == node {
			return parent, i
		}
	}
	return parent, -1
}

func queryAdjacentSiblingWithReader[N comparable, C any, R queryNodeReader[N, C]](node, parent N, childIdx, delta int, reader R) (N, N, int, bool) {
	if reader.IsNil(node) {
		var zero N
		return zero, zero, -1, false
	}
	if reader.IsNil(parent) || childIdx < 0 {
		parent, childIdx = queryPatternSiblingContextWithReader(node, parent, childIdx, reader)
	}
	nextIdx := childIdx + delta
	if reader.IsNil(parent) || nextIdx < 0 || nextIdx >= reader.ChildCount(parent) {
		var zero N
		return zero, parent, -1, false
	}
	next, ok := reader.Child(parent, nextIdx)
	return next, parent, nextIdx, ok
}

func matchPatternOnceAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pat *Pattern, node, parent N, childIdx int, lang *Language, source []byte, captures []C, budget *queryMatchBudget, reader R) [][]C {
	var matches [][]C
	matchStepsAllWithReader(q, pat.steps, 0, node, parent, childIdx, lang, source, pat.predicates, captures, budget, reader, func(next []C, _ queryFinish[N]) {
		matches = append(matches, cloneQueryCapturesWithReader(next))
	})
	return matches
}

func matchStepsAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, steps []QueryStep, stepIdx int, node, parent N, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func(captures []C, finish queryFinish[N])) {
	if stepIdx >= len(steps) || reader.IsNil(node) {
		return
	}
	step := &steps[stepIdx]
	if len(step.alternatives) > 0 {
		matchAlternationStepAllWithReader(q, step, node, parent, childIdx, lang, source, predicates, captures, budget, reader, func(next []C, finish queryFinish[N]) {
			matchStepChildrenAllWithReader(q, steps, stepIdx, node, lang, source, predicates, next, finish, budget, reader, emit)
		})
		return
	}
	if !nodeMatchesStepWithReader(q, step, node, lang, reader) {
		return
	}
	next := captures
	if len(step.captureIDs) > 0 {
		next = cloneQueryCapturesWithReader(captures)
		appendCaptureIDsWithReader(q, step.captureIDs, node, &next, reader)
	}
	if predicatesStillViableWithReader(q, predicates, next, source, reader) {
		matchStepChildrenAllWithReader(q, steps, stepIdx, node, lang, source, predicates, next, queryFinish[N]{node: node}, budget, reader, emit)
	}
}

// matchStepChildrenAllWithReader matches the child steps of steps[stepIdx]
// under node. finish is where the C cursor would finish the match so far;
// emit receives it for each completion.
func matchStepChildrenAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, steps []QueryStep, stepIdx int, node N, lang *Language, source []byte, predicates []QueryPredicate, captures []C, finish queryFinish[N], budget *queryMatchBudget, reader R, emit func([]C, queryFinish[N])) {
	step := &steps[stepIdx]
	childStart := stepIdx + 1
	if childStart >= len(steps) || steps[childStart].depth <= step.depth {
		emit(captures, finish)
		return
	}
	childDepth := step.depth + 1
	var inline [16]queryChildStepInfo
	childSteps := inline[:0]
	for i := childStart; i < len(steps) && steps[i].depth > step.depth; i++ {
		if steps[i].depth == childDepth {
			childSteps = append(childSteps, queryChildStepInfo{stepIdx: i, field: steps[i].field})
		}
	}
	matchChildStepsAllWithReader(q, node, steps, childSteps, lang, source, predicates, captures, finish, budget, reader, emit)
}

func matchAlternationStepAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, node, parent N, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func([]C, queryFinish[N])) {
	hasStepCaptures := len(step.captureIDs) > 0
	nodeNamed := reader.IsNamed(node)
	nodeSymbol := lang.PublicSymbolForNamedness(reader.Symbol(node), nodeNamed)
	var nodeType string
	nodeTypeLoaded := false
	// Arms emit in arm order. matchPatternAllWithReader puts the finished
	// matches in the C cursor's order once the steps after the alternation
	// have run.
	for i := range step.alternatives {
		alt := &step.alternatives[i]
		if !alternativeMatchesNodeWithReader(*alt, node, lang, nodeSymbol, nodeNamed, &nodeType, &nodeTypeLoaded, reader) ||
			!alternativeFieldMatchesWithReader(alt, node, parent, childIdx, lang, reader) {
			continue
		}
		if len(alt.steps) > 0 {
			// The arm's completions are buffered here, so emit is only
			// called directly. Passing emit into the arm's recursion would
			// make it escape and put the caller's closure on the heap for
			// every alternation step, structured or not.
			var arm []queryCompletion[N, C]
			matchAlternationStructuredBranchWithReader(q, step, alt, node, lang, source, predicates, captures, budget, hasStepCaptures, reader, func(next []C, finish queryFinish[N]) {
				arm = append(arm, queryCompletion[N, C]{captures: next, finish: finish})
			})
			for _, c := range arm {
				emit(c.captures, c.finish)
			}
			continue
		}
		next := cloneQueryCapturesWithReader(captures)
		if matchAlternationSimpleBranchWithReader(q, step, alt, node, predicates, source, &next, hasStepCaptures, reader) {
			emit(next, queryFinish[N]{node: node})
		}
	}
}

// queryCompletion is one finished match of a pattern at one root node: its
// captures and where the C query cursor finishes it.
type queryCompletion[N comparable, C any] struct {
	captures []C
	finish   queryFinish[N]
}

// queryFinishKind says where the C query cursor finishes a match relative to
// queryFinish.node.
type queryFinishKind uint8

const (
	// queryFinishAtNode: on node, where the match's last step matched.
	queryFinishAtNode queryFinishKind = iota
	// queryFinishAtChild: on node's child number index, where the state that
	// waits on a quantified step is dropped.
	queryFinishAtChild
	// queryFinishAfterNode: when the cursor leaves node.
	queryFinishAfterNode
)

// queryFinish records where the C query cursor finishes a match.
type queryFinish[N comparable] struct {
	node  N
	index int32
	kind  queryFinishKind
}

// queryLevelHold tracks, within one child list, where the C cursor would
// release a finished match that it holds for a quantified step.
//
// After a `?`, `*` or `+` step, C keeps a state that waits for one more match
// of the step, with the captures the match had when it passed the step. If
// the match adds no capture after that step, the finished match and the
// waiting state have equal captures, so C flags the finished match
// (has_in_progress_alternatives) and finishes it only once the waiting state
// is dropped (ts_query_cursor__advance in lib/src/query.c). A capture added
// later lets C finish the match at once. queryQuantifierDrop gives the drop
// point; the hold keeps the latest one since the last capture.
type queryLevelHold struct {
	captures int             // capture count when the hold was last set
	index    int32           // child index of the drop point
	kind     queryFinishKind // queryFinishAtNode: no hold
}

// advance returns the hold after step matched count children and the match
// had captures captures.
func (h queryLevelHold) advance(step *QueryStep, count int, span childStepNamedSpan, prev childStepPrevMatch, namedPositions []int, captures int) queryLevelHold {
	if h.kind != queryFinishAtNode && captures > h.captures {
		h = queryLevelHold{}
	}
	index, after, ok := queryQuantifierDrop(step, count, span, prev, namedPositions)
	if !ok {
		return h
	}
	switch {
	case h.kind == queryFinishAfterNode:
	case after:
		h.kind = queryFinishAfterNode
	case h.kind == queryFinishAtNode || int32(index) > h.index:
		h.kind, h.index = queryFinishAtChild, int32(index)
	}
	h.captures = captures
	return h
}

// queryHeldFinish returns where the C cursor finishes a match that reached
// the end of the child list under parent: on finish, or later at the held
// drop point. lastIdx is the child index of the list's last matched child.
func queryHeldFinish[N comparable](h queryLevelHold, parent N, finish queryFinish[N], lastIdx, captures int) queryFinish[N] {
	switch {
	case h.kind == queryFinishAtNode || captures != h.captures:
		return finish
	case h.kind == queryFinishAfterNode:
		return queryFinish[N]{node: parent, kind: queryFinishAfterNode}
	case int(h.index) > lastIdx:
		return queryFinish[N]{node: parent, index: h.index, kind: queryFinishAtChild}
	}
	return finish
}

// queryQuantifierDrop returns where the C cursor drops the state that waits
// for one more match of a quantified step, as a child index under the step's
// parent, or after when the state lasts until the cursor leaves the parent.
// It reports false when no waiting state remains.
//
//   - After a `*` or `+` run, on the next sibling, because the waiting state
//     seeks an immediate match.
//   - After a skipped `?` or `*` with a leading anchor, on the next named
//     sibling.
//   - After a skipped `?` or `*`, on the parent's last child.
//   - Otherwise when the cursor leaves the parent.
//
// A `?` that matched leaves no waiting state.
func queryQuantifierDrop(step *QueryStep, count int, span childStepNamedSpan, prev childStepPrevMatch, namedPositions []int) (index int, after, ok bool) {
	if step.quantifier == queryQuantifierOne || (step.quantifier == queryQuantifierZeroOrOne && count > 0) {
		return 0, false, false
	}
	childCount := len(namedPositions)
	if count > 0 {
		if next := span.lastIdx + 1; next > 0 && next < childCount {
			return next, false, true
		}
		return 0, true, true
	}
	from := prev.lastIdx + 1
	if step.anchorBefore {
		for i := from; i < childCount; i++ {
			if prev.unnamedWildcard || namedPositions[i] >= 0 {
				return i, false, true
			}
		}
	}
	if last := childCount - 1; last >= from {
		return last, false, true
	}
	return 0, true, true
}

// queryFinishAfterChild turns a finish "when the cursor leaves child", where
// child is parent's child number childIdx, into the point where the C cursor
// next finishes held matches: on entering child's next sibling, or when it
// leaves parent if child is the last child.
func queryFinishAfterChild[N comparable](finish queryFinish[N], child, parent N, childIdx, childCount int) queryFinish[N] {
	if finish.kind != queryFinishAfterNode || finish.node != child {
		return finish
	}
	if childIdx+1 < childCount {
		return queryFinish[N]{node: parent, index: int32(childIdx + 1), kind: queryFinishAtChild}
	}
	return queryFinish[N]{node: parent, kind: queryFinishAfterNode}
}

// queryOrderScratch holds the buffers orderQueryCompletions uses to sort.
// orderQueryCompletions allocates it the first time a pattern's matches need
// reordering; callers keep it across pattern roots and runs.
type queryOrderScratch[N comparable, C any] struct {
	keys   []queryCompletionKey[N, C]
	byHash map[uint64]int
}

// Marker capture names. A match that does not finish on the node of its last
// capture carries one marker capture at the end, naming where it finishes,
// so collecting matches needs no side buffer. Capture names never contain
// NUL, so no query produces these names; orderQueryCompletions removes the
// marker before a match is returned.
const (
	queryFinishOnNodeCapture    = "\x00finish-on"
	queryFinishAfterNodeCapture = "\x00finish-after"
)

// queryCapturesWithFinish returns a copy of one finished match's captures,
// ending with a marker capture when the match finishes somewhere other than
// the node of its last capture.
func queryCapturesWithFinish[N comparable, C any, R queryNodeReader[N, C]](captures []C, finish queryFinish[N], reader R) []C {
	name := queryFinishOnNodeCapture
	node := finish.node
	switch finish.kind {
	case queryFinishAtNode:
		if len(captures) > 0 && reader.CaptureNode(captures[len(captures)-1]) == node {
			return cloneQueryCapturesWithReader(captures)
		}
	case queryFinishAtChild:
		if child, ok := reader.Child(node, int(finish.index)); ok {
			node = child
		} else {
			name = queryFinishAfterNodeCapture
		}
	default:
		name = queryFinishAfterNodeCapture
	}
	out := make([]C, len(captures)+1)
	copy(out, captures)
	out[len(captures)] = reader.NewCapture(name, node)
	return out
}

// querySplitFinish returns a match's captures without its finish marker and
// where the match finishes. A match without captures or marker has no known
// finish: the zero queryFinish.
func querySplitFinish[N comparable, C any, R queryNodeReader[N, C]](captures []C, reader R) ([]C, queryFinish[N]) {
	n := len(captures)
	if n == 0 {
		return captures, queryFinish[N]{}
	}
	last := captures[n-1]
	switch reader.CaptureName(last) {
	case queryFinishOnNodeCapture:
		return queryCapturesOrNil(captures[:n-1]), queryFinish[N]{node: reader.CaptureNode(last)}
	case queryFinishAfterNodeCapture:
		return queryCapturesOrNil(captures[:n-1]), queryFinish[N]{node: reader.CaptureNode(last), kind: queryFinishAfterNode}
	}
	return captures, queryFinish[N]{node: reader.CaptureNode(last)}
}

// queryCapturesOrNil returns nil for an empty capture list, as the matcher
// returns for a match without captures.
func queryCapturesOrNil[C any](captures []C) []C {
	if len(captures) == 0 {
		return nil
	}
	return captures
}

// queryStepsHaveCaptures reports whether any step, alternative or nested
// alternative step captures a node.
func queryStepsHaveCaptures(steps []QueryStep) bool {
	for i := range steps {
		step := &steps[i]
		if len(step.captureIDs) > 0 {
			return true
		}
		for j := range step.alternatives {
			alt := &step.alternatives[j]
			if len(alt.captureIDs) > 0 || queryStepsHaveCaptures(alt.steps) {
				return true
			}
		}
	}
	return false
}

// queryCompletionKey carries the sort keys of one completion.
type queryCompletionKey[N comparable, C any] struct {
	captures    []C
	finishNode  N
	first       N
	hash        uint64
	sameHash    int // index of the previous key with this hash, or -1
	finishStart uint32
	finishEnd   uint32
	firstStart  uint32
	firstEnd    uint32
	afterNode   bool
	hasCaptures bool
	mergeable   bool
}

// queryCompletionSmallSet is the size up to which equal-capture detection
// scans the hashes instead of using a map.
const queryCompletionSmallSet = 8

// querySameSpanWalkLimit bounds the walk through same-span descendants.
const querySameSpanWalkLimit = 64

// orderQueryCompletions puts the completions of one pattern at one root node
// in the order the pinned C query cursor returns them
// (ts_query_cursor__advance and ts_query_cursor_next_match in
// lib/src/query.c), reusing matches' storage, and removes the finish markers
// (see queryCapturesWithFinish). matches lists the completions in enumeration
// order. It returns scratch, allocating it on first use.
//
//   - C returns a match when it finishes, in the order of a pre-order walk.
//     A match finishes on the node where its last step matched, captured or
//     not, or later while a quantified step with no capture after it keeps
//     a waiting state (see queryLevelHold).
//   - Matches that finish on the same node keep C's state order
//     (ts_query_cursor__state_precedes): a match without captures first,
//     then the earlier first capture. With equal first-capture starts, the
//     state that captured later had no captures when the other captured, so
//     it sorted first: the first capture later in pre-order comes first.
//     Then the order the states split, which is the enumeration order.
//   - C keeps one state for equal captures: the longest-match pass drops the
//     later state, and the kept state finishes when the later one would. C
//     skips that check when the first capture starts at or after the last
//     capture ends, so equal zero-width captures stay two matches.
//
// Equal captures are found through a hash of each capture list, so the work
// is linear in len(matches) plus one sort. When the matches are already in
// order and no two can be equal, scratch is not touched.
func orderQueryCompletions[N comparable, C any, R queryNodeReader[N, C]](matches [][]C, scratch *queryOrderScratch[N, C], reader R) ([][]C, *queryOrderScratch[N, C]) {
	if len(matches) < 2 || queryCompletionsOrdered(matches, reader) {
		for i := range matches {
			matches[i], _ = querySplitFinish[N](matches[i], reader)
		}
		return matches, scratch
	}
	if scratch == nil {
		scratch = new(queryOrderScratch[N, C])
	}
	var byHash map[uint64]int
	if len(matches) > queryCompletionSmallSet {
		if scratch.byHash == nil {
			scratch.byHash = make(map[uint64]int, len(matches))
		} else {
			clear(scratch.byHash)
		}
		byHash = scratch.byHash
	}
	keys := scratch.keys[:0]
	for _, marked := range matches {
		captures, finish := querySplitFinish[N](marked, reader)
		k := newQueryCompletionKey(captures, finish, reader)
		if k.mergeable {
			k.hash = hashQueryCaptures(captures, reader)
			head := -1
			if byHash != nil {
				if i, ok := byHash[k.hash]; ok {
					head = i
				}
			} else {
				for i := len(keys) - 1; i >= 0; i-- {
					if keys[i].mergeable && keys[i].hash == k.hash {
						head = i
						break
					}
				}
			}
			if j := equalQueryCompletion(keys, head, captures, reader); j >= 0 {
				if compareQueryFinishKeys(k, keys[j], reader) > 0 {
					keys[j].finishNode = k.finishNode
					keys[j].finishStart = k.finishStart
					keys[j].finishEnd = k.finishEnd
					keys[j].afterNode = k.afterNode
				}
				continue
			}
			k.sameHash = head
			if byHash != nil {
				byHash[k.hash] = len(keys)
			}
		}
		keys = append(keys, k)
	}
	slices.SortStableFunc(keys, func(a, b queryCompletionKey[N, C]) int {
		return compareQueryCompletionKeys(a, b, reader)
	})
	matches = matches[:len(keys)]
	for i := range keys {
		matches[i] = keys[i].captures
	}
	clear(keys)
	scratch.keys = keys[:0]
	return matches, scratch
}

// queryCompletionsOrdered reports whether matches are already in C's order
// with no two that could be one match. Equal capture lists share their last
// capture, so strictly increasing last captures rule that out.
func queryCompletionsOrdered[N comparable, C any, R queryNodeReader[N, C]](matches [][]C, reader R) bool {
	captures, finish := querySplitFinish[N](matches[0], reader)
	prev := newQueryCompletionKey(captures, finish, reader)
	if !prev.hasCaptures {
		return false
	}
	prevLast := reader.CaptureNode(prev.captures[len(prev.captures)-1])
	for i := 1; i < len(matches); i++ {
		captures, finish := querySplitFinish[N](matches[i], reader)
		cur := newQueryCompletionKey(captures, finish, reader)
		if !cur.hasCaptures || compareQueryCompletionKeys(prev, cur, reader) > 0 {
			return false
		}
		curLast := reader.CaptureNode(cur.captures[len(cur.captures)-1])
		if compareQueryNodes(prevLast, curLast, reader) >= 0 {
			return false
		}
		prev, prevLast = cur, curLast
	}
	return true
}

func newQueryCompletionKey[N comparable, C any, R queryNodeReader[N, C]](captures []C, finish queryFinish[N], reader R) queryCompletionKey[N, C] {
	k := queryCompletionKey[N, C]{captures: captures, sameHash: -1, mergeable: true}
	var zero N
	if node := finish.node; node != zero {
		k.finishNode = node
		if finish.kind == queryFinishAfterNode {
			k.afterNode = true
			end := reader.EndByte(node)
			k.finishStart, k.finishEnd = end, end
		} else {
			k.finishStart, k.finishEnd = reader.StartByte(node), reader.EndByte(node)
		}
	}
	if n := len(captures); n > 0 {
		k.hasCaptures = true
		k.first = reader.CaptureNode(captures[0])
		k.firstStart, k.firstEnd = reader.StartByte(k.first), reader.EndByte(k.first)
		k.mergeable = k.firstStart < reader.EndByte(reader.CaptureNode(captures[n-1]))
	}
	return k
}

// equalQueryCompletion follows the chain of keys that share a hash, starting
// at head, and returns the index of one whose captures equal captures, or
// -1.
func equalQueryCompletion[N comparable, C any, R queryNodeReader[N, C]](keys []queryCompletionKey[N, C], head int, captures []C, reader R) int {
	for k := head; k >= 0; k = keys[k].sameHash {
		if equalQueryCaptures(keys[k].captures, captures, reader) {
			return k
		}
	}
	return -1
}

func compareQueryCompletionKeys[N comparable, C any, R queryNodeReader[N, C]](a, b queryCompletionKey[N, C], reader R) int {
	if c := compareQueryFinishKeys(a, b, reader); c != 0 {
		return c
	}
	if a.hasCaptures != b.hasCaptures {
		if !a.hasCaptures {
			return -1
		}
		return 1
	}
	if !a.hasCaptures || a.first == b.first {
		return 0
	}
	if a.firstStart != b.firstStart {
		return cmp.Compare(a.firstStart, b.firstStart)
	}
	// Equal starts: the first capture later in pre-order comes first.
	if a.firstEnd != b.firstEnd {
		return cmp.Compare(a.firstEnd, b.firstEnd)
	}
	if querySameSpanDescendant(a.first, b.first, reader) {
		return 1
	}
	if querySameSpanDescendant(b.first, a.first, reader) {
		return -1
	}
	return 0
}

// compareQueryFinishKeys orders two finish points as the C cursor meets
// them: a node before the nodes inside it, and leaving a node after every
// node that ends where it ends.
func compareQueryFinishKeys[N comparable, C any, R queryNodeReader[N, C]](a, b queryCompletionKey[N, C], reader R) int {
	if a.finishStart != b.finishStart {
		return cmp.Compare(a.finishStart, b.finishStart)
	}
	if a.finishEnd != b.finishEnd {
		return cmp.Compare(b.finishEnd, a.finishEnd)
	}
	if a.afterNode != b.afterNode {
		if b.afterNode {
			return -1
		}
		return 1
	}
	if a.afterNode || a.finishNode == b.finishNode {
		return 0
	}
	if querySameSpanDescendant(a.finishNode, b.finishNode, reader) {
		return -1
	}
	if querySameSpanDescendant(b.finishNode, a.finishNode, reader) {
		return 1
	}
	return 0
}

// compareQueryNodes orders two nodes as a pre-order walk meets them: the
// earlier start first, and an enclosing node before a node inside it, also
// when both have one span (ts_query_cursor__compare_nodes compares spans
// only).
func compareQueryNodes[N comparable, C any, R queryNodeReader[N, C]](a, b N, reader R) int {
	if a == b {
		return 0
	}
	if as, bs := reader.StartByte(a), reader.StartByte(b); as != bs {
		return cmp.Compare(as, bs)
	}
	if ae, be := reader.EndByte(a), reader.EndByte(b); ae != be {
		return cmp.Compare(be, ae)
	}
	if querySameSpanDescendant(a, b, reader) {
		return -1
	}
	if querySameSpanDescendant(b, a, reader) {
		return 1
	}
	return 0
}

// querySameSpanDescendant reports whether node lies inside ancestor with the
// same span, so a pre-order walk enters ancestor first. It walks down
// through children with that span and does not build parent links.
func querySameSpanDescendant[N comparable, C any, R queryNodeReader[N, C]](ancestor, node N, reader R) bool {
	start, end := reader.StartByte(ancestor), reader.EndByte(ancestor)
	if reader.StartByte(node) != start || reader.EndByte(node) != end {
		return false
	}
	for depth := 0; depth < querySameSpanWalkLimit; depth++ {
		var next N
		found := false
		for i, n := 0, reader.ChildCount(ancestor); i < n; i++ {
			child, ok := reader.Child(ancestor, i)
			if !ok {
				continue
			}
			if child == node {
				return true
			}
			if !found && reader.StartByte(child) == start && reader.EndByte(child) == end {
				next, found = child, true
			}
		}
		if !found {
			return false
		}
		ancestor = next
	}
	return false
}

// equalQueryCaptures reports whether two capture lists name the same nodes
// in the same order, as C compares capture lists (capture index and node).
func equalQueryCaptures[N comparable, C any, R queryNodeReader[N, C]](a, b []C, reader R) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if reader.CaptureNode(a[i]) != reader.CaptureNode(b[i]) || reader.CaptureName(a[i]) != reader.CaptureName(b[i]) {
			return false
		}
	}
	return true
}

// hashQueryCaptures hashes the capture names and the span and symbol of each
// captured node (FNV-1a). Equal capture lists hash equal; equalQueryCaptures
// settles a collision.
func hashQueryCaptures[N comparable, C any, R queryNodeReader[N, C]](captures []C, reader R) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for _, c := range captures {
		name := reader.CaptureName(c)
		for i := 0; i < len(name); i++ {
			h = (h ^ uint64(name[i])) * prime
		}
		node := reader.CaptureNode(c)
		h = (h ^ uint64(reader.StartByte(node))) * prime
		h = (h ^ uint64(reader.EndByte(node))) * prime
		h = (h ^ uint64(reader.Symbol(node))) * prime
	}
	return h
}

// matchAlternationStructuredBranchWithReader delivers every completion of a
// structured arm to emit, each carrying the captures made before the
// alternation and where the C cursor would finish it. The arm shares
// the caller's work budget.
func matchAlternationStructuredBranchWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, alt *alternativeSymbol, node N, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, hasStepCaptures bool, reader R, emit func([]C, queryFinish[N])) {
	base := cloneQueryCapturesWithReader(captures)
	if hasStepCaptures {
		appendCaptureIDsWithReader(q, step.captureIDs, node, &base, reader)
		if !predicatesStillViableWithReader(q, predicates, base, source, reader) {
			return
		}
	}
	matchStepsAllWithReader(q, alt.steps, 0, node, *new(N), -1, lang, source, predicates, base, budget, reader, func(next []C, finish queryFinish[N]) {
		if len(alt.predicates) == 0 || matchesPredicatesWithReader(q, alt.predicates, next, lang, source, reader) {
			emit(cloneQueryCapturesWithReader(next), finish)
		}
	})
}

func matchAlternationSimpleBranchWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, alt *alternativeSymbol, node N, predicates []QueryPredicate, source []byte, captures *[]C, hasStepCaptures bool, reader R) bool {
	if !hasStepCaptures && len(alt.captureIDs) == 0 {
		return true
	}
	checkpoint := len(*captures)
	if hasStepCaptures {
		appendCaptureIDsWithReader(q, step.captureIDs, node, captures, reader)
		if !predicatesStillViableWithReader(q, predicates, *captures, source, reader) {
			*captures = (*captures)[:checkpoint]
			return false
		}
	}
	appendCaptureIDsWithReader(q, alt.captureIDs, node, captures, reader)
	if !predicatesStillViableWithReader(q, predicates, *captures, source, reader) {
		*captures = (*captures)[:checkpoint]
		return false
	}
	return true
}

func matchChildStepsAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, steps []QueryStep, childSteps []queryChildStepInfo, lang *Language, source []byte, predicates []QueryPredicate, captures []C, finish queryFinish[N], budget *queryMatchBudget, reader R, emit func([]C, queryFinish[N])) {
	childCount := reader.ChildCount(parent)
	var inline [64]int
	var namedPositions []int
	if childCount <= len(inline) {
		namedPositions = inline[:childCount]
	} else {
		namedPositions = make([]int, childCount)
	}
	namedPosition := 0
	for i := 0; i < childCount; i++ {
		named, ok := reader.ChildNamed(parent, i)
		if ok && named {
			namedPositions[i] = namedPosition
			namedPosition++
		} else {
			namedPositions[i] = -1
		}
	}
	matchChildStepsRecursiveAllWithReader(q, parent, namedPositions, namedPosition-1, steps, childSteps, 0, 0, childStepPrevMatch{lastIdx: -1}, queryLevelHold{}, lang, source, predicates, captures, finish, budget, reader, emit)
}

func matchChildStepsRecursiveAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, namedPositions []int, parentLastNamedPos int, steps []QueryStep, childSteps []queryChildStepInfo, childPos, nextChildIdx int, prev childStepPrevMatch, hold queryLevelHold, lang *Language, source []byte, predicates []QueryPredicate, captures []C, finish queryFinish[N], budget *queryMatchBudget, reader R, emit func([]C, queryFinish[N])) {
	if childPos >= len(childSteps) {
		emit(captures, queryHeldFinish(hold, parent, finish, prev.lastIdx, len(captures)))
		return
	}
	cs := childSteps[childPos]
	step := &steps[cs.stepIdx]
	minCount, maxCount, ok := quantifierBounds(step.quantifier)
	if !ok {
		return
	}
	// Reject a run before enumerating its capture combinations when a required
	// following sibling cannot occur anywhere after it. This also covers an
	// anchored successor; the precise adjacency check still runs below.
	if childPos+1 < len(childSteps) {
		following := childSteps[childPos+1]
		followingStep := &steps[following.stepIdx]
		followingMin, _, valid := quantifierBounds(followingStep.quantifier)
		if valid && followingMin > 0 {
			var followingInline [32]int
			possible, ok := collectChildCandidateIndicesWithReader(q, parent, followingStep, following.field, nextChildIdx, lang, followingInline[:0], reader)
			if !ok || len(possible) == 0 {
				return
			}
		}
	}
	var inline [32]int
	candidates, ok := collectChildCandidateIndicesWithReader(q, parent, step, cs.field, nextChildIdx, lang, inline[:0], reader)
	if !ok {
		return
	}
	if maxCount < 0 || maxCount > len(candidates) {
		maxCount = len(candidates)
	}
	if minCount > len(candidates) {
		return
	}
	for count := maxCount; count >= minCount; count-- {
		emittedForCount := false
		emitForCount := emit
		if step.quantifier != queryQuantifierOne {
			emitForCount = func(next []C, nextFinish queryFinish[N]) { emittedForCount = true; emit(next, nextFinish) }
		}
		var combinations func(int, int, int, childStepNamedSpan, []C, queryFinish[N])
		combinations = func(candidatePos, chosen, nextIdx int, span childStepNamedSpan, current []C, currentFinish queryFinish[N]) {
			if !budget.charge() {
				return
			}
			if chosen == count {
				if count > 0 && !q.stepAnchorsSatisfied(step, namedPositions, span, prev, parentLastNamedPos) {
					return
				}
				if count > 0 && currentFinish.kind == queryFinishAfterNode {
					if last, ok := reader.Child(parent, span.lastIdx); ok {
						currentFinish = queryFinishAfterChild(currentFinish, last, parent, span.lastIdx, len(namedPositions))
					}
				}
				nextHold := hold.advance(step, count, span, prev, namedPositions, len(current))
				matchChildStepsRecursiveAllWithReader(q, parent, namedPositions, parentLastNamedPos, steps, childSteps, childPos+1, nextIdx, advancePrevMatch(prev, step, namedPositions, span), nextHold, lang, source, predicates, current, currentFinish, budget, reader, emitForCount)
				return
			}
			remaining := count - chosen
			limit := len(candidates) - remaining
			for i := candidatePos; i <= limit; i++ {
				childIdx := candidates[i]
				child, ok := reader.Child(parent, childIdx)
				if !ok {
					continue
				}
				nextIdxForChoice := maxInt(nextIdx, childIdx+1)
				spanForChoice := span.withChild(childIdx, namedPositions[childIdx])
				matchStepsAllWithReader(q, steps, cs.stepIdx, child, parent, childIdx, lang, source, predicates, current, budget, reader, func(next []C, nextFinish queryFinish[N]) {
					combinations(i+1, chosen+1, nextIdxForChoice, spanForChoice, next, nextFinish)
				})
			}
		}
		combinations(0, 0, nextChildIdx, emptyChildStepNamedSpan(), captures, finish)
		if budget.tripped() || (step.quantifier != queryQuantifierOne && emittedForCount) {
			return
		}
	}
}

func collectChildCandidateIndicesWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, step *QueryStep, field FieldID, nextChildIdx int, lang *Language, dst []int, reader R) ([]int, bool) {
	if field != 0 && (lang == nil || int(field) <= 0 || int(field) >= len(lang.FieldNames) || lang.FieldNames[field] == "") {
		return dst, false
	}
	contiguous, started := stepUsesContiguousRun(step), false
	for i := nextChildIdx; i < reader.ChildCount(parent); i++ {
		matched := reader.ChildCanMatch(q, step, parent, i, lang) && reader.ChildHasField(parent, i, field, lang)
		if matched {
			dst = append(dst, i)
			started = true
		} else if contiguous && started {
			break
		}
	}
	return dst, true
}

func nodeMatchesStepWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, node N, lang *Language, reader R) bool {
	if !nodeCanMatchStepShallowWithReader(q, step, node, lang, reader) {
		return false
	}
	// The legacy matcher only evaluates absent fields on scalar symbol steps.
	if len(step.alternatives) > 0 || step.textMatch != "" || step.symbol == 0 {
		return true
	}
	for _, field := range step.absentFields {
		if int(field) <= 0 || int(field) >= len(lang.FieldNames) || lang.FieldNames[field] == "" || reader.HasChildWithField(node, field, lang) {
			return false
		}
	}
	return true
}

// nodeCanMatchStepShallowWithReader is the shared child-prefilter contract.
// It deliberately omits absent-field traversal, matching the legacy
// stack-entry prefilter; exact absence is checked after candidate selection.
func nodeCanMatchStepShallowWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, node N, lang *Language, reader R) bool {
	if q == nil || step == nil || reader.IsNil(node) || lang == nil {
		return false
	}
	if len(step.alternatives) > 0 {
		named := reader.IsNamed(node)
		symbol := lang.PublicSymbolForNamedness(reader.Symbol(node), named)
		if idx := step.altIndex; idx != nil {
			if len(idx.wildcard) > 0 || len(idx.bySymbolNamed[alternationSymbolNamedKey(symbol, named)]) > 0 {
				return true
			}
			return !named && len(idx.byText[reader.Type(node, lang)]) > 0
		}
		var nodeType string
		loaded := false
		for _, alt := range step.alternatives {
			if alternativeMatchesNodeWithReader(alt, node, lang, symbol, named, &nodeType, &loaded, reader) {
				return true
			}
		}
		return false
	}
	if step.textMatch != "" {
		return !reader.IsNamed(node) && reader.Type(node, lang) == step.textMatch && (!step.isMissing || reader.IsMissing(node))
	}
	if step.symbol == 0 {
		if step.isMissing {
			return reader.IsMissing(node)
		}
		// A wildcard never matches an ERROR node (ts_query_cursor__advance).
		if reader.Symbol(node) == errorSymbol {
			return false
		}
		if !readerSupertypeSatisfied(reader, node, lang, step.supertype) {
			return false
		}
		return !step.isNamed || reader.IsNamed(node)
	}
	named := reader.IsNamed(node)
	if named != step.isNamed || lang.PublicSymbolForNamedness(reader.Symbol(node), named) != lang.PublicSymbolForNamedness(step.symbol, step.isNamed) || (step.isMissing && !reader.IsMissing(node)) {
		return false
	}
	return readerSupertypeSatisfied(reader, node, lang, step.supertype)
}

// readerSupertypeSatisfied checks a supertype constraint through the reader.
func readerSupertypeSatisfied[N comparable, C any, R queryNodeReader[N, C]](reader R, node N, lang *Language, supertype Symbol) bool {
	if supertype == 0 {
		return true
	}
	bit := lang.supertypeBit(supertype)
	return bit != 0 && reader.SupertypeMask(node)&bit != 0
}

func alternativeMatchesNodeWithReader[N comparable, C any, R queryNodeReader[N, C]](alt alternativeSymbol, node N, lang *Language, nodeSymbol Symbol, nodeNamed bool, nodeType *string, loaded *bool, reader R) bool {
	if alt.symbol == 0 && alt.textMatch == "" {
		if alt.isMissing {
			return reader.IsMissing(node)
		}
		return (!alt.isNamed || nodeNamed) && reader.Symbol(node) != errorSymbol && readerSupertypeSatisfied(reader, node, lang, alt.supertype)
	}
	if alt.textMatch != "" {
		if nodeNamed {
			return false
		}
		if !*loaded {
			*nodeType, *loaded = reader.Type(node, lang), true
		}
		return *nodeType == alt.textMatch && (!alt.isMissing || reader.IsMissing(node))
	}
	return nodeNamed == alt.isNamed && nodeSymbol == lang.PublicSymbolForNamedness(alt.symbol, alt.isNamed) && (!alt.isMissing || reader.IsMissing(node)) && readerSupertypeSatisfied(reader, node, lang, alt.supertype)
}

func alternativeFieldMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](alt *alternativeSymbol, node, parent N, childIdx int, lang *Language, reader R) bool {
	if alt == nil || alt.field == 0 {
		return true
	}
	if reader.IsNil(parent) || childIdx < 0 {
		parent, childIdx = queryPatternSiblingContextWithReader(node, parent, childIdx, reader)
	}
	return !reader.IsNil(parent) && childIdx >= 0 && reader.ChildHasField(parent, childIdx, alt.field, lang)
}
