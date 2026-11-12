package lexbuild

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func sizeGrammar(pattern string) []byte {
	return []byte(`{"name":"t","rules":{"source":{"type":"PATTERN","value":"` + pattern + `"}}}`)
}

// choiceGrammar has two tokens, source_token1 and source_token2, one for each pattern.
func choiceGrammar(a, b string) []byte {
	return []byte(`{"name":"t","rules":{"source":{"type":"CHOICE","members":[{"type":"PATTERN","value":"` + a +
		`"},{"type":"PATTERN","value":"` + b + `"}]}}}`)
}

// seqChoiceGrammar is a SEQ of k independent CHOICE(STRING a, STRING b): 2^k productions.
func seqChoiceGrammar(k int) []byte {
	var m []string
	for i := 0; i < k; i++ {
		m = append(m, `{"type":"CHOICE","members":[{"type":"STRING","value":"a"},{"type":"STRING","value":"b"}]}`)
	}
	return []byte(`{"name":"t","rules":{"source":{"type":"SEQ","members":[` + strings.Join(m, ",") + `]}}}`)
}

// withLimit lowers (or raises) the limit of one budget for the test.
func withLimit(t *testing.T, b *budget, n int64) {
	t.Helper()
	old := b.limit
	b.limit = n
	t.Cleanup(func() { b.limit = old })
}

func tokenZero() []TokenSet {
	var ts TokenSet
	ts.Insert(terminalSym(0))
	return []TokenSet{ts}
}

func wantSize(t *testing.T, err error, what string) {
	t.Helper()
	var se *SizeError
	if !errors.As(err, &se) || !strings.Contains(se.What, what) {
		t.Fatalf("want a SizeError about %q, got %v", what, err)
	}
}

// measure runs f and returns its time and its peak live heap above the heap before it (sampled every 2 ms;
// the sample can miss a peak shorter than that).
func measure(f func()) (time.Duration, uint64) {
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	var peak uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()
	start := time.Now()
	f()
	d := time.Since(start)
	close(stop)
	<-done
	if peak < base.HeapAlloc {
		return d, 0
	}
	return d, peak - base.HeapAlloc
}

// A small budget stops BuildUnmerged and BuildLexTables with a *SizeError, for each limit of the lex table
// construction, and a budget that is large enough builds.
func TestLexSizeBudget(t *testing.T) {
	const pat = "abcdefghijklmnopqrstuvwxyz"
	p, err := Prepare(sizeGrammar(pat))
	if err != nil {
		t.Fatal(err)
	}
	if tb, _, err := p.BuildUnmerged(tokenZero()); err != nil || tb.NumStates() != 27 {
		t.Fatalf("default limits: states %v, err %v", tb, err)
	}
	for _, tc := range []struct {
		name string
		b    *budget
		n    int64
		what string
	}{
		{"states", bLexStates, 10, "lex states"},
		{"automaton states", bLexIDs, 10, "automaton states summed over lex states"},
		{"transitions", bLexAdvances, 10, "character transitions"},
		{"ranges", bLexRanges, 10, "character ranges summed over lex-state"},
		{"single-token constructions", bLargeStates, 10, "single-token constructions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withLimit(t, tc.b, tc.n)
			p, err := Prepare(sizeGrammar(pat))
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = p.BuildUnmerged(tokenZero())
			wantSize(t, err, tc.what)
			a, err := p.Analyze(nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.BuildLexTables(a, tokenZero())
			wantSize(t, err, tc.what)
		})
	}
	// The limit is checked before the state is inserted: exactly 27 states fit in a budget of 27.
	withLimit(t, bLexStates, 27)
	if _, _, err := p.BuildUnmerged(tokenZero()); err != nil {
		t.Errorf("a budget of 27 states must build 27 states: %v", err)
	}
	withLimit(t, bLexStates, 26)
	if _, _, err := p.BuildUnmerged(tokenZero()); err == nil {
		t.Error("a budget of 26 states must refuse 27 states")
	}
}

