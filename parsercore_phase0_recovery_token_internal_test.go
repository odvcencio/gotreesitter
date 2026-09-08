//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoveryAcceptancePreservesEventOrder(t *testing.T) {
	s := newLineageSelectionScheduler(t, true)
	seed, err := s.compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := s.compact.ErrorRegionLeaf(4, 6, 16, false)
	if err != nil {
		t.Fatal(err)
	}
	head, err := s.compact.ErrorRegionResume(seed, 3, 6, 16, []core.SubtreeID{leaf})
	if err != nil {
		t.Fatal(err)
	}
	s.headers[1].head = head
	s.headers[0].creationSeq, s.headers[1].creationSeq = 1, 2
	s.recoveryTurns.active = true
	s.recoveryIsolation = true
	s.headers[1].accepted = true
	if err := s.recordRecoveryAcceptance(1); err != nil {
		t.Fatal(err)
	}
	s.work.Accepts++
	snapshot := captureDiagnosticParserCoreS5Scheduler(s)
	s.headers[0].accepted = true
	if err := s.recordRecoveryAcceptance(0); err != nil {
		t.Fatal(err)
	}
	s.work.Accepts++
	if err := s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error { return s.canonicalizeOwned(owner) }); err != nil {
		t.Fatal(err)
	}
	winner, resolved, err := s.selectCompetingRecoveryLineageIndices([]int{0, 1})
	if err != nil || !resolved || winner != 0 {
		t.Fatalf("equal-cost acceptance winner=%d resolved=%v err=%v", winner, resolved, err)
	}
	snapshot.restore(s)
	if s.headers[0].accepted || s.headers[0].versionState != nil && s.headers[0].versionState.acceptanceSeq != 0 || s.work.Accepts != 1 {
		t.Fatal("rollback retained the later acceptance")
	}
	s.headers[0].accepted = true
	s.work.Accepts = ^uint64(0)
	before := s.headers[0].versionState
	if err := s.recordRecoveryAcceptance(0); err == nil || s.headers[0].versionState != before {
		t.Fatal("overflow published an acceptance sequence")
	}
	s.headers[0].clearRecoveryLineage()
	if _, resolved, err := s.selectCompetingRecoveryLineageIndices([]int{0, 1}); err != nil || resolved {
		t.Fatalf("ordinary ambiguity consulted recovery acceptance order: resolved=%v err=%v", resolved, err)
	}
}

func TestRecoveryAcceptanceSequenceSurvivesLexerStateSharing(t *testing.T) {
	for _, equal := range []bool{false, true} {
		t.Run(fmt.Sprint(equal), func(t *testing.T) {
			compact, err := core.New(recoveryLineageForkTable{}, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			language := &Language{Name: "recovery-order"}
			before := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 1)
			after := newDiagnosticParserCoreOwnedLexerSnapshot(t, compact, language, 2)
			sequence := uint64(2)
			if equal {
				sequence = 1
			}
			s := &diagnosticParserCoreGenericScheduler{
				compact: compact, tokenSource: &dfaTokenSource{language: language},
				headers: []diagnosticParserCoreHeader{
					{versionState: &diagnosticParserCoreVersionState{relexSnapshot: before, acceptanceSeq: 1}},
					{versionState: &diagnosticParserCoreVersionState{relexSnapshot: before, acceptanceSeq: sequence}},
				},
			}
			if err := s.installEquivalentVersionLexerState(&s.headers[1], before, 0, nil); err != nil {
				t.Fatal(err)
			}
			if s.headers[1].versionState.acceptanceSeq != sequence || (s.headers[0].versionState == s.headers[1].versionState) != equal {
				t.Fatal("state sharing changed acceptance order or lost equal-state sharing")
			}
			saved := captureDiagnosticParserCoreS5Scheduler(s)
			oldState := s.headers[1].versionState
			if err := s.installEquivalentVersionLexerState(&s.headers[1], after, 0, nil); err != nil {
				t.Fatal(err)
			}
			if s.headers[1].versionState.acceptanceSeq != sequence || oldState.relexSnapshot != before {
				t.Fatal("fresh lexer publication lost order or mutated shared state")
			}
			s.headers[1].publishVersionState(nil, nil, 0, 0, 0, 0, false)
			if s.headers[1].versionState == nil || s.headers[1].versionState.acceptanceSeq != sequence {
				t.Fatal("clearing lexical state erased acceptance order")
			}
			saved.restore(s)
			if s.headers[1].versionState != oldState || s.headers[0].versionState.acceptanceSeq != 1 || s.headers[1].versionState.acceptanceSeq != sequence {
				t.Fatal("rollback did not restore acceptance order")
			}
		})
	}
}

