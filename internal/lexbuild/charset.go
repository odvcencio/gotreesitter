package lexbuild

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

// add is nfa.rs: fn CharacterSet::add.
func (c CharSet) add(other CharSet) CharSet {
	index := 0
	for _, rg := range other.r {
		index = c.addIntRange(index, rg.start, rg.end)
	}
	return c
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
	for i < len(c.r) {
		rg := &c.r[i]
		if rg.start > end {
			c.r = append(c.r, urange{})
			copy(c.r[i+1:], c.r[i:])
			c.r[i] = urange{start, end}
			return i
		}
		if rg.end >= start {
			rg.end = maxU32(rg.end, end)
			rg.start = minU32(rg.start, start)
			for i+1 < len(c.r) && c.r[i+1].start <= c.r[i].end {
				c.r[i].end = maxU32(c.r[i].end, c.r[i+1].end)
				c.r = append(c.r[:i+1], c.r[i+2:]...)
			}
			return i
		}
		i++
	}
	c.r = append(c.r, urange{start, end})
	return i
}

// doesIntersect is nfa.rs: fn CharacterSet::does_intersect.
func (c CharSet) doesIntersect(o CharSet) bool {
	li, ri := 0, 0
	for li < len(c.r) && ri < len(o.r) {
		l, r := c.r[li], o.r[ri]
		if l.end <= r.start {
			li++
		} else if l.start >= r.end {
			ri++
		} else {
			return true
		}
	}
	return false
}

func (c *CharSet) removeAt(i int) { c.r = append(c.r[:i], c.r[i+1:]...) }
func (c *CharSet) insertAt(i int, r urange) {
	c.r = append(c.r, urange{})
	copy(c.r[i+1:], c.r[i:])
	c.r[i] = r
}

// removeIntersection is nfa.rs: fn CharacterSet::remove_intersection. It
// returns the common characters and removes them from both operands.
func (c *CharSet) removeIntersection(other *CharSet) CharSet {
	var inter []urange
	li, ri := 0, 0
	for li < len(c.r) && ri < len(other.r) {
		l := &c.r[li]
		r := &other.r[ri]
		switch {
		case l.start < r.start:
			// [ L ]
			//     [ R ]
			if l.end <= r.start {
				li++
				continue
			}
			switch {
			case l.end < r.end:
				inter = append(inter, urange{r.start, l.end})
				l.end, r.start = r.start, l.end
				li++
			case l.end == r.end:
				inter = append(inter, *r)
				l.end = r.start
				other.removeAt(ri)
			default:
				inter = append(inter, *r)
				nr := urange{l.start, r.start}
				l.start = r.end
				c.insertAt(li, nr)
				other.removeAt(ri)
				li++
			}
		case l.start == r.start:
			switch {
			case l.end < r.end:
				inter = append(inter, urange{l.start, l.end})
				r.start = l.end
				c.removeAt(li)
			case l.end == r.end:
				inter = append(inter, *l)
				c.removeAt(li)
				other.removeAt(ri)
			default:
				inter = append(inter, *r)
				l.start = r.end
				other.removeAt(ri)
			}
		default: // l.start > r.start
			if l.start >= r.end {
				ri++
				continue
			}
			switch {
			case l.end < r.end:
				inter = append(inter, *l)
				nr := urange{r.start, l.start}
				r.start = l.end
				other.insertAt(ri, nr)
				c.removeAt(li)
				ri++
			case l.end == r.end:
				inter = append(inter, *l)
				r.end = l.start
				c.removeAt(li)
			default:
				inter = append(inter, urange{l.start, r.end})
				l.start, r.end = r.end, l.start
				ri++
			}
		}
	}
	return CharSet{inter}
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
