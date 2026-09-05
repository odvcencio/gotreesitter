//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func cleanupG18D6bDriverTestRunner(t *testing.T, runner *parserCoreFreshFullRunner) {
	t.Helper()
	t.Cleanup(func() {
		runner.certificateAdmissionEnabled = false
		runner.frontierRecordingEnabled = false
		runner.frontierVerificationEnabled = false
		runner.options.recordDropCohortCertificates = false
		runner.options.recordDropCohortFrontiers = false
		runner.options.verifyDropCohortFrontiers = false
		if runner.compact != nil {
			if err := runner.compact.ResetReleasingRetention(); err != nil {
				t.Errorf("reset D6b test runner: %v", err)
			}
		}
	})
}

func newG18D6bDriverTestRunner(t *testing.T) *parserCoreFreshFullRunner {
	t.Helper()
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatal(err)
	}
	lang.CompactConvergedReductionSplitDropsCertified = false
	// These frontier proofs inspect one candidate attempt, without recovery retries.
	lang.CompactOwnedEOFRecoveryCertified = false
	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		t.Fatal(err)
	}
	cleanupG18D6bDriverTestRunner(t, runner)
	return runner
}

func TestG18D6bDriverVerificationIsDefaultOff(t *testing.T) {
	runner := newG18D6bDriverTestRunner(t)
	if runner.frontierVerificationEnabled || runner.options.verifyDropCohortFrontiers {
		t.Fatal("D6b driver verification is enabled by default")
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		options: DiagnosticParserCorePrefixOptions{},
		headers: []diagnosticParserCoreHeader{
			{frontierSequence: 0},
			{frontierSequence: 9},
		},
	}
	if err := scheduler.consumeDropCohortFrontierOwned([]int{0}); err != nil {
		t.Fatalf("default-off verifier changed the drop path: %v", err)
	}
}

func withG18D6bDriverSyntheticFrontier(
	t *testing.T,
	consume func(core.SchedulerTransactionToken, *diagnosticParserCoreGenericScheduler, core.DropCohortFrontierHandle) error,
) error {
	t.Helper()
	runner := newG18D6bDriverTestRunner(t)
	compact := runner.compact
	start, err := compact.InternCheckpoint([]byte{1, 2, 3})
	if err != nil {
		return err
	}
	end, err := compact.InternCheckpoint([]byte{4, 5, 6})
	if err != nil {
		return err
	}
	if err := compact.SetPhaseExternalTokenScannerCheckpoints(start, end); err != nil {
		return err
	}
	_, beforeDigest, beforeOK := compact.CheckpointReceipt(start)
	_, afterDigest, afterOK := compact.CheckpointReceipt(end)
	if !beforeOK || !afterOK {
		return fmt.Errorf("synthetic frontier checkpoint receipt is unavailable")
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		return err
	}
	droppedSeed, err := compact.Seed(2, 0)
	if err != nil {
		return err
	}
	return compact.RunFreshSchedulerSession(func(owner core.SchedulerTransactionToken) error {
		identity := core.DropCohortActionIdentity{
			BoundaryState: 2,
			Lookahead:     7,
			ActionOrdinal: 3,
			Action: core.Action{
				Type: core.ActionReduce, State: 19, Symbol: 7, ChildCount: 2,
				DynamicPrecedence: -2, ProductionID: 41, Extra: true,
				ExtraChain: true, Repetition: true,
			},
			NoLookahead: true,
			Selection:   core.DropCohortSelectionConflictPolicy,
		}
		cohort, err := compact.BeginDropCohortOwned(owner, identity, 2)
		if err != nil {
			return err
		}
		checkpoint := core.DropCohortSourceCheckpoint{
			StartByte: 0, EndByte: 1, ScannerStart: start, ScannerEnd: end,
		}
		for branch := uint16(0); branch < 2; branch++ {
			derivation, buildErr := compact.BuildDropCohortDerivationOwned(owner, seed, checkpoint)
			if buildErr != nil {
				return buildErr
			}
			if writeErr := compact.WriteDropCohortMemberOwned(owner, cohort, seed, branch, derivation); writeErr != nil {
				return writeErr
			}
		}
		refs, err := compact.FinalizeDropCohortOwned(owner, cohort)
		if err != nil {
			return err
		}
		token := core.DropCohortFrontierToken{
			Symbol: 9, StartByte: 0, EndByte: 1,
			ScannerBefore: start, ScannerAfter: end,
			ScannerBeforeDigest: beforeDigest, ScannerAfterDigest: afterDigest,
		}
		handle, complete, err := compact.PublishDropCohortFrontierOwned(
			owner, 11, token, []core.Head{seed, droppedSeed}, []uint64{0, 1},
			[]core.DropCohortRefSet{refs, refs},
		)
		if err != nil {
			return err
		}
		if !complete {
			return fmt.Errorf("synthetic frontier did not complete")
		}
		scheduler := &diagnosticParserCoreGenericScheduler{
			compact:            compact,
			freshSessionOwner:  &owner,
			checkpointBeforeID: start,
			checkpointID:       end,
			token:              Token{Symbol: 9, StartByte: 0, EndByte: 1},
			currentElection: DiagnosticParserCoreElection{
				ScannerBefore: DiagnosticParserCoreScannerCheckpoint{Length: 3},
				ScannerAfter:  DiagnosticParserCoreScannerCheckpoint{Length: 3},
			},
			electionIndex: 10,
			options:       DiagnosticParserCorePrefixOptions{verifyDropCohortFrontiers: true},
			headers: []diagnosticParserCoreHeader{
				{head: seed, frontierSequence: uint32(handle.Sequence), dropCohortRefs: refs},
				{head: droppedSeed, frontierSequence: uint32(handle.Sequence), dropCohortRefs: refs},
			},
		}
		return consume(owner, scheduler, handle)
	})
}

