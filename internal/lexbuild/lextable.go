package lexbuild

import (
	"fmt"
	"sync/atomic"
)

// Size limits of the lex table construction. The generator has no such limit:
// a pattern such as [ab]*a[ab]{24} (29 automaton states) has 33,554,432
// reachable lex states and the generator runs until memory is gone. The port
// refuses a grammar whose construction passes one of these limits, with a
// *SizeError, before it inserts the state that would pass it. The limits apply
// to one construction: the entry states of one table, the keyword table, or
// the states of one token (the last loop of build_lex_table).
//
//   - maxLexStates: lex states (subset construction results).
//   - maxLexNFAIDs: the sum, over lex states, of the automaton states in each
//     (the size of the keys and queue entries).
//   - maxLexAdvances: the sum, over lex states, of their character transitions.
//
// They are variables so that tests can lower them. The bundled grammars need
// at most 11,785 lex states in one construction (the census prints the
// largest sums); the limits are well above that.
var (
	maxLexStates   = 1 << 17
	maxLexNFAIDs   = 1 << 25
	maxLexAdvances = 1 << 22
)

// SizeError reports that a construction passed a size limit of this package.
type SizeError struct {
	What  string // what was counted
	Limit int
}

func (e *SizeError) Error() string {
	return fmt.Sprintf("lexbuild: size limit: more than %d %s in one lex table construction", e.Limit, e.What)
}

type lexTooLarge struct{ err *SizeError }

// guardSize runs f and turns the panic of a passed size limit into an error.
func guardSize(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if l, ok := r.(lexTooLarge); ok {
				err = l.err
				return
			}
			panic(r)
		}
	}()
	f()
	return nil
}

// peakLexStates is the largest number of lex states that one construction
// reached since the program began (read by the census test).
var peakLexStates, peakLexNFAIDs, peakLexAdvances atomic.Int64

func notePeak(p *atomic.Int64, n int) {
	for {
		old := p.Load()
		if int64(n) <= old || p.CompareAndSwap(old, int64(n)) {
			return
		}
	}
}

// advanceAction is tables.rs: struct AdvanceAction.
type advanceAction struct {
	state       uint32
	inMainToken bool
}

type charAction struct {
	chars  CharSet
	action advanceAction
}

// lexState is tables.rs: struct LexState.
type lexState struct {
	accept    Symbol
	hasAccept bool
	eof       advanceAction
	hasEOF    bool
	advances  []charAction
}

// lexTable is tables.rs: struct LexTable.
type lexTable struct {
	states []lexState
}

type queueEntry struct {
	stateID   uint32
	nfaStates []uint32
	eofValid  bool
}

// lexTableBuilder is build_lex_table.rs: struct LexTableBuilder.
type lexTableBuilder struct {
	g        *LexicalGrammar
	cursor   *nfaCursor
	table    lexTable
	queue    []queueEntry
	byNFASet map[string]uint32

	nfaIDs, advances int // counted against maxLexNFAIDs and maxLexAdvances
}

// newLexTableBuilder is build_lex_table.rs: fn LexTableBuilder::new.
func newLexTableBuilder(g *LexicalGrammar) *lexTableBuilder {
	return &lexTableBuilder{g: g, cursor: newNfaCursor(&g.nfa, nil), byNFASet: map[string]uint32{}}
}

// reset is build_lex_table.rs: fn LexTableBuilder::reset.
func (b *lexTableBuilder) reset() {
	b.table = lexTable{}
	b.queue = nil
	b.byNFASet = map[string]uint32{}
	b.nfaIDs, b.advances = 0, 0
}

