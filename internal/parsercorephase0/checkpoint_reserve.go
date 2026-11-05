package parsercorephase0

// growCheckpointBuffersFromObservedDensity reserves only storage capacity.
// The complete existing record arenas and checkpoint storage share maxBytes;
// a proposal that cannot fit is declined before any allocation. Identities,
// collision chains, receipt digests, limits and serialized bytes do not change.
func (c *Core) growCheckpointBuffersFromObservedDensity(sourceBytes int, progress uint32, maxBytes uint64) bool {
	if c == nil || sourceBytes <= 1 || uint64(sourceBytes) > uint64(^uint32(0)) || progress == 0 || uint64(progress) >= uint64(sourceBytes) || uint64(progress) < uint64(sourceBytes/16) {
		return false
	}
	i := &c.checkpoints
	if len(i.records) == 0 || len(i.bytes) == 0 {
		return false
	}
	project := func(count int, limit uint64) int {
		value := (uint64(min(count, 1<<31-1))*uint64(sourceBytes) + uint64(progress) - 1) / uint64(progress)
		value = (value*17 + 15) / 16
		return int(min(value, limit, uint64(1<<31-1)))
	}
	records := project(len(i.records), uint64(i.maxIDs))
	bytes := project(len(i.bytes), min(i.maxBytes, uint64(^uint32(0))))
	recordDelta := max(records-cap(i.records), 0)
	byteDelta := max(bytes-cap(i.bytes), 0)
	additional := uint64(recordDelta)*coreCheckpointRecordBytes + uint64(byteDelta)
	if additional == 0 {
		return false
	}
	if maxBytes > 0 {
		current := c.nodes.FootprintBytes() + uint64(cap(c.nodeLineages))*coreNodeLineageRecordBytes + c.links.FootprintBytes() + c.subtrees.FootprintBytes() + uint64(cap(c.children))*coreChildRecordBytes + i.footprintBytes()
		if current >= maxBytes || additional > maxBytes-current {
			return false
		}
	}
	i.records = growArena(i.records, records)
	i.bytes = growArena(i.bytes, bytes)
	return true
}
