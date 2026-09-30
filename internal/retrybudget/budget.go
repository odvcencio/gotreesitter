// Package retrybudget bounds retry work within one parse operation.
package retrybudget

import "math"

// Budget belongs to an operation, including its nested parses. The first pass
// seeds the work allowance once; later results cannot renew it.
type Budget struct {
	Passes    int
	Remaining uint64
	Seeded    bool
	Limited   bool
}

// Seed grants at most two first-pass work units to the retry ladder. A pass
// that already reached its stack ceiling gets no retry work.
func (b *Budget) Seed(tokens uint64, stacks, stackCap int) {
	if b.Seeded {
		return
	}
	b.Seeded = true
	b.Limited = true
	if tokens == 0 || stacks <= 0 || stackCap <= 0 || stacks >= stackCap {
		return
	}
	b.Remaining = product(product(tokens, uint64(stacks)), 2)
}

// TakePass reserves a retry slot. The pass-count limit remains authoritative
// even for grammars whose work policy has not been certified.
func (b *Budget) TakePass(limit int) bool {
	if b.Exhausted(limit) {
		return false
	}
	b.Passes++
	return true
}

func (b *Budget) Exhausted(limit int) bool {
	return b.Passes >= limit || (b.Limited && b.Remaining == 0)
}

// Charge accounts for the completed pass using the same token/stack product
// as the first-pass allowance. Saturation makes malformed-input overflow
// exhaust the allowance instead of restoring it.
func (b *Budget) Charge(tokens uint64, stacks int) {
	if !b.Limited || stacks <= 0 {
		return
	}
	work := product(tokens, uint64(stacks))
	if work >= b.Remaining {
		b.Remaining = 0
	} else {
		b.Remaining -= work
	}
}

func product(a, b uint64) uint64 {
	if b != 0 && a > math.MaxUint64/b {
		return math.MaxUint64
	}
	return a * b
}
