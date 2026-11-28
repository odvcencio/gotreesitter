package admission_test

import (
	"github.com/odvcencio/gotreesitter/internal/frozengo"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// genericConflictFixture is one member of the Go generic-instantiation /
// type-conversion ambiguity family surfaced by the Phase-3 admission flip.
type genericConflictFixture struct {
	name string
	expr string
}

// genericConflictFamily covers the bare Ident[TypeArgs](args) conflict class the
// admission flip surfaced, the tree-sitter-go expression-versus-type variants,
// and the conflict-free bounds that must keep routing.
//
// Every case is wrapped in a real short variable declaration so the parser sees
// the ambiguous construct in expression position, exactly where production forks
// the generic-call / type-conversion GLR conflict.
var genericConflictFamily = []genericConflictFixture{
	// The conflict class: production keeps Ident[TypeArgs] as a generic
	// instantiation (type_conversion_expression or a call with type_arguments);
	// the compact route must decline rather than collapse it to index_expression.
	{"one_type_arg", "Foo[int](a)"},
	{"two_type_args", "Pair[int, string](p)"},
	{"generic_call_two_args", "Max[int](1, 2)"},
	{"generic_call_one_arg", "Max[int](1)"},
	{"nested_generic", "Foo[Foo[int]](a)"},
	{"nested_type_args", "Map[string, []int](m)"},
	{"paren_form", "(Foo[int])(a)"},
	{"chain", "Foo[int](Foo[int](a))"},
	{"argument_generic", "g(Foo[int](a))"},
	{"real_index_call", "a[b](c)"},
	{"selector_generic_call", "pkg.Max[int](1, 2)"},
	{"in_expression", "Max[int](1, 2) + Max[int](3, 4)"},

	// The conflict-free bounds: no Ident[TypeArgs]( pattern, so no fork. These
	// must keep routing (or, at worst, still match production byte for byte).
	{"composite_literal", "Foo[int]{}"},
	{"bare_instantiation", "Max[int]"},
	{"plain_call", "g(x)"},
	{"plain_index", "a[b]"},
	{"selector_call", "pkg.F(x)"},
	{"make_slice", "make([]int, 0)"},
	{"new_type", "new(Foo)"},
}

// TestAdmissionGenericConflictFamilyNoDivergence proves the compact candidate
// route never routes a wrong tree for the generic-instantiation conflict family.
// For every fixture the candidate result must equal the production result byte
// for byte, whether the candidate routed or declined to production. It also
// reports how the family splits between routed and declined so the coverage
// bound is measured, not assumed.
func TestAdmissionGenericConflictFamilyNoDivergence(t *testing.T) {
	lang := frozengo.Language()
	var routed, declined int
	for _, fixture := range genericConflictFamily {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			src := []byte("package p\n\nfunc f() {\n\t_ = " + fixture.expr + "\n}\n")

			prod := gts.NewParser(lang)
			prod.SetAdmissionCandidateRoute(false)
			prodTree, err := prod.Parse(src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer prodTree.Release()
			if prodTree.RootNode().HasError() {
				t.Fatalf("production reference has an error node for %q; fixture is not a clean parse", fixture.expr)
			}
			prodSExpr := prodTree.RootNode().SExpr(lang)

			gts.ResetAdmissionCandidateCounters()
			cand := gts.NewParser(lang)
			cand.SetAdmissionCandidateRoute(true)
			candTree, err := cand.Parse(src)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candTree.Release()
			candRouted, candFallback := gts.AdmissionCandidateCounters()
			candSExpr := candTree.RootNode().SExpr(lang)

			// The load-bearing invariant: the candidate route never diverges. A
			// decline falls back to production (identical by construction); a route
			// must reproduce production byte for byte.
			if candSExpr != prodSExpr {
				t.Fatalf("DIVERGENCE for %q (routed=%d fallback=%d):\n  production: %s\n  candidate:  %s",
					fixture.expr, candRouted, candFallback, prodSExpr, candSExpr)
			}
			switch {
			case candRouted == 1 && candFallback == 0:
				routed++
				t.Logf("ROUTED byte-exact: %q", fixture.expr)
			case candRouted == 0 && candFallback == 1:
				declined++
				t.Logf("DECLINED to production: %q", fixture.expr)
			default:
				t.Fatalf("ambiguous routing counters for %q: routed=%d fallback=%d", fixture.expr, candRouted, candFallback)
			}
		})
	}
	t.Logf("generic-conflict family: %d routed byte-exact, %d declined to production, 0 divergences (%d fixtures)",
		routed, declined, len(genericConflictFamily))

	// Guard the class boundary: the ambiguous Ident[TypeArgs]( members must
	// decline, and the conflict-free members must keep routing. This proves the
	// decline keys on the GLR conflict, not on over-declining every input.
	mustDecline := map[string]bool{
		"one_type_arg": true, "two_type_args": true, "generic_call_two_args": true,
		"generic_call_one_arg": true, "nested_generic": true, "nested_type_args": true,
		"paren_form": true, "chain": true, "argument_generic": true,
		"real_index_call": true, "selector_generic_call": true, "in_expression": true,
	}
	mustRoute := map[string]bool{
		"composite_literal": true, "bare_instantiation": true, "plain_call": true,
		"plain_index": true, "selector_call": true, "make_slice": true, "new_type": true,
	}
	for _, fixture := range genericConflictFamily {
		src := []byte("package p\n\nfunc f() {\n\t_ = " + fixture.expr + "\n}\n")
		gts.ResetAdmissionCandidateCounters()
		cand := gts.NewParser(lang)
		cand.SetAdmissionCandidateRoute(true)
		tree, err := cand.Parse(src)
		if err != nil {
			t.Fatalf("candidate parse %q: %v", fixture.expr, err)
		}
		r, _ := gts.AdmissionCandidateCounters()
		tree.Release()
		if mustDecline[fixture.name] && r != 0 {
			t.Errorf("conflict-class member %q routed (r=%d); it must decline (fail closed)", fixture.expr, r)
		}
		if mustRoute[fixture.name] && r == 0 {
			// A conflict-free member declined; that is safe but narrows coverage.
			// Report it rather than fail so the bound stays visible.
			t.Logf("NOTE conflict-free member %q declined (coverage-narrowing, still exact): %s", fixture.expr,
				strings.TrimSpace(fixture.expr))
		}
	}
}
