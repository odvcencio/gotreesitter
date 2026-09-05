//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"strings"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreFreshFullRunner owns reusable state for compact UTF-8 parsing.
// Admission configures the language, scanner, and supported recovery paths.
// Incremental callers can supply an authenticated subtree reuse session.
// Each call starts a fresh core generation, independent of Parser.parseInternal.
// Callers must process declines before publishing a tree.
// The runner is not safe for concurrent use. Successful results belong to the caller.
type parserCoreFreshFullRunner struct {
	// legacyParseRuns verifies legacy entries without growing the Parser layout.
	legacyParseRuns                   uint64
	lang                              *Language
	parser                            *Parser
	tables                            *parserCoreRootTables
	compact                           *core.Core
	options                           DiagnosticParserCorePrefixOptions
	replayParseStates                 bool
	allowConvergedReductionSplitDrops bool
	// recoveryPlainFirst preserves ordinary lexing for the first attempt. A
	// fail-closed decline then retries with C error-mode lexing enabled.
	recoveryPlainFirst bool
	// certificateAdmissionEnabled is armed only by the private D3 test seam.
	// It is copied into options for each cached parse after Core.Reset.
	certificateAdmissionEnabled bool
	// frontierRecordingEnabled is armed only by the private D6a test seam.
	// It never enables historical certificate authentication or admission.
	frontierRecordingEnabled bool
	// frontierVerificationEnabled is armed only by the private D6b test seam.
	// It never enables the production admission route.
	frontierVerificationEnabled bool
	frontierVerificationToken   *dropCohortActivationToken
	certificateAdmissionToken   *dropCohortActivationToken
	scannerScratch              []byte
	// scratch retains the reusable per-parse materialization buffers. The runner
	// is per-Parser and single-goroutine, so reusing these buffers across parses
	// mirrors production's parser-held arena reuse and keeps the warm steady
	// state from re-allocating the public-tree scratch on every parse.
	scratch   parserCoreRunnerScratch
	scheduler diagnosticParserCoreGenericScheduler
	// frontierPublishedObserver is populated only by the focused D6 test seam.
	// The production route leaves it nil and keeps the observer allocation-free.
	frontierPublishedObserver func(*diagnosticParserCoreGenericScheduler, core.SchedulerTransactionToken, []int) error
}

func newParserCoreFreshFullRunner(scanner ExternalScanner, options DiagnosticParserCorePrefixOptions) (*parserCoreFreshFullRunner, error) {
	// S3 supplies the base recovery mechanism. A dedicated recover_eof route
	// has its own artifact gate and does not imply S3, S4, or S5.
	if (options.Recovery && !options.allowCompactStrategy2ErrorRegion && !options.allowCompactRecoverEOF) ||
		options.Retry || options.Incremental || options.IncludedRanges || options.GenericStopAtClosedByte != nil {
		return nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "fresh-full runner declines recovery/retry/incremental/included-range/closed-prefix routes",
		}
	}
	if options.ReceiptMode != DiagnosticParserCoreReceiptSummary {
		return nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "fresh-full runner requires bounded summary receipts",
		}
	}
	if options.MaxDispatches == 0 {
		options.MaxDispatches = 100000
	}
	if options.MaxTokens == 0 {
		options.MaxTokens = 100000
	}
	options.freshSchedulerSession = true
	options.allowConvergedSplitDropArtifact = true

	lang, err := authenticatedParserCoreGoLanguage(scanner)
	if err != nil {
		return nil, err
	}
	parser := NewParser(lang)
	options.noLookaheadRootSymbol = parser.rootSymbol
	options.hasNoLookaheadRootSymbol = parser.hasRootSymbol
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		return nil, err
	}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return nil, err
	}
	configureParserCoreScannerProvenance(compact, lang)
	return &parserCoreFreshFullRunner{
		lang: lang, parser: parser, tables: tables, compact: compact, options: options,
		allowConvergedReductionSplitDrops: true,
	}, nil
}

func (r *parserCoreFreshFullRunner) executeSchedulerOpen(source []byte, compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	return r.executeSchedulerOpenWithObserver(source, compact, reset, diagnosticParserCoreSeedObserver{})
}

