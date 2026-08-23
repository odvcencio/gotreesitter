package parsercorephase0

import (
	"errors"
	"math"
	"unsafe"
)

// DropCohortRef identifies one branch of one drop cohort. The owner, epoch,
// and sequence identify the cohort; the branch identifies one valid member.
type DropCohortRef struct {
	Owner    uint64
	Epoch    uint64
	Sequence uint64
	Branch   uint16
}

const (
	dropCohortRefInlineCapacity = 2
	dropCohortRefHardCap        = 32

	dropCohortRefFlagOverflowed uint8 = 1 << iota
	dropCohortRefFlagSpilled
	dropCohortRefFlagBlended
)

// DropCohortRefSet is an ordered, value-owned reference set. It keeps two
// references inline and stores wider sets in Core.dropCohortRefSpill.
type DropCohortRefSet struct {
	Inline [dropCohortRefInlineCapacity]DropCohortRef
	Count  uint8
	Flags  uint8
	Spill  uint32 // one-based start index in Core.dropCohortRefSpill
}

func (s DropCohortRefSet) Empty() bool { return s.Count == 0 }

// Len reports the number of recorded references. Overflowed sets contain a
// sound subset of the true references and must not prove a drop.
func (s DropCohortRefSet) Len() int { return int(s.Count) }

func (s DropCohortRefSet) Overflowed() bool { return s.Flags&dropCohortRefFlagOverflowed != 0 }
func (s DropCohortRefSet) Spilled() bool    { return s.Flags&dropCohortRefFlagSpilled != 0 }
func (s DropCohortRefSet) Blended() bool    { return s.Flags&dropCohortRefFlagBlended != 0 }

func dropCohortRefLess(a, b DropCohortRef) bool {
	if a.Owner != b.Owner {
		return a.Owner < b.Owner
	}
	if a.Epoch != b.Epoch {
		return a.Epoch < b.Epoch
	}
	if a.Sequence != b.Sequence {
		return a.Sequence < b.Sequence
	}
	return a.Branch < b.Branch
}

// Add inserts an inline reference. Core-owned sets use AddDropCohortRef so
// they can spill without exposing an arena pointer in this value type.
func (s *DropCohortRefSet) Add(ref DropCohortRef) bool {
	if s == nil || ref == (DropCohortRef{}) || s.Overflowed() {
		return false
	}
	for i := 0; i < int(s.Count) && i < len(s.Inline); i++ {
		if s.Inline[i] == ref {
			return false
		}
	}
	if s.Count >= uint8(len(s.Inline)) {
		return false
	}
	index := int(s.Count)
	s.Inline[index] = ref
	s.Count++
	for index > 0 && dropCohortRefLess(s.Inline[index], s.Inline[index-1]) {
		s.Inline[index], s.Inline[index-1] = s.Inline[index-1], s.Inline[index]
		index--
	}
	return true
}

func (c *Core) dropCohortRefCount(set DropCohortRefSet) (int, bool) {
	if c == nil || !set.Spilled() {
		return int(set.Count), int(set.Count) <= len(set.Inline)
	}
	if set.Spill == 0 {
		return int(set.Count), set.Count == 0
	}
	start := int(set.Spill) - 1
	end := start + int(set.Count)
	if start < 0 || end < start || end > len(c.dropCohortRefSpill) {
		return 0, false
	}
	return int(set.Count), true
}

func (c *Core) dropCohortRefAt(set DropCohortRefSet, index int) (DropCohortRef, bool) {
	if index < 0 || index >= int(set.Count) {
		return DropCohortRef{}, false
	}
	if !set.Spilled() {
		if index >= len(set.Inline) {
			return DropCohortRef{}, false
		}
		return set.Inline[index], true
	}
	if c == nil || set.Spill == 0 {
		return DropCohortRef{}, false
	}
	position := int(set.Spill) - 1 + index
	if position < 0 || position >= len(c.dropCohortRefSpill) {
		return DropCohortRef{}, false
	}
	return c.dropCohortRefSpill[position], true
}

// DropCohortRefAt returns one sorted reference without exposing an inline
// slice. Callers can enumerate into fixed storage without a heap escape.
func (c *Core) DropCohortRefAt(set DropCohortRefSet, index int) (DropCohortRef, bool) {
	return c.dropCohortRefAt(set, index)
}

