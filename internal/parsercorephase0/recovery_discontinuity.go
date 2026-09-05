package parsercorephase0

import (
	"errors"
)

// RecoveryDiscontinuityContext authenticates the zero-width ERROR_STATE
// boundary at which a recovery discontinuity is published. The scheduler
// supplies the grammar state, byte position, and scanner checkpoint together.
type RecoveryDiscontinuityContext struct {
	ErrorState StateID
	ByteOffset uint32
	Checkpoint CheckpointID
}

// RecoveryStoredErrorCost returns the authenticated C stack-node error cost
// stored on one graph head. It supports scheduler pricing and merge audits.
func (c *Core) RecoveryStoredErrorCost(head Head) (uint32, error) {
	lineage, err := c.nodeLineage(head.Node)
	if err != nil {
		return 0, err
	}
	return lineage.storedErrorCost, nil
}

func (c *Core) validateRecoveryDiscontinuityContext(
	predecessor Head,
	context RecoveryDiscontinuityContext,
) (*nodeRecord, error) {
	if c == nil {
		return nil, errors.New("parser-core phase zero: recovery discontinuity on nil core")
	}
	if context.ErrorState != 0 {
		return nil, errors.New("parser-core phase zero: recovery discontinuity requires ERROR_STATE")
	}
	previous, err := c.node(predecessor.Node)
	if err != nil {
		return nil, err
	}
	if previous.byteOffset != context.ByteOffset {
		return nil, errors.New("parser-core phase zero: recovery discontinuity is not zero-width")
	}
	previousCheckpoint, exact := c.nodeScannerCheckpoint(predecessor.Node)
	if !exact || previousCheckpoint != context.Checkpoint {
		return nil, errors.New("parser-core phase zero: recovery discontinuity scanner checkpoint is not exact")
	}
	return previous, nil
}

func (c *Core) appendRecoveryDiscontinuityUncheckpointed(
	predecessor Head,
	context RecoveryDiscontinuityContext,
) (Head, error) {
	previous, err := c.validateRecoveryDiscontinuityContext(predecessor, context)
	if err != nil {
		return Head{}, err
	}
	previousLineage, err := c.nodeLineage(predecessor.Node)
	if err != nil {
		return Head{}, err
	}
	link := linkRecord{prev: predecessor.Node, flags: linkFlagRecoveryDiscontinuity}
	maximum := precedenceMaximumWitness{seed: previous.precedenceMax, hasSeed: true}
	node, err := c.appendAdjacencyNodeAtWithPrecedenceAndStoredErrorCost(
		context.ErrorState, context.ByteOffset, context.Checkpoint,
		[]linkRecord{link}, maximum, previousLineage.storedErrorCost, true,
	)
	if err != nil {
		return Head{}, err
	}
	if err := c.copyRecoveryDiscontinuityLineage(predecessor.Node, node); err != nil {
		return Head{}, err
	}
	return Head{Node: node}, nil
}

// copyRecoveryDiscontinuityLineage carries predecessor history onto the new
// marker without electing a scheduler owner. The copy aliases only immutable
// reference-set spill segments; later unions append to a separate segment.
func (c *Core) copyRecoveryDiscontinuityLineage(source, target NodeID) error {
	from, err := c.nodeLineage(source)
	if err != nil {
		return err
	}
	to, err := c.nodeLineage(target)
	if err != nil {
		return err
	}
	*to = *from
	to.owner = 0
	c.invalidateReusedLineageProof(target, *to)
	return nil
}

// AppendRecoveryDiscontinuityOwned publishes one scheduler-authenticated
// zero-width recovery edge. Ordinary shift and reduction inputs cannot create
// this edge because they expose no nullable payload form.
func (c *Core) AppendRecoveryDiscontinuityOwned(
	owner SchedulerTransactionToken,
	predecessor Head,
	context RecoveryDiscontinuityContext,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	out, err = c.appendRecoveryDiscontinuityUncheckpointed(predecessor, context)
	return out, c.finishSchedulerOwned(owner, err)
}

func (c *Core) recoveryDiscontinuityHeadLinks(head Head) (nodeRecord, []linkRecord, error) {
	node, err := c.node(head.Node)
	if err != nil {
		return nodeRecord{}, nil, err
	}
	if node.linkCount == 0 || node.firstLink == 0 || uint64(node.firstLink) > uint64(len(c.links)) {
		return nodeRecord{}, nil, errors.New("parser-core phase zero: recovery discontinuity head has no edges")
	}
	var inline [inlineAdjacencyCapacity]linkRecord
	links, err := c.publishedNodeLinksInto(inline[:0], *node)
	if err != nil {
		return nodeRecord{}, nil, err
	}
	for _, link := range links {
		if err := link.validateShape(); err != nil {
			return nodeRecord{}, nil, err
		}
		if !link.isRecoveryDiscontinuity() {
			return nodeRecord{}, nil, errors.New("parser-core phase zero: recovery discontinuity head has a payload edge")
		}
		if link.prev == 0 || link.prev >= head.Node {
			return nodeRecord{}, nil, errors.New("parser-core phase zero: recovery discontinuity predecessor does not decrease")
		}
	}
	return *node, links, nil
}

