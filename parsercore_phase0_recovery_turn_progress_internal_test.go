//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"reflect"
	"testing"
)

func TestRecoveryTurnPreacceptedEOFActivation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s := newRecoveryLineageForkScheduler(t, true)
		s.options.allowCompactRecoveryVersionTurns = true
		s.recoveryTurns.active, s.recoveryIsolation = true, true
		s.token = Token{StartByte: 2, EndByte: 2}
		s.headers[0].accepted = true
		s.headers[0].markRecoveryLineage()
		if err := s.recordRecoveryAcceptance(0); err != nil {
			t.Fatal(err)
		}
		if fail {
			s.token.Symbol = 4
		}
		before := captureDiagnosticParserCoreS5Scheduler(s)
		scannerBefore := s.tokenSource.snapshotRelexState()
		if err := s.activateRecoveryVersionTurns(); fail {
			if err == nil || !reflect.DeepEqual(s.headers, before.value.headers) ||
				len(s.versionLexerRequests) != 0 || s.versionLexerOwnershipActive {
				t.Fatalf("failed activation changed publication: %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			request := s.versionLexerRequestForHeader(0)
			if request == nil || request.token != s.token ||
				request.before.dfa.lexerPos != 1 || request.after.dfa.lexerPos != 2 ||
				request.beforeID != s.checkpointBeforeID || request.afterID != s.checkpointID ||
				s.headers[0].versionState.acceptanceSeq != 1 {
				t.Fatalf("activation changed the finished EOF election: %+v", request)
			}
		}
		if !reflect.DeepEqual(scannerBefore, s.tokenSource.snapshotRelexState()) {
			t.Fatal("activation relexed the finished EOF token")
		}
	}
}

func TestRecoveryTurnSameByteReductionChain(t *testing.T) {
	for _, terminal := range []string{"shift", "pause", "accept"} {
		t.Run(terminal, func(t *testing.T) {
			s := &diagnosticParserCoreGenericScheduler{
				options:       DiagnosticParserCorePrefixOptions{MaxDispatches: 3},
				recoveryTurns: diagnosticParserCoreRecoveryTurns{index: 2, lastByte: 13},
			}
			h := diagnosticParserCoreHeader{}
			for reduction := 0; reduction < 3; reduction++ {
				if err := s.reserveDispatches(1); err != nil {
					t.Fatal(err)
				}
				s.completeRecoveryVersionOperation(2, h, 13)
				if s.recoveryTurns.index != 2 {
					t.Fatal("intermediate reduction ended the version visit")
				}
			}
			if err := s.reserveDispatches(1); err == nil {
				t.Fatal("cyclic reductions escaped the dispatch budget")
			}
			switch terminal {
			case "shift":
				h.shifted = true
			case "pause":
				h.paused = true
			case "accept":
				h.accepted = true
			}
			s.completeRecoveryVersionOperation(2, h, 13)
			if s.recoveryTurns.index != 3 {
				t.Fatal("completed operation did not end the visit")
			}
		})
	}
}

func TestRecoveryTurnCondenseRetiredProvenance(t *testing.T) {
	for _, mode := range []string{"retired", "snapshot", "request", "open", "live_group", "ordinary"} {
		t.Run(mode, func(t *testing.T) {
			s := &diagnosticParserCoreGenericScheduler{recoveryTurns: diagnosticParserCoreRecoveryTurns{active: true}}
			snapshot := &diagnosticParserCoreVersionLexerSnapshot{}
			left := diagnosticParserCoreHeader{versionState: &diagnosticParserCoreVersionState{relexSnapshot: snapshot, missingGroup: 15, recoveryNodeBaseline: 8, recoveryNodeBaselineSet: true}}
			right := diagnosticParserCoreHeader{versionState: &diagnosticParserCoreVersionState{relexSnapshot: snapshot, recoveryNodeBaseline: 10, recoveryNodeBaselineSet: true}}
			switch mode {
			case "snapshot":
				right.versionState.relexSnapshot = &diagnosticParserCoreVersionLexerSnapshot{}
			case "request":
				right.versionState.lexerRequest = 1
			case "open":
				right.versionState.s3Region = &diagnosticParserCoreS3Region{}
			case "live_group":
				right.versionState.recoveryGroup = 1
			case "ordinary":
				s.recoveryTurns.active = false
			}
			if got := s.recoveryCondenseLexerStateEqual(left, right); got != (mode == "retired") {
				t.Fatalf("equivalent=%v", got)
			}
			if left.versionState.recoveryNodeBaseline != 8 || left.versionState.missingGroup != 15 {
				t.Fatal("comparison mutated incumbent")
			}
		})
	}
}