func nfaSetKey(ids []uint32, eof bool) string {
	buf := make([]byte, 0, 4*len(ids)+1)
	for _, id := range ids {
		buf = append(buf, byte(id), byte(id>>8), byte(id>>16), byte(id>>24))
	}
	if eof {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	return string(buf)
}

// addStateForTokens is build_lex_table.rs: fn LexTableBuilder::add_state_for_tokens.
// Go shape: the Rust entry-point debug log is not written.
func (b *lexTableBuilder) addStateForTokens(tokens *TokenSet) uint32 {
	eofValid := false
	var nfaStates []uint32
	for _, t := range tokens.symbols() {
		if t.IsTerminal() {
			nfaStates = append(nfaStates, b.g.Variables[t.Index].StartState)
		} else {
			eofValid = true
		}
	}
	stateID, _ := b.addState(nfaStates, eofValid)
	for len(b.queue) > 0 {
		e := b.queue[0]
		b.queue = b.queue[1:]
		b.populateState(e.stateID, e.nfaStates, e.eofValid)
	}
	return stateID
}

// addState is build_lex_table.rs: fn LexTableBuilder::add_state.
func (b *lexTableBuilder) addState(nfaStates []uint32, eofValid bool) (uint32, bool) {
	b.cursor.reset(nfaStates)
	key := nfaSetKey(b.cursor.stateIDs, eofValid)
	if id, ok := b.byNFASet[key]; ok {
		return id, false
	}
	// Size limits, checked before the state is inserted.
	if len(b.table.states) >= maxLexStates {
		panic(lexTooLarge{&SizeError{"lex states", maxLexStates}})
	}
	if b.nfaIDs+len(b.cursor.stateIDs) > maxLexNFAIDs {
		panic(lexTooLarge{&SizeError{"automaton states summed over lex states", maxLexNFAIDs}})
	}
	b.nfaIDs += len(b.cursor.stateIDs)
	notePeak(&peakLexStates, len(b.table.states)+1)
	notePeak(&peakLexNFAIDs, b.nfaIDs)
	id := uint32(len(b.table.states))
	b.table.states = append(b.table.states, lexState{})
	b.queue = append(b.queue, queueEntry{id, append([]uint32(nil), b.cursor.stateIDs...), eofValid})
	b.byNFASet[key] = id
	return id, true
}

// populateState is build_lex_table.rs: fn LexTableBuilder::populate_state.
func (b *lexTableBuilder) populateState(stateID uint32, nfaStates []uint32, eofValid bool) {
	b.cursor.forceReset(nfaStates)

	// The EOF state is represented as an empty list of NFA states.
	var compID, compPrec int
	haveCompletion := false
	for _, c := range b.cursor.completions() {
		id, prec := c[0], c[1]
		if haveCompletion && preferToken(b.g, [2]int{compPrec, compID}, [2]int{prec, id}) {
			continue
		}
		compID, compPrec, haveCompletion = id, prec, true
	}

	transitions, hasSep := b.cursor.transitionsAndAnySep()

	// If EOF is a valid lookahead token, add a transition predicated on the null
	// character that leads to the empty set of NFA states.
	if eofValid {
		next, _ := b.addState(nil, false)
		b.table.states[stateID].eof = advanceAction{next, true}
		b.table.states[stateID].hasEOF = true
	}

	if b.advances+len(transitions) > maxLexAdvances {
		panic(lexTooLarge{&SizeError{"character transitions summed over lex states", maxLexAdvances}})
	}
	b.advances += len(transitions)
	notePeak(&peakLexAdvances, b.advances)
	for _, t := range transitions {
		if haveCompletion && !preferTransition(b.g, &t, compID, int32(compPrec), hasSep) {
			continue
		}
		next, _ := b.addState(t.states, eofValid && t.isSeparator)
		st := &b.table.states[stateID]
		st.advances = append(st.advances, charAction{t.characters, advanceAction{next, !t.isSeparator}})
	}

	st := &b.table.states[stateID]
	if haveCompletion {
		st.accept, st.hasAccept = terminalSym(compID), true
	} else if len(b.cursor.stateIDs) == 0 {
		st.accept, st.hasAccept = endSym(), true
	}
}

// preferToken is token_conflicts.rs: fn TokenConflictMap::prefer_token. left
// and right are (precedence, variable index).
func preferToken(g *LexicalGrammar, left, right [2]int) bool {
	switch {
	case left[0] < right[0]:
		return false
	case left[0] > right[0]:
		return true
	}
	li, ri := g.Variables[left[1]].ImplicitPrecedence, g.Variables[right[1]].ImplicitPrecedence
	switch {
	case li < ri:
		return false
	case li > ri:
		return true
	}
	return left[1] < right[1]
}

// preferTransition is token_conflicts.rs: fn TokenConflictMap::prefer_transition.
func preferTransition(g *LexicalGrammar, t *nfaTransition, completedID int, completedPrecedence int32, hasSeparatorTransitions bool) bool {
	if t.precedence < completedPrecedence {
		return false
	}
	if t.precedence == completedPrecedence {
		if t.isSeparator {
			return false
		}
		if hasSeparatorTransitions {
			found := false
			for _, i := range g.variableIndicesForNFAStates(t.states) {
				if i == completedID {
					found = true
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}
