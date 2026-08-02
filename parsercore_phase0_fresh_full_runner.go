//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreFreshFullRunner owns the reusable state for the authenticated,
// fresh UTF-8 compact-parser route. It is deliberately build-tagged and does
// not participate in Parser.parseInternal: callers must opt into this exact
// diagnostic route and handle a fail-closed decline themselves.
//
// The only admitted identity is the certified embedded Go blob and exact Go
// external scanner. The route is fresh UTF-8 only and declines recovery,
// missing/no-lookahead tokens, repetition shifts, extra chains, and any EOF
// frontier other than one accepted head with one exact derivation.
//
// The runner is stateful and not safe for concurrent use. A successful parse
// returns a caller-owned tree. Every call resets the compact arenas before
// acquiring a production DFA token source, so a declined or capped parse
// cannot publish state into the next call.
type parserCoreFreshFullRunner struct {
	lang                              *Language
	parser                            *Parser
	tables                            *parserCoreRootTables
	compact                           *core.Core
	options                           DiagnosticParserCorePrefixOptions
	replayParseStates                 bool
	allowConvergedReductionSplitDrops bool
	scannerScratch                    []byte
	// scratch retains the reusable per-parse materialization buffers. The runner
	// is per-Parser and single-goroutine, so reusing these buffers across parses
	// mirrors production's parser-held arena reuse and keeps the warm steady
	// state from re-allocating the public-tree scratch on every parse.
	scratch   parserCoreRunnerScratch
	scheduler diagnosticParserCoreGenericScheduler
}

func newParserCoreFreshFullRunner(scanner ExternalScanner, options DiagnosticParserCorePrefixOptions) (*parserCoreFreshFullRunner, error) {
	// B3 stage S3: admit options.Recovery only when it is paired with the
	// certified-capability gate (allowCompactStrategy2ErrorRegion, set only
	// from a grammar's own CompactStrategy2ErrorRegionCertified flag). Every
	// other Recovery request -- and Retry/Incremental/IncludedRanges/closed-
	// prefix, none of which this stage touches -- still declines exactly as
	// before this stage landed.
	if (options.Recovery && !options.allowCompactStrategy2ErrorRegion) ||
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
	certifyParserCoreExternalPayloadQuiescence(compact, lang)
	return &parserCoreFreshFullRunner{
		lang: lang, parser: parser, tables: tables, compact: compact, options: options,
		allowConvergedReductionSplitDrops: true,
	}, nil
}

func (r *parserCoreFreshFullRunner) executeSchedulerOpen(source []byte, compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	if r == nil || r.parser == nil || r.lang == nil || r.tables == nil || compact == nil {
		return nil, nil, errors.New("parser-core fresh-full runner is incomplete")
	}
	if reset {
		if err := compact.Reset(); err != nil {
			return nil, nil, err
		}
	}
	// B3 stage S3: force the shared token source's error-run lexing on when
	// native compact recovery is admitted, so a genuinely unlexable byte run
	// surfaces as its own errorSymbol token (parser_api.go's
	// acquireParserDFATokenSourceWithErrorRuns doc comment) instead of the
	// plain lexer silently skipping it -- the silent-skip shape a true no-
	// table-action dispatch point can never observe, verified against every
	// committed html_erroneous_end_tag witness.
	forceErrorRuns := r.options.Recovery && r.options.allowCompactStrategy2ErrorRegion
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
		r.options, diagnosticParserCoreSeedObserver{},
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
	if acceptance.Token.Symbol != 0 || acceptance.Token.StartByte != wantEOF || acceptance.Token.EndByte != wantEOF ||
		acceptance.Token.Missing || acceptance.Token.NoLookahead || acceptance.Token.ExternalScannerToken ||
		!header.Accepted || header.Paused || header.ExactPaths != 1 && !selectedCertifiedPrimary ||
		!parserCoreFreshFullAcceptedTailIsClean(source, header.ByteOffset, continuationEscape) || acceptance.Accepts != 1 || acceptance.Work.Accepts != 1 {
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
	return materializeDiagnosticParserCoreAcceptedSelection(compact, scheduler.acceptedHead, scheduler.acceptedPayloads, r.parser, source, &r.scratch, r.replayParseStates, r.s3AllowErrorRoot())
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
	scheduler, tokenSource, err2 := r.executeSchedulerOpen(source, r.compact, true)
	if err2 != nil {
		return nil, err2
	}
	defer tokenSource.Close()
	tree, err = r.materializeSelection(source, r.compact, scheduler)
	if err != nil {
		return nil, err
	}
	return tree, nil
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
	store, err := r.compact.BuildAuthenticatedSelectedStore(scheduler.acceptedPayloads, source, poll)
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
