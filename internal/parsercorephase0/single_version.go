package parsercorephase0

import "errors"

// reductionPopPaths handles a one-child, non-extra pop directly when the
// scheduler has no live condense candidate. Scores and order come from the one
// link; there are no reverse paths or trailing extras to collect. All other
// shapes retain the ordinary enumeration and its ordering.
func (c *Core) reductionPopPaths(head NodeID, childCount int) ([]popPath, error) {
	if childCount != 1 || !c.condenseScopeActive || len(c.condenseCandidates) != 0 {
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
