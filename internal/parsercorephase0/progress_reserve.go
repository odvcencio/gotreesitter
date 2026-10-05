package parsercorephase0

// GrowRecordArenasForProgress projects record density from an authenticated
// source prefix. Unlike the initial source heuristic, this uses records the
// parse has actually constructed. It changes only capacity. The byte ceiling
// includes the existing footprint AND replacement allocations, so the old
// arrays can remain live during copying without exceeding the caller's budget.
func (c *Core) GrowRecordArenasForProgress(sourceBytes, progressBytes uint32, maxBytes uint64) bool {
	if c == nil || progressBytes == 0 || progressBytes >= sourceBytes || maxBytes == 0 {
		return false
	}
	project := func(count int, limit uint32) int {
		value := (uint64(count)*uint64(sourceBytes) + uint64(progressBytes) - 1) / uint64(progressBytes)
		// A small margin accommodates prefix density variation without the
		// repeated growth and discarded backing arrays of append.
		value += (value + 15) / 16
		value = min(value, uint64(limit), uint64(^uint32(0)>>1))
		return int(value)
	}
	nodes := project(len(c.nodes), c.limits.MaxNodes)
	links := project(len(c.links), c.limits.MaxLinks)
	subtrees := project(len(c.subtrees), c.limits.MaxSubtrees)
	children := project(len(c.children), c.limits.MaxChildren)
	var additional uint64
	add := func(want, capacity int, size uint64) {
		if want > capacity {
			additional += uint64(want) * size
		}
	}
	add(nodes, cap(c.nodes), coreNodeRecordBytes)
	if c.sharedLineages {
		add(nodes, cap(c.nodeOwners), coreUint32Bytes)
		add(nodes, cap(c.nodeLineageRefs), coreUint32Bytes)
	} else {
		add(nodes, cap(c.nodeLineages), coreNodeLineageRecordBytes)
	}
	add(links, cap(c.links), coreLinkRecordBytes)
	add(subtrees, cap(c.subtrees), coreSubtreeRecordBytes)
	if len(c.recoveryVisibleCounts) != 0 {
		add(subtrees, cap(c.recoveryVisibleCounts), coreRecoveryVisibleCountBytes)
	}
	add(children, cap(c.children), coreChildRecordBytes)
	footprint := c.FootprintBytes()
	if additional == 0 || footprint >= maxBytes || additional >= maxBytes-footprint {
		return false
	}
	c.nodes = growArena(c.nodes, nodes)
	c.reserveLineages(nodes, true)
	c.links = growArena(c.links, links)
	c.subtrees = growArena(c.subtrees, subtrees)
	if len(c.recoveryVisibleCounts) != 0 {
		c.recoveryVisibleCounts = growArena(c.recoveryVisibleCounts, subtrees)
	}
	c.children = growArena(c.children, children)
	return true
}
