// Package recover contains grammar-independent recovery policies.
package recover

// PaddingBeforeLookahead proves that an empty external token consumes padding
// before, but none of, an already lexed lookahead. Replaying it advances the
// recovering version without changing the shared token stream.
func PaddingBeforeLookahead(position, start, end, lookaheadStart uint32) bool {
	return start == end && end > position && end == lookaheadStart
}

// PreferConflictShift orders equally ranked paused siblings as C does: the
// original shift version precedes appended reduction versions. Other versions
// retain their existing order.
func PreferConflictShift(aPaused, bPaused bool, aGroup, bGroup uint16, aReduced, bReduced bool) bool {
	return aPaused && bPaused && aGroup != 0 && aGroup == bGroup && !aReduced && bReduced
}

// HasOriginalConflictShift identifies a grammar shift that owns C's original
// version; repetition and extra shifts do not advance the grammar frontier.
func HasOriginalConflictShift(lastIsShift, lastIsRepetition, lastIsExtra bool) bool {
	return lastIsShift && !lastIsRepetition && !lastIsExtra
}

// HasErrorModeExternalToken identifies an external scanner that participates
// in C error-mode lexing. A scanner used only in ordinary grammar states has
// no recovery-version ordering proof.
func HasErrorModeExternalToken(validSymbols []bool) bool {
	for _, valid := range validSymbols {
		if valid {
			return true
		}
	}
	return false
}