func (r *parserCoreFreshFullRunner) executeSchedulerOpenWithObserver(
	source []byte,
	compact *core.Core,
	reset bool,
	observer diagnosticParserCoreSeedObserver,
) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	forceErrorRuns := r != nil && r.options.Recovery && r.options.allowCompactStrategy2ErrorRegion
	return r.executeSchedulerOpenWithObserverAndErrorRuns(source, compact, reset, observer, forceErrorRuns)
}

func (r *parserCoreFreshFullRunner) executeSchedulerOpenWithObserverAndErrorRuns(
	source []byte,
	compact *core.Core,
	reset bool,
	observer diagnosticParserCoreSeedObserver,
	forceErrorRuns bool,
) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	if r == nil || r.parser == nil || r.lang == nil || r.tables == nil || compact == nil {
		return nil, nil, errors.New("parser-core fresh-full runner is incomplete")
	}
	if observer.frontierPublished == nil {
		observer.frontierPublished = r.frontierPublishedObserver
	}
	if reset {
		if err := compact.Reset(); err != nil {
			return nil, nil, err
		}
	}
	// Core.Reset clears the session authentication bit. Refresh the scheduler
	// option on every cached parse so the owner callback can re-arm it only for
	// this fresh session. The default path remains false and allocation-free.
	r.options.recordDropCohortCertificates = r.certificateAdmissionEnabled
	r.options.recordDropCohortFrontiers = r.frontierRecordingEnabled
	r.options.verifyDropCohortFrontiers = r.frontierVerificationEnabled
	// B3 stage S3: force the shared token source's error-run lexing on when
	// native compact recovery is admitted, so a genuinely unlexable byte run
	// surfaces as its own errorSymbol token (parser_api.go's
	// acquireParserDFATokenSourceWithErrorRuns doc comment) instead of the
	// plain lexer silently skipping it -- the silent-skip shape a true no-
	// table-action dispatch point can never observe, verified against every
	// committed html_erroneous_end_tag witness.
	tokenSource := r.parser.acquireParserDFATokenSourceWithErrorRuns(source, forceErrorRuns)
	if tokenSource == nil {
		return nil, nil, errors.New("parser-core fresh-full runner: production DFA unavailable")
	}
	// Recomputed fresh for every call: the production soft budget is a
	// function of source length (parseMemoryBudgetForParser), and this
	// runner is reused across parses of different sizes. Only armed when the
	// caller bound a stop-control Parser (admission_switch_candidate.go); the
	// diagnostic and benchmark runners leave stopControlParser nil, so this
	// stays zero and the scheduler's memory-budget poll is a no-op for them.
	// The hard ceiling is armed independently of the soft budget (it stays
	// on even when a caller zeroes GOT_PARSE_MEMORY_BUDGET_MB), mirroring
	// production's own soft/hard independence.
	if r.options.stopControlParser != nil {
		r.options.stopControlMemoryBudgetBytes = parseMemoryBudgetForParser(r.options.stopControlParser, len(source))
		r.options.stopControlHardCeilingBytes = parseMemoryHardCeilingBytes()
	}
	// See DiagnosticParserCorePrefixOptions.materializationParser: this runner
	// is reused across parses, so these fields are refreshed on every call
	// rather than set once at construction. materializationForceReplayParseStates
	// forwards r.replayParseStates itself (not a hardcoded value): it is true
	// only for the admission-candidate route (newAdmissionCandidateRunner),
	// so the gate's trial materializations carry the same ParseState/
	// PreGotoState presence the caller's own post-acceptance materialization
	// (materializeSelection, below) will publish.
	r.options.materializationParser = r.parser
	r.options.materializationSource = source
	r.options.materializationForceReplayParseStates = r.replayParseStates
	r.options.materializationContextSet = true
	// The gate (completeAcceptance, run synchronously inside the call below)
	// is the only reader of these fields, and it runs to completion before
	// this function returns. Clear both this runner's own copy and the
	// scheduler's copy (scheduler.options is a byte-for-byte struct copy
	// taken inside the call, so it holds its own slice header aliasing the
	// same backing array) on every exit path, so the cached, per-Parser
	// runner does not pin the caller's source buffer between parses.
	defer func() {
		r.options.materializationParser = nil
		r.options.materializationSource = nil
		r.options.materializationContextSet = false
		r.scheduler.options.materializationParser = nil
		r.scheduler.options.materializationSource = nil
		r.scheduler.options.materializationContextSet = false
	}()
	scheduler, err := executeDiagnosticParserCoreGenericSchedulerFromSeedInto(
		&r.scheduler, compact, tokenSource, &r.scannerScratch, r.lang.InitialState,
		r.options, observer,
	)
	if err != nil {
		tokenSource.Close()
		return nil, nil, err
	}
	if err := requireParserCoreFreshFullAcceptance(scheduler, source, r.allowConvergedReductionSplitDrops, languageLineContinuationEscapeByte(r.lang)); err != nil {
		tokenSource.Close()
		// The scheduler run itself already committed here (RunFreshSchedulerSession,
		// invoked from inside executeDiagnosticParserCoreGenericSchedulerFromSeedInto,
		// resets the core only on its own internal error), so this acceptance-gate
		// decline -- accepted, but not the strict sole-exact-EOF frontier -- is the
		// one decline path that would otherwise leave the compact core's storage
		// retained on the cached runner until the next parse call resets it lazily.
		// Reset immediately, releasing any oversized retention the accepted run
		// grew: a large declined parse must never leave compact storage retained
		// while the caller's production fallback runs (tranche B9 gate).
		if resetErr := compact.ResetReleasingRetention(); resetErr != nil {
			err = errors.Join(err, fmt.Errorf("parser-core fresh-full runner: reset after acceptance decline: %w", resetErr))
		}
		return nil, nil, err
	}
	if err := scheduler.markCompactEOFRecoverySchedulerReturned(source); err != nil {
		tokenSource.Close()
		if resetErr := compact.ResetReleasingRetention(); resetErr != nil {
			err = errors.Join(err, fmt.Errorf("parser-core fresh-full runner: reset after EOF receipt decline: %w", resetErr))
		}
		return nil, nil, err
	}
	return scheduler, tokenSource, nil
}

