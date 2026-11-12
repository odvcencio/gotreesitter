package lexbuild

import (
	"math/bits"
	"sort"
)

// This file ports the part of render.rs that decides what each lex state does
// on input: the choice between ADVANCE_MAP and conditions, the large
// character sets, and the form of each condition. The generator writes these
// as C text. Here they are kept as data and evaluated, so that the record
// (accept, step at end of input, step at an ill-formed byte, ranges) can be
// written without text.

// largeCharacterRangeCount is build_lex_table.rs: const LARGE_CHARACTER_RANGE_COUNT.
const largeCharacterRangeCount = 8

type largeSet struct {
	hasSymbol bool
	symbol    Symbol
	set       CharSet
}

// collectLargeCharacterSets is the last loop of build_lex_table.rs: fn
// build_lex_table. For each token alone, it builds that token's lex states and
// collects the character sets that have more than LARGE_CHARACTER_RANGE_COUNT
// ranges.
func collectLargeCharacterSets(b *lexTableBuilder, g *LexicalGrammar) []largeSet {
	var out []largeSet
	var work int64
	has := func(c CharSet) bool {
		// The comparison reads each set kept so far, and the caller copies c
		// when it is new: charge both before either runs.
		work += mulWork(len(out)+1, 1+c.rangeCount())
		bRenderWork.checkWork(work)
		for _, s := range out {
			if equalCharSet(s.set, c) {
				return true
			}
		}
		return false
	}
	total := 0
	for ix := range g.Variables {
		sym := terminalSym(ix)
		b.reset()
		var ts TokenSet
		ts.Insert(sym)
		b.addStateForTokens(&ts)
		total += len(b.table.states)
		bLargeStates.check(total)
		for _, st := range b.table.states {
			var mainSets []*CharSet
			for i := range st.advances {
				ca := &st.advances[i]
				if ca.action.inMainToken {
					mainSets = append(mainSets, &ca.chars)
					continue
				}
				if ca.chars.rangeCount() > largeCharacterRangeCount && !has(ca.chars) {
					out = append(out, largeSet{false, Symbol{}, ca.chars.clone()})
				}
			}
			// The union of the sets of the advances in the main token (the
			// generator adds them one at a time). The join sorts the ranges of
			// all of them: charge that before it runs.
			mainRanges := 0
			for _, s := range mainSets {
				mainRanges += s.rangeCount()
			}
			work += int64(len(st.advances)) + mulWork(mainRanges, bits.Len(uint(mainRanges))+1)
			bRenderWork.checkWork(work)
			characters := unionOf(mainSets)
			if characters.rangeCount() > largeCharacterRangeCount && !has(characters) {
				out = append(out, largeSet{true, sym, characters})
			}
		}
	}
	return out
}

// validRanges is nfa.rs: fn CharacterSet::ranges. Each stored range is cut to
// the Unicode scalar values it holds: the start moves up past the surrogates
// and the end moves down past them; a range inside the surrogate gap is
// dropped. Ends are inclusive.
func (c CharSet) validRanges() []classRng {
	var out []classRng
	for _, r := range c.r {
		lo, hi := r.start, r.end-1
		if lo >= 0xD800 && lo <= 0xDFFF {
			lo = 0xE000
		}
		if hi >= 0xD800 && hi <= 0xDFFF {
			hi = 0xD7FF
		}
		if lo > hi || lo > 0x10FFFF {
			continue
		}
		if hi > 0x10FFFF {
			hi = 0x10FFFF
		}
		out = append(out, classRng{lo, hi})
	}
	return out
}

