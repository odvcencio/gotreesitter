package gotreesitter

import (
	"time"

	"github.com/odvcencio/gotreesitter/internal/incr"
)

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
		if left == right {
			continue
		}
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

// incrementalEOFExtraAppendMatchesOld checks a narrow lexer-owned proof
// against the exact edited old tree. The old accepted parse is
// a fresh witness when all lexical decisions and source-sensitive merge policy
// are unchanged. Comparing every public property also authenticates the edit's
// coordinate projection and the rebuilt frontier; shared nodes need no walk.
func (p *Parser) incrementalEOFExtraAppendMatchesOld(source []byte, oldTree, tree *Tree, ts TokenSource) bool {
	// The old witness does not certify fresh allocation or elapsed work under
	// explicit stop controls. Preserve the existing verifier for those callers.
	if oldTree == nil || tree == nil || oldTree.eofExtraTokenSourceProofID == 0 || len(oldTree.edits) != 1 || len(p.included) != 0 ||
		p.MemoryBudgetBytes() != 0 || p.timeoutMicros != 0 || p.cancellationFlag != nil ||
		oldTree.language != p.language || oldTree.sourceEncoding != InputEncodingUTF8 ||
		!resultCompatibilityElisionEligible(p.language) ||
		!oldTree.tokenInvariantReadSpanResultEligible() || !tree.tokenInvariantReadSpanResultEligible() ||
		tree.rawParseRuntime().MaxStacksSeen != 1 ||
		oldTree.rawParseRuntime().SourceLen != uint32(len(oldTree.source)) {
		return false
	}
	edit := oldTree.edits[0]
	if edit.StartPoint != edit.OldEndPoint || edit.NewEndPoint.Row != edit.StartPoint.Row ||
		uint64(edit.NewEndPoint.Column) != uint64(edit.StartPoint.Column)+1 {
		return false
	}
	leaf := oldTree.lastEditedLeaf
	if leaf == nil || leaf.ChildCount() != 0 || !leaf.IsExtra() || leaf.IsMissing() || leaf.HasError() ||
		leaf.EndByte() != uint32(len(source)) || oldTree.RootNode().EndByte() != uint32(len(source)) ||
		uint32(leaf.Symbol()) >= p.language.TokenCount {
		return false
	}
	if !incr.EOFExtraAppend(ts, p.language, oldTree.eofExtraTokenSourceProofID, oldTree.source, source,
		incr.TokenEdit{Start: edit.StartByte, OldEnd: edit.OldEndByte, NewEnd: edit.NewEndByte, Row: edit.StartPoint.Row},
		uint16(leaf.Symbol()), leaf.StartByte()) ||
		p.resolveParseMergePerKeyCap(oldTree.source, nil, 0) != p.resolveParseMergePerKeyCap(source, nil, 0) {
		return false
	}
	return incrementalTreesStructurallyEqual(tree, oldTree, p.language)
}

// verifyIncrementalFreshResult authenticates the final public shape after
// recovery and compatibility normalization.
func (p *Parser) verifyIncrementalFreshResult(source []byte, oldTree *Tree, ts TokenSource, tree *Tree, timing *incrementalParseTiming) *Tree {
	if p.incrementalEOFExtraAppendMatchesOld(source, oldTree, tree, ts) {
		return tree
	}
	// An error recovery frontier or a forced top-level settle can
	// change reductions outside the edited span. Verify the result
	// against the production fresh parse before publishing it.
	// Large unproven frontiers need a fresh result. Release the
	// incremental tree first to bound peak memory.
	largeUnprovenFrontier := incr.RequiresFreshResult(len(source))
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
	if fresh != nil {
		fresh.incrementalFreshVerified = true
	}
	freshNanos := time.Since(started).Nanoseconds()
	if tree != nil && fresh != nil {
		// Compare published trees: the fresh API already normalized its
		// result, while this incremental attempt has not reached its API
		// normalization yet.
		p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	}
	if fresh != nil && (largeUnprovenFrontier || !incrementalTreesStructurallyEqual(tree, fresh, p.language)) {
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
		tree.incrementalFreshVerified = true
		tree.eofExtraTokenSourceProofID = fresh.eofExtraTokenSourceProofID
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
