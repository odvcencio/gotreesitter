package tablebuild

import (
	"errors"
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// The tests in this file port the #[cfg(test)] module of
// prepare_grammar/process_inlines.rs (v0.27.1).

func term(i int) Symbol    { return lexbuild.TerminalSym(i) }
func nonTerm(i int) Symbol { return lexbuild.NonTerminalSym(i) }

func decorated(sym Symbol, prec lexbuild.Precedence, assoc lexbuild.Associativity, alias string) lexbuild.ProductionStep {
	st := lexbuild.ProductionStep{Symbol: sym, Precedence: prec, Associativity: assoc}
	if alias != "" {
		st.Alias = Alias{Set: true, Value: alias, Named: true}
	}
	return st
}

func precInt(n int32) lexbuild.Precedence {
	return lexbuild.Precedence{Kind: lexbuild.PrecInteger, Int: n}
}

// inlinedProduction is one replacement production: its steps and dynamic precedence.
type inlinedProduction struct {
	steps []lexbuild.ProductionStep
	dyn   int32
}

// inlined returns the ids recorded at (prodID, step) and each one's steps and
// dynamic precedence (the inlined helper of the Rust tests).
func inlined(sg *lexbuild.SyntaxGrammar, m *InlinedProductionMap, prodID, step uint32) ([]uint32, []inlinedProduction, bool) {
	ids, ok := m.InlinedProdIDs(prodID, step)
	if !ok {
		return nil, nil, false
	}
	var prods []inlinedProduction
	for _, id := range ids {
		p := sg.Productions[id]
		prods = append(prods, inlinedProduction{p.Steps, p.DynamicPrecedence})
	}
	return ids, prods, true
}

func emptyLex() *lexbuild.LexicalGrammar { return &lexbuild.LexicalGrammar{} }

// test_basic_inlining
func TestBasicInlining(t *testing.T) {
	// var0: [t10, nt1, t11] (nt1 is inlined)
	// var1: [t12, t13] | [t14]
	sg := &lexbuild.SyntaxGrammar{}
	addVariable(sg, "v0", prod(plainStep(term(10)), plainStep(nonTerm(1)), plainStep(term(11))))
	addVariable(sg, "v1", prod(plainStep(term(12)), plainStep(term(13))), prodSpec{steps: []lexbuild.ProductionStep{plainStep(term(14))}, dyn: -2})
	sg.VariablesToInline = []Symbol{nonTerm(1)}

	m, err := processInlines(sg, emptyLex())
	if err != nil {
		t.Fatal(err)
	}
	prod0 := uint32(sg.VarProds[0][0])

	// Nothing to inline at step 0.
	if _, _, ok := inlined(sg, &m, prod0, 0); ok {
		t.Fatal("an entry at step 0")
	}

	// Inlining variable 1 yields two productions.
	_, prods, ok := inlined(sg, &m, prod0, 1)
	if !ok {
		t.Fatal("no entry at step 1")
	}
	want := []inlinedProduction{
		{[]lexbuild.ProductionStep{plainStep(term(10)), plainStep(term(12)), plainStep(term(13)), plainStep(term(11))}, 0},
		{[]lexbuild.ProductionStep{plainStep(term(10)), plainStep(term(14)), plainStep(term(11))}, -2},
	}
	if !reflect.DeepEqual(prods, want) {
		t.Fatalf("got %+v\nwant %+v", prods, want)
	}
}

// test_nested_inlining
func TestNestedInlining(t *testing.T) {
	// var0: [t10, nt1, t11, nt2, t12]    (nt1, nt2 inlined)
	// var1: [t13] | [nt3, t14]           (nt3 inlined)
	// var2: [t15]
	// var3: [t16]
	sg := &lexbuild.SyntaxGrammar{}
	addVariable(sg, "v0", prod(plainStep(term(10)), plainStep(nonTerm(1)), plainStep(term(11)), plainStep(nonTerm(2)), plainStep(term(12))))
	addVariable(sg, "v1", prod(plainStep(term(13))), prod(plainStep(nonTerm(3)), plainStep(term(14))))
	addVariable(sg, "v2", prod(plainStep(term(15))))
	addVariable(sg, "v3", prod(plainStep(term(16))))
	sg.VariablesToInline = []Symbol{nonTerm(1), nonTerm(2), nonTerm(3)}

	m, err := processInlines(sg, emptyLex())
	if err != nil {
		t.Fatal(err)
	}
	prod0 := uint32(sg.VarProds[0][0])

	ids, prods, ok := inlined(sg, &m, prod0, 1)
	if !ok {
		t.Fatal("no entry at step 1")
	}
	want := []inlinedProduction{
		{[]lexbuild.ProductionStep{plainStep(term(10)), plainStep(term(13)), plainStep(term(11)), plainStep(nonTerm(2)), plainStep(term(12))}, 0},
		{[]lexbuild.ProductionStep{plainStep(term(10)), plainStep(term(16)), plainStep(term(14)), plainStep(term(11)), plainStep(nonTerm(2)), plainStep(term(12))}, 0},
	}
	if !reflect.DeepEqual(prods, want) {
		t.Fatalf("step 1: got %+v\nwant %+v", prods, want)
	}

	// nt2, now at step 3.
	_, prods, ok = inlined(sg, &m, ids[0], 3)
	if !ok {
		t.Fatal("no entry at step 3")
	}
	want = []inlinedProduction{
		{[]lexbuild.ProductionStep{plainStep(term(10)), plainStep(term(13)), plainStep(term(11)), plainStep(term(15)), plainStep(term(12))}, 0},
	}
	if !reflect.DeepEqual(prods, want) {
		t.Fatalf("step 3: got %+v\nwant %+v", prods, want)
	}
}

// test_inlining_with_precedence_and_alias
func TestInliningWithPrecedenceAndAlias(t *testing.T) {
	// var0: [ nt1{prec 1, left}, t10, nt2{alias outer} ]     (nt1, nt2 inlined)
	// var1: [ t11{prec 2, alias inner}, t12 ]
	// var2: [ t13 ]
	none := lexbuild.Precedence{}
	sg := &lexbuild.SyntaxGrammar{}
	addVariable(sg, "v0", prod(
		decorated(nonTerm(1), precInt(1), lexbuild.AssocLeft, ""),
		plainStep(term(10)),
		decorated(nonTerm(2), none, lexbuild.AssocNone, "outer_alias"),
	))
	addVariable(sg, "v1", prod(decorated(term(11), precInt(2), lexbuild.AssocNone, "inner_alias"), plainStep(term(12))))
	addVariable(sg, "v2", prod(plainStep(term(13))))
	sg.VariablesToInline = []Symbol{nonTerm(1), nonTerm(2)}

	m, err := processInlines(sg, emptyLex())
	if err != nil {
		t.Fatal(err)
	}
	prod0 := uint32(sg.VarProds[0][0])

	ids, prods, ok := inlined(sg, &m, prod0, 0)
	if !ok {
		t.Fatal("no entry at step 0")
	}
	want := []inlinedProduction{{[]lexbuild.ProductionStep{
		// The first inlined step keeps its own precedence and alias.
		decorated(term(11), precInt(2), lexbuild.AssocNone, "inner_alias"),
		// The last inlined step inherits the inlined step's precedence.
		decorated(term(12), precInt(1), lexbuild.AssocLeft, ""),
		plainStep(term(10)),
		decorated(nonTerm(2), none, lexbuild.AssocNone, "outer_alias"),
	}, 0}}
	if !reflect.DeepEqual(prods, want) {
		t.Fatalf("step 0: got %+v\nwant %+v", prods, want)
	}

	_, prods, ok = inlined(sg, &m, ids[0], 3)
	if !ok {
		t.Fatal("no entry at step 3")
	}
	want = []inlinedProduction{{[]lexbuild.ProductionStep{
		decorated(term(11), precInt(2), lexbuild.AssocNone, "inner_alias"),
		decorated(term(12), precInt(1), lexbuild.AssocLeft, ""),
		plainStep(term(10)),
		// Every inlined step inherits the inlined step's alias.
		decorated(term(13), none, lexbuild.AssocNone, "outer_alias"),
	}, 0}}
	if !reflect.DeepEqual(prods, want) {
		t.Fatalf("step 3: got %+v\nwant %+v", prods, want)
	}
}

// test_error_when_inlining_tokens
func TestErrorWhenInliningTokens(t *testing.T) {
	sg := &lexbuild.SyntaxGrammar{VariablesToInline: []Symbol{term(0)}}
	lex := &lexbuild.LexicalGrammar{Variables: []lexbuild.LexicalVariable{{Ident: "something", Kind: lexbuild.VarNamed}}}
	_, err := processInlines(sg, lex)
	var pe *ProcessInlinesError
	if !errors.As(err, &pe) || *pe != (ProcessInlinesError{InlineToken, "something"}) {
		t.Fatalf("got %v, want Token(something)", err)
	}
	if err.Error() != "Token `something` cannot be inlined" {
		t.Fatalf("message %q", err.Error())
	}
}

// test_error_when_inlining_removes_all_productions
func TestErrorWhenInliningRemovesAllProductions(t *testing.T) {
	// var0: [nt1]
	// var1: [nt2, t10] (nt2 is inlined)
	// var2: one empty eof-gated production, like a flattened `_eof: _ => eof()`.
	// Inlining var2 into var1 drops var1's only production.
	sg := &lexbuild.SyntaxGrammar{}
	addVariable(sg, "rule0", prod(plainStep(nonTerm(1))))
	addVariable(sg, "rule1", prod(plainStep(nonTerm(2)), plainStep(term(10))))
	addVariable(sg, "rule2", prodSpec{eof: true})
	sg.VariablesToInline = []Symbol{nonTerm(2)}

	_, err := processInlines(sg, emptyLex())
	var pe *ProcessInlinesError
	if !errors.As(err, &pe) || *pe != (ProcessInlinesError{InlineNoReachableProductions, "rule1"}) {
		t.Fatalf("got %v, want NoReachableProductions(rule1)", err)
	}
}

// TestInlineBudget: a production with many inlined steps, each with two
// alternatives, asks for 2^n productions; the budget refuses it.
func TestInlineBudget(t *testing.T) {
	saved := bInlineSteps.limit
	bInlineSteps.limit = 1000
	defer func() { bInlineSteps.limit = saved }()
	sg := &lexbuild.SyntaxGrammar{}
	var steps []lexbuild.ProductionStep
	for i := 0; i < 20; i++ {
		steps = append(steps, plainStep(nonTerm(1)))
	}
	addVariable(sg, "v0", prod(steps...))
	addVariable(sg, "v1", prod(plainStep(term(0))), prod(plainStep(term(1))))
	sg.VariablesToInline = []Symbol{nonTerm(1)}
	_, err := processInlines(sg, emptyLex())
	var se *lexbuild.SizeError
	if !errors.As(err, &se) || se.What != bInlineSteps.what {
		t.Fatalf("got %v, want a size error on %q", err, bInlineSteps.what)
	}
}
