package lexbuild

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

type censusPanic string

// censusVerbose, when set by TestLexDebug, receives the first differences.
var censusVerbose func(format string, a ...any)

type panicT struct{}

func (panicT) Helper() {}
func (panicT) Fatalf(f string, a ...any) {
	panic(censusPanic(fmt.Sprintf(f, a...)))
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		s = s[:n]
	}
	return strings.ReplaceAll(s, " ", "_")
}

// censusLine runs the L1 comparison for one grammar folder and returns the
// LEXCENSUS line. It never fails: every problem becomes the result field.
func censusLine(root, name string) string {
	line, _ := censusLineCat(root, name)
	return line
}

// Categories that censusLineCat returns besides "" (an ordinary result).
const (
	catSizeLimit       = "size limit"           // the port refused the grammar for a size budget
	catReferenceUnread = "reference unreadable" // the reference parser.c could not be read
)

// censusLineCat is censusLine with a category: catSizeLimit when the port
// refused the grammar for a size budget at any stage, catReferenceUnread when
// the reference could not be read, "" otherwise.
func censusLineCat(root, name string) (line, cat string) {
	sizeRefusal := func(err error) bool {
		var se *SizeError
		if errors.As(err, &se) {
			cat = catSizeLimit
			return true
		}
		return false
	}
	res := func(states, starts, reuse [2]int, kw, result string) string {
		return fmt.Sprintf("LEXCENSUS %s states %d/%d lexstarts %d/%d reusable %d/%d keyword-lexer %s result %s",
			name, states[0], states[1], starts[0], starts[1], reuse[0], reuse[1], kw, result)
	}
	z := [2]int{}
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprint(r)
			if strings.Contains(msg, "lexexact.Parse") {
				cat = catReferenceUnread
				line = res(z, z, z, "none", "differs:reference-unreadable:"+oneLine(msg, 120))
			} else {
				line = res(z, z, z, "none", "differs:panic:"+oneLine(msg, 120))
			}
		}
	}()
	dir := filepath.Join(root, name)
	if _, err := os.Stat(filepath.Join(dir, "parser.c")); err != nil {
		msg, _ := os.ReadFile(filepath.Join(dir, "gen.log"))
		return res(z, z, z, "none", "generator-failed:"+oneLine(string(msg), 160)), cat
	}
	ref := loadRef(panicT{}, root, name)
	p, err := Prepare(ref.jsonBytes)
	if err != nil {
		var np *NotPortedError
		if errors.As(err, &np) {
			return res(z, z, z, "none", "refused:"+oneLine(np.Construct, 90)), cat
		}
		sizeRefusal(err)
		return res(z, z, z, "none", "refused:error:"+oneLine(err.Error(), 120)), cat
	}
	toSym, _, err := ref.symbolMap(p)
	if err != nil {
		return res(z, z, z, "none", "differs:symbolmap:"+oneLine(err.Error(), 100)), cat
	}
	states := ref.parseStates(toSym)
	final := make([]TokenSet, len(states))
	pre := make([]TokenSet, len(states))
	for i := range states {
		final[i], pre[i] = states[i].Tokens, states[i].Tokens
	}
	var pre0 TokenSet
	for _, c := range ref.cells[0] {
		if c.sym < ref.tokenCount && !c.recover {
			pre0.Insert(toSym[c.sym])
		}
	}
	pre[0] = pre0
	coinc := pre
	noopt := false
	if cs, ok := coincidentStates(panicT{}, root, name, ref, toSym); ok {
		coinc, noopt = cs, true
	}
	_ = noopt
	a, err := p.Analyze(coinc)
	if err != nil {
		sizeRefusal(err)
		return res(z, z, z, "none", "differs:analyze:"+oneLine(err.Error(), 100)), cat
	}
	errTokens, recoverSet := a.ErrorStateTokens(pre0)
	if censusVerbose != nil {
		inRef := map[Symbol]bool{}
		for _, sy := range final[0].symbols() {
			inRef[sy] = true
		}
		inPort := map[Symbol]bool{}
		for _, sy := range errTokens.symbols() {
			inPort[sy] = true
			if !inRef[sy] {
				censusVerbose("error state: port has %s, reference does not", tokenName(p, sy))
			}
		}
		for _, sy := range final[0].symbols() {
			if !inPort[sy] {
				censusVerbose("error state: reference has %s, port does not", tokenName(p, sy))
			}
		}
	}
	ids := p.TokenSymbolIDs(final)
	lt, err := p.BuildLexTables(a, ref.withReserved(final, toSym))
	if err != nil {
		sizeRefusal(err)
		return res(z, z, z, "none", "differs:build:"+oneLine(err.Error(), 100)), cat
	}
	// The advance-map plans equal the plans made with the earlier clone-and-add
	// loop for the ruled-out set, state by state, in both tables.
	if large, lerr := p.largeSets(); lerr == nil {
		for _, tb := range []*Table{lt.Main, lt.Keyword} {
			if tb == nil {
				continue
			}
			for i := range tb.t.states {
				var w int64
				if want := planStateWith(&tb.t.states[i], large, &w, ruledOutOfOld); !reflect.DeepEqual(tb.plans[i], want) {
					return res(z, z, z, "none", "differs:plan-differs-from-clone-loop"), cat
				}
			}
		}
	}
	symID := func(s Symbol) (uint32, bool) { id, ok := ids[s]; return id, ok }
	rec, err := lt.Main.Record(symID, lt.LexState)
	if err != nil {
		return res(z, z, z, "none", "differs:record:"+oneLine(err.Error(), 100)), cat
	}
	want := ref.tables.Main
	total := len(want.Accept)
	pts := literalPoints(ref.parserC)
	if censusVerbose != nil {
		censusVerbose("lex states: port %d, C %d", len(rec.Accept), total)
	}
	if censusVerbose != nil && lt != nil {
		censusMergeDebug(p, a, ref, lt, final, pre)
	}
	if censusVerbose != nil && len(rec.Accept) != total {
		censusPairing(p, ref, lt, ids, pts, censusVerbose)
	}
	equal := 0
	if len(rec.Accept) == total {
		lx := ref.src.Main
		for i := range rec.Accept {
			ok := rec.Accept[i] == want.Accept[i] && rec.OnEOF[i] == want.OnEOF[i] && rec.OnInvalid[i] == want.OnInvalid[i]
			if !ok && censusVerbose != nil {
				censusVerbose("state %d: accept %d/%d eof %d/%d invalid %d/%d", i, rec.Accept[i], want.Accept[i], rec.OnEOF[i], want.OnEOF[i], rec.OnInvalid[i], want.OnInvalid[i])
			}
			if ok {
				probe := map[int64]bool{}
				for q := range pts {
					probe[q] = true
				}
				for _, r := range rec.Trans[i] {
					for _, v := range []int64{int64(r.Lo), int64(r.Hi)} {
						for d := int64(-1); d <= 1; d++ {
							if q := v + d; q >= 0 && q <= 0x10FFFF {
								probe[q] = true
							}
						}
					}
				}
				for q := range probe {
					tg, sk, mok := lt.Main.Step(i, false, int32(q))
					st, rok := lx.Step(i, false, int32(q))
					if mok != rok || (mok && (tg != st.Target || sk != st.Skip)) {
						ok = false
						if censusVerbose != nil {
							censusVerbose("state %d: U+%04X port (%d %v %v) C (%d %v %v)", i, q, tg, sk, mok, st.Target, st.Skip, rok)
						}
						break
					}
				}
			}
			if ok {
				equal++
			}
		}
	}
	sN, sE := 0, 0
	for i, m := range ref.lexModes {
		if m < 0 {
			continue
		}
		sN++
		if int(lt.LexState[i]) == m {
			sE++
		}
	}
	rN, rE := 0, 0
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
			rN++
			if got == c.reusable {
				rE++
			}
		}
	}
	kw := "none"
	kwEqual := true
	hasRefKW := ref.src.Keyword != nil
	if hasRefKW || p.Syntax.HasWord {
		kw = "differs"
		if hasRefKW && lt.Keyword != nil {
			krec, err := lt.Keyword.Record(symID, nil)
			kwant := ref.tables.Keyword
			if err == nil && len(krec.Accept) == len(kwant.Accept) {
				kw = "equal"
				for i := range krec.Accept {
					if krec.Accept[i] != kwant.Accept[i] || krec.OnEOF[i] != kwant.OnEOF[i] || krec.OnInvalid[i] != kwant.OnInvalid[i] {
						kw = "differs"
					}
				}
			}
		}
		kwEqual = kw == "equal"
	}
	// Complete transitions of both lexers, the error-state tokens computed by the
	// port, and the reserved word sets computed by the port.
	extra := true
	if err := ref.src.Main.CheckTransitions(blobOf(rec)); err != nil {
		extra = false
		if censusVerbose != nil {
			censusVerbose("main CheckTransitions: %v", err)
		}
	}
	if ref.src.Keyword != nil && lt.Keyword != nil {
		if krec, err := lt.Keyword.Record(symID, nil); err == nil {
			if err := ref.src.Keyword.CheckTransitions(blobOf(krec)); err != nil {
				extra = false
				kw = "differs"
				kwEqual = false
				if censusVerbose != nil {
					censusVerbose("keyword CheckTransitions: %v", err)
				}
			}
		}
	}
	if fmt.Sprint(errTokens.symbols()) != fmt.Sprint(final[0].symbols()) {
		extra = false
		if censusVerbose != nil {
			censusVerbose("the error state tokens of the port differ from the reference")
		}
	}
	for id, row := range ref.reserved {
		var want TokenSet
		for _, c := range row {
			if sy, ok := toSym[c]; ok {
				want.Insert(sy)
			}
		}
		if len(row) == 0 {
			continue
		}
		// minimize_parse_table.rs:445-448 removes tokens from a state's reserved set
		// when states merge, so a row of the reference is a subset of one of the
		// grammar's sets.
		found := false
		for i := range p.Syntax.ReservedWordSets {
			sub := true
			for _, sy := range want.symbols() {
				if !p.Syntax.ReservedWordSets[i].Contains(sy) {
					sub = false
				}
			}
			if sub {
				found = true
			}
		}
		if !found {
			extra = false
			if censusVerbose != nil {
				best, bestSets := -1, 1<<30
				for i := range p.Syntax.ReservedWordSets {
					d := 0
					in := map[Symbol]bool{}
					for _, sy := range p.Syntax.ReservedWordSets[i].symbols() {
						in[sy] = true
						if !want.Contains(sy) {
							d++
						}
					}
					for _, sy := range want.symbols() {
						if !in[sy] {
							d++
						}
					}
					if d < bestSets {
						best, bestSets = i, d
					}
				}
				var names []string
				for _, sy := range want.symbols() {
					names = append(names, tokenName(p, sy))
				}
				censusVerbose("reserved word row %d (%d tokens in C, %d ids) is none of the port's sets; nearest set %d differs in %d tokens; row %v", id, len(want.symbols()), len(row), best, bestSets, names)
			}
		}
	}
	result := "equal"
	if equal != total || sE != sN || rE != rN || !kwEqual || !extra {
		result = "differs"
	}
	return res([2]int{equal, total}, [2]int{sE, sN}, [2]int{rE, rN}, kw, result), cat
}

