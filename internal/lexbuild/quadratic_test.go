package lexbuild

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"
)

// This file tests the functions that were rewritten to do work linear in their
// input (the review of draft pull request 1618, 4th round, and the search that
// followed it). Each rewritten function is compared with a copy of the earlier
// function kept here (the "Old" functions), on random inputs; and each input
// that made an earlier version quadratic inside the caps is run and its
// operation counter (opcount.go) is compared with a bound that the earlier
// version missed by a factor of 20 or more. No test here asserts elapsed time.

// ---- the earlier functions ----

func (c *CharSet) removeAtOld(i int) { c.r = append(c.r[:i], c.r[i+1:]...) }
func (c *CharSet) insertAtOld(i int, r urange) {
	c.r = append(c.r, urange{})
	copy(c.r[i+1:], c.r[i:])
	c.r[i] = r
}

// removeIntersectionOld is nfa.rs: fn CharacterSet::remove_intersection as
// ported before the fix: edits both vectors in place.
func (c *CharSet) removeIntersectionOld(other *CharSet) CharSet {
	var inter []urange
	li, ri := 0, 0
	for li < len(c.r) && ri < len(other.r) {
		l := &c.r[li]
		r := &other.r[ri]
		switch {
		case l.start < r.start:
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
				other.removeAtOld(ri)
			default:
				inter = append(inter, *r)
				nr := urange{l.start, r.start}
				l.start = r.end
				c.insertAtOld(li, nr)
				other.removeAtOld(ri)
				li++
			}
		case l.start == r.start:
			switch {
			case l.end < r.end:
				inter = append(inter, urange{l.start, l.end})
				r.start = l.end
				c.removeAtOld(li)
			case l.end == r.end:
				inter = append(inter, *l)
				c.removeAtOld(li)
				other.removeAtOld(ri)
			default:
				inter = append(inter, *r)
				l.start = r.end
				other.removeAtOld(ri)
			}
		default:
			if l.start >= r.end {
				ri++
				continue
			}
			switch {
			case l.end < r.end:
				inter = append(inter, *l)
				nr := urange{r.start, l.start}
				r.start = l.end
				other.insertAtOld(ri, nr)
				c.removeAtOld(li)
				ri++
			case l.end == r.end:
				inter = append(inter, *l)
				r.end = l.start
				c.removeAtOld(li)
			default:
				inter = append(inter, urange{l.start, r.end})
				l.start, r.end = r.end, l.start
				ri++
			}
		}
	}
	return CharSet{inter}
}

// addRangeLoopOld is the loop of expand_regex over the ranges of a class: one
// add_range for each, each scan from index 0. It returns the set and the
// number of iterations of the scan of add_int_range.
func addRangeLoopOld(cls []classRng) (CharSet, int) {
	var c CharSet
	iters := 0
	for _, r := range cls {
		start, end := uint32(rune(r.lo)), uint32(rune(r.hi))+1
		i := 0
		done := false
		for i < len(c.r) {
			iters++
			rg := &c.r[i]
			if rg.start > end {
				c.r = append(c.r, urange{})
				copy(c.r[i+1:], c.r[i:])
				c.r[i] = urange{start, end}
				done = true
				break
			}
			if rg.end >= start {
				rg.end = maxU32(rg.end, end)
				rg.start = minU32(rg.start, start)
				for i+1 < len(c.r) && c.r[i+1].start <= c.r[i].end {
					c.r[i].end = maxU32(c.r[i].end, c.r[i+1].end)
					c.r = append(c.r[:i+1], c.r[i+2:]...)
				}
				done = true
				break
			}
			i++
		}
		if !done {
			c.r = append(c.r, urange{start, end})
		}
	}
	return c, iters
}

func filterPatternFlagsOld(flags string) string {
	out := ""
	for _, c := range flags {
		if c == 'i' {
			out += "i"
		}
	}
	return out
}

