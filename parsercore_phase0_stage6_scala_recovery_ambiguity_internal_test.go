//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import "testing"

// TestStage6ScalaRecoveryThroughAmbiguity locks the first Scala grammar-
// certification witness. C selects the absorbed-error lineage after an
// ordinary grammar fork. The compact route must make the same selection.
func TestStage6ScalaRecoveryThroughAmbiguity(t *testing.T) {
	runner := package2ScalaStrictRunner(t)
	const source = "val f = (: Int) => x\n"
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

	acceptance := runner.scheduler.receipt.Acceptance
	work := acceptance.Work
	if work.RecoveryAmbiguityForks == 0 || work.RecoveryLineageSelections != 1 || work.Accepts != 1 {
		t.Fatalf("recovery competition did not execute as required: %+v", work)
	}
	if acceptance.CoreWork.PhysicalHeadMergeAttempts == 0 || acceptance.CoreWork.PhysicalHeadMergeSuccesses == 0 {
		t.Fatalf("physical recovery merging did not execute: %+v", acceptance.CoreWork)
	}
	if len(runner.scheduler.versionLexerRequests) == 0 {
		t.Fatal("per-version lexer ownership did not execute")
	}

	tree, err := runner.materializeSelection([]byte(source), runner.compact, &runner.scheduler)
	if err != nil {
		t.Fatalf("materialize compact selection: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	const wantSExpr = "(compilation_unit (val_definition (identifier) (lambda_expression (bindings (ERROR) (binding (identifier))) (identifier))))"
	if got := root.SExpr(runner.lang); got != wantSExpr {
		t.Fatalf("S-expression=%q, want %q", got, wantSExpr)
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || root.IsError() || !root.HasError() {
		t.Fatalf("root=%d..%d error=%t has_error=%t, want 0..%d non-error with error content",
			root.StartByte(), root.EndByte(), root.IsError(), root.HasError(), len(source))
	}

	var errorNode, binding *Node
	missingCount := 0
	walkResultTree(root, func(node *Node) {
		if node.IsMissing() {
			missingCount++
		}
		switch node.Type(runner.lang) {
		case "ERROR":
			errorNode = node
		case "binding":
			binding = node
		}
	})
	if missingCount != 0 {
		t.Fatalf("materialized missing nodes=%d, want zero", missingCount)
	}
	if errorNode == nil || errorNode.StartByte() != 9 || errorNode.EndByte() != 10 || !errorNode.IsError() {
		t.Fatalf("ERROR node=%v, want error 9..10", errorNode)
	}
	if errorNode.ChildCount() != 1 || errorNode.Child(0).Type(runner.lang) != ":" ||
		errorNode.Child(0).StartByte() != 9 || errorNode.Child(0).EndByte() != 10 {
		t.Fatalf("ERROR child shape is wrong: %v", errorNode)
	}
	if binding == nil || binding.StartByte() != 11 || binding.EndByte() != 14 || binding.ChildCount() != 1 ||
		binding.Child(0).Type(runner.lang) != "identifier" || binding.Child(0).Text([]byte(source)) != "Int" {
		t.Fatalf("binding shape is wrong: %v", binding)
	}
	const wantDigest = "454a064cdf50bdcfa6eb1cdfd1faaebcda41a02995dea6befbe356bb42ed8dda"
	if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, runner.lang); got != wantDigest {
		t.Fatalf("canonical tree digest=%s, want %s", got, wantDigest)
	}
}