// TestLexCensus is report mode: one LEXCENSUS line per grammar folder in the
// reference folder. It never fails because of a grammar.
func TestLexCensus(t *testing.T) {
	root := refDir(t)
	if _, err := os.Stat(filepath.Join(root, "CENSUS")); err != nil {
		t.Skip("census runs only on a folder that holds a file named CENSUS")
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	lines := make([]string, len(names))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, n := range names {
		wg.Add(1)
		go func(i int, n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// No per-grammar time limit: a hang ends with the timeout of the test binary.
			lines[i] = censusLine(root, n)
		}(i, n)
	}
	wg.Wait()
	for _, l := range lines {
		fmt.Println(l)
	}
	for _, b := range allBudgets {
		fmt.Printf("LEXBUDGET %s | largest %d | limit %d\n", b.what, b.peak.Load(), b.limit)
		// The limits are more than ten times the largest value over the grammars measured here.
		if b.peak.Load()*10 > int64(b.limit) {
			t.Errorf("budget %q: largest value %d is not ten times below the limit %d", b.what, b.peak.Load(), b.limit)
		}
	}
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(root, n, "parser-noopt.c")); err != nil {
			fmt.Printf("LEXCENSUS-NOTE %s coincident-index merged-table (the generator refuses to build the table without merging parse states)\n", n)
		}
	}
}

