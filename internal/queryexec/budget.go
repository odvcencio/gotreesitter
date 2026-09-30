// Package queryexec owns resource accounting shared by query matchers.
package queryexec

// MaxActiveStates bounds recursive matching states independently of work.
// Exhausting either bound produces an explicit incomplete result.
const MaxActiveStates = 4096

// MaxRetainedCaptures bounds materialized output for one pattern/node attempt.
const MaxRetainedCaptures = 1_000_000

// Budget belongs to one pattern/node attempt, including nested alternatives.
// A nil budget explicitly opts out of the resource bounds.
type Budget struct {
	remaining int
	active    uint16
	exceeded  bool
	outputs   uint16
	captures  uint32
	expansion *Expansion
}

// NewBudget returns nil for a nonpositive, explicitly unlimited work limit.
func NewBudget(limit int) *Budget {
	if limit <= 0 {
		return nil
	}
	return &Budget{remaining: limit}
}

// Reset reuses one cursor-owned budget for the next synchronous attempt.
func (b *Budget) Reset(limit int) *Budget {
	*b = Budget{remaining: limit}
	if limit <= 0 {
		b.remaining = -1
	}
	return b
}

// Charge consumes one enumerated state and latches exhaustion.
func (b *Budget) Charge() bool {
	if b == nil || b.remaining < 0 {
		return true
	}
	if b.exceeded || b.remaining == 0 {
		b.exceeded = true
		return false
	}
	b.remaining--
	return true
}

// Enter reserves one active recursive state. Pair a successful call with Leave.
func (b *Budget) Enter() bool {
	if b == nil || b.remaining < 0 {
		return true
	}
	if b.exceeded || b.active == MaxActiveStates {
		b.exceeded = true
		return false
	}
	b.active++
	return true
}

// Leave releases a recursive state reserved by Enter.
func (b *Budget) Leave() {
	if b != nil && b.remaining >= 0 {
		b.active--
	}
}

// Retain reserves materialized output states and capture cells. Successful
// matches consume storage without changing the deterministic work counter.
func (b *Budget) Retain(captures int) bool {
	if b == nil || b.remaining < 0 {
		return true
	}
	if b.exceeded || b.outputs == MaxActiveStates || captures > MaxRetainedCaptures-int(b.captures) {
		b.exceeded = true
		return false
	}
	b.outputs++
	b.captures += uint32(captures)
	return true
}

// Release returns storage after a partial match has been consumed.
func (b *Budget) Release(captures int) {
	if b != nil && b.remaining >= 0 {
		b.outputs--
		b.captures -= uint32(captures)
	}
}

// Exceeded reports latched work or active-state exhaustion.
func (b *Budget) Exceeded() bool { return b != nil && b.exceeded }

// Remaining reports the work allowance left, or -1 for an unlimited budget.
func (b *Budget) Remaining() int {
	if b == nil || b.remaining < 0 {
		return -1
	}
	return b.remaining
}
