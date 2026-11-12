package lexbuild

import (
	"errors"
	"fmt"
	"runtime"
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
func withLimit(t *testing.T, b *budget, n int) {
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

// measure runs f and returns its time and the bytes it allocated.
func measure(f func()) (time.Duration, uint64) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	d := time.Since(start)
	runtime.ReadMemStats(&after)
	return d, after.TotalAlloc - before.TotalAlloc
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
		n    int
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
	wantSize(t, err, "state comparisons while minimizing")
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
	t.Logf("BLOWUP lex table refused after %v, %d MiB allocated in total, limit %d lex states: %v", d, alloc>>20, bLexStates.limit, err)
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and allocated %d MiB; want at most 60 s and %d MiB", d, alloc>>20, blowupAllocBudget>>20)
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
	t.Logf("BLOWUP conflict search refused after %v, %d MiB allocated in total: %v", d, alloc>>20, err)
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and allocated %d MiB; want at most 60 s and %d MiB", d, alloc>>20, blowupAllocBudget>>20)
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
		n    int
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
	t.Logf("BLOWUP flatten refused after %v, %d MiB allocated in total: %v", d, alloc>>20, err)
	if d > 60*time.Second || alloc > blowupAllocBudget {
		t.Errorf("took %v and allocated %d MiB; want at most 60 s and %d MiB", d, alloc>>20, blowupAllocBudget>>20)
	}
}

// The automaton of the lexical grammar: states and ranges.
func TestNFABudget(t *testing.T) {
	_, err := Prepare(sizeGrammar("b{0,4294967295}"))
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
	wantSize(t, err, "character ranges read into the classes of one pattern")
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
