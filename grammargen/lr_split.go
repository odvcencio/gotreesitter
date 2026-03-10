package grammargen

import "fmt"

// localLR1Rebuild splits nominated LALR states into canonical LR(1) states
// by rebuilding a bounded neighborhood around each split candidate.
//
// Algorithm:
//  1. For each candidate state S, find all predecessor states (states with
//     transitions into S) using ctx.transitions.
//  2. For each predecessor P, extract the kernel items that would form S
//     from P's perspective alone, then compute LR(1) closure to get the
//     exact action table for this split.
//  3. If the resulting states have different action tables (the conflict
//     disappears when lookaheads are separated), create new states and
//     rewrite transitions.
//  4. Cap the total new states at maxNewStates to prevent explosion.
//
// Returns the number of states that were successfully split.
func localLR1Rebuild(
	tables *LRTables,
	ng *NormalizedGrammar,
	ctx *lrContext,
	candidates []splitCandidate,
	maxNewStates int,
) (int, error) {
	if len(candidates) == 0 {
		return 0, nil
	}

	tokenCount := ng.TokenCount()
	totalSplit := 0

	for _, cand := range candidates {
		if totalSplit >= maxNewStates {
			break
		}

		stateIdx := cand.stateIdx
		if stateIdx >= len(ctx.itemSets) {
			continue
		}

		// Find all predecessor states with transitions into this state.
		trans := ctx.transitions
		type predInfo struct {
			predState int
			transSym  int
		}
		var preds []predInfo
		for srcState, syms := range trans {
			for sym, target := range syms {
				if target == stateIdx {
					preds = append(preds, predInfo{srcState, sym})
				}
			}
		}

		if len(preds) < 2 {
			continue
		}

		// Cap predecessors per candidate to avoid state explosion.
		// With N predecessors we create N-1 new states; for states with
		// hundreds of predecessors this would bloat the table pointlessly.
		const maxPredsPerCandidate = 10
		if len(preds) > maxPredsPerCandidate {
			preds = preds[:maxPredsPerCandidate]
		}

		// For each predecessor, extract the kernel items that would form
		// this state from that predecessor's perspective alone.
		type predPartition struct {
			pred    predInfo
			kernel  []coreEntry
			actions map[int][]lrAction // lookahead → actions
		}

		var partitions []predPartition

		for _, pred := range preds {
			if pred.predState >= len(ctx.itemSets) {
				continue
			}
			predItemSet := &ctx.itemSets[pred.predState]

			// Find items in predecessor where dot is before the transition symbol.
			var kernel []coreEntry
			for _, ce := range predItemSet.cores {
				prod := &ng.Productions[ce.prodIdx]
				if ce.dot < len(prod.RHS) && prod.RHS[ce.dot] == pred.transSym {
					// Advance dot past the transition symbol.
					kernel = append(kernel, coreEntry{
						prodIdx:    ce.prodIdx,
						dot:        ce.dot + 1,
						lookaheads: ce.lookaheads.clone(),
					})
				}
			}

			if len(kernel) == 0 {
				continue
			}

			// Close the kernel with full LR(1) closure.
			closedSet := ctx.closureToSet(kernel)

			// Build action table from the closed set.
			actions := make(map[int][]lrAction)
			for _, ce := range closedSet.cores {
				prod := &ng.Productions[ce.prodIdx]
				if ce.dot >= len(prod.RHS) {
					// Reduce item.
					if ce.prodIdx == ng.AugmentProdID {
						actions[0] = append(actions[0], lrAction{kind: lrAccept})
					} else {
						ce.lookaheads.forEach(func(la int) {
							actions[la] = append(actions[la], lrAction{
								kind:    lrReduce,
								prodIdx: ce.prodIdx,
								lhsSym:  prod.LHS,
								isExtra: prod.IsExtra,
							})
						})
					}
				} else {
					// Shift item — look up target from original transitions.
					nextSym := prod.RHS[ce.dot]
					if nextSym < tokenCount {
						if target, ok := ctx.transitions[stateIdx][nextSym]; ok {
							actions[nextSym] = append(actions[nextSym], lrAction{
								kind:    lrShift,
								state:   target,
								prec:    prod.Prec,
								assoc:   prod.Assoc,
								lhsSym:  prod.LHS,
								isExtra: prod.IsExtra,
							})
						}
					}
				}
			}

			partitions = append(partitions, predPartition{
				pred:    pred,
				kernel:  kernel,
				actions: actions,
			})
		}

		if len(partitions) < 2 {
			continue
		}

		requiredNewStates := len(partitions) - 1
		if totalSplit+requiredNewStates > maxNewStates {
			continue
		}

		// Check if splitting would resolve the conflict.
		// ALL partitions must have strictly fewer actions on the conflict symbol
		// than the original. If any partition still has the same (or more), splitting
		// just moves the conflict around without resolving it.
		conflictSym := cand.lookaheadSym
		origConflictCount := len(tables.ActionTable[stateIdx][conflictSym])
		if origConflictCount <= 1 {
			continue // no actual conflict to resolve
		}

		allHelp := true
		for _, p := range partitions {
			if len(p.actions[conflictSym]) >= origConflictCount {
				allHelp = false
				break
			}
		}

		if !allHelp {
			continue
		}

		// Snapshot original state for rollback.
		origActions := tables.ActionTable[stateIdx]
		origGoto := tables.GotoTable[stateIdx]
		origStateCount := tables.StateCount

		// Create new states. First partition keeps the original state index.
		// Subsequent partitions get new state indices.
		// Overwrite original state's actions with first partition's.
		tables.ActionTable[stateIdx] = make(map[int][]lrAction)
		for sym, acts := range partitions[0].actions {
			tables.ActionTable[stateIdx][sym] = acts
		}

		splitStates := []int{stateIdx} // track all states for rollback
		var rewrites []struct {
			predState int
			transSym  int
			oldTarget int
		}

		for i := 1; i < len(partitions); i++ {
			p := partitions[i]
			newStateIdx := tables.StateCount
			tables.StateCount++

			splitStates = append(splitStates, newStateIdx)

			// Set up action table for new state.
			tables.ActionTable[newStateIdx] = make(map[int][]lrAction)
			for sym, acts := range p.actions {
				tables.ActionTable[newStateIdx][sym] = acts
			}

			// Copy goto table from original state.
			tables.GotoTable[newStateIdx] = make(map[int]int)
			for sym, target := range origGoto {
				tables.GotoTable[newStateIdx][sym] = target
			}

			// Rewrite predecessor's transition to point to new state.
			if p.pred.transSym < tokenCount {
				predActs := tables.ActionTable[p.pred.predState][p.pred.transSym]
				for j := range predActs {
					if predActs[j].kind == lrShift && predActs[j].state == stateIdx {
						rewrites = append(rewrites, struct {
							predState int
							transSym  int
							oldTarget int
						}{p.pred.predState, p.pred.transSym, stateIdx})
						predActs[j].state = newStateIdx
						break
					}
				}
			} else {
				if tables.GotoTable[p.pred.predState] != nil &&
					tables.GotoTable[p.pred.predState][p.pred.transSym] == stateIdx {
					rewrites = append(rewrites, struct {
						predState int
						transSym  int
						oldTarget int
					}{p.pred.predState, p.pred.transSym, stateIdx})
					tables.GotoTable[p.pred.predState][p.pred.transSym] = newStateIdx
				}
			}
		}

		// Verify the split didn't introduce more unresolvable (GLR) conflicts
		// than we had before. We must try conflict resolution, not just count
		// raw multi-action entries, because some multi-action entries resolve
		// via precedence/associativity and don't become GLR entries.
		countGLR := func(actionTable map[int][]lrAction) int {
			glr := 0
			for _, acts := range actionTable {
				if len(acts) > 1 {
					resolved, err := resolveActionConflict(acts, ng)
					if err == nil && len(resolved) > 1 {
						glr++
					}
				}
			}
			return glr
		}

		newTotalConflicts := 0
		for _, si := range splitStates {
			newTotalConflicts += countGLR(tables.ActionTable[si])
		}
		origTotalConflicts := countGLR(origActions)

		if newTotalConflicts > origTotalConflicts {
			// Rollback: splitting made things worse.
			tables.ActionTable[stateIdx] = origActions
			tables.GotoTable[stateIdx] = origGoto
			for _, si := range splitStates[1:] {
				delete(tables.ActionTable, si)
				delete(tables.GotoTable, si)
			}
			for _, rw := range rewrites {
				if rw.transSym < tokenCount {
					predActs := tables.ActionTable[rw.predState][rw.transSym]
					for j := range predActs {
						if predActs[j].kind == lrShift && predActs[j].state != rw.oldTarget {
							predActs[j].state = rw.oldTarget
							break
						}
					}
				} else if tables.GotoTable[rw.predState] != nil {
					tables.GotoTable[rw.predState][rw.transSym] = rw.oldTarget
				}
			}
			tables.StateCount = origStateCount
			continue
		}

		totalSplit += len(splitStates) - 1
	}

	return totalSplit, nil
}

// splitReport describes the result of a local LR(1) rebuild pass.
type splitReport struct {
	CandidatesFound int
	StatesSplit     int
	NewStatesAdded  int
	ConflictsBefore int
	ConflictsAfter  int
	GLRBefore       int
	GLRAfter        int
	Error           error
}

func (r *splitReport) String() string {
	s := fmt.Sprintf("candidates=%d split=%d new_states=%d conflicts=%d→%d glr=%d→%d",
		r.CandidatesFound, r.StatesSplit, r.NewStatesAdded,
		r.ConflictsBefore, r.ConflictsAfter, r.GLRBefore, r.GLRAfter)
	if r.Error != nil {
		s += fmt.Sprintf(" error=%v", r.Error)
	}
	return s
}