func requireParserCoreFreshFullAcceptance(scheduler *diagnosticParserCoreGenericScheduler, source []byte, allowConvergedReductionSplitDrops bool, continuationEscape byte) error {
	if scheduler == nil || scheduler.receipt == nil || scheduler.receipt.Acceptance == nil || scheduler.acceptedHead.Node == 0 {
		// GTS_ADMISSION_CENSUS=1 (admission_census.go) re-surfaces the boundary
		// and detail the scheduler already recorded in scheduler.receipt.Stop
		// when it declined mid-run. Unset (the default), this branch is
		// byte-identical to before that instrumentation existed.
		if admissionCensusEnabled() {
			return admissionCensusStopDecline(scheduler)
		}
		return errors.New("parser-core fresh-full runner did not accept EOF")
	}
	if uint64(len(source)) > uint64(^uint32(0)) {
		return errors.New("parser-core fresh-full runner source exceeds uint32 offsets")
	}
	acceptance := scheduler.receipt.Acceptance
	wantEOF := uint32(len(source))
	header := acceptance.Header.Header
	selectedCertifiedPrimary := scheduler.options.allowPrimaryAcceptDerivation &&
		header.ExactPaths > 1 && !acceptance.HasBranchOrder
	selectedMaterialityCertified := acceptance.MaterialityCertified && header.ExactPaths > 1
	selectedStructuralElectionCertified := acceptance.StructuralElectionCertified && header.ExactPaths > 1
	acceptCountValid := (acceptance.Accepts == 1 && acceptance.Work.Accepts == 1) ||
		(acceptance.Work.RecoveryLineageSelections == 1 &&
			acceptance.Accepts >= 2 && acceptance.Accepts == acceptance.Work.Accepts)
	if acceptance.Token.Symbol != 0 || acceptance.Token.StartByte != wantEOF || acceptance.Token.EndByte != wantEOF ||
		acceptance.Token.Missing || acceptance.Token.NoLookahead || acceptance.Token.ExternalScannerToken ||
		!header.Accepted || header.Paused || header.ExactPaths != 1 &&
		!selectedCertifiedPrimary && !selectedMaterialityCertified && !selectedStructuralElectionCertified ||
		!parserCoreFreshFullAcceptedTailIsClean(source, header.ByteOffset, continuationEscape) || !acceptCountValid {
		// See the comment above: census classification is opt-in and additive.
		if admissionCensusEnabled() {
			return admissionCensusAcceptanceDecline(acceptance, wantEOF)
		}
		return fmt.Errorf("parser-core fresh-full runner acceptance is not sole exact EOF: token=%+v header=%+v accepts=%d/%d",
			acceptance.Token, acceptance.Header.Header, acceptance.Accepts, acceptance.Work.Accepts)
	}
	if !allowConvergedReductionSplitDrops &&
		acceptance.Work.ConvergedReductionSplitDrops != acceptance.Work.ConvergedCoverageDrops {
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreAccept,
			detail: fmt.Sprintf(
				"accepted frontier followed %d converged-path reduction split drops with %d alternative-set coverage proofs",
				acceptance.Work.ConvergedReductionSplitDrops,
				acceptance.Work.ConvergedCoverageDrops,
			),
		}
	}
	return nil
}