// DropCohortRefAtOwned reads one reference only under the active scheduler
// token. It permits bounded spill access without an unowned store read.
func (c *Core) DropCohortRefAtOwned(owner SchedulerTransactionToken, set DropCohortRefSet, index int) (DropCohortRef, bool) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return DropCohortRef{}, false
	}
	return c.dropCohortRefAt(set, index)
}

// dropCohortRefSpillLimit returns the configured reference count ceiling.
func (c *Core) dropCohortRefSpillLimit() uint64 {
	if c == nil {
		return 0
	}
	return uint64(c.limits.MaxDropCohortRefs)
}

func (c *Core) dropCohortRefPreflight(additional int) error {
	if additional <= 0 {
		return nil
	}
	if uint64(len(c.dropCohortRefSpill))+uint64(additional) > c.dropCohortRefSpillLimit() {
		return errors.New("parser-core phase zero: drop-cohort reference spill cap")
	}
	bytes := uint64(additional) * uint64(unsafe.Sizeof(DropCohortRef{}))
	if bytes > c.limits.MaxDropCohortRefBytes ||
		uint64(len(c.dropCohortRefSpill))*uint64(unsafe.Sizeof(DropCohortRef{})) > c.limits.MaxDropCohortRefBytes-bytes {
		return errors.New("parser-core phase zero: drop-cohort reference byte cap")
	}
	if uint64(len(c.dropCohortRefSpill)) > uint64(math.MaxUint32)-uint64(additional) {
		return errors.New("parser-core phase zero: drop-cohort reference spill index overflow")
	}
	return nil
}

func dropCohortRefAppendUnique(dst *[dropCohortRefHardCap]DropCohortRef, count *int, ref DropCohortRef) (changed, overflowed bool) {
	for i := 0; i < *count; i++ {
		if dst[i] == ref {
			return false, false
		}
	}
	if *count >= len(dst) {
		return false, true
	}
	index := *count
	for i := 0; i < *count; i++ {
		if dropCohortRefLess(ref, dst[i]) {
			index = i
			break
		}
	}
	copy(dst[index+1:*count+1], dst[index:*count])
	dst[index] = ref
	(*count)++
	return true, false
}

