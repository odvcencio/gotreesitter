package parsercorephase0

import "errors"

// ReusedSubtree describes one clean public nonterminal authenticated by the scheduler.
// Key identifies the same immutable public node throughout this core generation.
// The scheduler authenticates source bytes, node identity, and a stateless scanner.
// Borrowed descendants may contain external tokens. This descriptor certifies no checkpoints.
type ReusedSubtree struct {
	Key                 uint32
	Symbol              Symbol
	PreGotoState, State StateID
	StartByte, EndByte  uint32
	DynamicPrecedence   int32
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
// The scheduler must decline conflicts and recovery after this operation.
func (c *Core) PushReusedSubtreeOwned(owner SchedulerTransactionToken, head Head, reused ReusedSubtree) (out Head, payload SubtreeID, err error) {
	return c.PushReusedSubtreeOwnedWithPoll(owner, head, reused, nil)
}

// PushReusedSubtreeOwnedWithPoll checks cancellation while validating newly allocated records.
// Keys must increase strictly. The complete allocated corridor must remain clean.
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
		if r.missing || r.fragile || r.symbol >= ErrorRegionSymbol-1 {
			return errors.New("parser-core phase zero: reused head contains an unclean payload")
		}
		if r.external && (!r.terminal || !c.externalPayloadsQuiescent) {
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
		if node.pathCount != 1 || node.linkCount > 1 || !reuseLineageClean(*lineage) {
			return errors.New("parser-core phase zero: reuse requires one clean exact corridor")
		}
		if node.linkCount == 0 {
			if node.firstLink != 0 {
				return errors.New("parser-core phase zero: reused corridor has invalid seed adjacency")
			}
		} else {
			if node.firstLink == 0 || uint64(node.firstLink) > uint64(len(c.links)) {
				return errors.New("parser-core phase zero: reused corridor has invalid link identifier")
			}
			link := c.links[node.firstLink-1]
			if err := link.validateShape(); err != nil {
				return err
			}
			if link.next != 0 || link.isRecoveryDiscontinuity() || link.hasOrder() || link.prev == 0 || link.prev >= id ||
				link.payload == 0 || uint64(link.payload) > uint64(c.reuseProof.subtrees) {
				return errors.New("parser-core phase zero: reused corridor has invalid or ambiguous ancestry")
			}
		}
		c.reuseProof.nodes++
	}
	return poll()
}

func reuseLineageClean(lineage nodeLineageRecord) bool {
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

func (c *Core) invalidateReusedLineageProof(id NodeID, lineage nodeLineageRecord) {
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
	if reused, ok := c.reusedSubtree(id); ok {
		view.ReusedKey = reused.Key
		view.ReusedPreGotoState, view.ReusedState = reused.PreGotoState, reused.State
		view.DynamicPrecedence = reused.DynamicPrecedence
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
