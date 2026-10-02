package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/sched"

// ParseWork counts work performed by engine attempts, including discarded
// results. Bytes counts tracked arena and scratch growth, not process RSS.
type ParseWork sched.Work

// ParseOperationWork accounts for the complete parse call. The phase counts
// are disjoint and sum to Total. ParseRuntime's other fields describe the
// selected attempt. An unchanged reparse retains the old tree's runtime;
// its incremental profile reports zero work.
type ParseOperationWork struct {
	Total, Initial, Compact, Retry, Fallback, Verification, Recovery, Forest ParseWork
}

func operationWorkSnapshot(work sched.OperationWork) ParseOperationWork {
	return ParseOperationWork{
		Total: ParseWork(work.Total), Initial: ParseWork(work.Initial),
		Compact: ParseWork(work.Compact), Retry: ParseWork(work.Retry),
		Fallback: ParseWork(work.Fallback), Verification: ParseWork(work.Verification),
		Recovery: ParseWork(work.Recovery), Forest: ParseWork(work.Forest),
	}
}

type parseOperationBudgetState struct {
	endBudget     func()
	owned         bool
	runtimeMemory runtimeMemoryBudgetRestore
}

func (p *Parser) beginParseOperationBudget(sourceLen ...int) parseOperationBudgetState {
	if p == nil {
		return parseOperationBudgetState{}
	}
	if p.cNodeMemoOperationDepth == 0 {
		// Keep retries in one operation warm. Start each independent operation
		// with the same logical cache size so parser reuse cannot change shape.
		if len(p.cNodeMemoCache) > cNodeMemoCacheInitialSize {
			p.cNodeMemoCache = p.cNodeMemoCache[:cNodeMemoCacheInitialSize]
		}
		p.cNodeMemoOperationPeakTier = RecoveryNodeMemoTierNone
		if cold := p.forestDeclineMemo; cold != nil {
			cold.cNodeMemoCollisions = 0
		}
	}
	p.cNodeMemoOperationDepth++
	state := parseOperationBudgetState{}
	if p.parseOperation == nil {
		p.parseOperationStorage = sched.Operation{
			NodeLimit:      uint64(p.parseWorkLimits.NodeLimit),
			IterationLimit: uint64(p.parseWorkLimits.IterationLimit),
		}
		if len(sourceLen) != 0 {
			p.parseOperationStorage.MemoryLimit = parseMemoryBudgetForParser(p, sourceLen[0])
			p.parseOperationStorage.MemoryConfigured = true
		}
		p.parseOperation = &p.parseOperationStorage
		p.parseOperationPhase = sched.Initial
		state.owned = true
	}
	if p.needsParseBudget() {
		state.endBudget = p.enterParseBudget()
	}
	if state.owned && len(sourceLen) != 0 {
		state.runtimeMemory = p.enterRuntimeMemoryBudget(p.parseOperation.MemoryLimit, sourceLen[0])
	}
	return state
}

func (p *Parser) endParseOperationBudget(state parseOperationBudgetState) {
	state.runtimeMemory.restore()
	if state.endBudget != nil {
		state.endBudget()
	}
	if p == nil {
		return
	}
	if state.owned {
		p.parseOperation = nil
		p.parseOperationPhase = sched.Initial
	}
	p.cNodeMemoOperationDepth--
	if p.cNodeMemoOperationDepth == 0 {
		p.finishCNodeMemoParse()
	}
}

func (p *Parser) captureOperationWork(tree *Tree) {
	if p != nil && p.parseOperation != nil && tree != nil {
		tree.ensureParseRuntime().OperationWork = operationWorkSnapshot(p.parseOperation.Work)
	}
}

