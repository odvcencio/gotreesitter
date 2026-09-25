package gotreesitter

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
	verifier.pinToProductionRoute()
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

// incrementalEditTouchesExistingContent excludes a pure suffix append. Such
// an edit does not replace the recovery context that precedes the old EOF.
func incrementalEditTouchesExistingContent(old *Tree) bool {
	if old == nil {
		return false
	}
	length := int64(len(old.source))
	for _, edit := range old.edits {
		if int64(edit.StartByte) < length {
			return true
		}
		length += int64(edit.NewEndByte) - int64(edit.OldEndByte)
	}
	return false
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
