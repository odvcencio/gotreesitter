// Package incr contains incremental reuse proofs shared by parser adapters.
package incr

// Entry describes one stack entry, numbered from the live top down. Present
// distinguishes a syntax payload from the initial parser-state sentinel.
type Entry struct {
	State   uint16
	Present bool
	Extra   bool
}

// Reduction is a certified normal-dispatch reduction for the current token.
type Reduction struct {
	Symbol   uint16
	Children int
}

// ReachesFrontier proves that reductions alone reach an old leaf's frontier.
// It reads the original stack through entryAt and keeps reduced parents on a
// small virtual stack. A failed proof leaves the real stack unchanged.
// Ambiguous ancestry, extras, missing gotos, shifts and cycles fail closed.
func ReachesFrontier(from, target uint16, entryAt func(int) (Entry, bool), reduction func(uint16) (Reduction, bool), gotoState func(uint16, uint16) uint16) bool {
	var parents [32]Entry
	count, offset := 0, 0
	peek := func() (Entry, bool) {
		if count > 0 {
			return parents[count-1], true
		}
		return entryAt(offset)
	}
	for step := 0; step < 128; step++ {
		if from == target {
			return true
		}
		act, ok := reduction(from)
		if !ok || act.Children < 0 {
			return false
		}
		for child := 0; child < act.Children; child++ {
			top, ok := peek()
			if !ok || !top.Present || top.Extra {
				return false
			}
			if count > 0 {
				count--
			} else {
				offset++
			}
		}
		before, ok := peek()
		if !ok || count == len(parents) {
			return false
		}
		from = gotoState(before.State, act.Symbol)
		if from == 0 {
			return false
		}
		parents[count] = Entry{State: from, Present: true}
		count++
	}
	return false
}
