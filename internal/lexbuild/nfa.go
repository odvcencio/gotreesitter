package lexbuild

import (
	"math/bits"
	"sort"
)

// nfaStateKind is the discriminant of nfa.rs: enum NfaState.
type nfaStateKind uint8

const (
	nfaAdvance nfaStateKind = iota
	nfaSplit
	nfaAccept
)

// nfaState is nfa.rs: enum NfaState. Advance uses chars, stateID, isSep and
// precedence; Split uses left and right; Accept uses variableIndex and
// precedence.
type nfaState struct {
	kind          nfaStateKind
	chars         CharSet
	stateID       uint32
	isSep         bool
	precedence    int32
	left, right   uint32
	variableIndex int
}

// nfa is nfa.rs: struct Nfa.
type nfa struct {
	states []nfaState
}

// lastStateID is nfa.rs: fn Nfa::last_state_id.
func (n *nfa) lastStateID() uint32 { return uint32(len(n.states)) - 1 }

// nfaTransition is nfa.rs: struct NfaTransition.
type nfaTransition struct {
	characters  CharSet
	isSeparator bool
	precedence  int32
	states      []uint32
}

// nfaCursor is nfa.rs: struct NfaCursor. stateIDs is sorted.
type nfaCursor struct {
	stateIDs []uint32
	n        *nfa
	work     int   // automaton states visited while closing sets (bClosureWork)
	gwork    int64 // steps of groupTransitions (bGroupWork), over all calls on this cursor

	mark []uint32 // mark[id] == gen when id is in the list being closed
	gen  uint32
}

// newNfaCursor is nfa.rs: fn NfaCursor::new.
func newNfaCursor(n *nfa, states []uint32) *nfaCursor {
	c := &nfaCursor{n: n}
	c.addStates(&states)
	return c
}

// reset is nfa.rs: fn NfaCursor::reset.
func (c *nfaCursor) reset(states []uint32) {
	c.stateIDs = c.stateIDs[:0]
	c.addStates(&states)
}

// forceReset is nfa.rs: fn NfaCursor::force_reset.
func (c *nfaCursor) forceReset(states []uint32) {
	c.stateIDs = append([]uint32(nil), states...)
}

type rawTransition struct {
	chars *CharSet
	isSep bool
	prec  int32
	state uint32
}

// rawTransitions is nfa.rs: fn NfaCursor::raw_transitions.
func (c *nfaCursor) rawTransitions() []rawTransition {
	var out []rawTransition
	for _, id := range c.stateIDs {
		st := &c.n.states[id]
		if st.kind == nfaAdvance {
			out = append(out, rawTransition{&st.chars, st.isSep, st.precedence, st.stateID})
		}
	}
	return out
}

// transitions is nfa.rs: fn NfaCursor::transitions.
func (c *nfaCursor) transitions() []nfaTransition {
	return groupTransitions(c.rawTransitions(), &c.gwork)
}

// transitionsAndAnySep is nfa.rs: fn NfaCursor::transitions_and_any_sep.
func (c *nfaCursor) transitionsAndAnySep() ([]nfaTransition, bool) {
	raw := c.rawTransitions()
	anySep := false
	for _, r := range raw {
		anySep = anySep || r.isSep
	}
	return groupTransitions(raw, &c.gwork), anySep
}

// transitionChars is nfa.rs: fn NfaCursor::transition_chars.
func (c *nfaCursor) transitionChars() []rawTransition { return c.rawTransitions() }

func equalU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// groupTransitions is nfa.rs: fn NfaCursor::group_transitions. Go shape: the
// Rust code reuses one CharacterSet buffer and moves vectors with mem::take;
// here sets are copied where Rust copies and moved where Rust moves. The
// final sort is by character set; the sets are disjoint, so the sort order is
// total and the unstable sort of Rust gives the same order as any sort.
func groupTransitions(raw []rawTransition, gwork *int64) []nfaTransition {
	var result []nfaTransition
	var chars CharSet
	copied := 0 // automaton state ids copied into transitions (bTransIDs)
	for _, rt := range raw {
		*gwork += int64(len(result) + 1 + rt.chars.rangeCount()) // the entries to scan, and the ranges copied
		bGroupWork.checkWork(*gwork)
		chars.assign(*rt.chars)
		i := 0
		for i < len(result) && !chars.isEmpty() {
			inter, steps := result[i].characters.removeIntersectionSteps(&chars)
			// The merge reads both sets, and the work of one call is at most
			// the ranges of the two sets: count it (one call passes the limit by
			// at most that many steps).
			*gwork += int64(steps)
			bGroupWork.checkWork(*gwork)
			if !inter.isEmpty() {
				charsIsEmpty := result[i].characters.isEmpty()
				var interStates []uint32
				if charsIsEmpty {
					interStates = result[i].states
					result[i].states = nil
				} else {
					copied += len(result[i].states) + 1
					bTransIDs.check(copied)
					interStates = append([]uint32(nil), result[i].states...)
				}
				if j := sort.Search(len(interStates), func(k int) bool { return interStates[k] >= rt.state }); j == len(interStates) || interStates[j] != rt.state {
					// The insertion moves the ids above the place: count them
					// before they move.
					*gwork += int64(len(interStates) - j + 1)
					bGroupWork.checkWork(*gwork)
					interStates = append(interStates, 0)
					copy(interStates[j+1:], interStates[j:])
					interStates[j] = rt.state
				}
				prec := result[i].precedence
				if rt.prec > prec {
					prec = rt.prec
				}
				t := nfaTransition{
					characters:  inter,
					isSeparator: result[i].isSeparator && rt.isSep,
					precedence:  prec,
					states:      interStates,
				}
				if charsIsEmpty {
					result[i] = t
				} else {
					result = append(result, t)
				}
			}
			i++
		}
		if !chars.isEmpty() {
			result = append(result, nfaTransition{
				characters:  chars.clone(),
				precedence:  rt.prec,
				states:      []uint32{rt.state},
				isSeparator: rt.isSep,
			})
			chars = CharSet{}
		}
	}

	i := 0
	for i < len(result) {
		*gwork += int64(i + 1)
		bGroupWork.checkWork(*gwork)
		for j := 0; j < i; j++ {
			// A comparison of two lists reads up to their common prefix.
			if len(result[j].states) == len(result[i].states) {
				*gwork += int64(1 + commonPrefixU32(result[j].states, result[i].states))
				bGroupWork.checkWork(*gwork)
			}
			if equalU32(result[j].states, result[i].states) &&
				result[j].isSeparator == result[i].isSeparator &&
				result[j].precedence == result[i].precedence {
				*gwork += int64(result[j].characters.rangeCount() + result[i].characters.rangeCount())
				bGroupWork.checkWork(*gwork)
				result[j].characters = result[j].characters.add(result[i].characters)
				// swap_remove(i)
				last := len(result) - 1
				result[i] = result[last]
				result = result[:last]
				i--
				break
			}
		}
		i++
	}
	// The sort key of an entry is its character count, then its ranges. The
	// count is read once for each entry here, not once for each comparison. The
	// sets are disjoint and not empty, so no two entries are equal and the order
	// is total: an unsorted-stable sort gives the order of any sort. Count the
	// n log n comparisons before the sort.
	*gwork += int64(len(result)) * int64(bits.Len(uint(len(result)))+1)
	bGroupWork.checkWork(*gwork)
	sort.Sort(&byCharSet{result, countsOf(result)})
	return result
}