func hirConcatOfOld(items []*hir) *hir {
	var flat []*hir
	for _, it := range items {
		switch it.kind {
		case hirEmpty:
		case hirConcat:
			flat = append(flat, it.subs...)
		default:
			flat = append(flat, it)
		}
	}
	var merged []*hir
	for _, it := range flat {
		if it.kind == hirLiteral && len(merged) > 0 && merged[len(merged)-1].kind == hirLiteral {
			last := merged[len(merged)-1]
			merged[len(merged)-1] = &hir{kind: hirLiteral, lit: append(append([]rune(nil), last.lit...), it.lit...)}
			continue
		}
		merged = append(merged, it)
	}
	switch len(merged) {
	case 0:
		return &hir{kind: hirEmpty}
	case 1:
		return merged[0]
	}
	return &hir{kind: hirConcat, subs: merged}
}

func classDifferenceOld(a, b []classRng) []classRng {
	create := func(lo, hi uint32) classRng {
		if lo <= hi {
			return classRng{lo, hi}
		}
		return classRng{hi, lo}
	}
	var out []classRng
	for _, r := range a {
		cur := r
		gone := false
		for _, s := range b {
			if s.hi < cur.lo {
				continue
			}
			if s.lo > cur.hi {
				break
			}
			if s.lo <= cur.lo && cur.hi <= s.hi {
				gone = true
				break
			}
			addLower := s.lo > cur.lo
			addUpper := s.hi < cur.hi
			var first, second *classRng
			if addLower {
				x := create(cur.lo, charDecrement(s.lo))
				first = &x
			}
			if addUpper {
				x := create(charIncrement(s.hi), cur.hi)
				if first == nil {
					first = &x
				} else {
					second = &x
				}
			}
			if second != nil {
				out = append(out, *first)
				cur = *second
			} else {
				cur = *first
			}
		}
		if !gone {
			out = append(out, cur)
		}
	}
	return out
}