var lexName = flag.String("lexname", "", "grammar folder for TestLexDebug")

// TestLexDebug prints the first differing states of one grammar of a census folder.
func TestLexDebug(t *testing.T) {
	root := refDir(t)
	if *lexName == "" {
		t.Skip("no -lexname")
	}
	n := 0
	censusVerbose = func(f string, a ...any) {
		if n < 40 {
			t.Logf("DEBUG "+f, a...)
		}
		n++
	}
	defer func() { censusVerbose = nil }()
	t.Log(censusLine(root, *lexName))
}

// censusPairing walks the port's table and the reference in step from every
// lex start and reports built states that pair with two reference states.
func censusPairing(p *Prepared, ref *refGrammar, lt *LexTables, ids map[Symbol]uint32, pts map[int64]bool, say func(string, ...any)) {
	var probes []int64
	for q := range pts {
		probes = append(probes, q)
	}
	sort.Slice(probes, func(a, b int) bool { return probes[a] < probes[b] })
	lx := ref.src.Main
	m2r := map[int]int{}
	var queue [][2]int
	conflicts, accDiffs, stepDiffs := 0, 0, 0
	link := func(m, r int, from string) {
		if x, ok := m2r[m]; ok {
			if x != r && conflicts < 6 {
				conflicts++
				say("built state %d pairs with C states %d and %d (via %s)", m, x, r, from)
			}
			return
		}
		m2r[m] = r
		queue = append(queue, [2]int{m, r})
	}
	for i, ls := range ref.lexModes {
		if ls >= 0 {
			link(int(lt.LexState[i]), ls, "start")
		}
	}
	seenRef := map[int]bool{}
	for len(queue) > 0 {
		pr := queue[0]
		queue = queue[1:]
		m, r := pr[0], pr[1]
		seenRef[r] = true
		if ma, mok := lt.Main.Accept(m); true {
			ra, rok := lx.Accept(r)
			mid := -1
			if mok {
				mid = int(ids[ma])
				if ma.IsEOF() {
					mid = 0
				}
			}
			if mok != rok || (mok && mid != ra) {
				if accDiffs < 6 {
					accDiffs++
					say("pair (built %d, C %d): accept built %v %q (id %d), C %d (ok %v)", m, r, ma, tokenName(p, ma), mid, ra, rok)
				}
			}
		}
		for _, q := range probes {
			tg, sk, mok := lt.Main.Step(m, false, int32(q))
			st, rok := lx.Step(r, false, int32(q))
			if mok != rok || (mok && sk != st.Skip) {
				if stepDiffs < 8 {
					stepDiffs++
					say("pair (built %d, C %d) U+%04X: built ok=%v skip=%v, C ok=%v skip=%v", m, r, q, mok, sk, rok, st.Skip)
				}
				_ = tg
				break
			}
		}
		for _, e := range []struct {
			eof  bool
			look int32
		}{{true, 0}, {false, -1}} {
			tg, _, mok := lt.Main.Step(m, e.eof, e.look)
			st, rok := lx.Step(r, e.eof, e.look)
			if mok && rok {
				link(tg, st.Target, "eof/invalid")
			}
		}
		for _, q := range probes {
			tg, _, mok := lt.Main.Step(m, false, int32(q))
			st, rok := lx.Step(r, false, int32(q))
			if mok && rok {
				link(tg, st.Target, fmt.Sprintf("U+%04X from (%d,%d)", q, m, r))
			}
		}
	}
	for r := 0; r < lx.NumStates(); r++ {
		if !seenRef[r] {
			say("C state %d is not reached from any lex start", r)
		}
	}
}

