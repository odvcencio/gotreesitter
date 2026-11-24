package tablebuild

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// The tests in this file port the #[cfg(test)] module of
// prepare_grammar/extract_default_aliases.rs (v0.27.1).

func aliasedStep(sym Symbol, name string) lexbuild.ProductionStep {
	return lexbuild.ProductionStep{Symbol: sym, Alias: Alias{Set: true, Value: name, Named: true}}
}

func plainStep(sym Symbol) lexbuild.ProductionStep { return lexbuild.ProductionStep{Symbol: sym} }

// addVariable appends one variable's productions to sg and records its range
// (the add_variable helper of the Rust tests). Each production is its steps
// and dynamic precedence.
func addVariable(sg *lexbuild.SyntaxGrammar, name string, prods ...prodSpec) {
	start := len(sg.Productions)
	for _, p := range prods {
		sg.Productions = append(sg.Productions, lexbuild.Production{Steps: p.steps, DynamicPrecedence: p.dyn, RequiresEOF: p.eof})
	}
	sg.VarProds = append(sg.VarProds, [2]int{start, len(sg.Productions)})
	sg.Variables = append(sg.Variables, lexbuild.SyntaxVariable{Ident: name, Kind: lexbuild.VarNamed})
}

type prodSpec struct {
	steps []lexbuild.ProductionStep
	dyn   int32
	eof   bool
}

func prod(steps ...lexbuild.ProductionStep) prodSpec { return prodSpec{steps: steps} }

// test_extract_simple_aliases
func TestExtractSimpleAliases(t *testing.T) {
	t0, t1, t2, t3 := lexbuild.TerminalSym(0), lexbuild.TerminalSym(1), lexbuild.TerminalSym(2), lexbuild.TerminalSym(3)
	sg := &lexbuild.SyntaxGrammar{}
	// v1: every token aliased.
	addVariable(sg, "_", prod(aliasedStep(t0, "a1"), aliasedStep(t1, "a2"), aliasedStep(t2, "a3"), aliasedStep(t3, "a4")))
	// v2: t0 same alias, t1 unaliased, t2 aliased differently, t3 aliased twice as a6.
	addVariable(sg, "_", prod(aliasedStep(t0, "a1"), plainStep(t1), aliasedStep(t2, "a5"), aliasedStep(t3, "a6"), aliasedStep(t3, "a6")))

	m := extractDefaultAliases(sg, 4)

	// t0 -> a1, t2 -> a3 (v1 wins the tie from appearing first), t3 -> a6 (used
	// twice); t1 has none (it appears unaliased in v2).
	if len(m) != 3 {
		t.Fatalf("default aliases: got %d, want 3: %v", len(m), m)
	}
	want := map[Symbol]string{t0: "a1", t2: "a3", t3: "a6"}
	for s, v := range want {
		a, ok := m[s]
		if !ok || a.Value != v || !a.Named {
			t.Errorf("default alias of %v: got %+v, want %s named", s, a, v)
		}
	}
	if _, ok := m[t1]; ok {
		t.Errorf("t1 has a default alias: %+v", m[t1])
	}

	// Steps that carry their symbol's default are cleared; the rest keep their alias.
	var steps []lexbuild.ProductionStep
	for _, p := range sg.Productions {
		steps = append(steps, p.Steps...)
	}
	wantSteps := []string{"", "a2", "", "a4", "", "", "a5", "", ""}
	for i, w := range wantSteps {
		got := ""
		if steps[i].Alias.Set {
			got = steps[i].Alias.Value
		}
		if got != w {
			t.Errorf("step %d alias: got %q, want %q", i, got, w)
		}
	}
}
