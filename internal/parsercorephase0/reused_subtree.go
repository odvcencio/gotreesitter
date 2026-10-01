package parsercorephase0

import "errors"

// ReusedSubtree describes one clean public nonterminal authenticated by the scheduler.
// Key identifies the same immutable public node throughout this core generation.
// The scheduler authenticates source bytes, node identity and scanner boundary
// states. An opaque descriptor supplies no scanner proof; ScannerExact requires
// both interned endpoints, including checkpoint zero for the empty state.
type ReusedSubtree struct {
	Key                      uint32
	Symbol                   Symbol
	PreGotoState, State      StateID
	StartByte, EndByte       uint32
	DynamicPrecedence        int32
	ScannerStart, ScannerEnd CheckpointID
	ScannerExact             bool
}

type reusedSubtreeProvenance struct {
	payload    SubtreeID
	descriptor ReusedSubtree
}

type reuseValidationProof struct {
	subtrees uint32
	nodes    uint32
	invalid  bool
}

// PushReusedSubtreeOwned publishes one opaque nonterminal through its authenticated goto.
// Recovery and mutations that change the authenticated ancestry still decline.
func (c *Core) PushReusedSubtreeOwned(owner SchedulerTransactionToken, head Head, reused ReusedSubtree) (out Head, payload SubtreeID, err error) {
	return c.PushReusedSubtreeOwnedWithPoll(owner, head, reused, nil)
}

// PushReusedSubtreeOwnedWithPoll checks cancellation while validating newly allocated records.
// Keys must increase strictly. The allocated corridor must remain error-free
// with exact clean graph ancestry. Fresh fragility does not certify an old candidate.
func (c *Core) PushReusedSubtreeOwnedWithPoll(owner SchedulerTransactionToken, head Head, reused ReusedSubtree, poll func() error) (out Head, payload SubtreeID, err error) {
	err = c.RunSchedulerOwned(owner, func() error {
		node, err := c.node(head.Node)
		if err != nil {
			return err
		}
		if reused.Key == 0 || reused.Symbol == 0 || reused.Symbol >= ErrorRegionSymbol-1 ||
			reused.StartByte >= reused.EndByte || reused.StartByte < node.byteOffset ||
			node.state != reused.PreGotoState || reused.State == 0 || c.reduceConflictContext ||
			!c.metadataConstructionAuthenticated {
			return errors.New("parser-core phase zero: invalid reused nonterminal boundary")
		}
		target, err := c.tables.Goto(reused.PreGotoState, reused.Symbol)
		if err != nil {
			return err
		}
		if target != reused.State {
			return errors.New("parser-core phase zero: reused nonterminal goto mismatch")
		}
		if len(c.reusedSubtrees) != 0 && reused.Key <= c.reusedSubtrees[len(c.reusedSubtrees)-1].descriptor.Key {
			return errors.New("parser-core phase zero: reused keys must increase strictly")
		}
		if err := c.validateReusedHead(head, poll); err != nil {
			return err
		}
		payload, err = c.appendSubtreeRecord(subtreeRecord{
			symbol: reused.Symbol, startByte: reused.StartByte, endByte: reused.EndByte,
		}, nil, nil, nil)
		if err != nil {
			return err
		}
		c.subtrees[payload-1].externalProvenanceState = subtreeExternalProvenanceReusedOpaque
		if reused.ScannerExact {
			if _, _, ok := c.checkpoints.receipt(reused.ScannerStart); !ok {
				return errors.New("parser-core phase zero: missing borrowed scanner start")
			}
			if _, _, ok := c.checkpoints.receipt(reused.ScannerEnd); !ok {
				return errors.New("parser-core phase zero: missing borrowed scanner end")
			}
			c.subtrees[payload-1].externalProvenanceState = subtreeExternalProvenanceReusedExact
			c.recordScannerBoundary(payload, reused.ScannerStart, reused.ScannerEnd)
		}
		c.reusedSubtrees = append(c.reusedSubtrees, reusedSubtreeProvenance{payload: payload, descriptor: reused})
		out, err = c.appendPrivate(reused.State, reused.EndByte, linkInput{
			prev: head.Node, payload: payload, scoreDelta: int64(reused.DynamicPrecedence),
		})
		return err
	})
	if err != nil {
		return Head{}, 0, err
	}
	return out, payload, nil
}

