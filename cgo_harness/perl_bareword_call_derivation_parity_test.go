//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlBarewordCallDerivationCOracleParity pins the Perl bareword call
// derivation against the locked C oracle on the production route.
//
// Perl declares the conflict [$.function, $.function_call_expression]
// (tree-sitter-perl grammar.js:170). At state 873 with lookahead "(" the
// blob holds two actions: reduce to `function`, and shift into state 4185.
// State 4185 has an action only on `_NONASSOC` (symbol 289), the zero-width
// external token that function_call_expression (grammar.js:784-790) uses to
// commit to the unambiguous form. C lexes every stack version with that
// version's own lex mode, so the version parked on 4185 always receives
// `_NONASSOC`. The Go runtime lexes one token for the whole GLR frontier,
// and the shared-frontier arbitration
// (preferGLRUnionDFAOverExternalToken) used to hand the frontier the ")"
// DFA token instead, because ")" scored higher on the name-shape
// specificity tie-break. That killed the only fork that could reach
// function_call_expression, so EVERY bareword call in Perl -- the most
// common construct in the language -- produced
// ambiguous_function_call_expression where C produces
// function_call_expression.
//
// The rescue is in the arbitration: a zero-width external token consumes no
// input, so keeping it costs the stacks that cannot use it nothing (they
// skip it and re-lex the same byte), while dropping it permanently removes
// a derivation. Every source below must therefore match the C oracle
// exactly. "&g();" and "print(\"x\");" were already clean before the fix
// (each reaches state 4185 with a single live stack, so no arbitration
// ran); they are kept here as controls.
func TestPerlBarewordCallDerivationCOracleParity(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source string
	}{
		{"bareword_no_args", "g();"},
		{"bareword_one_scalar_arg", "g($x);"},
		{"bareword_two_number_args", "foo(1, 2);"},
		{"bareword_call_in_assignment", "my $y = f(3);"},
		{"package_qualified_bareword", "Foo::bar(1);"},
		{"ampersand_call_control", "&g();"},
		{"builtin_print_control", "print(\"x\");"},
		{"bareword_call_in_bare_block", "{ A(); }\n"},
		{"bareword_call_in_sub_body", "sub f { A(); }\n"},
		{"bareword_call_in_conditional_block", "if (1) { A(); }\n"},
		// The A3 sweep's local_dynamic_scope witness
		// (perl_a3_certification_sweep_test.go), which this repair burned
		// down out of perlA3KnownDivergences.
		{"local_dynamic_scope", "our $x = 1;\nsub f { local $x = 2; g(); }\n"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			src := []byte(test.source)

			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			tree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer tree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned a nil tree")
			}
			defer cTree.Close()

			var mismatches []string
			compareNodes(tree.RootNode(), goLang, cTree.RootNode(), "root", &mismatches)
			if len(mismatches) != 0 {
				t.Fatalf("production tree diverges from the C oracle for %q:\n  go: %s\n   c: %s\n%s",
					test.source, tree.RootNode().SExpr(goLang), cTree.RootNode().ToSexp(),
					strings.Join(mismatches, "\n"))
			}
		})
	}
}
