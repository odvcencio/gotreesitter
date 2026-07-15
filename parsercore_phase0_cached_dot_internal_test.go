//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// InstallDiagnosticParserCoreCachedDotFaultForTest is compiled only into the
// tagged test archive. The production diagnostic seam remains unexported and
// no compact-core type crosses this bridge.
func InstallDiagnosticParserCoreCachedDotFaultForTest(afterElection bool) (error, func() error, func()) {
	faultPoint := diagnosticParserCoreCachedDotFaultAfterCohort
	if afterElection {
		faultPoint = diagnosticParserCoreCachedDotFaultAfterElection
	}
	fault := errors.New("injected cached-dot failure")
	var beforeToken dfaRelexSnapshot
	var beforeState StateID
	var beforeGLR []StateID
	var sawFault, sawRollback bool
	var validationErr error
	diagnosticParserCoreCachedDotFaultHook = func(
		point diagnosticParserCoreCachedDotFaultPoint,
		compact *core.Core,
		callerHeaders []diagnosticParserCoreHeader,
		trialHeaders []diagnosticParserCoreHeader,
		tokenSource *dfaTokenSource,
	) error {
		if err := authenticateCachedDotFaultCaller(compact, callerHeaders); err != nil {
			validationErr = err
			return err
		}
		switch point {
		case diagnosticParserCoreCachedDotFaultAfterCohort:
			if err := authenticateCachedDotFaultTrial(compact, trialHeaders); err != nil {
				validationErr = err
				return err
			}
			beforeToken = tokenSource.snapshotRelexState()
			beforeState = tokenSource.state
			beforeGLR = append([]StateID(nil), tokenSource.glrStates...)
			if faultPoint == point {
				sawFault = true
				return fault
			}
		case diagnosticParserCoreCachedDotFaultAfterElection:
			if tokenSource.lexer.pos != 747 || tokenSource.state != 164 || !reflect.DeepEqual(tokenSource.glrStates, []StateID{164, 194}) {
				validationErr = fmt.Errorf("edits election source state=(pos=%d state=%d glr=%v)", tokenSource.lexer.pos, tokenSource.state, tokenSource.glrStates)
				return validationErr
			}
			if faultPoint == point {
				sawFault = true
				return fault
			}
		case diagnosticParserCoreCachedDotFaultAfterRollback:
			sawRollback = true
			if tokenSource.state != beforeState || !reflect.DeepEqual(tokenSource.glrStates, beforeGLR) || !reflect.DeepEqual(tokenSource.snapshotRelexState(), beforeToken) {
				validationErr = fmt.Errorf("token source rollback drifted: state=%d/%d glr=%v/%v", tokenSource.state, beforeState, tokenSource.glrStates, beforeGLR)
				return validationErr
			}
		}
		return nil
	}
	check := func() error {
		if validationErr != nil {
			return validationErr
		}
		if !sawFault || !sawRollback {
			return fmt.Errorf("cached-dot fault lifecycle saw_fault=%t saw_rollback=%t", sawFault, sawRollback)
		}
		return nil
	}
	reset := func() { diagnosticParserCoreCachedDotFaultHook = nil }
	return fault, check, reset
}

func authenticateCachedDotFaultCaller(compact *core.Core, headers []diagnosticParserCoreHeader) error {
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return err
	}
	want := []DiagnosticParserCoreHeaderReceipt{
		{CreationSeq: 1, State: 520, ByteOffset: 741, ExactPaths: 1, Checkpoint: sha256.Sum256(nil)},
		{CreationSeq: 5, State: 407, ByteOffset: 741, ExactPaths: 1, Checkpoint: sha256.Sum256(nil)},
		{CreationSeq: 4, State: 194, ByteOffset: 742, Shifted: true, ExactPaths: 2, Checkpoint: sha256.Sum256(nil)},
	}
	if !reflect.DeepEqual(receipts, want) {
		return fmt.Errorf("caller-owned dot headers drifted: got=%+v want=%+v", receipts, want)
	}
	return nil
}

func authenticateCachedDotFaultTrial(compact *core.Core, headers []diagnosticParserCoreHeader) error {
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return err
	}
	want := []DiagnosticParserCoreHeaderReceipt{
		{CreationSeq: 1, State: 164, ByteOffset: 742, Shifted: true, ExactPaths: 2, Checkpoint: sha256.Sum256(nil)},
		{CreationSeq: 4, State: 194, ByteOffset: 742, Shifted: true, ExactPaths: 2, Checkpoint: sha256.Sum256(nil)},
	}
	if !reflect.DeepEqual(receipts, want) {
		return fmt.Errorf("trial cached-dot headers drifted: got=%+v want=%+v", receipts, want)
	}
	return nil
}
