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
// (tree-sitter-perl grammar.js:170). At state 873 with lookahead "(" the blob
// holds two actions: reduce to `function`, and shift into state 4185. State
// 4185 has an action only on `_NONASSOC` (symbol 289), the zero-width external
// token that function_call_expression (grammar.js:784-790) uses to commit to
// the unambiguous form. C lexes every stack version with that version's own
// lex mode, so the version parked on 4185 always receives `_NONASSOC`. The Go
// runtime lexes one token for the whole GLR frontier, and the shared-frontier
// arbitration (preferGLRUnionDFAOverExternalToken) used to hand the frontier
// the ")" DFA token instead, because ")" scored higher on the name-shape
// specificity tie-break. That killed the only fork that could reach
// function_call_expression.
//
// SCOPE, measured, not assumed. This repair does NOT make every bareword call
// in Perl match C. It restores the derivation for a bareword call whose fork
// does not compete with another bareword call's fork. When a file carries two
// such calls, one of them still loses the election; see
// TestPerlBarewordCallResidualDivergenceCOracleParity for those witnesses. The
// residual is the separate family-D derivation-selection lane.
//
// "&g();" and "print(\"x\");" were already clean before the repair (each
// reaches state 4185 with a single live stack, so no arbitration ran). They are
// kept here as controls.
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

// TestPerlBarewordCallResidualDivergenceCOracleParity records the bareword
// call shapes the fork rescue does NOT fix, so the repair's scope stays honest.
// In each source two bareword calls compete, and exactly one of them still
// materializes as ambiguous_function_call_expression where C produces
// function_call_expression. The fork now survives to the end in both cases, so
// this is no longer a lexing defect: it is the derivation SELECTION between two
// surviving derivations, the separate family-D tie-break lane.
//
// wantDivergence is implicit and always true here. If a case stops diverging,
// that lane has been repaired for that shape -- re-verify and move the witness
// up into TestPerlBarewordCallDerivationCOracleParity.
func TestPerlBarewordCallResidualDivergenceCOracleParity(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source string
	}{
		// The FIRST call stays ambiguous; the second is repaired.
		{"two_top_level_calls", "A();\nB();\n"},
		// The SECOND call stays ambiguous; the first is repaired.
		{"two_sub_bodies", "sub f { A(); }\nsub g { B(); }\n"},
		// Real-corpus shape: bareword calls inside try/catch/finally, the
		// 12 remaining production points in corpus_real/perl/medium__statements.pm.
		{"try_catch_blocks", "try { A(); } catch($e) { B(); }\n"},
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
			if len(mismatches) == 0 {
				t.Fatalf(
					"expected %q to still diverge from the C oracle on the derivation-selection lane, "+
						"but it now matches; move this witness into "+
						"TestPerlBarewordCallDerivationCOracleParity after re-verifying",
					test.source)
			}
			t.Skipf("known derivation-selection residual:\n  go: %s\n   c: %s\n%s",
				tree.RootNode().SExpr(goLang), cTree.RootNode().ToSexp(),
				strings.Join(mismatches, "\n"))
		})
	}
}

// TestPerlForkedFrontierKeepsRecoveringToEOF is the permanent regression
// witness for the cost of keeping a rescued fork alive. Holding an extra live
// version past the point where it used to be pruned changes how many versions
// the no-action recovery ladder sees. While that ladder asked len(stacks), a
// frontier that had ever forked lost its recovery entirely: every version was
// killed, the parse ended at ParseStopNoStacksAlive, and the tail of the file
// was never parsed. This source lost 6 of its 41 bytes that way.
//
// The gates now ask whether a LIVE sibling remains
// (anotherLiveParseStackRemains), so the last live version still reaches the
// recovery ladder. The tree here is an ERROR tree in both runtimes -- the
// source is deliberately malformed -- so this test asserts coverage, not
// shape: the parse must reach the end of the source and must recover the
// trailing statement, exactly as the C oracle does.
func TestPerlForkedFrontierKeepsRecoveringToEOF(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("foo { A(); } bar($e) { B(); }\nmy $z = 1;\n")

	scheduleParityMemoryScavenge(t)
	goParser := gotreesitter.NewParser(goLang)
	goParser.SetAdmissionCandidateRoute(false)
	tree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer tree.Release()

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parse returned a nil tree")
	}
	defer cTree.Close()

	goEnd := int(tree.RootNode().EndByte())
	cEnd := int(cTree.RootNode().EndByte())
	if goEnd < cEnd {
		t.Fatalf("production parse stopped %d byte(s) short of the C oracle (go=%d c=%d of %d):\n  go: %s",
			cEnd-goEnd, goEnd, cEnd, len(source), tree.RootNode().SExpr(goLang))
	}
	if got := tree.RootNode().SExpr(goLang); !strings.Contains(got, "assignment_expression") {
		t.Fatalf("production parse did not recover the trailing `my $z = 1;` statement:\n  go: %s", got)
	}
}
