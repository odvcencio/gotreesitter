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
// order the C query cursor returns them (see orderQueryCompletions).
func matchPatternAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pat *Pattern, node N, lang *Language, source []byte, budget *queryMatchBudget, reader R) []queryCompletion[N, C] {
	if len(pat.steps) == 0 || reader.IsNil(node) {
		return nil
	}
	if pat.steps[0].quantifier == queryQuantifierZeroOrMore {
		if matchesPredicatesWithReader(q, pat.predicates, []C(nil), lang, source, reader) {
			captures := applyDirectivesWithReader(q, pat.predicates, []C(nil), source, reader)
			return []queryCompletion[N, C]{{captures: cloneQueryCapturesWithReader(captures), finish: node}}
		}
		return nil
	}
	if pat.steps[0].quantifier == queryQuantifierOneOrMore {
		return nil
	}

	var done []queryCompletion[N, C]
	matchStepsAllWithReader(q, pat.steps, 0, node, *new(N), -1, lang, source, pat.predicates, nil, budget, reader, func(captures []C, finish N) {
		if !matchesPredicatesWithReader(q, pat.predicates, captures, lang, source, reader) {
			return
		}
		captures = applyDirectivesWithReader(q, pat.predicates, captures, source, reader)
		captures = filterDisabledCapturesWithReader(q, captures, reader)
		done = append(done, queryCompletion[N, C]{captures: cloneQueryCapturesWithReader(captures), finish: finish})
	})
	return orderQueryCompletions(done, reader)
}

func matchPatternPostorderAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pat *Pattern, node, parent N, childIdx int, lang *Language, source []byte, budget *queryMatchBudget, reader R) []queryCompletion[N, C] {
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

	var matches []queryCompletion[N, C]
	for _, captures := range partials {
		if matchesPredicatesWithReader(q, pat.predicates, captures, lang, source, reader) {
			captures = applyDirectivesWithReader(q, pat.predicates, captures, source, reader)
			captures = filterDisabledCapturesWithReader(q, captures, reader)
			matches = append(matches, queryCompletion[N, C]{captures: cloneQueryCapturesWithReader(captures), finish: node})
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
	matchStepsAllWithReader(q, pat.steps, 0, node, parent, childIdx, lang, source, pat.predicates, captures, budget, reader, func(next []C, _ N) {
		matches = append(matches, cloneQueryCapturesWithReader(next))
	})
	return matches
}

func matchStepsAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, steps []QueryStep, stepIdx int, node, parent N, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func(captures []C, finish N)) {
	if stepIdx >= len(steps) || reader.IsNil(node) {
		return
	}
	step := &steps[stepIdx]
	if len(step.alternatives) > 0 {
		matchAlternationStepAllWithReader(q, step, node, parent, childIdx, lang, source, predicates, captures, budget, reader, func(next []C, finish N) {
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
		matchStepChildrenAllWithReader(q, steps, stepIdx, node, lang, source, predicates, next, node, budget, reader, emit)
	}
}

// matchStepChildrenAllWithReader matches the child steps of steps[stepIdx]
// under node. finish is the node where the latest step matched so far; emit
// receives the node where the latest step of each completion matched.
func matchStepChildrenAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, steps []QueryStep, stepIdx int, node N, lang *Language, source []byte, predicates []QueryPredicate, captures []C, finish N, budget *queryMatchBudget, reader R, emit func([]C, N)) {
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

func matchAlternationStepAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, node, parent N, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func([]C, N)) {
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
			matchAlternationStructuredBranchWithReader(q, step, alt, node, lang, source, predicates, captures, budget, hasStepCaptures, reader, func(next []C, finish N) {
				arm = append(arm, queryCompletion[N, C]{captures: next, finish: finish})
			})
			for _, c := range arm {
				emit(c.captures, c.finish)
			}
			continue
		}
		next := cloneQueryCapturesWithReader(captures)
		if matchAlternationSimpleBranchWithReader(q, step, alt, node, predicates, source, &next, hasStepCaptures, reader) {
			emit(next, node)
		}
	}
}

// queryCompletion is one finished match of a pattern at one root node: its
// captures and the node where its last step matched, captured or not.
type queryCompletion[N comparable, C any] struct {
	captures []C
	finish   N
}

// queryCompletionKey carries the sort keys of one completion.
type queryCompletionKey[N comparable, C any] struct {
	queryCompletion[N, C]
	hash        uint64
	sameHash    int // index of the previous key with this hash, or -1
	finishStart uint32
	finishEnd   uint32
	firstStart  uint32
	hasCaptures bool
}

// queryCompletionSmallSet is the size up to which equal-capture detection
// scans the hashes instead of building a map.
const queryCompletionSmallSet = 8

