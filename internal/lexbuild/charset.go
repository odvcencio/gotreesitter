package lexbuild

import (
	"math/bits"
	"sort"
)

// charEnd is nfa.rs: const END, one past char::MAX.
const charEnd uint32 = 0x110000

type urange struct{ start, end uint32 } // end exclusive

// CharSet is nfa.rs: struct CharacterSet, a sorted vector of disjoint,
// non-adjacent half-open ranges.
type CharSet struct {
	r []urange
}

func (c CharSet) clone() CharSet { return CharSet{append([]urange(nil), c.r...)} }

// charSetFromChar is nfa.rs: fn CharacterSet::from_char.
func charSetFromChar(ch rune) CharSet {
	return CharSet{[]urange{{uint32(ch), uint32(ch) + 1}}}
}

func (c CharSet) isEmpty() bool     { return len(c.r) == 0 }
func (c CharSet) rangeCount() int   { return len(c.r) }
func (c *CharSet) assign(o CharSet) { c.r = append(c.r[:0], o.r...) }

// negate is nfa.rs: fn CharacterSet::negate.
func (c CharSet) negate() CharSet {
	i := 0
	var previousEnd uint32
	for i < len(c.r) {
		rg := c.r[i]
		start := previousEnd
		previousEnd = rg.end
		if start < rg.start {
			c.r[i] = urange{start, rg.start}
			i++
		} else {
			c.r = append(c.r[:i], c.r[i+1:]...)
		}
	}
	if previousEnd < charEnd {
		c.r = append(c.r, urange{previousEnd, charEnd})
	}
	return c
}

// addRange is nfa.rs: fn CharacterSet::add_range (inclusive ends).
func (c CharSet) addRange(start, end rune) CharSet {
	c.addIntRange(0, uint32(start), uint32(end)+1)
	return c
}

// charSetFromClass is the loop of expand_tokens.rs: fn expand_regex over the
// ranges of a class: one add_range for each range, from an empty set. When the
// ranges are in order of start (a class is canonical, so they are), the
// insertion cursor of add_int_range stays where the last range went: every
// range before it ends before the start of the next range, and the generator's
// scan from index 0 would pass them all. Without that order the scan restarts
// at index 0 for each range, as the generator does.
func charSetFromClass(cls []classRng) CharSet {
	var c CharSet
	ordered := true
	for i := 1; i < len(cls); i++ {
		if cls[i].lo < cls[i-1].lo {
			ordered = false
			break
		}
	}
	idx := 0
	for _, r := range cls {
		at := 0
		if ordered {
			at = idx
		}
		idx = c.addIntRange(at, uint32(rune(r.lo)), uint32(rune(r.hi))+1)
	}
	return c
}

// add is nfa.rs: fn CharacterSet::add.
//
// Go shape: the Rust code inserts the ranges of other one by one, and each
// insertion in the middle moves the tail of the vector. Every CharSet of this
// package is canonical (sorted, disjoint, no two ranges touch): the
// constructors keep that. For a canonical receiver and a canonical other, the
// union is the same canonical set however it is computed, so the ranges of c
// below the first range that other touches stay where they are and the rest is
// merged in one pass (O(len(c) - p + len(other)) for the first touched index
// p). A set that is not canonical in other takes the generator's range by
// range path, addSlow; the receiver must be canonical.
func (c CharSet) add(other CharSet) CharSet {
	if len(other.r) == 0 {
		return c
	}
	if !other.canonical() {
		return c.addSlow(other)
	}
	first := other.r[0].start
	p := sort.Search(len(c.r), func(i int) bool { return c.r[i].end >= first })
	if p == len(c.r) {
		c.r = append(c.r, other.r...)
		return c
	}
	tail := c.r[p:]
	ops.addSteps.add(len(tail) + len(other.r))
	out := make([]urange, 0, len(tail)+len(other.r))
	i, j := 0, 0
	for i < len(tail) || j < len(other.r) {
		var next urange
		if j >= len(other.r) || (i < len(tail) && tail[i].start <= other.r[j].start) {
			next = tail[i]
			i++
		} else {
			next = other.r[j]
			j++
		}
		if n := len(out); n > 0 && next.start <= out[n-1].end {
			out[n-1].end = maxU32(out[n-1].end, next.end)
		} else {
			out = append(out, next)
		}
	}
	c.r = append(c.r[:p], out...)
	return c
}

