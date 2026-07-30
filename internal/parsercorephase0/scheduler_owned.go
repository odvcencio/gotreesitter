package parsercorephase0

import (
	"errors"
	"fmt"
)

const inlineSchedulerCohortTargets = 8

// ReindexCondenseCandidatesOwned replaces the retained boundary lookup set
// with the scheduler versions that can receive the next action output.
func (c *Core) ReindexCondenseCandidatesOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate) error {
	return c.RunSchedulerOwned(owner, func() error {
		return c.reindexCondenseCandidatesUncheckpointed(candidates)
	})
}

// RecordReductionLineageOwned attaches exact scheduler lineage to compact graph
// versions. The journal restores prior provenance if the scheduler transaction
// rolls back.
func (c *Core) RecordReductionLineageOwned(owner SchedulerTransactionToken, outputs []ReductionOutput, lineage uint16) error {
	return c.RunSchedulerOwned(owner, func() error {
		if lineage == 0 {
			return errors.New("parser-core phase zero: zero reduction lineage")
		}
		for _, output := range outputs {
			if !output.MultiplePopPaths {
				return errors.New("parser-core phase zero: lineage requires multiple pop paths")
			}
			if err := c.recordNodeLineage(output.Head, output.CleanPathRank, lineage); err != nil {
				return err
			}
		}
		return nil
	})
}

// RecordHeadLineageOwned persists inherited scheduler lineage on one graph
// version. Unknown lineage invalidates an earlier exact record conservatively.
func (c *Core) RecordHeadLineageOwned(
	owner SchedulerTransactionToken,
	head Head,
	rank CleanPathRankSelection,
	lineage uint16,
) error {
	return c.RunSchedulerOwned(owner, func() error {
		if rank != CleanPathRankSelected && rank != CleanPathRankUnselected || lineage == 0 {
			rank = CleanPathRankUnknown
			lineage = 0
		}
		return c.recordNodeLineage(head, rank, lineage)
	})
}

func (c *Core) recordNodeLineage(head Head, rank CleanPathRankSelection, lineage uint16) error {
	node, err := c.nodeLineage(head.Node)
	if err != nil {
		return err
	}
	nextRank := rank
	nextLineage := lineage
	nextConverged := true
	switch rank {
	case CleanPathRankSelected, CleanPathRankUnselected:
	case CleanPathRankUnknown:
		nextLineage = 0
	case CleanPathRankNotApplicable:
		return nil
	default:
		return errors.New("parser-core phase zero: invalid clean path rank")
	}
	switch {
	case node.rank == CleanPathRankNotApplicable && node.lineage == 0:
	case node.lineage == nextLineage:
		nextRank = mergeCleanPathRank(node.rank, nextRank)
		if nextRank == CleanPathRankUnknown {
			nextLineage = 0
		}
	case node.rank == CleanPathRankUnknown && node.lineage == 0:
		nextRank = CleanPathRankUnknown
		nextLineage = 0
	default:
		nextRank = CleanPathRankUnknown
		nextLineage = 0
	}
	if node.rank == nextRank && node.lineage == nextLineage && node.converged == nextConverged {
		return nil
	}
	if len(c.transactions) != 0 {
		c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
			node: head.Node, owner: node.owner, lineage: node.lineage, rank: node.rank, converged: node.converged,
		})
	}
	node.rank = nextRank
	node.lineage = nextLineage
	node.converged = nextConverged
	return nil
}

// RecordHeadOwnerOwned binds one compact head to its scheduler lineage.
func (c *Core) RecordHeadOwnerOwned(owner SchedulerTransactionToken, head Head, lineage uint32) error {
	return c.RunSchedulerOwned(owner, func() error {
		if lineage == 0 {
			return errors.New("parser-core phase zero: zero scheduler lineage")
		}
		node, err := c.nodeLineage(head.Node)
		if err != nil {
			return err
		}
		if node.owner == lineage {
			return nil
		}
		if node.owner != 0 {
			return errors.New("parser-core phase zero: compact head has multiple scheduler owners")
		}
		if len(c.transactions) != 0 {
			c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
				node: head.Node, owner: node.owner, lineage: node.lineage, rank: node.rank, converged: node.converged,
			})
		}
		node.owner = lineage
		return nil
	})
}

