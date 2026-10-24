package parsercorephase0

// GrowRecordArenasFromObservedDensity projects the live record counts at the
// first staged reserve. Unlike a fixed source density, this keeps the observed
// mixture of graph and syntax records. It changes capacity only. The complete
// existing record arenas, checkpoint storage and every proposed increase fit
// maxBytes when
// that ceiling is armed; no live records or limits change.
func (c *Core) GrowRecordArenasFromObservedDensity(sourceBytes int, progress uint32, maxBytes uint64) bool {
	if c == nil || sourceBytes <= 1 || uint64(sourceBytes) > uint64(^uint32(0)) || progress == 0 || uint64(progress) >= uint64(sourceBytes) || uint64(progress) < uint64(sourceBytes/16) {
		return false
	}
	grewCheckpoints := c.growCheckpointBuffersFromObservedDensity(sourceBytes, progress, maxBytes)
	project := func(count int, limit uint32) int {
		value := (uint64(min(count, 1<<31-1))*uint64(sourceBytes) + uint64(progress) - 1) / uint64(progress)
		value = (value*17 + 15) / 16
		return int(min(value, uint64(limit), uint64(1<<31-1)))
	}
	nodes := project(c.nodes.Len(), c.limits.MaxNodes)
	links := project(c.links.Len(), c.limits.MaxLinks)
	subtrees := project(c.subtrees.Len(), c.limits.MaxSubtrees)
	children := project(len(c.children), c.limits.MaxChildren)
	dn := max(nodes-c.nodes.Capacity(), 0)
	dl := max(nodes-cap(c.nodeLineages), 0)
	de := max(links-c.links.Cap(), 0)
	ds := max(subtrees-c.subtrees.Capacity(), 0)
	dc := max(children-cap(c.children), 0)
	additionalBytes := func() uint64 {
		return c.nodes.GrowthFootprintBytes(c.nodes.Capacity()+dn) + uint64(dl)*coreNodeLineageRecordBytes + c.links.GrowthFootprintBytes(c.links.Cap()+de) + c.subtrees.GrowthFootprintBytes(c.subtrees.Capacity()+ds) + uint64(dc)*coreChildRecordBytes
	}
	additional := additionalBytes()
	if additional == 0 {
		return grewCheckpoints
	}
	if maxBytes > 0 {
		current := c.nodes.FootprintBytes() + uint64(cap(c.nodeLineages))*coreNodeLineageRecordBytes + c.links.FootprintBytes() + c.subtrees.FootprintBytes() + uint64(cap(c.children))*coreChildRecordBytes + c.checkpoints.footprintBytes()
		if current >= maxBytes {
			return grewCheckpoints
		}
		allowed := maxBytes - current
		if additional > allowed {
			dn = scaleReserveCount(dn, allowed, additional)
			dl = scaleReserveCount(dl, allowed, additional)
			de = scaleReserveCount(de, allowed, additional)
			ds = scaleReserveCount(ds, allowed, additional)
			dc = scaleReserveCount(dc, allowed, additional)
			// Block pointer tables do not scale linearly with slots.
			// Decline a proposal that still exceeds the ceiling before allocation.
			if dn|dl|de|ds|dc == 0 || additionalBytes() > allowed {
				return grewCheckpoints
			}
		}
	}
	c.nodes.Grow(c.nodes.Capacity() + dn)
	c.nodeLineages = growArena(c.nodeLineages, cap(c.nodeLineages)+dl)
	c.links.Grow(c.links.Cap() + de)
	c.subtrees.Grow(c.subtrees.Capacity() + ds)
	c.children = growArena(c.children, cap(c.children)+dc)
	return true
}
