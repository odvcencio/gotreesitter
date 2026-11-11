package lexbuild

import (
	"reflect"
	"testing"
)

func setOf(rs ...[2]rune) CharSet {
	var c CharSet
	for _, r := range rs {
		c = c.addRange(r[0], r[1])
	}
	return c
}

// Ported from the unit tests of nfa.rs.
func TestAddingRanges(t *testing.T) {
	set := setOf([2]rune{'c', 'm'}, [2]rune{'q', 's'})
	set = set.addRange('d', 'd')
	if !equalCharSet(set, setOf([2]rune{'c', 'm'}, [2]rune{'q', 's'})) {
		t.Fatal("within existing range")
	}
	set = set.addRange('n', 'n')
	if !equalCharSet(set, setOf([2]rune{'c', 'n'}, [2]rune{'q', 's'})) {
		t.Fatal("adjacent to end")
	}
	set = set.addRange('o', 'p')
	if !equalCharSet(set, setOf([2]rune{'c', 's'})) {
		t.Fatal("filling gap")
	}
	set = setOf([2]rune{'c', 'f'}, [2]rune{'i', 'l'}, [2]rune{'n', 'r'}).addRange('d', 'o')
	if !equalCharSet(set, setOf([2]rune{'c', 'r'})) {
		t.Fatal("bridging")
	}
}

func TestRemoveIntersectionRows(t *testing.T) {
	type row struct{ l, r, lo, ro, in CharSet }
	rows := []row{
		{setOf([2]rune{'a', 'f'}), setOf([2]rune{'g', 'm'}), setOf([2]rune{'a', 'f'}), setOf([2]rune{'g', 'm'}), CharSet{}},
		{setOf([2]rune{'a', 'f'}), setOf([2]rune{'c', 'i'}), setOf([2]rune{'a', 'b'}), setOf([2]rune{'g', 'i'}), setOf([2]rune{'c', 'f'})},
		{setOf([2]rune{'a', 'f'}), setOf([2]rune{'d', 'f'}), setOf([2]rune{'a', 'c'}), CharSet{}, setOf([2]rune{'d', 'f'})},
		{setOf([2]rune{'a', 'm'}), setOf([2]rune{'d', 'f'}), setOf([2]rune{'a', 'c'}, [2]rune{'g', 'm'}), CharSet{}, setOf([2]rune{'d', 'f'})},
		{setOf([2]rune{',', '/'}), setOf([2]rune{'/', '/'}), setOf([2]rune{',', '.'}), CharSet{}, setOf([2]rune{'/', '/'})},
		{setOf([2]rune{'a', 'e'}, [2]rune{'h', 'l'}), setOf([2]rune{'c', 'i'}), setOf([2]rune{'a', 'b'}, [2]rune{'j', 'l'}), setOf([2]rune{'f', 'g'}), setOf([2]rune{'c', 'e'}, [2]rune{'h', 'i'})},
	}
	for i, r := range rows {
		l, rr := r.l.clone(), r.r.clone()
		in := l.removeIntersection(&rr)
		if !equalCharSet(in, r.in) || !equalCharSet(l, r.lo) || !equalCharSet(rr, r.ro) {
			t.Errorf("row %d a", i)
		}
		l, rr = r.l.clone(), r.r.clone()
		in = rr.removeIntersection(&l)
		if !equalCharSet(in, r.in) || !equalCharSet(l, r.lo) || !equalCharSet(rr, r.ro) {
			t.Errorf("row %d b", i)
		}
	}
}

func TestGroupTransitionsRows(t *testing.T) {
	mk := func(c CharSet, prec int32, st ...uint32) nfaTransition {
		return nfaTransition{characters: c, precedence: prec, states: st}
	}
	type rt = rawTransition
	a := func(c CharSet, prec int32, s uint32) rt { return rt{chars: &c, prec: prec, state: s} }
	ch := func(c rune) CharSet { return setOf([2]rune{c, c}) }
	cases := []struct {
		in   []rt
		want []nfaTransition
	}{
		{[]rt{a(setOf([2]rune{'a', 'f'}), 0, 1), a(setOf([2]rune{'d', 'i'}), 1, 2)},
			[]nfaTransition{mk(setOf([2]rune{'a', 'c'}), 0, 1), mk(setOf([2]rune{'d', 'f'}), 1, 1, 2), mk(setOf([2]rune{'g', 'i'}), 1, 2)}},
		{[]rt{a(setOf([2]rune{'a', 'z'}), 0, 1), a(ch('d'), 0, 2), a(ch('i'), 0, 3), a(ch('f'), 0, 4)},
			[]nfaTransition{mk(ch('d'), 0, 1, 2), mk(ch('f'), 0, 1, 4), mk(ch('i'), 0, 1, 3),
				mk(setOf([2]rune{'a', 'c'}, [2]rune{'e', 'e'}, [2]rune{'g', 'h'}, [2]rune{'j', 'z'}), 0, 1)}},
		{[]rt{a(ch('a'), 0, 1), a(ch('b'), 0, 2), a(ch('c'), 0, 1), a(ch('d'), 0, 1), a(ch('e'), 0, 2)},
			[]nfaTransition{mk(setOf([2]rune{'b', 'b'}, [2]rune{'e', 'e'}), 0, 2), mk(setOf([2]rune{'a', 'a'}, [2]rune{'c', 'd'}), 0, 1)}},
	}
	for i, c := range cases {
		got := groupTransitions(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("row %d: %d transitions, want %d", i, len(got), len(c.want))
		}
		for k := range got {
			if !equalCharSet(got[k].characters, c.want[k].characters) || got[k].precedence != c.want[k].precedence || !reflect.DeepEqual(got[k].states, c.want[k].states) {
				t.Errorf("row %d transition %d: got %+v want %+v", i, k, got[k], c.want[k])
			}
		}
	}
}

func TestSimplifyIgnoring(t *testing.T) {
	fromChars := func(cs string) CharSet {
		var s CharSet
		for _, c := range cs {
			s = s.addRange(c, c)
		}
		return s
	}
	cases := []struct {
		chars, out string
		want       CharSet
	}{
		{"a", "", setOf([2]rune{'a', 'a'})},
		{"abcez", "", setOf([2]rune{'a', 'c'}, [2]rune{'e', 'e'}, [2]rune{'z', 'z'})},
		{"abcehz", "dfg", setOf([2]rune{'a', 'h'}, [2]rune{'z', 'z'})},
		{"abcghi", "dj", setOf([2]rune{'a', 'c'}, [2]rune{'g', 'i'})},
		{"cdegh", "abcdef", setOf([2]rune{'g', 'h'})},
		{"IN", "AINZ", CharSet{}},
	}
	for i, c := range cases {
		got := fromChars(c.chars).simplifyIgnoring(fromChars(c.out))
		if !equalCharSet(got, c.want) {
			t.Errorf("row %d: got %v want %v", i, got.r, c.want.r)
		}
	}
}

func TestNegateAndIntersect(t *testing.T) {
	n := setOf([2]rune{'a', 'c'}).negate()
	if n.doesIntersect(setOf([2]rune{'b', 'b'})) || !n.doesIntersect(setOf([2]rune{'d', 'd'})) {
		t.Fatal("negate")
	}
	if !n.containsChar(0x10FFFF) || n.containsChar('a') {
		t.Fatal("contains")
	}
}
