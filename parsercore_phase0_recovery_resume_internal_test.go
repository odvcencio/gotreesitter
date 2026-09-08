//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoveryPausedResumeTieAndEligibility(t *testing.T) {
	s := newRecoveryLineageForkScheduler(t, true)
	first := s.headers[0]
	first.paused = true
	second := first
	second.creationSeq++
	s.headers = []diagnosticParserCoreHeader{first, second}
	if got := s.recoveryPausedResumeIndex(); got != 0 {
		t.Fatalf("equal statuses selected %d", got)
	}
	s.headers[0].accepted = true
	if got := s.recoveryPausedResumeIndex(); got != 1 {
		t.Fatalf("finished pool blocked selection: %d", got)
	}
	s.headers[0].accepted = false
	s.headers[0].paused = false
	if got := s.recoveryPausedResumeIndex(); got != -1 {
		t.Fatalf("paused loser beat runnable incumbent: %d", got)
	}
	s.headers[0].paused = true
	s.work.Accepts = 6
	if got := s.recoveryPausedResumeIndex(); got != -1 {
		t.Fatalf("accepted version limit ignored: %d", got)
	}
	s.headers = nil
	s.work.Accepts = 0
	if got := s.recoveryPausedResumeIndex(); got != -1 {
		t.Fatalf("empty frontier selected %d", got)
	}
}

func TestRecoveryPausedResumeOwnedLexerAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s := newRecoveryLineageForkScheduler(t, true)
		s.options.MaxDispatches = 100
		s.options.materializationSource = []byte("a?")
		s.options.allowCompactRecoveryVersionTurns = true
		s.recoveryTurns.active = true
		s.recoveryIsolation = true
		s.versionLexerOwnershipActive = true
		err := s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
			before, err := s.newVersionLexerSnapshot(owner, dfaRelexSnapshot{lexerPos: 1, lexerCol: 1}, 0, 0)
			if err != nil {
				return err
			}
			after, err := s.newVersionLexerSnapshot(owner, dfaRelexSnapshot{lexerPos: 2, lexerCol: 2}, 0, 0)
			if err != nil {
				return err
			}
			if err = s.installEquivalentVersionLexerState(&s.headers[0], before, 1, nil); err != nil {
				return err
			}
			s.versionLexerRequests = []diagnosticParserCoreVersionLexerRequest{{valid: true, headerCreationSeq: s.headers[0].creationSeq, state: 1, token: s.token, before: before, after: after, beforeCheckpoint: before.afterCheckpointInfo, afterCheckpoint: after.afterCheckpointInfo}}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		s.headers[0].paused = true
		s.headers[0].markRecoveryLineage()
		original := s.headers[0]
		loser := original
		loser.creationSeq++
		s.headers = append(s.headers, loser)
		if fail {
			s.options.allowCompactMissingTokenInsertion = false
		}
		before := captureDiagnosticParserCoreS5Scheduler(s)
		err = s.resumePausedRecovery()
		if fail {
			if err == nil || !reflect.DeepEqual(s.headers, before.value.headers) || s.nextSeq != before.value.nextSeq {
				t.Fatalf("failed resume changed frontier: err=%v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(s.headers) < 2 {
			t.Fatalf("missing competition: %+v", s.headers)
		}
		for _, header := range s.headers {
			if header.creationSeq == loser.creationSeq || header.paused {
				t.Fatal("paused loser survived")
			}
			if !header.isRecoveryLineage() || header.versionLexerSnapshot() == nil {
				t.Fatal("resume lost lineage or scanner snapshot")
			}
			want := 1
			if header.recoveryRegion() != nil {
				want = 2
			}
			if got := header.versionLexerSnapshot().dfa.lexerPos; got != want {
				t.Fatalf("version cursor=%d want=%d", got, want)
			}
		}
		if s.tokenSource.lexer.pos != 2 {
			t.Fatal("resume changed shared lexer cursor")
		}
	}
}

