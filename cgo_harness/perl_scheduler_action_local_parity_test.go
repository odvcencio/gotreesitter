//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPerlSchedulerActionLoadBearingCOracleParity keeps the historical push
// fixtures. The exact-blob declared-conflict rules now make their raw trees
// match C directly, and the compatibility pass must preserve that result.
// The function name remains stable for callers that select this regression.
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

			if len(rawVsC) != 0 {
				t.Fatalf("raw tree diverges from the C oracle: %s", strings.Join(rawVsC, " | "))
			}
			if len(normVsC) != 0 {
				t.Fatalf("normalized (dispatch.perl-corrected) tree diverges from the C oracle: %s", strings.Join(normVsC, " | "))
			}
		})
	}
}

// TestPerlSchedulerActionKnownGapCOracleParity retains four former gaps.
// Each raw parse now matches the locked C oracle through the certified
// declared-conflict rules, without a compatibility rewrite.
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
			// This also matches C without the push-only compatibility pass.
			name:           "unshift_two_args_uncovered_by_push_subpass",
			source:         "unshift @found, $_;\n",
			wantDivergence: false,
		},
		{
			// The raw parse now keeps C's grouped join arguments.
			name:           "join_assignment_precondition_never_matches",
			source:         "my $x = join \"\\n\", \"a\", \"b\";\n",
			wantDivergence: false,
		},
		{
			name:           "join_bare_uncovered_no_assignment_subpass",
			source:         "join \"\\n\", \"a\", \"b\";\n",
			wantDivergence: false,
		},
		{
			// Return arguments also retain C's grouping.
			name:           "return_precondition_never_matches",
			source:         "sub f { return $a, $b; }\n",
			wantDivergence: false,
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
