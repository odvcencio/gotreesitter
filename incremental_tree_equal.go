package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/sched"

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
	verifier.SetCancellationFlag(p.cancellationFlag)
	verifier.maxConflictWidth = p.maxConflictWidth
	verifier.errorCostCompetition = p.errorCostCompetition
	verifier.recoveryInitialOnly = p.recoveryInitialOnly
	verifier.skipRecoveryReparse = p.skipRecoveryReparse
	verifier.inheritParseOperation(p, sched.Verification)
	return verifier
}

// incrementalTreesStructurallyEqual checks every public tree property used
// by the incremental parity gate. It runs only when a recovery frontier or
// a top-level state mismatch requires a fresh result check.
func incrementalTreesStructurallyEqual(a, b *Tree, lang *Language, check ...func() bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	type pair struct{ a, b *Node }
	stack := []pair{{a.RootNode(), b.RootNode()}}
	for len(stack) != 0 {
		if len(check) != 0 && !check[0]() {
			return false
		}
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
			if len(check) != 0 && !check[0]() {
				return false
			}
			if left.FieldNameForChild(i, lang) != right.FieldNameForChild(i, lang) {
				return false
			}
			stack = append(stack, pair{left.Child(i), right.Child(i)})
		}
	}
	return true
}

// Comparison belongs to the fresh verification attempt. Bound both node
// comparisons and wide child enumeration, and retain any lazy-view work.
func (p *Parser) incrementalTreesEqualForOperation(a, b *Tree) (bool, ParseStopReason) {
	type frame struct {
		arena *nodeArena
		nodes int
		bytes int64
	}
	var local [8]frame
	frames := local[:0]
	add := func(arena *nodeArena) {
		if arena == nil {
			return
		}
		for _, item := range frames {
			if item.arena == arena {
				return
			}
		}
		frames = append(frames, frame{arena, arena.used, arena.allocatedBytes})
	}
	for _, tree := range []*Tree{a, b} {
		if tree != nil {
			add(tree.arena)
			for _, arena := range tree.borrowedArena {
				add(arena)
			}
		}
	}
	phase := sched.Verification
	if p.parseOperationPhase == sched.Recovery {
		phase = sched.Recovery
	}
	defer func() {
		if operation := p.parseOperation; operation != nil {
			for _, item := range frames {
				operation.Add(phase, sched.Work{Nodes: uint64(max(0, item.arena.used-item.nodes)), Bytes: uint64(max(int64(0), item.arena.allocatedBytes-item.bytes))})
			}
		}
	}()
	reason := ParseStopNone
	check := func() bool {
		if reason = p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
			return false
		}
		operation := p.parseOperation
		if operation == nil {
			return true
		}
		if operation.IterationLimit > 0 && operation.IterationsSpent() >= operation.IterationLimit {
			reason = ParseStopIterationLimit
			return false
		}
		growth, volume, nodes := uint64(0), uint64(0), uint64(0)
		for _, item := range frames {
			growth += uint64(max(int64(0), item.arena.allocatedBytes-item.bytes))
			nodes += uint64(max(0, item.arena.used-item.nodes))
			volume += uint64(max(int64(0), item.arena.allocatedBytes))
		}
		if operation.NodeLimit > 0 && operation.NodesSpent()+nodes >= operation.NodeLimit {
			reason = ParseStopNodeLimit
			return false
		}
		if operation.MemoryExceeded(growth) || p.runtimeMemoryBudgetStopReason(volume) == ParseStopMemoryBudget {
			reason = ParseStopMemoryBudget
			return false
		}
		operation.Add(phase, sched.Work{Iterations: 1})
		return true
	}
	equal := incrementalTreesStructurallyEqual(a, b, p.language, check)
	return equal, reason
}
