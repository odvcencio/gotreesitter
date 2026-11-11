package lexbuild

import (
	"fmt"

	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var refGrammars = []string{"json", "regex", "ini"}

// literalPoints reads the integer and character literals of the lex
// functions of a parser.c, each with its neighbours: the code points where a
// C lex state can change its answer.
func literalPoints(parserC string) map[int64]bool {
	pts := map[int64]bool{0: true, 0x10FFFF: true, 0xD7FF: true, 0xD800: true, 0xDFFF: true, 0xE000: true}
	start := strings.Index(parserC, "static bool ts_lex(")
	end := strings.Index(parserC, "static const TSLexerMode ts_lex_modes")
	if start < 0 || end < start {
		return pts
	}
	body := parserC[start:end]
	add := func(v int64) {
		for d := int64(-1); d <= 1; d++ {
			if p := v + d; p >= 0 && p <= 0x10FFFF {
				pts[p] = true
			}
		}
	}
	for _, m := range regexp.MustCompile(`0x[0-9a-fA-F]+|\b\d+\b`).FindAllString(body, -1) {
		if v, err := strconv.ParseInt(m, 0, 64); err == nil {
			add(v)
		}
	}
	for _, m := range regexp.MustCompile(`'(\\.|[^\\'])'`).FindAllStringSubmatch(body, -1) {
		s := m[1]
		if len(s) == 2 {
			switch s[1] {
			case 'n':
				add('\n')
			case 'r':
				add('\r')
			case 't':
				add('\t')
			default:
				add(int64(s[1]))
			}
		} else {
			r := []rune(s)
			add(int64(r[0]))
		}
	}
	return pts
}

// The built automaton is not minimized, so several built states can pair with
// one C state; each built state must pair with exactly one C state.
//
// TestLexbuildBehaviourEqualsC: from the lex start of every parse state, the
// automaton built here and the lexer of the reference parser.c give the same
// accept, end-of-input step, invalid-input step and steps on every interval
// end. The reference supplies the valid tokens of each parse state and the
// grouping of parse states into lex states (the merging itself is checked by
// TestLexbuildRecordEqualsC). The comparison walks the two automata in step
// from each pair of start states; a state pair is counted once.
func TestLexbuildBehaviourEqualsC(t *testing.T) {
	root := refDir(t)
	for _, name := range refGrammars {
		t.Run(name, func(t *testing.T) {
			ref := loadRef(t, root, name)
			p, err := Prepare(ref.jsonBytes)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			toSym, toID, err := ref.symbolMap(p)
			if err != nil {
				t.Fatal(err)
			}
			states := ref.parseStates(toSym)
			// group parse states by the reference's lex state
			var order []int
			sets := map[int]*TokenSet{}
			for i, s := range states {
				ls := ref.lexModes[i]
				if ls < 0 {
					continue
				}
				if sets[ls] == nil {
					sets[ls] = &TokenSet{}
					order = append(order, ls)
				}
				sets[ls].insertAll(&s.Tokens)
			}
			var tokenSets []TokenSet
			for _, ls := range order {
				tokenSets = append(tokenSets, *sets[ls])
			}
			table, starts := p.BuildUnmerged(tokenSets)
			pts := literalPoints(ref.parserC)
			for i := 0; i < table.NumStates(); i++ {
				for _, r := range table.Ranges(i) {
					for _, v := range []int64{int64(r.Lo), int64(r.Hi)} {
						for d := int64(-1); d <= 1; d++ {
							if q := v + d; q >= 0 && q <= 0x10FFFF {
								pts[q] = true
							}
						}
					}
				}
			}
			var probes []int64
			for q := range pts {
				probes = append(probes, q)
			}
			sort.Slice(probes, func(a, b int) bool { return probes[a] < probes[b] })

			mine2ref := map[int]int{}
			var queue [][2]int
			link := func(m, r int) error {
				if x, ok := mine2ref[m]; ok && x != r {
					return fmt.Errorf("built state %d pairs with C states %d and %d", m, x, r)
				}
				if _, ok := mine2ref[m]; !ok {
					mine2ref[m] = r
					queue = append(queue, [2]int{m, r})
				}
				return nil
			}
			for i, ls := range order {
				if err := link(int(starts[i]), ls); err != nil {
					t.Fatalf("start of lex state %d: %v", ls, err)
				}
			}
			lx := ref.src.Main
			pairs, badPairs, bad := 0, 0, 0
			report := func(m, r int, format string, a ...any) {
				bad++
				if bad <= 10 {
					t.Errorf("pair (built %d, C %d): %s", m, r, fmt.Sprintf(format, a...))
				}
			}
			for len(queue) > 0 {
				pr := queue[0]
				queue = queue[1:]
				m, r := pr[0], pr[1]
				pairs++
				ok0 := true
				// accept
				macc, mok := table.Accept(m)
				racc, rok := lx.Accept(r)
				mid := -1
				if mok {
					if macc.IsEOF() {
						mid = 0
					} else if id, found := toID[macc]; found {
						mid = id
					} else {
						mid = -2
					}
				}
				if mok != rok || (mok && mid != racc) {
					report(m, r, "accept: built %v (id %d), C %v (ok=%v)", macc, mid, racc, rok)
					ok0 = false
				}
				cmp := func(what string, eof bool, look int32) {
					tg, sk, mok := table.Step(m, eof, look)
					st, rok := lx.Step(r, eof, look)
					if mok != rok {
						report(m, r, "%s: built ok=%v, C ok=%v", what, mok, rok)
						ok0 = false
						return
					}
					if !mok {
						return
					}
					if sk != st.Skip {
						report(m, r, "%s: built skip=%v, C skip=%v", what, sk, st.Skip)
						ok0 = false
						return
					}
					if err := link(tg, st.Target); err != nil {
						report(m, r, "%s: %v", what, err)
						ok0 = false
					}
				}
				cmp("end of input", true, 0)
				cmp("invalid byte", false, -1)
				for _, q := range probes {
					cmp(fmt.Sprintf("U+%04X", q), false, int32(q))
				}
				if !ok0 {
					badPairs++
				}
			}
			t.Logf("BEHAVIOUR %s: %d of %d state pairs equal; %d built states, %d C states, %d probe points", name, pairs-badPairs, pairs, table.NumStates(), lx.NumStates(), len(probes))
			if bad != 0 {
				t.Fatalf("%d differences", bad)
			}
		})
	}
}
