// Package lexproof supplies bounded scratch for lexical reuse proofs.
package lexproof

// Modes preserves lexer-mode order while eliminating duplicate states. The
// capacity matches the primitive proof limit. Slots store indexes into values,
// so every uint32 state, including zero, has an unambiguous representation.
type Modes struct {
	values [1024]uint32
	slots  [2048]uint16
	count  uint16
}

// Add returns false only when a new mode would exceed the proof capacity.
func (m *Modes) Add(state uint32) bool {
	slot := (state ^ state>>16) & uint32(len(m.slots)-1)
	for m.slots[slot] != 0 {
		if m.values[m.slots[slot]-1] == state {
			return true
		}
		slot = (slot + 1) & uint32(len(m.slots)-1)
	}
	if int(m.count) == len(m.values) {
		return false
	}
	m.values[m.count] = state
	m.count++
	m.slots[slot] = m.count
	return true
}

func (m *Modes) Values() []uint32 { return m.values[:m.count] }
