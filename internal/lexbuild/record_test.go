package lexbuild

import (
	"fmt"
	"sort"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// TestLexbuildRecordEqualsC (the lexer comparison): for each grammar, the lex table
// built here from grammar.json and the valid tokens of the parse states equals
// the lexer of the reference parser.c state for state: state count; per state
// accept, end-of-input step, invalid-input step and the steps on every
// interval end; the lex start of every parse state; the reusable flag of every
// terminal action entry. It also checks the symbol ids and the error-state
// token set that the port computes.
func TestLexbuildRecordEqualsC(t *testing.T) {
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
			final := make([]TokenSet, len(states))
			pre := make([]TokenSet, len(states))
			for i := range states {
				final[i] = states[i].Tokens
				pre[i] = states[i].Tokens
			}
			// State 0 before populate_error_state: its entries that are not recovery entries.
			var pre0 TokenSet
			for _, c := range ref.cells[0] {
				if c.sym < ref.tokenCount && !c.recover {
					pre0.Insert(toSym[c.sym])
				}
			}
			pre[0] = pre0

			// The generator builds the coincident token index before parse states are
			// merged; the reference table written without merging has those states.
			coinc, ok := coincidentStates(t, root, name, ref, toSym)
			if !ok {
				t.Logf("no parser-noopt.c: the coincident index comes from the merged table (limit of this test)")
				coinc = pre
			}
			a, err := p.Analyze(coinc)
			if err != nil {
				t.Fatal(err)
			}
			errTokens, recoverSet := a.ErrorStateTokens(pre0)
			errEqual := fmt.Sprint(errTokens.symbols()) == fmt.Sprint(final[0].symbols())
			t.Logf("error state tokens: port %d, reference %d, equal %v", len(errTokens.symbols()), len(final[0].symbols()), errEqual)
			if !errEqual {
				t.Errorf("error state tokens differ:\n port %v\n  ref %v", errTokens.symbols(), final[0].symbols())
			}

			ids := p.TokenSymbolIDs(final)
			badIDs := 0
			for s, id := range toID {
				if ids[s] != uint32(id) {
					badIDs++
					t.Errorf("symbol id of %v: port %d, C %d", s, ids[s], id)
				}
			}
			symID := func(s Symbol) (uint32, bool) { id, ok := ids[s]; return id, ok }

			lt, err := p.BuildLexTables(a, ref.withReserved(final, toSym))
			if err != nil {
				t.Fatal(err)
			}
			rec, err := lt.Main.Record(symID, lt.LexState)
			if err != nil {
				t.Fatal(err)
			}
			want := ref.tables.Main
			if len(rec.Accept) != len(want.Accept) {
				t.Fatalf("lex states: port %d, C %d", len(rec.Accept), len(want.Accept))
			}
			pts := literalPoints(ref.parserC)
			for i := range rec.Trans {
				for _, r := range rec.Trans[i] {
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
			sort.Slice(probes, func(x, y int) bool { return probes[x] < probes[y] })

			equal := 0
			lx := ref.src.Main
			for i := range rec.Accept {
				ok := true
				note := func(f string, a ...any) {
					if ok {
						t.Errorf("state %d: %s", i, fmt.Sprintf(f, a...))
					}
					ok = false
				}
				if rec.Accept[i] != want.Accept[i] {
					note("accept: port %d, C %d", rec.Accept[i], want.Accept[i])
				}
				if rec.OnEOF[i] != want.OnEOF[i] {
					note("end-of-input step: port %d, C %d", rec.OnEOF[i], want.OnEOF[i])
				}
				if rec.OnInvalid[i] != want.OnInvalid[i] {
					note("invalid-input step: port %d, C %d", rec.OnInvalid[i], want.OnInvalid[i])
				}
				for _, q := range probes {
					tg, sk, mok := lt.Main.Step(i, false, int32(q))
					st, rok := lx.Step(i, false, int32(q))
					if mok != rok || (mok && (tg != st.Target || sk != st.Skip)) {
						note("U+%04X: port (%d skip=%v ok=%v), C (%d skip=%v ok=%v)", q, tg, sk, mok, st.Target, st.Skip, rok)
						break
					}
				}
				if ok {
					equal++
				}
			}
			// The blob check of lexexact on the ranges: every state, the first matching range.
			blob := make([][]lexexact.Transition, len(rec.Trans))
			for i, rs := range rec.Trans {
				for _, r := range rs {
					blob[i] = append(blob[i], lexexact.Transition{Lo: rune(r.Lo), Hi: rune(r.Hi), Next: int(r.Target), Skip: r.Skip})
				}
			}
			if err := lx.CheckTransitions(blob); err != nil {
				t.Errorf("CheckTransitions: %v", err)
			}

			// Lex start of every parse state.
			startEq, startN := 0, 0
			for i, m := range ref.lexModes {
				if m < 0 {
					continue
				}
				startN++
				if int(lt.LexState[i]) == m {
					startEq++
				} else if startN-startEq <= 5 {
					t.Errorf("parse state %d: lex start port %d, C %d", i, lt.LexState[i], m)
				}
			}

			// Reusable flag of every terminal cell.
			reuseEq, reuseN := 0, 0
			for s, cells := range ref.cells {
				fragile := a.Fragile(&final[s])
				for _, c := range cells {
					if c.sym >= ref.tokenCount {
						continue
					}
					sym := toSym[c.sym]
					got := true
					if s == 0 && recoverSet.Contains(sym) {
						got = false
					}
					if sym.IsTerminal() && fragile.Contains(sym) {
						got = false
					}
					reuseN++
					if got == c.reusable {
						reuseEq++
					} else if reuseN-reuseEq <= 5 {
						t.Errorf("parse state %d, token %v: reusable port %v, C %v", s, sym, got, c.reusable)
					}
				}
			}
			// The keyword lexer, when the grammar has a word token.
			if ref.src.Keyword != nil || p.Syntax.HasWord {
				if lt.Keyword == nil || ref.src.Keyword == nil {
					t.Errorf("keyword lexer: port %v, reference %v", lt.Keyword != nil, ref.src.Keyword != nil)
				} else if krec, err := lt.Keyword.Record(symID, nil); err != nil || len(krec.Accept) != len(ref.tables.Keyword.Accept) {
					t.Errorf("keyword lexer: %v, %d states against %d", err, len(krec.Accept), len(ref.tables.Keyword.Accept))
				} else {
					kw := ref.tables.Keyword
					for i := range krec.Accept {
						if krec.Accept[i] != kw.Accept[i] || krec.OnEOF[i] != kw.OnEOF[i] || krec.OnInvalid[i] != kw.OnInvalid[i] {
							t.Errorf("keyword lexer state %d differs", i)
						}
					}
				}
			}
			t.Logf("RECORD %s: %d of %d states equal; lex starts %d of %d; reusable flags %d of %d; symbol ids differing %d", name, equal, len(rec.Accept), startEq, startN, reuseEq, reuseN, badIDs)
		})
	}
}
