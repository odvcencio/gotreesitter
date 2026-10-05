package parsercorephase0

// Keep the index descriptor behind one pointer, as the previous map was.
// Recovery shadows copy both the descriptor and its mutable slots.
type sharedLineageIndex struct {
	slots   []uint32
	entries int
}

func (c *Core) sharedLineageSlots() []uint32 {
	if c.nodeLineageIntern == nil {
		return nil
	}
	return c.nodeLineageIntern.slots
}

func (c *Core) sharedLineageEntries() int {
	if c.nodeLineageIntern == nil {
		return 0
	}
	return c.nodeLineageIntern.entries
}

// The index stores references into the immutable history arena, rather than
// duplicating every history in a map key. Hash collisions always compare the
// complete record. A power-of-two table stays at most half full.
func sharedLineageHash(record nodeLineageRecord) uint64 {
	hash := uint64(14695981039346656037)
	add := func(value uint32) { hash = (hash ^ uint64(value)) * 1099511628211 }
	add(record.owner)
	add(record.storedErrorCost)
	add(record.dropCohortRefs)
	for _, member := range record.set.inline {
		add(member)
	}
	add(record.set.spillRef)
	add(uint32(record.set.count) | uint32(record.set.flags)<<8 | uint32(record.lineage)<<16)
	add(uint32(record.rank))
	if record.converged {
		add(1)
	} else {
		add(0)
	}
	if record.blended {
		add(1)
	} else {
		add(0)
	}
	// Mix high bits into low bits before masking by the table capacity.
	hash ^= hash >> 33
	hash *= 0xff51afd7ed558ccd
	hash ^= hash >> 33
	return hash
}

func (c *Core) lookupSharedLineage(record nodeLineageRecord) uint32 {
	if len(c.sharedLineageSlots()) == 0 {
		return 0
	}
	mask := uint64(len(c.sharedLineageSlots()) - 1)
	for slot := sharedLineageHash(record) & mask; ; slot = (slot + 1) & mask {
		reference := c.nodeLineageIntern.slots[slot]
		if reference == 0 || c.nodeLineages[reference-1] == record {
			return reference
		}
	}
}

func (c *Core) insertSharedLineage(reference uint32) {
	if 2*(c.sharedLineageEntries()+1) > len(c.sharedLineageSlots()) {
		previous := c.sharedLineageSlots()
		if c.nodeLineageIntern == nil {
			c.nodeLineageIntern = new(sharedLineageIndex)
		}
		c.nodeLineageIntern.slots = make([]uint32, max(16, 2*len(previous)))
		for _, oldReference := range previous {
			if oldReference != 0 {
				c.placeSharedLineage(oldReference)
			}
		}
	}
	c.placeSharedLineage(reference)
	c.nodeLineageIntern.entries++
}

func (c *Core) placeSharedLineage(reference uint32) {
	mask := uint64(len(c.sharedLineageSlots()) - 1)
	for slot := sharedLineageHash(c.nodeLineages[reference-1]) & mask; ; slot = (slot + 1) & mask {
		if c.nodeLineageIntern.slots[slot] == 0 {
			c.nodeLineageIntern.slots[slot] = reference
			return
		}
	}
}

// Rollback removes a newly appended record and closes its probe-chain hole.
// Reinserting the following cluster preserves lookup without tombstones or
// an allocation, including when the cluster wraps around the table.
func (c *Core) deleteSharedLineage(record nodeLineageRecord) {
	mask := uint64(len(c.sharedLineageSlots()) - 1)
	slot := sharedLineageHash(record) & mask
	for {
		reference := c.nodeLineageIntern.slots[slot]
		if reference == 0 {
			return
		}
		if c.nodeLineages[reference-1] == record {
			break
		}
		slot = (slot + 1) & mask
	}
	c.nodeLineageIntern.slots[slot] = 0
	c.nodeLineageIntern.entries--
	for slot = (slot + 1) & mask; c.nodeLineageIntern.slots[slot] != 0; slot = (slot + 1) & mask {
		reference := c.nodeLineageIntern.slots[slot]
		c.nodeLineageIntern.slots[slot] = 0
		c.placeSharedLineage(reference)
	}
}
