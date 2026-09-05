//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// This file is the compact candidate route for the Phase-3 admission switch.
// Phase-3 admission promoted the compact route into the default build, so it
// compiles unless the opt-out emergency tag gts_no_parsercorephase0 is set. In
// an emergency build (-tags gts_no_parsercorephase0) the stub in
// admission_switch_stub.go replaces this file and every eligible full parse
// fails closed to production.
//
// The route reuses the fresh-full runner (parsercore_phase0_fresh_full_runner.go)
// but binds it to the caller's own Parser and Language rather than the certified
// Go blob, so it can attempt every DFA-lexable grammar. The runner's strict
// acceptance gate (one accepted head, one exact EOF derivation, a clean full-
// span root) declines everything it cannot reproduce, so an unsupported grammar
// or a recovering input fails closed and the caller falls back to production.

// admissionCandidateLimits are generous compact-arena bounds for production-
// scale full parses. They mirror the canonical admission limits.
func admissionCandidateLimits() core.Limits {
	return core.Limits{
		MaxNodes: 1 << 20, MaxLinks: 1 << 20, MaxSubtrees: 1 << 20,
		MaxChildren: 4 << 20, MaxMetadata: 2 << 20,
		MaxLinksPerBoundary: 8, MaxPopPaths: 1 << 16, MaxDerivations: 1 << 16,
	}
}

// dropCohortActivationToken identifies one private test activation without a
// counter wrap. The non-zero payload keeps every allocation independently
// addressable, even when an older runner is discarded.
type dropCohortActivationToken struct {
	marker byte
}

// newAdmissionCandidateRunner builds a fresh-full runner bound to p's own
// language, external scanner, and DFA tables.
func newAdmissionCandidateRunner(p *Parser) (*parserCoreFreshFullRunner, error) {
	if p == nil || p.language == nil {
		return nil, errors.New("admission candidate route: parser has no language")
	}
	allowRecoverEOF := compactRecoverEOFArtifactConfigured(p.language)
	allowOwnedEOFRecovery := p.language.CompactOwnedEOFRecoveryCertified
	options := DiagnosticParserCorePrefixOptions{
		ReceiptMode:                    DiagnosticParserCoreReceiptSummary,
		MaxTokens:                      1 << 24,
		MaxDispatches:                  1 << 24,
		Limits:                         admissionCandidateLimits(),
		freshSchedulerSession:          true,
		allowEOFAcceptNoActionSiblings: p.language.CompactEOFAcceptNoActionSiblingsCertified,
		// The metadata producer is private to the admission candidate. It runs
		// only for an exact two-head EOF frontier and proves its own immutable
		// event topology. An explicit public sibling grant keeps the legacy
		// bypass and takes precedence in dispatchPassActive.
		allowMetadataEOFAcceptRecovery:           true,
		allowPrimaryAcceptDerivation:             p.language.CompactPrimaryAcceptanceDerivationCertified,
		allowCompactAcceptanceStructuralElection: p.language.CompactAcceptanceStructuralElectionCertified,
		allowConvergedSplitDropArtifact:          p.language.CompactConvergedReductionSplitDropsCertified,
		captureLexerSkippedPrefixProvenance:      p.language.CompactLexerSkippedPrefixTilingCertified,
		// Recovery binds the shared mechanism, the dedicated recover_eof route,
		// or the owned EOF bundle. The bundle cannot publish shared recovery.
		Recovery:                          p.language.CompactStrategy2ErrorRegionCertified || allowRecoverEOF || allowOwnedEOFRecovery,
		allowCompactStrategy2ErrorRegion:  p.language.CompactStrategy2ErrorRegionCertified || allowOwnedEOFRecovery,
		allowCompactRecoverEOF:            allowRecoverEOF,
		allowCompactStackSummaryRecovery:  p.language.CompactStackSummaryRecoveryCertified,
		allowCompactMissingTokenInsertion: p.language.CompactMissingTokenInsertionCertified || allowOwnedEOFRecovery,
		allowCompactS5EOFMissingInsertion: p.language.CompactS5EOFMissingInsertionCertified,
		allowCompactFaithfulS5Recovery:    p.language.CompactFaithfulS5RecoveryCertified || allowOwnedEOFRecovery,
		allowCompactRecoveryVersionTurns:  allowOwnedEOFRecovery,
		allowCompactRecoveryLineageSelection: allowOwnedEOFRecovery || (p.language.CompactStrategy2ErrorRegionCertified &&
			(p.language.CompactStackSummaryRecoveryCertified || p.language.CompactMissingTokenInsertionCertified)),
		allowCompactRecoveryTrailingLineageRetirement: p.language.CompactRecoveryTrailingLineageRetirementCertified,
		allowCompactRecoveryErrorModeKeywordCapture:   p.language.CompactRecoveryErrorModeKeywordCaptureCertified,
		noLookaheadRootSymbol:                         p.rootSymbol,
		hasNoLookaheadRootSymbol:                      p.hasRootSymbol,
		// Tranche B8 scheduler stop-control: bind this Parser so the scheduler
		// polls its deadline, cancellation flag, and (via
		// stopControlMemoryBudgetBytes, recomputed per parse in
		// executeSchedulerOpen) its production-compatible memory budget. This
		// is the one runner constructor that arms the poll; the diagnostic and
		// benchmark runner (newParserCoreFreshFullRunner) leaves it nil.
		stopControlParser: p,
	}
	tables, err := newParserCoreRootTables(p)
	if err != nil {
		return nil, err
	}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return nil, err
	}
	configureParserCoreScannerProvenance(compact, p.language)
	return &parserCoreFreshFullRunner{
		lang: p.language, parser: p, tables: tables, compact: compact, options: options,
		// Incremental reuse is admitted only when materialization can attach the
		// table-replayed state proof. Diagnostic runners retain their explicit
		// GTS_REPLAY_PARSESTATE A/B switch; the production candidate always asks
		// for the proof and still fails closed per tree when it is incomplete.
		replayParseStates:                 true,
		allowConvergedReductionSplitDrops: p.language.CompactConvergedReductionSplitDropsCertified,
		recoveryPlainFirst:                p.language.CompactRecoveryPlainFirstCertified || allowOwnedEOFRecovery,
	}, nil
}