// rangeConditionAtOld is render.rs: fn add_character_range_conditions as
// evaluated before the fix: one test for each range.
func rangeConditionAtOld(chars CharSet, isIncluded bool, eof bool, look int64) bool {
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

func (c CharSet) doesIntersectOld(o CharSet) bool {
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

func terminalIndicesOld(s *TokenSet) []int {
	var out []int
	for i, w := range s.terminals {
		for b := 0; b < 64; b++ {
			if w&(1<<uint(b)) != 0 {
				out = append(out, i*64+b)
			}
		}
	}
	return out
}

// getFollowingTokensOld is build_tables.rs: fn get_following_tokens as ported
// before the fix: one addition for each pair of neighbouring steps and each
// terminal of the LAST set of the left one.
func getFollowingTokensOld(sg *SyntaxGrammar, lg *LexicalGrammar, fl *firstLast) []TokenSet {
	n := len(lg.Variables)
	result := make([]TokenSet, n)
	var all TokenSet
	for i := 0; i < n; i++ {
		all.Insert(terminalSym(i))
	}
	for _, p := range sg.Productions {
		steps := p.Steps
		for i := 1; i < len(steps); i++ {
			left := fl.last[steps[i-1].Symbol]
			right := fl.first[steps[i].Symbol]
			rightReserved := &sg.ReservedWordSets[fl.reserved[steps[i].Symbol]]
			for _, lt := range left.symbols() {
				if lt.IsTerminal() {
					result[lt.Index].insertAllTerminals(right)
					result[lt.Index].insertAllTerminals(rightReserved)
				}
			}
		}
	}
	for _, extra := range sg.ExtraSymbols {
		if extra.IsTerminal() {
			for i := range result {
				result[i].Insert(extra)
			}
			result[extra.Index] = all.clone()
		}
	}
	return result
}

// ---- random inputs ----

// randCanon returns a canonical set (sorted, disjoint, not touching) of up to
// n ranges inside [0, max).
func randCanon(rng *rand.Rand, n int, max uint32) CharSet {
	var c CharSet
	pos := uint32(rng.Intn(4))
	for i := 0; i < n && pos < max-2; i++ {
		l := uint32(1 + rng.Intn(5))
		if pos+l >= max {
			break
		}
		c.r = append(c.r, urange{pos, pos + l})
		pos += l + uint32(1+rng.Intn(5))
	}
	return c
}

func randRanges(rng *rand.Rand, n int, max uint32) []classRng {
	out := make([]classRng, n)
	for i := range out {
		lo := uint32(rng.Intn(int(max)))
		out[i] = classRng{lo, lo + uint32(rng.Intn(6))}
	}
	return out
}

func canonClassRanges(rng *rand.Rand, n int, max uint32, gap bool) []classRng {
	var out []classRng
	pos := uint32(rng.Intn(4))
	for i := 0; i < n; i++ {
		l := uint32(rng.Intn(6))
		if gap && rng.Intn(8) == 0 {
			l = uint32(rng.Intn(0x3000))
		}
		if pos+l >= max {
			break
		}
		out = append(out, classRng{pos, pos + l})
		pos += l + uint32(2+rng.Intn(6))
	}
	return out
}

func equalRangeList(a, b []classRng) bool {
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

// ---- equality with the earlier functions ----

func TestFilterPatternFlagsMatchesAppendLoop(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []string{"i", "I", "x", "ii", "é", "ı", "", "m", "s", "i"}
	for n := 0; n < 3000; n++ {
		var sb strings.Builder
		for k := rng.Intn(60); k > 0; k-- {
			sb.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		s := sb.String()
		if got, want := filterPatternFlags(s), filterPatternFlagsOld(s); got != want {
			t.Fatalf("flags %q: got %q, want %q", s, got, want)
		}
	}
}

func TestCharSetFromClassMatchesRestartLoop(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for n := 0; n < 4000; n++ {
		var cls []classRng
		switch n % 3 {
		case 0: // canonical, as the parser gives them
			cls = canonClassRanges(rng, rng.Intn(40), 5000, false)
		case 1: // sorted by start, overlapping and touching
			cls = randRanges(rng, rng.Intn(40), 200)
			sort.Slice(cls, func(i, j int) bool { return cls[i].lo < cls[j].lo })
		default: // not sorted
			cls = randRanges(rng, rng.Intn(40), 200)
		}
		want, _ := addRangeLoopOld(cls)
		if got := charSetFromClass(cls); !equalCharSet(got, want) {
			t.Fatalf("class %v: got %v, want %v", cls, got.r, want.r)
		}
	}
}

func TestRemoveIntersectionMatchesInPlaceVersion(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for n := 0; n < 20000; n++ {
		max := uint32(30 + rng.Intn(300))
		a, b := randCanon(rng, rng.Intn(25), max), randCanon(rng, rng.Intn(25), max)
		if n%7 == 0 {
			b = a.clone() // identical sets: the case of the review
		}
		a1, b1 := a.clone(), b.clone()
		a2, b2 := a.clone(), b.clone()
		want := a1.removeIntersectionOld(&b1)
		got, steps := a2.removeIntersectionSteps(&b2)
		if !equalCharSet(got, want) || !equalCharSet(a2, a1) || !equalCharSet(b2, b1) {
			t.Fatalf("a=%v b=%v\n got inter=%v a=%v b=%v\nwant inter=%v a=%v b=%v", a.r, b.r, got.r, a2.r, b2.r, want.r, a1.r, b1.r)
		}
		if steps > len(a.r)+len(b.r)+1 {
			t.Fatalf("steps %d for %d and %d ranges", steps, len(a.r), len(b.r))
		}
		if !got.canonical() || !a2.canonical() || !b2.canonical() {
			t.Fatalf("not canonical: a=%v b=%v", a.r, b.r)
		}
	}
}

func TestAddMatchesRangeByRangeAdd(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for n := 0; n < 20000; n++ {
		max := uint32(30 + rng.Intn(300))
		a, b := randCanon(rng, rng.Intn(25), max), randCanon(rng, rng.Intn(25), max)
		if n%5 == 0 { // other not canonical: the generator's path
			b = CharSet{[]urange{{5, 9}, {2, 4}, {3, 20}}}
		}
		want := a.clone().addSlow(b)
		if got := a.clone().add(b); !equalCharSet(got, want) {
			t.Fatalf("a=%v b=%v got %v want %v", a.r, b.r, got.r, want.r)
		}
	}
}

func TestUnionOfMatchesAddOneByOne(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for n := 0; n < 5000; n++ {
		var sets []*CharSet
		var want CharSet
		for k := rng.Intn(8); k > 0; k-- {
			s := randCanon(rng, rng.Intn(20), 200)
			sets = append(sets, &s)
			want = want.addSlow(s)
		}
		if got := unionOf(sets); !equalCharSet(got, want) {
			t.Fatalf("got %v want %v", got.r, want.r)
		}
	}
}

func TestHirConcatOfMatchesPairwiseJoin(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	mk := func() *hir {
		switch rng.Intn(6) {
		case 0:
			return &hir{kind: hirEmpty}
		case 1:
			return &hir{kind: hirClass, class: []classRng{{1, 5}}}
		case 2:
			return &hir{kind: hirConcat, subs: []*hir{{kind: hirLiteral, lit: []rune("xy")}, {kind: hirClass, class: []classRng{{2, 3}}}, {kind: hirLiteral, lit: []rune("z")}}}
		default:
			return &hir{kind: hirLiteral, lit: []rune(string(rune('a' + rng.Intn(4))))}
		}
	}
	for n := 0; n < 5000; n++ {
		var items []*hir
		for k := rng.Intn(12); k > 0; k-- {
			items = append(items, mk())
		}
		if got, want := hirConcatOf(items), hirConcatOfOld(items); !hirEqual(got, want) {
			t.Fatalf("items differ")
		}
	}
}

func TestClassDifferenceMatchesRestartScan(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for n := 0; n < 20000; n++ {
		max := uint32(60 + rng.Intn(400))
		a := canonClassRanges(rng, rng.Intn(25), max, false)
		b := canonClassRanges(rng, rng.Intn(25), max, false)
		switch n % 5 {
		case 1: // ranges across the surrogate gap
			a = canonClassRanges(rng, rng.Intn(25), 0x12000, true)
			b = canonClassRanges(rng, rng.Intn(25), 0x12000, true)
		case 2: // not canonical
			a = randRanges(rng, rng.Intn(15), 100)
			b = randRanges(rng, rng.Intn(15), 100)
		case 3: // exotic ranges as in the case fold
			b = []classRng{{0x17F, 0x17F}, {0x212A, 0x212A}}
			a = canonClassRanges(rng, rng.Intn(25), 0x2200, true)
		}
		if got, want := classDifference(a, b), classDifferenceOld(a, b); !equalRangeList(got, want) {
			t.Fatalf("a=%v b=%v got %v want %v", a, b, got, want)
		}
	}
}

func TestRangeConditionMatchesRangeByRange(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	for n := 0; n < 20000; n++ {
		var c CharSet
		switch n % 4 {
		case 0:
			c = randCanon(rng, rng.Intn(10), 40)
		case 1: // starts at 0
			c = randCanon(rng, rng.Intn(10), 40)
			if len(c.r) > 0 {
				c.r[0].start = 0
			}
		case 2: // around the surrogate gap
			c = CharSet{[]urange{{0xD000, 0xD900}, {0xDC00, 0xE100}, {0xE200, 0xE300}}}
		default: // around the top
			c = CharSet{[]urange{{0x10FFF0, 0x110000}}}
			if rng.Intn(2) == 0 {
				c = CharSet{[]urange{{0, 1}, {3, 0x110000}}}
			}
		}
		rs := c.validRanges()
		for _, look := range []int64{-2, -1, 0, 1, 2, 3, 4, 5, 9, 10, 11, 20, 39, 40, 41, 0xD7FF, 0xD800, 0xE000, 0xE0FF, 0xE250, 0x10FFEF, 0x10FFF0, 0x10FFFF, 0x110000} {
			for _, inc := range []bool{true, false} {
				for _, eof := range []bool{true, false} {
					if got, want := rangeConditionAt(rs, inc, eof, look), rangeConditionAtOld(c, inc, eof, look); got != want {
						t.Fatalf("set %v inc=%v eof=%v look=%d: got %v want %v", c.r, inc, eof, look, got, want)
					}
				}
			}
		}
	}
}

func TestDoesIntersectMatchesMerge(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	for n := 0; n < 20000; n++ {
		max := uint32(30 + rng.Intn(300))
		a, b := randCanon(rng, rng.Intn(25), max), randCanon(rng, rng.Intn(25), max)
		if got, want := a.doesIntersect(b), a.doesIntersectOld(b); got != want {
			t.Fatalf("a=%v b=%v got %v want %v", a.r, b.r, got, want)
		}
	}
}

func TestTerminalIndicesMatchesBitLoop(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	for n := 0; n < 2000; n++ {
		var s TokenSet
		for k := rng.Intn(30); k > 0; k-- {
			s.Insert(terminalSym(rng.Intn(500)))
			s.Insert(externalSym(rng.Intn(200)))
		}
		got, want := s.terminalIndices(), terminalIndicesOld(&s)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// getFollowingTokens joins the sets that follow a left symbol before it adds
// them to the terminals of LAST: the result sets must be the same, with the
// same number of words.
func TestGetFollowingTokensMatchesPairwiseAdd(t *testing.T) {
	grammars := []string{
		`{"name":"t","word":"id","extras":[{"type":"PATTERN","value":"\\s"}],"rules":{"source":{"type":"REPEAT1","content":{"type":"SYMBOL","name":"item"}},` +
			`"item":{"type":"CHOICE","members":[{"type":"SEQ","members":[{"type":"STRING","value":"if"},{"type":"SYMBOL","name":"id"},{"type":"STRING","value":"{"},{"type":"SYMBOL","name":"source"},{"type":"STRING","value":"}"}]},` +
			`{"type":"SEQ","members":[{"type":"SYMBOL","name":"id"},{"type":"STRING","value":"="},{"type":"SYMBOL","name":"num"}]}]},` +
			`"id":{"type":"PATTERN","value":"[a-z]+"},"num":{"type":"PATTERN","value":"[0-9]+"}}}`,
		`{"name":"t","rules":{"source":{"type":"SEQ","members":[{"type":"SYMBOL","name":"a"},{"type":"SYMBOL","name":"b"},{"type":"SYMBOL","name":"a"}]},` +
			`"a":{"type":"CHOICE","members":[{"type":"STRING","value":"x"},{"type":"STRING","value":"y"}]},"b":{"type":"REPEAT1","content":{"type":"STRING","value":"z"}}}}`,
	}
	for gi, g := range grammars {
		p, err := Prepare([]byte(g))
		if err != nil {
			t.Fatalf("grammar %d: %v", gi, err)
		}
		fl := newFirstLast(p.Syntax, p.Lex)
		got, want := getFollowingTokens(p.Syntax, p.Lex, fl), getFollowingTokensOld(p.Syntax, p.Lex, fl)
		if len(got) != len(want) {
			t.Fatalf("grammar %d: %d sets, want %d", gi, len(got), len(want))
		}
		for i := range got {
			if len(got[i].terminals) != len(want[i].terminals) || !got[i].Equal(&want[i]) {
				t.Fatalf("grammar %d token %d: got %+v want %+v", gi, i, got[i], want[i])
			}
		}
	}
}

// ---- inputs that made an earlier version quadratic inside the caps ----
//
// Each test runs the input, reads the operation counters of opcount.go and
// compares the count with a bound. The bound is a small multiple of the size of
// the input (a linear version meets it) and far below the count of the earlier
// version (the figure is in the comment of each test). The elapsed time is
// printed for the reader and never asserted: a paused or loaded runner must not
// fail a test.

func jsonPatternGrammar(pattern string) []byte { return sizeGrammar(pattern) }

func classOfSingleCodePoints(n int) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < n; i++ {
		sb.WriteRune(rune(0x10000 + 2*i))
	}
	sb.WriteString("]")
	return sb.String()
}

// counted runs f with the counters at zero, prints its elapsed time, and
// returns the value of the counter c afterwards.
func counted(t *testing.T, what string, c *opCounter, f func()) int64 {
	t.Helper()
	resetOps()
	start := time.Now()
	f()
	d := time.Since(start)
	n := c.n.Load()
	t.Logf("%s: count %d (elapsed %v, printed only); all counters: flags %d, insert %d, merge %d, add %d, union %d, concat %d, classdiff %d, groups %d, errbytes %d, evals %d, probes %d",
		what, n, d, ops.flagBytes.n.Load(), ops.rangeSteps.n.Load(), ops.mergeSteps.n.Load(), ops.addSteps.n.Load(), ops.unionRanges.n.Load(), ops.concatRunes.n.Load(),
		ops.classSteps.n.Load(), ops.groupRunes.n.Load(), ops.errBytes.n.Load(), ops.transEvals.n.Load(), ops.condProbes.n.Load())
	return n
}

// atMost fails if n is above bound. oldCount is the figure of the earlier version, for the message.
func atMost(t *testing.T, what string, n, bound, oldCount int64) {
	t.Helper()
	if n > bound {
		t.Fatalf("%s: count %d is above the bound %d (the earlier quadratic version counted %d)", what, n, bound, oldCount)
	}
	if bound*20 > oldCount {
		t.Fatalf("%s: the bound %d is not 20 times below the earlier count %d; the test cannot tell the two apart", what, bound, oldCount)
	}
}

// Finding 1 (3rd round): a PATTERN with 1,000,000 flag characters. The earlier
// loop copied the prefix for each flag: 5 x 10^11 bytes.
func TestPatternFlagsAreLinear(t *testing.T) {
	const n = 1000000
	g := []byte(`{"name":"t","rules":{"source":{"type":"PATTERN","value":"a","flags":"` + strings.Repeat("i", n) + `"}}}`)
	got := counted(t, "Prepare, 1,000,000 flags", &ops.flagBytes, func() {
		if _, err := Prepare(g); err != nil {
			t.Fatal(err)
		}
	})
	atMost(t, "bytes read and written", got, 4*n, int64(n)*int64(n)/2)
}

// Finding 2 (3rd round): a class of 65,500 separate code points, repeated 64
// times. The earlier scan from index 0 made 64 x 65,500 x 65,499 / 2 =
// 137,285,904,000 iterations in Prepare.
func TestClassOfManyRangesIsLinear(t *testing.T) {
	const n = 65500
	g := jsonPatternGrammar(classOfSingleCodePoints(n) + "{64}")
	if len(g)-len(`{"name":"t","rules":{"source":{"type":"PATTERN","value":""}}}`) > 1<<18 {
		t.Fatal("the pattern is over the cap")
	}
	got := counted(t, "Prepare, class of 65,500 ranges x 64", &ops.rangeSteps, func() {
		if _, err := Prepare(g); err != nil {
			t.Fatal(err)
		}
	})
	atMost(t, "range insertion steps", got, 8*n*64, 64*int64(n)*int64(n-1)/2)
	// The iterations of the earlier scan for one copy, counted on a smaller class
	// (the count is n(n-1)/2 for n separate ranges, so for the review's class it
	// is 2,145,092,250 for one copy and 137,285,904,000 for 64).
	var cls []classRng
	for i := 0; i < 2000; i++ {
		cls = append(cls, classRng{uint32(0x10000 + 2*i), uint32(0x10000 + 2*i)})
	}
	if _, iters := addRangeLoopOld(cls); iters != 2000*1999/2 {
		t.Fatalf("earlier scan: %d iterations, want %d", iters, 2000*1999/2)
	}
}

// Finding 3 (3rd round): rendering the 64 advancing states of the same input.
// The earlier removal copied 274,571,808,000 range elements.
func TestRenderingManyRangesIsLinear(t *testing.T) {
	g := jsonPatternGrammar(classOfSingleCodePoints(65500) + "{64}")
	p, err := Prepare(g)
	if err != nil {
		t.Fatal(err)
	}
	got := counted(t, "BuildUnmerged, class of 65,500 ranges x 64", &ops.mergeSteps, func() {
		tb, _, err := p.BuildUnmerged(tokenZero())
		if err != nil {
			t.Fatal(err)
		}
		if tb.NumStates() < 64 {
			t.Fatalf("%d states", tb.NumStates())
		}
	})
	atMost(t, "range merge steps", got, 16*65500*64, 274571808000)
}

// A pattern of 262,000 literal characters: the earlier join of literals copied
// the run for each character (3.4 x 10^10 runes).
func TestLongLiteralIsLinear(t *testing.T) {
	const n = 262000
	got := counted(t, "Prepare, 262,000 literal characters", &ops.concatRunes, func() {
		if _, err := Prepare(jsonPatternGrammar(strings.Repeat("a", n))); err != nil {
			t.Fatal(err)
		}
	})
	atMost(t, "runes and nodes copied", got, 4*n, int64(n)*int64(n)/2)
}

// A class difference of two classes of 30,000 ranges that interleave. The
// earlier scan from the start of the right operand made 30,000 x 30,000 / 2 =
// 4.5 x 10^8 passes.
func TestClassDifferenceIsLinear(t *testing.T) {
	var a, b strings.Builder
	a.WriteString("[")
	b.WriteString("[")
	for i := 0; i < 30000; i++ {
		a.WriteRune(rune(0x10000 + 3*i))
		b.WriteRune(rune(0x10000 + 3*i + 1))
	}
	a.WriteString("--")
	b.WriteString("]]")
	got := counted(t, "Prepare, difference of classes of 30,000 ranges", &ops.classSteps, func() {
		if _, err := Prepare(jsonPatternGrammar(a.String() + b.String())); err != nil {
			t.Fatal(err)
		}
	})
	atMost(t, "ranges passed or compared", got, 8*30000, 30000*30000/2)
}

// Many one-character alternatives of two characters each, whose starting
// characters arrive in descending order: the earlier insertion moved the
// ranges above the place for each of them (n^2 / 2 = 1.25 x 10^7 moves for the
// starting sets of 5,000 alternatives; 2.6 x 10^10 for 32 patterns of 40,000).
func TestStartingCharsAreLinear(t *testing.T) {
	var sb strings.Builder
	const n = 5000
	for i := n - 1; i >= 0; i-- {
		if i != n-1 {
			sb.WriteString("|")
		}
		sb.WriteRune(rune(0x10000 + 3*i))
		sb.WriteString("a")
	}
	p, err := Prepare(jsonPatternGrammar(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	// The sets are joined by unionOf (a sort of m ranges, m log m comparisons).
	got := counted(t, "Analyze, 5,000 alternatives", &ops.unionRanges, func() {
		if _, err := p.Analyze(nil); err != nil {
			t.Fatal(err)
		}
	})
	atMost(t, "ranges joined (Analyze)", got, 2*n*14, n*n/2)
	moves := counted(t, "BuildUnmerged, 5,000 alternatives", &ops.addSteps, func() {
		if _, _, err := p.BuildUnmerged(tokenZero()); err != nil {
			t.Fatal(err)
		}
	})
	atMost(t, "ranges read by add (BuildUnmerged)", moves, 16*n, n*n/2)
	// The grouping of the transitions compares each new transition with the
	// entries before it, as the generator does (n^2 / 2 pairs, 6 steps for a
	// pair of one-range sets): counted by bGroupWork, which is why this count is
	// quadratic and is not bounded here. The insertions in the middle of a vector
	// that made the earlier version slow (the sum of ranges moved) are none.
	if ins := ops.rangeSteps.n.Load(); ins > 16*n {
		t.Fatalf("%d insertion steps, want at most %d", ins, 16*n)
	}
	t.Logf("pairwise merge steps of the generator's grouping: %d (%d pairs)", ops.mergeSteps.n.Load(), n*(n-1)/2)
}

// A table whose ranges are listed from many ranges: Ranges evaluates the state
// at about twice as many points as it has ranges. An evaluation tests the
// transitions in order and each test is a few binary searches. The earlier
// version read every range of every condition for an evaluation (65,000 ranges
// and three conditions: 8.6 x 10^9 steps).
func TestRangesOfStateAreLinearInPoints(t *testing.T) {
	// Two tokens that split each other's class: the combined state holds
	// transitions of 10,000 ranges that no single-token set matches.
	var a, b strings.Builder
	a.WriteString("[")
	b.WriteString("[")
	for i := 0; i < 10000; i++ {
		a.WriteRune(rune(0x10000 + 4*i))
		a.WriteRune(rune(0x10000 + 4*i + 2))
		b.WriteRune(rune(0x10000 + 4*i))
	}
	a.WriteString("]x")
	b.WriteString("]y")
	p, err := Prepare(choiceGrammar(a.String(), b.String()))
	if err != nil {
		t.Fatal(err)
	}
	var s TokenSet
	s.Insert(terminalSym(0))
	s.Insert(terminalSym(1))
	var evals, probes int64
	counted(t, "BuildUnmerged and Record", &ops.transEvals, func() {
		tb, _, err := p.BuildUnmerged([]TokenSet{s})
		if err != nil {
			t.Fatal(err)
		}
		resetOps()
		for i := 0; i < tb.NumStates(); i++ {
			_ = tb.Ranges(i)
		}
		evals, probes = ops.transEvals.n.Load(), ops.condProbes.n.Load()
	})
	t.Logf("transition evaluations %d, binary search probes %d", evals, probes)
	// 10,000 ranges of the second token and 20,000 of the first: about 2 x
	// 30,000 points, each tested against at most a few transitions.
	atMost(t, "transition evaluations", evals, 8*30000*4, 30000*30000)
	// A binary search over at most 30,000 ranges probes at most 16 times, and an
	// evaluation makes at most three of them.
	if probes > 64*evals {
		t.Fatalf("%d probes for %d evaluations: more than 64 for each", probes, evals)
	}
}

// A deep chain of equal states: each round of the minimization splits one
// group, and a round reads every group.
func TestMinimizationRoundsAreCounted(t *testing.T) {
	p, err := Prepare(sizeGrammar("(ab|ac)(ab|ac)(ab|ac)(ab|ac)(ab|ac)(ab|ac)"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.Analyze(nil)
	if err != nil {
		t.Fatal(err)
	}
	withLimit(t, bMinCompares, 1)
	_, err = p.BuildLexTables(a, tokenZero())
	wantSize(t, err, "comparisons while minimizing")
}

// The two budgets added with the rewrites stop the construction they count.
func TestNewWorkBudgets(t *testing.T) {
	t.Run("following characters", func(t *testing.T) {
		p, err := Prepare(choiceGrammar("a+", "b+"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Analyze(nil); err != nil {
			t.Fatalf("default limits: %v", err)
		}
		withLimit(t, bFollowChars, 0)
		_, err = p.Analyze(nil)
		wantSize(t, err, "starting characters of the tokens that follow")
	})
	t.Run("ranges of lex states", func(t *testing.T) {
		p, err := Prepare(sizeGrammar("[a-c]x|[d-f]y"))
		if err != nil {
			t.Fatal(err)
		}
		withLimit(t, bRangesWork, 0)
		_, _, err = p.BuildUnmerged(tokenZero())
		wantSize(t, err, "list the ranges of the lex states")
	})
}
