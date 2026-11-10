package ccore

// Handles, lengths, constants and flag bits (C/subtree.h, C/length.h, C/parser.c).

// Symbol is a grammar symbol (C TSSymbol).
type Symbol uint16

// Handle is C's Subtree: bit 0 = inline leaf; otherwise index<<1 into a Store.
type Handle uint64 // C Subtree

// Length is C's Length (bytes plus a row/column extent).
type Length struct{ bytes, row, col uint32 } // C Length

func lenAdd(a, b Length) Length { // C/length.h length_add + point_add
	r := Length{bytes: a.bytes + b.bytes}
	if b.row > 0 {
		r.row, r.col = a.row+b.row, b.col
	} else {
		r.row, r.col = a.row, a.col+b.col
	}
	return r
}

func lenSub(a, b Length) Length { // C/length.h length_sub + point_sub
	var r Length
	if a.bytes >= b.bytes {
		r.bytes = a.bytes - b.bytes
	}
	if a.row > b.row {
		r.row, r.col = a.row-b.row, a.col
	} else if a.col >= b.col {
		r.col = a.col - b.col
	}
	return r
}

const (
	symError           Symbol = 65535 // ts_builtin_sym_error
	symErrorRepeat     Symbol = 65534 // ts_builtin_sym_error_repeat
	stateNone                 = 65535 // TS_TREE_STATE_NONE
	maxVersionCount           = 6
	maxVersionOverflow        = 4
	maxCostDifference         = 18 * 100
	costPerRecovery           = 500
	costPerSkippedTree        = 100
	costPerMissingTree        = 110
	maxLinkCount              = 8
	maxIteratorCount          = 64
)

// Inline flag bits (C SubtreeInlineData, in this order).
const (
	inlInline uint64 = 1 << iota
	inlVisible
	inlNamed
	inlExtra
	inlHasChanges
	inlMissing
	inlKeyword
	inlDependsOnColumn
)

// Heap flag bits (C SubtreeHeapData).
const (
	heapVisible uint16 = 1 << iota
	heapNamed
	heapExtra
	heapFragileLeft
	heapFragileRight
	heapHasChanges
	heapHasExternalTokens
	heapDependsOnColumn
	heapMissing
	heapKeyword
)

func lenSaturatingSub(a, b Length) Length { // C/length.h length_saturating_sub
	if a.bytes > b.bytes {
		return lenSub(a, b)
	}
	return Length{}
}

// Bytes, Row and Col read a Length (C Length.bytes and Length.extent).
func (l Length) Bytes() uint32 { return l.bytes }
func (l Length) Row() uint32   { return l.row }
func (l Length) Col() uint32   { return l.col }

// NewLength builds a Length from a byte offset and a row/column point.
func NewLength(bytes, row, col uint32) Length { return Length{bytes: bytes, row: row, col: col} }
