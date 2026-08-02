//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Compact-route decline census instrumentation.
//
// The scorecard test (TestAdmissionCandidateScorecard206) already sorts every
// registered language's full-parse admission attempt into PASS / DIVERGE /
// FALLBACK / SKIP / ERROR. Every FALLBACK carries a decline reason string, but
// two call sites collapse that reason before it reaches the caller:
//
//   - requireParserCoreFreshFullAcceptance folds every scheduler decline that
//     never reaches an accepted EOF head into one message, "did not accept
//     EOF", even though the compact scheduler already recorded a precise
//     boundary and detail in scheduler.receipt.Stop before returning;
//   - the same function folds every accepted-but-not-admissible head (wrong
//     token identity, an ambiguous multi-derivation frontier, a short byte
//     offset) into one message, "acceptance is not sole exact EOF".
//
// This file adds an opt-in classification layer that reads the receipt state
// already computed by the scheduler and renders a fine-grained mechanism
// class alongside the existing detail text. It is diagnostic-only plumbing:
//
//   - gated behind GTS_ADMISSION_CENSUS=1 (unset by default, so every
//     existing test and the scorecard's own default run see byte-identical
//     decline text to before this file existed);
//   - a single cached env read (sync.Once) on the decline path only, which is
//     already the cold, diagnostic path -- it never touches a successful
//     parse.
//
// The classification never changes what the compact route accepts, declines,
// or materializes. It only changes the text of an already-produced decline
// reason when an operator opts in.

// admissionCensusMechanism is the fine-grained decline mechanism class the
// coverage census ranks and aggregates per language.
type admissionCensusMechanism string

