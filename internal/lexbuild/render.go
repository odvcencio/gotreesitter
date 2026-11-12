package lexbuild

import "sort"

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
	work := 0
	has := func(c CharSet) bool {
		work += len(out) * (1 + c.rangeCount())
		bRenderWork.check(work)
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
			var characters CharSet
			for _, ca := range st.advances {
				if ca.action.inMainToken {
					characters = characters.add(ca.chars)
					continue
				}
				if ca.chars.rangeCount() > largeCharacterRangeCount && !has(ca.chars) {
					out = append(out, largeSet{false, Symbol{}, ca.chars.clone()})
				}
			}
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
	asserted        CharSet // ranges to render as asserted conditions
	assertedInclude bool
	hasAsserted     bool
	negated         CharSet
	hasNegated      bool
	action          advanceAction
	hasCondition    bool
}

// statePlan is the rendered form of one lex state.
type statePlan struct {
	mapPairs    [][2]uint32 // ADVANCE_MAP: (key, target)
	transitions []plannedTransition
}

// planState is render.rs: fn add_lex_state, from the first line after the
// accept and end-of-input statements to END_STATE.
func planState(st *lexState, large []largeSet, work *int) statePlan {
	var plan statePlan
	var ruledOut CharSet
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
		for _, ca := range st.advances[:leadingCount] {
			for _, r := range ca.chars.validRanges() {
				plan.mapPairs = append(plan.mapPairs, [2]uint32{r.lo, ca.action.state})
				if r.hi > r.lo {
					plan.mapPairs = append(plan.mapPairs, [2]uint32{r.hi, ca.action.state})
				}
			}
			ruledOut = ruledOut.clone().add(ca.chars)
		}
	} else {
		leadingCount = 0
	}
	var charsCopy, largeCopy CharSet
	for _, ca := range st.advances[leadingCount:] {
		*work += 1 + ruledOut.rangeCount() + ca.chars.rangeCount()
		bRenderWork.check(*work)
		simplified := ca.chars.simplifyIgnoring(ruledOut)

		bestIx := -1
		var bestAdd, bestRem CharSet
		if simplified.rangeCount() >= largeCharacterRangeCount {
			*work += len(large) * (1 + simplified.rangeCount())
			bRenderWork.check(*work)
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
			pt.hasAsserted, pt.asserted, pt.assertedInclude = true, asserted, isIncluded
		}
		if hasNegative {
			pt.hasNegated, pt.negated = true, negated
		}
		plan.transitions = append(plan.transitions, pt)
	}
	return plan
}

// rangeConditionAt is render.rs: fn add_character_range_conditions, evaluated
// at (eof, look) instead of written. look is the lookahead as C sees it: -1 for
// an ill-formed byte, 0 at the end of input.
func rangeConditionAt(chars CharSet, isIncluded bool, eof bool, look int64) bool {
	rs := chars.validRanges()
	if isIncluded {
		res := false
		for _, r := range rs {
			start, end := int64(r.lo), int64(r.hi)
			var v bool
			switch {
			case start == 0:
				if end == 0 {
					v = !eof && look == 0
				} else {
					v = !eof && look <= end
				}
			case end == start:
				v = look == start
			case end == start+1:
				v = look == start || look == end
			default:
				v = start <= look && look <= end
			}
			res = res || v
		}
		return res
	}
	res := true
	for _, r := range rs {
		start, end := int64(r.lo), int64(r.hi)
		var v bool
		switch {
		case end == start:
			v = look != start
		case end == start+1:
			v = look != start && look != end
		case start != 0:
			v = look < start || end < look
		default:
			v = look > end
		}
		res = res && v
	}
	return res
}

// stepAt evaluates the rendered state at (eof, look): the first statement that
// holds gives the step. ok is false if no statement holds.
func (st *lexState) stepAt(plan *statePlan, eof bool, look int64) (target uint32, skip bool, ok bool) {
	if eof && st.hasEOF {
		return st.eof.state, false, true
	}
	for _, p := range plan.mapPairs {
		if int64(p[0]) == look {
			return p[1], false, true
		}
	}
	for _, t := range plan.transitions {
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