func (c *Core) runSchedulerMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, fn func() error) error {
	return c.RunSchedulerOwned(owner, func() error {
		if !liveScoped {
			return fn()
		}
		return c.runLiveCondenseCandidates(candidates, fn)
	})
}

func (c *Core) runLiveCondenseCandidates(candidates []CondenseCandidate, fn func() error) error {
	if c.condenseScopeActive {
		return errors.New("parser-core phase zero: nested live condense candidate scope")
	}
	for index, candidate := range candidates {
		node, err := c.node(candidate.Head.Node)
		if err != nil {
			return err
		}
		for prior := 0; prior < index; prior++ {
			other := candidates[prior]
			otherNode, err := c.node(other.Head.Node)
			if err != nil {
				return err
			}
			if node.state == otherNode.state &&
				node.byteOffset == otherNode.byteOffset &&
				candidate.Shifted == other.Shifted &&
				candidate.Checkpoint == other.Checkpoint &&
				candidate.Head.Node != other.Head.Node {
				return errors.New("parser-core phase zero: distinct condense candidates share one boundary")
			}
		}
	}
	c.condenseCandidates = candidates
	c.condenseNewNode = NodeID(len(c.nodes) + 1)
	c.condenseScopeActive = true
	if c.schedulerFrame.fresh {
		err := fn()
		c.clearLiveCondenseCandidates()
		return err
	}
	defer c.clearLiveCondenseCandidates()
	return fn()
}

func (c *Core) clearLiveCondenseCandidates() {
	if c == nil {
		return
	}
	c.condenseCandidates = nil
	c.condenseNewNode = 0
	c.condenseScopeActive = false
}

func (c *Core) reindexCondenseCandidatesUncheckpointed(candidates []CondenseCandidate) error {
	c.boundaries.advanceGeneration()
	for _, candidate := range candidates {
		node, err := c.node(candidate.Head.Node)
		if err != nil {
			return err
		}
		key := boundaryKey{
			frontier: c.frontier, state: node.state, byteOffset: node.byteOffset,
			shifted: candidate.Shifted, checkpoint: candidate.Checkpoint,
		}
		probe, existing := c.boundaries.probe(boundaryIdentityFromKey(key))
		if probe.found {
			if existing != candidate.Head.Node {
				return errors.New("parser-core phase zero: distinct condense candidates share one boundary")
			}
			continue
		}
		if err := c.publishBoundary(probe, candidate.Head.Node); err != nil {
			return err
		}
	}
	return nil
}

// ShiftClassifiedOwned authenticates the scheduler owner, then delegates to
// the same uncheckpointed implementation used by standalone ShiftClassified.
func (c *Core) ShiftClassifiedOwned(owner SchedulerTransactionToken, boundary ClassifiedBoundary, actionOrdinal int, shifted Token, fork ForkOrder) (out Head, err error) {
	return c.shiftClassifiedMaybeLiveScopedOwned(owner, nil, false, boundary, actionOrdinal, shifted, fork)
}

// ShiftClassifiedWithLiveCondenseCandidatesOwned scopes condense candidates and shifts
// under one scheduler ownership check.
func (c *Core) ShiftClassifiedWithLiveCondenseCandidatesOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, boundary ClassifiedBoundary, actionOrdinal int, shifted Token, fork ForkOrder) (out Head, err error) {
	return c.shiftClassifiedMaybeLiveScopedOwned(owner, candidates, true, boundary, actionOrdinal, shifted, fork)
}

func (c *Core) shiftClassifiedMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, boundary ClassifiedBoundary, actionOrdinal int, shifted Token, fork ForkOrder) (out Head, err error) {
	err = c.runSchedulerMaybeLiveScopedOwned(owner, candidates, liveScoped, func() error {
		var innerErr error
		out, innerErr = c.shiftClassifiedUncheckpointed(boundary, actionOrdinal, shifted, fork)
		return innerErr
	})
	return out, err
}

