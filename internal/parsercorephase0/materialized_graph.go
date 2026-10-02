package parsercorephase0

import "errors"

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
	c.nodes = nil
	c.nodeLineages = nil
	c.nodeDropCohortRefs = nil
	c.nodeCheckpoints = nil
	c.links = nil
	c.nodeLineageJournal = nil
	c.boundaryJournal = nil
	c.boundaries.reset()
	c.boundaries.dropOversized()
	return nil
}