// parserCoreFreshFullAcceptedTailIsClean applies the production parser's
// existing accepted-tail proof to compact acceptance. The compact head may
// end before authenticated EOF only when every remaining byte is parser
// padding; a real source byte keeps the route fail-closed. continuationEscape
// is the runner's language-declared line-continuation escape byte (0 when
// the language declares none), threaded down from requireParserCoreFreshFullAcceptance
// so a trailing language-declared continuation is padding here exactly like
// it is in the production parser's own tail check.
func parserCoreFreshFullAcceptedTailIsClean(source []byte, headByte uint32, continuationEscape byte) bool {
	if uint64(len(source)) > uint64(^uint32(0)) || headByte > uint32(len(source)) {
		return false
	}
	return parserTailAllowsCleanAcceptance(source, headByte, uint32(len(source)), nil, continuationEscape)
}

// s3AllowErrorRoot reports whether this runner's current options admit
// native S3 recovery, the sole condition under which a materialized root may
// legitimately carry HasError (finalizeDiagnosticParserCoreAcceptedRootSpan's
// allowErrorRoot parameter). Mirrors s3ErrorRegionAdmitted exactly (same two
// fields); duplicated here because the runner, not the scheduler, owns
// materialization.
func (r *parserCoreFreshFullRunner) s3AllowErrorRoot() bool {
	return r != nil && r.options.Recovery && r.options.allowCompactStrategy2ErrorRegion
}

func (r *parserCoreFreshFullRunner) recoverEOFFinalizationAdmitted(
	scheduler *diagnosticParserCoreGenericScheduler,
) bool {
	return r != nil && scheduler != nil && r.options.Recovery &&
		r.options.allowCompactRecoverEOF &&
		scheduler.acceptedRootFinalization == diagnosticParserCoreFinalizeRecoverEOF
}

func (r *parserCoreFreshFullRunner) compactRecoveryTerminalAliasSymbol(
	scheduler *diagnosticParserCoreGenericScheduler,
) (Symbol, bool) {
	if r == nil || r.lang == nil || scheduler == nil ||
		!scheduler.s3RegionOpened || scheduler.s3ResumeCount != 1 {
		return 0, false
	}
	standaloneRegion := scheduler.s5MissingInsertions == 0 &&
		scheduler.work.RecoveryLineageSelections == 0 &&
		scheduler.work.RecoveryLineageRetirements == 0 && scheduler.work.NoActionDrops == 1
	retiredMissingLineage := scheduler.s5MissingInsertions == 1 &&
		scheduler.work.RecoveryLineageSelections == 0 &&
		scheduler.work.RecoveryLineageRetirements == 1 && scheduler.work.NoActionDrops == 1
	// S5 publishes the original absorb lineage before its missing sibling.
	// collapseToRecoveryWinner records this proof before it clears the loser.
	// A missing-lineage winner must not borrow the absorb lineage's resume rule.
	selectedAbsorbLineage := scheduler.s5MissingInsertions == 1 &&
		scheduler.work.RecoveryLineageSelections == 1 &&
		scheduler.work.RecoveryLineageRetirements == 0 && scheduler.work.NoActionDrops == 1 &&
		scheduler.selectedRecoveryAbsorbLineage
	if !standaloneRegion && !retiredMissingLineage && !selectedAbsorbLineage {
		return 0, false
	}
	for _, rule := range r.lang.CompactRecoveryTerminalAliasRules {
		if rule.ResumeState == scheduler.s3ResumeState && rule.ResumeSymbol == scheduler.s3ResumeSymbol {
			return rule.AliasSymbol, true
		}
	}
	return 0, false
}

