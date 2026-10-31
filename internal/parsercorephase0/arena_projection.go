package parsercorephase0

// ProjectRecordArenas takes one late capacity estimate from work already done.
// It never changes a live record or a limit. maxAdditionalBytes bounds the
// total increase in retained capacity; zero means no caller ceiling is armed.
func (c *Core) ProjectRecordArenas(sourceBytes int, progress uint32, maxAdditionalBytes uint64) bool {
	if c == nil || sourceBytes <= 0 || uint64(sourceBytes) > uint64(^uint32(0)) || progress == 0 || uint64(progress) >= uint64(sourceBytes) || uint64(progress)*2 < uint64(sourceBytes) {
		return false
	}
	project := func(count int, limit uint32) int {
		value := (uint64(min(count, 1<<31-1))*uint64(sourceBytes) + uint64(progress) - 1) / uint64(progress)
		value = (value*17 + 15) / 16
		value = min(value, uint64(limit), uint64(1<<31-1))
		return int(value)
	}
	nodes := project(c.nodes.Len(), c.limits.MaxNodes)
	links := project(c.links.Len(), c.limits.MaxLinks)
	subtrees := project(c.subtrees.Len(), c.limits.MaxSubtrees)
	children := project(len(c.children), c.limits.MaxChildren)
	delta := func(want, capacity int, size uint64) uint64 {
		if want <= capacity {
			return 0
		}
		return uint64(want-capacity) * size
	}
	additional := c.nodes.GrowthFootprintBytes(nodes) + delta(nodes, cap(c.nodeLineages), coreNodeLineageRecordBytes) + c.links.GrowthFootprintBytes(links) + c.subtrees.GrowthFootprintBytes(subtrees) + delta(children, cap(c.children), coreChildRecordBytes)
	if additional == 0 || (maxAdditionalBytes > 0 && additional > maxAdditionalBytes) {
		return false
	}
	c.nodes.Grow(nodes)
	c.nodeLineages = growArena(c.nodeLineages, nodes)
	c.links.Grow(links)
	c.subtrees.Grow(subtrees)
	c.children = growArena(c.children, children)
	return true
}

// RecordArenaProjectionStart returns the first byte at or beyond halfway.
// Invalid or unrepresentable source lengths cannot arm a projection.
func RecordArenaProjectionStart(sourceBytes int) uint32 {
	if sourceBytes <= 1 || uint64(sourceBytes) > uint64(^uint32(0)) {
		return 0
	}
	return uint32(sourceBytes/2 + sourceBytes%2)
}

// RecordArenaProjectionBudget leaves at least three quarters of every armed
// limit's remaining capacity for later parse work. No armed limit means no
// caller ceiling. A spent limit refuses projection instead of treating zero
// remaining bytes as an unlimited ceiling.
func RecordArenaProjectionBudget(footprint uint64, budgetBytes, ceilingBytes int64) (uint64, bool) {
	available := uint64(0)
	for _, limit := range [2]int64{budgetBytes, ceilingBytes} {
		if limit <= 0 {
			continue
		}
		if uint64(limit) <= footprint {
			return 0, false
		}
		remaining := (uint64(limit) - footprint) / 4
		if remaining == 0 {
			return 0, false
		}
		if available == 0 || remaining < available {
			available = remaining
		}
	}
	return available, true
}
