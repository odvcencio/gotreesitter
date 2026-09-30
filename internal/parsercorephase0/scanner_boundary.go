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
		c.externalProvenance = append(c.externalProvenance, externalPayloadProvenance{payload: id, start: start, end: end})
	}
}
