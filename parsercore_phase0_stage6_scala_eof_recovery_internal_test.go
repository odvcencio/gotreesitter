//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestStage6S5PotentialReductionCollectorTreatsAcceptAsShiftable(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 1}: {{Type: core.ActionAccept}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{language: &Language{
			TokenCount: 2,
		}},
	}
	reductions, canShift, err := scheduler.s5CollectReductionActions(
		5, diagnosticParserCoreS5AnyTerminal, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !canShift || len(reductions) != 0 {
		t.Fatalf("accept row returned canShift=%t reductions=%d, want true/0", canShift, len(reductions))
	}
}

// TestStage6ScalaEOFReductionBeforeMissingInsertion locks the six-byte EOF
// witness from issue #1053. C reduces the owned-width survivor before it
// inserts one missing closing parenthesis at byte 6.
func TestStage6ScalaEOFReductionBeforeMissingInsertion(t *testing.T) {
	runner := package2ScalaStrictRunner(t)
	const source = "((y)->"
	_, tokenSource, runErr := runner.executeSchedulerOpenWithObserverAndErrorRuns(
		[]byte(source), runner.compact, true, diagnosticParserCoreSeedObserver{}, true,
	)
	if tokenSource != nil {
		tokenSource.Close()
	}
	if runErr != nil {
		if runner.scheduler.receipt != nil {
			t.Fatalf("compact route declined: %v; stop=%+v", runErr, runner.scheduler.receipt.Stop)
		}
		t.Fatalf("compact route declined before receipt: %v", runErr)
	}
	if runner.scheduler.receipt == nil || runner.scheduler.receipt.Acceptance == nil {
		t.Fatalf("compact route returned no acceptance: %+v", runner.scheduler.receipt)
	}

	work := runner.scheduler.receipt.Acceptance.Work
	if work.Passes != 20 || work.PotentialReductionActions != 8 ||
		work.PotentialReductionOutputs != 8 || work.ReductionPromotions != 0 ||
		work.MissingTokenTrials != 1 || work.MissingTokenCommits != 1 ||
		work.RecoveryLineageSelections != 1 || work.RecoveryCondensePasses != 7 ||
		work.SingleHeaderPasses != 6 || work.ActionLookups != 42 ||
		work.Dispatches != 17 || work.Conflicts != 1 || work.ConflictActions != 2 ||
		work.Forks != 1 || work.ConflictActionArmsAdmitted != 2 ||
		work.CausalConflictForks != 1 || work.ConflictHeads != 2 ||
		work.Reductions != 9 || work.OrdinaryShifts != 6 ||
		work.OrdinaryCohorts != 1 || work.Accepts != 1 || work.NoActionDrops != 1 ||
		work.Elections != 6 || work.PerVersionLexRequests != 2 ||
		work.PerVersionLexRestores != 2 || work.PerVersionLexPublications != 7 ||
		work.PerVersionLexViabilityDrops != 1 || work.PeakLiveVersions != 6 ||
		work.Canonicalizations != 17 || work.PeakHeaders != 6 || work.Overflow {
		t.Fatalf("EOF recovery work receipt drifted: %+v", work)
	}
	coreWork := runner.scheduler.receipt.Acceptance.CoreWork
	if coreWork.Shifts != 6 || coreWork.Reductions != 19 ||
		coreWork.ReductionPopRequests != 19 || coreWork.EmittedPopPaths != 19 ||
		coreWork.EmittedPopPayloads != 26 || coreWork.PhysicalHeadMergeAttempts != 4 ||
		coreWork.PhysicalHeadMergeSuccesses != 4 || coreWork.PhysicalHeadMergeInputLinks != 4 ||
		coreWork.PredecessorLinkUnionAttempts != 4 || coreWork.PredecessorLinkUnionDuplicateNoop != 4 ||
		coreWork.GraphLinkAdditionsProxy != 26 || coreWork.LeafConstructionsProxy != 6 ||
		coreWork.ParentConstructionsProxy != 19 || coreWork.Overflow {
		t.Fatalf("EOF recovery core receipt drifted: %+v", coreWork)
	}

	requirePackage2ScalaTree(
		t, runner, source,
		"(compilation_unit (parenthesized_expression (postfix_expression (parenthesized_expression (identifier)) (operator_identifier))))",
		"03a458abd5832f6326e03b833a3890ea8d48ca43eb91ada1951bb020871f49a6",
		6,
	)
	tree, err := runner.materializeSelection([]byte(source), runner.compact, &runner.scheduler)
	if err != nil {
		t.Fatalf("materialize compact EOF selection: %v", err)
	}
	defer tree.Release()
	walkResultTree(tree.RootNode(), func(node *Node) {
		if node.Type(runner.lang) == "ERROR" {
			t.Errorf("compact EOF tree contains a visible ERROR node: %s", tree.RootNode().SExpr(runner.lang))
		}
	})
	t.Logf("Scala EOF recovery receipt: work=%+v core=%+v", work, runner.scheduler.receipt.Acceptance.CoreWork)
}

func TestStage6ScalaEOFReductionRequiresExactArtifactCapability(t *testing.T) {
	runner := package2ScalaStrictRunner(t)
	runner.options.allowCompactS5EOFMissingInsertion = false
	_, tokenSource, runErr := runner.executeSchedulerOpenWithObserverAndErrorRuns(
		[]byte("((y)->"), runner.compact, true, diagnosticParserCoreSeedObserver{}, true,
	)
	if tokenSource != nil {
		tokenSource.Close()
	}
	if runErr == nil {
		t.Fatal("uncertified EOF S5 route unexpectedly accepted")
	}
	if runner.scheduler.receipt == nil {
		t.Fatal("uncertified EOF S5 route returned no decline receipt")
	}
	if got := runner.scheduler.work.MissingTokenTrials; got != 0 {
		t.Fatalf("uncertified EOF S5 route ran %d missing-token trials, want 0", got)
	}
}