// Minimization compares the states of a group in pairs; a small budget stops it.
func TestLexMinimizeBudget(t *testing.T) {
	p, err := Prepare(sizeGrammar("aaaaaaaaaaaaaaaaaaaa"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.Analyze(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.BuildLexTables(a, tokenZero()); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bMinCompares, 5)
	_, err = p.BuildLexTables(a, tokenZero())
	wantSize(t, err, "comparisons while minimizing")
}

// Overlapping classes give many automaton states in each transition of one lex state.
func TestTransitionIDsBudget(t *testing.T) {
	var alts []string
	for c := 'a'; c < 'a'+16; c++ {
		alts = append(alts, fmt.Sprintf("[%c-z]x", c)) // nested ranges: each overlaps all earlier ones
	}
	g := sizeGrammar(strings.Join(alts, "|"))
	p, err := Prepare(g)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.BuildUnmerged(tokenZero()); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bTransIDs, 20)
	_, _, err = p.BuildUnmerged(tokenZero())
	wantSize(t, err, "summed over the transitions of one lex state")
}

// The pattern [ab]*a[ab]{24} has 29 automaton states and 33,554,432 reachable lex states. With the
// default limits the construction must stop quickly and within bounded memory, and say why.
func TestLexSizeBlowupRefused(t *testing.T) {
	p, err := Prepare(sizeGrammar("[ab]*a[ab]{24}"))
	if err != nil {
		t.Fatal(err)
	}
	d, alloc := measure(func() { _, _, err = p.BuildUnmerged(tokenZero()) })
	wantSize(t, err, "lex states")
	t.Logf("BLOWUP lex table refused after %v, %d MiB peak heap, limit %d lex states: %v", d, alloc>>20, bLexStates.limit, err)
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and peak heap %d MiB; want at most 60 s and %d MiB", d, alloc>>20, blowupAllocBudget>>20)
	}
	a, err := p.Analyze(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.BuildLexTables(a, tokenZero()); err == nil {
		t.Fatal("BuildLexTables: want a SizeError")
	}
}

// Token-conflict analysis searches pairs of tokens; two tokens that stay live together build many state sets.
func TestConflictSearchBudget(t *testing.T) {
	small := choiceGrammar("[ab]*a[ab]{6}", "[ab]+")
	p, err := Prepare(small)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Analyze(nil); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bConflictSets, 20)
	_, err = p.Analyze(nil)
	wantSize(t, err, "state sets in one token-pair conflict search")
	withLimit(t, bConflictSets, 1<<17)
	withLimit(t, bConflictIDs, 50)
	_, err = p.Analyze(nil)
	wantSize(t, err, "automaton states retained by one token-pair conflict search")
}

// The review's input: 32 automaton states, 33,554,432 reachable state sets.
func TestConflictSearchBlowupRefused(t *testing.T) {
	p, err := Prepare(choiceGrammar("[ab]*a[ab]{24}", "[ab]+"))
	if err != nil {
		t.Fatal(err)
	}
	d, alloc := measure(func() { _, err = p.Analyze(nil) })
	wantSize(t, err, "token-pair conflict search")
	t.Logf("BLOWUP conflict search refused after %v, %d MiB peak heap: %v", d, alloc>>20, err)
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and peak heap %d MiB; want at most 60 s and %d MiB", d, alloc>>20, blowupAllocBudget>>20)
	}
}

// Flattening a SEQ of k independent two-way CHOICEs gives 2^k productions.
func TestFlattenBudget(t *testing.T) {
	if _, err := Prepare(seqChoiceGrammar(6)); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	for _, tc := range []struct {
		name string
		b    *budget
		n    int64
		what string
	}{
		{"paths", bFlattenPaths, 10, "choice paths"},
		{"steps", bFlattenSteps, 100, "production steps retained"},
		{"one production", bFlattenPath1, 3, "steps in one flattened production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withLimit(t, tc.b, tc.n)
			_, err := Prepare(seqChoiceGrammar(6))
			wantSize(t, err, tc.what)
		})
	}
}

// The review's input: 24 members, 16,777,216 productions with 402,653,184 steps.
func TestFlattenBlowupRefused(t *testing.T) {
	var err error
	d, alloc := measure(func() { _, err = Prepare(seqChoiceGrammar(24)) })
	wantSize(t, err, "while flattening rules")
	t.Logf("BLOWUP flatten refused after %v, %d MiB peak heap: %v", d, alloc>>20, err)
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and peak heap %d MiB; want at most 60 s and %d MiB", d, alloc>>20, blowupAllocBudget>>20)
	}
}