// orderQueryCompletions puts the completions of one pattern at one root node
// in the order the pinned C query cursor returns them
// (ts_query_cursor__advance and ts_query_cursor_next_match in
// lib/src/query.c), reusing done's storage. done lists the completions in
// enumeration order.
//
//   - C finishes a match on the node where its last step matched, captured or
//     not, and returns matches in the order a pre-order walk finishes them.
//   - Matches that finish on the same node keep C's state order
//     (ts_query_cursor__state_precedes): a match without captures first,
//     then the earlier first capture, then the order the states split, which
//     is the enumeration order.
//   - C keeps one state for equal captures: the longest-match pass drops the
//     later state, and the kept state finishes when the later one would.
//
// Equal captures are found through a hash of each capture list, so the work
// is linear in len(done) plus one sort.
func orderQueryCompletions[N comparable, C any, R queryNodeReader[N, C]](done []queryCompletion[N, C], reader R) []queryCompletion[N, C] {
	if len(done) < 2 {
		return done
	}
	keys := make([]queryCompletionKey[N, C], 0, len(done))
	var byHash map[uint64]int
	if len(done) > queryCompletionSmallSet {
		byHash = make(map[uint64]int, len(done))
	}
	for _, c := range done {
		h := hashQueryCaptures(c.captures, reader)
		head := -1
		if byHash != nil {
			if k, ok := byHash[h]; ok {
				head = k
			}
		} else {
			for k := len(keys) - 1; k >= 0; k-- {
				if keys[k].hash == h {
					head = k
					break
				}
			}
		}
		if k := equalQueryCompletion(keys, head, c.captures, reader); k >= 0 {
			if compareQueryNodes(c.finish, keys[k].finish, reader) > 0 {
				keys[k].finish = c.finish
			}
			continue
		}
		if byHash != nil {
			byHash[h] = len(keys)
		}
		keys = append(keys, queryCompletionKey[N, C]{queryCompletion: c, hash: h, sameHash: head})
	}
	for i := range keys {
		k := &keys[i]
		k.finishStart = reader.StartByte(k.finish)
		k.finishEnd = reader.EndByte(k.finish)
		if len(k.captures) > 0 {
			k.hasCaptures = true
			k.firstStart = reader.StartByte(reader.CaptureNode(k.captures[0]))
		}
	}
	slices.SortStableFunc(keys, compareQueryCompletionKeys[N, C])
	done = done[:len(keys)]
	for i := range keys {
		done[i] = keys[i].queryCompletion
	}
	return done
}

// equalQueryCompletion follows the chain of keys that share a hash,
// starting at head, and returns the index of one whose captures equal
// captures, or -1.
func equalQueryCompletion[N comparable, C any, R queryNodeReader[N, C]](keys []queryCompletionKey[N, C], head int, captures []C, reader R) int {
	for k := head; k >= 0; k = keys[k].sameHash {
		if equalQueryCaptures(keys[k].captures, captures, reader) {
			return k
		}
	}
	return -1
}

func compareQueryCompletionKeys[N comparable, C any](a, b queryCompletionKey[N, C]) int {
	if a.finishStart != b.finishStart {
		return cmp.Compare(a.finishStart, b.finishStart)
	}
	if a.finishEnd != b.finishEnd {
		return cmp.Compare(b.finishEnd, a.finishEnd)
	}
	if a.hasCaptures != b.hasCaptures {
		if !a.hasCaptures {
			return -1
		}
		return 1
	}
	return cmp.Compare(a.firstStart, b.firstStart)
}

// compareQueryNodes orders two nodes as a pre-order walk meets them: the
// earlier start first, and an enclosing node before a node inside it
// (ts_query_cursor__compare_nodes). Distinct nodes with one span compare
// equal.
func compareQueryNodes[N comparable, C any, R queryNodeReader[N, C]](a, b N, reader R) int {
	if a == b {
		return 0
	}
	if as, bs := reader.StartByte(a), reader.StartByte(b); as != bs {
		return cmp.Compare(as, bs)
	}
	return cmp.Compare(reader.EndByte(b), reader.EndByte(a))
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
// alternation and the node where the arm's last step matched. The arm shares
// the caller's work budget.
func matchAlternationStructuredBranchWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, alt *alternativeSymbol, node N, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, hasStepCaptures bool, reader R, emit func([]C, N)) {
	base := cloneQueryCapturesWithReader(captures)
	if hasStepCaptures {
		appendCaptureIDsWithReader(q, step.captureIDs, node, &base, reader)
		if !predicatesStillViableWithReader(q, predicates, base, source, reader) {
			return
		}
	}
	matchStepsAllWithReader(q, alt.steps, 0, node, *new(N), -1, lang, source, predicates, base, budget, reader, func(next []C, finish N) {
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

func matchChildStepsAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, steps []QueryStep, childSteps []queryChildStepInfo, lang *Language, source []byte, predicates []QueryPredicate, captures []C, finish N, budget *queryMatchBudget, reader R, emit func([]C, N)) {
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
	matchChildStepsRecursiveAllWithReader(q, parent, namedPositions, namedPosition-1, steps, childSteps, 0, 0, childStepPrevMatch{lastIdx: -1}, lang, source, predicates, captures, finish, budget, reader, emit)
}

func matchChildStepsRecursiveAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, namedPositions []int, parentLastNamedPos int, steps []QueryStep, childSteps []queryChildStepInfo, childPos, nextChildIdx int, prev childStepPrevMatch, lang *Language, source []byte, predicates []QueryPredicate, captures []C, finish N, budget *queryMatchBudget, reader R, emit func([]C, N)) {
	if childPos >= len(childSteps) {
		emit(captures, finish)
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
			emitForCount = func(next []C, nextFinish N) { emittedForCount = true; emit(next, nextFinish) }
		}
		var combinations func(int, int, int, childStepNamedSpan, []C, N)
		combinations = func(candidatePos, chosen, nextIdx int, span childStepNamedSpan, current []C, currentFinish N) {
			if !budget.charge() {
				return
			}
			if chosen == count {
				if count > 0 && !q.stepAnchorsSatisfied(step, namedPositions, span, prev, parentLastNamedPos) {
					return
				}
				matchChildStepsRecursiveAllWithReader(q, parent, namedPositions, parentLastNamedPos, steps, childSteps, childPos+1, nextIdx, advancePrevMatch(prev, step, namedPositions, span), lang, source, predicates, current, currentFinish, budget, reader, emitForCount)
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
				matchStepsAllWithReader(q, steps, cs.stepIdx, child, parent, childIdx, lang, source, predicates, current, budget, reader, func(next []C, nextFinish N) {
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
