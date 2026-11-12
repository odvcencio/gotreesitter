package lexexact

import (
	"fmt"
	"sort"
)

// MaxCodePoint is the largest Unicode code point.
const MaxCodePoint = 0x10FFFF

// Transition is one range of a blob lex state, in blob order: code points
// Lo..Hi lead to state Next. Skip is the SKIP flag.
type Transition struct {
	Lo, Hi rune
	Next   int
	Skip   bool
}

// CheckTransitions is the blob check. For every state it compares
// the first matching blob range with the step of the C text at every point
// where the C text can change its answer: each literal it tests (and its
// neighbours), and the ends of the blob's own ranges (and their neighbours).
// Only valid code points are tested; the blob says nothing about the end of
// input or lookahead -1. It returns the first difference.
func (l *Lexer) CheckTransitions(blob [][]Transition) error {
	if len(blob) != len(l.states) {
		return fmt.Errorf("blob has %d lex states, the C function has %d", len(blob), len(l.states))
	}
	for i := range l.states {
		rs := blob[i]
		if !l.states[i].defined {
			if len(rs) != 0 {
				return fmt.Errorf("state %d: the blob has %d ranges, C has no case", i, len(rs))
			}
			continue
		}
		pts := map[int64]bool{0: true, MaxCodePoint: true}
		add := func(v int64) {
			for d := int64(-1); d <= 1; d++ {
				if p := v + d; p >= 0 && p <= MaxCodePoint {
					pts[p] = true
				}
			}
		}
		for _, v := range l.stateLiterals(i) {
			add(v)
		}
		for _, r := range rs {
			add(int64(r.Lo))
			add(int64(r.Hi))
		}
		list := make([]int64, 0, len(pts))
		for p := range pts {
			list = append(list, p)
		}
		sort.Slice(list, func(a, b int) bool { return list[a] < list[b] })
		for _, c := range list {
			want, wok := l.Step(i, false, int32(c))
			var got Step
			gok := false
			for _, r := range rs {
				if r.Lo <= rune(c) && rune(c) <= r.Hi {
					got, gok = Step{Target: r.Next, Skip: r.Skip}, true
					break
				}
			}
			if gok != wok || got != want {
				return fmt.Errorf("state %d, code point U+%04X: C steps to %v (ok=%v), the blob to %v (ok=%v)", i, c, want, wok, got, gok)
			}
		}
	}
	return nil
}
