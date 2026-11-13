package lexbuild

// coincidentTokenIndex is coincident_tokens.rs: struct CoincidentTokenIndex.
// contains[a][b] is set when terminals a and b are both valid in one parse
// state. withoutWord[a][b] is set when they are coincident in a state where
// the word token is not valid.
type coincidentTokenIndex struct {
	n           int
	contains    [][]bool
	withoutWord [][]bool
}

// newCoincidentTokenIndex is coincident_tokens.rs: fn CoincidentTokenIndex::new.
// states are the valid tokens of each parse state.
func newCoincidentTokenIndex(states []TokenSet, n int, word Symbol, hasWord bool) *coincidentTokenIndex {
	bSymbolCells.check(cells(n, n)) // two boolean matrices
	c := &coincidentTokenIndex{n: n, contains: make([][]bool, n), withoutWord: make([][]bool, n)}
	for i := 0; i < n; i++ {
		c.contains[i] = make([]bool, n)
		c.withoutWord[i] = make([]bool, n)
	}
	var work int64
	for _, st := range states {
		idx := st.terminalIndices()
		work += int64(len(idx))*int64(len(idx)+1)/2 + 1
		bCoincidentWork.checkWork(work)
		has := hasWord && st.Contains(word)
		for i, a := range idx {
			for _, b := range idx[i:] {
				c.contains[a][b], c.contains[b][a] = true, true
				if !has {
					c.withoutWord[a][b], c.withoutWord[b][a] = true, true
				}
			}
		}
	}
	return c
}

// allCoincidentStatesHaveWord is coincident_tokens.rs: fn all_coincident_states_have_word.
func (c *coincidentTokenIndex) allCoincidentStatesHaveWord(a, b Symbol) bool {
	return !c.withoutWord[a.Index][b.Index]
}

// containsPair is coincident_tokens.rs: fn contains.
func (c *coincidentTokenIndex) containsPair(a, b Symbol) bool {
	return c.contains[a.Index][b.Index]
}