// dropCohortRefUnion performs a transactional, deterministic union. It
// preflights the complete spill append before changing either the set or the
// arena, so a rejected union leaves every byte and flag unchanged.
func (c *Core) dropCohortRefUnion(dst *DropCohortRefSet, src DropCohortRefSet) (bool, error) {
	if c == nil || dst == nil {
		return false, errors.New("parser-core phase zero: nil drop-cohort reference union")
	}
	if dst.Overflowed() {
		return false, nil
	}
	srcCount, ok := c.dropCohortRefCount(src)
	if !ok {
		return false, errors.New("parser-core phase zero: invalid drop-cohort reference spill")
	}
	if src.Overflowed() {
		before := dst.Flags
		dst.Flags |= dropCohortRefFlagOverflowed
		if src.Blended() {
			dst.Flags |= dropCohortRefFlagBlended
		}
		return dst.Flags != before, nil
	}
	if srcCount == 0 {
		if src.Blended() && !dst.Blended() {
			dst.Flags |= dropCohortRefFlagBlended
			return true, nil
		}
		return false, nil
	}
	dstCount, ok := c.dropCohortRefCount(*dst)
	if !ok {
		return false, errors.New("parser-core phase zero: invalid destination drop-cohort reference spill")
	}
	var merged [dropCohortRefHardCap]DropCohortRef
	count := 0
	blended := dst.Blended() || src.Blended()
	overflowed := false
	for index := 0; index < dstCount; index++ {
		ref, valid := c.DropCohortRefAt(*dst, index)
		if !valid {
			return false, errors.New("parser-core phase zero: invalid destination drop-cohort reference spill")
		}
		changed, overflow := dropCohortRefAppendUnique(&merged, &count, ref)
		overflowed = overflowed || overflow
		if !changed {
			continue
		}
	}
	for index := 0; index < srcCount; index++ {
		ref, valid := c.DropCohortRefAt(src, index)
		if !valid {
			return false, errors.New("parser-core phase zero: invalid source drop-cohort reference spill")
		}
		changed, overflow := dropCohortRefAppendUnique(&merged, &count, ref)
		overflowed = overflowed || overflow
		if !changed {
			continue
		}
	}
	if overflowed {
		before := dst.Flags
		dst.Flags |= dropCohortRefFlagOverflowed
		if blended {
			dst.Flags |= dropCohortRefFlagBlended
		}
		return dst.Flags != before, nil
	}

	oldFlags := dst.Flags
	oldCount := int(dst.Count)
	if count == oldCount && oldCount != 0 {
		same := true
		for i := 0; i < dstCount; i++ {
			ref, valid := c.DropCohortRefAt(*dst, i)
			if !valid || merged[i] != ref {
				same = false
				break
			}
		}
		if same {
			if blended {
				dst.Flags |= dropCohortRefFlagBlended
			}
			return dst.Flags != oldFlags, nil
		}
	}
	newFlags := uint8(0)
	if blended {
		newFlags |= dropCohortRefFlagBlended
	}
	if count <= dropCohortRefInlineCapacity {
		*dst = DropCohortRefSet{Count: uint8(count), Flags: newFlags}
		copy(dst.Inline[:], merged[:count])
		return true, nil
	}
	// A destination that already aliases exactly this sequence needs no new
	// storage. This check also keeps repeated canonicalization idempotent.
	if dst.Spilled() && dstCount == count {
		same := true
		for i := 0; i < dstCount; i++ {
			ref, valid := c.DropCohortRefAt(*dst, i)
			if !valid || merged[i] != ref {
				same = false
				break
			}
		}
		if same {
			dst.Flags = dst.Flags&^dropCohortRefFlagBlended | newFlags
			return dst.Flags != oldFlags, nil
		}
	}
	// A sorted tail extension keeps the existing segment stable and appends
	// only the new suffix. Copies of the set still read their old prefix.
	// This avoids quadratic spill growth during sequential reference inserts.
	if dst.Spilled() && oldCount != 0 && dstCount == oldCount &&
		int(dst.Spill)-1+oldCount == len(c.dropCohortRefSpill) {
		prefixEqual := true
		for index := 0; index < dstCount; index++ {
			ref, valid := c.DropCohortRefAt(*dst, index)
			if !valid || merged[index] != ref {
				prefixEqual = false
				break
			}
		}
		if prefixEqual && count > oldCount {
			additional := count - oldCount
			if err := c.dropCohortRefPreflight(additional); err != nil {
				return false, err
			}
			c.dropCohortRefSpill = append(c.dropCohortRefSpill, merged[oldCount:count]...)
			dst.Count = uint8(count)
			dst.Flags = newFlags | dropCohortRefFlagSpilled
			return true, nil
		}
	}
	if err := c.dropCohortRefPreflight(count); err != nil {
		return false, err
	}
	start := len(c.dropCohortRefSpill)
	c.dropCohortRefSpill = append(c.dropCohortRefSpill, merged[:count]...)
	dst.Spill = uint32(start) + 1
	dst.Count = uint8(count)
	dst.Flags = newFlags | dropCohortRefFlagSpilled
	return true, nil
}

func (c *Core) addDropCohortRef(set *DropCohortRefSet, ref DropCohortRef) (bool, error) {
	if ref == (DropCohortRef{}) {
		return false, nil
	}
	var singleton DropCohortRefSet
	singleton.Inline[0] = ref
	singleton.Count = 1
	return c.dropCohortRefUnion(set, singleton)
}

// AddDropCohortRef inserts one reference and spills when required. It returns
// false for duplicates, overflow, or a rejected preflight.
func (c *Core) AddDropCohortRef(set *DropCohortRefSet, ref DropCohortRef) bool {
	changed, _ := c.addDropCohortRef(set, ref)
	return changed
}

// UnionDropCohortRefs unions src into dst. A failed preflight leaves dst
// unchanged and returns false.
func (c *Core) UnionDropCohortRefs(dst *DropCohortRefSet, src DropCohortRefSet) bool {
	changed, _ := c.dropCohortRefUnion(dst, src)
	return changed
}

// UnionDropCohortRefsChecked reports a spill preflight or stale-reference
// error to scheduler callers that must fail the enclosing operation.
func (c *Core) UnionDropCohortRefsChecked(dst *DropCohortRefSet, src DropCohortRefSet) (bool, error) {
	return c.dropCohortRefUnion(dst, src)
}

func (c *Core) NodeLineageDropCohortRefs(id NodeID) (DropCohortRefSet, error) {
	record, err := c.nodeLineage(id)
	if err != nil {
		return DropCohortRefSet{}, err
	}
	return record.dropCohortRefs, nil
}
