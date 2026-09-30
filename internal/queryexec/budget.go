// Package queryexec owns resource accounting shared by query matchers.
package queryexec

// MaxActiveStates bounds recursive matching states independently of work.
// Exhausting either bound produces an explicit incomplete result.
const MaxActiveStates = 4096

// Budget belongs to one pattern/node attempt, including nested alternatives.
// A nil budget explicitly opts out of the resource bounds.
type Budget struct {
	remaining int
	active    int
	exceeded  bool
}

// NewBudget returns nil for a nonpositive, explicitly unlimited work limit.
func NewBudget(limit int) *Budget {
	if limit <= 0 {
		return nil
	}
	return &Budget{remaining: limit}
}

// Charge consumes one enumerated state and latches exhaustion.
func (b *Budget) Charge() bool {
	if b == nil {
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
	if b == nil {
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
	if b != nil {
		b.active--
	}
}

// Exceeded reports latched work or active-state exhaustion.
func (b *Budget) Exceeded() bool { return b != nil && b.exceeded }

// Remaining reports the work allowance left, or -1 for an unlimited budget.
func (b *Budget) Remaining() int {
	if b == nil {
		return -1
	}
	return b.remaining
}