const (
	// censusMechanismRecovery: the scheduler reached a recover action, either
	// directly or inside a table-driven conflict. Only the production route
	// implements recovery, so this is a hard, expected decline for any input
	// (or grammar) that needs error correction.
	censusMechanismRecovery admissionCensusMechanism = "recovery-entered"
	// censusMechanismExtraChain: an action carries the ExtraChain flag, which
	// requires distinct nonterminal-chain semantics the generic scheduler does
	// not implement.
	censusMechanismExtraChain admissionCensusMechanism = "extra-chain-shape"
	// censusMechanismExtraShiftShape: an extra-shift cohort was not the sole,
	// homogeneous, all-runnable frontier the generic scheduler requires.
	censusMechanismExtraShiftShape admissionCensusMechanism = "extra-shift-shape"
	// censusMechanismRepetitionShift: an action carries the Repetition flag,
	// which requires production frontier-suppression semantics.
	censusMechanismRepetitionShift admissionCensusMechanism = "repetition-shift-class"
	// censusMechanismZeroWidthShift: an ordinary (non-EOF) shift cell offers a
	// zero-width token -- the scheduler's shift path requires positive width
	// and only the dedicated accept path may consume a zero-width token. This
	// is a distinct, uniform shape from the general scheduler-frontier-shape
	// catch-all, so it is split out rather than folded into it.
	censusMechanismZeroWidthShift admissionCensusMechanism = "zero-width-shift"
	// censusMechanismExternalScanner: a scanner-checkpoint identity mismatch,
	// or an accepted token sourced from the external scanner where the strict
	// gate requires an authenticated internal EOF.
	censusMechanismExternalScanner admissionCensusMechanism = "external-scanner-state"
	// censusMechanismModeLexFeature: a missing or no-lookahead token, which
	// only the production recovery-lexer path can honor.
	censusMechanismModeLexFeature admissionCensusMechanism = "mode-lex-feature"
	// censusMechanismCapHit: a dispatch or token cap was reached before the
	// frontier closed. The admission scorecard's caps are generous, so a cap
	// hit here is a signal, not routine truncation.
	censusMechanismCapHit admissionCensusMechanism = "cap-hit"
	// censusMechanismMultiDerivation: the accepted head carries more than one
	// exact derivation at EOF, or more than one accept action fired during
	// the run -- an unresolved grammar ambiguity, not a missing feature.
	censusMechanismMultiDerivation admissionCensusMechanism = "multi-derivation-at-eof"
	// censusMechanismEOFByteShort: the accepted head's byte offset is short
	// of the source length even though exactly one derivation and one accept
	// were observed -- the frontier closed before consuming every byte (for
	// example a trailing extra/whitespace token attached outside the
	// accepted derivation).
	censusMechanismEOFByteShort admissionCensusMechanism = "eof-byte-short-frontier"
	// censusMechanismNoTableAction: every runnable frontier head lacks an
	// action for the elected token. This often marks an input that needs
	// recovery. A clean production tree can instead reveal a scheduler gap.
	//
	// B3 stage S1 promoted the pure form of this shape (every no-action head
	// genuinely table-empty, none from the unrelated group-election pause)
	// from a detail-string match here to its own dispatch boundary,
	// DiagnosticParserCoreRecovery (parsercore_phase0_driver.go), which
	// classifies as censusMechanismRecovery below instead. This case and
	// constant stay as a defensive mapping for any other caller that still
	// pairs DiagnosticParserCoreNoAction with diagnosticParserCoreNoTableActionDetail.
	censusMechanismNoTableAction admissionCensusMechanism = "no-table-action"
	// censusMechanismSchedulerShape: the generic scheduler's structural
	// invariants (sole runnable head, homogeneous accept frontier, no mixed
	// accepted/shifted heads, closed-and-checkpoint-continuous election) were
	// not met, independent of any specific grammar feature.
	censusMechanismSchedulerShape admissionCensusMechanism = "scheduler-frontier-shape"
	// censusMechanismAcceptedLeafTilingGap: the scheduler accepted a clean,
	// full-span root, but diagnosticParserCoreReduceChildrenTilingGap
	// (parsercore_phase0_driver.go, campaign v7 tranche B1), checked once per
	// reduce during materialization, found a byte range under some subtree's
	// declared span with no covering child and no tolerated-trivia excuse.
	// This is the false-clean route-equality gate: without it, the accepted
	// derivation would publish HasError()==false while production and the
	// locked C oracle both return an error tree for the same input
	// (cgo_harness/testdata/compact_t3_oracle_witnesses_v2.json).
	censusMechanismAcceptedLeafTilingGap admissionCensusMechanism = "accepted-leaf-tiling-gap"
	// censusMechanismAcceptedRootLeadingGap: the accepted derivation's own
	// root reduce declared a span starting after the source's first
	// non-trivia byte (parsercore_phase0_driver.go,
	// materializeDiagnosticParserCoreAcceptedSelection). The root reduce is
	// exempt from censusMechanismAcceptedLeafTilingGap's own-children check
	// (isDerivationRootReduce), so a dropped leading byte at the root symbol
	// itself passes that gate by construction; the shared post-materialization
	// normalizeRootSourceStart (parser_result_root_build.go) then pulls the
	// public root span back over the gap on the assumption of a legitimately
	// elided leading extra, publishing HasError()==false. This is the second,
	// root-specific false-clean route-equality gate: without it, an accepted
	// derivation that never represented one real byte at document start would
	// publish a clean tree while production and the locked C oracle both
	// report an error for the same input.
	censusMechanismAcceptedRootLeadingGap admissionCensusMechanism = "accepted-root-leading-gap"
	// censusMechanismMaterialAcceptanceElection: the accepted head carried a
	// tied compact acceptance election (selectCompactAcceptanceDerivation's
	// score-tie guard, gated by compactAcceptanceElectionIsVacuous in
	// parsercore_phase0_driver.go) with more than one live derivation, and
	// materializing every one of them did not prove they all publish the
	// same tree. The route declines instead of admitting the positional
	// primary derivation; production still serves the input.
	censusMechanismMaterialAcceptanceElection admissionCensusMechanism = "material-acceptance-election"
	// censusMechanismOther is the catch-all for a decline this classifier
	// does not yet recognize. The full original detail is always preserved
	// alongside it.
	censusMechanismOther admissionCensusMechanism = "other-with-detail"
)

