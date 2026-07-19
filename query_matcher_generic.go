package gotreesitter

import "slices"

func cloneQueryCapturesWithReader[C any](captures []C) []C {
	if len(captures) == 0 {
		return nil
	}
	return slices.Clone(captures)
}

func appendCaptureIDsWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, ids []int, node N, captures *[]C, reader R) {
	if len(ids) == 0 {
		return
	}
	if len(q.disabledCaptureName) == 0 {
		start := len(*captures)
		*captures = slices.Grow(*captures, len(ids))
		expanded := (*captures)[:start+len(ids)]
		for i, captureID := range ids {
			expanded[start+i] = reader.NewCapture(q.captures[captureID], node)
		}
		*captures = expanded
		return
	}
	for _, captureID := range ids {
		name := q.captures[captureID]
		if !q.isCaptureDisabled(name) {
			*captures = append(*captures, reader.NewCapture(name, node))
		}
	}
}

func matchPatternAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pat *Pattern, node N, lang *Language, source []byte, budget *queryMatchBudget, reader R) [][]C {
	if len(pat.steps) == 0 || reader.IsNil(node) {
		return nil
	}
	if pat.steps[0].quantifier == queryQuantifierZeroOrMore {
		if matchesPredicatesWithReader(q, pat.predicates, []C(nil), lang, source, reader) {
			captures := applyDirectivesWithReader(q, pat.predicates, []C(nil), source, reader)
			return [][]C{cloneQueryCapturesWithReader(captures)}
		}
		return nil
	}
	if pat.steps[0].quantifier == queryQuantifierOneOrMore {
		return nil
	}

	var matches [][]C
	matchStepsAllWithReader(q, pat.steps, 0, node, *new(N), -1, lang, source, pat.predicates, nil, budget, reader, func(captures []C) {
		if !matchesPredicatesWithReader(q, pat.predicates, captures, lang, source, reader) {
			return
		}
		captures = applyDirectivesWithReader(q, pat.predicates, captures, source, reader)
		matches = append(matches, cloneQueryCapturesWithReader(captures))
	})
	return matches
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
	matchStepsAllWithReader(q, pat.steps, 0, node, parent, childIdx, lang, source, pat.predicates, captures, budget, reader, func(next []C) {
		matches = append(matches, cloneQueryCapturesWithReader(next))
	})
	return matches
}

func matchStepsAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, steps []QueryStep, stepIdx int, node, parent N, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func([]C)) {
	if stepIdx >= len(steps) || reader.IsNil(node) {
		return
	}
	step := &steps[stepIdx]
	if len(step.alternatives) > 0 {
		matchAlternationStepAllWithReader(q, step, node, parent, childIdx, lang, source, predicates, captures, reader, func(next []C) {
			matchStepChildrenAllWithReader(q, steps, stepIdx, node, lang, source, predicates, next, budget, reader, emit)
		})
		return
	}
	if !nodeMatchesStepWithReader(q, step, node, lang, reader) {
		return
	}
	next := cloneQueryCapturesWithReader(captures)
	appendCaptureIDsWithReader(q, step.captureIDs, node, &next, reader)
	if predicatesStillViableWithReader(q, predicates, next, source, reader) {
		matchStepChildrenAllWithReader(q, steps, stepIdx, node, lang, source, predicates, next, budget, reader, emit)
	}
}

func matchStepChildrenAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, steps []QueryStep, stepIdx int, node N, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func([]C)) {
	step := &steps[stepIdx]
	childStart := stepIdx + 1
	if childStart >= len(steps) || steps[childStart].depth <= step.depth {
		emit(captures)
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
	matchChildStepsAllWithReader(q, node, steps, childSteps, lang, source, predicates, captures, budget, reader, emit)
}

func matchAlternationStepAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, node, parent N, childIdx int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, reader R, emit func([]C)) {
	hasStepCaptures := len(step.captureIDs) > 0
	nodeNamed := reader.IsNamed(node)
	nodeSymbol := lang.PublicSymbolForNamedness(reader.Symbol(node), nodeNamed)
	var nodeType string
	nodeTypeLoaded := false
	for i := range step.alternatives {
		alt := &step.alternatives[i]
		if !alternativeMatchesNodeWithReader(*alt, node, lang, nodeSymbol, nodeNamed, &nodeType, &nodeTypeLoaded, reader) ||
			!alternativeFieldMatchesWithReader(alt, node, parent, childIdx, lang, reader) {
			continue
		}
		next := cloneQueryCapturesWithReader(captures)
		if matchAlternationBranchWithReader(q, step, alt, node, lang, source, predicates, &next, hasStepCaptures, reader) {
			emit(next)
		}
	}
}

func matchAlternationBranchWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, step *QueryStep, alt *alternativeSymbol, node N, lang *Language, source []byte, predicates []QueryPredicate, captures *[]C, hasStepCaptures bool, reader R) bool {
	if len(alt.steps) > 0 {
		checkpoint := len(*captures)
		if hasStepCaptures {
			appendCaptureIDsWithReader(q, step.captureIDs, node, captures, reader)
			if !predicatesStillViableWithReader(q, predicates, *captures, source, reader) {
				*captures = (*captures)[:checkpoint]
				return false
			}
		}
		matched := false
		matchStepsAllWithReader(q, alt.steps, 0, node, *new(N), -1, lang, source, predicates, *captures, newQueryMatchBudget(defaultQueryMatchWorkBudget), reader, func(next []C) {
			if matched {
				return
			}
			if len(alt.predicates) == 0 || matchesPredicatesWithReader(q, alt.predicates, next, lang, source, reader) {
				*captures = cloneQueryCapturesWithReader(next)
				matched = true
			}
		})
		if !matched {
			*captures = (*captures)[:checkpoint]
		}
		return matched
	}
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

func matchChildStepsAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, steps []QueryStep, childSteps []queryChildStepInfo, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func([]C)) {
	childCount := reader.ChildCount(parent)
	var inline [64]int
	namedPositions := inline[:0]
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
	matchChildStepsRecursiveAllWithReader(q, parent, namedPositions, namedPosition-1, steps, childSteps, 0, 0, false, -1, lang, source, predicates, captures, budget, reader, emit)
}

func matchChildStepsRecursiveAllWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, parent N, namedPositions []int, parentLastNamedPos int, steps []QueryStep, childSteps []queryChildStepInfo, childPos, nextChildIdx int, prevHasNamed bool, prevLastNamedPos int, lang *Language, source []byte, predicates []QueryPredicate, captures []C, budget *queryMatchBudget, reader R, emit func([]C)) {
	if childPos >= len(childSteps) {
		emit(captures)
		return
	}
	cs := childSteps[childPos]
	step := &steps[cs.stepIdx]
	minCount, maxCount, ok := quantifierBounds(step.quantifier)
	if !ok {
		return
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
			emitForCount = func(next []C) { emittedForCount = true; emit(next) }
		}
		var combinations func(int, int, int, bool, int, int, []C)
		combinations = func(candidatePos, chosen, nextIdx int, hasNamed bool, firstNamed, lastNamed int, current []C) {
			if !budget.charge() {
				return
			}
			if chosen == count {
				if count > 0 && !q.stepAnchorsSatisfied(step, childPos, hasNamed, firstNamed, lastNamed, prevHasNamed, prevLastNamedPos, parentLastNamedPos) {
					return
				}
				nextHasNamed := prevHasNamed || hasNamed
				nextLastNamed := prevLastNamedPos
				if hasNamed {
					nextLastNamed = lastNamed
				}
				matchChildStepsRecursiveAllWithReader(q, parent, namedPositions, parentLastNamedPos, steps, childSteps, childPos+1, nextIdx, nextHasNamed, nextLastNamed, lang, source, predicates, current, budget, reader, emitForCount)
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
				hasNamedForChoice, firstNamedForChoice, lastNamedForChoice := hasNamed, firstNamed, lastNamed
				if named := namedPositions[childIdx]; named >= 0 {
					if !hasNamedForChoice {
						hasNamedForChoice, firstNamedForChoice = true, named
					}
					lastNamedForChoice = named
				}
				matchStepsAllWithReader(q, steps, cs.stepIdx, child, parent, childIdx, lang, source, predicates, current, budget, reader, func(next []C) {
					combinations(i+1, chosen+1, nextIdxForChoice, hasNamedForChoice, firstNamedForChoice, lastNamedForChoice, next)
				})
			}
		}
		combinations(0, 0, nextChildIdx, false, -1, -1, captures)
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
		return !step.isNamed || reader.IsNamed(node)
	}
	named := reader.IsNamed(node)
	if named != step.isNamed || lang.PublicSymbolForNamedness(reader.Symbol(node), named) != lang.PublicSymbolForNamedness(step.symbol, step.isNamed) || (step.isMissing && !reader.IsMissing(node)) {
		return false
	}
	return true
}

func alternativeMatchesNodeWithReader[N comparable, C any, R queryNodeReader[N, C]](alt alternativeSymbol, node N, lang *Language, nodeSymbol Symbol, nodeNamed bool, nodeType *string, loaded *bool, reader R) bool {
	if alt.symbol == 0 && alt.textMatch == "" {
		if alt.isMissing {
			return reader.IsMissing(node)
		}
		return !alt.isNamed || nodeNamed
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
	return nodeNamed == alt.isNamed && nodeSymbol == lang.PublicSymbolForNamedness(alt.symbol, alt.isNamed) && (!alt.isMissing || reader.IsMissing(node))
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