func TestRecoveryTokenSixVersionTransition(t *testing.T) {
	for _, count := range []int{5, 6} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			s.options.MaxDispatches = 100
			s.options.materializationSource = []byte("a++")
			seed, err := s.compact.Seed(3, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.headers[0].head = seed
			var absorber diagnosticParserCoreHeader
			err = s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
				var err error
				absorber, err = s.s5AppendAndMergeAbsorberOwned(owner, s.headers, 0, 0, &diagnosticParserCoreS5Work{})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			s.headers[0] = absorber
			for len(s.headers) < count {
				head, err := s.compact.Seed(core.StateID(30+len(s.headers)), 1)
				if err != nil {
					t.Fatal(err)
				}
				paused := diagnosticParserCoreHeader{head: head, paused: true}
				paused.creationSeq = uint64(len(s.headers) + 30)
				s.headers = append(s.headers, paused)
			}
			region := absorber.recoveryRegion()
			s.token = Token{Symbol: 4, StartByte: 2, EndByte: 3}
			if err := s.advanceRecoveryToken(0); err != nil {
				t.Fatal(err)
			}
			if len(s.headers) != count+1 || s.work.StackSummaryRecoveryForks != 1 {
				t.Fatalf("headers=%d forks=%d", len(s.headers), s.work.StackSummaryRecoveryForks)
			}
			if s.headers[count].recoveryRegion() != nil || s.headers[count].shifted {
				t.Fatal("recovered version consumed the lookahead")
			}
			if count == 6 {
				if !s.headers[0].paused || s.recoveryTurns.halted[0] != 1 || s.headers[0].recoveryRegion() != region {
					t.Fatal("seventh version did not halt the unmodified absorber")
				}
			} else if s.headers[0].paused || s.recoveryTurns.halted[0] != 0 || s.headers[0].recoveryRegion().endByte != 3 || len(region.children) != 1 {
				t.Fatal("six-version frontier did not retain a separate growing absorber")
			}
		})
	}
}

func TestRecoveryZeroWidthTokenResumesBeforeAbsorption(t *testing.T) {
	for _, scannerChanged := range []bool{false, true} {
		t.Run(fmt.Sprint(scannerChanged), func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			s.options.MaxDispatches = 100
			s.options.materializationSource = []byte("a++")
			seed, err := s.compact.Seed(3, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.headers[0].head = seed
			var absorber diagnosticParserCoreHeader
			if err := s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
				var err error
				absorber, err = s.s5AppendAndMergeAbsorberOwned(owner, s.headers, 0, 0, &diagnosticParserCoreS5Work{})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			s.headers[0] = absorber
			s.token = Token{Symbol: 4, StartByte: 2, EndByte: 2, ExternalScannerToken: true}
			if scannerChanged {
				s.checkpointID, err = s.compact.InternCheckpoint([]byte("after scanner transition"))
				if err != nil {
					t.Fatal(err)
				}
			}
			before := captureDiagnosticParserCoreS5Scheduler(s)
			stats, err := s.compact.Stats(absorber.head)
			if err != nil {
				t.Fatal(err)
			}
			err = s.advanceRecoveryToken(0)
			if !scannerChanged {
				afterStats, statsErr := s.compact.Stats(absorber.head)
				if err == nil || !reflect.DeepEqual(s.headers, before.value.headers) || s.work != before.value.work || statsErr != nil || afterStats != stats {
					t.Fatalf("zero-width absorption did not roll back: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(s.headers) != 2 || s.work.StackSummaryRecoveryForks != 1 || !s.headers[0].paused || s.recoveryTurns.halted[0] != 1 {
				t.Fatalf("scanner transition did not resume and halt: headers=%d forks=%d", len(s.headers), s.work.StackSummaryRecoveryForks)
			}
			if s.headers[0].recoveryRegion() != absorber.recoveryRegion() || s.headers[1].recoveryRegion() != nil || s.headers[1].shifted {
				t.Fatal("zero-width resume consumed the lookahead or changed the error region")
			}
		})
	}
}

func TestRecoveryOwnedEpisodeAdmissionPreservesSharedGuards(t *testing.T) {
	for _, guard := range []string{"completed_owned_episode", "shared_resume", "no_action_drop"} {
		t.Run(guard, func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			s.options.MaxDispatches = 100
			s.options.allowCompactRecoveryVersionTurns = true
			s.s3RegionOpened = true
			switch guard {
			case "shared_resume":
				s.s3ResumeCount = 1
			case "no_action_drop":
				s.work.NoActionDrops = 1
			}
			handled, err := s.s5TryRecoveryTransaction(0, false)
			if guard == "completed_owned_episode" {
				if err != nil || !handled || !s.recoveryTurns.active {
					t.Fatalf("second owned episode handled=%v active=%v err=%v", handled, s.recoveryTurns.active, err)
				}
			} else if err == nil || handled || s.recoveryTurns.active {
				t.Fatalf("shared guard admitted recovery: handled=%v active=%v err=%v", handled, s.recoveryTurns.active, err)
			}
		})
	}
}

func TestRecoveryAcceptanceSequencePartitionsMergeIdentity(t *testing.T) {
	s := &diagnosticParserCoreGenericScheduler{}
	s.headers = []diagnosticParserCoreHeader{
		{accepted: true, versionState: &diagnosticParserCoreVersionState{acceptanceSeq: 1}},
		{accepted: true, versionState: &diagnosticParserCoreVersionState{acceptanceSeq: 2}},
	}
	if s.versionLexerStateEqual(s.headers[0].versionState, s.headers[1].versionState) {
		t.Fatal("different acceptance orders compared equal")
	}
	if s.condenseCandidateMergeIdentity(0) == s.condenseCandidateMergeIdentity(1) {
		t.Fatal("different acceptance orders share a merge identity")
	}
	s.headers[1].versionState.acceptanceSeq = 1
	if !s.versionLexerStateEqual(s.headers[0].versionState, s.headers[1].versionState) {
		t.Fatal("equal acceptance orders did not compare equal")
	}
	if s.condenseCandidateMergeIdentity(0) != s.condenseCandidateMergeIdentity(1) {
		t.Fatal("equal acceptance orders have different merge identities")
	}
}