// unionOf is the set that nfa.rs: fn CharacterSet::add gives when it is called
// for each of the sets in turn on an empty set. The sets must be canonical.
// Adding them one at a time moves the tail of the growing vector for each
// insertion in the middle; here the ranges are collected, sorted by start and
// merged once (O(m log m) for m ranges). The union of canonical sets is the
// same canonical set in either way.
func unionOf(sets []*CharSet) CharSet {
	n := 0
	for _, s := range sets {
		n += len(s.r)
	}
	if n == 0 {
		return CharSet{}
	}
	all := make([]urange, 0, n)
	for _, s := range sets {
		all = append(all, s.r...)
	}
	ops.unionRanges.add(n * (bits.Len(uint(n)) + 1)) // collected, and compared by the sort
	sort.Slice(all, func(i, j int) bool { return all[i].start < all[j].start })
	out := all[:1]
	for _, r := range all[1:] {
		if last := &out[len(out)-1]; r.start <= last.end {
			last.end = maxU32(last.end, r.end)
		} else {
			out = append(out, r)
		}
	}
	return CharSet{out}
}

// addSlow is the generator's add: one add_int_range for each range of other,
// with the insertion cursor.
func (c CharSet) addSlow(other CharSet) CharSet {
	index := 0
	for _, rg := range other.r {
		index = c.addIntRange(index, rg.start, rg.end)
	}
	return c
}

// canonical reports whether the ranges are sorted, not empty, disjoint, and
// do not touch.
func (c CharSet) canonical() bool {
	for i, r := range c.r {
		if r.start >= r.end || (i > 0 && c.r[i-1].end >= r.start) {
			return false
		}
	}
	return true
}

func maxU32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// addIntRange is nfa.rs: fn CharacterSet::add_int_range. The receiver is a
// pointer in effect: c.r is reassigned through the caller's copy, so callers
// that keep using the value must use the returned set (add and addRange do).
func (c *CharSet) addIntRange(i int, start, end uint32) int {
	first := i
	for i < len(c.r) {
		rg := &c.r[i]
		if rg.start > end {
			c.r = append(c.r, urange{})
			copy(c.r[i+1:], c.r[i:])
			c.r[i] = urange{start, end}
			ops.rangeSteps.add(i - first + 1 + len(c.r) - i) // passes, and the ranges moved
			return i
		}
		if rg.end >= start {
			rg.end = maxU32(rg.end, end)
			rg.start = minU32(rg.start, start)
			moved := 0
			for i+1 < len(c.r) && c.r[i+1].start <= c.r[i].end {
				c.r[i].end = maxU32(c.r[i].end, c.r[i+1].end)
				c.r = append(c.r[:i+1], c.r[i+2:]...)
				moved += len(c.r) - i
			}
			ops.rangeSteps.add(i - first + 1 + moved)
			return i
		}
		i++
	}
	c.r = append(c.r, urange{start, end})
	ops.rangeSteps.add(i - first + 1)
	return i
}

// doesIntersect is nfa.rs: fn CharacterSet::does_intersect.
//
// Go shape: the generator walks both vectors from their starts. Here each range
// of the shorter set is looked up in the longer one by binary search, so a short
// set against a long one reads the short set once (O(min x log max)). Both sets
// are canonical, and the answer (is there a pair of ranges that overlap) is the
// same.
func (c CharSet) doesIntersect(o CharSet) bool {
	a, b := c.r, o.r
	if len(a) > len(b) {
		a, b = b, a
	}
	for _, r := range a {
		i := sort.Search(len(b), func(i int) bool { return b[i].end > r.start })
		if i < len(b) && b[i].start < r.end {
			return true
		}
	}
	return false
}

// removeIntersection is nfa.rs: fn CharacterSet::remove_intersection. It
// returns the common characters and removes them from both operands.
func (c *CharSet) removeIntersection(other *CharSet) CharSet {
	inter, _ := c.removeIntersectionSteps(other)
	return inter
}

