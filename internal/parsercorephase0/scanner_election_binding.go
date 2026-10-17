package parsercorephase0

import (
	"errors"
	"math"
)

// ErrUnknownScannerCheckpoint identifies a scanner election whose checkpoint
// IDs do not belong to the current interner contents.
var ErrUnknownScannerCheckpoint = errors.New("parser-core phase zero: unknown checkpoint identity")

// BindScannerElection validates both core-local checkpoint IDs and binds their
// exact pair in one operation. Identity errors precede transaction errors, as
// they do for a scheduler that resolves both receipts before binding. No user
// code runs between validation and mutation.
func (c *Core) BindScannerElection(start, end CheckpointID) error {
	if c == nil || uint64(start) > uint64(len(c.checkpoints.records)) ||
		uint64(end) > uint64(len(c.checkpoints.records)) {
		return ErrUnknownScannerCheckpoint
	}
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: set checkpoint during active transaction")
	}
	return c.setPhaseExternalTokenScannerCheckpoints(start, end)
}

// Both callers validate the pair and reject active transactions first.
func (c *Core) setPhaseExternalTokenScannerCheckpoints(start, end CheckpointID) error {
	if c.checkpoint == end && c.externalTokenScannerExact &&
		c.externalTokenScannerStart == start && c.externalTokenScannerEnd == end {
		return nil
	}
	if c.classificationPhase == math.MaxUint64 {
		return errors.New("parser-core phase zero: classification phase overflow")
	}
	c.classificationPhase++
	c.checkpoint = end
	c.externalTokenScannerStart = start
	c.externalTokenScannerEnd = end
	c.externalTokenScannerExact = true
	return nil
}