func (p *Parser) recordOperationAttempt(phase sched.Phase, rt *ParseRuntime, allocatedNodes ...int) {
	if p == nil || p.parseOperation == nil {
		return
	}
	nodes := rt.NodesAllocated
	if len(allocatedNodes) != 0 {
		nodes = allocatedNodes[0]
	}
	p.parseOperation.LoopNodesSpent += uint64(max(0, rt.NodesAllocated))
	p.parseOperation.Add(phase, sched.Work{
		Attempts: 1, Tokens: rt.TokensConsumed,
		Nodes: uint64(max(0, nodes)), Iterations: uint64(max(0, rt.Iterations)),
		Bytes: uint64(max(int64(0), rt.ArenaBytesAllocated-rt.ArenaBaselineBytes)) +
			uint64(max(int64(0), rt.ScratchBytesAllocated-rt.ScratchBaselineBytes)),
	})
}

func (p *Parser) enterOperationPhase(phase sched.Phase) func() {
	previous := p.parseOperationPhase
	if previous != sched.Verification && previous != sched.Recovery {
		p.parseOperationPhase = phase
	}
	return func() { p.parseOperationPhase = previous }
}

// inheritParseOperation borrows the exact deadline and operation ledger. The
// child cannot create a new timeout window when its public parse method enters.
func (p *Parser) inheritParseOperation(parent *Parser, phase sched.Phase) func() {
	previousOperation, previousPhase := p.parseOperation, p.parseOperationPhase
	previousDepth, previousDeadline, previousStopped := p.parseBudgetDepth, p.parseDeadline, p.parseStoppedReason
	previousLive := uint64(0)
	if operation := parent.parseOperation; operation != nil {
		previousLive = operation.LiveBytes
		if parent.parseOperationArena != nil {
			operation.LiveBytes = parent.parseOperationMemoryBase + parent.operationMemoryGrowth(parent.parseOperationArena)
		}
	}
	p.parseOperation = parent.parseOperation
	if parent.parseOperationPhase == sched.Verification || parent.parseOperationPhase == sched.Recovery {
		phase = parent.parseOperationPhase
	}
	p.parseOperationPhase = phase
	p.timeoutMicros = parent.timeoutMicros
	p.cancellationFlag = parent.cancellationFlag
	p.parseWorkLimits = parent.parseWorkLimits
	if parent.parseBudgetDepth > 0 {
		p.parseBudgetDepth = 1
		p.parseDeadline = parent.parseDeadline
		p.parseStoppedReason = parent.parseStoppedReason
	}
	return func() {
		if operation := parent.parseOperation; operation != nil {
			operation.LiveBytes = previousLive
		}
		if parseStopReasonIsActive(p.parseStoppedReason) {
			parent.markActiveParseStopped(p.parseStoppedReason)
		}
		p.parseOperation, p.parseOperationPhase = previousOperation, previousPhase
		p.parseBudgetDepth, p.parseDeadline, p.parseStoppedReason = previousDepth, previousDeadline, previousStopped
	}
}

func (p *Parser) parseStopReasonNow() ParseStopReason {
	// Parse loops arm parseBudgetDepth. Retry work can poll cancellation between budgets.
	if p == nil || (p.parseBudgetDepth == 0 && p.cancellationFlag == nil) {
		return ParseStopNone
	}
	return p.activeParseStopReason()
}

func parseStopReasonIsTerminal(reason ParseStopReason) bool {
	return parseStopReasonIsActive(reason) || reason == ParseStopInvariantViolation
}

func (p *Parser) operationMemoryBudgetExceeded(arena *nodeArena) bool {
	return p != nil && p.parseOperation.MemoryExceeded(p.operationMemoryGrowth(arena))
}

func (p *Parser) operationMemoryGrowth(arena *nodeArena) uint64 {
	growth := uint64(0)
	if arena != nil {
		growth += uint64(max(int64(0), arena.allocatedBytes-arena.budgetBaselineBytes))
	}
	if scratch := p.budgetScratch; scratch != nil {
		growth += uint64(max(int64(0), scratch.allocatedBytes()-scratch.budgetBaselineBytes))
	}
	return growth
}
