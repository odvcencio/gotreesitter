package parsercorephase0

import (
	"cmp"
	"errors"
	"slices"
	"unsafe"
)

// SelectedUnaryRow separates the two common symbol-only rules. Overrides
// address off-diagonal pairs; all slices supplied to the constructor are copied.
type SelectedUnaryRow struct {
	Default SelectedUnaryRule
	Self    SelectedUnaryRule
}

type SelectedUnaryOverride struct {
	Parent, Child Symbol
	Rule          SelectedUnaryRule
}

type selectedUnaryPair struct {
	key  uint32
	rule SelectedUnaryRule
}

// SelectedUnaryProgram is immutable after construction. Copying the value
// shares its private backing arrays; no caller can mutate those arrays. A byte per symbol
// stores its default and diagonal rule; only exceptional pairs need records.
// Unknown symbols preserve the dense policy's conservative keep decision.
type SelectedUnaryProgram struct {
	rows  []uint8
	pairs []selectedUnaryPair
}

const selectedUnaryHasPairs = 1 << 4

func NewSelectedUnaryProgram(rows []SelectedUnaryRow, overrides []SelectedUnaryOverride) (SelectedUnaryProgram, error) {
	if len(rows) == 0 || len(rows) > 1<<16 {
		return SelectedUnaryProgram{}, errors.New("parser-core phase zero: unary program width is invalid")
	}
	if uint64(len(overrides)) > uint64(len(rows))*uint64(len(rows)-1) {
		return SelectedUnaryProgram{}, errors.New("parser-core phase zero: unary program pair count is invalid")
	}
	p := SelectedUnaryProgram{rows: make([]uint8, len(rows))}
	for i, row := range rows {
		if row.Default > SelectedUnaryRenameLeaf || row.Self > SelectedUnaryRenameLeaf {
			return SelectedUnaryProgram{}, errors.New("parser-core phase zero: unary program rule is invalid")
		}
		p.rows[i] = uint8(row.Default) | uint8(row.Self)<<2
	}
	if len(overrides) != 0 {
		p.pairs = make([]selectedUnaryPair, len(overrides))
	}
	for i, pair := range overrides {
		if int(pair.Parent) >= len(rows) || int(pair.Child) >= len(rows) || pair.Parent == pair.Child || pair.Rule > SelectedUnaryRenameLeaf {
			return SelectedUnaryProgram{}, errors.New("parser-core phase zero: unary program pair is invalid")
		}
		p.pairs[i] = selectedUnaryPair{key: uint32(pair.Parent)<<16 | uint32(pair.Child), rule: pair.Rule}
		p.rows[pair.Parent] |= selectedUnaryHasPairs
	}
	slices.SortFunc(p.pairs, func(a, b selectedUnaryPair) int { return cmp.Compare(a.key, b.key) })
	for i := 1; i < len(p.pairs); i++ {
		if p.pairs[i-1].key == p.pairs[i].key {
			return SelectedUnaryProgram{}, errors.New("parser-core phase zero: duplicate unary program pair")
		}
	}
	return p, nil
}

func (p *SelectedUnaryProgram) Width() int {
	if p == nil {
		return 0
	}
	return len(p.rows)
}

func (p *SelectedUnaryProgram) Rule(parent, child Symbol) SelectedUnaryRule {
	if p == nil || int(parent) >= len(p.rows) || int(child) >= len(p.rows) {
		return SelectedUnaryKeep
	}
	row := p.rows[parent]
	if parent == child {
		return SelectedUnaryRule(row >> 2 & 3)
	}
	if row&selectedUnaryHasPairs != 0 {
		key := uint32(parent)<<16 | uint32(child)
		lo, hi := 0, len(p.pairs)
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			if p.pairs[mid].key < key {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo < len(p.pairs) && p.pairs[lo].key == key {
			return p.pairs[lo].rule
		}
	}
	return SelectedUnaryRule(row & 3)
}

func (p *SelectedUnaryProgram) RetainedBytes() uint64 {
	if p == nil || len(p.rows) == 0 {
		return 0
	}
	return uint64(unsafe.Sizeof(*p)) + p.backingBytes()
}

// backingBytes excludes the inline program value already included in its
// policy holder. Immutable backing is shared by transient policy copies.
func (p *SelectedUnaryProgram) backingBytes() uint64 {
	if p == nil {
		return 0
	}
	return uint64(cap(p.rows)) + uint64(cap(p.pairs))*uint64(unsafe.Sizeof(selectedUnaryPair{}))
}

// NewSelectedStoreProgramPolicy copies symbol metadata and retains an immutable
// rule program by value, sharing its sealed backing. The dense constructor
// remains available to mutable adapters.
// A program's private backing arrays are copied at its own construction, so
// sharing this read-only value does not couple mutable policy fields.
func NewSelectedStoreProgramPolicy(symbols []SelectedSymbolPolicy, program SelectedUnaryProgram, expectedRoot Symbol) (SelectedStorePolicy, error) {
	if len(symbols) == 0 || int(expectedRoot) >= len(symbols) {
		return SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store policy has no authenticated root")
	}
	if program.Width() != len(symbols) {
		return SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store unary program width drifted")
	}
	return SelectedStorePolicy{Symbols: append([]SelectedSymbolPolicy(nil), symbols...), unaryProgram: program, ExpectedRoot: expectedRoot}, nil
}
