package parsercorephase0

import "errors"

// nodeDropCohortRefSet resolves immutable, one-based lineage metadata. Zero
// denotes the empty set, so ordinary stack records carry no inline reference
// array. Reference-bearing records and rollback journals share the immutable
// value; a union publishes a new value rather than changing either owner's set.
func (c *Core) nodeDropCohortRefSet(id uint32) DropCohortRefSet {
	if id == 0 {
		return DropCohortRefSet{}
	}
	return c.nodeDropCohortRefs[id-1]
}

func (c *Core) unionNodeDropCohortRefs(id uint32, incoming DropCohortRefSet) (uint32, bool, error) {
	set := c.nodeDropCohortRefSet(id)
	spillMark := len(c.dropCohortRefSpill)
	changed, err := c.dropCohortRefUnion(&set, incoming)
	if err != nil || !changed {
		return id, false, err
	}
	if uint64(len(c.nodeDropCohortRefs))+1 > uint64(c.limits.MaxMetadata) {
		c.dropCohortRefSpill = c.dropCohortRefSpill[:spillMark]
		return id, false, errors.New("parser-core phase zero: node lineage reference arena cap")
	}
	c.nodeDropCohortRefs = append(c.nodeDropCohortRefs, set)
	return uint32(len(c.nodeDropCohortRefs)), true, nil
}