func (c *Core) shiftClassifiedUncheckpointed(boundary ClassifiedBoundary, actionOrdinal int, shifted Token, fork ForkOrder) (Head, error) {
	act, err := c.classifiedActionRef(boundary, actionOrdinal)
	if err != nil {
		return Head{}, err
	}
	if act.Type != ActionShift {
		return Head{}, fmt.Errorf("parser-core phase zero: action %d is %v, not shift", actionOrdinal, act.Type)
	}
	if shifted.Symbol != boundary.lookahead {
		return Head{}, fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", shifted.Symbol, boundary.lookahead)
	}
	if shifted.Extra != act.Extra {
		return Head{}, fmt.Errorf("parser-core phase zero: token extra=%t disagrees with decoded action extra=%t", shifted.Extra, act.Extra)
	}
	targetState := act.State
	if act.Extra && targetState == 0 {
		targetState = boundary.state
	}
	payload, err := c.appendAuthenticatedTerminal(subtreeRecord{
		symbol: shifted.Symbol, startByte: shifted.StartByte, endByte: shifted.EndByte,
		extra: act.Extra, external: shifted.External, terminal: true,
	})
	if err != nil {
		return Head{}, err
	}
	phase0AObserveTerminalShift(c, payload, boundary.head.Node, targetState, shifted.EndByte, true, act.Extra, 0, fork)
	outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(targetState, shifted.EndByte), linkInput{
		prev: boundary.head.Node, payload: payload, order: fork,
	})
	if err != nil {
		return Head{}, err
	}
	c.addWork(&c.work.Shifts, 1)
	return outcome.head, nil
}

// ShiftOrdinaryClassifiedCohortOwned authenticates the scheduler owner, then
// delegates to the standalone cohort's uncheckpointed implementation.
func (c *Core) ShiftOrdinaryClassifiedCohortOwned(owner SchedulerTransactionToken, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	return c.shiftOrdinaryClassifiedCohortMaybeLiveScopedOwned(owner, nil, false, boundaries, shifted)
}

// ShiftOrdinaryClassifiedCohortWithLiveCondenseCandidatesOwned scopes the
// condense candidates and shifts one cohort under one scheduler ownership check.
func (c *Core) ShiftOrdinaryClassifiedCohortWithLiveCondenseCandidatesOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	return c.shiftOrdinaryClassifiedCohortMaybeLiveScopedOwned(owner, candidates, true, boundaries, shifted)
}

func (c *Core) shiftOrdinaryClassifiedCohortMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	err = c.runSchedulerMaybeLiveScopedOwned(owner, candidates, liveScoped, func() error {
		var inlineTargets [inlineSchedulerCohortTargets]StateID
		targets := inlineTargets[:]
		if len(boundaries) > len(targets) {
			targets = make([]StateID, len(boundaries))
		} else {
			targets = targets[:len(boundaries)]
		}
		if err := c.prepareOrdinaryClassifiedCohortInto(boundaries, shifted, targets); err != nil {
			return err
		}
		var innerErr error
		out, innerErr = c.shiftOrdinaryClassifiedCohortUncheckpointed(boundaries, targets, shifted)
		return innerErr
	})
	return out, err
}

