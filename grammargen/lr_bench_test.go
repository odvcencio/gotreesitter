package grammargen

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkLRTableGeneration benchmarks LR table construction on the built-in
// grammars of various sizes.
func BenchmarkLRTableGeneration(b *testing.B) {
	grammars := []struct {
		name string
		fn   func() *Grammar
	}{
		{"json", JSONGrammar},
		{"calc", CalcGrammar},
		{"ini", INIGrammar},
		{"lox", LoxGrammar},
		{"mustache", MustacheGrammar},
	}

	for _, g := range grammars {
		b.Run(g.name, func(b *testing.B) {
			gram := g.fn()
			ng, err := Normalize(gram)
			if err != nil {
				b.Fatalf("normalize: %v", err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := buildLRTables(ng)
				if err != nil {
					b.Fatalf("buildLRTables: %v", err)
				}
			}
		})
	}
}

// TestLRGenerationScaling measures generation time for grammars of known
// complexity. This is not a benchmark (it always runs) but reports timing
// for manual inspection.
func TestLRGenerationScaling(t *testing.T) {
	grammars := []struct {
		name string
		fn   func() *Grammar
	}{
		{"json", JSONGrammar},
		{"calc", CalcGrammar},
		{"ini", INIGrammar},
		{"lox", LoxGrammar},
		{"mustache", MustacheGrammar},
	}

	for _, g := range grammars {
		t.Run(g.name, func(t *testing.T) {
			gram := g.fn()
			ng, err := Normalize(gram)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}

			start := time.Now()
			tables, err := buildLRTables(ng)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("buildLRTables: %v", err)
			}

			t.Logf("%-12s: %d prods, %d symbols (%d tokens), %d states, %v",
				g.name, len(ng.Productions), len(ng.Symbols),
				ng.TokenCount(), tables.StateCount, elapsed)
		})
	}
}

// synthGrammar creates a synthetic grammar with the given number of rules and
// terminals, used for scalability testing.
func synthGrammar(numRules, numTerminals int) *Grammar {
	g := NewGrammar("synth")
	rules := make(map[string]*Rule)

	// Create terminal patterns.
	for i := 0; i < numTerminals; i++ {
		name := fmt.Sprintf("t%d", i)
		rules[name] = Pat(fmt.Sprintf("[a-z]%d", i))
	}

	// Create rules that use the terminals.
	for i := 0; i < numRules; i++ {
		name := fmt.Sprintf("rule_%d", i)
		// Each rule is a choice between several sequences of terminals.
		numAlts := 3
		if numAlts > numTerminals {
			numAlts = numTerminals
		}
		alts := make([]*Rule, numAlts)
		for j := 0; j < numAlts; j++ {
			tIdx := (i*numAlts + j) % numTerminals
			alts[j] = Seq(Sym(fmt.Sprintf("t%d", tIdx)), Sym(fmt.Sprintf("t%d", (tIdx+1)%numTerminals)))
		}
		rules[name] = Choice(alts...)
	}

	// Source rule uses all the other rules.
	srcAlts := make([]*Rule, numRules)
	for i := 0; i < numRules; i++ {
		srcAlts[i] = Sym(fmt.Sprintf("rule_%d", i))
	}
	rules["source"] = Repeat(Choice(srcAlts...))

	g.Rules = rules
	g.RuleOrder = make([]string, 0, len(rules))
	g.RuleOrder = append(g.RuleOrder, "source")
	for i := 0; i < numRules; i++ {
		g.RuleOrder = append(g.RuleOrder, fmt.Sprintf("rule_%d", i))
	}
	for i := 0; i < numTerminals; i++ {
		g.RuleOrder = append(g.RuleOrder, fmt.Sprintf("t%d", i))
	}

	return g
}

