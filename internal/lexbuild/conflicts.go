package lexbuild

// conflictStatus is token_conflicts.rs: struct TokenConflictStatus (bit flags).
type conflictStatus uint8

const (
	matchesPrefix          conflictStatus = 1 << 0
	doesMatchContinuation  conflictStatus = 1 << 1
	doesMatchValidCont     conflictStatus = 1 << 2
	doesMatchSeparators    conflictStatus = 1 << 3
	matchesSameString      conflictStatus = 1 << 4
	matchesDifferentString conflictStatus = 1 << 5
)

// TokenConflictMap is token_conflicts.rs: struct TokenConflictMap. The Rust
// per-row bitsets (conflict_or_prefix_bits, overlap_either_bits) are boolean
// matrices here.
type TokenConflictMap struct {
	n      int
	status []conflictStatus

	conflictOrPrefix [][]bool
	overlapEither    [][]bool
}

func (m *TokenConflictMap) at(i, j int) conflictStatus { return m.status[m.n*i+j] }

// newTokenConflictMap is token_conflicts.rs: fn TokenConflictMap::new.
func newTokenConflictMap(g *LexicalGrammar, following []TokenSet) *TokenConflictMap {
	cursor := newNfaCursor(&g.nfa, nil)
	starting := getStartingChars(cursor, g)
	followingChars := getFollowingChars(starting, following)
	nfaStateToVar := make([]int, len(g.nfa.states))
	for id := range nfaStateToVar {
		nfaStateToVar[id] = g.variableIndexForNFAState(uint32(id))
	}
	n := len(g.Variables)
	m := &TokenConflictMap{n: n, status: make([]conflictStatus, n*n)}
	for i := 0; i < n; i++ {
		for j := 0; j < i; j++ {
			a, b := computeConflictStatus(cursor, g, followingChars, nfaStateToVar, i, j)
			m.status[n*i+j] = a
			m.status[n*j+i] = b
		}
	}
	const conflictMask = doesMatchValidCont | doesMatchSeparators | matchesSameString | matchesPrefix
	const overlapMask = doesMatchSeparators | matchesPrefix | matchesSameString | doesMatchContinuation
	m.conflictOrPrefix = make([][]bool, n)
	m.overlapEither = make([][]bool, n)
	for i := 0; i < n; i++ {
		m.conflictOrPrefix[i] = make([]bool, n)
		m.overlapEither[i] = make([]bool, n)
		for j := 0; j < n; j++ {
			eij, eji := m.at(i, j), m.at(j, i)
			if eij&conflictMask != 0 {
				m.conflictOrPrefix[i][j] = true
			}
			if eij&overlapMask != 0 || eji&overlapMask != 0 {
				m.overlapEither[i][j] = true
			}
		}
	}
	return m
}

// hasSameConflictStatus is token_conflicts.rs: fn has_same_conflict_status.
func (m *TokenConflictMap) hasSameConflictStatus(a, b, other int) bool {
	return m.at(a, other) == m.at(b, other)
}

// doesMatchDifferentString is token_conflicts.rs: fn does_match_different_string.
func (m *TokenConflictMap) doesMatchDifferentString(i, j int) bool {
	return m.at(i, j)&matchesDifferentString != 0
}

// doesMatchSameString is token_conflicts.rs: fn does_match_same_string.
func (m *TokenConflictMap) doesMatchSameString(i, j int) bool {
	return m.at(i, j)&matchesSameString != 0
}

// doesConflict is token_conflicts.rs: fn does_conflict.
func (m *TokenConflictMap) doesConflict(i, j int) bool {
	return m.at(i, j)&(doesMatchValidCont|doesMatchSeparators|matchesSameString) != 0
}

// doesMatchShorterOrLonger is token_conflicts.rs: fn does_match_shorter_or_longer.
func (m *TokenConflictMap) doesMatchShorterOrLonger(i, j int) bool {
	e, r := m.at(i, j), m.at(j, i)
	return e&(doesMatchValidCont|doesMatchSeparators) != 0 && r&doesMatchSeparators == 0
}

// doesOverlap is token_conflicts.rs: fn does_overlap.
func (m *TokenConflictMap) doesOverlap(i, j int) bool {
	return m.at(i, j)&(doesMatchSeparators|matchesPrefix|matchesSameString|doesMatchContinuation) != 0
}