var (
	admissionCensusEnabledOnce sync.Once
	admissionCensusEnabledVal  bool
)

// admissionCensusEnabled reports whether GTS_ADMISSION_CENSUS requests the
// fine-grained decline classification. The result is cached after the first
// read (sync.Once), so every later decline on the same process pays only one
// atomic-guarded boolean read, not a fresh os.Getenv.
func admissionCensusEnabled() bool {
	admissionCensusEnabledOnce.Do(func() {
		switch os.Getenv("GTS_ADMISSION_CENSUS") {
		case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
			admissionCensusEnabledVal = true
		}
	})
	return admissionCensusEnabledVal
}

// admissionCensusClassify maps any (boundary, detail) decline pair -- whether
// it came from a hard *diagnosticParserCoreDecline error or from a scheduler
// Stop receipt recorded on a soft (nil-error) decline -- onto one census
// mechanism. Matching is boundary-first, then a small set of detail
// substrings that separate feature classes sharing the "unsupported_route"
// boundary (repetition shifts and mode/lex features both decline there).
func admissionCensusClassify(boundary DiagnosticParserCoreBoundaryKind, detail string) admissionCensusMechanism {
	switch boundary {
	case DiagnosticParserCoreRecovery:
		return censusMechanismRecovery
	case DiagnosticParserCoreExtraChain:
		return censusMechanismExtraChain
	case DiagnosticParserCoreExtra:
		return censusMechanismExtraShiftShape
	case DiagnosticParserCoreIdentity:
		return censusMechanismExternalScanner
	case DiagnosticParserCoreCap:
		return censusMechanismCapHit
	case DiagnosticParserCoreRoute:
		switch {
		case strings.Contains(detail, "repetition shift"):
			return censusMechanismRepetitionShift
		case strings.Contains(detail, "no-lookahead token"), strings.Contains(detail, "missing token"):
			return censusMechanismModeLexFeature
		case strings.Contains(detail, "recover"):
			return censusMechanismRecovery
		case strings.Contains(detail, "not positive-width"):
			return censusMechanismZeroWidthShift
		default:
			return censusMechanismSchedulerShape
		}
	case DiagnosticParserCoreNoAction:
		if detail == diagnosticParserCoreNoTableActionDetail {
			return censusMechanismNoTableAction
		}
		return censusMechanismSchedulerShape
	case DiagnosticParserCoreAccept:
		switch {
		case strings.Contains(detail, "accepted-leaf-tiling-gap"):
			return censusMechanismAcceptedLeafTilingGap
		case strings.Contains(detail, "accepted-root-leading-gap"):
			return censusMechanismAcceptedRootLeadingGap
		case strings.Contains(detail, "material-acceptance-election"):
			return censusMechanismMaterialAcceptanceElection
		default:
			return censusMechanismSchedulerShape
		}
	case DiagnosticParserCoreGenericClosed:
		return censusMechanismSchedulerShape
	case censusBoundaryMultiDerivation:
		return censusMechanismMultiDerivation
	case censusBoundaryEOFByteShort:
		return censusMechanismEOFByteShort
	case censusBoundaryEOFTokenMismatch:
		return censusMechanismExternalScanner
	default:
		return censusMechanismOther
	}
}

// Synthetic boundary kinds for the strict-acceptance decompositions that have
// no natural DiagnosticParserCoreBoundaryKind of their own: the scheduler DID
// accept, so none of the scheduler's own mid-run unsupported-construct
// boundaries apply. These exist only so admissionCensusAcceptanceDecline can
// return the same *diagnosticParserCoreDecline type every other decline path
// already returns, letting admissionCandidateDeclineReason format every
// decline (hard or census-classified) through one code path.
const (
	censusBoundaryMultiDerivation  DiagnosticParserCoreBoundaryKind = "census_multi_derivation_at_eof"
	censusBoundaryEOFByteShort     DiagnosticParserCoreBoundaryKind = "census_eof_byte_short"
	censusBoundaryEOFTokenMismatch DiagnosticParserCoreBoundaryKind = "census_eof_token_mismatch"
)

