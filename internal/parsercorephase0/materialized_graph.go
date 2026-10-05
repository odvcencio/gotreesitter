package parsercorephase0

import (
	"errors"
	"runtime"
)

// ReleaseStackGraphForMaterialization ends the stack graph's lifetime after
// the scheduler has authenticated and copied its accepted payload IDs and
// census. The syntax records remain live for construction and state replay.
// The caller must retain no graph ID and must Reset before scheduling again.
// Small graphs keep their warm capacity under the existing retention cap.
func (c *Core) ReleaseStackGraphForMaterialization() error {
	if c == nil || c.FootprintBytes() <= coreRetentionCapBytes {
		return nil
	}
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: materialization cannot release an active stack graph")
	}
	before := c.FootprintBytes()
	c.nodes = nil
	c.nodeLineages = nil
	c.releaseSharedLineages()
	c.nodeDropCohortRefs = nil
	c.nodeCheckpoints = nil
	c.links = nil
	c.nodeLineageJournal = nil
	c.boundaryJournal = nil
	c.boundaries.reset()
	c.boundaries.dropOversized()
	// Collect the discarded large graph before allocating a public tree.
	// Otherwise lower allocation churn can postpone collection until both
	// generations and the public tree overlap at the heap peak.
	if c.FootprintBytes() < before {
		runtime.GC()
	}
	return nil
}
