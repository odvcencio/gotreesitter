package parsercorephase0

import "errors"

var errInvalidNodeLineage = errors.New("parser-core phase zero: invalid node lineage identifier")

type nodeLineageView struct {
	*nodeLineageHot
	nodeLineageRecovery
}

func (v nodeLineageView) record() nodeLineageRecord {
	return nodeLineageRecord{owner: v.owner, storedErrorCost: v.storedErrorCost, dropCohortRefIndex: v.dropCohortRefIndex, set: v.set, lineage: v.lineage, rank: v.rank, converged: v.isConverged(), blended: v.isBlended()}
}
func (c *Core) nodeLineageRecoveryAt(id NodeID) nodeLineageRecovery {
	if uint64(id)-1 >= uint64(len(c.nodeLineageRecovery)) {
		return nodeLineageRecovery{}
	}
	return c.nodeLineageRecovery[id-1]
}
func (c *Core) lineageView(id NodeID) (nodeLineageView, error) {
	if uint64(id)-1 >= uint64(len(c.nodeLineages)) {
		return nodeLineageView{}, errInvalidNodeLineage
	}
	return nodeLineageView{nodeLineageHot: &c.nodeLineages[id-1], nodeLineageRecovery: c.nodeLineageRecoveryAt(id)}, nil
}

// Only nonzero recovery data grows this array; untouched nodes read as zero.
// Callers authenticate the node before writing. Extension always zeroes stale
// retained elements, and transaction rollback truncates the length while retained capacity stays charged.
func (c *Core) writeNodeLineageRecovery(id NodeID, value nodeLineageRecovery) {
	if uint64(id)-1 >= uint64(len(c.nodeLineageRecovery)) {
		if value == (nodeLineageRecovery{}) {
			return
		}
		if int(id) > cap(c.nodeLineageRecovery) {
			next := make([]nodeLineageRecovery, int(id), cap(c.nodeLineages))
			copy(next, c.nodeLineageRecovery)
			c.nodeLineageRecovery = next
		} else {
			old := len(c.nodeLineageRecovery)
			c.nodeLineageRecovery = c.nodeLineageRecovery[:id]
			clear(c.nodeLineageRecovery[old:])
		}
	}
	c.nodeLineageRecovery[id-1] = value
}
func (c *Core) publishNodeLineage(id NodeID, value nodeLineageRecord) error {
	if uint64(id)-1 >= uint64(len(c.nodeLineages)) {
		return errInvalidNodeLineage
	}
	hot := nodeLineageHot{owner: value.owner, set: value.set, lineage: value.lineage, rank: value.rank}
	hot.setConverged(value.converged)
	hot.setBlended(value.blended)
	c.nodeLineages[id-1] = hot
	c.writeNodeLineageRecovery(id, nodeLineageRecovery{storedErrorCost: value.storedErrorCost, dropCohortRefIndex: value.dropCohortRefIndex})
	return nil
}

const (
	nodeLineageConverged uint8 = 1 << iota
	nodeLineageBlended
)

func (n *nodeLineageHot) isConverged() bool { return n.flags&nodeLineageConverged != 0 }
func (n *nodeLineageHot) isBlended() bool   { return n.flags&nodeLineageBlended != 0 }
func (n *nodeLineageHot) setConverged(value bool) {
	n.flags &^= nodeLineageConverged
	if value {
		n.flags |= nodeLineageConverged
	}
}
func (n *nodeLineageHot) setBlended(value bool) {
	n.flags &^= nodeLineageBlended
	if value {
		n.flags |= nodeLineageBlended
	}
}
