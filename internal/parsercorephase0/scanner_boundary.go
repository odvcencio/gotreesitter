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
	if n := len(c.externalProvenance); n != 0 {
		last := &c.externalProvenance[n-1]
		if last.payload+1 == id && last.start == start && last.end == end {
			last.payload = id
			return
		}
	}
	c.externalProvenance = append(c.externalProvenance, scannerBoundaryRun{externalPayloadProvenance: externalPayloadProvenance{payload: id, start: start, end: end}, first: id})
}

// Extending a run changes only its endpoint. An extension across a
// transaction boundary requires that endpoint to equal the previous subtree
// count: otherwise the new payload leaves a gap and starts a separate run.
// Clamping to the saved subtree count therefore restores the exact endpoint
// without adding fields to every scheduler checkpoint.
func (c *Core) restoreScannerProvenanceRows(count int, subtrees SubtreeID) {
	c.externalProvenance = c.externalProvenance[:count]
	if count != 0 {
		c.externalProvenance[count-1].payload = min(c.externalProvenance[count-1].payload, subtrees)
	}
}

func (c *Core) lastExternalProvenanceRun() scannerBoundaryRun {
	if len(c.externalProvenance) == 0 {
		return scannerBoundaryRun{}
	}
	return c.externalProvenance[len(c.externalProvenance)-1]
}