func (r *parserCoreFreshFullRunner) materialize(source []byte, compact *core.Core, head core.Head) (*Tree, error) {
	if r == nil {
		return nil, errors.New("parser-core fresh-full runner is nil")
	}
	return materializeDiagnosticParserCoreAcceptedTree(compact, head, r.parser, source, &r.scratch, r.replayParseStates, r.s3AllowErrorRoot())
}

func (r *parserCoreFreshFullRunner) materializeSelection(source []byte, compact *core.Core, scheduler *diagnosticParserCoreGenericScheduler) (*Tree, error) {
	if r == nil || scheduler == nil {
		return nil, errors.New("parser-core fresh-full selected materialization is incomplete")
	}
	if r.recoveryVersionTurnPublicationRequired(scheduler) &&
		(!scheduler.recoveryTurns.active || scheduler.work.RecoverEOFAccepts == 0) {
		return nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreAccept,
			detail:   "owned recovery publication requires an executed EOF turn",
		}
	}
	gated, err := scheduler.beginCompactEOFRecoveryConstruction(
		source,
		compactEOFRecoveryAdmissionRoutePublicTree,
	)
	if err != nil {
		return nil, err
	}
	r.scratch.recoveryTerminalAliasSymbol, r.scratch.recoveryTerminalAliasCertified =
		r.compactRecoveryTerminalAliasSymbol(scheduler)
	rootFinalization := diagnosticParserCoreFinalizeDefault
	allowErrorRoot := r.s3AllowErrorRoot()
	if r.recoverEOFFinalizationAdmitted(scheduler) {
		rootFinalization = diagnosticParserCoreFinalizeRecoverEOF
		allowErrorRoot = true
	} else if r.options.allowCompactRecoveryVersionTurns && scheduler.recoveryTurns.active && allowErrorRoot {
		rootFinalization = diagnosticParserCoreFinalizeOwnedRecovery
		if scheduler.acceptedRootFinalization == diagnosticParserCoreFinalizeRecoverEOF {
			rootFinalization = diagnosticParserCoreFinalizeRecoverEOF
		}
	}
	r.scratch.materializationBudgetScheduler = scheduler
	tree, err := materializeDiagnosticParserCoreAcceptedSelectionWithRootFinalization(
		compact,
		scheduler.acceptedHead,
		scheduler.acceptedPayloads,
		r.parser,
		source,
		&r.scratch,
		r.replayParseStates,
		allowErrorRoot,
		rootFinalization,
	)
	if err != nil && gated {
		scheduler.failCompactEOFRecoveryConstruction(err)
	}
	return tree, err
}

// The new route does not certify shared S3 recovery. Require its own execution
// for recovery results, while leaving clean parsing and older routes unchanged.
func (r *parserCoreFreshFullRunner) recoveryVersionTurnPublicationRequired(scheduler *diagnosticParserCoreGenericScheduler) bool {
	return r.options.allowCompactRecoveryVersionTurns &&
		(scheduler.s3RegionOpened || scheduler.recoveryTurns.active || scheduler.work.RecoverEOFAccepts != 0)
}

func (r *parserCoreFreshFullRunner) materializeSelectedStoreSelection(
	source []byte,
	compact *core.Core,
	scheduler *diagnosticParserCoreGenericScheduler,
	poll func() error,
) (*core.SelectedStore, error) {
	if r == nil || compact == nil || scheduler == nil {
		return nil, errors.New("parser-core fresh-full selected-store construction is incomplete")
	}
	if r.recoveryVersionTurnPublicationRequired(scheduler) {
		return nil, errors.New("parser-core fresh-full selected-store does not support owned recovery roots")
	}
	if scheduler.acceptedRootFinalization == diagnosticParserCoreFinalizeRecoverEOF {
		return nil, errors.New("parser-core fresh-full selected-store does not support recover_eof roots")
	}
	gated, err := scheduler.beginCompactEOFRecoveryConstruction(
		source,
		compactEOFRecoveryAdmissionRouteSelectedStore,
	)
	if err != nil {
		return nil, err
	}
	store, err := compact.BuildAuthenticatedSelectedStore(scheduler.acceptedPayloads, source, poll)
	if err != nil && gated {
		scheduler.failCompactEOFRecoveryConstruction(err)
	}
	return store, err
}

