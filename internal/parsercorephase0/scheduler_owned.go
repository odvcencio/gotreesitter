package parsercorephase0

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
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
//
// Each output's branch identity (spec.b4b-alternative-set.v2 section 3.1) is
// its index within outputs, the dispatch's stable first-boundary order
// (reduceOutputsClassifiedIntoActive's final emission loop below). The
// driver's own establishment loops (parsercore_phase0_driver.go) iterate the
// identical outputs slice after this call returns, so their ordinals agree
// with no further synchronization. alternativeSetBranchCap declines rather
// than silently wrapping a branch ordinal past uint16 range; canonical
// reductions never approach it.
func (c *Core) RecordReductionLineageOwned(owner SchedulerTransactionToken, outputs []ReductionOutput, lineage uint16) error {
	return c.RunSchedulerOwned(owner, func() error {
		if lineage == 0 {
			return errors.New("parser-core phase zero: zero reduction lineage")
		}
		if len(outputs) >= alternativeSetBranchCap {
			return errAlternativeSetBranchOrdinalCap
		}
		for index, output := range outputs {
			if !output.MultiplePopPaths {
				return errors.New("parser-core phase zero: lineage requires multiple pop paths")
			}
			if err := c.recordNodeLineage(output.Head, output.CleanPathRank, lineage); err != nil {
				return err
			}
			if err := c.recordNodeLineageRefs(output.Head, output.DropCohortRefs); err != nil {
				return err
			}
			if err := c.recordNodeLineageMember(output.Head, lineage, uint16(index)); err != nil {
				return err
			}
		}
		// Count the authentic establishment only after every lineage mutation
		// succeeds. The surrounding scheduler transaction restores this counter
		// with the same checkpoint as the lineage journal.
		c.addDropCohortProducerWrite(dropCohortProducerReductionEstablishment)
		return nil
	})
}

// RecordHeadLineageOwned persists inherited scheduler lineage on one graph
// version, and unions set into its recorded alternative set in the same
// scheduler-owned operation. Unknown lineage invalidates an earlier exact
// scalar record conservatively; set is never invalidated (union only).
// Sharing one RunSchedulerOwned call for both halves -- rather than a
// second Owned call for the set -- halves the per-header token-validation
// cost of persistHeaderLineageOwned's per-dispatch persistence loop
// (parsercore_phase0_driver.go), its sole caller.
//
// setDirty lets that caller skip the set union outright: scalar dirtiness
// cannot be inferred from set dirtiness (a rank flip on an already-recorded
// lineage id changes rank without adding a member), so the scalar merge
// always runs -- it is already cheap when nothing changed
// (recordNodeLineage's own no-op guard) -- but the set union is both more
// expensive per call and, empirically, redundant far more often (the same
// (head, altSet) pair persisted again on a dispatch that never touched this
// header), so the caller may prove that in advance and skip it entirely.
func (c *Core) RecordHeadLineageOwned(
	owner SchedulerTransactionToken,
	head Head,
	rank CleanPathRankSelection,
	lineage uint16,
	set AlternativeSet,
	setBlended bool,
	setDirty bool,
	dropCohortRefs ...DropCohortRefSet,
) error {
	return c.RunSchedulerOwned(owner, func() error {
		if rank != CleanPathRankSelected && rank != CleanPathRankUnselected || lineage == 0 {
			rank = CleanPathRankUnknown
			lineage = 0
		}
		if err := c.recordNodeLineage(head, rank, lineage); err != nil {
			return err
		}
		if setDirty {
			if err := c.recordNodeLineageSet(head, set, setBlended); err != nil {
				return err
			}
		}
		for _, refs := range dropCohortRefs {
			if err := c.recordNodeLineageRefs(head, refs); err != nil {
				return err
			}
		}
		return nil
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
			node: head.Node, owner: node.owner, dropCohortRefs: node.dropCohortRefs,
			setCount: node.set.count, setFlags: node.set.flags, setSpillRef: node.set.spillRef,
			lineage: node.lineage, rank: node.rank, converged: node.converged, blended: node.blended,
			storedErrorCost: node.storedErrorCost,
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
				node: head.Node, owner: node.owner, dropCohortRefs: node.dropCohortRefs,
				setCount: node.set.count, setFlags: node.set.flags, setSpillRef: node.set.spillRef,
				lineage: node.lineage, rank: node.rank, converged: node.converged, blended: node.blended,
				storedErrorCost: node.storedErrorCost,
			})
		}
		node.owner = lineage
		return nil
	})
}

// RecordHeadStoredErrorCostOwned publishes the exact C stack-node error cost
// carried by one recovery head. The cost is lineage metadata, so nodeRecord
// remains compact and all updates stay rollback-safe.
func (c *Core) RecordHeadStoredErrorCostOwned(owner SchedulerTransactionToken, head Head, cost uint32) error {
	return c.RunSchedulerOwned(owner, func() error {
		return c.recordNodeStoredErrorCost(head, cost)
	})
}

func (c *Core) recordNodeStoredErrorCost(head Head, cost uint32) error {
	node, err := c.nodeLineage(head.Node)
	if err != nil {
		return err
	}
	if node.storedErrorCost == cost {
		return nil
	}
	frame := &c.schedulerFrame
	if !frame.active || len(c.transactions) == 0 ||
		c.transactions[len(c.transactions)-1] != frame.mark.transaction {
		return errors.New("parser-core phase zero: stored recovery cost requires a nested scheduler speculation")
	}
	if uint64(head.Node) <= uint64(frame.mark.nodeLineages) {
		return errors.New("parser-core phase zero: stored recovery cost cannot rewrite a published node")
	}
	if len(c.transactions) != 0 {
		c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
			node: head.Node, owner: node.owner, dropCohortRefs: node.dropCohortRefs,
			setCount: node.set.count, setFlags: node.set.flags, setSpillRef: node.set.spillRef,
			lineage: node.lineage, rank: node.rank, converged: node.converged, blended: node.blended,
			storedErrorCost: node.storedErrorCost,
		})
	}
	node.storedErrorCost = cost
	return nil
}

// RecordHeadLineageSetOwned unions set into one compact head's node record.
// Unlike the scalar clean-path pair, membership is never invalidated: this
// is a pure union, never a poisoning overwrite (spec.b4b-alternative-set.v1
// section 4).
func (c *Core) RecordHeadLineageSetOwned(owner SchedulerTransactionToken, head Head, set AlternativeSet, setBlended bool) error {
	return c.RunSchedulerOwned(owner, func() error {
		return c.recordNodeLineageSet(head, set, setBlended)
	})
}

