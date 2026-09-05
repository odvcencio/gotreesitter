//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestCompactRecoveryMaterializationCountsDiscardedNodes(t *testing.T) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		t.Fatal(err)
	}
	var work compactFreshAttemptWork
	scratch := parserCoreRunnerScratch{freshAttemptWork: &work}
	truncated := prepared.source[:len(prepared.source)/2]
	tree, err := materializeDiagnosticParserCoreAcceptedTree(
		prepared.acceptedCore, prepared.acceptedHead, prepared.parser, truncated, &scratch, false, false,
	)
	if tree != nil {
		tree.Release()
	}
	if err == nil || work.allocatedNodes == 0 {
		t.Fatalf("discarded materialization allocations=%d err=%v", work.allocatedNodes, err)
	}
	discarded := work.allocatedNodes
	tree, err = materializeDiagnosticParserCoreAcceptedTree(
		prepared.acceptedCore, prepared.acceptedHead, prepared.parser, prepared.source, &scratch, false, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if work.allocatedNodes != discarded+uint64(tree.parseRuntime.NodesAllocated) {
		t.Fatalf("retry allocations=%d, want discarded %d plus successful %d", work.allocatedNodes, discarded, tree.parseRuntime.NodesAllocated)
	}
	scratch.freshAttemptWork = nil
	if len(scratch.nodesByID) != 0 || len(scratch.nodes) != 0 {
		t.Fatal("materialization retained tree pointers")
	}
}

func TestCompactRecoveryProfiledAllocationScope(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	source := []byte("func f(){}\nfunc g(){}\n")
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	edited, edit := compactExecutionEdit(source, 21, 22, "+")
	old.Edit(edit)
	var borrowedWork incrementalParseTiming
	borrowed, _, recoveryDeclined := p.attemptCompactIncrementalParse(edited, old, &borrowedWork)
	if borrowed != nil {
		borrowed.Release()
	}
	if borrowed != nil || !recoveryDeclined || borrowedWork.tokensConsumed == 0 {
		t.Fatal("fixture did not exercise the discarded borrowed attempt")
	}
	runner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	plain, plainErr := runner.parseWithObserverAndErrorRuns(edited, diagnosticParserCoreSeedObserver{}, false, false)
	if plain != nil {
		plain.Release()
	}
	plainTokens := runner.scheduler.tokens
	if plainErr == nil || plainTokens == 0 {
		t.Fatal("fixture did not exercise the discarded plain-first attempt")
	}
	recovery, err := runner.parseWithObserverAndErrorRuns(edited, diagnosticParserCoreSeedObserver{}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	recoveryTokens := recovery.parseRuntime.TokensConsumed
	recovery.Release()
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if !next.parseRuntime.CompactIncrementalFullRecoveryRoute ||
		profile.NewNodesAllocated < uint64(next.parseRuntime.NodesAllocated) {
		t.Fatal("profile omitted compact recovery allocations")
	}
	if want := borrowedWork.tokensConsumed + plainTokens + recoveryTokens; profile.TokensConsumed != want {
		t.Fatalf("profile tokens=%d, want borrowed %d + plain %d + recovery %d", profile.TokensConsumed, borrowedWork.tokensConsumed, plainTokens, recoveryTokens)
	}
	if runner.scratch.freshAttemptWork != nil {
		t.Fatal("profile retained its attempt counter")
	}
}

func TestCompactRecoveryDeclineReleasesAttemptWork(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("func f(){}\nfunc g(){}+@")
	plain, plainErr := runner.parseWithObserverAndErrorRuns(source, diagnosticParserCoreSeedObserver{}, false, false)
	if plain != nil {
		plain.Release()
	}
	plainTokens := runner.scheduler.tokens
	if plainErr == nil || plainTokens == 0 {
		t.Fatal("fixture requires a failed plain attempt")
	}
	var timing incrementalParseTiming
	tree := p.attemptCompactIncrementalRecoveryFullParse(source, "recovery probe", true, &timing)
	if tree != nil {
		tree.Release()
		t.Fatal("unsupported lexical reentry unexpectedly succeeded")
	}
	if timing.tokensConsumed != plainTokens+runner.scheduler.tokens {
		t.Fatalf("declined tokens=%d, want plain %d + recovery %d", timing.tokensConsumed, plainTokens, runner.scheduler.tokens)
	}
	if runner.scratch.freshAttemptWork != nil {
		t.Fatal("declined fallback retained its attempt counter")
	}
}

type compactRecoveryPanicTables struct{ *parserCoreRootTables }

func (compactRecoveryPanicTables) Actions(core.StateID, core.Symbol) (core.ActionRow, error) {
	panic("compact recovery attempt probe")
}

func TestCompactRecoveryPanicReleasesAttemptWork(t *testing.T) {
	p := newCompactRecoveryVersionTurnGoParser(t)
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	runner.compact, err = core.New(compactRecoveryPanicTables{runner.tables}, runner.options.Limits)
	if err != nil {
		t.Fatal(err)
	}
	panicked := false
	func() {
		defer func() { panicked = recover() == "compact recovery attempt probe" }()
		var timing incrementalParseTiming
		p.attemptCompactIncrementalRecoveryFullParse([]byte("func f(){}+"), "recovery probe", true, &timing)
	}()
	if !panicked || runner.scratch.freshAttemptWork != nil {
		t.Fatalf("panic=%t retained attempt work=%t", panicked, runner.scratch.freshAttemptWork != nil)
	}
}