func (c *Core) mergeRecoveryDiscontinuityHeadsUncheckpointed(
	context RecoveryDiscontinuityContext,
	incumbent Head,
	incoming Head,
) (Head, error) {
	if context.ErrorState != 0 {
		return Head{}, errors.New("parser-core phase zero: recovery discontinuity requires ERROR_STATE")
	}
	if incumbent == incoming {
		return incumbent, nil
	}
	left, leftLinks, err := c.recoveryDiscontinuityHeadLinks(incumbent)
	if err != nil {
		return Head{}, err
	}
	right, rightLinks, err := c.recoveryDiscontinuityHeadLinks(incoming)
	if err != nil {
		return Head{}, err
	}
	if left.state != context.ErrorState || right.state != context.ErrorState ||
		left.byteOffset != context.ByteOffset || right.byteOffset != context.ByteOffset {
		return Head{}, errors.New("parser-core phase zero: recovery discontinuity heads are outside ERROR_STATE context")
	}
	leftCheckpoint, leftExact := c.nodeScannerCheckpoint(incumbent.Node)
	rightCheckpoint, rightExact := c.nodeScannerCheckpoint(incoming.Node)
	if !leftExact || !rightExact || leftCheckpoint != context.Checkpoint || rightCheckpoint != context.Checkpoint {
		return Head{}, errors.New("parser-core phase zero: recovery discontinuity heads have different scanner checkpoints")
	}
	leftLineage, err := c.nodeLineage(incumbent.Node)
	if err != nil {
		return Head{}, err
	}
	rightLineage, err := c.nodeLineage(incoming.Node)
	if err != nil {
		return Head{}, err
	}
	if leftLineage.storedErrorCost != rightLineage.storedErrorCost {
		return Head{}, errors.New("parser-core phase zero: recovery discontinuity heads have different stored recovery costs")
	}
	c.addWork(&c.work.PhysicalHeadMergeAttempts, 1)
	c.addWork(&c.work.PhysicalHeadMergeInputLinks, uint64(len(rightLinks)))
	links := append([]linkRecord(nil), leftLinks...)
	leftMaximum, err := c.nodePrecedenceMaximum(incumbent.Node)
	if err != nil {
		return Head{}, err
	}
	folded := precedenceMaximumWitness{seed: leftMaximum.value, hasSeed: true}
	changed := false
	for _, link := range rightLinks {
		var inserted bool
		links, inserted, err = c.insertLinkBounded(context.ErrorState, context.ByteOffset, links, link, 0, &folded)
		if err != nil {
			return Head{}, err
		}
		changed = changed || inserted
	}
	maximum, err := c.computePrecedenceMaximum(links, folded)
	if err != nil {
		return Head{}, err
	}
	if !changed && maximum.value <= leftMaximum.value {
		if err := c.mergeNodeLineageMetadata(incumbent.Node, incoming.Node, incumbent.Node); err != nil {
			return Head{}, err
		}
		c.addWork(&c.work.PhysicalHeadMergeSuccesses, 1)
		return incumbent, nil
	}
	merged, err := c.appendAdjacencyNodeAtWithPrecedenceAndStoredErrorCost(
		context.ErrorState, context.ByteOffset, context.Checkpoint,
		links, folded, leftLineage.storedErrorCost, true,
	)
	if err != nil {
		return Head{}, err
	}
	if err := c.mergeNodeLineageMetadata(incumbent.Node, incoming.Node, merged); err != nil {
		return Head{}, err
	}
	c.addWork(&c.work.PhysicalHeadMergeSuccesses, 1)
	return Head{Node: merged}, nil
}

// MergeRecoveryDiscontinuityHeadsOwned merges two scheduler-authenticated
// marker heads after recursively merging their older predecessor graphs.
func (c *Core) MergeRecoveryDiscontinuityHeadsOwned(
	owner SchedulerTransactionToken,
	context RecoveryDiscontinuityContext,
	incumbent Head,
	incoming Head,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	out, err = c.mergeRecoveryDiscontinuityHeadsUncheckpointed(context, incumbent, incoming)
	return out, c.finishSchedulerOwned(owner, err)
}