// recordNodeLineageSet unions set into head's node's recorded alternative
// set. This is a fold-class union (spec.b4b-alternative-set.v2 section 3.4):
// the node's own recorded set may already carry ancestry from a different
// header/derivation thread that structurally shares this node (the shape
// section 2 describes), so set and the node's prior set are two
// independently tracked histories. The node's blended mark becomes true when
// setBlended is true, or when the node's prior recorded set and the
// incoming set are incomparable under containment -- computed before the
// union, since the union itself mutates node.set in place.
func (c *Core) recordNodeLineageSet(head Head, set AlternativeSet, setBlended bool) error {
	if set.count == 0 {
		return nil
	}
	node, err := c.nodeLineage(head.Node)
	if err != nil {
		return err
	}
	before := node.set
	beforeBlended := node.blended
	incomparable := c.AlternativeSetIncomparable(before, set)
	if !c.alternativeSetUnion(&node.set, set) {
		return nil
	}
	nextBlended := beforeBlended || setBlended || incomparable
	if len(c.transactions) != 0 {
		c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
			node: head.Node, owner: node.owner, dropCohortRefs: node.dropCohortRefs,
			setCount: before.count, setFlags: before.flags, setSpillRef: before.spillRef,
			lineage: node.lineage, rank: node.rank, converged: node.converged, blended: beforeBlended,
			storedErrorCost: node.storedErrorCost,
		})
	}
	node.blended = nextBlended
	return nil
}

// RecordHeadLineageRefsOwned unions drop-cohort references into one compact
// head. Keep this separate from the older scalar-and-alternative-set method
// so callers can migrate without changing the established API shape.
func (c *Core) RecordHeadLineageRefsOwned(owner SchedulerTransactionToken, head Head, refs DropCohortRefSet) error {
	return c.RunSchedulerOwned(owner, func() error {
		return c.recordNodeLineageRefs(head, refs)
	})
}

func (c *Core) recordNodeLineageRefs(head Head, refs DropCohortRefSet) error {
	if refs.Empty() && !refs.Overflowed() && !refs.Blended() {
		return nil
	}
	node, err := c.nodeLineage(head.Node)
	if err != nil {
		return err
	}
	before := node.dropCohortRefs
	changed, err := c.dropCohortRefUnion(&node.dropCohortRefs, refs)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if len(c.transactions) != 0 {
		c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
			node: head.Node, owner: node.owner, dropCohortRefs: before,
			setCount: node.set.count, setFlags: node.set.flags, setSpillRef: node.set.spillRef,
			lineage: node.lineage, rank: node.rank, converged: node.converged, blended: node.blended,
			storedErrorCost: node.storedErrorCost,
		})
	}
	return nil
}

// recordNodeLineageMember inserts pack(event, branch) into head's node's
// alternative set, unconditionally: membership is a per-event ancestry fact
// established at multi-pop reduction time, and it is never gated by
// clean-path rank or invalidated once recorded (spec.b4b-alternative-set.v1
// section 4). This runs alongside, and never changes the outcome of,
// recordNodeLineage's existing scalar merge. Establishment (spec.b4b-
// alternative-set.v2 section 3.4): inserting one freshly established member
// can never make a set incomparable with its own prior self, so this never
// touches the node's blended mark.
func (c *Core) recordNodeLineageMember(head Head, event, branch uint16) error {
	if !alternativeSetRecordingEnabled() {
		return nil
	}
	node, err := c.nodeLineage(head.Node)
	if err != nil {
		return err
	}
	before := node.set
	if !c.alternativeSetInsert(&node.set, packAlternativeSetMember(event, branch)) {
		return nil
	}
	if len(c.transactions) != 0 {
		c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
			node: head.Node, owner: node.owner, dropCohortRefs: node.dropCohortRefs,
			setCount: before.count, setFlags: before.flags, setSpillRef: before.spillRef,
			lineage: node.lineage, rank: node.rank, converged: node.converged, blended: node.blended,
			storedErrorCost: node.storedErrorCost,
		})
	}
	return nil
}

// enterLiveCondenseCandidates installs the live condense-candidate scope. It
// first merges clean, scheduler-proved equivalents that occupy one boundary.
// The normalized scope then contains one immutable physical head per boundary.
// It is the non-closure setup half of runLiveCondenseCandidates, factored out
// so the hot shift/reduce/cohort dispatch paths can call it directly.
func (c *Core) enterLiveCondenseCandidates(candidates []CondenseCandidate) error {
	return c.enterLiveCondenseCandidatesWithCost(candidates, false)
}

// enterLiveCondenseCandidatesWithCost installs a live candidate scope and
// optionally admits equal authenticated recovery costs.
func (c *Core) enterLiveCondenseCandidatesWithCost(candidates []CondenseCandidate, allowNonzeroCost bool) error {
	if c.condenseScopeActive {
		return errors.New("parser-core phase zero: nested live condense candidate scope")
	}
	if !c.schedulerFrame.active {
		return errors.New("parser-core phase zero: live condense candidate scope requires a scheduler transaction")
	}
	owned := append(c.condenseCandidates[:0], candidates...)
	normalized, err := c.mergeLiveCondenseCandidatesUncheckpointedWithCost(owned, allowNonzeroCost)
	if err != nil {
		clear(c.condenseCandidates)
		c.condenseCandidates = c.condenseCandidates[:0]
		return err
	}
	c.condenseCandidates = normalized
	c.condenseNewNode = NodeID(len(c.nodes) + 1)
	c.condenseScopeActive = true
	return nil
}

func (c *Core) mergeLiveCondenseCandidatesUncheckpointed(candidates []CondenseCandidate) ([]CondenseCandidate, error) {
	return c.mergeLiveCondenseCandidatesUncheckpointedWithCost(candidates, false)
}

func (c *Core) mergeLiveCondenseCandidatesUncheckpointedWithCost(candidates []CondenseCandidate, allowNonzeroCost bool) ([]CondenseCandidate, error) {
	write := 0
	for _, candidate := range candidates {
		lineage, err := c.nodeLineage(candidate.Head.Node)
		if err != nil {
			return nil, err
		}
		if candidate.ErrorCost != lineage.storedErrorCost {
			return nil, errors.New("parser-core phase zero: condense candidate error cost does not match its head")
		}
		if candidate.ErrorCost != 0 && !allowNonzeroCost {
			return nil, errors.New("parser-core phase zero: physical head merge requires zero error cost")
		}
		if err := c.recordNodeLineageRefs(candidate.Head, candidate.DropCohortRefs); err != nil {
			return nil, err
		}
		node, err := c.node(candidate.Head.Node)
		if err != nil {
			return nil, err
		}
		match := -1
		for prior := 0; prior < write; prior++ {
			other := candidates[prior]
			otherNode, err := c.node(other.Head.Node)
			if err != nil {
				return nil, err
			}
			candidateNodeCheckpoint, candidateNodeExact := c.nodeScannerCheckpoint(candidate.Head.Node)
			otherNodeCheckpoint, otherNodeExact := c.nodeScannerCheckpoint(other.Head.Node)
			if node.state == otherNode.state &&
				node.byteOffset == otherNode.byteOffset &&
				candidate.Shifted == other.Shifted &&
				candidate.Checkpoint == other.Checkpoint &&
				candidate.MergeIdentity == other.MergeIdentity &&
				candidateNodeExact && otherNodeExact &&
				candidateNodeCheckpoint == otherNodeCheckpoint {
				match = prior
				break
			}
		}
		if match < 0 {
			candidates[write] = candidate
			write++
			continue
		}
		group := &candidates[match]
		if group.Head != candidate.Head {
			key := boundaryKey{
				frontier: c.frontier, state: node.state, byteOffset: node.byteOffset,
				shifted: candidate.Shifted, checkpoint: candidate.Checkpoint,
			}
			probe, existing := c.boundaries.probe(boundaryIdentityFromKey(key))
			switch {
			case !probe.found:
				if err := c.publishBoundary(probe, group.Head.Node); err != nil {
					return nil, err
				}
			case existing == candidate.Head.Node:
				if err := c.publishBoundary(probe, group.Head.Node); err != nil {
					return nil, err
				}
			case existing != group.Head.Node:
				// A live scope can observe an older entry from the same parser
				// frontier. Keep both current candidates separate. The next
				// authenticated reindex establishes a fresh boundary generation.
				candidates[write] = candidate
				write++
				continue
			}
			merged, err := c.mergeEquivalentHeadsAtBoundaryUncheckpointedWithCost(key, group.Head, candidate.Head, allowNonzeroCost)
			if err != nil {
				return nil, err
			}
			group.Head = merged
		}
		if _, err := c.dropCohortRefUnion(&group.DropCohortRefs, candidate.DropCohortRefs); err != nil {
			return nil, err
		}
	}
	clear(candidates[write:])
	return candidates[:write], nil
}

