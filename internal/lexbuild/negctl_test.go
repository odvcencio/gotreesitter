package lexbuild

import (
	"math/bits"
	"strings"
	"testing"
)

// Negative controls (sixth review, finding 2). Every work test in this package
// checks a counter against lo <= n <= hi (workRange). A check that accepts
// zero would also accept an implementation that records nothing, so each
// control runs the earlier implementation, which counts its work in the same
// counter (the "Old" functions of quadratic_test.go, the groupHeadOf hook, the
// ruledOutOf hook), on a smaller input, and requires that the same check
// FAILS. The current implementation must pass the same check at that size. The
// failure messages are logged and kept in the test output.

type control struct {
	name   string
	lo, hi int64
	run    func(old bool) int64 // returns the count of the implementation on the input
}

func TestWorkChecksRejectEarlierImplementations(t *testing.T) {
	var controls []control

	controls = append(controls, control{"groups (hook groupHeadOf)", 4 * 2000, 3 * 4 * 2000, func(old bool) int64 {
		var members []string
		for _, c := range "abcd" {
			members = append(members, `{"type":"PATTERN","value":"`+strings.Repeat("()", 2000)+string(c)+`"}`)
		}
		g := []byte(`{"name":"t","rules":{"source":{"type":"CHOICE","members":[` + strings.Join(members, ",") + `]}}}`)
		saved := groupHeadOf
		defer func() { groupHeadOf = saved }()
		if old {
			groupHeadOf = groupHeadOld
		}
		return counted(t, "groups", &ops.groupRunes, func() {
			if _, err := Prepare(g); err != nil {
				t.Fatal(err)
			}
		})
	}})

	const nf = 20000
	controls = append(controls, control{"flags (filterPatternFlags)", nf, 4 * nf, func(old bool) int64 {
		flags := strings.Repeat("i", nf)
		return counted(t, "flags", &ops.flagBytes, func() {
			if old {
				_ = filterPatternFlagsOld(flags)
			} else {
				_ = filterPatternFlags(flags)
			}
		})
	}})

	const nl = 20000
	controls = append(controls, control{"literal join (hirConcatOf)", nl, 4 * nl, func(old bool) int64 {
		items := make([]*hir, nl)
		for i := range items {
			items[i] = &hir{kind: hirLiteral, lit: []rune{'a'}}
		}
		return counted(t, "literal join", &ops.concatRunes, func() {
			if old {
				_ = hirConcatOfOld(items)
			} else {
				_ = hirConcatOf(items)
			}
		})
	}})

	const nc = 4000
	controls = append(controls, control{"class of single code points (charSetFromClass)", nc, 8 * nc, func(old bool) int64 {
		var cls []classRng
		for i := 0; i < nc; i++ {
			cls = append(cls, classRng{uint32(0x10000 + 2*i), uint32(0x10000 + 2*i)})
		}
		return counted(t, "class", &ops.rangeSteps, func() {
			if old {
				addRangeLoopOld(cls)
			} else {
				charSetFromClass(cls)
			}
		})
	}})

	const nd = 3000
	controls = append(controls, control{"class difference", nd, 8 * nd, func(old bool) int64 {
		var a, b []classRng
		for i := 0; i < nd; i++ {
			a = append(a, classRng{uint32(0x10000 + 3*i), uint32(0x10000 + 3*i)})
			b = append(b, classRng{uint32(0x10000 + 3*i + 1), uint32(0x10000 + 3*i + 1)})
		}
		return counted(t, "difference", &ops.classSteps, func() {
			if old {
				classDifferenceOld(a, b)
			} else {
				classDifference(a, b)
			}
		})
	}})

	const nr = 3000
	controls = append(controls, control{"remove intersection", nr, 8 * nr, func(old bool) int64 {
		// One range against n separate ranges: the earlier version removed from the front of the vector for each.
		mk := func() (CharSet, CharSet) {
			c := CharSet{r: []urange{{0, 2 * nr}}}
			var o CharSet
			for i := 0; i < nr; i++ {
				o.r = append(o.r, urange{uint32(2 * i), uint32(2*i + 1)})
			}
			return c, o
		}
		c, o := mk()
		return counted(t, "remove intersection", &ops.mergeSteps, func() {
			if old {
				c.removeIntersectionOld(&o)
			} else {
				c.removeIntersectionSteps(&o)
			}
		})
	}})

	const nu = 3000
	controls = append(controls, control{"union of starting characters (unionOf)", nu, 16 * nu, func(old bool) int64 {
		var sets []*CharSet
		for i := nu - 1; i >= 0; i-- {
			sets = append(sets, &CharSet{r: []urange{{uint32(0x10000 + 3*i), uint32(0x10000 + 3*i + 1)}}})
		}
		return counted(t, "union", &ops.unionRanges, func() {
			if old {
				// the generator's add, one set at a time; addIntRange counts into rangeSteps, so read that
				var acc CharSet
				for _, s := range sets {
					acc = acc.addSlow(*s)
				}
				ops.unionRanges.add(int(ops.rangeSteps.n.Load()))
			} else {
				unionOf(sets)
			}
		})
	}})

	const ne = 2000
	controls = append(controls, control{"range conditions (rangeConditionAt)", ne, ne * int64(bits.Len(uint(5000))+1), func(old bool) int64 {
		var cs CharSet
		var rs []classRng
		for i := 0; i < 5000; i++ {
			cs.r = append(cs.r, urange{uint32(0x10000 + 3*i), uint32(0x10000 + 3*i + 1)})
			rs = append(rs, classRng{uint32(0x10000 + 3*i), uint32(0x10000 + 3*i)})
		}
		return counted(t, "conditions", &ops.condProbes, func() {
			for k := 0; k < ne; k++ {
				if old {
					rangeConditionAtOld(cs, true, false, int64(0x10000+3*k))
				} else {
					rangeConditionAt(rs, true, false, int64(0x10000+3*k))
				}
			}
		})
	}})

	const na = 2000
	controls = append(controls, control{"advance map ruled-out sets (hook ruledOutOf)", 5 * na, 4 * 5 * na, func(old bool) int64 {
		saved := ruledOutOf
		defer func() { ruledOutOf = saved }()
		if old {
			ruledOutOf = ruledOutOfOld
		}
		return runAdvanceMap(t, na)
	}})

	for _, c := range controls {
		t.Run(c.name, func(t *testing.T) {
			if got := c.run(false); workRange(c.name, got, c.lo, c.hi) != nil {
				t.Fatalf("the current implementation fails its own check: %v", workRange(c.name, got, c.lo, c.hi))
			}
			got := c.run(true)
			err := workRange(c.name, got, c.lo, c.hi)
			if err == nil {
				t.Fatalf("the earlier implementation counted %d and passed the check %d <= n <= %d", got, c.lo, c.hi)
			}
			t.Logf("NEGATIVE CONTROL failed as required: %v", err)
		})
	}
}

// A counter whose test expects zero must still be able to read non-zero: the
// refused-pattern case of TestCountedRepetitionsBuildNoMessage requires the
// exact count len(pattern), and here the eager message of the earlier code is
// counted the same way (one quote of the pattern for each repetition).
func TestMessageCounterCountsEagerQuotes(t *testing.T) {
	pat := strings.Repeat("a{1}", 100)
	resetOps()
	p := &patParser{src: []rune(pat), orig: pat}
	for i := 0; i < 100; i++ {
		_ = p.errf("unclosed counted repetition") // what the earlier parseCounted did for each {n}
	}
	if got := ops.errBytes.n.Load(); got != int64(100*len(pat)) {
		t.Fatalf("eager messages counted %d, want %d", got, 100*len(pat))
	}
}
