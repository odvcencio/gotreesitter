package lexbuild

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

func sizeGrammar(pattern string) []byte {
	return []byte(`{"name":"t","rules":{"source":{"type":"PATTERN","value":"` + pattern + `"}}}`)
}

func withLimits(t *testing.T, states, ids, adv int) {
	t.Helper()
	os, oi, oa := maxLexStates, maxLexNFAIDs, maxLexAdvances
	maxLexStates, maxLexNFAIDs, maxLexAdvances = states, ids, adv
	t.Cleanup(func() { maxLexStates, maxLexNFAIDs, maxLexAdvances = os, oi, oa })
}

func tokenZero() []TokenSet {
	var ts TokenSet
	ts.Insert(terminalSym(0))
	return []TokenSet{ts}
}

// A small budget stops BuildUnmerged and BuildLexTables with a *SizeError, for each of the three limits,
// and a budget that is large enough builds.
func TestLexSizeBudget(t *testing.T) {
	p, err := Prepare(sizeGrammar("abcdefghijklmnopqrstuvwxyz"))
	if err != nil {
		t.Fatal(err)
	}
	if tb, _, err := p.BuildUnmerged(tokenZero()); err != nil || tb.NumStates() != 27 {
		t.Fatalf("default limits: states %v, err %v", tb, err)
	}
	for _, tc := range []struct {
		name          string
		s, ids, adv   int
		wantWhatInErr string
	}{
		{"states", 10, 1 << 20, 1 << 20, "lex states"},
		{"automaton states", 1 << 20, 10, 1 << 20, "automaton states"},
		{"transitions", 1 << 20, 1 << 20, 10, "character transitions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withLimits(t, tc.s, tc.ids, tc.adv)
			p, err := Prepare(sizeGrammar("abcdefghijklmnopqrstuvwxyz"))
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = p.BuildUnmerged(tokenZero())
			var se *SizeError
			if !errors.As(err, &se) || !contains(se.What, tc.wantWhatInErr) {
				t.Fatalf("BuildUnmerged: want a SizeError about %q, got %v", tc.wantWhatInErr, err)
			}
			a, err := p.Analyze(nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.BuildLexTables(a, tokenZero()); !errors.As(err, &se) {
				t.Fatalf("BuildLexTables: want a SizeError, got %v", err)
			}
		})
	}
	// The limit is checked before the state is inserted: exactly 27 states fit in a budget of 27.
	withLimits(t, 27, 1<<20, 1<<20)
	if _, _, err := p.BuildUnmerged(tokenZero()); err != nil {
		t.Errorf("a budget of 27 states must build 27 states: %v", err)
	}
	withLimits(t, 26, 1<<20, 1<<20)
	if _, _, err := p.BuildUnmerged(tokenZero()); err == nil {
		t.Error("a budget of 26 states must refuse 27 states")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// The pattern [ab]*a[ab]{24} has 29 automaton states and 33,554,432 reachable lex states. With the
// default limits the construction must stop quickly and within bounded memory, and say why.
func TestLexSizeBlowupRefused(t *testing.T) {
	p, err := Prepare(sizeGrammar("[ab]*a[ab]{24}"))
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	_, _, err = p.BuildUnmerged(tokenZero())
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	var se *SizeError
	if !errors.As(err, &se) {
		t.Fatalf("want a SizeError, got %v", err)
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	t.Logf("BLOWUP refused after %v, %d MiB allocated in total, limit %d lex states: %v", elapsed, alloc>>20, maxLexStates, err)
	if elapsed > 60*time.Second {
		t.Errorf("took %v", elapsed)
	}
	if alloc > blowupAllocBudget {
		t.Errorf("allocated %d MiB; want at most %d MiB", alloc>>20, blowupAllocBudget>>20)
	}
	a, err := p.Analyze(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.BuildLexTables(a, tokenZero()); !errors.As(err, &se) {
		t.Fatalf("BuildLexTables: want a SizeError, got %v", err)
	}
}

// blowupAllocBudget is the most that TestLexSizeBlowupRefused lets the refused construction allocate.
const blowupAllocBudget = 1 << 30