// The automaton of the lexical grammar: states and ranges.
func TestNFABudget(t *testing.T) {
	_, err := Prepare(sizeGrammar("b{0,4294967295}"))
	wantSize(t, err, "repetition")
	withLimit(t, bRepeatCount, 1<<32) // reach the limit of the automaton
	_, err = Prepare(sizeGrammar("b{0,4294967295}"))
	wantSize(t, err, "automaton states")
	g := sizeGrammar(`[a-cx-z]{50}`)
	if _, err := Prepare(g); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bNFARanges, 60)
	_, err = Prepare(g)
	wantSize(t, err, "character ranges summed over automaton states")
}

// Classes and escapes of one pattern: \p{L} brings a table of ranges each time it is read.
func TestPatternRangesBudget(t *testing.T) {
	g := sizeGrammar(`[\\p{L}\\p{L}\\p{L}]x`)
	if _, err := Prepare(g); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bPatternRanges, 100)
	_, err := Prepare(g)
	wantSize(t, err, "character ranges read into the classes of the patterns")
}

// The reachability check of inline rules copies symbol lists.
func TestInlineBudget(t *testing.T) {
	g := []byte(`{"name":"t","rules":{"a":{"type":"SEQ","members":[{"type":"SYMBOL","name":"b"},{"type":"STRING","value":"y"}]},"b":{"type":"CHOICE","members":[{"type":"STRING","value":"x"},{"type":"STRING","value":"z"}]}},"inline":["b"]}`)
	if _, err := Prepare(g); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bInlineCalls, 0)
	_, err := Prepare(g)
	wantSize(t, err, "expansion steps in the check of inline rules")
	withLimit(t, bInlineCalls, 200000)
	withLimit(t, bInlineSymbols, 0)
	_, err = Prepare(g)
	wantSize(t, err, "symbols copied in the check of inline rules")
}

// Matrices that grow with tokens times tokens or rules times tokens.
func TestSymbolCellsBudget(t *testing.T) {
	g := choiceGrammar("a+", "b+")
	if _, err := Prepare(g); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bSymbolCells, 3)
	_, err := Prepare(g)
	wantSize(t, err, "matrix")
}