// containsCodepointRange is nfa.rs: fn CharacterSet::contains_codepoint_range.
// The Rust code binary-searches with a comparator that orders a probe range
// before the seek range when probe.end <= seek.start, after it when
// probe.start > seek.start, and equal otherwise; Ok(ix) and Err(ix) are both
// used as the index. For sorted disjoint ranges that index is the first range
// whose end is greater than seek.start.
func (c CharSet) containsCodepointRange(start, end uint32) bool {
	ix := sort.Search(len(c.r), func(i int) bool { return c.r[i].end > start })
	if ix >= len(c.r) {
		return false
	}
	r := c.r[ix]
	return r.start <= start && r.end >= end
}

func (c CharSet) containsChar(ch uint32) bool { return c.containsCodepointRange(ch, ch+1) }

// simplifyIgnoring is nfa.rs: fn CharacterSet::simplify_ignoring.
func (c CharSet) simplifyIgnoring(ruledOut CharSet) CharSet {
	var out []urange
	var prev urange
	hasPrev := false
	n := len(c.r)
	for idx := 0; idx <= n; idx++ {
		var rg urange
		isSome := idx < n
		if isSome {
			rg = c.r[idx]
			if ruledOut.containsCodepointRange(rg.start, rg.end) {
				continue
			}
			if hasPrev && ruledOut.containsCodepointRange(prev.end, rg.start) {
				prev.end = rg.end
				continue
			}
		}
		if hasPrev {
			out = append(out, prev)
		}
		if isSome {
			prev, hasPrev = rg, true
		} else {
			hasPrev = false
		}
	}
	return CharSet{out}
}

// plannedTransition is one `if (...) ADVANCE/SKIP(n);` of a lex state.
type plannedTransition struct {
	hasLarge        bool
	large           CharSet
	largeCheckEOF   bool
	asserted        []classRng // valid ranges to render as asserted conditions (validRanges of the set)
	assertedInclude bool
	hasAsserted     bool
	negated         []classRng // valid ranges of the negated conditions
	hasNegated      bool
	action          advanceAction
	hasCondition    bool
}

// statePlan is the rendered form of one lex state.
type statePlan struct {
	mapPairs    [][2]uint32 // ADVANCE_MAP: (key, target), sorted by key; equal keys keep the order of the advances
	transitions []plannedTransition
}

// planState is render.rs: fn add_lex_state, from the first line after the
// accept and end-of-input statements to END_STATE.
func planState(st *lexState, large []largeSet, work *int64) statePlan {
	return planStateWith(st, large, work, ruledOutOf)
}

// leadingRuledOut is the set of the characters of the leading advances (those
// that go into the advance map): the union of their canonical character sets.
// The generator adds them to the ruled-out set one at a time; the Go form joins
// them once (unionOf: the same canonical union), so that the accumulated set is
// not copied for each advance (8,000 advances of one range each copied
// 8,000 x 7,999 / 2 ranges). The ranges collected are counted in
// ops.ruledCopies, and planState charges the work to bRenderWork before it
// calls this.
func leadingRuledOut(advs []charAction) CharSet {
	sets := make([]*CharSet, len(advs))
	n := 0
	for i := range advs {
		sets[i] = &advs[i].chars
		n += advs[i].chars.rangeCount()
	}
	ops.ruledCopies.add(n)
	return unionOf(sets)
}

// ruledOutOf is leadingRuledOut; a variable so that a test can put the earlier
// clone-and-add loop in its place.
var ruledOutOf = leadingRuledOut