// parse runs one fresh full parse and returns the materialized tree, or a
// decline error when the compact route cannot serve this input.
//
// A single deferred cleanup guarantees release on every path that returns
// without a tree -- including ones executeSchedulerOpen does not reset
// explicitly itself (a compact.Seed or scheduler-init failure ahead of
// RunFreshSchedulerSession, inside executeDiagnosticParserCoreGenericSchedulerFromSeedInto)
// and a materialization decline below, which runs after acceptance already
// committed the full derivation graph to the core (tranche B9
// reset-completeness gate: before this defer, a materialization decline left
// the ENTIRE accepted parse graph retained on the cached runner while the
// caller's production fallback ran beside it). This is redundant with the
// explicit resets on paths that already handle their own release
// (RunFreshSchedulerSession's own defer on a hard scheduler error, and
// executeSchedulerOpen's acceptance-gate branch); Core.Reset guards against
// double-reset cleanly and is cheap on an already-reset core, so the
// redundancy costs one extra FootprintBytes comparison on the decline path,
// never on the accepted-and-returned path.
func (r *parserCoreFreshFullRunner) parse(source []byte) (tree *Tree, err error) {
	return r.parseWithObserver(source, diagnosticParserCoreSeedObserver{})
}

func (r *parserCoreFreshFullRunner) parseWithObserver(
	source []byte,
	observer diagnosticParserCoreSeedObserver,
) (tree *Tree, err error) {
	if r == nil || r.compact == nil {
		return nil, errors.New("parser-core fresh-full runner is incomplete")
	}
	defer func() {
		if tree != nil {
			return
		}
		if resetErr := r.compact.ResetReleasingRetention(); resetErr != nil {
			err = errors.Join(err, fmt.Errorf("parser-core fresh-full runner: reset after decline: %w", resetErr))
		}
	}()
	recoveryEnabled := r.options.Recovery &&
		(r.options.allowCompactStrategy2ErrorRegion || r.options.allowCompactRecoverEOF)
	if r.recoveryPlainFirst && recoveryEnabled {
		tree, err = r.parseWithObserverAndErrorRuns(source, observer, false, false)
		if err == nil {
			return tree, nil
		}
		if !compactRecoveryRetryAfterPlainEligible(err) {
			return nil, err
		}
		if r.scratch.freshAttemptWork != nil {
			r.scratch.freshAttemptWork.tokens += r.scheduler.tokens
		}
		return r.parseWithObserverAndErrorRuns(source, observer, true, true)
	}
	forceErrorRuns := recoveryEnabled && r.options.allowCompactStrategy2ErrorRegion
	return r.parseWithObserverAndErrorRuns(source, observer, recoveryEnabled, forceErrorRuns)
}

