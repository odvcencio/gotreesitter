//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlSchedulerActionLoadBearingCOracleParity is an A3
// (spec.campaign.v7, Workstream A tranche A3) adversarial probe for the
// dispatch.perl arm (parser_result_perl.go,
// testdata/result_compat_ownership_v1.json). It pins the one sub-pass the
// real-corpus dispatcher census observed firing
// (normalizePerlPushExpressionLists;
// cgo_harness/corpus_real/perl/medium__unicode_ranges.pl, "push @found,
// $_;") plus generalizations of its trigger shape, and proves each witness
// is load-bearing two ways: the RAW production tree (result-compatibility
// tail off) diverges from the locked C oracle, and the NORMALIZED tree
// (compat tail on) matches it exactly. If a RAW witness here ever stops
// diverging, the underlying gotreesitter grammar/scheduler defect has been
// fixed upstream and dispatch.perl may be retirable for that witness --
// investigate before treating this test's failure as a regression.
//
// dispatch.perl cannot retire yet: this is the arm's uniform retirement
// condition (testdata/result_compat_ownership_v1.json) failing on the
// authoritative owner (scheduler_action_semantics). The root cause is a
// genuine GLR fork on perl's own declared conflict
// ([$._listexpr, $._term_rightward] in tree-sitter-perl's grammar.js, kept
// live in the generated table because it is declared, not resolved away at
// table-build time): after "push @found" the parser forks into a stack that
// shifts the comma (extending the call's own argument list -- the grouped
// derivation C elects) and a stack that reduces early (closing the call at
// one argument and reopening a list one level up -- the split derivation
// gotreesitter elects). Both stacks reach an identical (state, byte offset)
// repeatedly without ever merging (Go's GSS merge refuses across differing
// materialized shapes), so both survive to acceptance and the final choice
// falls to the shared accepted-stack tie-break
// (stackCompareForResultSelectionWithRawShape, parser_result.go). Error
// cost and dynamic precedence tie at zero for this input, so the decision
// falls through to compareRawStackEntriesRec's (parser_reduce.go) last
// rung: lower generated symbol ID wins. gotreesitter's generated numbering
// happens to give list_expression a lower ID than
// ambiguous_function_call_expression/assignment_expression/return_expression
// for this grammar, so the split shape wins on every input, independent of
// content -- see TestPerlSchedulerActionKnownGapCOracleParity for witnesses
// (unshift, bare join, join-with-assignment, return) where this same defect
// class reaches acceptance uncorrected today. That comparator is shared by
// every grammar's final accepted-stack selection, and the fix is either
// grammar-table-wide symbol renumbering to match the C oracle's own
// generator or a change to the shared comparator's tie-break rule -- both
// carry fleet-wide blast radius that a single-arm change cannot safely
// audit, so this pins the arm as blocked-with-mechanism per
// spec.campaign.v7 workstream A3 rather than forcing a root fix.
func TestPerlSchedulerActionLoadBearingCOracleParity(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source string
	}{
		{
			// Real-corpus witness:
			// cgo_harness/corpus_real/perl/medium__unicode_ranges.pl,
			// inside the byte-class-list loop: "push @found, $_;". This is
			// the sole firing the dispatcher census records for dispatch.perl
			// (parser_result_test/dispatcher_census_test.go,
			// TestDispatcherArmCensusOverRealCorpus).
			name:   "push_two_args_real_corpus_witness",
			source: "push @found, $_;\n",
		},
		{
			name:   "push_three_args",
			source: "push @found, $a, $b;\n",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			src := []byte(test.source)

			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			rawTree, err := goParser.ParseNoResultCompatibilityBenchmarkOnly(src)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			defer rawTree.Release()

			normParser := gotreesitter.NewParser(goLang)
			normParser.SetAdmissionCandidateRoute(false)
			normTree, err := normParser.Parse(src)
			if err != nil {
				t.Fatalf("normalized parse: %v", err)
			}
			defer normTree.Release()

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

			var rawVsC, normVsC []string
			compareNodes(rawTree.RootNode(), goLang, cTree.RootNode(), "root", &rawVsC)
			compareNodes(normTree.RootNode(), goLang, cTree.RootNode(), "root", &normVsC)

			if len(rawVsC) == 0 {
				t.Fatalf(
					"raw tree now matches the C oracle for %q; the upstream grammar/scheduler election "+
						"defect this arm patches around may be fixed -- investigate dispatch.perl retirement "+
						"before accepting this as passing",
					test.name,
				)
			}
			if len(normVsC) != 0 {
				t.Fatalf("normalized (dispatch.perl-corrected) tree diverges from the C oracle: %s", strings.Join(normVsC, " | "))
			}
		})
	}
}