// planStateWith is planState with the construction of the ruled-out set of
// the leading advances as a parameter (tests put the earlier clone-and-add
// loop in its place and compare the plans).
func planStateWith(st *lexState, large []largeSet, work *int64, ruled func([]charAction) CharSet) statePlan {
	var plan statePlan
	var ruledOut CharSet
	// Each comparison with the large sets reads all of them (assign and
	// remove_intersection read the whole set, not only the ranges of the
	// advance): count their ranges once for the state, and once more for each
	// advance that is compared (below).
	var largeRanges int64
	for _, ls := range large {
		largeRanges += int64(ls.set.rangeCount())
	}
	*work += int64(len(large))
	bRenderWork.checkWork(*work)
	leadingCount, leadingRanges := 0, 0
	for _, ca := range st.advances {
		simple := ca.action.inMainToken
		if simple {
			for _, r := range ca.chars.validRanges() {
				if !(r.hi <= r.lo+1 && r.hi <= 0xFFFF) {
					simple = false
					break
				}
			}
		}
		if simple {
			leadingCount++
			leadingRanges += ca.chars.rangeCount()
		} else {
			break
		}
	}
	if leadingRanges >= 8 {
		// Charge the map before it is made: the pairs (two for each range), the
		// sort of the pairs, and the join of the leading sets (a sort of the
		// ranges).
		pairs := 2 * leadingRanges
		*work += int64(pairs) + mulWork(pairs, bits.Len(uint(pairs))+1) + mulWork(leadingRanges, bits.Len(uint(leadingRanges))+1)
		bRenderWork.checkWork(*work)
		for _, ca := range st.advances[:leadingCount] {
			for _, r := range ca.chars.validRanges() {
				plan.mapPairs = append(plan.mapPairs, [2]uint32{r.lo, ca.action.state})
				if r.hi > r.lo {
					plan.mapPairs = append(plan.mapPairs, [2]uint32{r.hi, ca.action.state})
				}
			}
		}
		ruledOut = ruled(st.advances[:leadingCount])
		// The pairs are looked up by key (stepAt); the first pair of a key
		// wins, so the sort keeps the order of equal keys.
		sort.SliceStable(plan.mapPairs, func(i, j int) bool { return plan.mapPairs[i][0] < plan.mapPairs[j][0] })
	} else {
		leadingCount = 0
	}
	var charsCopy, largeCopy CharSet
	for _, ca := range st.advances[leadingCount:] {
		*work += int64(1 + ruledOut.rangeCount() + ca.chars.rangeCount())
		bRenderWork.checkWork(*work)
		simplified := ca.chars.simplifyIgnoring(ruledOut)

		bestIx := -1
		var bestAdd, bestRem CharSet
		if simplified.rangeCount() >= largeCharacterRangeCount {
			*work += mulWork(len(large), 1+simplified.rangeCount()) + largeRanges
			bRenderWork.checkWork(*work)
			for ix, ls := range large {
				charsCopy.assign(simplified)
				largeCopy.assign(ls.set)
				inter := charsCopy.removeIntersection(&largeCopy)
				if !inter.isEmpty() {
					additions := charsCopy.simplifyIgnoring(ruledOut)
					removals := largeCopy.simplifyIgnoring(ruledOut)
					total := additions.rangeCount() + removals.rangeCount()
					if total >= simplified.rangeCount() {
						continue
					}
					if bestIx >= 0 {
						if bestAdd.rangeCount()+bestRem.rangeCount() < total {
							continue
						}
					}
					bestIx, bestAdd, bestRem = ix, additions.clone(), removals.clone()
				}
			}
		}

		ops.ruledCopies.add(ruledOut.rangeCount()) // the copy below, charged above
		ruledOut = ruledOut.clone().add(ca.chars)

		pt := plannedTransition{action: ca.action}
		asserted := simplified
		var negated CharSet
		if bestIx >= 0 {
			asserted = bestAdd
			negated = bestRem
			pt.hasLarge = true
			pt.large = large[bestIx].set
			pt.largeCheckEOF = pt.large.containsChar(0)
		}
		hasPositive := pt.hasLarge || !asserted.isEmpty()
		hasNegative := !negated.isEmpty()
		pt.hasCondition = hasPositive || hasNegative
		if !asserted.isEmpty() {
			isIncluded := !asserted.containsChar(0x10FFFF)
			if !isIncluded {
				asserted = asserted.clone().negate().add(charSetFromChar(0))
			}
			pt.hasAsserted, pt.asserted, pt.assertedInclude = true, asserted.validRanges(), isIncluded
		}
		if hasNegative {
			pt.hasNegated, pt.negated = true, negated.validRanges()
		}
		plan.transitions = append(plan.transitions, pt)
	}
	return plan
}

