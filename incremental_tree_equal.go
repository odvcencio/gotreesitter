package gotreesitter

import "time"

// incrementalWholeDocumentError identifies recovery shapes that need a fresh
// result check, including a whole-document ERROR child with stale flags.
func incrementalWholeDocumentError(tree *Tree, parser *Parser) bool {
	if tree == nil || parser == nil || !parser.hasRootSymbol {
		return false
	}
	root := tree.RootNode()
	if root == nil || !root.HasError() {
		return false
	}
	if root.IsError() && root.ChildCount() == 1 {
		child := root.Child(0)
		return child != nil && child.Symbol() == parser.rootSymbol &&
			child.StartByte() == root.StartByte() && child.EndByte() == root.EndByte()
	}
	if root.Symbol() != parser.rootSymbol {
		return false
	}
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child != nil && child.IsError() &&
			child.StartByte() == root.StartByte() && child.EndByte() == root.EndByte() {
			return true
		}
	}
	return false
}

// newIncrementalFreshVerifier keeps a hidden parse from changing the caller's
// memo counters, recovery state, and other parser diagnostics.
func (p *Parser) newIncrementalFreshVerifier() *Parser {
	verifier := NewParser(p.language)
	verifier.ensureParserColdState().admissionCountersSuppressed = true
	// Authenticate against the caller's fresh API. Suppression belongs to the
	// reuse attempt; carrying it into the verifier can select a different
	// recovery tree or public span from a fresh candidate parse (D8).
	verifier.admissionCandidateRoute = p.admissionCandidateRoute
	// Observers keep the caller's fresh parse on the production route. Keep
	// that routing constraint without emitting hidden verification events.
	if p.hasActiveParseObservability() {
		verifier.pinToProductionRoute()
	}
	verifier.SetIncludedRanges(p.included)
	verifier.SetMemoryBudgetBytes(p.MemoryBudgetBytes())
	verifier.SetParseWorkLimits(p.parseWorkLimits)
	verifier.SetTimeoutMicros(p.timeoutMicros)
	verifier.SetCancellationFlag(p.cancellationFlag)
	verifier.maxConflictWidth = p.maxConflictWidth
	verifier.errorCostCompetition = p.errorCostCompetition
	verifier.recoveryInitialOnly = p.recoveryInitialOnly
	verifier.skipRecoveryReparse = p.skipRecoveryReparse
	return verifier
}

// incrementalTreesStructurallyEqual checks every public tree property used
// by the incremental parity gate. It runs only when a recovery frontier or
// a top-level state mismatch requires a fresh result check.
func incrementalTreesStructurallyEqual(a, b *Tree, lang *Language) bool {
	if a == nil || b == nil {
		return a == b
	}
	type pair struct{ a, b *Node }
	stack := []pair{{a.RootNode(), b.RootNode()}}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		left, right := current.a, current.b
		if left == nil || right == nil {
			if left != right {
				return false
			}
			continue
		}
		if left.Symbol() != right.Symbol() || left.Range() != right.Range() ||
			left.IsNamed() != right.IsNamed() || left.IsMissing() != right.IsMissing() ||
			left.IsExtra() != right.IsExtra() || left.IsError() != right.IsError() ||
			left.HasError() != right.HasError() || left.ChildCount() != right.ChildCount() {
			return false
		}
		for i := left.ChildCount() - 1; i >= 0; i-- {
			if left.FieldNameForChild(i, lang) != right.FieldNameForChild(i, lang) {
				return false
			}
			stack = append(stack, pair{left.Child(i), right.Child(i)})
		}
	}
	return true
}

// verifyIncrementalFreshResult authenticates the final public shape after
// recovery and compatibility normalization.
func (p *Parser) verifyIncrementalFreshResult(source []byte, oldTree *Tree, ts TokenSource, tree *Tree, timing *incrementalParseTiming) *Tree {
	// An error recovery frontier or a forced top-level settle can
	// change reductions outside the edited span. Verify the result
	// against the production fresh parse before publishing it.
	// Large unproven frontiers need a fresh result. Release the
	// incremental tree first to bound peak memory.
	largeUnprovenFrontier := len(source) >= 512*1024
	if largeUnprovenFrontier {
		tree.Release()
		tree = nil
	}
	started := time.Now()
	verifier := p.newIncrementalFreshVerifier()
	var fresh *Tree
	if p.reparseFactory != nil {
		if freshTokens, err := p.reparseFactory(source); err == nil {
			fresh, _ = verifier.ParseWithTokenSource(source, freshTokens)
		}
	} else {
		fresh, _ = verifier.Parse(source)
	}
	freshNanos := time.Since(started).Nanoseconds()
	if tree != nil && fresh != nil {
		// Compare published trees: the fresh API already normalized its
		// result, while this incremental attempt has not reached its API
		// normalization yet.
		p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	}
	// A stopped attempt can have the same visible shape as an accepted parse,
	// but its stop reason still controls the later retry policy. Authenticate
	// that reason too so a widening retry cannot replace a verified result.
	if fresh != nil && (largeUnprovenFrontier || tree.rawParseStopReason() != fresh.rawParseStopReason() || !incrementalTreesStructurallyEqual(tree, fresh, p.language)) {
		if tree != nil {
			tree.Release()
		}
		tree = fresh
		if timing != nil {
			reason := timing.reuseUnsupportedReason
			if reason == "" {
				reason = "recovery_frontier_unproven"
			}
			timing.recordFreshFallback(tree, freshNanos, reason)
		}
	} else if fresh != nil {
		fresh.Release()
		if timing != nil {
			timing.totalNanos += freshNanos
		}
	} else {
		// A failed verifier cannot authenticate the incremental tree.
		// Retry on the caller's full-parse route, even for a small source.
		if tree != nil {
			tree.Release()
		}
		tree = p.incrementalTokenSourceFreshFullParse(source, ts, timing)
		if timing != nil {
			timing.totalNanos += freshNanos
		}
	}
	return tree
}
