//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"os"
	"strings"
	"sync"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
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
	// censusMechanismEOFScannerQuiescence: the frontier reached the compact
	// end-of-file admission shape (one accepting head, one no-action
	// sibling) in a scanner-owning language, and
	// proveCompactEOFScannerQuiescence
	// (parsercore_phase0_eof_scanner_quiescence.go) could not prove that
	// every head sees the same authenticated end-of-input token under its
	// own external lex row. The decline detail names the step that failed,
	// so this bucket sizes the remaining scanner work by cause instead of
	// by language.
	censusMechanismEOFScannerQuiescence admissionCensusMechanism = "eof-scanner-quiescence"
	// censusMechanismOther is the catch-all for a decline this classifier
	// does not yet recognize. The full original detail is always preserved
	// alongside it.
	censusMechanismOther admissionCensusMechanism = "other-with-detail"
)

var (
	admissionCensusEnabledOnce sync.Once
	admissionCensusEnabledVal  bool

	admissionCensusRecoveryShapeOnce sync.Once
	admissionCensusRecoveryShapeVal  bool
)

// admissionCensusRecoveryShapeEnabled reports whether the operator asked for
// the recovery sub-classification on top of the ordinary census.
//
// It is a SEPARATE opt-in from GTS_ADMISSION_CENSUS, for two reasons.
//
// First, blast radius. cgo_harness/testmain_cgo_test.go sets
// GTS_ADMISSION_CENSUS=1 process-wide for that whole package, and six tests
// there pin the recovery decline reason by EXACT equality (for example
// sqlC26ahCompactFallback). Appending to the census detail under the existing
// flag silently breaks all six, and those tests are container-only -- they
// authenticate a pinned compiler and grammar artifact, so a host run cannot
// verify a change to them. Keeping this behind its own flag leaves that
// harness byte-identical.
//
// Second, cost. The classification walks every terminal in the grammar's
// action table for each classified decline, which is far heavier than the
// existing census's constant-time boundary mapping. An operator who wants
// only the coarse mechanism should not pay for the fine one.
func admissionCensusRecoveryShapeEnabled() bool {
	admissionCensusRecoveryShapeOnce.Do(func() {
		switch os.Getenv("GTS_ADMISSION_CENSUS_RECOVERY_SHAPE") {
		case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
			admissionCensusRecoveryShapeVal = true
		}
	})
	return admissionCensusRecoveryShapeVal
}

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
		case strings.Contains(detail, "scanner quiescence"),
			strings.Contains(detail, "zero-width external"):
			return censusMechanismEOFScannerQuiescence
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

// admissionCensusRecoveryShape sub-classifies a recovery-boundary decline by
// the C recovery mechanism that would own the input at that exact point.
//
// censusMechanismRecovery already tells an operator "only production
// implements recovery here". That answer is too coarse to schedule work
// against: the real-corpus recovery cohort is not one mechanism but four,
// and they sit in different B3 stages with different costs
// (spec.compact-recovery-ownership.v1 section 4). This classification splits
// the cohort the way C's own ts_parser__handle_error / ts_parser__recover
// split it, so the census reports which stage owns each declining row.
//
// Evaluation order mirrors C exactly, because C's mechanisms are tried in a
// fixed order and the first one that applies is the one that runs:
//
//  1. missing-token insertion (ts_parser__handle_error step 2,
//     parser.c:2154-2230; the Go port is cHandleError's missingTokenSearch,
//     parser_recover_c.go);
//  2. strategy 1, the stack-summary resume election (ts_parser__recover's
//     summary scan, cRecoverStrategy1Election); C runs this for every
//     non-error lookahead INCLUDING end-of-file;
//  3. the end-of-file whole-file wrap (ts_parser__recover's recover_eof,
//     cRecoverEOFAccept);
//  4. strategy 2, error-region absorb (ts_parser__recover's skip_token tail,
//     cAbsorbTokenIntoError).
//
// Every probe below is READ-ONLY. It reads action rows and walks recorded
// link adjacency; it publishes no record and mutates no header. In
// particular it deliberately does NOT run the compact analogue of C's step 1
// (s3CloseInProgressProductions), which performs real reductions and appends
// arena records: a diagnostic classification must not change what the parse
// did. The consequence is stated in the shape's own name -- the scan happens
// at the declining state, not at C's post-closure state -- so a row whose
// missing-token opportunity only appears after closure classifies here as a
// later mechanism. That is a floor on the missing-token cohort, never an
// overcount.
//
// Read the answer as "the mechanism that owns THIS decline point", not "the
// only mechanism this source needs". A source can require several recovery
// mechanisms in sequence, and the compact route reports only the first point
// at which it gave up. Measured on the shipped real corpus: widening the
// native strategy-2 owner to four uncertified grammars opened twenty error
// regions and still graduated no row, because each row went on to reach an
// end-of-file wrap, a missing-token insertion, or an error-mode lex
// disagreement that strategy 2 does not own. Use this classification to size
// and order the stages, not to predict a per-row graduation.
type admissionCensusRecoveryShape string