func (r *parserCoreFreshFullRunner) parseWithObserverAndErrorRuns(
	source []byte,
	observer diagnosticParserCoreSeedObserver,
	recoveryEnabled bool,
	forceErrorRuns bool,
) (*Tree, error) {
	savedRecovery := r.options.Recovery
	savedErrorRegion := r.options.allowCompactStrategy2ErrorRegion
	savedRecoverEOF := r.options.allowCompactRecoverEOF
	savedStackSummary := r.options.allowCompactStackSummaryRecovery
	savedMissingInsertion := r.options.allowCompactMissingTokenInsertion
	savedLineageSelection := r.options.allowCompactRecoveryLineageSelection
	savedTrailingRetirement := r.options.allowCompactRecoveryTrailingLineageRetirement
	savedErrorModeKeyword := r.options.allowCompactRecoveryErrorModeKeywordCapture
	if !recoveryEnabled {
		r.options.Recovery = false
		r.options.allowCompactStrategy2ErrorRegion = false
		r.options.allowCompactRecoverEOF = false
		r.options.allowCompactStackSummaryRecovery = false
		r.options.allowCompactMissingTokenInsertion = false
		r.options.allowCompactRecoveryLineageSelection = false
		r.options.allowCompactRecoveryTrailingLineageRetirement = false
		r.options.allowCompactRecoveryErrorModeKeywordCapture = false
	}
	defer func() {
		r.options.Recovery = savedRecovery
		r.options.allowCompactStrategy2ErrorRegion = savedErrorRegion
		r.options.allowCompactRecoverEOF = savedRecoverEOF
		r.options.allowCompactStackSummaryRecovery = savedStackSummary
		r.options.allowCompactMissingTokenInsertion = savedMissingInsertion
		r.options.allowCompactRecoveryLineageSelection = savedLineageSelection
		r.options.allowCompactRecoveryTrailingLineageRetirement = savedTrailingRetirement
		r.options.allowCompactRecoveryErrorModeKeywordCapture = savedErrorModeKeyword
	}()
	scheduler, tokenSource, err := r.executeSchedulerOpenWithObserverAndErrorRuns(
		source, r.compact, true, observer, forceErrorRuns,
	)
	if err != nil {
		return nil, err
	}
	tree, materializeErr := r.materializeSelection(source, r.compact, scheduler)
	if materializeErr == nil && tree != nil && r.options.compactIncrementalReuse == nil &&
		!scheduler.s3RegionOpened && !scheduler.recoveryIsolation {
		tree.captureTokenInvariantReadSpan(tokenSource)
	}
	tokenSource.Close()
	if materializeErr != nil {
		if tree != nil {
			tree.Release()
		}
		return nil, materializeErr
	}
	if tree != nil {
		tree.parseRuntime.TokensConsumed = scheduler.tokens
		tree.parseRuntime.CompactReductions = scheduler.work.Reductions
	}
	return tree, nil
}

func compactRecoveryRetryAfterPlainEligible(err error) bool {
	var decline *diagnosticParserCoreDecline
	if errors.As(err, &decline) {
		return decline.boundary != DiagnosticParserCoreCap
	}
	message := err.Error()
	return strings.HasPrefix(message, "parser-core fresh-full runner did not accept EOF") ||
		strings.HasPrefix(message, "parser-core fresh-full runner acceptance is not sole exact EOF")
}

func (r *parserCoreFreshFullRunner) parseSelectedStore(source []byte) (*core.SelectedStore, error) {
	if r == nil || r.compact == nil {
		return nil, errors.New("parser-core fresh-full selected-store runner is incomplete")
	}
	scheduler, tokenSource, err := r.executeSchedulerOpen(source, r.compact, true)
	if err != nil {
		return nil, err
	}
	defer tokenSource.Close()
	leaveBudget := r.parser.enterParseBudget()
	defer leaveBudget()
	poller := parseStopPoller{check: r.parser.activeParseStopCheck(), memoryBudgetParser: r.parser}
	poll := func() error {
		reason := poller.pollNow()
		if reason == ParseStopNone || reason == "" {
			return nil
		}
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreCap,
			detail:   "selected-store sealing stopped: " + string(reason),
		}
	}
	store, err := r.materializeSelectedStoreSelection(source, r.compact, scheduler, poll)
	if err != nil {
		return nil, err
	}
	return requireParserCoreSelectedStoreCompleteness(store, len(source))
}

// requireParserCoreSelectedStoreCompleteness adopts store on success and
// releases it on every failure. Callers must not retain another owner.
func requireParserCoreSelectedStoreCompleteness(store *core.SelectedStore, sourceBytes int) (*core.SelectedStore, error) {
	if store == nil || sourceBytes < 0 || uint64(sourceBytes) > uint64(^uint32(0)) {
		if store != nil {
			store.Release()
		}
		return nil, errors.New("parser-core fresh-full selected-store completeness input is invalid")
	}
	root, ok := store.Record(store.Root())
	if !ok || root.StartByte != 0 || root.EndByte != uint32(sourceBytes) {
		store.Release()
		return nil, fmt.Errorf("parser-core fresh-full selected-store root is incomplete: %d..%d source=%d", root.StartByte, root.EndByte, sourceBytes)
	}
	return store, nil
}