// SubtreeReadBoundary exposes a record's physical end without copying its
// children. The scheduler records the lookahead that justified its reduction.
func (c *Core) SubtreeReadBoundary(id SubtreeID) (uint32, error) {
	record, err := c.subtree(id)
	if err != nil {
		return 0, err
	}
	return record.endByte, nil
}

func (c *Core) validateReusedHead(head Head, poll func() error) error {
	if c.reuseProof.invalid {
		return errors.New("parser-core phase zero: reused corridor proof was invalidated")
	}
	if _, err := c.node(head.Node); err != nil {
		return err
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return err
	}
	work := uint32(0)
	step := func() error {
		work++
		if work&127 == 0 {
			return poll()
		}
		return nil
	}
	// Child identifiers precede parents. Each completed prefix therefore proves all descendants.
	for uint64(c.reuseProof.subtrees) < uint64(len(c.subtrees)) {
		if err := step(); err != nil {
			return err
		}
		id := SubtreeID(c.reuseProof.subtrees + 1)
		r, err := c.subtree(id)
		if err != nil {
			return err
		}
		// Fragility of a freshly reduced prefix does not block reuse: C keys
		// reuse on the version's state and on the candidate subtree's own
		// metadata, never on how the prefix was reduced. A fragile terminal
		// payload still blocks reuse; no production path marks a terminal
		// fragile today (reductionParentForPath and markSubtreeFragile only
		// touch reduce parents), so that clause is the rule the fixture in
		// TestReusedSubtreeCleanExternalAncestorRequiresQuiescence encodes.
		// The graph checks below validate every retained clean path.
		if r.missing || (r.fragile && r.terminal) || r.symbol >= ErrorRegionSymbol-1 {
			return errors.New("parser-core phase zero: reused head contains an unclean payload")
		}
		if r.external && (!r.terminal || !c.externalPayloadsQuiescent && !c.reusedPrefixScannerExact(id)) {
			return errors.New("parser-core phase zero: reused head requires certified quiescent external tokens")
		}
		childEnd := uint64(r.firstChild) + uint64(r.childCount)
		if childEnd > uint64(len(c.children)) {
			return errors.New("parser-core phase zero: reused head has invalid child window")
		}
		for _, child := range c.children[r.firstChild:childEnd] {
			if err := step(); err != nil {
				return err
			}
			if child == 0 || child >= id {
				return errors.New("parser-core phase zero: reused head has invalid child order")
			}
		}
		c.reuseProof.subtrees++
	}
	// Graph topology is immutable. Mutation seams invalidate proofs of unsafe lineage changes.
	for uint64(c.reuseProof.nodes) < uint64(len(c.nodes)) {
		if err := step(); err != nil {
			return err
		}
		id := NodeID(c.reuseProof.nodes + 1)
		node := &c.nodes[id-1]
		lineage, err := c.nodeLineage(id)
		if err != nil {
			return err
		}
		if lineage.storedErrorCost != 0 {
			return errors.New("parser-core phase zero: reused prefix contains recovery")
		}
		if node.linkCount == 0 {
			if node.firstLink != 0 || node.pathCount != 1 {
				return errors.New("parser-core phase zero: reused prefix has invalid seed adjacency")
			}
		} else {
			paths := uint64(0)
			linkID := node.firstLink
			for count := uint32(0); count < node.linkCount; count++ {
				if err := step(); err != nil {
					return err
				}
				if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
					return errors.New("parser-core phase zero: reused prefix has invalid link identifier")
				}
				link := c.links[linkID-1]
				if err := link.validateShape(); err != nil {
					return err
				}
				if link.isRecoveryDiscontinuity() || link.prev == 0 || link.prev >= id ||
					link.payload == 0 || uint64(link.payload) > uint64(c.reuseProof.subtrees) {
					return errors.New("parser-core phase zero: reused prefix has invalid ancestry")
				}
				paths = saturatingAddPaths(paths, c.nodes[link.prev-1].pathCount)
				linkID = uint32(link.next)
			}
			if linkID != 0 || paths != node.pathCount {
				return errors.New("parser-core phase zero: reused prefix has invalid path count")
			}
		}
		c.reuseProof.nodes++
	}
	return poll()
}