// A size refusal is a permitted difference from the generator, and the differential test must put it in its own
// bucket from the category that the port returns, not from the text of the error.
func TestDifferentialClassifiesSizeRefusal(t *testing.T) {
	withLimit(t, bLexStates, 5)
	p, err := Prepare(sizeGrammar("abcdefghij"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.BuildUnmerged(tokenZero())
	wantSize(t, err, "lex states")
	// The census formats an error through oneLine (spaces become underscores).
	line := "LEXCENSUS g1 states 0/0 lexstarts 0/0 reusable 0/0 keyword-lexer none result differs:build:" + oneLine(err.Error(), 100)
	if strings.Contains(line, "size limit") {
		t.Fatal("the formatted line was expected to lose the space of \"size limit\"")
	}
	if o := classifyCensus(line, catSizeLimit, "{}"); o.bucket != "port refuses: size limit" {
		t.Errorf("size category: bucket %q", o.bucket)
	}
	if o := classifyCensus(line, "", "{}"); o.bucket != "differ" {
		t.Errorf("no category: bucket %q, want differ", o.bucket)
	}
	if o := classifyCensus("LEXCENSUS g1 states 1/1 result equal", "", "{}"); o.bucket != "equal" {
		t.Errorf("equal line: bucket %q", o.bucket)
	}
	if o := classifyCensus(line, catReferenceUnread, "{}"); o.bucket != "reference unreadable" {
		t.Errorf("reference category: bucket %q", o.bucket)
	}
}

// blowupAllocBudget is the most that a refused construction may allocate in these tests.
const blowupAllocBudget = 1 << 30

// ---- Shape caps and the budgets of the passes behind them ----

// Each cap on the shape of the input is checked first and names itself in its error.
func TestShapeCaps(t *testing.T) {
	const (
		str  = `{"type":"STRING","value":"abc"}`
		base = `{"name":"t","rules":{"source":` + str + `}}`
	)
	deep := `{"name":"t","rules":{"source":{"type":"SEQ","members":[{"type":"SEQ","members":[` + str + `,` + str + `]},` + str + `]}}}`
	for _, tc := range []struct {
		name    string
		b       *budget
		limit   int64
		grammar string
	}{
		{"bytes", bInputBytes, 20, base},
		{"rules", bRules, 1, `{"name":"t","rules":{"source":` + str + `,"other":` + str + `}}`},
		{"externals", bExternals, 0, `{"name":"t","rules":{"source":` + str + `},"externals":[{"type":"SYMBOL","name":"ext"}]}`},
		{"orderings", bPrecOrderings, 0, `{"name":"t","rules":{"source":` + str + `},"precedences":[[{"type":"STRING","value":"p"}]]}`},
		{"names in an ordering", bPrecNames, 1, `{"name":"t","rules":{"source":` + str + `},"precedences":[[{"type":"STRING","value":"p"},{"type":"STRING","value":"q"}]]}`},
		{"name length", bNameLen, 3, base},
		{"string length", bStringLen, 2, base},
		{"pattern length", bPatternLen, 2, `{"name":"t","rules":{"source":{"type":"PATTERN","value":"abc"}}}`},
		{"repeat count", bRepeatCount, 5, `{"name":"t","rules":{"source":{"type":"PATTERN","value":"a{6}"}}}`},
		{"rule depth", bRuleDepth, 2, deep},
		{"list length", bListLen, 1, `{"name":"t","rules":{"source":{"type":"SEQ","members":[` + str + `,` + str + `]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Prepare([]byte(tc.grammar)); err != nil {
				t.Fatalf("default caps: %v", err)
			}
			withLimit(t, tc.b, tc.limit)
			_, err := Prepare([]byte(tc.grammar))
			wantSize(t, err, tc.b.what)
			if _, err := ParseGrammar([]byte(tc.grammar)); tc.b != bRepeatCount && err == nil {
				t.Error("ParseGrammar must apply the cap too")
			}
		})
	}
}

// The key of a production holds ids of strings, not the strings: a long field name does not multiply.
func TestFlattenKeysDoNotCopyNames(t *testing.T) {
	member := `{"type":"CHOICE","members":[{"type":"STRING","value":"a"},{"type":"STRING","value":"b"}]}`
	build := func(k, nameLen int) []byte {
		var m []string
		for i := 0; i < k; i++ {
			m = append(m, member)
		}
		return []byte(`{"name":"t","rules":{"source":{"type":"FIELD","name":"` + strings.Repeat("f", nameLen) +
			`","content":{"type":"SEQ","members":[` + strings.Join(m, ",") + `]}}}}`)
	}
	withLimit(t, bNameLen, 1<<20)
	// The review's input has k = 16 and a name of 4096 bytes: 65,536 productions of 16 steps.
	var err error
	d, alloc := measure(func() { _, err = Prepare(build(14, 4096)) })
	t.Logf("KEYS k=14 name 4096: %v, %d MiB peak heap, err %v", d, alloc>>20, err)
	if err != nil {
		wantSize(t, err, "while flattening rules")
	}
	if alloc > blowupAllocBudget {
		t.Errorf("peak heap %d MiB; want at most %d MiB", alloc>>20, blowupAllocBudget>>20)
	}
	// The review's input (k = 16, 1,048,576 steps) is refused by the key budget with bounded memory.
	d, alloc = measure(func() { _, err = Prepare(build(16, 4096)) })
	t.Logf("KEYS k=16 name 4096: %v, %d MiB peak heap, err %v", d, alloc>>20, err)
	wantSize(t, err, "while flattening rules")
	if alloc > blowupAllocBudget {
		t.Errorf("k=16: peak heap %d MiB; want at most %d MiB", alloc>>20, blowupAllocBudget>>20)
	}
	// With the name cap at its default the same input is refused at the entry.
	withLimit(t, bNameLen, 1<<10)
	_, err = Prepare(build(16, 4096))
	wantSize(t, err, "bytes in a name")
	withLimit(t, bNameLen, 1<<20)
	_, a1 := measure(func() { _, _ = Prepare(build(10, 10)) })
	_, a2 := measure(func() { _, _ = Prepare(build(10, 4096)) })
	if a2 > 3*a1+(8<<20) {
		t.Errorf("a 4096-byte field name allocated %d bytes against %d for a 10-byte name: keys copy the name", a2, a1)
	}
	withLimit(t, bFlattenKeyBytes, 1000)
	_, err = Prepare(build(6, 10))
	wantSize(t, err, "bytes of production keys")
}

// Precedence validation keeps one entry for each distinct pair of an ordering.
func TestPrecedencePairsBudget(t *testing.T) {
	ordering := func(n int) []byte {
		var e []string
		for i := 0; i < n; i++ {
			e = append(e, fmt.Sprintf(`{"type":"STRING","value":"p%05d"}`, i))
		}
		return []byte(`{"name":"t","rules":{"source":{"type":"STRING","value":"x"}},"precedences":[[` + strings.Join(e, ",") + `]]}`)
	}
	if _, err := Prepare(ordering(30)); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bPrecPairs, 100)
	_, err := Prepare(ordering(30))
	wantSize(t, err, "distinct precedence pairs")
	// The review's input: 10,000 names give 49,995,000 pairs. The caps on the shape refuse it first, so
	// they are raised here to reach the budget of the pairs.
	withLimit(t, bPrecNames, 1<<20)
	withLimit(t, bPrecPairs, 1<<11)
	g := ordering(10000)
	d, alloc := measure(func() { _, err = Prepare(g) })
	wantSize(t, err, "distinct precedence pairs")
	t.Logf("PAIRS 10000 names refused after %v, %d MiB peak heap: %v", d, alloc>>20, err)
	if alloc > blowupAllocBudget {
		t.Errorf("peak heap %d MiB; want at most %d MiB", alloc>>20, blowupAllocBudget>>20)
	}
}

// FIRST and LAST: one check at the allocation, for both entry paths.
func TestFirstLastBudget(t *testing.T) {
	sg := &SyntaxGrammar{ExternalTokens: make([]ExternalToken, 131072), Variables: make([]SyntaxVariable, 1)}
	lg := &LexicalGrammar{Variables: make([]LexicalVariable, 1)}
	d, alloc := measure(func() {
		_, err := NewFirstLastSets(sg, lg)
		wantSize(t, err, "matrix")
	})
	if d > 5*time.Second || alloc > 64<<20 {
		t.Errorf("NewFirstLastSets: %v and %d MiB before it refused", d, alloc>>20)
	}
	// Overflow: dimensions whose product does not fit.
	maxInt := int(^uint(0) >> 1)
	if got := cells(maxInt, maxInt); int64(got) <= bSymbolCells.limit {
		t.Errorf("cells(max, max) = %d, want a value above the limit", got)
	}
	if got := addCells(int(bSymbolCells.limit), int(bSymbolCells.limit)); int64(got) <= bSymbolCells.limit {
		t.Errorf("addCells = %d, want a value above the limit", got)
	}
	// The path through Analyze: the grammar passes Prepare, then the limit is lowered.
	p, err := Prepare(choiceGrammar("a+", "b+"))
	if err != nil {
		t.Fatal(err)
	}
	withLimit(t, bSymbolCells, 3)
	_, err = p.Analyze(nil)
	wantSize(t, err, "matrix")
	_, err = NewFirstLastSets(p.Syntax, p.Lex)
	wantSize(t, err, "matrix")
	withLimit(t, bSymbolCells, 1<<24)
	if _, err := NewFirstLastSets(p.Syntax, p.Lex); err != nil {
		t.Errorf("default limit: %v", err)
	}
}

// The closure over split states is linear and counted. Its result equals the generator's scan-and-insert
// version, which is kept here as the reference.
func TestAddStatesMatchesGenerator(t *testing.T) {
	ref := func(c *nfaCursor, newIDs *[]uint32) {
		ids := *newIDs
		i := 0
		for i < len(ids) {
			id := ids[i]
			st := &c.n.states[id]
			if st.kind == nfaSplit {
				hasLeft, hasRight := false, false
				for _, x := range ids {
					if x == st.left {
						hasLeft = true
					}
					if x == st.right {
						hasRight = true
					}
				}
				if !hasLeft {
					ids = append(ids, st.left)
				}
				if !hasRight {
					ids = append(ids, st.right)
				}
			} else {
				j := sort.Search(len(c.stateIDs), func(k int) bool { return c.stateIDs[k] >= id })
				if j == len(c.stateIDs) || c.stateIDs[j] != id {
					c.stateIDs = append(c.stateIDs, 0)
					copy(c.stateIDs[j+1:], c.stateIDs[j:])
					c.stateIDs[j] = id
				}
			}
			i++
		}
		*newIDs = ids
	}
	for seed := int64(0); seed < 300; seed++ {
		r := rand.New(rand.NewSource(seed))
		n := nfa{}
		size := 2 + r.Intn(60)
		for i := 0; i < size; i++ {
			switch r.Intn(3) {
			case 0:
				n.states = append(n.states, nfaState{kind: nfaAccept})
			case 1:
				n.states = append(n.states, nfaState{kind: nfaAdvance, stateID: uint32(r.Intn(size))})
			default:
				n.states = append(n.states, nfaState{kind: nfaSplit, left: uint32(r.Intn(size)), right: uint32(r.Intn(size))})
			}
		}
		var start []uint32
		for k := r.Intn(6); k >= 0; k-- {
			start = append(start, uint32(r.Intn(size)))
		}
		c1, c2 := &nfaCursor{n: &n}, &nfaCursor{n: &n}
		if seed%3 == 0 { // a cursor that already holds states
			pre := []uint32{uint32(r.Intn(size))}
			p1, p2 := append([]uint32(nil), pre...), append([]uint32(nil), pre...)
			ref(c1, &p1)
			c2.addStates(&p2)
		}
		s1, s2 := append([]uint32(nil), start...), append([]uint32(nil), start...)
		ref(c1, &s1)
		c2.addStates(&s2)
		// The list may differ only by a repeated id (both sides of a split equal).
		if !equalU32(c1.stateIDs, c2.stateIDs) {
			t.Fatalf("seed %d: state ids %v want %v", seed, c2.stateIDs, c1.stateIDs)
		}
		_ = s1
		_ = s2
	}
}

// a?{1000000} has 2,000,001 automaton states; closing its first state scanned a growing list once for each split.
func TestClosureWorkBudget(t *testing.T) {
	small := sizeGrammar("a?{1000}")
	p, err := Prepare(small)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.BuildUnmerged(tokenZero()); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	withLimit(t, bClosureWork, 100)
	p, err = Prepare(small)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.BuildUnmerged(tokenZero())
	wantSize(t, err, "closing state sets over splits")
	withLimit(t, bClosureWork, 1<<25)
	// The review's input a?{1000000} is refused by the cap on the count.
	_, err = Prepare(sizeGrammar("a?{1000000}"))
	wantSize(t, err, "repetition")
	// With the cap raised, the largest automaton the limit allows (240,001 states) is closed in bounded time and memory.
	withLimit(t, bRepeatCount, 1<<30)
	big, err := Prepare(sizeGrammar("a?{120000}"))
	if err != nil {
		t.Fatal(err)
	}
	d, alloc := measure(func() { _, _, err = big.BuildUnmerged(tokenZero()) })
	t.Logf("CLOSURE a?{120000}: %v, %d MiB peak heap, err %v", d, alloc>>20, err)
	if err != nil {
		wantSize(t, err, "lex states")
	}
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and peak heap %d MiB", d, alloc>>20)
	}
}

// The work counters of the passes behind the caps: each stops its pass when lowered to zero.
func TestWorkBudgets(t *testing.T) {
	kw := []byte(`{"name":"t","word":"id","rules":{"source":{"type":"SEQ","members":[{"type":"STRING","value":"if"},{"type":"SYMBOL","name":"id"}]},"id":{"type":"PATTERN","value":"[a-z]+"}}}`)
	rep := []byte(`{"name":"t","rules":{"source":{"type":"REPEAT","content":{"type":"STRING","value":"a"}}}}`)
	two := []byte(`{"name":"t","rules":{"source":{"type":"SEQ","members":[{"type":"STRING","value":"a"},{"type":"STRING","value":"b"}]}}}`)
	t.Run("tree comparisons", func(t *testing.T) {
		withLimit(t, bTreeEqWork, 0)
		_, err := Prepare(two)
		wantSize(t, err, "rule nodes compared")
	})
	t.Run("repeat expansion", func(t *testing.T) {
		withLimit(t, bRepeatWork, 0)
		_, err := Prepare(rep)
		wantSize(t, err, "rule nodes evaluated")
	})
	t.Run("coincident tokens", func(t *testing.T) {
		p, err := Prepare(choiceGrammar("a+", "b+"))
		if err != nil {
			t.Fatal(err)
		}
		var s1 TokenSet
		s1.Insert(terminalSym(0))
		s1.Insert(terminalSym(1))
		if _, err := p.Analyze([]TokenSet{s1}); err != nil {
			t.Fatalf("default limits: %v", err)
		}
		withLimit(t, bCoincidentWork, 1)
		_, err = p.Analyze([]TokenSet{s1})
		wantSize(t, err, "tokens that share a parse state")
	})
	t.Run("removal of unreachable rules", func(t *testing.T) {
		g := []byte(`{"name":"t","rules":{"source":{"type":"STRING","value":"a"},"unused":{"type":"STRING","value":"b"}}}`)
		if _, err := Prepare(g); err != nil {
			t.Fatalf("default limits: %v", err)
		}
		withLimit(t, bNormalizeWork, 1)
		_, err := Prepare(g)
		wantSize(t, err, "removal of unreachable rules")
	})
	t.Run("cycle search", func(t *testing.T) {
		withLimit(t, bCycleWork, 0)
		_, err := Prepare(two)
		wantSize(t, err, "indirect-recursion")
	})
	t.Run("keywords", func(t *testing.T) {
		p, err := Prepare(kw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Analyze(nil); err != nil {
			t.Fatalf("default limits: %v", err)
		}
		withLimit(t, bKeywordWork, 0)
		_, err = p.Analyze(nil)
		wantSize(t, err, "keyword identification")
	})
	t.Run("grouping of parse states", func(t *testing.T) {
		p, err := Prepare(choiceGrammar("a+", "b+"))
		if err != nil {
			t.Fatal(err)
		}
		a, err := p.Analyze(nil)
		if err != nil {
			t.Fatal(err)
		}
		var s1, s2 TokenSet
		s1.Insert(terminalSym(0))
		s2.Insert(terminalSym(1))
		withLimit(t, bGroupingWork, 0)
		_, err = p.BuildLexTables(a, []TokenSet{s1, s2})
		wantSize(t, err, "grouping of parse states")
	})
	t.Run("ordering of lex states", func(t *testing.T) {
		p, err := Prepare(sizeGrammar("abc|abd|abe"))
		if err != nil {
			t.Fatal(err)
		}
		a, err := p.Analyze(nil)
		if err != nil {
			t.Fatal(err)
		}
		withLimit(t, bSortWork, 0)
		_, err = p.BuildLexTables(a, tokenZero())
		wantSize(t, err, "ordering of lex states")
	})
	t.Run("rendering", func(t *testing.T) {
		p, err := Prepare(sizeGrammar("[a-c]x|[d-f]y"))
		if err != nil {
			t.Fatal(err)
		}
		withLimit(t, bRenderWork, 0)
		_, _, err = p.BuildUnmerged(tokenZero())
		wantSize(t, err, "rendering lex states")
	})
	t.Run("grouping of transitions", func(t *testing.T) {
		p, err := Prepare(sizeGrammar("ab"))
		if err != nil {
			t.Fatal(err)
		}
		withLimit(t, bGroupWork, 0)
		_, _, err = p.BuildUnmerged(tokenZero())
		wantSize(t, err, "grouping of transitions into character sets")
	})
}

// doc.go lists the caps and budgets; it must list the values that the code uses.
func TestDocListsTheLimits(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range allBudgets {
		want := fmt.Sprintf("//\t%s: %s\n", b.what, commas(b.limit))
		if !strings.Contains(string(doc), want) {
			t.Errorf("doc.go does not list %q", strings.TrimSpace(want))
		}
	}
}

func commas(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