// removeIntersectionSteps is remove_intersection with the work counted. Go
// shape: the Rust code edits both vectors in place, and removes or inserts a
// range at the cursor, which moves the rest of the vector. Here the two
// remainders are written to new vectors in one pass: the ranges before the
// cursor are final, the range at the cursor is held in a local copy, and a
// removed range is not written. The three results and their order are the
// generator's. The second result is the number of loop passes plus the ranges
// copied at the end, at most len(c) + len(other) + 1 (the caller counts it).
func (c *CharSet) removeIntersectionSteps(other *CharSet) (CharSet, int) {
	var inter, outC, outO []urange
	li, ri := 0, 0
	var l, r urange
	if len(c.r) > 0 {
		l = c.r[0]
	}
	if len(other.r) > 0 {
		r = other.r[0]
	}
	steps := 1
	// advL moves the cursor of c to the next range and writes the range it
	// leaves when keep is true; advR does the same for other.
	advL := func(keep bool) {
		if keep {
			outC = append(outC, l)
		}
		li++
		if li < len(c.r) {
			l = c.r[li]
		}
	}
	advR := func(keep bool) {
		if keep {
			outO = append(outO, r)
		}
		ri++
		if ri < len(other.r) {
			r = other.r[ri]
		}
	}
	for li < len(c.r) && ri < len(other.r) {
		steps++
		switch {
		case l.start < r.start:
			// [ L ]
			//     [ R ]
			if l.end <= r.start {
				advL(true)
				continue
			}
			switch {
			case l.end < r.end:
				inter = append(inter, urange{r.start, l.end})
				l.end, r.start = r.start, l.end
				advL(true)
			case l.end == r.end:
				inter = append(inter, r)
				l.end = r.start
				advR(false)
			default:
				inter = append(inter, r)
				outC = append(outC, urange{l.start, r.start})
				l.start = r.end
				advR(false)
			}
		case l.start == r.start:
			switch {
			case l.end < r.end:
				inter = append(inter, urange{l.start, l.end})
				r.start = l.end
				advL(false)
			case l.end == r.end:
				inter = append(inter, l)
				advL(false)
				advR(false)
			default:
				inter = append(inter, r)
				l.start = r.end
				advR(false)
			}
		default: // l.start > r.start
			if l.start >= r.end {
				advR(true)
				continue
			}
			switch {
			case l.end < r.end:
				inter = append(inter, l)
				outO = append(outO, urange{r.start, l.start})
				r.start = l.end
				advL(false)
			case l.end == r.end:
				inter = append(inter, l)
				r.end = l.start
				advL(false)
			default:
				inter = append(inter, urange{l.start, r.end})
				l.start, r.end = r.end, l.start
				advR(true)
			}
		}
	}
	// The range at the cursor of an operand that is not used up has its
	// edited value; the ranges after it are as they were.
	if li < len(c.r) {
		outC = append(outC, l)
		outC = append(outC, c.r[li+1:]...)
		steps += len(c.r) - li
	}
	if ri < len(other.r) {
		outO = append(outO, r)
		outO = append(outO, other.r[ri+1:]...)
		steps += len(other.r) - ri
	}
	c.r, other.r = outC, outO
	ops.mergeSteps.add(steps)
	return CharSet{inter}, steps
}

func (c CharSet) charCount() uint64 {
	var n uint64
	for _, r := range c.r {
		n += uint64(r.end - r.start)
	}
	return n
}

// compareCharSet is nfa.rs: impl Ord for CharacterSet. It returns -1, 0 or 1.
// Sets compare by character count, then range by range by length, then by
// start (equal-length ranges compare element by element, which is the start).
func compareCharSet(a, b CharSet) int {
	ca, cb := a.charCount(), b.charCount()
	if ca != cb {
		if ca < cb {
			return -1
		}
		return 1
	}
	return compareCharSetSameCount(a, b)
}

// compareCharSetSameCount is compareCharSet for two sets of the same count.
func compareCharSetSameCount(a, b CharSet) int {
	n := len(a.r)
	if len(b.r) < n {
		n = len(b.r)
	}
	for i := 0; i < n; i++ {
		la, lb := a.r[i].end-a.r[i].start, b.r[i].end-b.r[i].start
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		if a.r[i].start != b.r[i].start {
			if a.r[i].start < b.r[i].start {
				return -1
			}
			return 1
		}
	}
	return 0
}

func equalCharSet(a, b CharSet) bool {
	if len(a.r) != len(b.r) {
		return false
	}
	for i := range a.r {
		if a.r[i] != b.r[i] {
			return false
		}
	}
	return true
}