func reuseLineageClean(lineage *nodeLineageRecord) bool {
	return lineage.storedErrorCost == 0 && !lineage.blended && !lineage.converged && lineage.set.count == 0 && lineage.lineage == 0
}

func (c *Core) invalidateReusedSubtreeProof(id SubtreeID) {
	if id != 0 && uint64(id) <= uint64(c.reuseProof.subtrees) {
		c.reuseProof.invalid = true
	}
}

func (c *Core) markSubtreeFragile(id SubtreeID) {
	if record, err := c.subtree(id); err == nil && !record.fragile {
		record.fragile = true
		c.invalidateReusedSubtreeProof(id)
	}
}

func (c *Core) invalidateReusedLineageProof(id NodeID, lineage *nodeLineageRecord) {
	if id != 0 && uint64(id) <= uint64(c.reuseProof.nodes) && !reuseLineageClean(lineage) {
		c.reuseProof.invalid = true
	}
}

func (c *Core) reusedSubtree(id SubtreeID) (ReusedSubtree, bool) {
	low, high := 0, len(c.reusedSubtrees)
	for low < high {
		mid := low + (high-low)/2
		if c.reusedSubtrees[mid].payload < id {
			low = mid + 1
		} else {
			high = mid
		}
	}
	if low == len(c.reusedSubtrees) || c.reusedSubtrees[low].payload != id {
		return ReusedSubtree{}, false
	}
	return c.reusedSubtrees[low].descriptor, true
}

func (c *Core) validateReusedRecord(id SubtreeID, r subtreeRecord) error {
	reused, ok := c.reusedSubtree(id)
	if !ok || reused.Key == 0 || reused.Symbol != r.symbol || reused.StartByte != r.startByte ||
		reused.EndByte != r.endByte || r.startByte >= r.endByte || r.terminal || r.extra ||
		r.external || r.missing || r.fragile || r.childCount != 0 || r.fieldCount != 0 ||
		r.aliasCount != 0 || r.productionID != 0 || r.dynamicPrecedence != 0 {
		return errors.New("parser-core phase zero: reused nonterminal provenance is stale")
	}
	return nil
}

func (c *Core) applyReusedMaterializationView(id SubtreeID, view *MaterializationSubtreeView) {
	if len(c.reusedSubtrees) == 0 {
		return
	}
	if reused, ok := c.reusedSubtree(id); ok {
		view.ReusedKey = reused.Key
		view.ReusedPreGotoState, view.ReusedState = reused.PreGotoState, reused.State
		view.DynamicPrecedence = reused.DynamicPrecedence
		view.ExternalScannerCheckpointExact = reused.ScannerExact
		view.ExternalScannerCheckpointStart, view.ExternalScannerCheckpointEnd = reused.ScannerStart, reused.ScannerEnd
	}
}

func (c *Core) claimReusedOwnership(id SubtreeID, owners map[uint32]SubtreeID) error {
	if reused, ok := c.reusedSubtree(id); ok {
		if owners[reused.Key] != 0 {
			return errors.New("parser-core phase zero: reused node has repeated public-tree ownership")
		}
		owners[reused.Key] = id
	}
	return nil
}

// Fresh external terminals preceding a borrow must own a complete core pair.
func (c *Core) reusedPrefixScannerExact(id SubtreeID) bool {
	pair, ok := c.externalPayloadScannerProvenance(id)
	if !ok {
		return false
	}
	_, _, startOK := c.checkpoints.receipt(pair.start)
	_, _, endOK := c.checkpoints.receipt(pair.end)
	return startOK && endOK
}