func (c *Core) prepareOrdinaryClassifiedCohortInto(boundaries []ClassifiedBoundary, shifted Token, targets []StateID) error {
	if len(boundaries) == 0 {
		return errors.New("parser-core phase zero: empty ordinary classified shift cohort")
	}
	if len(targets) != len(boundaries) {
		return errors.New("parser-core phase zero: ordinary cohort target storage length mismatch")
	}
	if shifted.Extra {
		return errors.New("parser-core phase zero: cohort token is not an ordinary terminal")
	}
	for index, boundary := range boundaries {
		if shifted.Symbol != boundary.lookahead {
			return fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", shifted.Symbol, boundary.lookahead)
		}
		if cohortHasDuplicateHead(boundaries, index) {
			return fmt.Errorf("parser-core phase zero: duplicate ordinary cohort head %d", boundary.head.Node)
		}
		action, err := c.classifiedActionRef(boundary, 0)
		if err != nil {
			return err
		}
		if boundary.actions.Len() != 1 || action.State == 0 || *action != (Action{Type: ActionShift, State: action.State}) {
			return fmt.Errorf("parser-core phase zero: head %d does not select one ordinary shift", boundary.head.Node)
		}
		targets[index] = action.State
	}
	return nil
}

// cohortHasDuplicateHead reports whether boundaries[index] repeats a head node
// already present earlier in the cohort. Cohort widths are bounded by the live
// frontier, so this linear back-scan replaces a per-shift map allocation.
func cohortHasDuplicateHead(boundaries []ClassifiedBoundary, index int) bool {
	head := boundaries[index].head.Node
	for prior := 0; prior < index; prior++ {
		if boundaries[prior].head.Node == head {
			return true
		}
	}
	return false
}

func (c *Core) shiftOrdinaryClassifiedCohortUncheckpointed(boundaries []ClassifiedBoundary, targets []StateID, shifted Token) ([]Head, error) {
	payload, err := c.appendAuthenticatedTerminal(subtreeRecord{
		symbol: shifted.Symbol, startByte: shifted.StartByte, endByte: shifted.EndByte,
		external: shifted.External, terminal: true,
	})
	if err != nil {
		return nil, err
	}
	phase0AObserveTerminalCohortShift(c, payload, boundaries, targets, shifted.EndByte, false)
	out := c.cohortHeads(len(boundaries))
	for index, boundary := range boundaries {
		outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(targets[index], shifted.EndByte), linkInput{
			prev: boundary.head.Node, payload: payload,
		})
		if err != nil {
			return nil, err
		}
		out[index] = outcome.head
	}
	c.addWork(&c.work.Shifts, uint64(len(boundaries)))
	return out, nil
}

// cohortHeads returns a reused scratch slice of exactly width heads. The
// caller consumes the result before the next cohort shift, so one scheduler-
// owned buffer serves both the ordinary and extra cohort paths without
// allocating one head slice per shift.
func (c *Core) cohortHeads(width int) []Head {
	out := c.cohortHeadScratch
	if cap(out) < width {
		out = make([]Head, width, max(width, 2*cap(out)))
	} else {
		out = out[:width]
	}
	c.cohortHeadScratch = out
	return out
}

// ShiftExtraClassifiedCohortOwned authenticates the scheduler owner, then
// delegates to the standalone cohort's uncheckpointed implementation.
func (c *Core) ShiftExtraClassifiedCohortOwned(owner SchedulerTransactionToken, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	return c.shiftExtraClassifiedCohortMaybeLiveScopedOwned(owner, nil, false, boundaries, shifted)
}

// ShiftExtraClassifiedCohortWithLiveCondenseCandidatesOwned scopes the condense candidates
// and shifts one extra cohort under one scheduler ownership check.
func (c *Core) ShiftExtraClassifiedCohortWithLiveCondenseCandidatesOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	return c.shiftExtraClassifiedCohortMaybeLiveScopedOwned(owner, candidates, true, boundaries, shifted)
}

func (c *Core) shiftExtraClassifiedCohortMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	err = c.runSchedulerMaybeLiveScopedOwned(owner, candidates, liveScoped, func() error {
		var inlineTargets [inlineSchedulerCohortTargets]StateID
		targets := inlineTargets[:]
		if len(boundaries) > len(targets) {
			targets = make([]StateID, len(boundaries))
		} else {
			targets = targets[:len(boundaries)]
		}
		if err := c.prepareExtraClassifiedCohortInto(boundaries, shifted, targets); err != nil {
			return err
		}
		var innerErr error
		out, innerErr = c.shiftExtraClassifiedCohortUncheckpointed(boundaries, targets, shifted)
		return innerErr
	})
	return out, err
}