// runLiveCondenseCandidates keeps the closure-taking shape used directly by
// tests. Production dispatch no longer routes through it; see
// enterLiveCondenseCandidates and the *MaybeLiveScopedOwned methods below.
func (c *Core) runLiveCondenseCandidates(candidates []CondenseCandidate, fn func() error) error {
	if err := c.enterLiveCondenseCandidates(candidates); err != nil {
		return err
	}
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
	clear(c.condenseCandidates)
	c.condenseCandidates = c.condenseCandidates[:0]
	c.condenseNewNode = 0
	c.condenseScopeActive = false
}

func (c *Core) reindexCondenseCandidatesUncheckpointed(candidates []CondenseCandidate) error {
	c.boundaries.advanceGeneration()
	for index, candidate := range candidates {
		lineage, err := c.nodeLineage(candidate.Head.Node)
		if err != nil {
			return err
		}
		if candidate.ErrorCost != lineage.storedErrorCost {
			return errors.New("parser-core phase zero: condense candidate error cost does not match its head")
		}
		if candidate.ErrorCost != 0 {
			return errors.New("parser-core phase zero: physical head merge requires zero error cost")
		}
		if err := c.recordNodeLineageRefs(candidate.Head, candidate.DropCohortRefs); err != nil {
			return err
		}
		node, err := c.node(candidate.Head.Node)
		if err != nil {
			return err
		}
		candidateNodeCheckpoint, candidateNodeExact := c.nodeScannerCheckpoint(candidate.Head.Node)
		if !candidateNodeExact {
			return errors.New("parser-core phase zero: physical head merge has inexact scanner provenance")
		}
		for prior := 0; prior < index; prior++ {
			other := candidates[prior]
			otherNode, err := c.node(other.Head.Node)
			if err != nil {
				return err
			}
			if node.state != otherNode.state || node.byteOffset != otherNode.byteOffset ||
				candidate.Shifted != other.Shifted || candidate.Checkpoint != other.Checkpoint {
				continue
			}
			if candidate.MergeIdentity != other.MergeIdentity {
				return errors.New("parser-core phase zero: physical heads have different lexer ownership")
			}
			otherNodeCheckpoint, otherNodeExact := c.nodeScannerCheckpoint(other.Head.Node)
			if !otherNodeExact || candidateNodeCheckpoint != otherNodeCheckpoint {
				return errors.New("parser-core phase zero: physical heads have different scanner provenance")
			}
		}
		key := boundaryKey{
			frontier: c.frontier, state: node.state, byteOffset: node.byteOffset,
			shifted: candidate.Shifted, checkpoint: candidate.Checkpoint,
		}
		probe, existing := c.boundaries.probe(boundaryIdentityFromKey(key))
		if probe.found {
			if existing != candidate.Head.Node {
				if _, err := c.mergeEquivalentHeadsAtBoundaryUncheckpointed(
					key, Head{Node: existing}, candidate.Head,
				); err != nil {
					return err
				}
			}
			continue
		}
		if err := c.publishBoundary(probe, candidate.Head.Node); err != nil {
			return err
		}
	}
	return nil
}

// mergeEquivalentHeadsAtBoundaryUncheckpointed ports ts_stack_merge for two
// clean scheduler versions. It keeps the incumbent adjacency first, applies
// C's bounded link insertion rules to the incoming adjacency, then publishes
// one immutable replacement at the same authenticated boundary. Capacity
// rejection rolls back the merge. C removes the incoming version after that
// rejection, but this bounded route fails closed to preserve its memory limit.
//
// Header-owned recovery state and lexer ownership are outside Core. The
// scheduler must prove those identities before it supplies candidates here.
func (c *Core) mergeEquivalentHeadsAtBoundaryUncheckpointed(
	key boundaryKey,
	incumbent Head,
	incoming Head,
) (Head, error) {
	return c.mergeEquivalentHeadsAtBoundaryUncheckpointedWithCost(key, incumbent, incoming, false)
}

// mergeEquivalentHeadsAtBoundaryUncheckpointedWithCost merges equivalent
// heads after the caller authenticates their complete stored cost. Recovery
// reductions use the cost-enabled form because equal nonzero paths remain
// physically mergeable in the C stack.
func (c *Core) mergeEquivalentHeadsAtBoundaryUncheckpointedWithCost(
	key boundaryKey,
	incumbent Head,
	incoming Head,
	allowNonzeroCost bool,
) (Head, error) {
	return c.mergeEquivalentHeadsAtBoundaryUncheckpointedWithRecovery(
		key, incumbent, incoming, allowNonzeroCost, nil,
	)
}