// acquireAdmissionCandidateRunner returns p's cached candidate runner, building
// and caching one on first use or whenever the parser's language changed.
func (p *Parser) acquireAdmissionCandidateRunner() (*parserCoreFreshFullRunner, error) {
	if p == nil || p.language == nil {
		return nil, errors.New("admission candidate route: parser has no language")
	}
	if cached, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner); ok &&
		cached != nil && cached.lang == p.language && cached.parser == p {
		return cached, nil
	}
	runner, err := newAdmissionCandidateRunner(p)
	if err != nil {
		return nil, err
	}
	p.admissionCandidateRunner = runner
	return runner, nil
}

// DiagnosticEnableDropCohortCertificateAdmissionForTest enables one cached
// candidate parse for focused certificate tests. Production code never calls
// this method, and the returned closure restores the runner before reuse.
func (p *Parser) DiagnosticEnableDropCohortCertificateAdmissionForTest() func() {
	if p == nil || !p.admissionCandidateFullParseEligible(nil, true) {
		return func() {}
	}
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil || runner == nil {
		return func() {}
	}
	token := &dropCohortActivationToken{marker: 1}
	runner.frontierRecordingEnabled = false
	runner.options.recordDropCohortFrontiers = false
	runner.certificateAdmissionToken = token
	runner.certificateAdmissionEnabled = true
	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		cached, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
		if !ok || cached == nil || cached.certificateAdmissionToken != token {
			return
		}
		cached.certificateAdmissionEnabled = false
		cached.frontierRecordingEnabled = false
		cached.certificateAdmissionToken = nil
		cached.options.recordDropCohortCertificates = false
		cached.options.recordDropCohortFrontiers = false
	}
}

// admissionCandidateCompactStorageBytes reports p's cached admission-candidate
// runner's current compact-core storage (internal/parsercorephase0.Core.
// StorageBytes()), or 0 when no runner is cached yet.
//
// StorageBytes reads 0 after ANY Reset, released or not: it counts live
// length, and Reset always truncates length to zero even when it leaves
// capacity retained. It cannot by itself distinguish "genuinely released"
// from "reset but still holding a large backing array" -- use
// admissionCandidateCompactFootprintBytes for that (tranche B9 gate).
func admissionCandidateCompactStorageBytes(p *Parser) uint64 {
	if p == nil {
		return 0
	}
	runner, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !ok || runner == nil || runner.compact == nil {
		return 0
	}
	return runner.compact.StorageBytes()
}