func (c *Core) prepareExtraClassifiedCohortInto(boundaries []ClassifiedBoundary, shifted Token, targets []StateID) error {
	if len(boundaries) == 0 {
		return errors.New("parser-core phase zero: empty extra classified shift cohort")
	}
	if len(targets) != len(boundaries) {
		return errors.New("parser-core phase zero: extra cohort target storage length mismatch")
	}
	if !shifted.Extra || shifted.EndByte < shifted.StartByte || shifted.EndByte == shifted.StartByte && !shifted.External {
		return errors.New("parser-core phase zero: cohort token has invalid extra-terminal geometry")
	}
	for index, boundary := range boundaries {
		if shifted.Symbol != boundary.lookahead {
			return fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", shifted.Symbol, boundary.lookahead)
		}
		if cohortHasDuplicateHead(boundaries, index) {
			return fmt.Errorf("parser-core phase zero: duplicate extra cohort head %d", boundary.head.Node)
		}
		action, err := c.classifiedActionRef(boundary, 0)
		if err != nil {
			return err
		}
		if boundary.actions.Len() != 1 || *action != (Action{Type: ActionShift, State: action.State, Extra: true}) {
			return fmt.Errorf("parser-core phase zero: head %d does not select one extra shift", boundary.head.Node)
		}
		targets[index] = action.State
		if targets[index] == 0 {
			targets[index] = boundary.state
		}
	}
	return nil
}

func (c *Core) shiftExtraClassifiedCohortUncheckpointed(boundaries []ClassifiedBoundary, targets []StateID, shifted Token) ([]Head, error) {
	payload, err := c.appendAuthenticatedTerminal(subtreeRecord{
		symbol: shifted.Symbol, startByte: shifted.StartByte, endByte: shifted.EndByte,
		extra: true, external: shifted.External, terminal: true,
	})
	if err != nil {
		return nil, err
	}
	phase0AObserveTerminalCohortShift(c, payload, boundaries, targets, shifted.EndByte, true)
	out := c.cohortHeads(len(boundaries))
	for index, boundary := range boundaries {
		outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(targets[index], shifted.EndByte), linkInput{
			prev: boundary.head.Node, payload: payload,
		})
		if err != nil {
			return nil, err
		}
		out[index] = outcome.head
	}
	c.addWork(&c.work.Shifts, uint64(len(boundaries)))
	return out, nil
}

// ReduceOutputsClassifiedIntoOwned authenticates the scheduler owner, then
// delegates to the standalone reduction's uncheckpointed implementation.
func (c *Core) ReduceOutputsClassifiedIntoOwned(owner SchedulerTransactionToken, dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	return c.reduceOutputsClassifiedIntoMaybeLiveScopedOwned(owner, nil, false, dst, boundary, actionOrdinal, fork)
}

// ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned scopes the condense candidates
// and reduces under one scheduler ownership check.
func (c *Core) ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	return c.reduceOutputsClassifiedIntoMaybeLiveScopedOwned(owner, candidates, true, dst, boundary, actionOrdinal, fork)
}

func (c *Core) reduceOutputsClassifiedIntoMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	err = c.runSchedulerMaybeLiveScopedOwned(owner, candidates, liveScoped, func() error {
		var innerErr error
		frontier, innerErr = c.reduceOutputsClassifiedIntoUncheckpointed(dst, boundary, actionOrdinal, fork)
		return innerErr
	})
	return frontier, err
}

func (c *Core) reduceOutputsClassifiedIntoUncheckpointed(dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) ([]ReductionOutput, error) {
	frontier := dst[:0]
	if c.popScratch.busy {
		return nil, errors.New("parser-core phase zero: reentrant reduction while pop scratch is active")
	}
	c.popScratch.busy = true
	// Keep both panic-safe releases in this small wrapper. The reduction body is
	// intentionally separate so its large frame does not register two runtime
	// defers for every reduction.
	defer c.popScratch.resetLogical()
	c.reductionScratch.begin()
	defer c.reductionScratch.finish()
	return c.reduceOutputsClassifiedIntoActive(frontier, boundary, actionOrdinal, fork)
}