// censusMergeDebug finds two parse states that the port puts in one lex state
// and the reference puts in two, and prints the token pairs whose conflict
// flags decide the merge.
func censusMergeDebug(p *Prepared, a *Analysis, ref *refGrammar, lt *LexTables, final, pre []TokenSet) {
	shown := 0
	for i := range final {
		for j := i + 1; j < len(final); j++ {
			if shown >= 2 || ref.lexModes[i] < 0 || ref.lexModes[j] < 0 {
				continue
			}
			if lt.LexState[i] != lt.LexState[j] || ref.lexModes[i] == ref.lexModes[j] {
				continue
			}
			shown++
			censusVerbose("MERGE parse states %d and %d share port lex state %d; C has %d and %d", i, j, lt.LexState[i], ref.lexModes[i], ref.lexModes[j])
			for _, pr := range [][2]int{{i, j}, {j, i}} {
				for _, t := range final[pr[0]].terminalIndices() {
					if final[pr[1]].containsTerminal(t) {
						continue
					}
					for _, u := range final[pr[1]].terminalIndices() {
						st := a.conflicts.at(t, u)
						ts := a.conflicts.at(u, t)
						if (st|ts)&^matchesDifferentString != 0 {
							censusVerbose("  %q (only in state %d) vs %q: status %06b/%06b coincident %v", p.Lex.Variables[t].Ident, pr[0], p.Lex.Variables[u].Ident, st, ts, a.coinc.contains[t][u])
						}
					}
				}
			}
		}
	}
}

func blobOf(rec lexexact.Func) [][]lexexact.Transition {
	blob := make([][]lexexact.Transition, len(rec.Trans))
	for i, rs := range rec.Trans {
		for _, r := range rs {
			blob[i] = append(blob[i], lexexact.Transition{Lo: rune(r.Lo), Hi: rune(r.Hi), Next: int(r.Target), Skip: r.Skip})
		}
	}
	return blob
}
