package incr

import "slices"

// ProbeModeSet accelerates membership checks for common lexer state IDs.
// Wider IDs retain the caller's ordered slice as their authoritative set.
// The caller continues to enforce its independent mode and work budgets.
type ProbeModeSet [64]uint64

// SeenOrAdd reports whether state was already present. A first small state
// is recorded without allocating; a wide state is looked up in previous.
// The caller must append every new state to previous before the next call.
func (s *ProbeModeSet) SeenOrAdd(state uint32, previous []uint32) bool {
	if state < uint32(len(s)*64) {
		word, bit := state/64, uint64(1)<<(state%64)
		old := s[word]
		s[word] = old | bit
		return old&bit != 0
	}
	return slices.Contains(previous, state)
}
