//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
	"math"
	"unsafe"
)

// nativeSelectedOutputRunner is an experimental read-only output route. It
// keeps the accepted result compact through traversal and query execution.
// It does not claim the public Node or incremental-edit contract.
type nativeSelectedOutputRunner struct {
	runner *parserCoreFreshFullRunner
	replay core.MaterializationPostorderScratch
	output core.NativeSelectedScratch
}

func (n *nativeSelectedOutputRunner) retainedBytes() uint64 {
	if n == nil {
		return 0
	}
	a, b := n.output.RetainedBytes(), n.replay.RetainedBytes()
	if a > math.MaxUint64-uint64(unsafe.Sizeof(*n)) || b > math.MaxUint64-a-uint64(unsafe.Sizeof(*n)) {
		return math.MaxUint64
	}
	return a + b + uint64(unsafe.Sizeof(*n))
}

func (n *nativeSelectedOutputRunner) parse(source []byte) (store *core.SelectedStore, err error) {
	r := n.runner
	if r == nil || r.parser == nil {
		return nil, errors.New("native compact output requires a runner")
	}
	r.options.nativeSelectedOutput = n
	defer func() { r.options.nativeSelectedOutput = nil; r.scheduler.options.nativeSelectedOutput = nil }()
	leave := r.parser.enterParseBudget()
	defer leave()
	defer func() {
		if err != nil {
			_ = r.compact.Reset()
		}
	}()
	s, tokens, err := r.executeSchedulerOpen(source, r.compact, true)
	if tokens != nil {
		defer tokens.Close()
	}
	if err != nil {
		return nil, err
	}
	if r.recoveryVersionTurnPublicationRequired(s) || s.acceptedRootFinalization == diagnosticParserCoreFinalizeRecoverEOF || s.eofRecoveryAdmission.active || r.lang.CompactLexerSkippedPrefixTilingCertified {
		return nil, errors.New("native compact output: recovery or prefix provenance requires public adapter")
	}
	poll := func() error {
		if reason := r.parser.activeParseStopReason(); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		extra := s.compact.SelectedBuildAuxiliaryBytes()
		if reason := s.stopControlMemoryBudgetReasonWithAdditionalBytes(extra); resultMaterializationShouldStop(reason) {
			return diagnosticParserCoreStopControlTripped(reason)
		}
		return nil
	}
	replay := core.NativeProjectionReplay{
		CheckpointsRequired: languageUsesExternalScannerCheckpoints(r.lang),
		Scratch:             &n.replay, RootPre: core.StateID(r.parser.replayRootPreGotoState()), NativeTransition: r.tables,
		Tiling: &core.NativeProjectionTiling{RootSymbol: core.Symbol(r.parser.rootSymbol), HasRootSymbol: r.parser.hasRootSymbol, GapIsTolerated: diagnosticParserCoreGapIsTolerated},
	}
	store, err = s.compact.BuildNativeSelectedOutput(s.acceptedPayloads, source, replay, &n.output, poll)
	if err != nil {
		return nil, err
	}
	return requireParserCoreSelectedStoreCompleteness(store, len(source))
}
