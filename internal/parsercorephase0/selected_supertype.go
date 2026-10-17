package parsercorephase0

import (
	"errors"
	"math"
	"unsafe"
)

// SetSupertypeMasks snapshots the grammar's singleton hidden-rule mask bits.
// A nil vector keeps the existing zero-supertype policy allocation-free.
func (p *SelectedStorePolicy) SetSupertypeMasks(masks []uint32) error {
	if p == nil || len(masks) != 0 && len(masks) != len(p.Symbols) {
		return errors.New("parser-core phase zero: selected-store supertype policy width drifted")
	}
	for _, mask := range masks {
		if mask&(mask-1) != 0 {
			return errors.New("parser-core phase zero: selected-store supertype policy is not a single bit")
		}
	}
	p.SupertypeMasks = append([]uint32(nil), masks...)
	return nil
}

func (p SelectedStorePolicy) supertypeMask(symbol Symbol) uint32 {
	if int(symbol) >= len(p.SupertypeMasks) {
		return 0
	}
	return p.SupertypeMasks[symbol]
}

// NodeSupertypeMask returns the exact hidden-supertype ancestry of one
// selected occurrence. Released stores and invalid IDs fail closed.
func (s *SelectedStore) NodeSupertypeMask(id SelectedNodeID) (uint32, bool) {
	r, ok := s.record(id)
	if !ok {
		return 0, false
	}
	return r.SupertypeMask, true
}

// RetainedBytes includes the policy holder and every exclusively owned backing
// array retained by the core, including unused capacity.
func (p *SelectedStorePolicy) RetainedBytes() uint64 {
	if p == nil {
		return 0
	}
	total := uint64(unsafe.Sizeof(*p))
	add := func(capacity int, size uintptr) {
		n := uint64(capacity)
		b := uint64(size)
		if n > (math.MaxUint64-total)/b {
			total = math.MaxUint64
			return
		}
		total += n * b
	}
	add(cap(p.Symbols), unsafe.Sizeof(SelectedSymbolPolicy{}))
	add(cap(p.Unary), unsafe.Sizeof(SelectedUnaryRule(0)))
	add(cap(p.RetainedAliasChildren), unsafe.Sizeof(SelectedAliasChildPair{}))
	add(cap(p.SupertypeMasks), unsafe.Sizeof(uint32(0)))
	add(cap(p.SemicolonContainers), unsafe.Sizeof(false))
	add(cap(p.Cases), unsafe.Sizeof(false))
	add(cap(p.StatementLists), unsafe.Sizeof(false))
	return total
}
