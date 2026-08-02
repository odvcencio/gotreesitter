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

// newAdmissionCandidateRunner builds a fresh-full runner bound to p's own
// language, external scanner, and DFA tables.
func newAdmissionCandidateRunner(p *Parser) (*parserCoreFreshFullRunner, error) {
	if p == nil || p.language == nil {
		return nil, errors.New("admission candidate route: parser has no language")
	}
	options := DiagnosticParserCorePrefixOptions{
		ReceiptMode:                     DiagnosticParserCoreReceiptSummary,
		MaxTokens:                       1 << 24,
		MaxDispatches:                   1 << 24,
		Limits:                          admissionCandidateLimits(),
		freshSchedulerSession:           true,
		allowEOFAcceptNoActionSiblings:  p.language.CompactEOFAcceptNoActionSiblingsCertified,
		allowPrimaryAcceptDerivation:    p.language.CompactPrimaryAcceptanceDerivationCertified,
		allowConvergedSplitDropArtifact: p.language.CompactConvergedReductionSplitDropsCertified,
		// B3 stage S3: Recovery declares that this operation may attempt
		// native strategy-2 recovery; allowCompactStrategy2ErrorRegion is the
		// certified-capability gate the scheduler actually checks
		// (dispatchPassActive's s3ErrorRegionAdmitted). Both come from the
		// same grammar-blob-keyed certification, so an uncertified grammar
		// (every grammar but the one B3 stage S3 witness class covers today)
		// gets both false and this parse's behavior is unchanged.
		Recovery:                         p.language.CompactStrategy2ErrorRegionCertified,
		allowCompactStrategy2ErrorRegion: p.language.CompactStrategy2ErrorRegionCertified,
		noLookaheadRootSymbol:            p.rootSymbol,
		hasNoLookaheadRootSymbol:         p.hasRootSymbol,
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
	certifyParserCoreExternalPayloadQuiescence(compact, p.language)
	return &parserCoreFreshFullRunner{
		lang: p.language, parser: p, tables: tables, compact: compact, options: options,
		// Incremental reuse is admitted only when materialization can attach the
		// table-replayed state proof. Diagnostic runners retain their explicit
		// GTS_REPLAY_PARSESTATE A/B switch; the production candidate always asks
		// for the proof and still fails closed per tree when it is incomplete.
		replayParseStates:                 true,
		allowConvergedReductionSplitDrops: p.language.CompactConvergedReductionSplitDropsCertified,
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
	endOp := p.beginParseOperationBudget()
	defer endOp()
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
	// result_compat_elision.go), all three full-fidelity steps below are
	// provably no-ops on THIS route -- see docs/compat-tail-elision.md for the
	// per-step proof -- so an eligible language takes the reduced tail instead.
	if resultCompatibilityElisionActive(p.language) {
		p.finalizeCompactReturnedTreeForParse(tree, source)
		return tree, true, ""
	}
	p.normalizeReturnedTreeForParse(tree, source)
	tree = p.resolveCRecoverySwallowedError(source, tree)
	tree = p.maybeCompactReturnedFullTree(tree, source)
	return tree, true, ""
}

// finalizeCompactReturnedTreeForParse is the elided compat-tail for an
// elision-eligible language on the compact fresh path. It reproduces
// normalizeReturnedTreeForParse's own control flow with its three no-op steps
// removed for this route and these languages -- see
// docs/compat-tail-elision.md for the equivalence proof of each removal:
//
//  1. tree.hasDeferredResultCompatibility() is always false for a compact-
//     materialized tree: only resultRootBuild.finishTree (production's own
//     tree builder, parser_result_root_build.go) ever calls
//     deferResultCompatibility, and the compact route never runs it. The
//     deferred-truncation branch is dead here, so it is not reproduced.
//  2. tree.resultCompatibilityApplied is always false for a freshly
//     compact-materialized tree (materializeDiagnosticParserCoreAcceptedSelection
//     never sets it), so the original always entered its normalize branch.
//     This function always performs the one check that branch actually does
//     for an eligible language on this route (see point 3) and then always
//     sets the flag, matching the original's post-normalize assignment.
//  3. The original's call to p.normalizeReturnedTree, for an elision-eligible
//     language, reduces to exactly one more p.activeParseStopReason() read:
//     runLanguageResultCompatibility's switch has no matching case (a pure
//     ctx.stopReason() passthrough), and summarizeResultErrorsWithStop's
//     errorSummary and stopReason are both discarded by normalizeReturnedTree
//     itself, which recomputes p.parseStopReasonNow() -- the same
//     p.activeParseStopReason() call -- unconditionally afterward regardless
//     of what the discarded walk found. This function performs that same
//     final check directly instead of the walk that used to precede it.
//  4. resolveCRecoverySwallowedError and maybeCompactReturnedFullTree are not
//     called at all: both are unconditional no-ops for every compact-route
//     tree regardless of language (the former because compact materialization
//     never sets the two C-recovery signal fields it requires both to be
//     true; the latter because its 512 MiB retained-arena floor is far above
//     anything a single-derivation compact build produces for any fixture
//     this campaign measures). See docs/compat-tail-elision.md.
func (p *Parser) finalizeCompactReturnedTreeForParse(tree *Tree, source []byte) {
	if !shouldNormalizeReturnedTree(tree) {
		return
	}
	if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
		tree.setParseStopReason(reason)
		return
	}
	tree.resultCompatibilityApplied = true
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