const (
	// censusRecoveryShapeMissingToken: some terminal has a genuine shift from
	// the declining state to a different state whose leading action for the
	// elected token is a reduce -- C's step-2 predicate
	// (ts_language_next_state + ts_language_has_reduce_action). C would insert
	// a zero-width MISSING leaf for that terminal and keep parsing. B3 stage
	// S5 owns this mechanism.
	censusRecoveryShapeMissingToken admissionCensusRecoveryShape = "missing-token-insertion"
	// censusRecoveryShapeDeeperResume: no missing-token opportunity, but some
	// ancestor boundary within cRecoverMaxSummaryDepth has an action for the
	// elected token. C's strategy-1 summary election would recover to that
	// state and wrap the skipped span in one ERROR. B3 stage S4 owns this.
	//
	// This bucket OVER-APPROXIMATES strategy 1, so read it as an S4 ceiling
	// and, correspondingly, an S3 floor. AncestorStateWithActionExists is a
	// bounded existence probe with no cost comparison and no election, while
	// C's summary scan additionally skips entries at ERROR_STATE, skips
	// entries whose position equals the current position, and ABORTS the whole
	// scan when a cheaper live version exists -- even where the lookahead
	// would have had actions. None of those three are modeled here, and every
	// row this bucket over-reports is a row taken from the absorb bucket
	// below. An operator sizing S3 against S4 from this census will therefore
	// under-size S3.
	censusRecoveryShapeDeeperResume admissionCensusRecoveryShape = "stack-summary-resume"
	// censusRecoveryShapeEOFWrap: the elected token is authenticated
	// end-of-file and neither earlier mechanism applies, so C reaches
	// recover_eof and wraps the whole remaining stack in one ERROR root. B3
	// stage S5 owns this wrap.
	censusRecoveryShapeEOFWrap admissionCensusRecoveryShape = "recover-eof-wrap"
	// censusRecoveryShapeAbsorb: none of the above -- C falls through to
	// strategy 2 and absorbs the elected token into an open error region.
	// B3 stage S3 owns this mechanism; it is native today only for the
	// grammar-blob-keyed certified witness class
	// (CompactStrategy2ErrorRegionCertified).
	censusRecoveryShapeAbsorb admissionCensusRecoveryShape = "error-region-absorb"
)

// admissionCensusMissingTokenCandidates counts the terminals that satisfy C's
// step-2 missing-token predicate at state for lookahead, and reports the
// lowest such terminal id. C scans terminals in ascending id order and takes
// the first that also survives do_all_potential_reductions, so the lowest id
// is the candidate C would try first.
//
// The predicate is the same one s3MissingTokenOpportunityExists already
// applies as an S3 decline guard (parsercore_phase0_driver.go), kept as a
// separate counting variant here rather than widening that one: the S3 guard
// answers a yes/no question on the hot decline path and must stay cheap to
// read, while the census wants the population.
func admissionCensusMissingTokenCandidates(scheduler *diagnosticParserCoreGenericScheduler, state StateID, lookahead Symbol) (count int, first Symbol, err error) {
	if scheduler == nil || scheduler.compact == nil ||
		scheduler.tokenSource == nil || scheduler.tokenSource.language == nil {
		return 0, 0, nil
	}
	tokenCount := Symbol(scheduler.tokenSource.language.TokenCount)
	for ms := Symbol(1); ms < tokenCount; ms++ {
		row, rowErr := scheduler.compact.Actions(core.StateID(state), core.Symbol(ms))
		if rowErr != nil {
			return 0, 0, rowErr
		}
		if row.Len() == 0 {
			continue
		}
		last := row.At(row.Len() - 1)
		if last.Type != core.ActionShift {
			continue
		}
		nextState := core.StateID(last.State)
		if last.Extra {
			nextState = core.StateID(state)
		}
		if nextState == 0 || nextState == core.StateID(state) {
			continue
		}
		nextRow, nextErr := scheduler.compact.Actions(nextState, core.Symbol(lookahead))
		if nextErr != nil {
			return 0, 0, nextErr
		}
		if nextRow.Len() == 0 || nextRow.At(0).Type != core.ActionReduce {
			continue
		}
		if count == 0 {
			first = ms
		}
		count++
	}
	return count, first, nil
}