func TestG18D6bDriverConsumesSyntheticValidFrontier(t *testing.T) {
	err := withG18D6bDriverSyntheticFrontier(t, func(owner core.SchedulerTransactionToken, scheduler *diagnosticParserCoreGenericScheduler, handle core.DropCohortFrontierHandle) error {
		if err := scheduler.consumeDropCohortFrontierOwned([]int{1}); err != nil {
			return err
		}
		state, _, _, _, err := scheduler.compact.DropCohortFrontierStateOwned(owner, handle)
		if err != nil {
			return err
		}
		if state != core.DropCohortFrontierConsumed {
			return fmt.Errorf("synthetic frontier state=%v, want consumed", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestG18D6bDriverRejectsQueryCompileIncompleteFrontier(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile")
	runner := newG18D6bDriverTestRunner(t)
	runner.certificateAdmissionEnabled = true
	runner.frontierRecordingEnabled = true
	runner.frontierVerificationEnabled = true
	dropCallbacks := 0
	tree, err := runner.parseWithObserver(fixture.Source, diagnosticParserCoreSeedObserver{
		frontierPublished: func(_ *diagnosticParserCoreGenericScheduler, _ core.SchedulerTransactionToken, dropIndices []int) error {
			if len(dropIndices) != 0 {
				dropCallbacks++
			}
			return nil
		},
	})
	if tree != nil {
		tree.Release()
		t.Fatal("query_compile incomplete frontier was accepted")
	}
	if dropCallbacks != 0 {
		t.Fatalf("query_compile published %d complete drop frontiers", dropCallbacks)
	}
	if err == nil || !strings.Contains(err.Error(), "requires a nonzero frontier sequence") {
		t.Fatalf("query_compile error=%v, want zero-sequence rejection", err)
	}
}

func TestG18D6bDriverRejectsMutatedPublishedToken(t *testing.T) {
	var consumeErr error
	runErr := withG18D6bDriverSyntheticFrontier(t, func(owner core.SchedulerTransactionToken, scheduler *diagnosticParserCoreGenericScheduler, handle core.DropCohortFrontierHandle) error {
		token, ok := scheduler.dropCohortFrontierTokenOwned(owner)
		if !ok {
			return fmt.Errorf("synthetic frontier token rebuild failed")
		}
		token.Symbol++
		consumeErr = scheduler.compact.ConsumeDropCohortFrontierSequenceOwned(
			owner, handle.Sequence, uint64(scheduler.electionIndex+1), token,
			[]core.Head{scheduler.headers[0].head, scheduler.headers[1].head},
			[]core.DropCohortRefSet{scheduler.headers[0].dropCohortRefs, scheduler.headers[1].dropCohortRefs},
			[]int{1},
		)
		return nil
	})
	if consumeErr == nil || !strings.Contains(consumeErr.Error(), "frontier token mismatch") {
		t.Fatalf("mutated frontier token error=%v, want token mismatch", consumeErr)
	}
	if runErr != nil && !strings.Contains(runErr.Error(), "frontier token mismatch") {
		t.Fatalf("mutated frontier session error=%v, want token mismatch", runErr)
	}
}

func TestG18D6bDriverRejectsZeroAndMixedSequences(t *testing.T) {
	for _, test := range []struct {
		name       string
		sequences  []uint32
		wantDetail string
	}{
		{name: "zero", sequences: []uint32{0, 0}, wantDetail: "nonzero frontier sequence"},
		{name: "mixed", sequences: []uint32{7, 8}, wantDetail: "one common frontier sequence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := make([]diagnosticParserCoreHeader, len(test.sequences))
			for index, sequence := range test.sequences {
				headers[index] = diagnosticParserCoreHeader{
					head:             core.Head{Node: core.NodeID(index + 1)},
					frontierSequence: sequence,
				}
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact:           &core.Core{},
				freshSessionOwner: &core.SchedulerTransactionToken{},
				options: DiagnosticParserCorePrefixOptions{
					verifyDropCohortFrontiers: true,
				},
				headers: headers,
			}
			if err := scheduler.consumeDropCohortFrontierOwned([]int{1}); err == nil || !strings.Contains(err.Error(), test.wantDetail) {
				t.Fatalf("sequence rejection=%v, want %q", err, test.wantDetail)
			}
		})
	}
}
