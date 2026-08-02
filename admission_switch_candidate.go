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
	p.normalizeReturnedTreeForParse(tree, source)
	tree = p.resolveCRecoverySwallowedError(source, tree)
	tree = p.maybeCompactReturnedFullTree(tree, source)
	return tree, true, ""
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