func countsOf(ts []nfaTransition) []uint64 {
	n := make([]uint64, len(ts))
	for i := range ts {
		n[i] = ts[i].characters.charCount()
	}
	return n
}

// byCharSet orders transitions as compareCharSet does, with the character
// counts held in n.
type byCharSet struct {
	t []nfaTransition
	n []uint64
}

func (s *byCharSet) Len() int { return len(s.t) }
func (s *byCharSet) Swap(i, j int) {
	s.t[i], s.t[j] = s.t[j], s.t[i]
	s.n[i], s.n[j] = s.n[j], s.n[i]
}
func (s *byCharSet) Less(i, j int) bool {
	if s.n[i] != s.n[j] {
		return s.n[i] < s.n[j]
	}
	return compareCharSetSameCount(s.t[i].characters, s.t[j].characters) < 0
}

func commonPrefixU32(a, b []uint32) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// completions is nfa.rs: fn NfaCursor::completions. It returns
// (variable index, precedence) pairs in state-id order.
func (c *nfaCursor) completions() [][2]int {
	var out [][2]int
	for _, id := range c.stateIDs {
		st := &c.n.states[id]
		if st.kind == nfaAccept {
			out = append(out, [2]int{st.variableIndex, int(st.precedence)})
		}
	}
	return out
}

// addStates is nfa.rs: fn NfaCursor::add_states. It closes the given state
// ids over split states and inserts the non-split ids into the sorted
// stateIDs.
//
// Go shape: the Rust code asks, for each split, whether its left and right
// are already in the list that is being extended (a scan of the list), and
// inserts each non-split id at its place in stateIDs. Here a set answers the
// first question and the new ids are merged into stateIDs once, after the
// closure. The list ids grows in the same order and the sorted result is the
// same set, so the output equals the generator's. The work is counted against
// bClosureWork.
func (c *nfaCursor) addStates(newIDs *[]uint32) {
	ids := *newIDs
	if len(c.mark) < len(c.n.states) {
		c.mark = make([]uint32, len(c.n.states))
		c.gen = 0
	}
	c.gen++
	if c.gen == 0 { // wrapped: clear
		for i := range c.mark {
			c.mark[i] = 0
		}
		c.gen = 1
	}
	for _, x := range ids {
		c.mark[x] = c.gen
	}
	c.work += len(ids)
	bClosureWork.check(c.work)
	var adds []uint32
	for i := 0; i < len(ids); i++ {
		id := ids[i]
		st := &c.n.states[id]
		if st.kind == nfaSplit {
			if c.mark[st.left] != c.gen {
				c.mark[st.left] = c.gen
				ids = append(ids, st.left)
				c.work++
			}
			if c.mark[st.right] != c.gen {
				c.mark[st.right] = c.gen
				ids = append(ids, st.right)
				c.work++
			}
			bClosureWork.check(c.work)
		} else {
			j := sort.Search(len(c.stateIDs), func(k int) bool { return c.stateIDs[k] >= id })
			if j == len(c.stateIDs) || c.stateIDs[j] != id {
				adds = append(adds, id)
			}
		}
	}
	if len(adds) > 0 {
		c.stateIDs = append(c.stateIDs, adds...)
		sort.Slice(c.stateIDs, func(a, b int) bool { return c.stateIDs[a] < c.stateIDs[b] })
		out := c.stateIDs[:1]
		for _, x := range c.stateIDs[1:] {
			if x != out[len(out)-1] {
				out = append(out, x)
			}
		}
		c.stateIDs = out
	}
	*newIDs = ids
}
