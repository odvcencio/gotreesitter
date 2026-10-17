package parsercorephase0

// ExternalModesRejectAll proves that both explicit external-lex-state rows
// reject every external token, independently of input. Invalid row indices
// fail closed. The caller must select one parser state and retain checkpoint
// authentication; the error-mode row covers the lexer's recovery retry.
//
// This says nothing about internal DFA tokens. A caller that discards every
// internal candidate needs no speculative DFA reads when this proof holds.
//
//go:noinline
func ExternalModesRejectAll(rows [][]bool, primary, errorMode uint16) bool {
	if int(primary) >= len(rows) || int(errorMode) >= len(rows) {
		return false
	}
	for _, valid := range rows[primary] {
		if valid {
			return false
		}
	}
	if primary != errorMode {
		for _, valid := range rows[errorMode] {
			if valid {
				return false
			}
		}
	}
	return true
}
