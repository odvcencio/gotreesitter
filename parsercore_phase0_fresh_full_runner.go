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
	if r.options.stopControlParser != nil {
		r.options.stopControlMemoryBudgetBytes = parseMemoryBudgetForParser(r.options.stopControlParser, len(source))
	}
	scheduler, err := executeDiagnosticParserCoreGenericSchedulerFromSeedInto(
		&r.scheduler, compact, tokenSource, &r.scannerScratch, r.lang.InitialState,
		r.options, diagnosticParserCoreSeedObserver{},
	)
	if err != nil {
		tokenSource.Close()
		return nil, nil, err
	}
	if err := requireParserCoreFreshFullAcceptance(scheduler, source, r.allowConvergedReductionSplitDrops); err != nil {
		tokenSource.Close()
		return nil, nil, err
	}
	return scheduler, tokenSource, nil
}

func requireParserCoreFreshFullAcceptance(scheduler *diagnosticParserCoreGenericScheduler, source []byte, allowConvergedReductionSplitDrops bool) error {
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
		!parserCoreFreshFullAcceptedTailIsClean(source, header.ByteOffset) || acceptance.Accepts != 1 || acceptance.Work.Accepts != 1 {
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
// padding; a real source byte keeps the route fail-closed.
func parserCoreFreshFullAcceptedTailIsClean(source []byte, headByte uint32) bool {
	if uint64(len(source)) > uint64(^uint32(0)) || headByte > uint32(len(source)) {
		return false
	}
	return parserTailAllowsCleanAcceptance(source, headByte, uint32(len(source)), nil)
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

func (r *parserCoreFreshFullRunner) parse(source []byte) (*Tree, error) {
	if r == nil || r.compact == nil {
		return nil, errors.New("parser-core fresh-full runner is incomplete")
	}
	scheduler, tokenSource, err := r.executeSchedulerOpen(source, r.compact, true)
	if err != nil {
		return nil, err
	}
	defer tokenSource.Close()
	tree, err := r.materializeSelection(source, r.compact, scheduler)
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