// admissionCandidateCompactFootprintBytes reports p's cached
// admission-candidate runner's current compact-core retained-memory
// footprint (internal/parsercorephase0.Core.FootprintBytes()), or 0 when no
// runner is cached yet. Unlike admissionCandidateCompactStorageBytes, this
// reads real retained CAPACITY, so it can actually detect whether a decline
// path released its retained arenas or merely truncated their logical
// length (tranche B9 storage-release gate). It exists for regression tests
// proving that gate: a declined parse must not leave compact storage
// retained while the caller's production fallback runs.
func admissionCandidateCompactFootprintBytes(p *Parser) uint64 {
	if p == nil {
		return 0
	}
	runner, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !ok || runner == nil || runner.compact == nil {
		return 0
	}
	return runner.compact.FootprintBytes()
}

// tryCompactFullParseRoute attempts the compact candidate route for a fresh
// full parse. It returns (tree, true, "") on success and (nil, false, reason)
// on any decline, so the caller falls back to production.
func (p *Parser) tryCompactFullParseRoute(source []byte) (*Tree, bool, string) {
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		return nil, false, "runner unavailable: " + err.Error()
	}
	operationBudget := p.beginParseOperationBudget()
	defer p.endParseOperationBudget(operationBudget)
	endParse := p.enterParseBudget()
	defer endParse()
	tree, err := runner.parse(source)
	if err != nil {
		return nil, false, admissionCandidateDeclineReason(err)
	}
	if tree == nil {
		return nil, false, "compact route produced no tree"
	}
	// Apply the exact production Parse tail so every returned-tree API surface
	// matches production. The compact tree is already deep-equal to a fully
	// normalized production tree (the canonical admission test proves it), so
	// this deterministic tail is structure-preserving here; it runs for fidelity
	// on the shared runtime-flag and root-span finalization steps.
	//
	// spec.campaign.v7 tranche A5/C1 (compat-tail elision): for a language with
	// no live result-compatibility arm (resultCompatibilityElisionActive, see
	// result_compat_elision.go), two of the three steps below
	// (resolveCRecoverySwallowedError, maybeCompactReturnedFullTree) are
	// provably no-ops on THIS route -- see docs/compat-tail-elision.md for the
	// per-step proof -- so an eligible language skips them. The third step
	// (normalizeReturnedTreeForParse) is NOT skipped, and its own dispatch
	// switch (runLanguageResultCompatibility) and error-summary walk
	// (summarizeResultErrorsWithStop) run ZERO times here, for every
	// language, eligible or not: materializeDiagnosticParserCoreAcceptedSelection
	// already ran the full result-compatibility pass during materialization
	// (buildResultFromNodes -> resultRootBuild.finishTree ->
	// (*Parser).finalizeResultRoot, parser_result_root_build.go), which sets
	// tree.resultCompatibilityApplied before tryCompactFullParseRoute ever
	// runs, so normalizeReturnedTreeForParse's own copy of that dispatch and
	// walk is dead code on this route already -- a route-wide fact, not an
	// eligibility-conditioned one. finalizeCompactReturnedTreeForParse below
	// is control-flow-identical to normalizeReturnedTreeForParse; it exists
	// as its own function only so this route's benchmarks and profiles can
	// name it and so the two no-op steps stay skipped for an eligible
	// language. See docs/compat-tail-elision.md.
	if resultCompatibilityElisionActive(p.language) {
		p.finalizeCompactReturnedTreeForParse(tree, source)
		return tree, true, ""
	}
	p.normalizeReturnedTreeForParse(tree, source)
	tree = p.resolveCRecoverySwallowedError(source, tree)
	tree = p.maybeCompactReturnedFullTree(tree, source)
	return tree, true, ""
}

