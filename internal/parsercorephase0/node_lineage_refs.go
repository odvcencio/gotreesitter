package parsercorephase0

import (
	"errors"
	"math"
)

// nodeLineageRefs resolves an optional, one-based reference-set index. Published
// sets are immutable: updating a node appends a replacement and journals only the
// old index. Copies of lineage records can therefore share their reference set.
func (c *Core) nodeLineageRefs(index uint32) (DropCohortRefSet, error) {
	if index == 0 {
		return DropCohortRefSet{}, nil
	}
	if c == nil || uint64(index) > uint64(len(c.nodeLineageRefSets)) {
		return DropCohortRefSet{}, errors.New("parser-core phase zero: invalid lineage reference index")
	}
	return c.nodeLineageRefSets[index-1], nil
}

func (c *Core) appendNodeLineageRefs(refs DropCohortRefSet) (uint32, error) {
	if refs == (DropCohortRefSet{}) {
		return 0, nil
	}
	if uint64(len(c.nodeLineageRefSets)) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: lineage reference index overflow")
	}
	c.nodeLineageRefSets = append(c.nodeLineageRefSets, refs)
	return uint32(len(c.nodeLineageRefSets)), nil
}