// TestLALRBuildsStates verifies the DeRemer/Pennello LALR algorithm builds
// a valid LR(0) automaton with lookaheads on reduce items.
func TestLALRBuildsStates(t *testing.T) {
	grammars := []struct {
		name string
		fn   func() *Grammar
	}{
		{"json", JSONGrammar},
		{"calc", CalcGrammar},
		{"ini", INIGrammar},
		{"lox", LoxGrammar},
		{"mustache", MustacheGrammar},
	}

	for _, g := range grammars {
		t.Run(g.name, func(t *testing.T) {
			gram := g.fn()
			ng, err := Normalize(gram)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}

			ctx := makeLRContext(ng)
			ctx.computeFirstSets()
			lalrSets := ctx.buildItemSetsLALR()
			t.Logf("LALR states: %d (from %d productions)", len(lalrSets), len(ng.Productions))

			// Check: every reduce item has at least one lookahead.
			for si, set := range lalrSets {
				for _, ce := range set.cores {
					prod := &ng.Productions[ce.prodIdx]
					if ce.dot >= len(prod.RHS) && ce.prodIdx != ng.AugmentProdID {
						if ce.lookaheads.empty() {
							t.Errorf("state %d: reduce item prod %d (%s → ...) has no lookaheads",
								si, ce.prodIdx, ng.Symbols[prod.LHS].Name)
						}
					}
				}
			}

			// Check: state 0 exists and has the augmented production at dot=0.
			if len(lalrSets) == 0 {
				t.Fatal("no states generated")
			}
			found := false
			for _, ce := range lalrSets[0].cores {
				if ce.prodIdx == ng.AugmentProdID && ce.dot == 0 {
					found = true
				}
			}
			if !found {
				t.Error("state 0 missing augmented production")
			}

			// Check: there's an accept action somewhere.
			hasAccept := false
			augProd := &ng.Productions[ng.AugmentProdID]
			for si, set := range lalrSets {
				for _, ce := range set.cores {
					if ce.prodIdx == ng.AugmentProdID && ce.dot == len(augProd.RHS) {
						if ce.lookaheads.contains(0) {
							hasAccept = true
							t.Logf("accept state: %d", si)
						}
					}
				}
			}
			if !hasAccept {
				t.Error("no accept state found")
			}
		})
	}
}

// makeLRContext creates an lrContext ready for FIRST set computation and item set building.
func makeLRContext(ng *NormalizedGrammar) *lrContext {
	tokenCount := ng.TokenCount()
	ctx := &lrContext{
		ng:         ng,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: make(map[int][]int),
		betaCache:  make(map[uint32]*betaResult),
		tokenCount: tokenCount,
	}
	for i := range ng.Productions {
		lhs := ng.Productions[i].LHS
		ctx.prodsByLHS[lhs] = append(ctx.prodsByLHS[lhs], i)
	}
	for i := range ng.Productions {
		if ng.Productions[i].IsExtra {
			ctx.extraProdIndices = append(ctx.extraProdIndices, i)
		}
	}
	if len(ctx.extraProdIndices) > 0 {
		ctx.allTerminals = newBitset(tokenCount)
		for i := 0; i < tokenCount; i++ {
			ctx.allTerminals.add(i)
		}
	}
	ctx.dot0Index = make([]int, len(ng.Productions))
	for i := range ctx.dot0Index {
		ctx.dot0Index[i] = -1
	}
	return ctx
}

func BenchmarkLRTableScaling(b *testing.B) {
	sizes := []struct {
		rules, terminals int
	}{
		{10, 10},
		{50, 30},
		{100, 50},
		{200, 80},
	}

	for _, s := range sizes {
		name := fmt.Sprintf("r%d_t%d", s.rules, s.terminals)
		b.Run(name, func(b *testing.B) {
			gram := synthGrammar(s.rules, s.terminals)
			ng, err := Normalize(gram)
			if err != nil {
				b.Fatalf("normalize: %v", err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := buildLRTables(ng)
				if err != nil {
					b.Fatalf("buildLRTables: %v", err)
				}
			}
		})
	}
}
