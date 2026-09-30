package parsercorephase0

import "errors"

// reductionPopPaths handles short, non-extra pops directly when the scheduler
// has no live condense candidate. One child needs no reverse scratch; ordinary
// chains of two to four children use a fixed buffer. Other shapes retain the
// ordinary enumeration and its ordering.
func (c *Core) reductionPopPaths(head NodeID, childCount int) ([]popPath, error) {
	if !c.condenseScopeActive || len(c.condenseCandidates) != 0 {
		return c.popPaths(head, childCount)
	}
	if childCount >= 2 && childCount <= 4 {
		return c.shortSingleVersionPop(head, childCount)
	}
	if childCount != 1 {
		return c.popPaths(head, childCount)
	}
	node, err := c.node(head)
	if err != nil {
		return nil, err
	}
	count := uint64(node.linkCount)
	if count > uint64(c.limits.MaxLinks) || count > uint64(c.limits.MaxLinksPerBoundary) {
		return nil, errors.New("parser-core phase zero: recorded link count exceeds configured limit")
	}
	if count > uint64(len(c.links)) {
		return nil, errors.New("parser-core phase zero: recorded link count exceeds link arena")
	}
	if node.linkCount != 1 {
		return c.popPaths(head, childCount)
	}
	if node.firstLink == 0 {
		return nil, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
	}
	if uint64(node.firstLink) > uint64(len(c.links)) {
		return nil, errors.New("parser-core phase zero: link adjacency out of range")
	}
	link := c.links[node.firstLink-1]
	if link.next != 0 {
		return nil, errors.New("parser-core phase zero: adjacency exceeds recorded link count or cycles")
	}
	if link.prev == 0 || link.prev >= head {
		return nil, errors.New("parser-core phase zero: graph predecessor does not decrease")
	}
	if link.isRecoveryDiscontinuity() {
		return c.popPaths(head, childCount)
	}
	payload, err := c.subtree(link.payload)
	if err != nil {
		return nil, err
	}
	if payload.extra {
		return c.popPaths(head, childCount)
	}
	if c.limits.MaxPopPaths == 0 {
		return nil, errors.New("parser-core phase zero: pop enumeration cap")
	}
	scratch := &c.popScratch
	scratch.begin()
	path := scratch.nextPath()
	path.prev = link.prev
	path.children = append(path.children, link.payload)
	path.score = link.scoreDelta
	if link.hasOrder() {
		path.order = ForkOrder{Value: link.order, Present: true}
	}
	path.startByte = payload.startByte
	path.structuralEnd = payload.endByte
	return scratch.paths, nil
}

// singleVersionReductionOutput publishes the sole pop result directly. With
// no live condense candidate and no historical boundary there is nothing to aggregate:
// freshness comes from this one condensation, and historical provenance stays
// empty. Parent construction, extra migration, and graph publication have
// already used the ordinary paths, including their limits and transactions.
func (c *Core) singleVersionReductionOutput(frontier []ReductionOutput, path *popPath, head Head, change condenseChange) ([]ReductionOutput, error) {
	source, err := c.nodeLineage(path.prev)
	if err != nil {
		return nil, err
	}
	var refs DropCohortRefSet
	if _, err := c.dropCohortRefUnion(&refs, source.dropCohortRefs); err != nil {
		return nil, err
	}
	links, err := c.reductionLinkChainForHead(head)
	if err != nil {
		return nil, err
	}
	freshness := ReductionUnchanged
	switch change {
	case condenseNew:
		freshness = ReductionNew
	case condenseUpdated:
		freshness = ReductionUpdated
	}
	frontier = append(frontier, ReductionOutput{
		Head: head, Links: links, DropCohortRefs: refs,
		Freshness: freshness, CleanPathRank: path.cleanPathRank,
	})
	phase0AFinishReductionConstruction(c)
	return frontier, nil
}

// shortSingleVersionPop collects a short, ordinary chain once instead of
// constructing three reverse scratch slices and rereading every payload.
func (c *Core) shortSingleVersionPop(head NodeID, childCount int) ([]popPath, error) {
	var links [4]linkRecord
	id := head
	var startByte, endByte uint32
	for index := 0; index < childCount; index++ {
		node, err := c.node(id)
		if err != nil {
			return nil, err
		}
		count := uint64(node.linkCount)
		if count > uint64(c.limits.MaxLinks) || count > uint64(c.limits.MaxLinksPerBoundary) {
			return nil, errors.New("parser-core phase zero: recorded link count exceeds configured limit")
		}
		if count > uint64(len(c.links)) {
			return nil, errors.New("parser-core phase zero: recorded link count exceeds link arena")
		}
		if node.linkCount != 1 {
			return c.popPaths(head, childCount)
		}
		if node.firstLink == 0 {
			return nil, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
		if uint64(node.firstLink) > uint64(len(c.links)) {
			return nil, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[node.firstLink-1]
		if link.next != 0 {
			return nil, errors.New("parser-core phase zero: adjacency exceeds recorded link count or cycles")
		}
		if link.prev == 0 || link.prev >= id {
			return nil, errors.New("parser-core phase zero: graph predecessor does not decrease")
		}
		if link.isRecoveryDiscontinuity() {
			return c.popPaths(head, childCount)
		}
		payload, err := c.subtree(link.payload)
		if err != nil {
			return nil, err
		}
		if payload.extra {
			return c.popPaths(head, childCount)
		}
		if index == 0 {
			endByte = payload.endByte
		}
		if index == childCount-1 {
			startByte = payload.startByte
			if endByte == 0 {
				endByte = payload.endByte
			}
		}
		links[index] = link
		id = link.prev
	}
	if c.limits.MaxPopPaths == 0 {
		return nil, errors.New("parser-core phase zero: pop enumeration cap")
	}
	scratch := &c.popScratch
	scratch.begin()
	path := scratch.nextPath()
	path.prev, path.startByte, path.structuralEnd = id, startByte, endByte
	for index := childCount - 1; index >= 0; index-- {
		link := links[index]
		path.children = append(path.children, link.payload)
		var err error
		path.score, err = checkedAddScore(path.score, link.scoreDelta)
		if err != nil {
			scratch.paths = scratch.paths[:0]
			return nil, err
		}
		if link.hasOrder() {
			path.order = ForkOrder{Value: link.order, Present: true}
		}
	}
	return scratch.paths, nil
}
