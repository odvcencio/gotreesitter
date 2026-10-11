package parsercorephase0

import (
	"errors"
	"math"
	"sync/atomic"
)

// Each appended slot has an identity that survives neither rollback nor
// reset/reuse. A view authenticates its last slot: a retained prefix remains
// valid when extended, but a rolled-back extension cannot alias new contents
// published at the same index. Global identities also reject foreign cores.
type dropCohortRefRecord struct {
	ref      DropCohortRef
	identity uint64
}

var dropCohortRefIdentityCounter atomic.Uint64

func reserveDropCohortRefIdentities(counter *atomic.Uint64, count uint64) (uint64, error) {
	for {
		current := counter.Load()
		if count == 0 || count > math.MaxUint64-current {
			return 0, errors.New("parser-core phase zero: drop-cohort reference identity overflow")
		}
		if counter.CompareAndSwap(current, current+count) {
			return current + 1, nil
		}
	}
}

func (c *Core) appendDropCohortRefs(refs []DropCohortRef) error {
	if err := c.dropCohortRefPreflight(len(refs)); err != nil {
		return err
	}
	first, err := reserveDropCohortRefIdentities(&dropCohortRefIdentityCounter, uint64(len(refs)))
	if err != nil {
		return err
	}
	for index, ref := range refs {
		c.dropCohortRefSpill = append(c.dropCohortRefSpill, dropCohortRefRecord{
			ref: ref, identity: first + uint64(index),
		})
	}
	return nil
}

func (c *Core) dropCohortRefSuffixEqual(start int, refs []DropCohortRef) bool {
	for index, ref := range refs {
		if c.dropCohortRefSpill[start+index].ref != ref {
			return false
		}
	}
	return true
}