// admissionCensusStopDecline renders the fine-grained classification for a
// soft decline: the scheduler run completed without a Go error but never
// reached an accepted EOF head. scheduler.receipt.Stop already carries the
// boundary and detail the scheduler recorded when it gave up; this function
// only classifies and re-surfaces it under the shared decline type. Called
// only when admissionCensusEnabled is true.
func admissionCensusStopDecline(scheduler *diagnosticParserCoreGenericScheduler) error {
	if scheduler == nil || scheduler.receipt == nil || scheduler.receipt.Stop.Boundary == "" {
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "fresh-full runner did not accept EOF and no scheduler stop was recorded",
		}
	}
	stop := scheduler.receipt.Stop
	return &diagnosticParserCoreDecline{
		boundary: stop.Boundary,
		detail:   "did not accept EOF: " + stop.Detail,
	}
}

// admissionCensusAcceptanceDecline renders the fine-grained classification
// for a strict-acceptance decline: the scheduler accepted exactly one head
// with one exact derivation, but the fresh-full runner's byte-exact-sole-EOF
// admission bar still declined it. Called only when admissionCensusEnabled is
// true.
func admissionCensusAcceptanceDecline(acceptance *DiagnosticParserCoreGenericAcceptance, wantEOF uint32) error {
	header := acceptance.Header.Header
	var boundary DiagnosticParserCoreBoundaryKind
	var why string
	switch {
	case acceptance.Token.Missing, acceptance.Token.NoLookahead:
		boundary, why = DiagnosticParserCoreRoute, "accept token carries a recovery-lexer flag (missing or no-lookahead)"
	case acceptance.Token.ExternalScannerToken:
		boundary, why = censusBoundaryEOFTokenMismatch, "accept token is external-scanner-sourced, not the authenticated internal EOF"
	case acceptance.Token.Symbol != 0, acceptance.Token.StartByte != wantEOF, acceptance.Token.EndByte != wantEOF:
		boundary = censusBoundaryEOFTokenMismatch
		why = fmt.Sprintf("accept token is not authenticated zero-width EOF (symbol=%d start=%d end=%d want=%d)",
			acceptance.Token.Symbol, acceptance.Token.StartByte, acceptance.Token.EndByte, wantEOF)
	case !header.Accepted, header.Paused:
		boundary = DiagnosticParserCoreAccept
		why = fmt.Sprintf("accepted head is not a clean accepted/unpaused frontier (accepted=%v paused=%v)", header.Accepted, header.Paused)
	case header.ExactPaths != 1:
		boundary = censusBoundaryMultiDerivation
		why = fmt.Sprintf("accepted head carries %d exact derivations at EOF, not one", header.ExactPaths)
	case acceptance.Accepts != 1, acceptance.Work.Accepts != 1:
		boundary = censusBoundaryMultiDerivation
		why = fmt.Sprintf("scheduler fired %d/%d accept actions during the run, not one", acceptance.Accepts, acceptance.Work.Accepts)
	case header.ByteOffset != wantEOF:
		boundary = censusBoundaryEOFByteShort
		why = fmt.Sprintf("accepted head byte offset %d is short of source length %d", header.ByteOffset, wantEOF)
	default:
		boundary, why = DiagnosticParserCoreAccept, "acceptance failed the strict sole-exact-EOF bar for an unclassified reason"
	}
	return &diagnosticParserCoreDecline{
		boundary: boundary,
		detail:   "acceptance is not sole exact EOF: " + why,
	}
}