// rangeConditionAt is render.rs: fn add_character_range_conditions, evaluated
// at (eof, look) instead of written. rs are the valid ranges of the set (see
// validRanges), sorted and disjoint. look is the lookahead as C sees it: -1
// for an ill-formed byte, 0 at the end of input.
//
// Go shape: the generator writes one test for each range and the C compiler
// joins them. Evaluated one by one, they cost a pass over all ranges for each
// lookahead. The tests of a range are these: for a range [start, end],
//
//	included: start == 0: !eof && look == 0 when end == 0, else
//	          !eof && look <= end; otherwise look is in [start, end]
//	excluded: look is not in [start, end], except that for start == 0 and
//	          end >= 2 the test is look > end
//
// For look >= 0 they all say whether look is in the range, so a binary search
// finds the one range that can hold look. For look < 0 only a first range that
// starts at 0 can change the answer.
func rangeConditionAt(rs []classRng, isIncluded bool, eof bool, look int64) bool {
	if isIncluded {
		if len(rs) == 0 {
			return false
		}
		rest := rs
		if rs[0].lo == 0 {
			rest = rs[1:]
			if rs[0].hi == 0 {
				if !eof && look == 0 {
					return true
				}
			} else if !eof && look <= int64(rs[0].hi) {
				return true
			}
		}
		return look >= 0 && rangesContain(rest, look)
	}
	if look < 0 {
		return !(len(rs) > 0 && rs[0].lo == 0 && rs[0].hi >= 2)
	}
	return !rangesContain(rs, look)
}

// rangesContain reports whether look is in one of the sorted, disjoint ranges.
func rangesContain(rs []classRng, look int64) bool {
	ops.condProbes.add(bits.Len(uint(len(rs))) + 1)
	i := sort.Search(len(rs), func(i int) bool { return int64(rs[i].hi) >= look })
	return i < len(rs) && int64(rs[i].lo) <= look
}

// stepAt evaluates the rendered state at (eof, look): the first statement that
// holds gives the step. ok is false if no statement holds.
func (st *lexState) stepAt(plan *statePlan, eof bool, look int64) (target uint32, skip bool, ok bool) {
	evals := 0
	target, skip, ok = st.stepAtCount(plan, eof, look, &evals)
	ops.transEvals.add(evals)
	return
}

func (st *lexState) stepAtCount(plan *statePlan, eof bool, look int64, evals *int) (target uint32, skip bool, ok bool) {
	if eof && st.hasEOF {
		return st.eof.state, false, true
	}
	if look >= 0 && look <= 0xFFFFFFFF {
		k := uint32(look)
		if i := sort.Search(len(plan.mapPairs), func(i int) bool { return plan.mapPairs[i][0] >= k }); i < len(plan.mapPairs) && plan.mapPairs[i][0] == k {
			return plan.mapPairs[i][1], false, true
		}
	}
	for _, t := range plan.transitions {
		*evals++
		cond := true
		if t.hasCondition {
			positive := false
			hasPositive := t.hasLarge || t.hasAsserted
			if t.hasLarge {
				in := look >= 0 && t.large.containsChar(uint32(look))
				if t.largeCheckEOF {
					in = !eof && in
				}
				positive = in
			}
			if t.hasAsserted {
				positive = positive || rangeConditionAt(t.asserted, t.assertedInclude, eof, look)
			}
			neg := true
			if t.hasNegated {
				neg = rangeConditionAt(t.negated, false, eof, look)
			}
			switch {
			case hasPositive && t.hasNegated:
				cond = positive && neg
			case hasPositive:
				cond = positive
			default:
				cond = neg
			}
		}
		if cond {
			return t.action.state, !t.action.inMainToken, true
		}
	}
	return 0, false, false
}
