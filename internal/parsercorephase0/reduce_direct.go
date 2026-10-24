package parsercorephase0

import "errors"

// TryReduceDirectOwned completes a deterministic reduction and its scheduler
// publication in one owned operation. It declines before changing the graph
// when a pop encounters extras, alternatives, recovery, certificate references,
// or a previously published boundary. Those cases retain the general reducer.
//
// The fixed local window is an optimization bound, not a grammar limit. Its
// children are written in source order while walking the stack backwards;
// scores are still added left to right, preserving checked-overflow behavior.
func (c *Core) TryReduceDirectOwned(owner SchedulerTransactionToken, boundary *ClassifiedBoundary, publication ShiftHeadLineage) (out Head, handled bool, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, false, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	defer func() { err = c.finishSchedulerOwned(owner, err) }()
	return c.reduceDirectActive(owner, boundary, publication)
}

// reduceDirectActive is entered only under the single-version owner.
func (c *Core) reduceDirectActive(owner SchedulerTransactionToken, boundary *ClassifiedBoundary, publication ShiftHeadLineage) (out Head, handled bool, err error) {
	if !c.schedulerFrame.fresh || phase0AEnabled || c.condenseScopeActive || c.popScratch.busy || c.reduceConflictContext || c.reduceNoLookaheadContext {
		return out, false, nil
	}
	if publication.Owner == 0 {
		return out, false, errors.New("parser-core phase zero: zero scheduler lineage")
	}
	if boundary == nil {
		return out, false, errors.New("parser-core phase zero: nil direct reduction boundary")
	}
	act, err := c.classifiedActionRef(*boundary, 0)
	if err != nil {
		return out, false, err
	}
	const window = 32
	if boundary.actions.Len() != 1 || act.Type != ActionReduce || int(act.ChildCount) > window {
		return out, false, nil
	}
	// Resolve metadata before taking graph facts, in the same order as the
	// general reducer. Diagnostic adapters may publish recovery metadata.
	c.popScratch.busy = true
	defer c.popScratch.resetLogical()
	plan, err := c.reductionPlan(*act)
	if err != nil {
		return out, false, err
	}
	if c.nodeLineageRecoveryAt(boundary.head.Node).storedErrorCost != 0 {
		return out, false, nil
	}
	var children [window]SubtreeID
	var scores [window]int64
	path := popPath{prev: boundary.head.Node, startByte: boundary.byteOffset, structuralEnd: boundary.byteOffset}
	for index := int(act.ChildCount) - 1; index >= 0; index-- {
		n, nodeErr := c.node(path.prev)
		if nodeErr != nil {
			return out, false, nodeErr
		}
		if n.linkCount != 1 || n.firstLink == 0 || uint64(n.firstLink) > uint64(c.links.Len()) {
			return out, false, nil
		}
		link := c.links.At(int(n.firstLink - 1))
		if link.next != 0 || link.prev == 0 || link.prev >= path.prev || link.isRecoveryDiscontinuity() {
			return out, false, nil
		}
		payload, payloadErr := c.subtree(link.payload)
		if payloadErr != nil {
			return out, false, payloadErr
		}
		if payload.extra {
			return out, false, nil
		}
		children[index], scores[index] = link.payload, link.scoreDelta
		path.startByte = payload.startByte
		if index == int(act.ChildCount)-1 {
			path.structuralEnd = payload.endByte
		}
		if index == 0 && path.structuralEnd == 0 {
			path.structuralEnd = payload.endByte
		}
		// The generic source-order fold keeps the rightmost present order.
		if !path.order.Present && link.hasOrder() {
			path.order = ForkOrder{Value: link.order, Present: true}
		}
		path.prev = link.prev
	}
	previous, prevPaths, err := c.nodeWithPathCount(path.prev)
	if err != nil {
		return out, false, err
	}
	lineage, err := c.lineageView(path.prev)
	if err != nil {
		return out, false, err
	}
	if lineage.storedErrorCost != 0 || lineage.dropCohortRefIndex != 0 {
		return out, false, nil
	}
	target, err := c.tables.Goto(previous.state, act.Symbol)
	if err != nil {
		return out, false, err
	}
	if _, err := c.classifiedActionRef(*boundary, 0); err != nil {
		return out, false, err
	}
	if err := c.validateSchedulerTransaction(owner); err != nil {
		return out, false, err
	}
	boundary.preparedGotoPrev, boundary.preparedGotoState = path.prev, target
	// A callback may have changed recovery metadata without growing arenas.
	if target == 0 || c.nodeLineageRecoveryAt(boundary.head.Node).storedErrorCost != 0 ||
		c.nodeLineageRecoveryAt(path.prev).storedErrorCost != 0 || c.nodeLineageRecoveryAt(path.prev).dropCohortRefIndex != 0 {
		return out, false, nil
	}
	key := c.boundaryKey(target, path.structuralEnd)
	probe, _ := c.boundaries.probe(boundaryIdentityFromKey(key))
	if probe.found {
		return out, false, nil
	}
	path.children = children[:int(act.ChildCount)]
	for _, score := range scores[:int(act.ChildCount)] {
		path.score, err = checkedAddScore(path.score, score)
		if err != nil {
			return out, false, err
		}
	}
	// All refusals precede publication. The fresh owner poisons and discards
	// the session on any error from here, just as the general owned reducer.
	_, sourcePaths, err := c.nodeWithPathCount(boundary.head.Node)
	if err != nil {
		return out, false, err
	}
	c.reductionScratch.begin()
	defer c.reductionScratch.finish()
	if sourcePaths > uint64(c.limits.MaxLinksPerBoundary) {
		c.reductionScratch.pressureSeen = true
	}
	c.addWork(&c.work.Reductions, 1)
	c.addWork(&c.work.ReductionPopRequests, 1)
	c.addWork(&c.work.EmittedPopPaths, 1)
	c.addWork(&c.work.EmittedPopPayloads, uint64(act.ChildCount))
	payload, score, order, err := c.reductionParentForPath(*act, &plan, path, key, ForkOrder{}, false, &c.reductionScratch)
	if err != nil {
		return out, false, err
	}
	result, err := c.condenseDirectAppend(key, probe, prevPaths, linkInput{prev: path.prev, payload: payload, scoreDelta: score, order: order}, 0)
	if err != nil {
		return out, false, err
	}
	out = result.head
	if err = c.recordHeadOwner(out, publication.Owner); err != nil {
		return out, false, err
	}
	if publication.PublishLineage {
		err = c.publishShiftLineage(out, publication)
	}
	return out, err == nil, err
}
