package parsercorephase0

import "errors"

// reductionTerminalScannerExact authenticates one cursor candidate's complete
// scanner pair against the live interner. The caller never reuses this result
// beyond the matching terminal pair or across callbacks.
func (c *Core) reductionTerminalScannerExact(id SubtreeID) (bool, error) {
	provenance, exact := c.externalPayloadScannerProvenance(id)
	if !exact {
		return false, nil
	}
	for _, checkpoint := range [...]CheckpointID{provenance.start, provenance.end} {
		if checkpoint != 0 {
			if _, ok := c.checkpoints.record(checkpoint); !ok {
				return false, errors.New("parser-core phase zero: reduction terminal checkpoint is unavailable")
			}
		}
	}
	return true, nil
}