func (c *Core) reduceOutputsClassifiedIntoActive(frontier []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) ([]ReductionOutput, error) {
	act, err := c.classifiedActionRef(boundary, actionOrdinal)
	if err != nil {
		return nil, err
	}
	if act.Type != ActionReduce {
		return nil, fmt.Errorf("parser-core phase zero: action %d is %v, not reduce", actionOrdinal, act.Type)
	}
	source, err := c.nodeLineage(boundary.head.Node)
	if err != nil {
		return nil, err
	}
	previousSourceOwner := c.reductionSourceOwner
	c.reductionSourceOwner = source.owner
	defer func() {
		c.reductionSourceOwner = previousSourceOwner
	}()
	plan, err := c.reductionPlan(*act)
	if err != nil {
		return nil, err
	}
	paths, err := c.popPaths(boundary.head.Node, int(act.ChildCount))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("parser-core phase zero: reduction has no exact pop path")
	}
	phase0ABeginReductionConstruction(c, uint64(len(paths)))
	if phase0AEnabled {
		phase0AObservePopRoutes(c, boundary.head.Node, int(act.ChildCount), paths)
	}
	c.addWork(&c.work.Reductions, 1)
	c.addWork(&c.work.ReductionPopRequests, 1)
	c.addWork(&c.work.EmittedPopPaths, uint64(len(paths)))
	for _, path := range paths {
		c.addWork(&c.work.EmittedPopPayloads, uint64(len(path.children)+len(path.trailing)))
	}
	scratch := &c.reductionScratch
	// len(paths) > 1 is this reduce's pop.size > 1 (a GSS multi-pop / diamond
	// merge): more than one distinct way to pop act.ChildCount entries was
	// found, one per path below -- mirrors the production Go parser's
	// applyReduceActionForked len(forks) > 1 check (parser_reduce.go). Every
	// parent reductionParentForPath builds in this loop is fragile in that
	// case; see its doc comment.
	multiPop := len(paths) > 1
	if multiPop {
		c.markReductionProductionRanks(paths)
	}
	for _, path := range paths {
		prev, err := c.node(path.prev)
		if err != nil {
			return nil, err
		}
		gotoState, err := c.tables.Goto(prev.state, act.Symbol)
		if err != nil {
			return nil, err
		}
		if gotoState == 0 {
			return nil, fmt.Errorf("parser-core phase zero: no goto from state %d for reduced symbol %d", prev.state, act.Symbol)
		}
		key := c.boundaryKey(gotoState, path.structuralEnd)
		payload, scoreDelta, order, err := c.reductionParentForPath(*act, &plan, path, key, fork, multiPop, scratch)
		if err != nil {
			return nil, err
		}
		parentLink := linkInput{prev: path.prev, payload: payload, scoreDelta: scoreDelta, order: order}
		phase0AObserveReductionOccurrence(c, parentLink, key)
		var out Head
		var outcome condenseOutcome
		if len(path.trailing) == 0 {
			outcome, err = c.condenseWithOutcomeAtomic(key, parentLink)
			out = outcome.head
		} else {
			out, err = c.appendPrivate(gotoState, path.structuralEnd, parentLink)
		}
		if err != nil {
			return nil, err
		}
		for index, trailing := range path.trailing {
			extra, err := c.subtree(trailing.payload)
			if err != nil {
				return nil, err
			}
			key = c.boundaryKey(gotoState, extra.endByte)
			extraLink := linkInput{prev: out.Node, payload: trailing.payload, scoreDelta: trailing.scoreDelta}
			if phase0AEnabled {
				phase0AObserveTrailingExtraMigration(c, uint32(index), key, extraLink)
			}
			if index == len(path.trailing)-1 {
				outcome, err = c.condenseWithOutcomeAtomic(key, extraLink)
				out = outcome.head
			} else {
				out, err = c.appendPrivate(gotoState, extra.endByte, extraLink)
			}
			if err != nil {
				return nil, err
			}
		}
		boundaryIndex, seen := scratch.boundary(key)
		var previous reductionBoundaryOutput
		if seen {
			previous = scratch.boundaries[boundaryIndex]
		}
		freshness := previous.freshness
		cleanPathRank := mergeCleanPathRank(previous.cleanPathRank, path.cleanPathRank)
		lineageDestinationRank := mergeCleanPathRank(
			previous.lineageDestinationRank,
			path.lineageDestinationRank,
		)
		historicalBoundarySplit := previous.historicalBoundarySplit || outcome.historicalBoundarySplit
		historicalConvergedSplit := previous.historicalConvergedSplit
		historicalForestDeterministic := previous.historicalForestDeterministic
		historicalCleanPathRank := previous.historicalCleanPathRank
		historicalLineage := previous.historicalLineage
		if outcome.historicalBoundarySplit {
			switch {
			case !previous.historicalBoundarySplit:
				historicalConvergedSplit = outcome.historicalConvergedSplit
				historicalForestDeterministic = outcome.historicalForestDeterministic
				historicalCleanPathRank = outcome.historicalCleanPathRank
				historicalLineage = outcome.historicalLineage
			case !historicalConvergedSplit && outcome.historicalConvergedSplit:
				historicalConvergedSplit = true
				historicalCleanPathRank = outcome.historicalCleanPathRank
				historicalLineage = outcome.historicalLineage
			case historicalConvergedSplit && !outcome.historicalConvergedSplit:
				// Keep the exact provenance from the converged historical version.
			case historicalCleanPathRank != outcome.historicalCleanPathRank ||
				historicalLineage != outcome.historicalLineage:
				historicalCleanPathRank = CleanPathRankUnknown
				historicalLineage = 0
			}
			if previous.historicalBoundarySplit {
				historicalForestDeterministic =
					historicalForestDeterministic && outcome.historicalForestDeterministic
			}
		}
		switch outcome.change {
		case condenseUnchanged:
			if !seen {
				freshness = ReductionUnchanged
			}
		case condenseNew:
			freshness = ReductionNew
		case condenseUpdated:
			if freshness != ReductionNew {
				freshness = ReductionUpdated
			}
		}
		scratch.store(boundaryIndex, seen, reductionBoundaryOutput{
			key: key, head: out, freshness: freshness, cleanPathRank: cleanPathRank,
			lineageDestinationRank:        lineageDestinationRank,
			historicalBoundarySplit:       historicalBoundarySplit,
			historicalConvergedSplit:      historicalConvergedSplit,
			historicalForestDeterministic: historicalForestDeterministic,
			historicalCleanPathRank:       historicalCleanPathRank,
			historicalLineage:             historicalLineage,
		})
	}
	for _, output := range scratch.boundaries {
		historicalProvenance := HistoricalBoundaryNone
		switch {
		case output.historicalConvergedSplit:
			historicalProvenance = HistoricalBoundaryConverged
		case output.historicalBoundarySplit && output.historicalForestDeterministic:
			historicalProvenance = HistoricalBoundaryDeterministic
		case output.historicalBoundarySplit:
			historicalProvenance = HistoricalBoundaryUnproved
		}
		frontier = append(frontier, ReductionOutput{
			Head:                         output.head,
			Freshness:                    output.freshness,
			CleanPathRank:                output.cleanPathRank,
			LineageDestinationRank:       output.lineageDestinationRank,
			MultiplePopPaths:             multiPop,
			HistoricalBoundaryProvenance: historicalProvenance,
			HistoricalCleanPathRank:      output.historicalCleanPathRank,
			HistoricalCleanPathLineage:   output.historicalLineage,
		})
	}
	phase0AFinishReductionConstruction(c)
	return frontier, nil
}