// admissionCensusRecoveryShapeFor classifies one recovery-boundary decline,
// or reports an empty shape when the decline is not a classifiable recovery
// point, in which case the caller leaves the decline text unchanged.
//
// DiagnosticParserCoreRecovery is a ROUTING bucket, not a proof that C is in
// recovery. The driver publishes it with four different details, and three of
// them are Go-side deferrals where C never enters ts_parser__handle_error at
// all: the D2-1 ragged-relex decline, the issue-983 contextual close-angle
// deferral (whose state DOES have an action for the elected token), and an
// ActionRecover cell reached inside a conflict. Classifying those would
// attribute a C mechanism to a point C never reaches. The ragged-relex case is
// worse than merely wrong: `finish` records the SHARED election token, while
// the decline is about a different relexed token with a different EndByte, so
// any probe here would run against a lookahead that was never the stop point.
//
// So this classifies exactly one detail -- the genuine no-table-action shape
// that is C's own error-entry point -- and declines to guess on the rest.
//
// One modeled gap remains, recorded rather than silently carried: the Go
// port suppresses BOTH the missing-token search and the strategy-1 election
// for a graphql triple-quote lookahead (isGraphQLRecoveryTripleQuote). This
// ladder does not apply that predicate, so a graphql decline on that token
// can report missing-token-insertion or stack-summary-resume where the port
// would go straight to absorb. No graphql row reaches this path on the
// shipped corpus today; a census that adds one must model the predicate.
func admissionCensusRecoveryShapeFor(scheduler *diagnosticParserCoreGenericScheduler, stop DiagnosticParserCoreGenericStop) (admissionCensusRecoveryShape, string) {
	if scheduler == nil || scheduler.compact == nil {
		return "", ""
	}
	if stop.Detail != diagnosticParserCoreNoTableActionDetail {
		return "", ""
	}
	lookahead := Symbol(stop.Token.Symbol)

	// Step 2, missing-token insertion. C applies NO error-lookahead guard
	// here: ts_parser__handle_error wraps its missingTokenSearch only in the
	// graphql triple-quote exclusion, and the errorSymbol guard lives solely
	// in cRecoverStrategy1Election. Probing this first, for every lookahead,
	// is what keeps stage S5 from being under-credited.
	missingCount, missingFirst, err := admissionCensusMissingTokenCandidates(scheduler, stop.State, lookahead)
	if err != nil {
		return "", ""
	}
	if missingCount > 0 {
		return censusRecoveryShapeMissingToken, fmt.Sprintf("candidates=%d first=%d", missingCount, missingFirst)
	}

	// Strategy 1, the stack-summary resume election. C skips it for an
	// error-subtree lookahead and for a WIDE symbol-0 token, which is this
	// engine's unlexable run (cRecoverStrategy1Election's own guards). Both
	// fall straight through to strategy 2.
	wideUnlexable := lookahead == 0 && stop.Token.StartByte != stop.Token.EndByte
	if lookahead != errorSymbol && !wideUnlexable {
		if stop.HeaderIndex < 0 || stop.HeaderIndex >= len(scheduler.headers) {
			return "", ""
		}
		deeper, deeperErr := scheduler.compact.AncestorStateWithActionExists(
			scheduler.headers[stop.HeaderIndex].head, core.Symbol(lookahead), cRecoverMaxSummaryDepth)
		if deeperErr != nil {
			// The strategy-1 question was never answered. Emitting the last
			// bucket in the ladder here would publish a confident S3 verdict
			// on an unrun probe, so report no classification instead.
			return "", ""
		}
		if deeper {
			return censusRecoveryShapeDeeperResume, ""
		}
	}

	if lookahead == 0 && stop.Token.StartByte == stop.Token.EndByte {
		return censusRecoveryShapeEOFWrap, ""
	}
	return censusRecoveryShapeAbsorb, ""
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
	detail := "did not accept EOF: " + stop.Detail
	// A recovery-boundary decline is the largest real-corpus fallback cohort
	// and the only one whose members sit in different B3 stages. Sub-classify
	// it by the C mechanism that owns the input (admissionCensusRecoveryShape)
	// so the census reports a schedulable cohort instead of one undivided
	// "recovery" bucket.
	//
	// Behind its own opt-in: with GTS_ADMISSION_CENSUS alone every decline
	// text, recovery included, stays byte-identical to before this tranche.
	// See admissionCensusRecoveryShapeEnabled for why that separation is
	// load-bearing rather than tidy.
	if stop.Boundary == DiagnosticParserCoreRecovery && admissionCensusRecoveryShapeEnabled() {
		if shape, extra := admissionCensusRecoveryShapeFor(scheduler, stop); shape != "" {
			detail += " [c-mechanism=" + string(shape)
			if extra != "" {
				detail += " " + extra
			}
			detail += "]"
		}
	}
	if scheduler.work.StackSummaryRecoveryForks != 0 {
		marked := 0
		openRegions := 0
		for index := range scheduler.headers {
			if scheduler.headers[index].isRecoveryLineage() {
				marked++
			}
			if scheduler.headers[index].recoveryRegion() != nil {
				openRegions++
			}
		}
		detail += fmt.Sprintf(
			" [compact-s4-forks=%d headers=%d marked=%d open_regions=%d stop_header=%d]",
			scheduler.work.StackSummaryRecoveryForks, len(scheduler.headers), marked, openRegions, stop.HeaderIndex,
		)
	}
	return &diagnosticParserCoreDecline{
		boundary: stop.Boundary,
		detail:   detail,
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