// finalizeCompactReturnedTreeForParse is normalizeReturnedTreeForParse's own
// control flow, reproduced exactly, for an elision-eligible language on the
// compact fresh path. It is NOT a reduced version of that control flow: an
// earlier version of this function skipped the
// "!tree.resultCompatibilityApplied" guard's terminal-stop-reason check
// unconditionally, which was wrong -- see docs/compat-tail-elision.md's
// "Corrected finding" section for the measured, cross-worktree-verified bug
// report and the fix below.
//
// tree.resultCompatibilityApplied is TRUE for essentially every
// compact-route tree by the time this runs, for every language, eligible or
// not: materializeDiagnosticParserCoreAcceptedSelection already ran
// (*Parser).finalizeResultRoot during materialization (buildResultFromNodes
// -> resultRootBuild.finishTree), which applies the full result-compatibility
// pass -- including, for an ineligible language, a live dispatcher arm -- and
// sets both tree.resultCompatibilityApplied and tree.resultErrorSummary from
// that pass's own result. So the "!tree.resultCompatibilityApplied" guard
// below is false in the overwhelmingly common case, and this function's body
// reduces, in that case, to the same two calls
// normalizeReturnedTreeForParse's body reduces to:
// tree.hasDeferredResultCompatibility() (false for every eligible language:
// shouldDeferResultCompatibility, parser_result_root_build.go, names only
// typescript and tsx, both ineligible -- NOT because the compact route skips
// finishTree; it does not) and finalizeReturnedTreeRootSpan. Preserving the
// guard's exact shape (rather than hoisting the stop-reason check to the top,
// as an earlier version of this function did) matters precisely because it is
// usually false: hoisting it added a stop-reason re-check the original
// control flow never performs once compatibility is already applied, which
// let an unrelated concurrent cancellation discard an already-complete, good
// tree that the unmodified tail would have returned successfully.
//
// What the elision actually removes for an eligible language, then, is not
// an O(nodes) walk (see docs/compat-tail-elision.md for why that premise did
// not hold) but exactly two full-tail calls that are unconditional no-ops on
// this route regardless of language:
//
//  1. resolveCRecoverySwallowedError: it requires
//     rt.CRecoveryEnteredErrorState && rt.CRecoveryDroppedErrorForClean, both
//     read from the tree's own captured ParseRuntime. The only assignment
//     site for either field (parser.go, inside the classic GLR engine's own
//     result-building code) never runs on the compact route;
//     materializeDiagnosticParserCoreAcceptedSelection constructs the
//     returned tree's ParseRuntime from a fresh struct literal that never
//     touches either field, so both stay false for every compact-route tree.
//  2. maybeCompactReturnedFullTree: its own gate requires the tree's retained
//     arena to be at least 512 MiB before it does any O(nodes) work. The
//     largest retained arena measured across every real-corpus file this
//     campaign has for an eligible language is 47.45 MiB
//     (zig/large__x86.zig) -- an 11x margin below the floor, not a general
//     claim; see docs/compat-tail-elision.md.
func (p *Parser) finalizeCompactReturnedTreeForParse(tree *Tree, source []byte) {
	if !shouldNormalizeReturnedTree(tree) {
		return
	}
	if compactRecoverEOFRootSpanPreserved(tree) {
		return
	}
	if tree.hasDeferredResultCompatibility() {
		finalizeDeferredReturnedTreeTruncation(tree, source)
		return
	}
	if !tree.resultCompatibilityApplied {
		if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
			tree.setParseStopReason(reason)
			return
		}
		tree.resultCompatibilityApplied = true
	}
	finalizeReturnedTreeRootSpan(tree, source)
}

// admissionCandidateDeclineReason renders a runner error as a compact,
// operator-readable fallback reason, surfacing the decline boundary when known.
//
// With GTS_ADMISSION_CENSUS=1 it additionally tags every decline that carries
// a *diagnosticParserCoreDecline (whether it came from a hard scheduler error
// such as a cap hit, or from the census-classified soft declines in
// admission_census.go) with its fine-grained mechanism class, so a caller
// scraping this string can rank declines by mechanism rather than by the two
// coarse acceptance messages alone. Unset (the default), this is
// byte-identical to before that instrumentation existed.
func admissionCandidateDeclineReason(err error) string {
	var decline *diagnosticParserCoreDecline
	if errors.As(err, &decline) {
		if admissionCensusEnabled() {
			mechanism := admissionCensusClassify(decline.boundary, decline.detail)
			return fmt.Sprintf("compact route declined at %s [mechanism=%s]: %s", decline.boundary, mechanism, decline.detail)
		}
		return "compact route declined at " + string(decline.boundary) + ": " + decline.detail
	}
	return "compact route error: " + err.Error()
}