// getStartingChars is token_conflicts.rs: fn get_starting_chars.
func getStartingChars(cursor *nfaCursor, g *LexicalGrammar) []CharSet {
	out := make([]CharSet, 0, len(g.Variables))
	for _, v := range g.Variables {
		cursor.reset([]uint32{v.StartState})
		var all CharSet
		for _, rt := range cursor.transitionChars() {
			all = all.add(*rt.chars)
		}
		out = append(out, all)
	}
	return out
}

// getFollowingChars is token_conflicts.rs: fn get_following_chars.
func getFollowingChars(starting []CharSet, following []TokenSet) []CharSet {
	out := make([]CharSet, len(following))
	for i := range following {
		var chars CharSet
		for _, tok := range following[i].symbols() {
			if tok.IsTerminal() {
				chars = chars.add(starting[tok.Index])
			}
		}
		out[i] = chars
	}
	return out
}

// computeConflictStatus is token_conflicts.rs: fn compute_conflict_status.
// Go shape: the Rust visited set holds an FxHash of each state set (a hash
// collision would skip a set); this one holds the exact set. The search order
// (a stack) does not reach the result: every reachable set is visited once and
// the result flags only accumulate.
func computeConflictStatus(cursor *nfaCursor, g *LexicalGrammar, followingChars []CharSet, nfaStateToVar []int, i, j int) (conflictStatus, conflictStatus) {
	visited := map[string]bool{}
	queue := [][]uint32{{g.Variables[i].StartState, g.Variables[j].StartState}}
	var r0, r1 conflictStatus

	for len(queue) > 0 {
		stateSet := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		// If only one of the two tokens could possibly match from this state, then
		// there is no reason to analyze any of its successors.
		first := nfaStateToVar[stateSet[0]]
		all := true
		for _, s := range stateSet {
			if nfaStateToVar[s] != first {
				all = false
				break
			}
		}
		if all {
			if first == i {
				r0 |= matchesDifferentString
			} else {
				r1 |= matchesDifferentString
			}
			continue
		}

		cursor.reset(stateSet)

		// Computed lazily, as in Rust.
		withinSep, haveSep := false, false
		getSep := func() bool {
			if !haveSep {
				for _, rt := range cursor.transitionChars() {
					if rt.isSep {
						withinSep = true
						break
					}
				}
				haveSep = true
			}
			return withinSep
		}

		var compID, compPrec int
		haveCompletion := false
		for _, c := range cursor.completions() {
			id, precedence := c[0], c[1]
			sep := getSep()
			if sep {
				if id == i {
					r0 |= doesMatchSeparators
				} else {
					r1 |= doesMatchSeparators
				}
			}
			if haveCompletion {
				if id == compID {
					continue
				}
				var preferred int
				if preferToken(g, [2]int{compPrec, compID}, [2]int{precedence, id}) {
					preferred = compID
				} else {
					preferred = id
					compID, compPrec = id, precedence
				}
				if preferred == i {
					r0 |= matchesSameString
				} else {
					r1 |= matchesSameString
				}
			} else {
				compID, compPrec, haveCompletion = id, precedence, true
			}
		}

		for _, tr := range cursor.transitions() {
			tr := tr
			canAdvance := true
			if haveCompletion {
				advancedID := -1
				successorContainsCompleted := false
				prevVar := -1
				for _, sid := range tr.states {
					v := nfaStateToVar[sid]
					if prevVar == v {
						continue
					}
					prevVar = v
					if v == compID {
						successorContainsCompleted = true
						break
					}
					advancedID = v
				}
				if advancedID >= 0 && !successorContainsCompleted {
					sepArg := false
					if haveSep {
						sepArg = withinSep
					}
					if preferTransition(g, &tr, compID, int32(compPrec), sepArg) {
						canAdvance = true
						if advancedID == i {
							r0 |= doesMatchContinuation
							if tr.characters.doesIntersect(followingChars[j]) {
								r0 |= doesMatchValidCont
							}
						} else {
							r1 |= doesMatchContinuation
							if tr.characters.doesIntersect(followingChars[i]) {
								r1 |= doesMatchValidCont
							}
						}
					} else if compID == i {
						r0 |= matchesPrefix
					} else {
						r1 |= matchesPrefix
					}
				}
			}
			if canAdvance {
				key := nfaSetKey(tr.states, false)
				if !visited[key] {
					visited[key] = true
					queue = append(queue, tr.states)
				}
			}
		}
	}
	return r0, r1
}