func TestRecoveryPausedOpenAbsorberWinnerLoserAndRollback(t *testing.T) {
	for _, mode := range []string{"winner", "loser", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			s := newRecoveryLineageForkScheduler(t, true)
			s.options.MaxDispatches = 100
			s.options.materializationSource = []byte("a??")
			s.tokenSource.lexer.source = s.options.materializationSource
			s.tokenSource.lexer.pos = 3
			s.token = Token{Symbol: errorSymbol, StartByte: 2, EndByte: 3}
			s.options.allowCompactRecoveryVersionTurns = true
			s.recoveryTurns.active = true
			s.recoveryIsolation = true
			s.versionLexerOwnershipActive = true
			clean := s.headers[0]
			old, err := s.compact.ErrorRegionLeaf(1, 1, 2, false)
			if err != nil {
				t.Fatal(err)
			}
			err = s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
				marker, err := s.s5MergeRecoveryMarkerOwned(owner, s.headers, &diagnosticParserCoreS5Work{})
				if err != nil {
					return err
				}
				marker.openRecoveryRegion(&diagnosticParserCoreS3Region{startByte: 1, endByte: 2, children: []core.SubtreeID{old}})
				marker.markRecoveryLineage()
				s.headers[0] = marker
				before, err := s.newVersionLexerSnapshot(owner, dfaRelexSnapshot{lexerPos: 2, lexerCol: 2}, 0, 0)
				if err != nil {
					return err
				}
				after, err := s.newVersionLexerSnapshot(owner, dfaRelexSnapshot{lexerPos: 3, lexerCol: 3}, 0, 0)
				if err != nil {
					return err
				}
				if err = s.installEquivalentVersionLexerState(&s.headers[0], before, 1, nil); err != nil {
					return err
				}
				if err = s.installEquivalentVersionLexerState(&clean, before, 1, nil); err != nil {
					return err
				}
				s.versionLexerRequests = []diagnosticParserCoreVersionLexerRequest{{valid: true, state: 0, token: s.token, before: before, after: after, beforeCheckpoint: before.afterCheckpointInfo, afterCheckpoint: after.afterCheckpointInfo}}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.pauseRecoveryVersion(0); err != nil {
				t.Fatal(err)
			}
			if mode == "loser" {
				clean.creationSeq = 20
				clean.markRecoveryLineage()
				clean.paused = true
				s.headers = append([]diagnosticParserCoreHeader{clean}, s.headers...)
			}
			if mode == "rollback" {
				s.options.MaxDispatches = 1
				s.dispatches = 1
			}
			before := captureDiagnosticParserCoreS5Scheduler(s)
			err = s.resumePausedRecovery()
			if mode == "rollback" {
				if err == nil || !reflect.DeepEqual(s.headers, before.value.headers) {
					t.Fatalf("rollback retained recovery: err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(s.headers) != 1 || s.headers[0].paused {
				t.Fatalf("frontier=%+v", s.headers)
			}
			paths, err := s.compact.Derivations(s.headers[0].head)
			if err != nil || len(paths) != 1 {
				t.Fatalf("paths=%v err=%v", paths, err)
			}
			if mode == "winner" {
				if len(paths[0].Payloads) != 1 {
					t.Fatalf("prior repeat lost: %+v", paths[0])
				}
				v, err := s.compact.MaterializationView(paths[0].Payloads[0])
				if err != nil || v.Symbol != core.RecoveryErrorRepeatSymbol || !reflect.DeepEqual(v.Children, []core.SubtreeID{old}) {
					t.Fatalf("prior repeat=%+v err=%v", v, err)
				}
				cost, err := s.compact.RecoveryStoredErrorCost(s.headers[0].head)
				if err != nil || cost != 601 {
					t.Fatalf("prior cost=%d err=%v", cost, err)
				}
			} else if len(paths[0].Payloads) != 0 {
				t.Fatal("loser repeat entered winner")
			}
			if got := s.headers[0].versionLexerSnapshot().dfa.lexerPos; got != 3 {
				t.Fatalf("absorber cursor=%d", got)
			}
		})
	}
}
