// Package recover contains grammar-independent recovery policies.
package recover

// LocalSkip describes the two lookaheads at a parser dead end. The second
// lookahead is lexed in the current state after the unexpected terminal.
// A direct shift proves that skipping the terminal preserves that context.
type LocalSkip struct {
	UnexpectedNamed, UnexpectedExternal bool
	UnexpectedStart, UnexpectedEnd      uint32
	NextStart, NextEnd                  uint32
	NextShift, NextExtra                bool
	PaddingOnly                         bool
}

// PreferLocalSkip keeps a resumable production before recovery unwinds it.
// Named and external tokens retain their existing recovery competition.
func PreferLocalSkip(r LocalSkip) bool {
	return !r.UnexpectedNamed && !r.UnexpectedExternal &&
		r.UnexpectedEnd > r.UnexpectedStart &&
		r.NextStart >= r.UnexpectedEnd && r.NextEnd > r.NextStart &&
		r.NextShift && !r.NextExtra && r.PaddingOnly
}

// MatchingDelimiters identifies a completed production bounded by the same
// anonymous terminal. Ordinary assignments and declarations do not qualify.
func MatchingDelimiters(open, close uint16, openNamed, closeNamed bool) bool {
	return open != 0 && open == close && !openNamed && !closeNamed
}

// AlternativeStep describes a scan in the context recovery would unwind to.
// Only a lexical failure proves that this alternative is unterminated.
// A reduction, conflict, or exhausted probe budget keeps the existing recovery.
type AlternativeStep struct {
	State          uint16
	End            uint32
	Shift          bool
	LexicalFailure bool
}

// UnterminatedAlternative follows direct shifts through the alternate context.
// It never treats an ambiguous or unproved continuation as a lexical failure.
func UnterminatedAlternative(state uint16, at uint32, probe func(uint16, uint32) AlternativeStep) bool {
	for i := 0; i < 16; i++ {
		next := probe(state, at)
		if next.LexicalFailure {
			return true
		}
		if !next.Shift || next.End <= at {
			return false
		}
		state, at = next.State, next.End
	}
	return false
}
