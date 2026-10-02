package parsercorephase0

// Reduction boundaries include hidden and zero-width children. Public child
// projection can erase an indentation or delimiter transition, so it cannot
// authenticate the scanner state at the end of a borrowed nonterminal.
func (c *Core) recordReductionScannerBoundary(id SubtreeID, r subtreeRecord, children []SubtreeID) {
	if !c.terminalScannerCheckpointProvenance || r.terminal || len(children) == 0 {
		return
	}
	var start, end CheckpointID
	found := false
	for _, child := range children {
		pair, ok := c.externalPayloadScannerProvenance(child)
		if !ok {
			if child == 0 || child >= id {
				return
			}
			record := c.subtrees[child-1]
			if record.terminal || record.childCount != 0 || record.startByte != record.endByte {
				return
			}
			continue // an epsilon reduction performs no scan
		}
		if !found {
			start, found = pair.start, true
		}
		end = pair.end
	}
	if found {
		c.recordScannerBoundary(id, start, end)
	}
}

// Empty interned checkpoints are an exact pair only after the scanner boundary
// was authenticated. Pack that presence bit into derived metadata; absence of
// a sidecar row alone must never authenticate an empty scanner state.
const subtreeScannerEmptyPair subtreeExternalProvenanceState = 1 << 7

func (c *Core) recordScannerBoundary(id SubtreeID, start, end CheckpointID) {
	if start == 0 && end == 0 {
		c.subtrees[id-1].externalProvenanceState |= subtreeScannerEmptyPair
		return
	}
	c.externalProvenance = append(c.externalProvenance, externalPayloadProvenance{payload: id, start: start, end: end})
}