func (c *Core) mergeEquivalentHeadsAtBoundaryUncheckpointedWithRecovery(
	key boundaryKey,
	incumbent Head,
	incoming Head,
	allowNonzeroCost bool,
	recovery *recoveryMergeEquivalenceContext,
) (Head, error) {
	if key.frontier != c.frontier {
		return Head{}, errors.New("parser-core phase zero: physical head merge frontier mismatch")
	}
	if incumbent == incoming {
		return incumbent, nil
	}
	left, err := c.node(incumbent.Node)
	if err != nil {
		return Head{}, err
	}
	right, err := c.node(incoming.Node)
	if err != nil {
		return Head{}, err
	}
	if left.state != key.state || right.state != key.state ||
		left.byteOffset != key.byteOffset || right.byteOffset != key.byteOffset {
		return Head{}, errors.New("parser-core phase zero: physical heads are not boundary-equivalent")
	}
	leftCheckpoint, leftExact := c.nodeScannerCheckpoint(incumbent.Node)
	rightCheckpoint, rightExact := c.nodeScannerCheckpoint(incoming.Node)
	if !leftExact || !rightExact || leftCheckpoint != rightCheckpoint {
		return Head{}, errors.New("parser-core phase zero: physical heads have different scanner provenance")
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
		return Head{}, errors.New("parser-core phase zero: physical heads have different stored recovery costs")
	}
	if leftLineage.storedErrorCost != 0 && !allowNonzeroCost {
		return Head{}, errors.New("parser-core phase zero: physical head merge requires zero error cost")
	}
	related, err := c.nodesAncestryRelated(incumbent.Node, incoming.Node)
	if err != nil {
		return Head{}, err
	}
	if related {
		return Head{}, errors.New("parser-core phase zero: physical heads are ancestry-related")
	}

	var leftInline [inlineAdjacencyCapacity]linkRecord
	links, err := c.publishedNodeLinksInto(leftInline[:0], *left)
	if err != nil {
		return Head{}, err
	}
	var rightInline [inlineAdjacencyCapacity]linkRecord
	incomingLinks, err := c.publishedNodeLinksInto(rightInline[:0], *right)
	if err != nil {
		return Head{}, err
	}
	for _, link := range links {
		if link.isRecoveryDiscontinuity() {
			return Head{}, errors.New("parser-core phase zero: recovery discontinuity requires its dedicated merge seam")
		}
	}
	for _, link := range incomingLinks {
		if link.isRecoveryDiscontinuity() {
			return Head{}, errors.New("parser-core phase zero: recovery discontinuity requires its dedicated merge seam")
		}
	}
	// Count only pairs that pass the clean boundary and scanner predicates.
	// C's wider attempt counter also includes pairs that fail those predicates.
	c.addWork(&c.work.PhysicalHeadMergeAttempts, 1)
	c.addWork(&c.work.PhysicalHeadMergeInputLinks, uint64(len(incomingLinks)))
	leftMaximum, err := c.nodePrecedenceMaximum(incumbent.Node)
	if err != nil {
		return Head{}, err
	}
	folded := precedenceMaximumWitness{seed: leftMaximum.value, hasSeed: true}
	changed := false
	for _, link := range incomingLinks {
		var inserted bool
		links, inserted, err = c.insertLinkBoundedWithRecovery(
			key.state, key.byteOffset, links, link, 0, &folded, recovery,
		)
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
	// key.checkpoint authenticates the current lookahead boundary. The graph
	// node checkpoint authenticates the last consumed external scanner state.
	// Keep the source node checkpoint when those two identities differ.
	merged, err := c.appendAdjacencyNodeAtWithPrecedenceAndStoredErrorCost(
		key.state, key.byteOffset, leftCheckpoint, links, folded,
		leftLineage.storedErrorCost, true,
	)
	if err != nil {
		return Head{}, err
	}
	if err := c.mergeNodeLineageMetadata(incumbent.Node, incoming.Node, merged); err != nil {
		return Head{}, err
	}
	probe, existing := c.boundaries.probe(boundaryIdentityFromKey(key))
	if !probe.found || existing != incumbent.Node {
		return Head{}, errors.New("parser-core phase zero: physical head merge lost its incumbent boundary")
	}
	if err := c.publishBoundary(probe, merged); err != nil {
		return Head{}, err
	}
	c.addWork(&c.work.PhysicalHeadMergeSuccesses, 1)
	return Head{Node: merged}, nil
}

// MergeEquivalentHeadsOwned merges two clean scheduler heads under one
// authenticated owner. The scheduler supplies the complete boundary key.
// Recovery-costed heads use a separate pricing route and cannot enter this
// clean merge seam.
func (c *Core) MergeEquivalentHeadsOwned(
	owner SchedulerTransactionToken,
	state StateID,
	byteOffset uint32,
	checkpoint CheckpointID,
	shifted bool,
	incumbent Head,
	incoming Head,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	key := boundaryKey{
		frontier: c.frontier, state: state, byteOffset: byteOffset,
		shifted: shifted, checkpoint: checkpoint,
	}
	out, err = c.mergeEquivalentHeadsAtBoundaryUncheckpointed(key, incumbent, incoming)
	return out, c.finishSchedulerOwned(owner, err)
}

// MergeEquivalentHeadsWithStoredErrorCostOwned merges equal-cost scheduler
// heads under one authenticated owner. The Core checks the stored cost before
// it emits merge work or graph links.
func (c *Core) MergeEquivalentHeadsWithStoredErrorCostOwned(
	owner SchedulerTransactionToken,
	state StateID,
	byteOffset uint32,
	checkpoint CheckpointID,
	shifted bool,
	incumbent Head,
	incoming Head,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	key := boundaryKey{
		frontier: c.frontier, state: state, byteOffset: byteOffset,
		shifted: shifted, checkpoint: checkpoint,
	}
	out, err = c.mergeEquivalentHeadsAtBoundaryUncheckpointedWithCost(
		key, incumbent, incoming, true,
	)
	return out, c.finishSchedulerOwned(owner, err)
}

// MergeEquivalentRecoveryHeadsOwned merges equal-cost recovery heads with
// C's positive-error subtree equivalence rule. The scheduler supplies the
// recovery pricing decision through a callback.
func (c *Core) MergeEquivalentRecoveryHeadsOwned(
	owner SchedulerTransactionToken,
	state StateID,
	byteOffset uint32,
	checkpoint CheckpointID,
	shifted bool,
	incumbent Head,
	incoming Head,
	payloadHasError PayloadErrorPresenceFunc,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if payloadHasError == nil {
		return out, c.finishSchedulerOwned(
			owner,
			errors.New("parser-core phase zero: recovery head merge pricing context is unavailable"),
		)
	}
	key := boundaryKey{
		frontier: c.frontier, state: state, byteOffset: byteOffset,
		shifted: shifted, checkpoint: checkpoint,
	}
	context := recoveryMergeEquivalenceContext{payloadHasError: payloadHasError}
	out, err = c.mergeEquivalentHeadsAtBoundaryUncheckpointedWithRecovery(
		key, incumbent, incoming, true, &context,
	)
	return out, c.finishSchedulerOwned(owner, err)
}

// mergeNodeLineageMetadata preserves the histories represented by a physical
// graph merge. A new merged node has no scheduler owner until canonicalization
// elects its surviving header. An unchanged incumbent keeps its current owner.
func (c *Core) mergeNodeLineageMetadata(leftID, rightID, targetID NodeID) error {
	left, err := c.nodeLineage(leftID)
	if err != nil {
		return err
	}
	right, err := c.nodeLineage(rightID)
	if err != nil {
		return err
	}
	if left.storedErrorCost != right.storedErrorCost {
		return errors.New("parser-core phase zero: merged heads have different stored recovery costs")
	}
	target, err := c.nodeLineage(targetID)
	if err != nil {
		return err
	}
	before := *target
	merged := nodeLineageRecord{}
	merged.storedErrorCost = left.storedErrorCost
	if targetID == leftID {
		merged.owner = left.owner
	}
	merged.dropCohortRefs = left.dropCohortRefs
	if _, err := c.dropCohortRefUnion(&merged.dropCohortRefs, right.dropCohortRefs); err != nil {
		return err
	}
	merged.set = left.set
	merged.blended = left.blended || right.blended || c.AlternativeSetIncomparable(left.set, right.set)
	c.alternativeSetUnion(&merged.set, right.set)
	merged.rank, merged.lineage = mergeNodeLineageScalars(
		left.rank, left.lineage, right.rank, right.lineage,
	)
	merged.converged = left.converged || right.converged
	if *target == merged {
		return nil
	}
	if targetID == leftID && len(c.transactions) != 0 {
		c.nodeLineageJournal = append(c.nodeLineageJournal, nodeLineageMutation{
			node: targetID, owner: before.owner, dropCohortRefs: before.dropCohortRefs,
			setCount: before.set.count, setFlags: before.set.flags, setSpillRef: before.set.spillRef,
			lineage: before.lineage, rank: before.rank, converged: before.converged, blended: before.blended,
			storedErrorCost: before.storedErrorCost,
		})
	}
	*target = merged
	return nil
}

func mergeNodeLineageScalars(
	leftRank CleanPathRankSelection,
	leftLineage uint16,
	rightRank CleanPathRankSelection,
	rightLineage uint16,
) (CleanPathRankSelection, uint16) {
	if leftRank == CleanPathRankUnknown || rightRank == CleanPathRankUnknown {
		return CleanPathRankUnknown, 0
	}
	if leftRank == CleanPathRankNotApplicable || leftLineage == 0 {
		return rightRank, rightLineage
	}
	if rightRank == CleanPathRankNotApplicable || rightLineage == 0 {
		return leftRank, leftLineage
	}
	if leftLineage != rightLineage {
		return CleanPathRankUnknown, 0
	}
	return mergeCleanPathRank(leftRank, rightRank), leftLineage
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

// ErrorRegionResumeWithLiveCondenseCandidatesOwned isolates one recovery
// version from unrelated live versions while it publishes its ERROR payload.
func (c *Core) ErrorRegionResumeWithLiveCondenseCandidatesOwned(
	owner SchedulerTransactionToken,
	candidates []CondenseCandidate,
	preErrorHead Head,
	preErrorState StateID,
	startByte, endByte uint32,
	children []SubtreeID,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.enterLiveCondenseCandidates(candidates); err != nil {
		return out, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		out, err = c.ErrorRegionResume(preErrorHead, preErrorState, startByte, endByte, children)
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	out, err = c.ErrorRegionResume(preErrorHead, preErrorState, startByte, endByte, children)
	return out, c.finishSchedulerOwned(owner, err)
}

// ErrorRegionResumeWithLiveCondenseCandidatesAndCostOwned publishes one
// recovery ERROR region with an authenticated cumulative cost while keeping
// unrelated live candidates outside its condensation scope.
func (c *Core) ErrorRegionResumeWithLiveCondenseCandidatesAndCostOwned(
	owner SchedulerTransactionToken,
	candidates []CondenseCandidate,
	preErrorHead Head,
	preErrorState StateID,
	startByte, endByte uint32,
	children []SubtreeID,
	cost ReductionOutputCostFunc,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.enterLiveCondenseCandidatesWithCost(candidates, true); err != nil {
		return out, c.finishSchedulerOwned(owner, err)
	}
	if cost == nil {
		err = errors.New("parser-core phase zero: error-region cost callback is required")
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		out, err = c.ErrorRegionResumeWithCost(preErrorHead, preErrorState, startByte, endByte, children, cost)
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	out, err = c.ErrorRegionResumeWithCost(preErrorHead, preErrorState, startByte, endByte, children, cost)
	return out, c.finishSchedulerOwned(owner, err)
}

// ShiftDirectWithLiveCondenseCandidatesOwned applies a corridor-proven shift
// without rebuilding a ClassifiedBoundary or running the generic shift path.
// The caller must validate the action shape from the immutable corridor program.
func (c *Core) ShiftDirectWithLiveCondenseCandidatesOwned(
	owner SchedulerTransactionToken,
	candidates []CondenseCandidate,
	head Head,
	lookahead Symbol,
	targetState StateID,
	extra bool,
	shifted Token,
	fork ForkOrder,
) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.enterLiveCondenseCandidates(candidates); err != nil {
		return out, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		out, err = c.shiftDirectUncheckpointed(head, lookahead, targetState, extra, shifted, fork)
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	out, err = c.shiftDirectUncheckpointed(head, lookahead, targetState, extra, shifted, fork)
	return out, c.finishSchedulerOwned(owner, err)
}

// shiftClassifiedMaybeLiveScopedOwned inlines the former
// runSchedulerMaybeLiveScopedOwned/runLiveCondenseCandidates closure chain: it
// validates the owner, installs the live condense scope (when requested),
// calls the uncheckpointed shift directly, and tears the scope back down --
// all without allocating or invoking a fn func() error value. Every step here
// mirrors RunSchedulerOwned's and runLiveCondenseCandidates's own semantics
// exactly (same validation, same panic/error poisoning, same fresh-session
// cleanup asymmetry), so this dispatch call site is direct and inlinable.
func (c *Core) shiftClassifiedMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, boundary ClassifiedBoundary, actionOrdinal int, shifted Token, fork ForkOrder) (out Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if !liveScoped {
		out, err = c.shiftClassifiedUncheckpointed(boundary, actionOrdinal, shifted, fork)
		return out, c.finishSchedulerOwned(owner, err)
	}
	if err = c.enterLiveCondenseCandidates(candidates); err != nil {
		return out, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		out, err = c.shiftClassifiedUncheckpointed(boundary, actionOrdinal, shifted, fork)
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	out, err = c.shiftClassifiedUncheckpointed(boundary, actionOrdinal, shifted, fork)
	return out, c.finishSchedulerOwned(owner, err)
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
	}, shifted.LexerSkippedPrefixLength)
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

func (c *Core) shiftDirectUncheckpointed(
	head Head,
	lookahead Symbol,
	targetState StateID,
	extra bool,
	shifted Token,
	fork ForkOrder,
) (Head, error) {
	node, err := c.node(head.Node)
	if err != nil {
		return Head{}, err
	}
	if shifted.Symbol != lookahead {
		return Head{}, fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", shifted.Symbol, lookahead)
	}
	if shifted.Extra != extra {
		return Head{}, fmt.Errorf("parser-core phase zero: token extra=%t disagrees with compiled action extra=%t", shifted.Extra, extra)
	}
	if extra && targetState == 0 {
		targetState = node.state
	}
	payload, err := c.appendAuthenticatedTerminal(subtreeRecord{
		symbol: shifted.Symbol, startByte: shifted.StartByte, endByte: shifted.EndByte,
		extra: extra, external: shifted.External, terminal: true,
	}, shifted.LexerSkippedPrefixLength)
	if err != nil {
		return Head{}, err
	}
	phase0AObserveTerminalShift(c, payload, head.Node, targetState, shifted.EndByte, true, extra, 0, fork)
	outcome, err := c.condenseWithOutcomeAtomic(c.shiftedBoundaryKey(targetState, shifted.EndByte), linkInput{
		prev: head.Node, payload: payload, order: fork,
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

// shiftOrdinaryClassifiedCohortPrepared computes shift targets for one
// ordinary cohort and performs the shift. It hoists the former
// runSchedulerMaybeLiveScopedOwned closure body into a named, directly
// callable method so the dispatch site below never builds a func value.
func (c *Core) shiftOrdinaryClassifiedCohortPrepared(boundaries []ClassifiedBoundary, shifted Token) ([]Head, error) {
	var inlineTargets [inlineSchedulerCohortTargets]StateID
	targets := inlineTargets[:]
	if len(boundaries) > len(targets) {
		targets = make([]StateID, len(boundaries))
	} else {
		targets = targets[:len(boundaries)]
	}
	if err := c.prepareOrdinaryClassifiedCohortInto(boundaries, shifted, targets); err != nil {
		return nil, err
	}
	return c.shiftOrdinaryClassifiedCohortUncheckpointed(boundaries, targets, shifted)
}

// shiftOrdinaryClassifiedCohortMaybeLiveScopedOwned inlines the former
// runSchedulerMaybeLiveScopedOwned/runLiveCondenseCandidates closure chain;
// see shiftClassifiedMaybeLiveScopedOwned's doc comment for the equivalence
// argument, which applies identically here.
func (c *Core) shiftOrdinaryClassifiedCohortMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if !liveScoped {
		out, err = c.shiftOrdinaryClassifiedCohortPrepared(boundaries, shifted)
		return out, c.finishSchedulerOwned(owner, err)
	}
	if err = c.enterLiveCondenseCandidates(candidates); err != nil {
		return out, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		out, err = c.shiftOrdinaryClassifiedCohortPrepared(boundaries, shifted)
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	out, err = c.shiftOrdinaryClassifiedCohortPrepared(boundaries, shifted)
	return out, c.finishSchedulerOwned(owner, err)
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
	}, shifted.LexerSkippedPrefixLength)
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

// shiftExtraClassifiedCohortPrepared computes shift targets for one extra
// cohort and performs the shift. It hoists the former
// runSchedulerMaybeLiveScopedOwned closure body into a named, directly
// callable method so the dispatch site below never builds a func value.
func (c *Core) shiftExtraClassifiedCohortPrepared(boundaries []ClassifiedBoundary, shifted Token) ([]Head, error) {
	var inlineTargets [inlineSchedulerCohortTargets]StateID
	targets := inlineTargets[:]
	if len(boundaries) > len(targets) {
		targets = make([]StateID, len(boundaries))
	} else {
		targets = targets[:len(boundaries)]
	}
	if err := c.prepareExtraClassifiedCohortInto(boundaries, shifted, targets); err != nil {
		return nil, err
	}
	return c.shiftExtraClassifiedCohortUncheckpointed(boundaries, targets, shifted)
}

// shiftExtraClassifiedCohortMaybeLiveScopedOwned inlines the former
// runSchedulerMaybeLiveScopedOwned/runLiveCondenseCandidates closure chain;
// see shiftClassifiedMaybeLiveScopedOwned's doc comment for the equivalence
// argument, which applies identically here.
func (c *Core) shiftExtraClassifiedCohortMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, boundaries []ClassifiedBoundary, shifted Token) (out []Head, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return out, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if !liveScoped {
		out, err = c.shiftExtraClassifiedCohortPrepared(boundaries, shifted)
		return out, c.finishSchedulerOwned(owner, err)
	}
	if err = c.enterLiveCondenseCandidates(candidates); err != nil {
		return out, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		out, err = c.shiftExtraClassifiedCohortPrepared(boundaries, shifted)
		c.clearLiveCondenseCandidates()
		return out, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	out, err = c.shiftExtraClassifiedCohortPrepared(boundaries, shifted)
	return out, c.finishSchedulerOwned(owner, err)
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
	}, shifted.LexerSkippedPrefixLength)
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
	return c.reduceOutputsClassifiedIntoMaybeLiveScopedOwned(owner, nil, false, dst, boundary, actionOrdinal, fork, nil)
}

// ReduceOutputsClassifiedIntoWithCostOwned applies one standalone reduction
// and authenticates its cumulative recovery cost before publication.
func (c *Core) ReduceOutputsClassifiedIntoWithCostOwned(
	owner SchedulerTransactionToken,
	dst []ReductionOutput,
	boundary ClassifiedBoundary,
	actionOrdinal int,
	fork ForkOrder,
	cost ReductionOutputCostFunc,
) (frontier []ReductionOutput, err error) {
	if cost == nil {
		if err = c.beginSchedulerOwned(owner); err != nil {
			return frontier, err
		}
		return frontier, c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: reduction cost callback is required"))
	}
	return c.reduceOutputsClassifiedIntoMaybeLiveScopedOwned(
		owner, nil, false, dst, boundary, actionOrdinal, fork, cost,
	)
}

// ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned scopes the condense candidates
// and reduces under one scheduler ownership check.
func (c *Core) ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	return c.reduceOutputsClassifiedIntoMaybeLiveScopedOwned(owner, candidates, true, dst, boundary, actionOrdinal, fork, nil)
}

// ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesAndCostOwned applies
// one reduction while authenticating output costs before any boundary can
// fold or publish.
func (c *Core) ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
	owner SchedulerTransactionToken,
	candidates []CondenseCandidate,
	dst []ReductionOutput,
	boundary ClassifiedBoundary,
	actionOrdinal int,
	fork ForkOrder,
	cost ReductionOutputCostFunc,
) (frontier []ReductionOutput, err error) {
	if cost == nil {
		if err = c.beginSchedulerOwned(owner); err != nil {
			return frontier, err
		}
		return frontier, c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: reduction cost callback is required"))
	}
	return c.reduceOutputsClassifiedIntoMaybeLiveScopedOwned(
		owner, candidates, true, dst, boundary, actionOrdinal, fork, cost,
	)
}

// ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned applies
// one validated corridor reduction without revalidating its classification.
// The caller must pass a current boundary from ClassifyBoundaryWithRow and a
// sole reduce row from a validated corridor program.
func (c *Core) ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned(
	owner SchedulerTransactionToken,
	candidates []CondenseCandidate,
	dst []ReductionOutput,
	boundary ClassifiedBoundary,
	fork ForkOrder,
) (frontier []ReductionOutput, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return frontier, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.enterLiveCondenseCandidates(candidates); err != nil {
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		frontier, err = c.reduceOutputsCorridorClassifiedIntoUncheckpointed(owner, dst, boundary, fork)
		c.clearLiveCondenseCandidates()
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	frontier, err = c.reduceOutputsCorridorClassifiedIntoUncheckpointed(owner, dst, boundary, fork)
	return frontier, c.finishSchedulerOwned(owner, err)
}

// ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesAndCostOwned
// is the corridor form of the authenticated reduction-cost seam.
func (c *Core) ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
	owner SchedulerTransactionToken,
	candidates []CondenseCandidate,
	dst []ReductionOutput,
	boundary ClassifiedBoundary,
	fork ForkOrder,
	cost ReductionOutputCostFunc,
) (frontier []ReductionOutput, err error) {
	if cost == nil {
		if err = c.beginSchedulerOwned(owner); err != nil {
			return frontier, err
		}
		return frontier, c.finishSchedulerOwned(owner, errors.New("parser-core phase zero: reduction cost callback is required"))
	}
	if err = c.beginSchedulerOwned(owner); err != nil {
		return frontier, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if err = c.enterLiveCondenseCandidatesWithCost(candidates, true); err != nil {
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		frontier, err = c.reduceOutputsCorridorClassifiedIntoUncheckpointedWithCost(owner, dst, boundary, fork, cost)
		c.clearLiveCondenseCandidates()
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	frontier, err = c.reduceOutputsCorridorClassifiedIntoUncheckpointedWithCost(owner, dst, boundary, fork, cost)
	return frontier, c.finishSchedulerOwned(owner, err)
}

// reduceOutputsClassifiedIntoMaybeLiveScopedOwned inlines the former
// runSchedulerMaybeLiveScopedOwned/runLiveCondenseCandidates closure chain;
// see shiftClassifiedMaybeLiveScopedOwned's doc comment for the equivalence
// argument, which applies identically here.
func (c *Core) reduceOutputsClassifiedIntoMaybeLiveScopedOwned(owner SchedulerTransactionToken, candidates []CondenseCandidate, liveScoped bool, dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder, cost ReductionOutputCostFunc) (frontier []ReductionOutput, err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return frontier, err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	if !liveScoped {
		frontier, err = c.reduceOutputsClassifiedIntoUncheckpointed(owner, dst, boundary, actionOrdinal, fork, cost)
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	if err = c.enterLiveCondenseCandidatesWithCost(candidates, cost != nil); err != nil {
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	if c.schedulerFrame.fresh {
		frontier, err = c.reduceOutputsClassifiedIntoUncheckpointed(owner, dst, boundary, actionOrdinal, fork, cost)
		c.clearLiveCondenseCandidates()
		return frontier, c.finishSchedulerOwned(owner, err)
	}
	defer c.clearLiveCondenseCandidates()
	frontier, err = c.reduceOutputsClassifiedIntoUncheckpointed(owner, dst, boundary, actionOrdinal, fork, cost)
	return frontier, c.finishSchedulerOwned(owner, err)
}

func (c *Core) reduceOutputsClassifiedIntoUncheckpointed(owner SchedulerTransactionToken, dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder, cost ReductionOutputCostFunc) ([]ReductionOutput, error) {
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
	return c.reduceOutputsClassifiedIntoActive(owner, frontier, boundary, actionOrdinal, fork, false, cost)
}

func (c *Core) historicalImportEnvelope(owner SchedulerTransactionToken, refs DropCohortRefSet, historicalNode NodeID) (int, bool) {
	if c == nil || historicalNode == 0 || c.validateSchedulerTransaction(owner) != nil || refs.Empty() || refs.Overflowed() || refs.Blended() {
		return 0, false
	}
	if _, err := c.nodeLineage(historicalNode); err != nil {
		return 0, false
	}
	count, valid := c.dropCohortRefCount(refs)
	if !valid || count == 0 {
		return 0, false
	}
	storeBytes, addErr := dropCohortAddChecked(c.dropCohortStoreBytes(), c.dropCohortReservedBytes)
	if addErr != nil || storeBytes > c.limits.MaxDropCohortBytes {
		return 0, false
	}
	return count, true
}

func (c *Core) historicalImportRef(ref DropCohortRef, historicalNode NodeID, expected DropCohortActionIdentity) bool {
	if ref.Owner != c.dropCohortOwner || ref.Epoch != c.dropCohortEpoch || ref.Sequence == 0 {
		return false
	}
	recordIndex, handle, reason := c.dropCohortVerifierLookup(ref, false)
	if reason != dropCohortVerifierProved || recordIndex < 0 || recordIndex >= len(c.dropCohortRecords) {
		return false
	}
	record := c.dropCohortRecords[recordIndex]
	if record.handle != handle || dropCohortVerifierClassifyRecord(record) != dropCohortVerifierProved {
		return false
	}
	member, memberReason := c.dropCohortVerifierMember(record, Head{Node: historicalNode}, ref.Branch)
	if memberReason != dropCohortVerifierProved || member.action != expected {
		return false
	}
	return c.historicalImportDerivation(member.derivation, member.head, historicalNode)
}

func (c *Core) historicalImportDerivation(derivation DropCohortDerivationHandle, memberHead Head, historicalNode NodeID) bool {
	if derivation.Owner != c.dropCohortOwner || derivation.Epoch != c.dropCohortEpoch || derivation.Index == 0 ||
		uint64(derivation.Index) > uint64(len(c.dropCohortDerivations)) {
		return false
	}
	record := c.dropCohortDerivations[derivation.Index-1]
	if record.handle != derivation || record.head != memberHead || record.head.Node != historicalNode ||
		uint64(record.byteLength) > c.limits.MaxDropCohortBytes {
		return false
	}
	start := uint64(record.byteOffset)
	length := uint64(record.byteLength)
	if start > uint64(len(c.dropCohortDerivationBytes)) || length > uint64(len(c.dropCohortDerivationBytes))-start {
		return false
	}
	end := int(start + length)
	bytes := c.dropCohortDerivationBytes[int(start):end]
	return sha256.Sum256(bytes) == record.digest && c.historicalImportDerivationInterned(record)
}

func (c *Core) historicalImportDerivationInterned(record dropCohortDerivationRecord) bool {
	for _, entry := range c.dropCohortDerivationIntern {
		if entry.digest == record.digest && entry.byteOffset == record.byteOffset && entry.byteLength == record.byteLength {
			return true
		}
	}
	return false
}

// authenticateHistoricalDropCohortImport validates one dead-node reference
// set under the active scheduler owner before historical state can be reused.
// It reads only immutable certificate records and never publishes verifier
// diagnostics or changes the production drop route.
func (c *Core) authenticateHistoricalDropCohortImport(
	owner SchedulerTransactionToken,
	refs DropCohortRefSet,
	historicalNode NodeID,
	expected DropCohortActionIdentity,
) bool {
	count, ok := c.historicalImportEnvelope(owner, refs, historicalNode)
	if !ok {
		return false
	}
	for index := 0; index < count; index++ {
		ref, ok := c.DropCohortRefAt(refs, index)
		if !ok || !c.historicalImportRef(ref, historicalNode, expected) {
			return false
		}
	}
	return true
}

func (c *Core) reduceOutputsCorridorClassifiedIntoUncheckpointed(owner SchedulerTransactionToken, dst []ReductionOutput, boundary ClassifiedBoundary, fork ForkOrder) ([]ReductionOutput, error) {
	return c.reduceOutputsCorridorClassifiedIntoUncheckpointedWithCost(owner, dst, boundary, fork, nil)
}

func (c *Core) reduceOutputsCorridorClassifiedIntoUncheckpointedWithCost(owner SchedulerTransactionToken, dst []ReductionOutput, boundary ClassifiedBoundary, fork ForkOrder, cost ReductionOutputCostFunc) ([]ReductionOutput, error) {
	frontier := dst[:0]
	if c.popScratch.busy {
		return nil, errors.New("parser-core phase zero: reentrant reduction while pop scratch is active")
	}
	c.popScratch.busy = true
	defer c.popScratch.resetLogical()
	c.reductionScratch.begin()
	defer c.reductionScratch.finish()
	return c.reduceOutputsClassifiedIntoActive(owner, frontier, boundary, 0, fork, true, cost)
}

func (c *Core) reduceOutputsClassifiedIntoActive(owner SchedulerTransactionToken, frontier []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder, corridorTrusted bool, cost ReductionOutputCostFunc) ([]ReductionOutput, error) {
	var act *Action
	var err error
	if corridorTrusted {
		if boundary.actions.Len() != 1 {
			return nil, errors.New("parser-core phase zero: corridor reduction requires one action")
		}
		act = boundary.actions.actionRef(0)
	} else {
		act, err = c.classifiedActionRef(boundary, actionOrdinal)
		if err != nil {
			return nil, err
		}
	}
	if act.Type != ActionReduce {
		return nil, fmt.Errorf("parser-core phase zero: action %d is %v, not reduce", actionOrdinal, act.Type)
	}
	expectedHistoricalAction := DropCohortActionIdentity{
		BoundaryState: boundary.state,
		Lookahead:     boundary.lookahead,
		ActionOrdinal: int32(actionOrdinal),
		Action:        *act,
		NoLookahead:   c.reduceNoLookaheadContext,
		Selection:     c.dropCohortSelectionContext,
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
		// Stage 2b (spec.b4b-alternative-set.v2 section 8): the v2
		// containment predicate is the deciding proof and never reads
		// cleanPathRank/cleanPathLineage on the routing path, so the
		// prefix-DAG walk (markCleanProductionRank, the bisected dominant
		// share of the recording-era compact-perf regression) runs only
		// when the three-proof census asks for a meaningful scalar
		// comparison. Otherwise every path is marked Unknown directly --
		// cheap, and still marks nodeLineageRecord.converged=true, so
		// historical-boundary classification and alternative-set import are
		// unaffected (see cleanPathRankWalkEnabled's doc comment).
		if cleanPathRankWalkEnabled() {
			c.markCleanProductionRank(paths)
		} else {
			markCleanPathRankUnknown(paths)
		}
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
		if cost != nil {
			storedErrorCost, costErr := cost(path.prev, payload)
			if costErr != nil {
				return nil, costErr
			}
			parentLink.storedErrorCost = storedErrorCost
			parentLink.hasStoredErrorCost = true
		}
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
			if cost != nil {
				storedErrorCost, costErr := cost(out.Node, trailing.payload)
				if costErr != nil {
					return nil, costErr
				}
				extraLink.storedErrorCost = storedErrorCost
				extraLink.hasStoredErrorCost = true
			}
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
		historicalBoundarySplit := previous.historicalBoundarySplit || outcome.historicalBoundarySplit
		historicalConvergedSplit := previous.historicalConvergedSplit
		historicalForestDeterministic := previous.historicalForestDeterministic
		historicalCleanPathRank := previous.historicalCleanPathRank
		historicalLineage := previous.historicalLineage
		historicalSet := previous.historicalSet
		historicalBlended := previous.historicalBlended
		dropCohortRefs := previous.dropCohortRefs
		if source, refsErr := c.nodeLineage(path.prev); refsErr == nil {
			if _, refsErr = c.dropCohortRefUnion(&dropCohortRefs, source.dropCohortRefs); refsErr != nil {
				return nil, refsErr
			}
		} else {
			return nil, refsErr
		}
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
			// Membership is never invalidated: where the scalar pair above may
			// poison to Unknown/0 on disagreement between multiple historical
			// versions landing on the same boundary, the alternative set instead
			// unions every converged historical version's recorded set
			// (spec.b4b-alternative-set.v1 section 4, "Dead-node historical
			// import"). The dead node's lineage record persists for the rest of
			// the parse. Fold-class union (spec.b4b-alternative-set.v2 section
			// 3.4): two differing dead sets unioning into one output, or a dead
			// record that was itself blended, mark this boundary's accumulated
			// historicalSet blended -- computed before the union mutates it.
			if outcome.historicalConvergedSplit {
				if !c.historicalCertificateAuthentication {
					// Preserve the pre-D2 historical union and counter behavior.
					if dead, err := c.nodeLineage(outcome.historicalNode); err == nil {
						incomparable := c.AlternativeSetIncomparable(historicalSet, dead.set)
						unionChanged := c.alternativeSetUnion(&historicalSet, dead.set)
						wasBlended := historicalBlended
						historicalBlended = historicalBlended || dead.blended || incomparable
						if unionChanged || historicalBlended != wasBlended {
							c.addDropCohortProducerWrite(dropCohortProducerDeadHistoryImport)
							if c.dropCohortAuthenticatedHistory != math.MaxUint64 {
								c.dropCohortAuthenticatedHistory++
							}
						}
					}
					if _, refsErr := c.dropCohortRefUnion(&dropCohortRefs, outcome.historicalDropCohortRefs); refsErr != nil {
						return nil, refsErr
					}
				} else {
					markUnproved := func() {
						historicalConvergedSplit = false
						historicalForestDeterministic = false
						historicalCleanPathRank = CleanPathRankUnknown
						historicalLineage = 0
						historicalSet = AlternativeSet{}
						historicalBlended = false
						// Do not retain source or historical certificate refs on an
						// unproved result.
						dropCohortRefs = DropCohortRefSet{}
						if c.dropCohortUnprovedHistory != math.MaxUint64 {
							c.dropCohortUnprovedHistory++
						}
					}
					if !c.authenticateHistoricalDropCohortImport(
						owner, outcome.historicalDropCohortRefs, outcome.historicalNode, expectedHistoricalAction,
					) {
						markUnproved()
					} else if dead, err := c.nodeLineage(outcome.historicalNode); err != nil {
						markUnproved()
					} else {
						c.addDropCohortProducerWrite(dropCohortProducerDeadHistoryImport)
						if c.dropCohortAuthenticatedHistory != math.MaxUint64 {
							c.dropCohortAuthenticatedHistory++
						}
						incomparable := c.AlternativeSetIncomparable(historicalSet, dead.set)
						historicalBlended = historicalBlended || dead.blended || incomparable
						c.alternativeSetUnion(&historicalSet, dead.set)
						if _, refsErr := c.dropCohortRefUnion(&dropCohortRefs, outcome.historicalDropCohortRefs); refsErr != nil {
							return nil, refsErr
						}
					}
				}
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
		linkChain, err := c.reductionLinkChainForHead(out)
		if err != nil {
			return nil, err
		}
		scratch.store(boundaryIndex, seen, reductionBoundaryOutput{
			key: key, head: out, links: linkChain, freshness: freshness, cleanPathRank: cleanPathRank,
			dropCohortRefs:                dropCohortRefs,
			historicalBoundarySplit:       historicalBoundarySplit,
			historicalConvergedSplit:      historicalConvergedSplit,
			historicalForestDeterministic: historicalForestDeterministic,
			historicalCleanPathRank:       historicalCleanPathRank,
			historicalLineage:             historicalLineage,
			historicalSet:                 historicalSet,
			historicalBlended:             historicalBlended,
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
			Links:                        output.links,
			DropCohortRefs:               output.dropCohortRefs,
			Freshness:                    output.freshness,
			CleanPathRank:                output.cleanPathRank,
			MultiplePopPaths:             multiPop,
			HistoricalBoundaryProvenance: historicalProvenance,
			HistoricalCleanPathRank:      output.historicalCleanPathRank,
			HistoricalCleanPathLineage:   output.historicalLineage,
			HistoricalAlternativeSet:     output.historicalSet,
			HistoricalBlended:            output.historicalBlended,
		})
	}
	phase0AFinishReductionConstruction(c)
	return frontier, nil
}