// TestPerlSchedulerActionKnownGapCOracleParity pins the same
// ambiguous_function_call_expression election defect
// (TestPerlSchedulerActionLoadBearingCOracleParity's doc comment) on shapes
// dispatch.perl's sole remaining sub-pass (normalizePerlPushExpressionLists)
// does not reach today: a second built-in list operator (unshift, uncovered
// because normalizePerlPushExpressionLists matches only fn.Text=="push"),
// and join/return, whose grouped-input shape a former sub-pass pair
// (normalizePerlJoinAssignmentLists, normalizePerlReturnExpressionLists,
// removed as dead code once this file's own cases proved the underlying
// declared-conflict election never produces that grouped input on any real
// derivation -- production always resolves the tie to the split shape) used
// to target. The root cause is a runtime tie-break, not a materialization
// gap: when error cost and dynamic precedence both tie, the shared
// accepted-stack raw-shape comparator (compareRawStackEntriesRec,
// parser_reduce.go) falls back to comparing the two candidate derivations'
// top symbol IDs, and gotreesitter's generated symbol numbering for
// list_expression happens to sit below ambiguous_function_call_expression /
// assignment_expression / return_expression, so the comparator prefers the
// split shape on every input, independent of content. That comparator is
// shared by every grammar's final accepted-stack selection; making it (or
// gotreesitter's symbol numbering) agree with the C oracle here without
// auditing every other language's tie-break outcomes is out of scope for a
// single-arm fix. dispatch.perl's overall arm remains blocked (see
// TestPerlSchedulerActionLoadBearingCOracleParity), so no route/registry
// disposition changes here.
//
// wantDivergence stays true for every case: if one flips to false, the same
// grammar/scheduler defect has been fixed upstream for that shape.
func TestPerlSchedulerActionKnownGapCOracleParity(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		source         string
		wantDivergence bool
	}{
		{
			// Same shape as the push witness, uncovered: the push sub-pass
			// matches only fn.Text(source) == "push".
			name:           "unshift_two_args_uncovered_by_push_subpass",
			source:         "unshift @found, $_;\n",
			wantDivergence: true,
		},
		{
			// The tie-break always resolves to the split shape (see this
			// function's doc comment), so no sub-pass ever sees the grouped
			// input a join-rewrite would need to fire.
			name:           "join_assignment_precondition_never_matches",
			source:         "my $x = join \"\\n\", \"a\", \"b\";\n",
			wantDivergence: true,
		},
		{
			name:           "join_bare_uncovered_no_assignment_subpass",
			source:         "join \"\\n\", \"a\", \"b\";\n",
			wantDivergence: true,
		},
		{
			// Same always-split tie-break outcome as join_assignment above.
			name:           "return_precondition_never_matches",
			source:         "sub f { return $a, $b; }\n",
			wantDivergence: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			src := []byte(test.source)

			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			rawTree, err := goParser.ParseNoResultCompatibilityBenchmarkOnly(src)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			defer rawTree.Release()

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
			compareNodes(rawTree.RootNode(), goLang, cTree.RootNode(), "root", &mismatches)
			if test.wantDivergence {
				if len(mismatches) == 0 {
					t.Fatalf(
						"expected %q to diverge from the C oracle, but the raw tree now matches; the "+
							"underlying scheduler-election defect may be fixed -- flip wantDivergence to "+
							"false and re-verify before treating dispatch.perl as retirable for this shape",
						test.name,
					)
				}
				t.Skipf("known scheduler-action gap, not covered by any dispatch.perl sub-pass today:\n%s", strings.Join(mismatches, "\n"))
				return
			}
			if len(mismatches) != 0 {
				t.Fatalf("raw and C trees differ:\n%s", strings.Join(mismatches, "\n"))
			}
		})
	}
}
