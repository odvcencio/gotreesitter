package gotreesitter

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"unicode"
	"unsafe"
)

// parser_recover_c.go is the stage-1 faithful port of tree-sitter C's error
// recovery loop into the pure-Go GLR engine, gated per grammar via parser.c
// capability metadata, explicit default certification, and conservative
// runtime capability validation.
//
// THE C CODE IS THE SPEC (tree-sitter v0.25 lib/src):
//   - parser.c  ts_parser__handle_error / ts_parser__recover /
//     ts_parser__do_all_potential_reductions / ts_parser__recover_to_state /
//     ts_parser__compare_versions / ts_parser__version_status /
//     ts_parser__better_version_exists / ts_parser__condense_stack
//   - stack.c   pause/resume/summary/error-cost bookkeeping
//   - subtree.c ts_subtree_error_cost / summarize_children
//   - error_costs.h cost constants
//
// Mapping notes (Go GLR engine vs C stack versions):
//   - C keeps one Stack with multi-link versions; this engine keeps separate
//     glrStack copies. C's handle_error merges all do_all_potential_reductions
//     versions into ONE multi-path version before recording the stack summary;
//     here each forked version becomes its own absorbing stack with its own
//     summary. The union of per-stack summaries covers the same recovery
//     candidates; the C cost competition (ported below) prunes the set.
//   - C marks an erroring version "paused", lets other versions advance, and
//     resumes the best paused version in condense_stack. This engine processes
//     stacks in lockstep per token, so handle_error runs immediately at the
//     no-action point; cCondenseStacks applies the same cost competition after
//     each dispatch pass.
//   - C's ERROR_STATE (state 0) is real in the generated Go tables too (the
//     recover row). An absorbing stack pushes a node-less stackEntry{state: 0}
//     as the C "NULL subtree discontinuity"; with state 0 on top, the DFA token
//     source lexes with LexModes[0] — exactly C's error-mode lexing.
//   - C's error_repeat chain is flattened: the open error region is a single
//     ERROR node whose children are the absorbed tokens.
//   - C memoizes error cost per subtree; stage 1 recomputes it by walking the
//     stack (gated grammars parse small files). Stage 2 should make it
//     incremental if wider grammars need it.

// C error_costs.h. NOTE: ERROR_COST_PER_SKIPPED_LINE is 30 in the C header
// (the recovery-cost-competition.md table said 2; the header wins;
// that doc moved to gotreesitter-specs (external)).
const (
	cErrCostPerRecovery    = 500
	cErrCostPerMissingTree = 110
	cErrCostPerSkippedTree = 100
	cErrCostPerSkippedLine = 30
	cErrCostPerSkippedChar = 1
)

const (
	// C parser.c MAX_VERSION_COUNT / MAX_SUMMARY_DEPTH / MAX_COST_DIFFERENCE.
	cRecoverMaxVersionCount = 6
	cRecoverMaxSummaryDepth = 16
	// cRecoverMaxCostDifference: 18*ERROR_COST_PER_SKIPPED_TREE matches
	// tree-sitter v0.25.0 (parser.c:83 — the oracle cgo_harness links and this
	// port was verified against). Older tree-sitter releases (v0.24 and
	// v0.20.x) used 16*ERROR_COST_PER_SKIPPED_TREE; do NOT "correct" this back
	// to 16 — that would silently break oracle parity against the pinned
	// runtime.
	cRecoverMaxCostDifference = 18 * cErrCostPerSkippedTree
	// cErrorState is the C ERROR_STATE: the generated tables' recover row.
	cErrorState = StateID(0)
)

// cRecoverMaxReductionCandidateAttempts and cRecoverMaxMissingTokenTrials are
// Go-side backstop ceilings with NO exact C analog. Background
// (spore.2026-08-02.walnut-e.memory-exhaustion): a 4-byte erlang input and a
// 56-byte jsdoc input both drove cHandleError's missing-token search, via
// cDoAllPotentialReductions/cReductionCandidatesForAction, to clone the whole
// GSS stack (gssScratch, glr_gss.go) without bound — 2.4-2.6 GB RSS in
// seconds, unbounded for as long as host memory allows.
//
// C DOES intrinsically bound this same search, but by a mechanism that does
// not transplant onto this port's data structures without a larger, riskier
// redesign. ts_parser__reduce (parser.c:931-1046, called by both
// ts_parser__do_all_potential_reductions and, transitively, by
// ts_parser__handle_error's missing-token loop) checks every new stack
// version it is about to build against MAX_VERSION_COUNT (6) +
// MAX_VERSION_COUNT_OVERFLOW (4) BEFORE doing the parent-node/push work
// (parser.c:955-971), discarding the excess silently and cheaply. That cap is
// enforced against self->stack's version count, which is one persistent,
// shared structure for the parser's entire lifetime — a single reduce call's
// pop can yield several slices (several new versions at once) and the shared
// counter sees all of them, from every caller, in one place.
//
// This port's cDoAllPotentialReductions instead builds a fresh, per-call
// local `versions := []glrStack{start}` (parser_recover_c.go) with its own
// local cRecoverMaxVersionCount+1 cap (faithfully porting MAX_VERSION_COUNT
// itself — see cRecoverMaxVersionCount above), reset on every invocation.
// cHandleError's missing-token loop calls cDoAllPotentialReductions up to
// TokenCount-1 times (once per candidate missing symbol that survives the two
// cheap pre-filters, cTerminalNextState + stateHasLeadingReduceAction), and
// each of those nested calls gets its OWN fresh ~7-version allowance rather
// than sharing one global counter the way C's self->stack does. Faithfully
// porting C's exact mechanism would mean threading one shared, persistent
// version counter through cHandleError and every nested
// cDoAllPotentialReductions call it makes (including step 1's own call) and
// making it interact correctly with the existing local caps everywhere they
// already gate candidate selection (cAppendReductionVersion,
// cCollapseSamePopReductionCandidates, the cRecoverMaxVersionCount+1 break at
// the end of cDoAllPotentialReductions's loop) — exactly the kind of
// larger, not-yet-authorized redesign spec.recovery-strategy1-linearization-v2
// is already scoped to attempt, and getting the shared budget's SIZE wrong
// here risks silently eliminating a candidate an existing corpus fixture
// depends on (a tree-shape change, not just a slower parse).
//
// These two ceilings instead bound total WORK directly and fail exactly the
// way an ordinary, already-supported "search found nothing" outcome fails —
// no new ParseStopReason, no early exit that discards progress:
//   - cRecoverMaxReductionCandidateAttempts bounds total
//     cReductionCandidatesForAction calls (the expensive GSS-clone-and-reduce
//     step) within ONE cDoAllPotentialReductions call. Reaching it silently
//     truncates the remaining candidate exploration and returns whatever
//     versions/canShift were already accumulated with ParseStopNone — the
//     same graceful-truncation shape the function already uses at its
//     existing `len(versions) > cRecoverMaxVersionCount+1` break, and the
//     same "discard the excess, keep going" spirit as C's own per-slice
//     check above.
//   - cRecoverMaxMissingTokenTrials bounds total "expensive" trials (both
//     cheap pre-filters passed, so a clone + shift + nested
//     cDoAllPotentialReductions follows) across cHandleError's ENTIRE
//     missing-token search (every version times every candidate symbol, not
//     per version). Reaching it abandons the missing-token search exactly as
//     if every remaining symbol had failed — missingVersions stays nil, the
//     same outcome an ordinary exhaustive-but-unsuccessful search already
//     produces routinely, falling through to step 3's discontinuity-push +
//     ts_parser__recover-equivalent path.
//
// Sizing: the measured distribution sets this ceiling, not a tight algebraic
// bound on the loop shape below. A dedicated corpus pass (2026-08-02,
// consolidating PR #641 and its follow-up review) recorded
// CRecoverReductionCandidateAttemptsPeak — the largest candidateAttempts
// value any one cDoAllPotentialReductions call reached — across 719 real
// corpus files: 326 files never entered the search (peak 0), 243 files
// reached 1-9, 148 files reached 10-99, and only 2 files reached 100 or
// more. The highest value seen anywhere in that walk was 256: a 16x margin
// below this 4096 ceiling, with zero ceiling hits.
//
// A loop-shape argument motivated the original 4096 pick. Read it as rough
// orientation, not a derivation: it does not land on 4096 cleanly.
// cDoAllPotentialReductions's outer `for iter` loop shares ONE `iter` counter
// across the whole call, and the "reprocess in place" branch that can fire
// up to cRecoverMaxVersionCount (6) times is gated on that same shared
// counter (`iter < cRecoverMaxVersionCount`) — so those up-to-6 passes sit
// INSIDE the first 6 total passes, not stacked on top of them. Counting them
// separately anyway, alongside the up to cRecoverMaxVersionCount+1 (7)
// passes `v` can spend advancing across version slots before the loop stops
// accepting new ones, gives a loose upper bound of 6+7 = 13 outer passes,
// each visiting up to TokenCount (585, cobol, the largest of all 206 bundled
// grammars) reduce candidates: a naive worst case near 13*585 = 7,605 —
// already above this 4096 ceiling, so this loop shape alone does not justify
// 4096 either. The measured distribution above is the reason 4096 holds
// anyway: real recovery search terminates far short of any of these naive
// bounds on every file this corpus covers.
//
// Corpus-scale sweep, same basis (2026-08-02, consolidating PR #641): a
// 212-language, 12,717-file walk of real-world source under
// gotreesitter-corpora/corpus_sources — deliberately including files whose
// content does not match the language directory it was sampled from (e.g. a
// git diff fed to the bash grammar), the same "plausible adversarial-but-legal
// input shape" methodology the removed clone-cap mechanism used — found
// exactly one file that reaches this ceiling at all: that same content-
// mismatched diff file, which already failed to parse cleanly on origin/main
// (stopReason=memory_budget there, before this fix existed) and keeps
// failing to parse cleanly at every ceiling value tried, including 4x this
// one (its own search has no natural stopping point short of the ceiling,
// so it always lands at "ceiling+1", whatever the ceiling is — evidence the
// search is genuinely unbounded for this input, not evidence the ceiling is
// undersized). Every OTHER file across all 212 languages, plus every one of
// the 206 bundled languages' own smoke samples, stayed under this ceiling by
// a wide margin (missing-token-search peak across the whole walk: 259,
// against cRecoverMaxMissingTokenTrials's much larger headroom). Raising
// this ceiling does not help the one adversarial file — it only spends more
// of the four known memory-exhaustion witnesses' own bounded budget before
// the ceiling engages (measured: heap growth for the largest witness moved
// from ~130 MB at 4096 to ~750 MB at a 16384 trial), so it stays at the
// measured, validated value. Both ceilings stay small enough that reaching
// them costs a small, bounded amount of additional work (combined with the
// memory-budget feed alongside this one) well under a second, instead of an
// unbounded multi-GB climb.
//
// cRecoverMaxReductionCandidateAttempts is the ACTIVE mechanism: every
// witness and corpus-walk ceiling hit recorded against these two backstops
// (CRecoverReductionCandidateCeilingHits) came from this constant.
// cRecoverMaxMissingTokenTrials is a backstop that has never fired: across
// all four memory-exhaustion witnesses and the 719-file corpus pass above,
// CRecoverMissingTokenCeilingHits stayed 0 and
// CRecoverMissingTokenTrialAttemptsPeak (mtPeak) ranged 1-11 on the four
// witnesses and topped out at 120 across the corpus — a 68x margin below
// this 8192 ceiling. It stays in place: an unfired backstop still guards
// against a future input shape this codebase has not sampled, and the reduction-
// candidate ceiling above bounds a narrower scope (one
// cDoAllPotentialReductions call) than this one does (one entire
// cHandleError missing-token search). Do not read its silence as proof it is
// unreachable, and do not treat it as load-bearing for any witness fixed so
// far — it is not.
const (
	cRecoverMaxReductionCandidateAttempts = 4096
	cRecoverMaxMissingTokenTrials         = 8192
)

// errorCostCompetitionLanguage reports whether the faithful C error-recovery
// port is enabled for the active grammar. By default the gate requires
// parser.c-backed capability metadata, explicit parity certification, and
// conservative runtime table validation.
// GOT_C_RECOVERY=0 force-disables the gate (baseline A/B measurement);
// GOT_C_RECOVERY=all/1 or a comma-separated grammar list force-enables it for
// diagnostic sweeps when runtime table validation passes.
func errorCostCompetitionLanguage(lang *Language) bool {
	if lang == nil {
		return false
	}
	switch v := os.Getenv("GOT_C_RECOVERY"); v {
	case "":
	case "0":
		return false
	case "all", "1":
		return languageSupportsCRecoveryCostCompetition(lang)
	default:
		forced := false
		for _, n := range strings.Split(v, ",") {
			if strings.TrimSpace(n) == lang.Name {
				forced = true
				break
			}
		}
		if !forced {
			return false
		}
		return languageSupportsCRecoveryCostCompetition(lang)
	}
	if !lang.CRecoveryCostCompetitionCapable || !lang.CRecoveryCostCompetitionEnabledByDefault {
		return false
	}
	return languageSupportsCRecoveryCostCompetition(lang)
}

func cRecoveryGateReasonSlug(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return ""
	}
	var b strings.Builder
	prevUnderscore := false
	for _, r := range reason {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	out := b.String()
	return strings.Trim(out, "_")
}

func cRecoveryGateReason(lang *Language) string {
	if os.Getenv("GOT_C_RECOVERY") == "0" {
		return "disabled_by_got_c_recovery_0"
	}
	diag := DiagnoseCRecoveryGate(lang)
	if !diag.Supported {
		return cRecoveryGateReasonSlug(diag.Reason)
	}
	switch v := os.Getenv("GOT_C_RECOVERY"); v {
	case "all", "1":
		return ""
	case "":
		if lang == nil {
			return "nil_language"
		}
		if !lang.CRecoveryCostCompetitionCapable {
			return "not_c_recovery_capable"
		}
		if !lang.CRecoveryCostCompetitionEnabledByDefault {
			return "not_enabled_by_default"
		}
		return ""
	default:
		if lang != nil {
			for _, n := range strings.Split(v, ",") {
				if strings.TrimSpace(n) == lang.Name {
					return ""
				}
			}
		}
		return "not_enabled_by_got_c_recovery"
	}
}

func (p *Parser) cRecoveryGateReason() string {
	if p == nil {
		return "nil_parser"
	}
	if p.errorCostCompetition {
		if p.noTreeBenchmarkOnly {
			return "disabled_for_no_tree_benchmark"
		}
		if p.noTreeCheckpointBenchmarkOnly {
			return "disabled_for_no_tree_checkpoint_benchmark"
		}
		return ""
	}
	return cRecoveryGateReason(p.language)
}

// CRecoveryGateDiagnostics describes the runtime validation result for the
// C recovery-cost competition gate. Reason is empty when Supported is true;
// otherwise it names the first failed validation check.
type CRecoveryGateDiagnostics struct {
	Supported bool
	Reason    string

	StateCount       int
	SymbolCount      int
	TokenCount       int
	LexModeCount     int
	LexStateCount    int
	ParseActionCount int

	HasExternalScanner     bool
	ExternalSymbolCount    int
	ExternalTokenCount     int
	ExternalLexStateRows   int
	ExternalLexStateMinLen int
}

// cRecoveryGateCacheKey fingerprints every input DiagnoseCRecoveryGate reads:
// presence/lengths of the table surface plus the backing-array identity of
// each slice whose contents feed the validation. Language tables are immutable
// after decode; the only supported post-load mutations (external scanner and
// ExternalLexStates attach via AttachLanguageSupport / RegisterExternalScanner)
// swap whole slices or flip presence, which changes this key. Equal keys
// therefore imply an identical diagnosis, so the memo is answer-preserving.
//
// WARNING (test authors): the fingerprint captures each slice's backing-array
// identity (&ParseTable[0], &ParseActions[0], ...) plus its length — NOT its
// contents. An in-place cell/row poke (e.g. lang.ParseTable[i] = v, or mutating
// a ParseActionEntry's Actions in place) leaves both the base pointer and the
// length unchanged, so the key is unchanged and DiagnoseCRecoveryGate returns
// the STALE memoized diagnosis. To force a re-diagnosis, whole-swap the slice
// (lang.ParseTable = newSlice) or construct a fresh Language. This aliasing was
// flagged by review A as a memo-staleness footgun for gate tests.
type cRecoveryGateCacheKey struct {
	initialState       StateID
	stateCount         uint32
	symbolCount        uint32
	tokenCount         uint32
	externalTokenCount uint32

	symbolMetadataLen     int
	symbolNamesLen        int
	parseActionsLen       int
	lexModesLen           int
	lexStatesLen          int
	parseTableLen         int
	smallParseTableLen    int
	smallParseTableMapLen int
	largeStateGotosLen    int
	largeStateGotosHash   uint64

	hasExternalScanner     bool
	externalSymbolsLen     int
	externalLexStatesLen   int
	externalLexStateMinLen int

	parseTablePtr         *[]uint16
	smallParseTablePtr    *uint16
	smallParseTableMapPtr *uint32
	parseActionsPtr       *ParseActionEntry
	lexModesPtr           *LexMode
	externalSymbolsPtr    *Symbol
	externalLexStatesPtr  *[]bool
}

type cRecoveryGateCacheEntry struct {
	key  cRecoveryGateCacheKey
	diag CRecoveryGateDiagnostics
}

func cRecoveryGateCacheKeyFor(lang *Language) cRecoveryGateCacheKey {
	key := cRecoveryGateCacheKey{
		initialState:       lang.InitialState,
		stateCount:         lang.StateCount,
		symbolCount:        lang.SymbolCount,
		tokenCount:         lang.TokenCount,
		externalTokenCount: lang.ExternalTokenCount,

		symbolMetadataLen:     len(lang.SymbolMetadata),
		symbolNamesLen:        len(lang.SymbolNames),
		parseActionsLen:       len(lang.ParseActions),
		lexModesLen:           len(lang.LexModes),
		lexStatesLen:          len(lang.LexStates),
		parseTableLen:         len(lang.ParseTable),
		smallParseTableLen:    len(lang.SmallParseTable),
		smallParseTableMapLen: len(lang.SmallParseTableMap),
		largeStateGotosLen:    len(lang.LargeStateGotos),
		largeStateGotosHash:   largeStateGotosFingerprint(lang.LargeStateGotos),

		hasExternalScanner:     lang.ExternalScanner != nil,
		externalSymbolsLen:     len(lang.ExternalSymbols),
		externalLexStatesLen:   len(lang.ExternalLexStates),
		externalLexStateMinLen: externalLexStateMinLen(lang),
	}
	if len(lang.ParseTable) > 0 {
		key.parseTablePtr = &lang.ParseTable[0]
	}
	if len(lang.SmallParseTable) > 0 {
		key.smallParseTablePtr = &lang.SmallParseTable[0]
	}
	if len(lang.SmallParseTableMap) > 0 {
		key.smallParseTableMapPtr = &lang.SmallParseTableMap[0]
	}
	if len(lang.ParseActions) > 0 {
		key.parseActionsPtr = &lang.ParseActions[0]
	}
	if len(lang.LexModes) > 0 {
		key.lexModesPtr = &lang.LexModes[0]
	}
	if len(lang.ExternalSymbols) > 0 {
		key.externalSymbolsPtr = &lang.ExternalSymbols[0]
	}
	if len(lang.ExternalLexStates) > 0 {
		key.externalLexStatesPtr = &lang.ExternalLexStates[0]
	}
	return key
}

func largeStateGotosFingerprint(gotos map[uint64]StateID) uint64 {
	var h uint64
	for key, target := range gotos {
		v := uint64(target)
		h ^= key + 0x9e3779b97f4a7c15 + (v << 6) + (v >> 2)
		h ^= (key << 17) | (key >> 47)
	}
	return h
}

// DiagnoseCRecoveryGate validates the runtime table surface required by the
// faithful C recovery-cost competition path and returns the first failure.
//
// The full validation scans every parse-table row, so the result is memoized
// per Language behind an input fingerprint (cRecoveryGateCacheKey): callers on
// parse-adjacent paths (errorCostCompetitionLanguage via gap-token replay and
// token-source construction) would otherwise redo an O(tables) scan per call,
// which measurably dominated error-heavy parses.
func DiagnoseCRecoveryGate(lang *Language) CRecoveryGateDiagnostics {
	if lang == nil {
		return CRecoveryGateDiagnostics{Reason: "nil language"}
	}
	key := cRecoveryGateCacheKeyFor(lang)
	if e := lang.cRecoveryGateCache.Load(); e != nil && e.key == key {
		return e.diag
	}
	diag := diagnoseCRecoveryGateUncached(lang)
	lang.cRecoveryGateCache.Store(&cRecoveryGateCacheEntry{key: key, diag: diag})
	return diag
}

func diagnoseCRecoveryGateUncached(lang *Language) CRecoveryGateDiagnostics {
	if lang == nil {
		return CRecoveryGateDiagnostics{Reason: "nil language"}
	}
	d := CRecoveryGateDiagnostics{
		StateCount:             int(lang.StateCount),
		SymbolCount:            int(lang.SymbolCount),
		TokenCount:             int(lang.TokenCount),
		LexModeCount:           len(lang.LexModes),
		LexStateCount:          len(lang.LexStates),
		ParseActionCount:       len(lang.ParseActions),
		HasExternalScanner:     lang.ExternalScanner != nil,
		ExternalSymbolCount:    len(lang.ExternalSymbols),
		ExternalTokenCount:     int(lang.ExternalTokenCount),
		ExternalLexStateRows:   len(lang.ExternalLexStates),
		ExternalLexStateMinLen: externalLexStateMinLen(lang),
	}
	fail := func(reason string) CRecoveryGateDiagnostics {
		d.Reason = reason
		return d
	}
	if lang.InitialState != 1 {
		return fail("initial state is not 1")
	}
	if lang.StateCount == 0 {
		return fail("state count is zero")
	}
	if lang.SymbolCount == 0 {
		return fail("symbol count is zero")
	}
	if lang.TokenCount == 0 {
		return fail("token count is zero")
	}
	if len(lang.SymbolMetadata) < int(lang.SymbolCount) {
		return fail("symbol metadata is shorter than SymbolCount")
	}
	if len(lang.SymbolNames) < int(lang.SymbolCount) {
		return fail("symbol names are shorter than SymbolCount")
	}
	if len(lang.ParseActions) == 0 {
		return fail("parse actions are empty")
	}
	if len(lang.LexModes) == 0 {
		return fail("lex modes are empty")
	}
	if len(lang.LexStates) == 0 {
		return fail("lex states are empty")
	}
	ls := lang.LexModes[0].LexStateIndex()
	if ls == noLookaheadLexState {
		return fail("error-state lex mode has no lookahead lex state")
	}
	if int(ls) >= len(lang.LexStates) {
		return fail("error-state lex mode references missing lex state")
	}
	if int(lang.StateCount) > len(lang.LexModes) {
		return fail("state count exceeds lex mode count")
	}
	if langHasExternalRecoverySurface(lang) {
		if reason := externalLexStatesCRecoveryFailure(lang); reason != "" {
			return fail(reason)
		}
	}
	if reason := parseTablesCRecoveryFailure(lang); reason != "" {
		return fail(reason)
	}
	if reason := parseActionsCRecoveryFailure(lang); reason != "" {
		return fail(reason)
	}
	d.Supported = true
	return d
}

func languageSupportsCRecoveryCostCompetition(lang *Language) bool {
	return DiagnoseCRecoveryGate(lang).Supported
}

// CertifyCRecoveryCostCompetition validates a language's runtime recovery
// surface and updates the C recovery metadata.
//
// Capability means the tables satisfy the runtime gate. Default enablement is
// narrower: grammars with external recovery metadata are enabled only after an
// actual external scanner is attached and precise ExternalLexStates are
// available.
func CertifyCRecoveryCostCompetition(lang *Language) CRecoveryGateDiagnostics {
	diag := DiagnoseCRecoveryGate(lang)
	if lang == nil {
		return diag
	}
	lang.CRecoveryCostCompetitionEnabledByDefault = false
	if !diag.Supported {
		return diag
	}
	lang.CRecoveryCostCompetitionCapable = true
	lang.CRecoveryCostCompetitionEnabledByDefault = generatedCRecoveryDefaultSafe(lang)
	return diag
}

// CertifyGeneratedCRecoveryCostCompetition is kept for existing generated
// grammar call sites. New callers should use CertifyCRecoveryCostCompetition.
func CertifyGeneratedCRecoveryCostCompetition(lang *Language) CRecoveryGateDiagnostics {
	return CertifyCRecoveryCostCompetition(lang)
}

func generatedCRecoveryDefaultSafe(lang *Language) bool {
	if lang == nil {
		return false
	}
	if !langHasExternalRecoverySurface(lang) {
		return true
	}
	if cRecoveryDefaultOptOut(lang.Name) {
		return false
	}
	return lang.ExternalScanner != nil && externalLexStatesCRecoveryFailure(lang) == ""
}

func cRecoveryDefaultOptOut(name string) bool {
	switch name {
	case "cpp", "html", "javascript", "julia":
		return true
	default:
		return false
	}
}

func langHasExternalRecoverySurface(lang *Language) bool {
	return lang != nil && (lang.ExternalScanner != nil || len(lang.ExternalSymbols) > 0 || lang.ExternalTokenCount > 0)
}

func externalLexStateMinLen(lang *Language) int {
	if lang == nil || len(lang.ExternalLexStates) == 0 {
		return 0
	}
	min := len(lang.ExternalLexStates[0])
	for _, row := range lang.ExternalLexStates[1:] {
		if len(row) < min {
			min = len(row)
		}
	}
	return min
}

func externalLexStatesCRecoveryFailure(lang *Language) string {
	if lang == nil {
		return "nil language"
	}
	if len(lang.ExternalSymbols) == 0 {
		return "external scanner surface has no ExternalSymbols"
	}
	if len(lang.ExternalLexStates) == 0 {
		return "external scanner requires precise ExternalLexStates"
	}
	for _, sym := range lang.ExternalSymbols {
		if sym >= Symbol(lang.SymbolCount) {
			return "external symbol is outside SymbolCount"
		}
	}
	for _, row := range lang.ExternalLexStates {
		if len(row) < len(lang.ExternalSymbols) {
			return "ExternalLexStates row is shorter than ExternalSymbols"
		}
	}
	for state := 0; state < int(lang.StateCount) && state < len(lang.LexModes); state++ {
		if int(lang.LexModes[state].ExternalLexState) >= len(lang.ExternalLexStates) {
			return "lex mode references missing ExternalLexStates row"
		}
	}
	return ""
}

func parseTablesCRecoveryFailure(lang *Language) string {
	if lang == nil {
		return "nil language"
	}
	tokenCount := int(lang.TokenCount)
	symbolCount := int(lang.SymbolCount)
	stateCount := int(lang.StateCount)
	if tokenCount <= 0 {
		return "token count is zero"
	}
	if symbolCount <= 0 {
		return "symbol count is zero"
	}
	if stateCount <= 0 {
		return "state count is zero"
	}
	validateValue := func(sym int, val uint16) string {
		if val == 0 {
			return ""
		}
		if sym < 0 || sym >= symbolCount {
			return "parse table symbol is outside SymbolCount"
		}
		if sym < tokenCount {
			if int(val) >= len(lang.ParseActions) {
				return "parse table terminal action index is outside ParseActions"
			}
			return ""
		}
		if int(val) >= stateCount {
			return "parse table goto state is outside StateCount"
		}
		return ""
	}
	for _, row := range lang.ParseTable {
		for sym, val := range row {
			if reason := validateValue(sym, val); reason != "" {
				return reason
			}
		}
	}
	for key, target := range lang.LargeStateGotos {
		state := int(key >> 32)
		sym := int(uint32(key))
		if state < 0 || state >= stateCount {
			return "large-state goto source state is outside StateCount"
		}
		if sym < 0 || sym >= symbolCount {
			return "large-state goto symbol is outside SymbolCount"
		}
		if sym < tokenCount {
			return "large-state goto symbol is terminal"
		}
		if target == 0 || target >= StateID(stateCount) {
			return "large-state goto target state is outside StateCount"
		}
	}
	if len(lang.SmallParseTableMap) == 0 {
		return ""
	}
	table := lang.SmallParseTable
	if len(table) == 0 {
		return "small parse table map exists but table is empty"
	}
	for _, offset := range lang.SmallParseTableMap {
		pos := int(offset)
		if pos < 0 || pos >= len(table) {
			return "small parse table offset is outside table"
		}
		groupCount := int(table[pos])
		pos++
		for i := 0; i < groupCount; i++ {
			if pos+1 >= len(table) {
				return "small parse table group header is truncated"
			}
			val := table[pos]
			n := int(table[pos+1])
			pos += 2
			if n < 0 || pos+n > len(table) {
				return "small parse table group symbols are truncated"
			}
			for j := 0; j < n; j++ {
				if reason := validateValue(int(table[pos+j]), val); reason != "" {
					return reason
				}
			}
			pos += n
		}
	}
	return ""
}

func parseActionsCRecoveryFailure(lang *Language) string {
	if lang == nil {
		return "nil language"
	}
	for _, entry := range lang.ParseActions {
		for _, action := range entry.Actions {
			switch action.Type {
			case ParseActionShift, ParseActionRecover:
				if action.State >= StateID(lang.StateCount) {
					return "parse action target state is outside StateCount"
				}
			case ParseActionReduce:
				if action.Symbol >= Symbol(lang.SymbolCount) ||
					int(action.Symbol) >= len(lang.SymbolMetadata) ||
					int(action.Symbol) >= len(lang.SymbolNames) {
					return "reduce action symbol is outside symbol metadata"
				}
			case ParseActionAccept:
			default:
				return "parse action type is unsupported"
			}
		}
	}
	return ""
}

func (p *Parser) errorCostCompetitionEnabled() bool {
	return p != nil && p.errorCostCompetition && !p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly
}

// cRecoverAcquireToken is the token-acquisition front door for the faithful C
// recovery port. C lexes each stack version with its own state's lex mode, so
// a version absorbing at ERROR_STATE receives error-mode lookaheads
// (LexModes[0], most permissive, longest match — often hidden catch-all
// tokens spanning many bytes). The DFA token source honors that contract via
// SetParserState(0); custom TokenSources generally do not, which makes every
// downstream recovery decision (strategy-1 elections, absorption spans,
// error-region children) diverge from C. When every live stack is absorbing
// and the source cannot lex error mode itself, lex the token with the
// engine's own DFA from the group position and resynchronize the custom
// source afterwards (SkipToByte) once normal parsing resumes.
func (p *Parser) cRecoverAcquireToken(ts TokenSource, stacks []glrStack, source []byte) Token {
	workCountRecordLexerFrontDoor()
	// work-count-assembly: union-frontier election seam
	workCountRecordFrontierLexerElection()
	if !p.errorCostCompetitionEnabled() {
		return ts.Next()
	}
	p.cRecoverCustomSourceEligible = p.cRecoverCustomSourceEligibleFor(ts, source)
	if tok, ok := p.cRecoverInternalErrorModeToken(ts, stacks, source); ok {
		return tok
	}
	if p.cRecoverCustomResyncActive {
		p.cRecoverCustomResyncActive = false
		if skipper, ok := ts.(interface{ SkipToByte(uint32) Token }); ok {
			p.recordRecoveryScannerResync()
			return skipper.SkipToByte(p.cRecoverCustomResyncByte)
		}
	}
	return ts.Next()
}

// cRecoverCustomSourceEligibleFor reports whether the engine may substitute
// its own DFA lexing for this source during C recovery: a custom source that
// does not itself lex error mode, supports SkipToByte resynchronization, on a
// grammar whose tables carry the lex surface and no external scanner.
func (p *Parser) cRecoverCustomSourceEligibleFor(ts TokenSource, source []byte) bool {
	if len(source) == 0 {
		return false
	}
	if em, ok := ts.(errorModeLexingTokenSource); ok && em.lexesErrorModeAtErrorState() {
		return false
	}
	if _, ok := ts.(interface{ SkipToByte(uint32) Token }); !ok {
		return false
	}
	lang := p.language
	if lang == nil || len(lang.LexModes) == 0 || len(lang.LexStates) == 0 {
		return false
	}
	if lang.ExternalScanner != nil || len(lang.ExternalSymbols) > 0 {
		return false
	}
	ls := lang.LexModes[0].LexStateIndex()
	return ls != noLookaheadLexState && int(ls) < len(lang.LexStates)
}

// cRecoverResumeLookahead ports the error-mode half of ts_parser__lex for a
// paused lookahead. It accepts a direct DFA replacement only after that source
// re-lexes from the exact skipped-prefix start and reaches the same token end.
// Custom sources keep the existing internal-DFA path and deferred resync.
func (p *Parser) cRecoverResumeLookahead(ts TokenSource, source []byte, s *glrStack, tok Token, scratch *parserScratch) (Token, bool) {
	if p.cRecoverSharedTokenErrorModeLexed {
		return tok, false
	}
	if tok.Symbol == 0 || tok.Symbol == errorSymbol || tok.Missing || tok.NoLookahead {
		return tok, false
	}
	if s == nil || int(s.byteOffset) >= len(source) {
		return tok, false
	}
	if relexed, ok := p.cRecoverResumeDFALookahead(ts, source, s, tok, scratch); ok {
		return relexed, true
	}
	if !p.cRecoverCustomSourceEligible {
		return tok, false
	}
	lang := p.language
	state := s.top().state
	if int(state) >= len(lang.LexModes) {
		return tok, false
	}
	stateLS := lang.LexModes[state].LexStateIndex()
	errLS := lang.LexModes[0].LexStateIndex()
	if stateLS != noLookaheadLexState && int(stateLS) < len(lang.LexStates) {
		// C's fallback trigger: the state's own lex mode finds NO token at
		// the version position (whitespace skips permitted first). If it
		// lexes something — or cleanly reaches EOF — C's pause lookahead is a
		// normal-mode token and the source's token stands.
		probe := Lexer{
			states:          lang.LexStates,
			asciiTable:      lang.LexAsciiTable(),
			source:          source,
			pos:             int(s.byteOffset),
			immediateTokens: lang.ImmediateTokens,
			zeroWidthTokens: lang.ZeroWidthTokens,
		}
		if len(p.included) != 0 && lang.ExternalScanner == nil && len(lang.ExternalSymbols) == 0 {
			probe.setIncludedRanges(p.included)
		}
		stateLexFails := false
		for {
			if probe.pos >= len(source) {
				break
			}
			startPos := probe.pos
			t2, ok := probe.scan(uint32(stateLS), probe.pos, probe.row, probe.col)
			if !ok {
				stateLexFails = true
				break
			}
			if t2.Symbol == 0 {
				if probe.pos <= startPos {
					break
				}
				continue
			}
			// C shifts extra tokens (whitespace/comments) in place and lexes
			// again from the same state; only a non-extra token proves the
			// state's mode can lex here.
			if p.cRecoverStateShiftsExtra(state, t2.Symbol) {
				if probe.pos <= startPos {
					break
				}
				continue
			}
			break
		}
		if !stateLexFails {
			return tok, false
		}
	}
	pt := cStackPosPoint(s)
	lx := Lexer{
		states:              lang.LexStates,
		asciiTable:          lang.LexAsciiTable(),
		source:              source,
		pos:                 int(s.byteOffset),
		row:                 pt.Row,
		col:                 pt.Column,
		immediateTokens:     lang.ImmediateTokens,
		zeroWidthTokens:     lang.ZeroWidthTokens,
		errorRunLexState:    uint32(errLS),
		hasErrorRunLexState: true,
	}
	if len(p.included) != 0 && lang.ExternalScanner == nil && len(lang.ExternalSymbols) == 0 {
		lx.setIncludedRanges(p.included)
	}
	relexed := lx.NextWithErrorRuns(uint32(errLS))
	relexed.lexerErrorModeLexed = true
	if relexed.Symbol == tok.Symbol && relexed.StartByte == tok.StartByte && relexed.EndByte == tok.EndByte {
		return tok, false
	}
	p.cRecoverSharedTokenErrorModeLexed = true
	p.cRecoverCustomResyncActive = true
	p.cRecoverCustomResyncByte = relexed.EndByte
	p.recordRecoveryErrorModeToken()
	return relexed, true
}

// cRecoverResumeDFALookahead verifies C error-mode lexing on the active source.
// It commits the replacement only when the re-lexed error token has the exact
// original span and leaves the source at that token's end.
func (p *Parser) cRecoverResumeDFALookahead(ts TokenSource, source []byte, s *glrStack, tok Token, scratch *parserScratch) (Token, bool) {
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || scratch == nil || dts.lexer == nil || dts.language != p.language || !dts.cRecoveryEnabled ||
		tok.ExternalScannerToken || p.cSymbolVisible(tok.Symbol) ||
		p.lookupActionIndex(cErrorState, tok.Symbol) != 0 ||
		!dts.canRelexFromSkippedPrefix(tok) ||
		!tok.lexerSkippedPrefix || tok.lexerSkippedPrefixStart != s.byteOffset ||
		tok.StartByte <= s.byteOffset || tok.EndByte <= tok.StartByte ||
		int(tok.EndByte) > len(source) || len(dts.lexer.source) != len(source) {
		return Token{}, false
	}
	startPoint := cStackPosPoint(s)
	if startPoint.Row != tok.StartPoint.Row || startPoint.Column >= tok.StartPoint.Column {
		return Token{}, false
	}

	snapshot, retainedSnapshot := scratch.snapshotDFARelexState(dts)
	savedState := dts.state
	savedGLRStates := dts.glrStates
	restore := func() {
		snapshot.restore(dts)
		scratch.releaseDFARelexSnapshot(retainedSnapshot)
		dts.state = savedState
		dts.glrStates = savedGLRStates
	}
	dts.SetParserState(cErrorState)
	dts.SetGLRStates(nil)
	relexed, relexedOK := dts.relexRecoveryTokenFromSkippedPrefixInTransaction(tok, s.byteOffset, startPoint)
	if !relexedOK || !relexed.lexerErrorModeLexed || relexed.Symbol != errorSymbol || relexed.ExternalScannerToken ||
		relexed.StartByte != tok.StartByte || relexed.EndByte != tok.EndByte ||
		relexed.StartPoint != tok.StartPoint || relexed.EndPoint != tok.EndPoint ||
		dts.lexer.pos != int(relexed.EndByte) || dts.lexer.row != relexed.EndPoint.Row ||
		dts.lexer.col != relexed.EndPoint.Column {
		restore()
		return Token{}, false
	}
	scratch.releaseDFARelexSnapshot(retainedSnapshot)
	p.cRecoverSharedTokenErrorModeLexed = true
	p.recordRecoveryErrorModeToken()
	p.recordRecoveryScannerResync()
	return relexed, true
}

// cRecoverInternalErrorModeToken produces a C error-mode lookahead with the
// engine's own DFA when (a) the gate is on, (b) every live stack is an
// absorbing group member (C: the merged version is in ERROR_STATE, so C's
// lex uses the error mode), (c) the active source does not itself lex error
// mode, and (d) the source supports SkipToByte resynchronization. Grammars
// with an external scanner surface keep the shared source untouched — C
// consults the external scanner during error-mode lexing and the internal
// DFA cannot emulate that.
func (p *Parser) cRecoverInternalErrorModeToken(ts TokenSource, stacks []glrStack, source []byte) (Token, bool) {
	if len(source) == 0 || len(stacks) == 0 {
		return Token{}, false
	}
	if em, ok := ts.(errorModeLexingTokenSource); ok && em.lexesErrorModeAtErrorState() {
		return Token{}, false
	}
	if _, ok := ts.(interface{ SkipToByte(uint32) Token }); !ok {
		return Token{}, false
	}
	lang := p.language
	if lang == nil || len(lang.LexModes) == 0 || len(lang.LexStates) == 0 {
		return Token{}, false
	}
	if lang.ExternalScanner != nil || len(lang.ExternalSymbols) > 0 {
		return Token{}, false
	}
	ls := lang.LexModes[0].LexStateIndex()
	if ls == noLookaheadLexState || int(ls) >= len(lang.LexStates) {
		return Token{}, false
	}
	pos := uint32(0)
	var posPoint Point
	sawAbsorbing := false
	for i := range stacks {
		if stacks[i].dead || stacks[i].accepted {
			continue
		}
		if stacks[i].cRec == nil || stacks[i].top().state != cErrorState {
			// A live normally-parsing stack drives the lex in its own mode;
			// C's per-version independence degrades to the shared normal
			// token here (see cRecoverElectionLookaheadSymbol for the
			// election-side compensation).
			return Token{}, false
		}
		sawAbsorbing = true
		if stacks[i].byteOffset >= pos {
			pos = stacks[i].byteOffset
			posPoint = cStackPosPoint(&stacks[i])
		}
	}
	if !sawAbsorbing || int(pos) > len(source) {
		return Token{}, false
	}
	lx := Lexer{
		states:              lang.LexStates,
		asciiTable:          lang.LexAsciiTable(),
		source:              source,
		pos:                 int(pos),
		row:                 posPoint.Row,
		col:                 posPoint.Column,
		immediateTokens:     lang.ImmediateTokens,
		zeroWidthTokens:     lang.ZeroWidthTokens,
		errorRunLexState:    uint32(ls),
		hasErrorRunLexState: true,
	}
	if len(p.included) != 0 && lang.ExternalScanner == nil && len(lang.ExternalSymbols) == 0 {
		lx.setIncludedRanges(p.included)
	}
	tok := lx.NextWithErrorRuns(uint32(ls))
	tok.lexerErrorModeLexed = true
	// The shared token now carries the C error-mode identity; the election
	// can trust it directly.
	p.cRecoverSharedTokenErrorModeLexed = true
	p.cRecoverCustomResyncActive = true
	p.cRecoverCustomResyncByte = tok.EndByte
	p.recordRecoveryErrorModeToken()
	return tok, true
}

// cRecoverStateShiftsExtra reports whether sym's last action in state is an
// extra shift (C: the token lexes and shifts in place without leaving state).
func (p *Parser) cRecoverStateShiftsExtra(state StateID, sym Symbol) bool {
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return false
	}
	actions := p.language.ParseActions[idx].Actions
	if len(actions) == 0 {
		return false
	}
	last := actions[len(actions)-1]
	return last.Type == ParseActionShift && last.Extra
}

// cStackPosPoint mirrors cStackPosRow for full points: the end point of the
// topmost node-bearing entry.
func cStackPosPoint(s *glrStack) Point {
	if s == nil {
		return Point{}
	}
	if len(s.entries) > 0 {
		for i := len(s.entries) - 1; i >= 0; i-- {
			if stackEntryHasNode(s.entries[i]) {
				return stackEntryNodeEndPoint(s.entries[i])
			}
		}
		return Point{}
	}
	for gn := s.gss.head; gn != nil; gn = gn.prev {
		if stackEntryHasNode(gn.entry) {
			return stackEntryNodeEndPoint(gn.entry)
		}
	}
	return Point{}
}

// cStackSummaryEntry mirrors C StackSummaryEntry (stack.h): a (depth, state)
// pair with the stack position at that depth, recorded when entering the
// error state and consulted by ts_parser__recover strategy 1.
type cStackSummaryEntry struct {
	state    StateID
	posBytes uint32
	posRow   uint32
	depth    uint8 // cRecoverMaxSummaryDepth is 16.
}

// cRecoverElectionScratch owns the reusable cursors and generation-stamped
// state set used by strategy-1 recovery elections. Each member summary is
// already depth ordered by cRecordSummary, so one monotonically advancing
// cursor per member can enumerate the merged summary in depth-major,
// member-order-minor order without rescanning every summary at every depth.
//
// stateStamp is a per-depth set: beginDepth advances epoch, making every old
// stamp stale in O(1). The wrap path clears the stamps before reusing epoch 1.
// All fields are pointer-free slices, so retaining this scratch cannot retain
// parse trees or GSS nodes between parser-scratch pool uses.
type cRecoverElectionScratch struct {
	members    []int
	cursors    []int
	stateStamp []uint32
	epoch      uint32
}

func (s *cRecoverElectionScratch) prepare(stackCount, stateCount int) {
	s.members = s.members[:0]
	s.cursors = s.cursors[:0]
	if stackCount < 0 {
		stackCount = 0
	}
	if cap(s.members) < stackCount {
		s.members = make([]int, 0, stackCount)
	}
	if stateCount < 1 {
		stateCount = 1
	}
	if len(s.stateStamp) < stateCount {
		s.stateStamp = make([]uint32, stateCount)
	}
}

func (s *cRecoverElectionScratch) prepareMemberCursors(memberCount int) {
	if memberCount < 0 {
		memberCount = 0
	}
	if cap(s.cursors) < memberCount {
		s.cursors = make([]int, memberCount)
	} else {
		s.cursors = s.cursors[:memberCount]
		clear(s.cursors)
	}
}

func (s *cRecoverElectionScratch) allocatedBytes() int64 {
	if s == nil {
		return 0
	}
	return int64(cap(s.members)+cap(s.cursors))*int64(unsafe.Sizeof(int(0))) +
		int64(cap(s.stateStamp))*int64(unsafe.Sizeof(uint32(0)))
}

func (s *cRecoverElectionScratch) beginDepth(depth int) cRecoverElectionDepthIter {
	s.epoch++
	if s.epoch == 0 {
		clear(s.stateStamp)
		s.epoch = 1
	}
	return cRecoverElectionDepthIter{scratch: s, depth: depth}
}

func (s *cRecoverElectionScratch) resetForRelease() {
	const (
		maxRetainedMembers = maxStacksPerMergeKeyCeiling
		maxRetainedStates  = 64 * 1024
	)
	if cap(s.members) > maxRetainedMembers {
		s.members = nil
	} else {
		s.members = s.members[:0]
	}
	if cap(s.cursors) > maxRetainedMembers {
		s.cursors = nil
	} else {
		s.cursors = s.cursors[:0]
	}
	if cap(s.stateStamp) > maxRetainedStates {
		s.stateStamp = nil
		s.epoch = 0
	}
}

type cRecoverElectionDepthIter struct {
	scratch          *cRecoverElectionScratch
	depth            int
	member           int
	skippedSincePoll uint8
}

type cRecoverElectionIterStatus uint8

const (
	cRecoverElectionIterExhausted cRecoverElectionIterStatus = iota
	cRecoverElectionIterCandidate
	cRecoverElectionIterPoll
)

// next returns each first-encounter (depth,state) candidate together with the
// member that owns it. Member summaries are consumed monotonically, including
// duplicate and cErrorState entries, so exhausted cursors stay exhausted. A
// poll result after 64 otherwise-invisible skipped entries keeps cancellation
// and resource-budget checks bounded even when later members repeat a wide
// first member's entire summary.
func (it *cRecoverElectionDepthIter) next(stacks []glrStack) (int, cStackSummaryEntry, cRecoverElectionIterStatus) {
	if it == nil || it.scratch == nil {
		return 0, cStackSummaryEntry{}, cRecoverElectionIterExhausted
	}
	s := it.scratch
	for it.member < len(s.members) {
		memberOrder := it.member
		mi := s.members[memberOrder]
		if mi < 0 || mi >= len(stacks) || stacks[mi].cRec == nil {
			it.member++
			continue
		}
		summary := stacks[mi].cRec.summary
		cursor := s.cursors[memberOrder]
		for cursor < len(summary) && int(summary[cursor].depth) < it.depth {
			cursor++
		}
		for cursor < len(summary) && int(summary[cursor].depth) == it.depth {
			entry := summary[cursor]
			cursor++
			s.cursors[memberOrder] = cursor
			if entry.state == cErrorState {
				it.skippedSincePoll++
				if it.skippedSincePoll == 64 {
					it.skippedSincePoll = 0
					return 0, cStackSummaryEntry{}, cRecoverElectionIterPoll
				}
				continue
			}
			state := int(entry.state)
			if state < 0 || state >= len(s.stateStamp) {
				// The C-recovery capability gate proves every generated shift and
				// goto target is below Language.StateCount, and summaries contain
				// only those stack states. Keep unsupported hand-built tables safe
				// without allocating inside the election; their out-of-range state
				// simply cannot participate in dense dedupe.
				it.skippedSincePoll = 0
				return mi, entry, cRecoverElectionIterCandidate
			}
			if s.stateStamp[state] == s.epoch {
				it.skippedSincePoll++
				if it.skippedSincePoll == 64 {
					it.skippedSincePoll = 0
					return 0, cStackSummaryEntry{}, cRecoverElectionIterPoll
				}
				continue
			}
			s.stateStamp[state] = s.epoch
			it.skippedSincePoll = 0
			return mi, entry, cRecoverElectionIterCandidate
		}
		s.cursors[memberOrder] = cursor
		it.member++
	}
	return 0, cStackSummaryEntry{}, cRecoverElectionIterExhausted
}

// cRecGroup coordinates the absorbing stacks that map to C's ONE merged
// error-state version. C ts_parser__handle_error pushes the NULL discontinuity
// onto every do_all_potential_reductions result and merges them into a single
// multi-link version; this engine keeps one linear stack per path, so the
// group makes them act like the single C version: the per-token strategy-1
// summary scan (ts_parser__recover) is performed ONCE across the union of the
// members' summaries, in C's breadth-first record order.
type cRecGroup struct {
	// electionTokenStart/electionTokenSymbol identify the token for which the
	// group's strategy-1 election has already run.
	electionTokenStart  uint32
	electionTokenSymbol Symbol
	electionDone        bool
}

// cRecoverState marks a glrStack as being in the C error state (head at
// ERROR_STATE absorbing skipped tokens). nil == not in error.
type cRecoverState struct {
	summary []cStackSummaryEntry
	// group ties the absorbing stacks that represent the same C version.
	group *cRecGroup
	// openErr is the open error region node on the stack top (the C
	// error_repeat being accumulated). nil right after entering the error
	// state — the C "ERROR_STATE head with NULL subtree" shape, which costs an
	// extra ERROR_COST_PER_RECOVERY in ts_stack_error_cost.
	openErr *Node
	// groupOrder preserves the path order in its low bits. The reserved high
	// bit stores the recovery-leaf policy. Read the order through groupOrderValue.
	groupOrder uint32
	// extraRecoveries counts the additional error segments C opens while this
	// version keeps absorbing: an unlexable-run (ERROR-token) lookahead has no
	// action in the ERROR_STATE table row, so C pauses AGAIN mid-absorption,
	// and the condense→handle_error resume pushes a fresh NULL discontinuity
	// before the run is skipped (parser.c detect_error → handle_error). Each
	// such segment starts a new error_repeat nest whose own
	// ERROR_COST_PER_RECOVERY is baked into the stack subtree costs — and is
	// stripped again when a strategy-1 fork wraps the segments in a recovered
	// ERROR node (summarize_children excludes error_repeat child costs). The
	// engine's single flat open region charges its 500 once, so the extra
	// per-segment recoveries are tracked here; forks drop cRec and with it
	// these charges, exactly like C.
	extraRecoveries uint32
}

const (
	cRecoverGroupOrderLeafClearBit uint32 = 1 << 31
	cRecoverGroupOrderValueMask           = cRecoverGroupOrderLeafClearBit - 1
)

// cPackRecoverGroupOrder packs the path order and recovery-leaf policy. Current
// recovery ceilings keep vi below the reserved bit. The uint64 input checks a
// future larger value before narrowing. Invalid input saturates and clears policy.
func cPackRecoverGroupOrder(order uint64, clearOrdinaryLeafErrors bool) uint32 {
	if order > uint64(cRecoverGroupOrderValueMask) {
		return cRecoverGroupOrderValueMask
	}
	packed := uint32(order)
	if clearOrdinaryLeafErrors {
		return packed | cRecoverGroupOrderLeafClearBit
	}
	return packed
}

func (r *cRecoverState) groupOrderValue() uint32 {
	if r == nil {
		return 0
	}
	return r.groupOrder & cRecoverGroupOrderValueMask
}

func (r *cRecoverState) clearsOrdinaryLeafErrors() bool {
	return r != nil && r.groupOrder&cRecoverGroupOrderLeafClearBit != 0
}

var cRecoverStateCloneObserver func()

func (r *cRecoverState) clone() *cRecoverState {
	if r == nil {
		return nil
	}
	if observer := cRecoverStateCloneObserver; observer != nil {
		observer()
	}
	cp := &cRecoverState{
		openErr:         r.openErr,
		group:           r.group,
		groupOrder:      r.groupOrder,
		extraRecoveries: r.extraRecoveries,
		summary:         r.summary,
	}
	return cp
}

// cRecoverOutcome describes what the gated recovery did with the current
// stack for the current token.
type cRecoverOutcome int

const (
	// cRecFallthrough: not handled — caller continues normal dispatch.
	cRecFallthrough cRecoverOutcome = iota
	// cRecConsumed: token absorbed into the error region (or recover_eof
	// accepted); the stack is done with this token.
	cRecConsumed
	// cRecHalted: the stack was halted (clearly worse than another version).
	cRecHalted
)

// ---------------------------------------------------------------------------
// Error cost (subtree.c ts_subtree_error_cost / summarize_children port)
// ---------------------------------------------------------------------------

func cSymbolVisibleLang(lang *Language, sym Symbol) bool {
	if sym == errorSymbol {
		return true
	}
	if lang == nil || int(sym) >= len(lang.SymbolMetadata) {
		return false
	}
	return lang.SymbolMetadata[sym].Visible
}

func (p *Parser) cSymbolVisible(sym Symbol) bool {
	if p == nil {
		return false
	}
	return cSymbolVisibleLang(p.language, sym)
}

// cNodeVisibleChildCount mirrors SubtreeHeapData.visible_child_count:
// direct children that are visible, plus the visible children of invisible
// internal children.
func cNodeVisibleChildCountLang(lang *Language, n *Node) int {
	if n == nil {
		return 0
	}
	count := 0
	for _, c := range n.children {
		if c == nil {
			continue
		}
		if cSymbolVisibleLang(lang, c.symbol) {
			count++
		} else if len(c.children) > 0 {
			count += cNodeVisibleChildCountLang(lang, c)
		}
	}
	return count
}

// cNodeVisibleSubtreeCount mirrors stack.c stack__subtree_node_count: the
// visible descendant count (plus the node itself when visible), used for
// node-count-since-error bookkeeping. Memoized per (node, equivVersion) when
// the gate is on — C keeps this in the subtree header too.
func (p *Parser) cNodeVisibleSubtreeCount(n *Node) int {
	if n == nil {
		return 0
	}
	if p != nil && len(p.cNodeMemoCache) != 0 {
		slot := p.cNodeMemoPrimaryHit(n)
		if slot == nil {
			slot = p.cNodeMemoSlot(n)
		}
		if slot.hasVis && slot.ver == n.equivVersion {
			return int(slot.visCount)
		}
	}
	count := 0
	if p.cSymbolVisible(n.symbol) {
		count++
	}
	for _, c := range n.children {
		count += p.cNodeVisibleSubtreeCount(c)
	}
	if p != nil && len(p.cNodeMemoCache) != 0 {
		// Re-fetch the slot: the recursive calls above may have evicted n's
		// slot (a child's pointer hashing into the same 2-way set), so the
		// pointer captured before recursing could now be stale.
		slot := p.cNodeMemoPrimaryHit(n)
		if slot == nil {
			slot = p.cNodeMemoSlot(n)
		}
		if slot.ver != n.equivVersion {
			*slot = cNodeMemoCacheEntry{
				node:  uintptr(unsafe.Pointer(n)),
				ver:   n.equivVersion,
				epoch: p.cNodeMemoEpoch,
			}
		}
		if uint64(count) <= uint64(^uint32(0)) {
			slot.visCount = uint32(count)
			slot.hasVis = true
		}
	}
	return count
}

// cNodeErrorCost ports ts_subtree_error_cost + the error-cost part of
// ts_subtree_summarize_children. Go has no error_repeat chain: ERROR nodes
// hold absorbed children directly, so the per-ERROR recovery cost is charged
// once per ERROR node. Go nodes carry no padding either; an ERROR node's span
// already starts at its first real token, matching the C "size excludes
// padding" rule for the common case.
//
// Childless ERROR children are C's unlexable-run ERROR *tokens*
// (ts_subtree_new_error leaves): their subtree error_cost is 0 in C — the
// bytes are charged once via the enclosing error_repeat/ERROR node's size —
// so their own-node cost must not be added into a parent. They DO count as
// one skipped tree: C wraps each in an error_repeat whose visible_child_count
// (which has no childless-error exclusion) feeds the enclosing node's
// ERROR_COST_PER_SKIPPED_TREE bonus (subtree.c summarize_children). The
// flattened engine representation charges the +100 directly. A childless
// ERROR node reached as a stack entry (an open region whose only content is
// invisible) keeps its own 500+bytes charge, matching the C error_repeat
// wrapper around an invisible token.
func cNodeErrorCostLang(lang *Language, n *Node) uint32 {
	if n == nil {
		return 0
	}
	if n.isMissing() && len(n.children) == 0 {
		return cErrCostPerMissingTree + cErrCostPerRecovery
	}
	var cost uint32
	for _, c := range n.children {
		if c != nil && c.symbol == errorSymbol && len(c.children) == 0 {
			// C ERROR leaf: subtree error_cost 0.
			continue
		}
		cost += cNodeErrorCostLang(lang, c)
	}
	if n.symbol == errorSymbol {
		for _, c := range n.children {
			if c == nil || c.isExtra() {
				continue
			}
			if cSymbolVisibleLang(lang, c.symbol) {
				cost += cErrCostPerSkippedTree
			} else if len(c.children) > 0 {
				cost += cErrCostPerSkippedTree * uint32(cNodeVisibleChildCountLang(lang, c))
			}
		}
		bytes := uint32(0)
		rows := uint32(0)
		if n.endByte > n.startByte {
			bytes = n.endByte - n.startByte
		}
		if n.endPoint.Row > n.startPoint.Row {
			rows = n.endPoint.Row - n.startPoint.Row
		}
		cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
	}
	return cost
}

func cNodeErrorCostLangWithScratch(scratch *glrMergeScratch, lang *Language, n *Node) uint32 {
	if n == nil {
		return 0
	}
	if scratch == nil {
		return cNodeErrorCostLang(lang, n)
	}
	if parser := scratch.cErrorCostParser; parser != nil && parser.language == lang {
		return parser.cNodeErrorCost(n)
	}
	if cached, ok := scratch.cErrorCost[n]; ok && cached.ver == n.equivVersion {
		return cached.cost
	}
	if scratch.cErrorCost == nil {
		scratch.cErrorCost = make(map[*Node]glrCErrorCostEntry)
	}
	if n.isMissing() && len(n.children) == 0 {
		cost := uint32(cErrCostPerMissingTree + cErrCostPerRecovery)
		scratch.cErrorCost[n] = glrCErrorCostEntry{ver: n.equivVersion, cost: cost}
		return cost
	}
	var cost uint32
	for _, c := range n.children {
		if c != nil && c.symbol == errorSymbol && len(c.children) == 0 {
			// C ERROR leaf: subtree error_cost 0 (see cNodeErrorCostLang).
			continue
		}
		cost += cNodeErrorCostLangWithScratch(scratch, lang, c)
	}
	if n.symbol == errorSymbol {
		for _, c := range n.children {
			if c == nil || c.isExtra() {
				continue
			}
			if cSymbolVisibleLang(lang, c.symbol) {
				cost += cErrCostPerSkippedTree
			} else if len(c.children) > 0 {
				cost += cErrCostPerSkippedTree * uint32(cNodeVisibleChildCountLang(lang, c))
			}
		}
		bytes := uint32(0)
		rows := uint32(0)
		if n.endByte > n.startByte {
			bytes = n.endByte - n.startByte
		}
		if n.endPoint.Row > n.startPoint.Row {
			rows = n.endPoint.Row - n.startPoint.Row
		}
		cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
	}
	scratch.cErrorCost[n] = glrCErrorCostEntry{ver: n.equivVersion, cost: cost}
	return cost
}

// cNodeMemoCacheEntry caches the gated recovery's per-subtree aggregates, the
// engine analogue of C SubtreeHeapData.error_cost / node counts computed once
// per subtree in ts_subtree_summarize_children. Finished subtrees never
// mutate during a parse; the open error region is bumped (equivVersion) on
// every absorb, invalidating its entry. Without the memo every
// condense/competition step rewalks whole accumulated subtrees per token,
// which is O(n^2) on large gated files.
//
// This is one slot of a pointer-keyed 2-way set-associative cache (same
// layout discipline as glrShapePrefixCacheEntry / glrNodeEquivCacheEntry in
// glr.go): profiling error-recovery-heavy parses (kdl/uxntal-class
// fleet-tail cliffs, see cRecoverStrategy1Election/cHandleError/
// cCondenseAndResume) showed runtime.mapaccess2_fast64 as the single hottest
// leaf in the whole parse, driven almost entirely by the two memo lookups
// below (cNodeErrorCost, cNodeVisibleSubtreeCount) previously backed by a
// map[*Node]cNodeMemoEntry. A miss here is always safe: every caller falls
// through to a full recompute over n's children, so an approximate
// (evictable) cache changes only how often the answer is recomputed, never
// what the answer is.
type cNodeMemoCacheEntry struct {
	node     uintptr
	ver      uint32
	cost     uint32
	visCount uint32
	epoch    uint16
	hasCost  bool
	hasVis   bool
}

const (
	// cNodeMemoCacheInitialSize is what every C-recovery-capable parse
	// allocates up front (parseInternal), matching the previous
	// map[*Node]cNodeMemoEntry's practical footprint (make(..., 256) measured
	// ~3.8KiB): cCompareVersions-driven cost competition participates in
	// ordinary GLR disambiguation on well-formed input too (see the doc
	// comment above cVersionStatus), so this path is warm on every capable
	// parse, clean or not, and must stay cheap for the common case.
	cNodeMemoCacheInitialSize = 128
	// cNodeMemoCacheSize is what the cache grows to either (a) the first time
	// a lineage actually enters C error handling (cHandleError /
	// cRecoverDispatchInError setting crecoveryEnteredErrorState), or (b) the
	// first time THIS parse's own memo-set contention crosses
	// cNodeMemoThrashGrowThreshold (see cNodeMemoSlot) -- matching the sizing
	// of the merge-scratch pointer-keyed caches (glrShapePrefixCacheSize et
	// al.): 8192 sets, 2-way each. Growing loses whatever was cached at the
	// smaller size, which only costs a recompute (see cNodeMemoSlot) -- never
	// a wrong answer.
	cNodeMemoCacheSize = 16384
	// cNodeMemoRecoveryCacheSize is a temporary second tier for recovery
	// parses that continue to collide after the standard cache grows. The
	// packed 24-byte entry makes this active tier 3 MiB on 64-bit systems.
	// The retained 384 KiB standard tier makes total storage 3.375 MiB before
	// slice overhead.
	cNodeMemoRecoveryCacheSize = 131072
	// cNodeMemoRecoveryThrashGrowThreshold requires one collision per standard
	// cache entry before the parser allocates the temporary recovery tier.
	cNodeMemoRecoveryThrashGrowThreshold = cNodeMemoCacheSize
	// cNodeMemoThrashGrowThreshold is the number of genuine 2-way-set
	// collisions -- the primary way already holding a live current-epoch
	// entry for another node, see cNodeMemoSlot's contention branch; the
	// victim way may be relocated into rather than evicted, but it still
	// counts -- THIS parse must observe against the small (128-entry /
	// 64-set) cache before cNodeMemoSlot grows it to cNodeMemoCacheSize on
	// its own, independent of whether cHandleError has ever run.
	//
	// Provenance (issue #380/#388, measured on this box, linux/amd64, via
	// (*Parser).DebugCNodeMemoCacheStats instrumenting this exact counter):
	//   - The #388 repro (issue380_incremental_insert_slowdown_repro_test.go /
	//     issue380_incremental_insert_cache_warmup_nondeterminism_repro_test.go
	//     TestReproOrganicWarmVsCold's COLD arm: this repo's own tree.go,
	//     ~137KB, single-char insert at offset 200, fresh Parser, 128-entry/
	//     64-set cache never grown by cHandleError) accumulates ~235,000-
	//     245,000 evictions over the full cold parse (cNodeErrorCost /
	//     cNodeVisibleSubtreeCount's recursive fallback on a small, thrashing
	//     cache is what made the cold parse ~4x the warm one on this box --
	//     see the repro files' doc comments). With adaptive growth wired at
	//     threshold 512, the SAME scenario's wall time drops from ~427-514ms
	//     to ~97-136ms across repeated fresh-Parser runs, matching the
	//     already-warm baseline (~96-117ms) to within ~1.0-1.15x -- i.e. the
	//     threshold-512 grow fires early enough in the parse (a threshold
	//     four orders of magnitude below the cold parse's ~240K-eviction
	//     total) to capture essentially all of the available speedup. 512 (8x
	//     the 64-set capacity, i.e. each set forced to evict roughly 8 times
	//     on average) was chosen as a round number comfortably inside that
	//     margin, not tuned to the last eviction.
	//   - Clean, non-pathological parses observe EXACTLY 0 evictions end to
	//     end, not just "below the threshold": a single fresh-Parser full
	//     Parse() of this repo's own tree.go/parser.go (312KB)/
	//     parser_recover_c.go/glr.go/incremental.go/lexer.go/query.go, and of
	//     the benchmark_family_test.go corpus (go/js/ts/tsx/python/css/rust/
	//     java/kotlin/swift/c/json/html/fortran/bash/ruby/csharp at their
	//     default generated sizes), never enters cNodeMemoSlot's eviction
	//     branch at all -- see TestCNodeMemoCacheStaysSmallForCleanFullParse
	//     (issue380_cnode_memo_cache_growth_determinism_test.go). Property (b)
	//     (typical files must not grow to 16K) therefore holds with several
	//     orders of magnitude of margin, not merely "below 512".
	cNodeMemoThrashGrowThreshold = 512
)

func cNodeMemoCacheIndex(p uintptr, setCount int) int {
	// Nodes come from aligned slabs. Remove the alignment zeros, then fold
	// slab-address bits into the set index without multiplication.
	h := uint64(p / unsafe.Alignof(Node{}))
	h ^= h >> 17
	h ^= h >> 9
	return int(h&uint64(setCount-1)) << 1
}

func cNodeMemoCacheBytesForEntries(entries int) uint64 {
	if entries <= 0 {
		return 0
	}
	return uint64(entries) * uint64(unsafe.Sizeof(cNodeMemoCacheEntry{}))
}

func recoveryNodeMemoTierForEntries(entries int) RecoveryNodeMemoTier {
	switch {
	case entries <= 0:
		return RecoveryNodeMemoTierNone
	case entries <= cNodeMemoCacheInitialSize:
		return RecoveryNodeMemoTierInitial
	case entries <= cNodeMemoCacheSize:
		return RecoveryNodeMemoTierStandard
	default:
		return RecoveryNodeMemoTierTemporary
	}
}

func (p *Parser) cNodeMemoCollisionCount() uint64 {
	if p == nil || p.forestDeclineMemo == nil {
		return 0
	}
	return p.forestDeclineMemo.cNodeMemoCollisions
}

func (p *Parser) recordCNodeMemoCollision() {
	if cold := p.ensureParserColdState(); cold != nil {
		cold.cNodeMemoCollisions++
	}
}

// growCNodeMemoCacheTo replaces the active recovery memo with a larger empty
// cache. A miss always recomputes the exact result, so dropped entries are safe.
//
// The temporary tier preserves the standard cache. Parse-operation cleanup
// restores that cache and releases the larger backing array.
func (p *Parser) growCNodeMemoCacheTo(target int) {
	if p == nil || target <= len(p.cNodeMemoCache) {
		return
	}
	if target > cNodeMemoRecoveryCacheSize {
		target = cNodeMemoRecoveryCacheSize
	}
	if target > cNodeMemoCacheSize {
		cold := p.ensureParserColdState()
		if cold != nil && cold.cNodeMemoRetainedCache == nil {
			retained := p.cNodeMemoCache
			// An operation starts with a deterministic 128-entry view. Keep
			// the full standard slab when its backing array is already warm.
			if cap(retained) >= cNodeMemoCacheSize {
				retained = retained[:cNodeMemoCacheSize]
			}
			cold.cNodeMemoRetainedCache = retained
		}
	}
	p.cNodeMemoCache = make([]cNodeMemoCacheEntry, target)
	if target > cNodeMemoCacheSize {
		p.activateCNodeMemoMergeSharing()
	}
	p.cNodeMemoThrash = 0
	tier := recoveryNodeMemoTierForEntries(target)
	if tier > p.cNodeMemoPeakTier {
		p.cNodeMemoPeakTier = tier
	}
	if tier > p.cNodeMemoOperationPeakTier {
		p.cNodeMemoOperationPeakTier = tier
	}
}

func (p *Parser) growCNodeMemoCache() {
	p.growCNodeMemoCacheTo(cNodeMemoCacheSize)
}

// activateCNodeMemoMergeSharing moves merge cost comparisons to the parser's
// bounded memo. Discarded map entries recompute exactly on a cache miss.
func (p *Parser) activateCNodeMemoMergeSharing() {
	if p == nil || p.mergeScratch == nil {
		return
	}
	p.mergeScratch.cErrorCostParser = p
	p.mergeScratch.cErrorCost = nil
}

// finishCNodeMemoParse releases the temporary recovery tier. It restores the
// standard cache without an allocation and keeps the parser's warm behavior.
func (p *Parser) finishCNodeMemoParse() {
	if p == nil || p.forestDeclineMemo == nil || p.forestDeclineMemo.cNodeMemoRetainedCache == nil {
		return
	}
	p.cNodeMemoCache = p.forestDeclineMemo.cNodeMemoRetainedCache
	p.forestDeclineMemo.cNodeMemoRetainedCache = nil
}

// DebugCNodeMemoCacheStats reports the active cache size and the current tier's
// collision count. Tree.RecoveryNodeMemoRuntime reports the peak tier and the
// total collision count for a returned tree. This method is safe on a nil
// receiver.
func (p *Parser) DebugCNodeMemoCacheStats() (cacheLen int, thrash uint32) {
	if p == nil {
		return 0, 0
	}
	return len(p.cNodeMemoCache), p.cNodeMemoThrash
}

// DebugCNodeMemoOperationStats reports the peak cache entries, peak bytes,
// and collisions from the most recent recovery-capable outer parse operation.
// A compact-route return does not start such an operation.
func (p *Parser) DebugCNodeMemoOperationStats() (peakEntries int, peakBytes uint64, collisions uint64) {
	if p == nil {
		return 0, 0, 0
	}
	peakEntries = int(p.cNodeMemoOperationPeakTier.Entries())
	return peakEntries,
		cNodeMemoCacheBytesForEntries(peakEntries),
		p.cNodeMemoCollisionCount()
}

// beginCNodeMemoEpoch invalidates every recovery memo entry in O(1). Epoch
// zero is reserved for never-valid entries. On uint16 wrap, clear the cache
// once before reusing epoch 1 so no surviving entry can alias the new epoch.
func (p *Parser) beginCNodeMemoEpoch() {
	if p == nil || len(p.cNodeMemoCache) == 0 {
		return
	}
	if p.cNodeMemoEpoch == ^uint16(0) {
		clear(p.cNodeMemoCache)
		p.cNodeMemoEpoch = 0
	}
	p.cNodeMemoEpoch++
}

// cNodeMemoPrimaryHit checks the primary cache way. Callers must provide a
// non-nil parser and node with a provisioned cache.
func (p *Parser) cNodeMemoPrimaryHit(n *Node) *cNodeMemoCacheEntry {
	ptr := uintptr(unsafe.Pointer(n))
	idx := cNodeMemoCacheIndex(ptr, len(p.cNodeMemoCache)>>1)
	primary := &p.cNodeMemoCache[idx]
	if primary.epoch == p.cNodeMemoEpoch && primary.node == ptr {
		return primary
	}
	return nil
}

// cNodeMemoSlot returns the writable 2-way set-associative slot for node n.
// A current-epoch miss evicts the primary into the victim half; stale-epoch
// occupants are ignored. This mirrors map[*Node]cNodeMemoEntry lookup
// semantics within an epoch: an existing entry for n keeps its stored
// version/cost/vis fields until the caller explicitly overwrites them; a
// newly-resident node starts zeroed (matching a fresh map key's zero value).
// Returns nil only when the cache is unprovisioned (recovery gate off) or n/p
// is nil.
func (p *Parser) cNodeMemoSlot(n *Node) *cNodeMemoCacheEntry {
	if p == nil || n == nil || len(p.cNodeMemoCache) == 0 {
		return nil
	}
	if p.cNodeMemoEpoch == 0 {
		p.beginCNodeMemoEpoch()
	}
	setCount := len(p.cNodeMemoCache) >> 1
	ptr := uintptr(unsafe.Pointer(n))
	idx := cNodeMemoCacheIndex(ptr, setCount)
	primary := &p.cNodeMemoCache[idx]
	if primary.epoch == p.cNodeMemoEpoch && primary.node == ptr {
		return primary
	}
	victim := &p.cNodeMemoCache[idx+1]
	if victim.epoch == p.cNodeMemoEpoch && victim.node == ptr {
		*primary, *victim = *victim, *primary
		return primary
	}
	if primary.epoch == p.cNodeMemoEpoch {
		// The primary way already holds a live current-epoch entry for
		// another node: writing n here relocates primary's occupant down
		// into victim (the victim way may have been empty -- a relocation,
		// not a loss -- or itself occupied, in which case its own occupant
		// is discarded outright). Either way this is THIS PARSE's own
		// genuine capacity contention on the set (as opposed to "n simply
		// has never been looked up before"), and it is exactly what degrades
		// cNodeErrorCost/cNodeVisibleSubtreeCount from O(1) amortized to a
		// cascading O(depth)-or-worse recompute on a large/wide parse (see
		// cNodeMemoThrashGrowThreshold's doc comment, issue #380/#388).
		// Track it and adaptively grow this parser's cache once THIS parse's
		// own contention crosses the measured threshold -- independent of
		// whether cHandleError has ever run on this Parser instance, so the
		// same (source, edit) always takes the same growth path regardless of
		// the instance's unrelated history.
		*victim = *primary
		p.cNodeMemoThrash++
		p.recordCNodeMemoCollision()
		growTarget := 0
		growThreshold := uint32(0)
		switch {
		case len(p.cNodeMemoCache) < cNodeMemoCacheSize:
			growTarget = cNodeMemoCacheSize
			growThreshold = cNodeMemoThrashGrowThreshold
		case p.crecoveryEnteredErrorState && len(p.cNodeMemoCache) < cNodeMemoRecoveryCacheSize:
			growTarget = cNodeMemoRecoveryCacheSize
			growThreshold = cNodeMemoRecoveryThrashGrowThreshold
		}
		if growTarget != 0 && p.cNodeMemoThrash >= growThreshold {
			p.growCNodeMemoCacheTo(growTarget)
			// Re-resolve idx/primary against the freshly-grown cache: the
			// setCount above is now stale, and growCNodeMemoCache reset every
			// slot to unwritten (epoch 0), so this is guaranteed to be a
			// (safe -- see growCNodeMemoCache's doc) miss.
			setCount = len(p.cNodeMemoCache) >> 1
			idx = cNodeMemoCacheIndex(ptr, setCount)
			primary = &p.cNodeMemoCache[idx]
		}
	}
	*primary = cNodeMemoCacheEntry{node: ptr, epoch: p.cNodeMemoEpoch}
	return primary
}

// ---------------------------------------------------------------------------
// Open-error-region incremental cost maintenance
//
// While an error region is OPEN, every absorb appends children to the same
// ERROR node and bumps its equivVersion, invalidating the (node, equivVersion)
// memo entries above. Re-deriving the region's aggregates then costs
// O(len(children)) per lookup, and the cost competitions / condense keys /
// merge comparisons look the region up several times per token — O(n^2)
// across the region (the c_sharp Bicep witness class).
//
// C never pays this: stack.c stack_node_new maintains error_cost as a
// monotonic accumulator on push ("node->error_cost = previous_node->error_cost;
// ... node->error_cost += ts_subtree_error_cost(subtree);", stack.c:163-172 in
// the pinned tree-sitter), and ts_stack_error_cost (stack.c:493) reads it in
// O(1). The helpers below restore that shape for the port: an absorb ADDS its
// known delta (subtree cost + ERROR_COST_PER_SKIPPED_TREE charges + span
// growth) to the region's memoized aggregates instead of leaving them stale.
// Every write is answer-preserving: it stores exactly what a full walk at the
// new version would compute (assertable under
// GOT_DEBUG_RECOVERY_INCREMENTAL_COST=1), and any mutation path that does NOT
// go through these helpers simply leaves the entry stale, falling back to
// today's full recompute.
// ---------------------------------------------------------------------------

// debugRecoveryIncrementalCost enables the env-gated incremental-vs-full-walk
// assertion (GOT_DEBUG_RECOVERY_INCREMENTAL_COST=1), mirroring the
// GOT_DEBUG_RECOVERY_CYCLES pattern: zero overhead when unset, loud stderr
// (budgeted) plus counters when a divergence is found.
var debugRecoveryIncrementalCost = os.Getenv("GOT_DEBUG_RECOVERY_INCREMENTAL_COST") == "1"

var debugRecoveryIncrementalCostReportsLeft = 8
var debugRecoveryIncrementalCostDivergences uint64
var debugRecoveryIncrementalCostChecks uint64

// debugRecoveryIncrementalCostReport wires the otherwise write-only
// checks/divergences counters into observable output: a one-line summary of the
// env-gated incremental-vs-full aggregate assertions
// (GOT_DEBUG_RECOVERY_INCREMENTAL_COST=1), covering both the aggCost and the new
// aggVis walks. Called once at parse end (parseInternal). The counters are
// process-cumulative (they back a process-global report budget), so a
// single-parse witness run shows exactly that parse's totals; checks>0 confirms
// the assertions actually ran and divergences==0 confirms every memoized
// aggregate matched its full walk. Per-divergence detail (budgeted to
// debugRecoveryIncrementalCostReportsLeft) is printed at the divergence sites.
func debugRecoveryIncrementalCostReport() {
	if !debugRecoveryIncrementalCost || debugRecoveryIncrementalCostChecks == 0 {
		return
	}
	fmt.Fprintf(os.Stderr,
		"RECOVERY-INCREMENTAL-COST summary: checks=%d divergences=%d\n",
		debugRecoveryIncrementalCostChecks, debugRecoveryIncrementalCostDivergences)
}

// cErrRegionSpanCost returns the span-derived portion of an ERROR node's own
// error cost (the ERROR_COST_PER_SKIPPED_CHAR / _LINE terms), excluding the
// per-recovery constant and all child-derived terms.
func cErrRegionSpanCost(n *Node) uint32 {
	var bytes, rows uint32
	if n.endByte > n.startByte {
		bytes = n.endByte - n.startByte
	}
	if n.endPoint.Row > n.startPoint.Row {
		rows = n.endPoint.Row - n.startPoint.Row
	}
	return cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
}

// cErrRegionAbsorbPre captures an open ERROR region's memoized aggregates
// before an absorb mutates it. Capture MUST happen before any children/span
// mutation; apply cErrRegionPostAbsorb after the mutation and the
// nodeBumpEquivVersion call.
type cErrRegionAbsorbPre struct {
	node     *Node
	cost     uint32
	vis      int
	spanCost uint32
	valid    bool
}

func (p *Parser) cErrRegionPreAbsorb(n *Node) cErrRegionAbsorbPre {
	if p == nil || len(p.cNodeMemoCache) == 0 || p.language == nil || n == nil {
		return cErrRegionAbsorbPre{}
	}
	if n.symbol != errorSymbol || (n.isMissing() && len(n.children) == 0) {
		return cErrRegionAbsorbPre{}
	}
	// Route through the standard memoized walks so the delta base is exactly
	// the full-walk answer at the pre-absorb version (O(1) once warm).
	cost, vis := p.cNodeErrorCostAndVisibleSubtreeCount(n)
	return cErrRegionAbsorbPre{
		node:     n,
		cost:     cost,
		vis:      vis,
		spanCost: cErrRegionSpanCost(n),
		valid:    true,
	}
}

// cErrRegionPostAbsorb updates the region's memo entries for its NEW
// equivVersion from the captured pre-state plus the absorb delta: the span
// growth and, per appended child, its (finished, memoized) subtree cost, its
// skipped-tree charge, and its visible-subtree count. The child terms mirror
// cNodeErrorCostLang's ERROR-node summarization exactly; the primed values are
// what a full walk at the new version returns.
func (p *Parser) cErrRegionPostAbsorb(pre cErrRegionAbsorbPre, added ...*Node) {
	if !pre.valid || p == nil || len(p.cNodeMemoCache) == 0 {
		return
	}
	n := pre.node
	lang := p.language
	cost := pre.cost - pre.spanCost + cErrRegionSpanCost(n)
	vis := pre.vis
	for _, c := range added {
		if c == nil {
			continue
		}
		childCost, childVis := p.cNodeErrorCostAndVisibleSubtreeCount(c)
		vis += childVis
		if !(c.symbol == errorSymbol && len(c.children) == 0) {
			// C ERROR leaf children keep subtree error_cost 0 (see
			// cNodeErrorCostLang); everything else contributes its own cost.
			cost += childCost
		}
		if !c.isExtra() {
			if cSymbolVisibleLang(lang, c.symbol) {
				cost += cErrCostPerSkippedTree
			} else if len(c.children) > 0 {
				cost += cErrCostPerSkippedTree * uint32(cNodeVisibleChildCountLang(lang, c))
			}
		}
	}
	if debugRecoveryIncrementalCost {
		p.debugCheckErrRegionIncremental(n, cost, vis)
	}
	slot := p.cNodeMemoPrimaryHit(n)
	if slot == nil {
		slot = p.cNodeMemoSlot(n)
	}
	if slot != nil {
		entry := cNodeMemoCacheEntry{
			node:    uintptr(unsafe.Pointer(n)),
			ver:     n.equivVersion,
			cost:    cost,
			hasCost: true,
			epoch:   p.cNodeMemoEpoch,
		}
		if uint64(vis) <= uint64(^uint32(0)) {
			entry.visCount = uint32(vis)
			entry.hasVis = true
		}
		*slot = entry
	}
	if ms := p.mergeScratch; ms != nil && ms.cErrorCostParser != p {
		if ms.cErrorCost == nil {
			ms.cErrorCost = make(map[*Node]glrCErrorCostEntry)
		}
		ms.cErrorCost[n] = glrCErrorCostEntry{ver: n.equivVersion, cost: cost}
	}
}

// cErrRegionPrime warms both memo families for a freshly created (or freshly
// bumped) region node via the standard full walks — O(children) once, so the
// per-token lookups that follow are O(1) until the next absorb re-primes.
func (p *Parser) cErrRegionPrime(n *Node) {
	if p == nil || len(p.cNodeMemoCache) == 0 || p.language == nil || n == nil {
		return
	}
	cost, _ := p.cNodeErrorCostAndVisibleSubtreeCount(n)
	if ms := p.mergeScratch; ms != nil && ms.cErrorCostParser != p {
		if ms.cErrorCost == nil {
			ms.cErrorCost = make(map[*Node]glrCErrorCostEntry)
		}
		ms.cErrorCost[n] = glrCErrorCostEntry{ver: n.equivVersion, cost: cost}
	}
}

// cNodeVisibleSubtreeCountUncachedLang is the memo-free mirror of
// cNodeVisibleSubtreeCount, used only by the debug assertion so a poisoned
// memo cannot mask itself.
func cNodeVisibleSubtreeCountUncachedLang(lang *Language, n *Node) int {
	if n == nil {
		return 0
	}
	count := 0
	if cSymbolVisibleLang(lang, n.symbol) {
		count++
	}
	for _, c := range n.children {
		count += cNodeVisibleSubtreeCountUncachedLang(lang, c)
	}
	return count
}

func (p *Parser) debugCheckErrRegionIncremental(n *Node, cost uint32, vis int) {
	debugRecoveryIncrementalCostChecks++
	fullCost := cNodeErrorCostLang(p.language, n)
	fullVis := cNodeVisibleSubtreeCountUncachedLang(p.language, n)
	if fullCost == cost && fullVis == vis {
		return
	}
	debugRecoveryIncrementalCostDivergences++
	if debugRecoveryIncrementalCostReportsLeft > 0 {
		debugRecoveryIncrementalCostReportsLeft--
		fmt.Fprintf(os.Stderr,
			"RECOVERY-INCREMENTAL-COST divergence: node=%p sym=%d ver=%d span=[%d,%d) children=%d incremental(cost=%d vis=%d) full(cost=%d vis=%d)\n",
			n, n.symbol, n.equivVersion, n.startByte, n.endByte, len(n.children), cost, vis, fullCost, fullVis)
	}
}

func (p *Parser) cNodeErrorCost(n *Node) uint32 {
	if p == nil {
		return 0
	}
	if n == nil {
		return 0
	}
	if len(p.cNodeMemoCache) == 0 {
		return cNodeErrorCostLang(p.language, n)
	}
	slot := p.cNodeMemoPrimaryHit(n)
	if slot == nil {
		slot = p.cNodeMemoSlot(n)
	}
	if slot.hasCost && slot.ver == n.equivVersion {
		return slot.cost
	}
	if n.isMissing() && len(n.children) == 0 {
		return cErrCostPerMissingTree + cErrCostPerRecovery
	}
	var cost uint32
	for _, c := range n.children {
		if c != nil && c.symbol == errorSymbol && len(c.children) == 0 {
			// C ERROR leaf: subtree error_cost 0 (see cNodeErrorCostLang).
			continue
		}
		cost += p.cNodeErrorCost(c)
	}
	if n.symbol == errorSymbol {
		lang := p.language
		for _, c := range n.children {
			if c == nil || c.isExtra() {
				continue
			}
			if cSymbolVisibleLang(lang, c.symbol) {
				cost += cErrCostPerSkippedTree
			} else if len(c.children) > 0 {
				cost += cErrCostPerSkippedTree * uint32(cNodeVisibleChildCountLang(lang, c))
			}
		}
		bytes := uint32(0)
		rows := uint32(0)
		if n.endByte > n.startByte {
			bytes = n.endByte - n.startByte
		}
		if n.endPoint.Row > n.startPoint.Row {
			rows = n.endPoint.Row - n.startPoint.Row
		}
		cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
	}
	// Re-fetch the slot: the recursive p.cNodeErrorCost(c) calls above may
	// have evicted n's slot (a child's pointer hashing into the same 2-way
	// set), so the pointer captured before recursing could now be stale.
	slot = p.cNodeMemoPrimaryHit(n)
	if slot == nil {
		slot = p.cNodeMemoSlot(n)
	}
	if slot.ver != n.equivVersion {
		*slot = cNodeMemoCacheEntry{
			node:  uintptr(unsafe.Pointer(n)),
			ver:   n.equivVersion,
			epoch: p.cNodeMemoEpoch,
		}
	}
	slot.cost = cost
	slot.hasCost = true
	return cost
}

// cNodeErrorCostAndVisibleSubtreeCount computes both C subtree aggregates in
// one walk. The recovery stack needs both values at the same call sites.
func (p *Parser) cNodeErrorCostAndVisibleSubtreeCount(n *Node) (uint32, int) {
	if p == nil || n == nil {
		return 0, 0
	}
	if len(p.cNodeMemoCache) == 0 {
		return cNodeErrorCostLang(p.language, n), cNodeVisibleSubtreeCountUncachedLang(p.language, n)
	}

	version := n.equivVersion
	slot := p.cNodeMemoPrimaryHit(n)
	if slot == nil {
		slot = p.cNodeMemoSlot(n)
	}
	if slot.ver == version {
		switch {
		case slot.hasCost && slot.hasVis:
			return slot.cost, int(slot.visCount)
		case slot.hasCost:
			cost := slot.cost
			return cost, p.cNodeVisibleSubtreeCount(n)
		case slot.hasVis:
			visible := int(slot.visCount)
			return p.cNodeErrorCost(n), visible
		}
	}

	var cost uint32
	visible := 0
	if p.cSymbolVisible(n.symbol) {
		visible++
	}
	if n.isMissing() && len(n.children) == 0 {
		cost = cErrCostPerMissingTree + cErrCostPerRecovery
	} else {
		for _, child := range n.children {
			if child == nil {
				continue
			}
			childCost, childVisible := p.cNodeErrorCostAndVisibleSubtreeCount(child)
			visible += childVisible
			if child.symbol != errorSymbol || len(child.children) != 0 {
				cost += childCost
			}
		}
		if n.symbol == errorSymbol {
			lang := p.language
			for _, child := range n.children {
				if child == nil || child.isExtra() {
					continue
				}
				if cSymbolVisibleLang(lang, child.symbol) {
					cost += cErrCostPerSkippedTree
				} else if len(child.children) > 0 {
					cost += cErrCostPerSkippedTree * uint32(cNodeVisibleChildCountLang(lang, child))
				}
			}
			bytes := uint32(0)
			rows := uint32(0)
			if n.endByte > n.startByte {
				bytes = n.endByte - n.startByte
			}
			if n.endPoint.Row > n.startPoint.Row {
				rows = n.endPoint.Row - n.startPoint.Row
			}
			cost += cErrCostPerRecovery + cErrCostPerSkippedChar*bytes + cErrCostPerSkippedLine*rows
		}
	}

	// Child recursion can evict the original slot. Resolve it again before the
	// write and preserve any matching partial entry.
	slot = p.cNodeMemoPrimaryHit(n)
	if slot == nil {
		slot = p.cNodeMemoSlot(n)
	}
	if slot.ver != version {
		*slot = cNodeMemoCacheEntry{
			node:  uintptr(unsafe.Pointer(n)),
			ver:   version,
			epoch: p.cNodeMemoEpoch,
		}
	}
	slot.cost = cost
	slot.hasCost = true
	if uint64(visible) <= uint64(^uint32(0)) {
		slot.visCount = uint32(visible)
		slot.hasVis = true
	}
	return cost, visible
}

// ---------------------------------------------------------------------------
// GSS prefix aggregates: O(1) ts_stack_error_cost / node_count reads
//
// C maintains error_cost and node_count as monotonic accumulators on each
// stack node at push time (stack.c stack_node_new: "node->error_cost =
// previous_node->error_cost; ... += ts_subtree_error_cost(subtree);",
// stack.c:163-172 pinned), so ts_stack_error_cost (stack.c:493) and
// ts_stack_node_count_since_error (stack.c:504) are O(1) per call. The port
// re-summed the whole prev chain per call — O(depth) — and the condense /
// merge / competition paths issue hundreds of such calls per token, which
// dominates error-region parses even with warm per-node memos.
//
// The on-node aggregates below (gssNode.aggGen/aggCost/aggVis/aggValid)
// restore C's shape: per gssNode, the cumulative aggregates of the prev-chain
// prefix root..node inclusive. gssNode prev/entry links are write-once at
// allocation except setGSSMainLink (link-0 rewrite), and node payload
// contents mutate only through nodeBumpEquivVersion call sites; both choke
// points invalidate gssPrefixAggGen whenever recovery contributions can
// change, so an aggregate with a matching generation is exactly the full-walk
// answer. Metadata-only node changes and identity-preserving link rewrites do
// not affect these aggregates. allocNode zeroes the gens on every (possibly
// slab-recycled) node and the generation counter starts at 1, so stale or
// fresh nodes can never validate.
// ---------------------------------------------------------------------------

// gssPrefixAggGen is the global invalidation generation for the GSS prefix
// aggregates stored on gssNode (aggGen/aggCost/aggVis/aggValid). Bumped by
// recovery-relevant nodeBumpEquivVersion mutations (tree.go) and link-0
// rewrites that change the predecessor or full-Node payload (glr.go). Global
// rather than per-parser because nodeBumpEquivVersion has no parser in scope;
// cross-parser over-invalidation only costs a rebuild, never staleness.
// Initialized to 1 so the zero value of gssNode.aggGen (fresh or slab-cleared
// nodes) can never validate.
var gssPrefixAggGen atomic.Uint64

const maxRetainedGSSPrefixPath = 4 * 1024

func init() {
	gssPrefixAggGen.Store(1)
}

func resetGSSPrefixPath(path *[]*gssNode) {
	if path == nil || cap(*path) == 0 {
		return
	}
	if cap(*path) > maxRetainedGSSPrefixPath {
		*path = nil
		return
	}
	clear((*path)[:cap(*path)])
	*path = (*path)[:0]
}

// cStackPrefixAgg returns the cumulative (error cost, visible subtree count)
// of head's prev chain, filling the on-node aggregates bottom-up from the
// deepest still-valid node — O(new or invalidated suffix), O(1) steady-state.
func (p *Parser) cStackPrefixAgg(head *gssNode) (uint32, int) {
	gen := gssPrefixAggGen.Load()
	var cost uint32
	var vis int32
	path := p.cPrefixPath[:0]
	gn := head
	for gn != nil {
		if gn.aggGen == gen && gn.aggValid&(gssAggCostValid|gssAggVisValid) == (gssAggCostValid|gssAggVisValid) {
			cost, vis = gn.aggCost, gn.aggVis
			break
		}
		path = append(path, gn)
		gn = gn.prev
	}
	for i := len(path) - 1; i >= 0; i-- {
		gn := path[i]
		if n := stackEntryNode(gn.entry); n != nil {
			nodeCost, nodeVisible := p.cNodeErrorCostAndVisibleSubtreeCount(n)
			cost += nodeCost
			vis += int32(nodeVisible)
		}
		if gn.aggGen != gen {
			gn.cleanZeroState = gssCleanZeroUnknown
		}
		gn.aggGen = gen
		gn.aggValid = gssAggCostValid | gssAggVisValid
		gn.aggCost = cost
		gn.aggVis = vis
	}
	p.cPrefixPath = path
	return cost, int(vis)
}

// cStackPrefixCostForMerge is the merge-scratch twin of cStackPrefixAgg. It
// fills cost only (the merge side has no memoized visible-count walk), so it
// marks visibility invalid; the cost value is identical to the parser-side
// fill, letting both sides share aggCost.
func cStackPrefixCostForMerge(scratch *glrMergeScratch, lang *Language, head *gssNode) uint32 {
	gen := gssPrefixAggGen.Load()
	var cost uint32
	path := scratch.cPrefixPath[:0]
	gn := head
	for gn != nil {
		if gn.aggGen == gen && gn.aggValid&gssAggCostValid != 0 {
			cost = gn.aggCost
			break
		}
		path = append(path, gn)
		gn = gn.prev
	}
	for i := len(path) - 1; i >= 0; i-- {
		gn := path[i]
		if n := stackEntryNode(gn.entry); n != nil {
			cost += cNodeErrorCostLangWithScratch(scratch, lang, n)
		}
		if gn.aggGen != gen {
			gn.cleanZeroState = gssCleanZeroUnknown
		}
		gn.aggGen = gen
		gn.aggValid = gssAggCostValid
		gn.aggCost = cost
	}
	scratch.cPrefixPath = path
	return cost
}

// debugCheckStackPrefixCostLang re-derives the chain sum without any cache
// (GOT_DEBUG_RECOVERY_INCREMENTAL_COST=1 only).
func debugCheckStackPrefixCostLang(lang *Language, head *gssNode, got uint32, label string) {
	debugRecoveryIncrementalCostChecks++
	var want uint32
	for gn := head; gn != nil; gn = gn.prev {
		if n := stackEntryNode(gn.entry); n != nil {
			want += cNodeErrorCostLang(lang, n)
		}
	}
	if want == got {
		return
	}
	debugRecoveryIncrementalCostDivergences++
	if debugRecoveryIncrementalCostReportsLeft > 0 {
		debugRecoveryIncrementalCostReportsLeft--
		fmt.Fprintf(os.Stderr,
			"RECOVERY-PREFIX-AGG divergence (%s): head=%p cached=%d full=%d\n", label, head, got, want)
	}
}

func (p *Parser) debugCheckStackPrefixAgg(head *gssNode, gotCost uint32, gotVis int) {
	debugCheckStackPrefixCostLang(p.language, head, gotCost, "parser")
	debugCheckStackPrefixVisLang(p.language, head, gotVis, "parser")
}

// debugCheckStackPrefixVisLang is the visible-subtree-count twin of
// debugCheckStackPrefixCostLang: it re-derives the cumulative visible node
// count of head's prev chain without any cache (via the uncached per-node walk
// so a poisoned cNodeMemoCache cannot mask itself) and compares it against the
// memoized aggVis the same way the cost check guards aggCost. A divergence here
// means cStackCumulativeNodeCount / cNodeCountSinceError — and thus the php
// baseline gate — would read a corrupt count.
// (GOT_DEBUG_RECOVERY_INCREMENTAL_COST=1 only.)
func debugCheckStackPrefixVisLang(lang *Language, head *gssNode, got int, label string) {
	debugRecoveryIncrementalCostChecks++
	var want int
	for gn := head; gn != nil; gn = gn.prev {
		if n := stackEntryNode(gn.entry); n != nil {
			want += cNodeVisibleSubtreeCountUncachedLang(lang, n)
		}
	}
	if want == got {
		return
	}
	debugRecoveryIncrementalCostDivergences++
	if debugRecoveryIncrementalCostReportsLeft > 0 {
		debugRecoveryIncrementalCostReportsLeft--
		fmt.Fprintf(os.Stderr,
			"RECOVERY-PREFIX-AGG-VIS divergence (%s): head=%p cached=%d full=%d\n", label, head, got, want)
	}
}

// cStackErrorCost ports ts_stack_error_cost: the accumulated error cost of
// every subtree on the stack, plus one open recovery when the version just
// entered the error state and has not absorbed anything yet (the C
// "ERROR_STATE head with NULL subtree" case).
func (p *Parser) cStackErrorCost(s *glrStack) uint32 {
	if s == nil {
		return 0
	}
	if p != nil && p.mergeScratch.provesNoChildErrors() {
		return cStackOpenRecoveryCost(s)
	}
	var cost uint32
	if len(s.entries) > 0 {
		if p != nil && len(p.cNodeMemoCache) != 0 && s.cRec != nil {
			cost, _ = p.cStackEntryAgg(s)
		} else {
			for i := range s.entries {
				if n := stackEntryNode(s.entries[i]); n != nil {
					cost += p.cNodeErrorCost(n)
				}
			}
		}
	} else if p != nil && len(p.cNodeMemoCache) != 0 && s.gss.head != nil {
		var vis int
		cost, vis = p.cStackPrefixAgg(s.gss.head)
		if debugRecoveryIncrementalCost {
			p.debugCheckStackPrefixAgg(s.gss.head, cost, vis)
		}
	} else {
		for gn := s.gss.head; gn != nil; gn = gn.prev {
			if n := stackEntryNode(gn.entry); n != nil {
				cost += p.cNodeErrorCost(n)
			}
		}
	}
	return cost + cStackOpenRecoveryCost(s)
}

// cStackOpenRecoveryCost is the part of ts_stack_error_cost that does not
// come from materialized subtrees. Keeping it separate lets the condense path
// skip a provably-zero subtree walk without duplicating recovery semantics.
func cStackOpenRecoveryCost(s *glrStack) uint32 {
	if s == nil {
		return 0
	}
	var cost uint32
	if s.cPaused || (s.cRec != nil && s.cRec.openErr == nil) {
		cost += cErrCostPerRecovery
	}
	if s.cRec != nil && s.cRec.extraRecoveries > 0 {
		// Extra error_repeat segments opened by unlexable-run re-pauses
		// (see cRecoverState.extraRecoveries).
		cost += cErrCostPerRecovery * uint32(s.cRec.extraRecoveries)
	}
	return cost
}

// cStackEntryAgg is the contiguous-stack counterpart of cStackPrefixAgg. It
// computes both C recovery aggregates in one walk and keeps the answer on the
// stack until either a recovery-relevant node mutation bumps gssPrefixAggGen
// or a stack push/truncate/materialization explicitly invalidates it.
func (p *Parser) cStackEntryAgg(s *glrStack) (uint32, int) {
	if s == nil {
		return 0, 0
	}
	gen := gssPrefixAggGen.Load()
	if p != nil && len(p.cNodeMemoCache) != 0 && s.cRec != nil && s.cEntryAggGen == gen {
		return s.cEntryAggCost, int(s.cEntryAggVis)
	}
	var cost uint32
	var vis int32
	for i := range s.entries {
		if n := stackEntryNode(s.entries[i]); n != nil {
			nodeCost, nodeVisible := p.cNodeErrorCostAndVisibleSubtreeCount(n)
			cost += nodeCost
			vis += int32(nodeVisible)
		}
	}
	if p != nil && len(p.cNodeMemoCache) != 0 && s.cRec != nil {
		s.cEntryAggGen = gen
		s.cEntryAggCost = cost
		s.cEntryAggVis = vis
	}
	return cost, int(vis)
}

func cStackErrorCostForMerge(lang *Language, s *glrStack) uint32 {
	return cStackErrorCostForMergeWithScratch(nil, lang, s)
}

func cStackErrorCostForMergeWithScratch(scratch *glrMergeScratch, lang *Language, s *glrStack) uint32 {
	if s == nil {
		return 0
	}
	var cost uint32
	if len(s.entries) > 0 {
		for i := range s.entries {
			if n := stackEntryNode(s.entries[i]); n != nil {
				cost += cNodeErrorCostLangWithScratch(scratch, lang, n)
			}
		}
	} else if scratch != nil && s.gss.head != nil {
		cost = cStackPrefixCostForMerge(scratch, lang, s.gss.head)
		if debugRecoveryIncrementalCost {
			debugCheckStackPrefixCostLang(lang, s.gss.head, cost, "merge")
		}
	} else {
		for gn := s.gss.head; gn != nil; gn = gn.prev {
			if n := stackEntryNode(gn.entry); n != nil {
				cost += cNodeErrorCostLangWithScratch(scratch, lang, n)
			}
		}
	}
	if s.cPaused || (s.cRec != nil && s.cRec.openErr == nil) {
		cost += cErrCostPerRecovery
	}
	if s.cRec != nil && s.cRec.extraRecoveries > 0 {
		// Extra error_repeat segments opened by unlexable-run re-pauses
		// (see cRecoverState.extraRecoveries).
		cost += cErrCostPerRecovery * uint32(s.cRec.extraRecoveries)
	}
	return cost
}

// cStackCumulativeNodeCount mirrors C StackNode.node_count at the stack head:
// the sum of stack__subtree_node_count over every subtree on the stack. The
// engine's open ERROR region node plays the role of the C error_repeat chain
// (its own visible +1 matches the chain's single error_repeat bonus).
func (p *Parser) cStackCumulativeNodeCount(s *glrStack) int {
	if s == nil {
		return 0
	}
	count := 0
	if len(s.entries) > 0 {
		if p != nil && len(p.cNodeMemoCache) != 0 && s.cRec != nil {
			_, count = p.cStackEntryAgg(s)
		} else {
			for i := range s.entries {
				if n := stackEntryNode(s.entries[i]); n != nil {
					count += p.cNodeVisibleSubtreeCount(n)
				}
			}
		}
		return count
	}
	if p != nil && len(p.cNodeMemoCache) != 0 && s.gss.head != nil {
		var cost uint32
		cost, count = p.cStackPrefixAgg(s.gss.head)
		if debugRecoveryIncrementalCost {
			// Verify the memoized aggVis (this cumulative-count path is where the
			// php baseline gate ultimately reads it) against a full uncached walk.
			p.debugCheckStackPrefixAgg(s.gss.head, cost, count)
		}
		return count
	}
	for gn := s.gss.head; gn != nil; gn = gn.prev {
		if n := stackEntryNode(gn.entry); n != nil {
			count += p.cNodeVisibleSubtreeCount(n)
		}
	}
	return count
}

func (p *Parser) cApplyMergedErrorGroupBaseline(versions []glrStack) int {
	groupBaseline := 0
	for vi := range versions {
		if count := p.cStackCumulativeNodeCount(&versions[vi]); count > groupBaseline {
			groupBaseline = count
		}
	}
	for vi := range versions {
		versions[vi].cNodeBaseline = uint32(groupBaseline)
		// Writing a node-count baseline is definitionally an error-state entry;
		// set the sticky wreckage bit alongside it so the (untrustworthy — it can
		// be written as 0 here, or clamped back to 0 by cNodeCountSinceError)
		// baseline can never be the sole evidence the php gate relies on. See
		// glrStack.cEverErrored.
		versions[vi].cEverErrored = true
	}
	return groupBaseline
}

// cNodeCountSinceError ports ts_stack_node_count_since_error: the cumulative
// node count minus the count recorded when the error discontinuity was pushed
// (glrStack.cNodeBaseline; zero for stacks that never errored, which matches
// C's node_count_at_last_error starting at zero).
func (p *Parser) cNodeCountSinceError(s *glrStack) int {
	if s == nil {
		return 0
	}
	count := p.cStackCumulativeNodeCount(s) - int(s.cNodeBaseline)
	if count < 0 {
		// C clamps (and writes back) when the stack popped below the baseline.
		s.cNodeBaseline = uint32(p.cStackCumulativeNodeCount(s))
		return 0
	}
	return count
}

// ---------------------------------------------------------------------------
// Version status + comparison (parser.c ErrorStatus / ErrorComparison port)
// ---------------------------------------------------------------------------

type cErrorStatus struct {
	cost      uint32
	nodeCount int
	dynPrec   int
	isInError bool
}

type cErrorComparison int

const (
	cErrorComparisonTakeLeft cErrorComparison = iota
	cErrorComparisonPreferLeft
	cErrorComparisonNone
	cErrorComparisonPreferRight
	cErrorComparisonTakeRight
)

func (p *Parser) cVersionStatus(s *glrStack) cErrorStatus {
	cost := p.cStackErrorCost(s)
	if s.cPaused {
		cost += cErrCostPerSkippedTree
	}
	return cErrorStatus{
		cost:      cost,
		nodeCount: p.cNodeCountSinceError(s),
		dynPrec:   s.score,
		// C ts_parser__version_status: in error when paused or at ERROR_STATE.
		isInError: s.cPaused || s.cRec != nil,
	}
}

func (p *Parser) cCondenseVersionStatus(s *glrStack, subtreeCostRelevant bool) cErrorStatus {
	var cost uint32
	if subtreeCostRelevant {
		cost = p.cStackErrorCost(s)
	} else {
		// trackChildErrors is false only on a fresh full parse before any ERROR
		// or MISSING subtree has been inserted. The recursive subtree component
		// is therefore exactly zero, but paused/open-recovery costs still apply.
		cost = cStackOpenRecoveryCost(s)
	}
	if s.cPaused {
		cost += cErrCostPerSkippedTree
	}
	status := cErrorStatus{
		cost:      cost,
		dynPrec:   s.score,
		isInError: s.cPaused || s.cRec != nil,
	}
	// cNodeCountSinceError clamps a baseline that has moved above the current
	// stack count. Preserve that eager side effect for recovery lineages even
	// when this comparison branch will not read nodeCount.
	if s.cNodeBaseline != 0 {
		status.nodeCount = p.cNodeCountSinceError(s)
	}
	return status
}

// cCompareCondenseVersions preserves cCompareVersions' literal C semantics
// while deferring the fresh-lineage visible-node walk until the one
// unequal-cost branch that actually reads the cheaper side's count.
func (p *Parser) cCompareCondenseVersions(a, b cErrorStatus, aStack, bStack *glrStack) cErrorComparison {
	if a.isInError == b.isInError {
		if a.cost < b.cost {
			a.nodeCount = p.cNodeCountSinceError(aStack)
		} else if b.cost < a.cost {
			b.nodeCount = p.cNodeCountSinceError(bStack)
		}
	}
	return cCompareVersions(a, b)
}

func (p *Parser) cVersionStatusForTrace(s *glrStack, status cErrorStatus) cErrorStatus {
	status.nodeCount = p.cNodeCountSinceError(s)
	return status
}

// A condense-step drop of a recovery-owning version in favor of a marker-free
// sibling was evaluated as a second trigger for cRecoveryUnvalidatedMarker
// (tagging the surviving stack) but rejected: measured against this repo's
// own valid, compiling Go source, cCondenseAndResume's cost competition
// drops/re-forks recovery-owning-vs-clean candidates as routine, frequent
// (thousands of times per large file) GLR disambiguation on the Go grammar's
// LALR table, not evidence of a real syntax error — a "surviving stack"
// tag propagates through every subsequent clone() of that (winning, and
// therefore usually eventually-selected) lineage for the rest of the parse,
// so it cannot be scoped away by the same-position sibling check the way
// cRecoverToState's single-stack-gated marker can (see below). Only
// cRecoverToState marks cRecoveryUnvalidatedMarker; see its doc comment for
// why that one narrower trigger is sufficient for the confirmed defect class
// (java/php/gomod) without this one.

// cCompareVersions is a literal port of ts_parser__compare_versions.
func cCompareVersions(a, b cErrorStatus) cErrorComparison {
	if !a.isInError && b.isInError {
		if a.cost < b.cost {
			return cErrorComparisonTakeLeft
		}
		return cErrorComparisonPreferLeft
	}
	if a.isInError && !b.isInError {
		if b.cost < a.cost {
			return cErrorComparisonTakeRight
		}
		return cErrorComparisonPreferRight
	}
	if a.cost < b.cost {
		if (b.cost-a.cost)*uint32(1+a.nodeCount) > cRecoverMaxCostDifference {
			return cErrorComparisonTakeLeft
		}
		return cErrorComparisonPreferLeft
	}
	if b.cost < a.cost {
		if (a.cost-b.cost)*uint32(1+b.nodeCount) > cRecoverMaxCostDifference {
			return cErrorComparisonTakeRight
		}
		return cErrorComparisonPreferRight
	}
	if a.dynPrec > b.dynPrec {
		return cErrorComparisonPreferLeft
	}
	if b.dynPrec > a.dynPrec {
		return cErrorComparisonPreferRight
	}
	return cErrorComparisonNone
}

// cBetterVersionExists ports ts_parser__better_version_exists: would the
// candidate (self with hypothetical cost) clearly lose to an existing live
// stack at the same or later position? Stacks in the same absorbing group are
// excluded — they are paths of the same C version, not competitors.
func (p *Parser) cBetterVersionExists(stacks []glrStack, self int, isInError bool, cost uint32) bool {
	pos := stacks[self].byteOffset
	group := (*cRecGroup)(nil)
	if stacks[self].cRec != nil {
		group = stacks[self].cRec.group
	}
	status := cErrorStatus{
		cost:      cost,
		isInError: isInError,
		dynPrec:   stacks[self].score,
		nodeCount: p.cNodeCountSinceError(&stacks[self]),
	}
	for i := range stacks {
		if i == self || stacks[i].dead {
			continue
		}
		// C removes accepted versions from the pool and stashes their tree in
		// finished_tree, which better_version_exists checks FIRST and
		// position-independently: any finished result at least as cheap as
		// the hypothetical cost makes the candidate clearly worse
		// (parser.c: `if (finished_tree && error_cost(finished_tree) <= cost)
		// return true`). Without this an absorbing version elects an EOF
		// strategy-1 recovery C would never attempt once a cheaper result
		// has already been accepted.
		if stacks[i].accepted {
			if p.cStackErrorCost(&stacks[i]) <= cost {
				return true
			}
			continue
		}
		if stacks[i].byteOffset < pos {
			continue
		}
		if group != nil && stacks[i].cRec != nil && stacks[i].cRec.group == group {
			continue
		}
		// NOTE: missing-token versions born from this group's handle_error are
		// genuine competitors in C (ts_parser__better_version_exists loops
		// every live version, and the missing version is created BEFORE
		// ts_parser__recover runs); they are deliberately NOT excluded here.
		st := p.cVersionStatus(&stacks[i])
		switch cCompareVersions(status, st) {
		case cErrorComparisonTakeRight:
			return true
		case cErrorComparisonPreferRight:
			// C: only when the two versions could merge (ts_stack_can_merge:
			// same state, position, and error cost).
			if stacksHeaderEquivalent(&stacks[self], &stacks[i]) &&
				p.cStackErrorCost(&stacks[self]) == p.cStackErrorCost(&stacks[i]) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Stack walking helpers
// ---------------------------------------------------------------------------

// cStackEntriesTopFirst materializes the stack spine top-first. With non-nil
// scratch the returned slice is borrowed and remains valid only until the next
// call or scratch reset. A nil scratch returns independently owned storage.
func cStackEntriesTopFirst(s *glrStack, gssScratch *gssScratch) []stackEntry {
	s.ensureGSS(gssScratch)
	depth := s.depth()
	if depth == 0 {
		if gssScratch != nil {
			gssScratch.stackEntries = gssScratch.stackEntries[:0]
		}
		return nil
	}
	if gssScratch == nil {
		entries := make([]stackEntry, 0, depth)
		for n := s.gss.head; n != nil; n = n.prev {
			entries = append(entries, n.entry)
		}
		return entries
	}
	oldCap := cap(gssScratch.stackEntries)
	if oldCap < depth {
		gssScratch.stackEntries = make([]stackEntry, 0, depth)
		gssScratch.allocatedBytes += stackEntryBytesForCap(depth) - stackEntryBytesForCap(oldCap)
	} else {
		gssScratch.stackEntries = gssScratch.stackEntries[:0]
	}
	entries := gssScratch.stackEntries
	for n := s.gss.head; n != nil; n = n.prev {
		entries = append(entries, n.entry)
	}
	gssScratch.stackEntries = entries
	return entries
}

// cEntryCountsTowardDepth mirrors stack__iter subtree counting: non-extra
// subtrees count, NULL discontinuities count, extras do not.
func cEntryCountsTowardDepth(e stackEntry) bool {
	if !stackEntryHasNode(e) {
		// Node-less entries: the stack base does not count (C's base node has
		// no link); the error discontinuity (state 0) counts like C's NULL
		// subtree link. Both are node-less; distinguish by state — only the
		// discontinuity carries cErrorState above a non-empty prefix, and the
		// base is never crossed by bounded pops anyway.
		return e.state == cErrorState
	}
	return !stackEntryNodeIsExtra(e)
}

// cRecordSummary ports ts_stack_record_summary over this engine's linear
// spine: entries top-first, depth = crossings of depth-counting links,
// deduped on (depth, state), bounded by MAX_SUMMARY_DEPTH.
func (p *Parser) cRecordSummary(entries []stackEntry) []cStackSummaryEntry {
	summary, _ := p.cRecordSummaryWithScratch(entries, nil, nil)
	return summary
}

// cRecordSummaryWithScratch records a summary in parse-scoped GSS scratch.
// It builds the usual bounded shape inline, then reserves only the final size.
func (p *Parser) cRecordSummaryWithScratch(entries []stackEntry, gssScratch *gssScratch, arena *nodeArena) ([]cStackSummaryEntry, ParseStopReason) {
	if len(entries) == 0 {
		return nil, ParseStopNone
	}
	if gssScratch != nil {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return nil, reason
		}
	}
	var inline [cRecoverMaxSummaryDepth + 1]cStackSummaryEntry
	summary := inline[:0]
	var dynamic []cStackSummaryEntry
	var reason ParseStopReason
	reserveDynamic := func() bool {
		if dynamic != nil {
			return true
		}
		if gssScratch != nil {
			dynamic, reason = gssScratch.allocRecoverySummary(len(entries))
			if reason != ParseStopNone {
				return false
			}
			dynamic = dynamic[:len(summary)]
		} else {
			dynamic = make([]cStackSummaryEntry, len(entries))
			dynamic = dynamic[:len(summary)]
		}
		copy(dynamic, summary)
		summary = dynamic
		return true
	}
	depth := 0
	record := func(d int, st StateID, posBytes, posRow uint32) bool {
		for j := len(summary) - 1; j >= 0; j-- {
			if int(summary[j].depth) < d {
				break
			}
			if int(summary[j].depth) == d && summary[j].state == st {
				return true
			}
		}
		if dynamic == nil && len(summary) == cap(summary) && !reserveDynamic() {
			return false
		}
		summary = append(summary, cStackSummaryEntry{depth: uint8(d), state: st, posBytes: posBytes, posRow: posRow})
		return true
	}
	// A node-bearing entry owns its position. Node-less discontinuities use the
	// next payload below them. The cached index advances monotonically, so this
	// preserves the old bottom-up position table in O(depth) time without two
	// full-depth allocations.
	nextPayload := -1
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		var posBytes uint32
		var posRow uint32
		if stackEntryHasNode(entry) {
			posBytes = stackEntryNodeEndByte(entry)
			posRow = stackEntryNodeEndPoint(entry).Row
		} else {
			if nextPayload <= i {
				nextPayload = i + 1
				for nextPayload < len(entries) && !stackEntryHasNode(entries[nextPayload]) {
					nextPayload++
				}
			}
			if nextPayload < len(entries) {
				posBytes = stackEntryNodeEndByte(entries[nextPayload])
				posRow = stackEntryNodeEndPoint(entries[nextPayload]).Row
			}
		}
		if !record(depth, entry.state, posBytes, posRow) {
			return nil, p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceScratch)
		}
		if cEntryCountsTowardDepth(entry) {
			depth++
			if depth > cRecoverMaxSummaryDepth {
				break
			}
		}
	}
	if dynamic != nil {
		return dynamic[:len(summary):len(summary)], ParseStopNone
	}
	if gssScratch != nil {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return nil, reason
		}
		out, reason := gssScratch.allocRecoverySummary(len(summary))
		if reason != ParseStopNone {
			return nil, p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceScratch)
		}
		copy(out, summary)
		return out[:len(summary):len(summary)], ParseStopNone
	}
	out := make([]cStackSummaryEntry, len(summary))
	copy(out, summary)
	return out[:len(out):len(out)], ParseStopNone
}

// ---------------------------------------------------------------------------
// do_all_potential_reductions port
// ---------------------------------------------------------------------------

type cReduceActionKey struct {
	symbol Symbol
	count  uint8
}

// cCollectPotentialReductions gathers the deduped reduce-action set for the
// state over the symbol range, and whether any non-extra shift exists
// (parser.c lines 1121-1157).
func (p *Parser) cCollectPotentialReductions(state StateID, lookaheadSym Symbol, anyLookahead bool, reduces *[]ParseAction) bool {
	*reduces = (*reduces)[:0]
	hasShift := false
	seen := make(map[cReduceActionKey]bool, 4)
	scan := func(sym Symbol) {
		idx := p.lookupActionIndex(state, sym)
		if idx == 0 || int(idx) >= len(p.language.ParseActions) {
			return
		}
		for _, act := range p.language.ParseActions[idx].Actions {
			switch act.Type {
			case ParseActionShift, ParseActionRecover:
				if !act.Extra && !act.Repetition {
					hasShift = true
				}
			case ParseActionAccept:
				hasShift = true
			case ParseActionReduce:
				if act.ChildCount > 0 {
					key := cReduceActionKey{symbol: act.Symbol, count: act.ChildCount}
					if !seen[key] {
						seen[key] = true
						*reduces = append(*reduces, act)
					}
				}
			}
		}
	}
	if anyLookahead {
		tokenCount := Symbol(p.language.TokenCount)
		for sym := Symbol(1); sym < tokenCount; sym++ {
			scan(sym)
		}
	} else {
		scan(lookaheadSym)
	}
	return hasShift
}

// cDoAllPotentialReductions ports ts_parser__do_all_potential_reductions for
// one starting stack. It returns the resulting version set (what the starting
// version became, plus surviving forks) and whether some version can shift
// the lookahead. With anyLookahead true the reductions reachable on ANY
// symbol are applied (the "close in-progress productions" step); versions
// that dead-end keep their pre-reduction shape (C leaves them in place).
// With anyLookahead false, dead-end versions are dropped (C removes them).
// EOF is symbol 0, so callers must pass anyLookahead explicitly instead of
// overloading lookaheadSym == 0. The caller seed supplies reusable initial
// capacity for the returned version set; growth beyond that capacity remains
// ordinary append growth.
func (p *Parser) cDoAllPotentialReductions(source []byte, start glrStack, lookaheadSym Symbol, anyLookahead bool, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, trackChildErrors *bool, callerSeed []glrStack) ([]glrStack, bool, ParseStopReason) {
	oldDisablePostReduceForkMerge := p.disablePostReduceForkMerge
	p.disablePostReduceForkMerge = true
	defer func() {
		p.disablePostReduceForkMerge = oldDisablePostReduceForkMerge
	}()
	checkStop := func() ParseStopReason {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return reason
		}
		return ParseStopNone
	}
	if reason := checkStop(); reason != ParseStopNone {
		return nil, false, reason
	}

	versions := append(callerSeed[:0], start)
	canShift := false
	var reduces []ParseAction
	var singletonCandidate glrStack
	v := 0
	candidateAttempts := 0
	for iter := 0; ; iter++ {
		if reason := checkStop(); reason != ParseStopNone {
			return versions, canShift, reason
		}
		if v >= len(versions) {
			break
		}
		// Merge check against earlier versions created in this call.
		merged := false
		for j := 0; j < v; j++ {
			if reason := checkStop(); reason != ParseStopNone {
				return versions, canShift, reason
			}
			if p.cTryMergeReductionVersion(&versions[j], &versions[v]) {
				versions, _ = cRemoveReductionVersion(versions, v)
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		state := versions[v].top().state
		hasShift := p.cCollectPotentialReductions(state, lookaheadSym, anyLookahead, &reduces)
		lastReduction := -1
		for _, act := range reduces {
			if reason := checkStop(); reason != ParseStopNone {
				return versions, canShift, reason
			}
			// cRecoverMaxReductionCandidateAttempts backstop (see its doc
			// comment above cRecoverMaxVersionCount): truncate exactly like
			// the existing len(versions) > cRecoverMaxVersionCount+1 break
			// below — return what has already been accumulated, ParseStopNone,
			// no halted parse.
			candidateAttempts++
			if uint64(candidateAttempts) > p.crecoveryReductionCandidateAttemptsPeak {
				p.crecoveryReductionCandidateAttemptsPeak = uint64(candidateAttempts)
			}
			if candidateAttempts > cRecoverMaxReductionCandidateAttempts {
				p.crecoveryReductionCandidateCeilingHits++
				return versions, canShift, ParseStopNone
			}
			var actionReductionVersion int
			var reason ParseStopReason
			versions, actionReductionVersion, reason = p.cAppendReductionActionVersions(
				source, versions, v, act, tok, nodeCount, arena, entryScratch,
				gssScratch, tmpEntries, trackChildErrors, &singletonCandidate, nil,
			)
			if reason != ParseStopNone {
				return versions, canShift, reason
			}
			// C overwrites reduction_version for every reduce action, including
			// STACK_VERSION_NONE when the action only merges into an existing
			// version or produces no surviving version.
			lastReduction = actionReductionVersion
		}
		// C (ts_parser__do_all_potential_reductions): a version whose state can
		// shift SOME token is kept as its own path — even during the
		// handle_error ANY-lookahead pass — and its reduction results remain
		// separate versions. Replacing the shiftable original with its
		// reduction loses the C merged version's original-shape path and with
		// it the summary entries C's strategy-1 scan elects (authzed stray
		// backtick: C recovers to the pre-reduction state 8 at depth 1 and
		// closes binary_expression with the \n lookahead; without this path
		// the election lands in a deeper state and the caveat never closes).
		if hasShift {
			canShift = true
		} else if lastReduction >= 0 && iter < cRecoverMaxVersionCount {
			// C renumbers the LAST reduction version onto the current version
			// (reduction_version is overwritten per reduce action) and
			// reprocesses it in place.
			var renumbered bool
			versions, renumbered = cRenumberReductionVersion(versions, lastReduction, v)
			if !renumbered {
				return versions, canShift, ParseStopInvariantViolation
			}
			continue
		} else if !anyLookahead {
			versions, _ = cRemoveReductionVersion(versions, v)
			continue
		}
		if v == 0 {
			v = 1
		} else {
			v++
		}
		if len(versions) > cRecoverMaxVersionCount+1 {
			break
		}
	}
	return versions, canShift, ParseStopNone
}

func (p *Parser) cReductionCandidatesForAction(source []byte, start glrStack, act ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, trackChildErrors *bool) ([]glrStack, ParseStopReason) {
	var singletonCandidate glrStack
	candidates, hasSingletonCandidate, reason := p.cReductionCandidatesForActionInto(source, start, act, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, trackChildErrors, &singletonCandidate, nil)
	if hasSingletonCandidate {
		return []glrStack{singletonCandidate}, reason
	}
	return candidates, reason
}

// cAppendReductionActionVersions applies one reduction to an unchanged source
// version. It appends every surviving pop result in C stack-version order and
// merges each result into earlier compatible versions before the caller can
// promote a result into the source slot.
func (p *Parser) cAppendReductionActionVersions(source []byte, versions []glrStack, originalVersion int, act ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, trackChildErrors *bool, singletonCandidate *glrStack, anyReduced *bool) ([]glrStack, int, ParseStopReason) {
	if originalVersion < 0 || originalVersion >= len(versions) || singletonCandidate == nil {
		return versions, -1, ParseStopInvariantViolation
	}
	actionCandidates, hasSingletonCandidate, reason := p.cReductionCandidatesForActionInto(
		source, versions[originalVersion], act, tok, nodeCount, arena,
		entryScratch, gssScratch, tmpEntries, trackChildErrors, singletonCandidate, anyReduced,
	)
	if reason != ParseStopNone {
		return versions, -1, reason
	}
	if hasSingletonCandidate {
		var appended bool
		versions, appended = p.cAppendReductionVersion(versions, *singletonCandidate, originalVersion)
		if appended {
			return versions, len(versions) - 1, ParseStopNone
		}
		return versions, -1, ParseStopNone
	}
	if len(actionCandidates) == 0 {
		return versions, -1, ParseStopNone
	}
	versions, reductionVersion := p.cAppendActionReductionVersions(versions, actionCandidates, originalVersion, arena)
	return versions, reductionVersion, ParseStopNone
}

// cAppendActionCellReductionVersions applies the reductions in one parse-table
// cell to an unchanged physical source slot. A repetition shift is a no-op.
// The first ordinary shift, accept, or recover action owns the source slot and
// stops the scan. Without a terminal action, the caller promotes the first
// surviving result from the last successful reduction.
func (p *Parser) cAppendActionCellReductionVersions(source []byte, versions []glrStack, originalVersion int, actions []ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, trackChildErrors *bool, anyReduced *bool) ([]glrStack, int, int, ParseStopReason) {
	if originalVersion < 0 || originalVersion >= len(versions) {
		return versions, -1, -1, ParseStopInvariantViolation
	}
	if len(p.pendingForkStacks) != 0 || len(p.pendingFrontierForkStacks) != 0 {
		return versions, -1, -1, ParseStopInvariantViolation
	}
	oldDisablePostReduceForkMerge := p.disablePostReduceForkMerge
	p.disablePostReduceForkMerge = true
	defer func() {
		p.disablePostReduceForkMerge = oldDisablePostReduceForkMerge
	}()

	lastReductionVersion := -1
	var singletonCandidate glrStack
	for actionOrdinal, action := range actions {
		switch action.Type {
		case ParseActionReduce:
			var reductionVersion int
			var reason ParseStopReason
			versions, reductionVersion, reason = p.cAppendReductionActionVersions(
				source, versions, originalVersion, action, tok, nodeCount, arena,
				entryScratch, gssScratch, tmpEntries, trackChildErrors, &singletonCandidate, anyReduced,
			)
			if reason != ParseStopNone {
				return versions, lastReductionVersion, -1, reason
			}
			if reductionVersion >= 0 {
				lastReductionVersion = reductionVersion
			}
		case ParseActionShift:
			if action.Repetition {
				continue
			}
			return versions, lastReductionVersion, actionOrdinal, ParseStopNone
		case ParseActionAccept, ParseActionRecover:
			return versions, lastReductionVersion, actionOrdinal, ParseStopNone
		}
	}
	return versions, lastReductionVersion, -1, ParseStopNone
}

func (p *Parser) cReductionCandidatesForActionInto(source []byte, start glrStack, act ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, trackChildErrors *bool, singletonCandidate *glrStack, anyReduced *bool) ([]glrStack, bool, ParseStopReason) {
	p.pendingForkStacks = p.pendingForkStacks[:0]
	defer func() {
		clear(p.pendingForkStacks)
		p.pendingForkStacks = p.pendingForkStacks[:0]
	}()
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		return nil, false, reason
	}
	fork := start.cloneWithScratch(gssScratch)
	if workCountInstrumentationEnabled {
		workCountTopologyPrepareVersionCopy(&start, &fork)
		workCountTopologyRecordAction(&fork, tok, act, -1)
	}
	var dummy bool
	deferParentLinks := p.reduceScratch != nil && p.reduceScratch.transientParents != nil
	var localTmpEntries []stackEntry
	if tmpEntries != nil {
		localTmpEntries = *tmpEntries
	}
	p.applyAction(source, &fork, act, tok, &dummy, nodeCount, arena, entryScratch, gssScratch, &localTmpEntries, deferParentLinks, trackChildErrors)
	if dummy && anyReduced != nil {
		*anyReduced = true
	}
	if workCountInstrumentationEnabled {
		workCountTopologyRecordActionResult(&fork)
	}
	if tmpEntries != nil {
		if localTmpEntries != nil {
			*tmpEntries = localTmpEntries[:0]
		} else {
			*tmpEntries = (*tmpEntries)[:0]
		}
	}
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		return nil, false, reason
	}
	if len(p.pendingForkStacks) == 0 {
		if fork.dead {
			return nil, false, ParseStopNone
		}
		// The caller consumes this value synchronously. The append helper copies
		// it into versions and never retains the caller-owned slot.
		*singletonCandidate = fork
		return nil, true, ParseStopNone
	}
	candidates := make([]glrStack, 0, 1+len(p.pendingForkStacks))
	if !fork.dead {
		candidates = append(candidates, fork)
	}
	for i := range p.pendingForkStacks {
		if i&15 == 0 {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				return candidates, false, reason
			}
		}
		if !p.pendingForkStacks[i].dead {
			candidates = append(candidates, p.pendingForkStacks[i])
		}
	}
	return candidates, false, ParseStopNone
}

func (p *Parser) cAppendActionReductionVersions(versions []glrStack, candidates []glrStack, originalVersion int, arena *nodeArena) ([]glrStack, int) {
	candidates = p.cCollapseSamePopReductionCandidates(candidates, arena)
	firstAppended := -1
	for i := range candidates {
		var appended bool
		versions, appended = p.cAppendReductionVersion(versions, candidates[i], originalVersion)
		if appended && firstAppended < 0 {
			firstAppended = len(versions) - 1
		}
	}
	return versions, firstAppended
}

func (p *Parser) cCollapseSamePopReductionCandidates(candidates []glrStack, arena *nodeArena) []glrStack {
	if len(candidates) < 2 {
		return candidates
	}
	out := candidates[:0]
	for i := range candidates {
		keep := true
		for j := 0; j < len(out); j++ {
			if p.cTryCollapseSamePopReductionVersion(&out[j], &candidates[i], arena) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, candidates[i])
		}
	}
	return out
}

func (p *Parser) cAppendReductionVersion(versions []glrStack, candidate glrStack, originalVersion int) ([]glrStack, bool) {
	for i := range versions {
		if i == originalVersion {
			continue
		}
		if p.cTryMergeReductionVersion(&versions[i], &candidate) {
			return versions, false
		}
	}
	versions = append(versions, candidate)
	return versions, true
}

// cRemoveReductionVersion applies C's stable stack-version deletion. Every
// later physical slot moves left by one. Clear the retired tail so the caller's
// reusable backing array does not retain graph and error-region references.
func cRemoveReductionVersion(versions []glrStack, version int) ([]glrStack, bool) {
	if version < 0 || version >= len(versions) {
		return versions, false
	}
	copy(versions[version:], versions[version+1:])
	clear(versions[len(versions)-1:])
	return versions[:len(versions)-1], true
}

// cRenumberReductionVersion replaces an earlier physical slot with a later
// reduction result. C deletes the target head, moves the source head into that
// slot, and then removes the source slot with stable compaction.
func cRenumberReductionVersion(versions []glrStack, sourceVersion, targetVersion int) ([]glrStack, bool) {
	if targetVersion < 0 || sourceVersion <= targetVersion || sourceVersion >= len(versions) {
		return versions, false
	}
	versions[targetVersion] = versions[sourceVersion]
	return cRemoveReductionVersion(versions, sourceVersion)
}

func (p *Parser) cTryMergeReductionVersion(target, candidate *glrStack) bool {
	if target == nil || candidate == nil || target.dead || candidate.dead || target.accepted || candidate.accepted {
		return false
	}
	if target.entries != nil || candidate.entries != nil || target.gss.head == nil || candidate.gss.head == nil {
		return false
	}
	if !stacksHeaderEquivalent(target, candidate) {
		return false
	}
	return tryGSSMainMergeForParser(p, target, candidate)
}

func (p *Parser) cTryCollapseSamePopReductionVersion(target, candidate *glrStack, arena *nodeArena) bool {
	if target == nil || candidate == nil || target.dead || candidate.dead || target.accepted || candidate.accepted {
		return false
	}
	targetParent, targetPopTo, ok := cReductionParentAndPopTarget(target)
	if !ok {
		return false
	}
	candidateParent, candidatePopTo, ok := cReductionParentAndPopTarget(candidate)
	if !ok || targetPopTo != candidatePopTo {
		return false
	}
	if targetPopTo == nil || !stacksHeaderEquivalent(target, candidate) {
		return false
	}
	if p.cSelectReplacementParentEntry(arena, targetParent.entry, candidateParent.entry) {
		*target = *candidate
	}
	return true
}

func cReductionParentAndPopTarget(s *glrStack) (*gssNode, *gssNode, bool) {
	if s == nil || s.gss.head == nil || s.entries != nil {
		return nil, nil, false
	}
	n := s.gss.head
	for n != nil {
		entryNode := stackEntryNode(n.entry)
		if entryNode == nil || !entryNode.isExtra() {
			break
		}
		n = n.prev
	}
	if n == nil || !stackEntryHasNode(n.entry) {
		return nil, nil, false
	}
	return n, n.prev, true
}

func (p *Parser) cSelectReplacementParentEntry(arena *nodeArena, existing, candidate stackEntry) bool {
	existingCost := p.rawStackEntryErrorCost(arena, existing)
	candidateCost := p.rawStackEntryErrorCost(arena, candidate)
	if candidateCost < existingCost {
		return true
	}
	if existingCost < candidateCost {
		return false
	}
	existingDyn := stackEntryDynamicPrecedence(existing)
	candidateDyn := stackEntryDynamicPrecedence(candidate)
	if candidateDyn > existingDyn {
		return true
	}
	if existingDyn > candidateDyn {
		return false
	}
	if existingCost > 0 {
		return true
	}
	return p.compareRawStackEntries(arena, candidate, existing) < 0
}

// ---------------------------------------------------------------------------
// handle_error port
// ---------------------------------------------------------------------------

// cTerminalNextState ports ts_language_next_state for terminals: the shift
// target of the last action (extra shifts keep the state).
func (p *Parser) cTerminalNextState(state StateID, sym Symbol) (StateID, ParseAction, bool) {
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return 0, ParseAction{}, false
	}
	actions := p.language.ParseActions[idx].Actions
	if len(actions) == 0 {
		return 0, ParseAction{}, false
	}
	act := actions[len(actions)-1]
	if act.Type != ParseActionShift {
		return 0, ParseAction{}, false
	}
	if act.Extra {
		return state, act, true
	}
	return act.State, act, true
}

// cHandleError ports ts_parser__handle_error for the stack at index si: run
// do_all_potential_reductions on ANY symbol, attempt one missing-token
// insertion across the version set, push the error discontinuity on every
// version, record summaries, then run ts_parser__recover for the current
// lookahead. C merges all discontinuity versions into ONE multi-link version;
// this engine keeps one linear stack per path and ties them into a cRecGroup
// so the strategy-1 election runs once per token across the group.
//
// Returns the outcome for the original stack (which stacks[si] now reflects)
// and whether any new version still needs to act on the current token (the
// missing-token versions and strategy-1 recoveries) — the caller must force a
// re-dispatch pass for the same token.
func (p *Parser) cHandleError(stacks *[]glrStack, si int, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, trackChildErrors *bool) (cRecoverOutcome, bool, ParseStopReason) {
	p.recordRecoveryEntry()
	// C-recovery reads raw shapes unconditionally once it runs (see
	// cSelectReplacementParentEntry / compareRawStackEntries), and its
	// version-spawning (cRecoverToState and friends) creates multi-stack
	// exploration windows that the ordinary singleStackMode/everForked
	// bookkeeping in parser.go never observes, because recovery mutates
	// *stacks directly rather than going through the normal fork dispatch at
	// "len(actions) > 1". Latch everForked here, at the single funnel entry
	// for all C-recovery error handling, before cDoAllPotentialReductions (or
	// anything else recovery does) can build a single node: this guarantees
	// every reduction recovery performs from this point captures its raw
	// shape, and — because everForked is sticky for the rest of the parse —
	// also covers every later recovery version-spawn window and blocks a
	// later setSingleStackMode(true) re-collapse from re-enabling elision.
	// This does not retroactively add shapes to nodes already built before
	// recovery started — see raw_shape.go captureRawShape's doc comment for
	// why that is still safe (a structural pointer-sharing argument, not
	// retroactive backfill).
	if gssScratch != nil {
		gssScratch.everForked = true
	}
	checkStop := func() ParseStopReason {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return reason
		}
		return ParseStopNone
	}
	if reason := checkStop(); reason != ParseStopNone {
		return cRecHalted, false, reason
	}
	s := &(*stacks)[si]
	s.cPaused = false
	// Sticky per-stack wreckage bit: this lineage is entering C error handling.
	// Unlike cRec/cPaused/cNodeBaseline (all of which a later recovery resets to
	// pristine values), cEverErrored never clears, so the php comma-list gate can
	// tell "recovered wreckage that now looks clean" from "provably never
	// errored". Setting it at the funnel entry means every downstream version
	// (s.clone() below, its reduction/missing forks, and *s = versions[0])
	// inherits it via clone()/cloneWithScratch(). See glrStack.cEverErrored.
	s.cEverErrored = true
	// cHandleError running is NOT proof the input is malformed — LALR table
	// limitations routinely drive well-formed input into a momentary
	// no-action point that step 1 below (cDoAllPotentialReductions) resolves
	// losslessly. Recording that we ran here only lets
	// resolveCRecoverySwallowedError use this as a cheap pre-filter; the
	// actual suspicion signal is cRecoveryDroppedErrorForClean, scoped to the
	// selected result's own lineage in buildResultFromGLR.
	firstRecoveryEntry := !p.crecoveryEnteredErrorState
	p.crecoveryEnteredErrorState = true
	if firstRecoveryEntry {
		p.activateCNodeMemoMergeSharing()
	}
	// This lineage is now doing real recovery-competition work (as opposed to
	// the routine per-token GLR cost comparisons every capable parse runs even
	// when well-formed), so it is worth upgrading the per-subtree memo from its
	// small per-parse default to its full working-set size (see
	// growCNodeMemoCache / cNodeMemoCacheInitialSize).
	if firstRecoveryEntry {
		p.growCNodeMemoCache()
	}
	// Recovery machinery is running: stack error costs can now be nonzero, so
	// the merge cost competition must run its walks from here on (sticky
	// per-parse gate, see crecoveryCostCompetitionRelevant).
	p.crecoveryCostCompetitionRelevant = true
	// s is re-entering recovery, so whatever marker content it may already
	// carry from an earlier cRecoverToState fork or condense-drop win (see
	// cRecoveryUnvalidatedMarker) is about to be re-accounted for by this
	// call's own cost bookkeeping (and, if it competes again,
	// by cCondenseAndResume's comparison loop). Clear the "unvalidated" flag
	// so a lineage that keeps recovering normally doesn't look suspicious at
	// ACCEPT — only a lineage that creates/inherits a marker and then reaches
	// ACCEPT WITHOUT ever coming back through here does.
	s.cRecoveryUnvalidatedMarker = false
	// The swallowed-error defect class (see cRecoveryUnvalidatedMarker) is
	// specifically about a single-stack no-action dead end — the exact
	// scenario the parser.go ~4708 gate is about. Highly ambiguous grammars
	// (kotlin's generic-vs-comparison forks, for example) can drive multiple
	// unrelated GLR candidates into cHandleError while genuinely disambiguating
	// valid input; recovery forks born there routinely settle back to a
	// legitimately clean tree without ever being "re-validated", so they must
	// not be treated as suspicious. Track this per call so cRecoverToState
	// only marks a fork unvalidated when recovery owned the whole parse at
	// that moment.
	p.crecoveryHandleErrorSingleStack = len(*stacks) == 1

	// 1. Close in-progress productions: reductions reachable on any symbol.
	// Promote the error stack to the graph-structured stack before reductions.
	// Recovery forks then share the immutable prefix instead of copying each deep linear stack.
	var outerResultSeed [2]glrStack
	reductionSeed := s.cloneWithScratch(gssScratch)
	if workCountInstrumentationEnabled {
		workCountTopologyRecordVersionCopy(s, &reductionSeed)
	}
	versions, _, reason := p.cDoAllPotentialReductions(source, reductionSeed, 0, true, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, trackChildErrors, outerResultSeed[:0])
	if reason != ParseStopNone {
		return cRecHalted, false, reason
	}
	group := &cRecGroup{}

	// 2. Missing-token insertion (once across the version set, in order).
	// C keeps every version that survives do_all_potential_reductions on the
	// lookahead (the copied version plus its reduction forks).
	var missingVersions []glrStack
	var missingProbeSeed [2]glrStack
	if !p.isGraphQLRecoveryTripleQuote(tok.Symbol) {
		missingTokenTrialAttempts := 0
	missingTokenSearch:
		for vi := range versions {
			if reason := checkStop(); reason != ParseStopNone {
				return cRecHalted, false, reason
			}
			state := versions[vi].top().state
			tokenCount := Symbol(p.language.TokenCount)
			for ms := Symbol(1); ms < tokenCount; ms++ {
				if ms&63 == 0 {
					if reason := checkStop(); reason != ParseStopNone {
						return cRecHalted, false, reason
					}
				}
				nextState, shiftAct, ok := p.cTerminalNextState(state, ms)
				if !ok || nextState == 0 || nextState == state {
					continue
				}
				if !p.stateHasLeadingReduceAction(nextState, tok.Symbol) {
					continue
				}
				// cRecoverMaxMissingTokenTrials backstop (see its doc comment
				// above cRecoverMaxVersionCount): abandon the missing-token
				// search across every remaining (version, symbol) pair
				// exactly as if each had failed its own trial —
				// missingVersions stays nil, so step 3 below proceeds via its
				// ordinary "no missing-token match found" path.
				missingTokenTrialAttempts++
				if uint64(missingTokenTrialAttempts) > p.crecoveryMissingTokenTrialAttemptsPeak {
					p.crecoveryMissingTokenTrialAttemptsPeak = uint64(missingTokenTrialAttempts)
				}
				if missingTokenTrialAttempts > cRecoverMaxMissingTokenTrials {
					p.crecoveryMissingTokenCeilingHits++
					break missingTokenSearch
				}
				cand := versions[vi].cloneWithScratch(gssScratch)
				if workCountInstrumentationEnabled {
					workCountTopologyRecordVersionCopy(&versions[vi], &cand)
				}
				cand.cRec = nil
				cand.cRecoverMissingGroup = nil
				missingTok := Token{
					Symbol:     ms,
					StartByte:  tok.StartByte,
					EndByte:    tok.StartByte,
					StartPoint: tok.StartPoint,
					EndPoint:   tok.StartPoint,
					Missing:    true,
				}
				if top := cand.top(); stackEntryHasNode(top) && stackEntryNodeEndByte(top) <= tok.StartByte {
					missingTok.StartByte = stackEntryNodeEndByte(top)
					missingTok.EndByte = stackEntryNodeEndByte(top)
					missingTok.StartPoint = stackEntryNodeEndPoint(top)
					missingTok.EndPoint = stackEntryNodeEndPoint(top)
				}
				var dummy bool
				p.applyAction(source, &cand, shiftAct, missingTok, &dummy, nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
				if reason := checkStop(); reason != ParseStopNone {
					return cRecHalted, false, reason
				}
				if p.rejectUndrainedPendingForkStacks(&cand) {
					continue
				}
				cand.shifted = false
				if cand.dead {
					continue
				}
				reduced, canShift, reason := p.cDoAllPotentialReductions(source, cand, tok.Symbol, false, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, trackChildErrors, missingProbeSeed[:0])
				if reason != ParseStopNone {
					return cRecHalted, false, reason
				}
				if !canShift || len(reduced) == 0 {
					continue
				}
				missingVersions = reduced
				break
			}
			if missingVersions != nil {
				break
			}
		}
	}

	// 3. Enter the error state on every version: push the discontinuity
	// (C NULL subtree at ERROR_STATE), apply the merged node-count baseline
	// (C: ts_stack_merge resets node_count_at_last_error to the merged head's
	// max node count), and record the stack summary. All versions share one
	// cRecGroup — the engine equivalent of C merging them into one version
	// before ts_stack_record_summary.
	for vi := range versions {
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, false, reason
		}
		v := &versions[vi]
		v.pushEntry(stackEntry{state: cErrorState}, entryScratch, gssScratch)
		v.shifted = false
	}
	p.cApplyMergedErrorGroupBaseline(versions)
	for vi := range versions {
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, false, reason
		}
		v := &versions[vi]
		entries := cStackEntriesTopFirst(v, gssScratch)
		hasParsedPrefix := cRecoveryEntriesHaveParsedPrefix(entries, tok.StartByte)
		if debugRecoveryCycleChecks {
			for ei := range entries {
				if entries[ei].node != nil {
					debugRecoveryCheckAcyclic(p, arena, fmt.Sprintf("handle-error-version-%d-spine-%d", vi, ei), entries[ei])
				}
			}
		}
		summary, reason := p.cRecordSummaryWithScratch(entries, gssScratch, arena)
		if reason != ParseStopNone {
			return cRecHalted, false, reason
		}
		// Ordinary recovery caps versions near cRecoverMaxVersionCount. Pass vi
		// before narrowing so the packer fails closed if a future path exceeds it.
		v.cRec = &cRecoverState{
			summary: summary, group: group,
			groupOrder: cPackRecoverGroupOrder(
				uint64(vi),
				cRecoveryRegionClearsOrdinaryLeafErrors(p, tok, hasParsedPrefix),
			),
		}
		v.cRecoverMissingGroup = nil
	}

	// The original stack becomes the first absorbing version.
	*s = versions[0]
	for vi := 1; vi < len(versions); vi++ {
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, false, reason
		}
		*stacks = append(*stacks, versions[vi])
	}

	// C creates the missing-token version INSIDE handle_error, before
	// ts_parser__recover runs, so recover's would-merge guard sees it:
	// a summary entry whose (state, position) the missing version already
	// occupies is skipped, sending the election to a deeper entry (php
	// `static function a() {}` in context: the missing-";" version sits at
	// the expression-statement state, so C pops through the preceding
	// function_definition instead of recovering into that state). Append
	// the missing versions before the recover pass for the same visibility.
	needsRedispatch := false
	for vi := range missingVersions {
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, false, reason
		}
		missingVersions[vi].branchOrder = (*stacks)[si].branchOrder
		missingVersions[vi].cRecoverMissingGroup = group
		*stacks = append(*stacks, missingVersions[vi])
		needsRedispatch = true
	}

	// 4. Run recover for the current lookahead across the absorbing group.
	// Recover may fork one strategy-1 candidate (which must act on this
	// token), absorb the token on each member, or halt members.
	outcome := cRecoverOutcome(cRecConsumed)
	first := true
	for i := 0; i < len(*stacks); i++ {
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, needsRedispatch, reason
		}
		v := &(*stacks)[i]
		if v.dead || v.cRec == nil || v.cRec.group != group {
			continue
		}
		res, forked, reason := p.cRecover(stacks, v, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		if reason != ParseStopNone {
			return cRecHalted, needsRedispatch || forked, reason
		}
		v = &(*stacks)[i]
		if forked {
			needsRedispatch = true
		}
		if first {
			outcome = res
			first = false
		} else if res == cRecHalted {
			v.dead = true
		}
	}
	p.recordRecoveryLiveVersions(*stacks)
	return outcome, needsRedispatch, ParseStopNone
}

// cRecoveryRegionClearsOrdinaryLeafErrors reports whether C would keep
// ordinary visible leaves clean in this recovery region. The predicate uses
// token provenance, symbol metadata, and a source-bearing stack prefix.
func cRecoveryRegionClearsOrdinaryLeafErrors(p *Parser, tok Token, hasParsedPrefix bool) bool {
	return p != nil && hasParsedPrefix && p.cSymbolVisible(tok.Symbol) &&
		p.isNamedSymbol(tok.Symbol) && !tok.lexerSkippedPrefix &&
		cRecoveryTokenCanClearOrdinaryLeafError(tok)
}

// cRecoveryEntriesHaveParsedPrefix proves that recovery follows a clean,
// source-bearing stack node at or before the current token. The error
// discontinuity and the base state do not provide this proof.
func cRecoveryEntriesHaveParsedPrefix(entries []stackEntry, tokenStartByte uint32) bool {
	for _, entry := range entries {
		if entry.node == nil || entry.state == cErrorState || entry.kind == stackEntryKindPendingParent ||
			(entry.kind != stackEntryKindNode && entry.kind != stackEntryKindNoTreeNode && entry.kind != stackEntryKindCompactFullLeaf) ||
			stackEntryNodeSymbol(entry) == 0 || stackEntryNodeSymbol(entry) == errorSymbol ||
			stackEntryNodeParseState(entry) != entry.state || stackEntryNodeIsMissing(entry) ||
			stackEntryNodeHasError(entry) || stackEntryNodeDirty(entry) {
			continue
		}
		startByte := stackEntryNodeStartByte(entry)
		endByte := stackEntryNodeEndByte(entry)
		if endByte > startByte && endByte <= tokenStartByte {
			return true
		}
	}
	return false
}

// cRecoveryTokenCanClearOrdinaryLeafError requires positive internal-DFA
// provenance for each absorbed token. A region proof cannot authorize a
// later external, generated, error-mode, zero-width, or EOF token.
func cRecoveryTokenCanClearOrdinaryLeafError(tok Token) bool {
	return tok.Symbol != 0 && tok.Symbol != errorSymbol && tok.EndByte > tok.StartByte &&
		tok.lexerInternalDFALexed && !tok.ExternalScannerToken &&
		!tok.lexerErrorModeLexed && !tok.Missing && !tok.NoLookahead
}

// ---------------------------------------------------------------------------
// recover port
// ---------------------------------------------------------------------------

// cRecover ports ts_parser__recover for one absorbing group member. The
// strategy-1 summary scan runs ONCE per token across the whole group (C runs
// it once on its single merged version); the skip-token tail runs per member.
// It may append one strategy-1 recovered fork to *stacks (returned
// forked=true: the fork must act on the current token), absorb the token into
// the open error region (cRecConsumed), accept at EOF (cRecConsumed), or halt
// the member (cRecHalted).
func (p *Parser) cRecover(stacks *[]glrStack, v *glrStack, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool, ParseStopReason) {
	checkStop := func() ParseStopReason {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return reason
		}
		return ParseStopNone
	}
	if reason := checkStop(); reason != ParseStopNone {
		return cRecHalted, false, reason
	}
	rec := v.cRec
	if rec == nil {
		return cRecFallthrough, false, ParseStopNone
	}
	vIndex := -1
	for i := range *stacks {
		if i&63 == 0 {
			if reason := checkStop(); reason != ParseStopNone {
				return cRecHalted, false, reason
			}
		}
		if &(*stacks)[i] == v {
			vIndex = i
			break
		}
	}

	// Strategy 1: recover to a previous state from the group summary in which
	// the lookahead is valid. Elected once per token across the group.
	didRecover := false
	forked := false
	if g := rec.group; g != nil {
		if !g.electionDone || g.electionTokenStart != tok.StartByte || g.electionTokenSymbol != tok.Symbol {
			g.electionDone = true
			g.electionTokenStart = tok.StartByte
			g.electionTokenSymbol = tok.Symbol
			var reason ParseStopReason
			didRecover, forked, reason = p.cRecoverStrategy1Election(stacks, g, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			if reason != ParseStopNone {
				return cRecHalted, forked, reason
			}
			// Re-resolve v: the fork append may have reallocated the slice.
			if vIndex >= 0 {
				v = &(*stacks)[vIndex]
				rec = v.cRec
			}
		}
	}

	// C: if strategy 1 succeeded and there are already too many versions,
	// drop the absorbing version. Count the group as ONE version (C keeps the
	// absorbing paths inside a single merged version).
	if didRecover && p.cEffectiveVersionCount(*stacks, rec.group) > cRecoverMaxVersionCount {
		v.dead = true
		return cRecHalted, forked, ParseStopNone
	}

	// EOF: wrap everything and accept (ts_parser__recover recover_eof).
	if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, forked, reason
		}
		p.cRecoverEOFAccept(v, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		if reason := checkStop(); reason != ParseStopNone {
			return cRecHalted, forked, reason
		}
		return cRecConsumed, forked, ParseStopNone
	}

	// An unlexable-run lookahead while the region already holds content re-runs
	// C's pause→handle_error cycle (no action for the ERROR symbol in the
	// ERROR_STATE row): a fresh NULL discontinuity opens a new error_repeat
	// segment costing one more ERROR_COST_PER_RECOVERY. C's skip-token cost
	// gate below already sees that marker's cost (ts_stack_error_cost of the
	// resumed version), so bump before computing newCost.
	if tok.Symbol == errorSymbol && rec.openErr != nil {
		rec.extraRecoveries++
	}

	// Do not skip the token if doing so would clearly be worse than some
	// existing version.
	tokBytes := uint32(0)
	if tok.EndByte > tok.StartByte {
		tokBytes = tok.EndByte - tok.StartByte
	}
	tokRows := uint32(0)
	if tok.EndPoint.Row > tok.StartPoint.Row {
		tokRows = tok.EndPoint.Row - tok.StartPoint.Row
	}
	newCost := p.cStackErrorCost(v) + cErrCostPerSkippedTree +
		tokBytes*cErrCostPerSkippedChar + tokRows*cErrCostPerSkippedLine
	// C (parser.c ts_parser__recover skip-token gate, parser.c:1346) hardcodes
	// is_in_error=false for this check — not ts_subtree_is_error(lookahead).
	// Passing true here (for error-run lookaheads) made the absorbing version
	// compare under the aggressive in-error rules and die as soon as any
	// recovered fork was marginally cheaper; C keeps the absorber alive (php
	// `static function`: C's absorber survives to EOF and its lineage
	// provides the final `(program php_tag (ERROR ...) compound_statement)`
	// shape).
	if reason := checkStop(); reason != ParseStopNone {
		return cRecHalted, forked, reason
	}
	if vIndex >= 0 && p.cBetterVersionExists(*stacks, vIndex, false, newCost) {
		v.dead = true
		return cRecHalted, forked, ParseStopNone
	}

	// Wrap the lookahead into the open error region (strategy 2).
	if !p.guardRealTokenAttachmentGap(source, v, tok, "c-recover") {
		return cRecHalted, forked, ParseStopNone
	}
	if reason := checkStop(); reason != ParseStopNone {
		return cRecHalted, forked, reason
	}
	p.cAbsorbTokenIntoError(v, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	if reason := checkStop(); reason != ParseStopNone {
		return cRecHalted, forked, reason
	}
	v.shifted = true
	return cRecConsumed, forked, ParseStopNone
}

// cEffectiveVersionCount counts live stacks with the absorbing group folded
// into a single version, mirroring C's version accounting.
func (p *Parser) cEffectiveVersionCount(stacks []glrStack, group *cRecGroup) int {
	count := 0
	members := 0
	for i := range stacks {
		if stacks[i].dead || stacks[i].accepted {
			continue
		}
		if group != nil && stacks[i].cRec != nil && stacks[i].cRec.group == group {
			members++
			continue
		}
		count++
	}
	if members > 0 {
		count++
	}
	return count
}

// cRecoverElectionLookaheadSymbol returns the lookahead symbol C's
// ts_parser__recover strategy-1 summary scan would test. C lexes each stack
// version with its own state's lex mode: an absorbing version sits at
// ERROR_STATE, so its lookahead comes from LexModes[0] (the most permissive
// mode, longest match). This engine lexes ONE shared token per iteration,
// preferring a live normally-parsing stack's state, so during mixed
// normal/absorbing phases the shared token can carry an identity the
// error-mode DFA never produces at the group position (php's context-split
// "(" tokens: the normal-mode id has actions in the anonymous-function state,
// the error-mode id does not). Judging election validity with the shared
// identity generates recovery forks C cannot generate — the over-localized
// recovery-shape family. Re-lex the group position in error mode and use
// THAT identity for the election.
//
// Approximations, documented: the relex is internal-DFA only (C also offers
// the error-mode external scanner first) and skips keyword capture
// post-processing. EOF, wide unlexable runs and missing tokens keep the
// shared symbol (C: `end` / error-subtree lookaheads — the caller's guards
// handle those). When the shared token was already produced by error-mode
// lexing (DFA source with every live stack absorbing) the relex is skipped.
func (p *Parser) cRecoverElectionLookaheadSymbol(source []byte, member *glrStack, tok Token) Symbol {
	if tok.Symbol == 0 || tok.Symbol == errorSymbol || tok.Missing {
		return tok.Symbol
	}
	if p == nil || p.language == nil || member == nil || len(source) == 0 {
		return tok.Symbol
	}
	if p.cRecoverSharedTokenErrorModeLexed {
		return tok.Symbol
	}
	lang := p.language
	if len(lang.LexModes) == 0 || len(lang.LexStates) == 0 {
		return tok.Symbol
	}
	ls := lang.LexModes[0].LexStateIndex()
	if ls == noLookaheadLexState || int(ls) >= len(lang.LexStates) {
		return tok.Symbol
	}
	pos := member.byteOffset
	if pos > tok.StartByte {
		// The shared token begins before the group position (should not
		// happen for the current token); trust the shared identity.
		return tok.Symbol
	}
	if int(pos) >= len(source) {
		return tok.Symbol
	}
	lx := Lexer{
		states:              lang.LexStates,
		asciiTable:          lang.LexAsciiTable(),
		source:              source,
		pos:                 int(pos),
		row:                 cStackPosRow(member),
		immediateTokens:     lang.ImmediateTokens,
		zeroWidthTokens:     lang.ZeroWidthTokens,
		errorRunLexState:    uint32(ls),
		hasErrorRunLexState: true,
	}
	if len(p.included) != 0 && lang.ExternalScanner == nil && len(lang.ExternalSymbols) == 0 {
		lx.setIncludedRanges(p.included)
	}
	relexed := lx.NextWithErrorRuns(uint32(ls))
	if relexed.Symbol == 0 && relexed.StartByte == relexed.EndByte {
		// Whitespace-only tail: C would see `end` here while the shared
		// token disagrees; don't fabricate an EOF election.
		return tok.Symbol
	}
	return relexed.Symbol
}

// cRecoverStrategy1Election runs the C summary scan once per token across all
// absorbing group members, in C's merged-summary order: depth-major, member
// order minor (ts_stack_record_summary's breadth-first traversal of the
// merged version's paths), deduped on (depth, state). At most one fork is
// created, owned by the member whose path carried the elected entry.
//
// C semantics preserved deliberately (parser.c ts_parser__recover):
//   - position / error cost / node-count-since-error come from the ONE merged
//     C version — this engine's first group member (m0);
//   - each summary entry that passes the state / position / would-merge
//     guards is cost-checked BEFORE the lookahead-validity check, and a
//     better existing version ABORTS the whole scan (C `break`), even when
//     the lookahead would have had no actions in that entry's state;
//   - entries are deduped on (depth, state) at first encounter, mirroring
//     ts_stack_record_summary's record-time dedup across the merged paths.
func (p *Parser) cRecoverStrategy1Election(stacks *[]glrStack, group *cRecGroup, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (didRecover, forked bool, reason ParseStopReason) {
	p.recordStrategy1Election()
	checkStop := func() ParseStopReason {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return reason
		}
		return ParseStopNone
	}
	if reason := checkStop(); reason != ParseStopNone {
		return false, false, reason
	}
	// C runs the summary scan for every non-error lookahead INCLUDING the EOF
	// token (parser.c ts_parser__recover: `summary && !ts_subtree_is_error`).
	// EOF election is what lets C pop the open error region to a state where
	// the end symbol is valid and finish with a named root + contained ERROR
	// instead of recover_eof's whole-file ERROR wrap. A wide symbol-0 token is
	// this engine's unlexable run (C: error-subtree lookahead) — C skips
	// strategy 1 for those.
	if tok.Symbol == errorSymbol {
		return false, false, ParseStopNone
	}
	if p.isGraphQLRecoveryTripleQuote(tok.Symbol) {
		return false, false, ParseStopNone
	}
	if tok.Symbol == 0 && tok.StartByte != tok.EndByte {
		return false, false, ParseStopNone
	}
	var localElectionScratch cRecoverElectionScratch
	electionScratch := &localElectionScratch
	var electionScratchBytes int64
	if gssScratch != nil {
		electionScratch = &gssScratch.recoveryElection
		electionScratchBytes = electionScratch.allocatedBytes()
	}
	stateCount := 0
	if p != nil && p.language != nil {
		stateCount = int(p.language.StateCount)
	}
	electionScratch.prepare(len(*stacks), stateCount)
	if gssScratch != nil {
		gssScratch.allocatedBytes += electionScratch.allocatedBytes() - electionScratchBytes
		electionScratchBytes = electionScratch.allocatedBytes()
	}
	members := electionScratch.members
	for i := range *stacks {
		if i&63 == 0 {
			if reason := checkStop(); reason != ParseStopNone {
				return false, false, reason
			}
		}
		if !(*stacks)[i].dead && (*stacks)[i].cRec != nil && (*stacks)[i].cRec.group == group {
			members = append(members, i)
		}
	}
	electionScratch.members = members
	if len(members) == 0 {
		return false, false, ParseStopNone
	}
	electionScratch.prepareMemberCursors(len(members))
	if gssScratch != nil {
		gssScratch.allocatedBytes += electionScratch.allocatedBytes() - electionScratchBytes
	}
	if reason := checkStop(); reason != ParseStopNone {
		return false, false, reason
	}
	cSortRecoverMembersByGroupOrder((*stacks), members)
	// C computes position, error cost and node-count-since-error once for the
	// single merged version; m0 is this engine's stand-in for it.
	m0 := members[0]
	pos := (*stacks)[m0].byteOffset
	curCost := p.cStackErrorCost(&(*stacks)[m0])
	curRow := cStackPosRow(&(*stacks)[m0])
	depthBump := 0
	if p.cNodeCountSinceError(&(*stacks)[m0]) > 0 {
		// C: the open error region occupies one extra non-extra slot above
		// the recorded summary.
		depthBump = 1
	}
	// C's absorbing version lexes its own lookahead in error mode; judge the
	// election with that identity, not the shared normal-mode token's.
	electionSym := p.cRecoverElectionLookaheadSymbol(source, &(*stacks)[m0], tok)
	if electionSym == errorSymbol {
		// C skips strategy 1 for error-subtree lookaheads.
		return false, false, ParseStopNone
	}
	for d := 0; d <= cRecoverMaxSummaryDepth+1; d++ {
		if reason := checkStop(); reason != ParseStopNone {
			return false, false, reason
		}
		iter := electionScratch.beginDepth(d)
		for {
			mi, entry, status := iter.next(*stacks)
			if status == cRecoverElectionIterExhausted {
				break
			}
			if status == cRecoverElectionIterPoll {
				if reason := checkStop(); reason != ParseStopNone {
					return false, false, reason
				}
				continue
			}
			if reason := checkStop(); reason != ParseStopNone {
				return false, false, reason
			}
			if entry.posBytes == pos {
				continue
			}
			depth := int(entry.depth) + depthBump
			// Do not recover in ways that create redundant stack versions.
			wouldMerge := false
			for i := range *stacks {
				if (*stacks)[i].dead || (*stacks)[i].accepted {
					continue
				}
				if (*stacks)[i].top().state == entry.state && (*stacks)[i].byteOffset == pos {
					wouldMerge = true
					break
				}
			}
			if wouldMerge {
				continue
			}
			// C: the cost check runs for every surviving entry — BEFORE
			// the lookahead-validity check — and a better version aborts
			// the entire scan (parser.c `break`), falling through to
			// strategy 2.
			newCost := curCost +
				uint32(entry.depth)*cErrCostPerSkippedTree +
				(pos-entry.posBytes)*cErrCostPerSkippedChar +
				(curRow-entry.posRow)*cErrCostPerSkippedLine
			if p.cBetterVersionExists(*stacks, m0, false, newCost) {
				return false, false, ParseStopNone
			}
			if p.lookupActionIndex(entry.state, electionSym) == 0 {
				continue
			}
			if reason := checkStop(); reason != ParseStopNone {
				return false, false, reason
			}
			if fork, ok := p.cRecoverToState(&(*stacks)[mi], depth, entry.state, arena, entryScratch, gssScratch, trackChildErrors); ok {
				if reason := checkStop(); reason != ParseStopNone {
					return false, false, reason
				}
				fork.branchOrder = (*stacks)[mi].branchOrder
				*stacks = append(*stacks, fork)
				p.recordRecoveryLiveVersions(*stacks)
				if nodeCount != nil {
					*nodeCount = *nodeCount + 1
				}
				if p.glrTrace {
					traceCRecoverToState(entry.state, depth)
				}
				return true, true, ParseStopNone
			}
		}
	}
	return false, false, ParseStopNone
}

func cSortRecoverMembersByGroupOrder(stacks []glrStack, members []int) {
	for i := 1; i < len(members); i++ {
		cur := members[i]
		curOrder := stacks[cur].cRec.groupOrderValue()
		j := i - 1
		for ; j >= 0; j-- {
			prev := members[j]
			if stacks[prev].cRec.groupOrderValue() <= curOrder {
				break
			}
			members[j+1] = prev
		}
		members[j+1] = cur
	}
}

// cRecoverEOFAccept ports the recover_eof tail of ts_parser__recover combined
// with ts_parser__accept's root construction: wrap every subtree on the stack
// (with the open error region's children spliced, mirroring the invisible
// error_repeat flattening) into one ERROR root, and accept.
func (p *Parser) cRecoverEOFAccept(v *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	entries := cStackEntriesTopFirst(v, gssScratch)
	children := make([]*Node, 0, len(entries))
	openErr := (*Node)(nil)
	if v.cRec != nil {
		openErr = v.cRec.openErr
	}
	var rawFirst, rawLast *Node
	for i := len(entries) - 1; i >= 0; i-- {
		if !stackEntryHasNode(entries[i]) {
			continue // the stack base and the error discontinuity
		}
		n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, entries[i], materializeForRecovery, materializeForRecovery)
		if n == nil {
			continue
		}
		if rawFirst == nil {
			rawFirst = n
		}
		rawLast = n
		if n == openErr {
			// Open-region children were visible-spliced at absorb time.
			children = append(children, n.children...)
			continue
		}
		// C parity: recover_eof/accept keep closed ERROR subtrees as-is;
		// only invisible (hidden-symbol) subtrees flatten.
		children = p.cAppendVisibleSplice(children, n)
	}
	root := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, children, 0)
	if rawFirst != nil {
		cSetNodeSpan(root, rawFirst.startByte, rawLast.endByte, rawFirst.startPoint, rawLast.endPoint)
	} else {
		cSetNodeSpan(root, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	}
	root.setHasError(true)
	nodeBumpEquivVersionBeforePublication(root)
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	v.truncate(1)
	v.cRec = nil
	v.cRecoverMissingGroup = nil
	p.pushStackNode(v, 1, root, entryScratch, gssScratch)
	if debugRecoveryCycleChecks {
		debugRecoveryCheckNodeAcyclic(p, arena, "recover-eof-accept-root", root)
	}
	v.accepted = true
	v.shifted = true
}

func cStackPosRow(s *glrStack) uint32 {
	if s == nil {
		return 0
	}
	if len(s.entries) > 0 {
		for i := len(s.entries) - 1; i >= 0; i-- {
			if stackEntryHasNode(s.entries[i]) {
				return stackEntryNodeEndPoint(s.entries[i]).Row
			}
		}
		return 0
	}
	for gn := s.gss.head; gn != nil; gn = gn.prev {
		if stackEntryHasNode(gn.entry) {
			return stackEntryNodeEndPoint(gn.entry).Row
		}
	}
	return 0
}

// cAppendVisibleSplice appends n's visible projection to dst the way the
// engine's reduce does: invisible nodes are spliced to their children at
// build time (C keeps them and hides them at query time instead — same
// visible tree). ERROR and missing nodes always stay.
func (p *Parser) cAppendVisibleSplice(dst []*Node, n *Node) []*Node {
	if n == nil {
		return dst
	}
	if n.symbol == errorSymbol || n.isMissing() || p.cSymbolVisible(n.symbol) {
		return append(dst, n)
	}
	for _, c := range n.children {
		dst = p.cAppendVisibleSplice(dst, c)
	}
	return dst
}

func (p *Parser) cAppendVisibleSpliceUntil(dst []*Node, n *Node, limit int) ([]*Node, bool) {
	if n == nil {
		return dst, true
	}
	if n.symbol == errorSymbol || n.isMissing() || p.cSymbolVisible(n.symbol) {
		if limit >= 0 && len(dst) >= limit {
			return dst, false
		}
		return append(dst, n), true
	}
	for _, c := range n.children {
		var ok bool
		dst, ok = p.cAppendVisibleSpliceUntil(dst, c, limit)
		if !ok {
			return dst, false
		}
	}
	return dst, true
}

// cSetNodeSpan pins a recovery node's span explicitly: C error regions span
// every absorbed subtree, including invisible ones the engine splices away.
func cSetNodeSpan(n *Node, startByte, endByte uint32, startPoint, endPoint Point) {
	n.startByte = startByte
	n.endByte = endByte
	n.startPoint = startPoint
	n.endPoint = endPoint
}

// cRecoverToState ports ts_parser__recover_to_state: pop `depth`
// depth-counting links off a copy of v, splice in any open error region
// children, wrap the popped subtrees (minus trailing extras) into an extra
// ERROR node pushed at the goal state, and re-push the trailing extras.
func (p *Parser) cRecoverToState(v *glrStack, depth int, goal StateID, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (glrStack, bool) {
	entries := cStackEntriesTopFirst(v, gssScratch)
	if len(entries) == 0 {
		return glrStack{}, false
	}
	// Find the cut index: cross `depth` depth-counting links from the top.
	crossed := 0
	cut := -1
	for i := 0; i < len(entries); i++ {
		if crossed == depth {
			cut = i
			break
		}
		if cEntryCountsTowardDepth(entries[i]) {
			crossed++
		}
	}
	if cut < 0 {
		if crossed == depth {
			cut = len(entries)
		} else {
			return glrStack{}, false
		}
	}
	if cut >= len(entries) || entries[cut].state != goal {
		return glrStack{}, false
	}

	// Materialize popped payloads in stack order (base-most first).
	popped := entries[:cut]
	nodes := make([]*Node, 0, len(popped))
	for i := len(popped) - 1; i >= 0; i-- {
		if !stackEntryHasNode(popped[i]) {
			continue // the error discontinuity
		}
		n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, popped[i], materializeForRecovery, materializeForRecovery)
		if n == nil {
			return glrStack{}, false
		}
		nodes = append(nodes, n)
	}

	// Split trailing extras (re-pushed after the ERROR per C).
	end := len(nodes)
	for end > 0 && nodes[end-1].isExtra() {
		end--
	}
	wrapped := nodes[:end]
	trailing := nodes[end:]

	// Flatten the open error region (C splices popped error subtrees /
	// keeps error_repeat chains invisible; the Go equivalent is splicing the
	// open ERROR node's children) and splice invisible nodes the way the
	// engine's reduce does. The raw popped extent pins the ERROR span (C
	// error regions cover invisible subtrees too).
	children := make([]*Node, 0, len(wrapped)+2)
	openErr := (*cRecoverState)(nil)
	if v.cRec != nil {
		openErr = v.cRec
	}
	var rawFirst, rawLast *Node
	for _, n := range wrapped {
		if rawFirst == nil {
			rawFirst = n
		}
		rawLast = n
		if openErr != nil && n == openErr.openErr {
			// Open-region children were visible-spliced at absorb time.
			children = append(children, n.children...)
			continue
		}
		// C parity: popped closed subtrees (ERROR carriers included) keep
		// their identity inside the new ERROR; only invisible subtrees
		// flatten.
		children = p.cAppendVisibleSplice(children, n)
	}

	fork := v.cloneWithScratch(gssScratch)
	if workCountInstrumentationEnabled {
		workCountTopologyRecordVersionCopy(v, &fork)
	}
	fork.cRec = nil
	fork.cRecoverMissingGroup = nil
	fork.dead = false
	fork.shifted = false
	// This recovered fork clears cRec (above) and may later reset its baseline,
	// but it still descends from error wreckage and is about to wrap popped
	// content in a real ERROR node. Keep the sticky bit set so it cannot pass
	// the php gate two tokens later looking pristine. cloneWithScratch already
	// copied it from v; this is the belt-and-suspenders entry-funnel site. See
	// glrStack.cEverErrored.
	fork.cEverErrored = true
	keepDepth := len(entries) - cut
	if !fork.truncate(keepDepth) {
		return glrStack{}, false
	}
	// C also pops a directly-preceding closed ERROR subtree and splices its
	// children in front (ts_stack_pop_error).
	if top := stackEntryNode(fork.top()); top != nil && top.symbol == errorSymbol && !top.isMissing() && fork.depth() > 1 {
		prev := top
		if fork.truncate(fork.depth() - 1) {
			children = append(append(make([]*Node, 0, len(prev.children)+len(children)), prev.children...), children...)
			if rawFirst == nil {
				rawLast = prev
			}
			rawFirst = prev
		}
	}

	if rawFirst != nil {
		errNode := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, children, 0)
		cSetNodeSpan(errNode, rawFirst.startByte, rawLast.endByte, rawFirst.startPoint, rawLast.endPoint)
		errNode.setHasError(true)
		errNode.setExtra(true)
		errNode.preGotoState = goal
		errNode.parseState = goal
		nodeBumpEquivVersionBeforePublication(errNode)
		if perfCountersEnabled {
			perfRecordErrorNode()
		}
		if trackChildErrors != nil {
			*trackChildErrors = true
		}
		// This fork now genuinely carries an ERROR node (real C's
		// recover_to_state always wraps something). If it reaches ACCEPT
		// without ever cycling back through cHandleError to be
		// re-validated by another cost competition, cStackErrorCost cannot
		// legitimately have dropped to zero — see the ACCEPT-time check in
		// buildResultFromGLR. Track this only for:
		//   - single-stack dead ends (see crecoveryHandleErrorSingleStack): a
		//     fork born while several unrelated GLR ambiguity candidates were
		//     already live is normal disambiguation, not the swallowed-error
		//     defect class (e.g. kotlin's generic-vs-comparison forks).
		//   - a small absolute recovered span (see
		//     crecoverySwallowedErrorMaxFallbackErrorBytes, reused here for
		//     the same "local, not whole-construct, recovery" rationale): an
		//     adversarial review found cRecoverToState firing on ordinary,
		//     syntactically valid Go source — a real repo file's own
		//     composite-literal/statement disambiguation can drive a
		//     hundreds-of-bytes single-stack strategy-1 recovery that the
		//     rest of the (entirely valid) parse absorbs losslessly, the
		//     same "LALR table gap resolved without a real error" pattern as
		//     eds's legitimate empty-value production. The confirmed
		//     defect-class fixtures recover a single malformed token or
		//     directive (java 6 bytes, gomod 5 bytes) — nowhere near that
		//     scale.
		if p.crecoveryHandleErrorSingleStack &&
			errNode.endByte-errNode.startByte <= crecoverySwallowedErrorMaxFallbackErrorBytes {
			fork.cRecoveryUnvalidatedMarker = true
		}
		p.pushStackNode(&fork, goal, errNode, entryScratch, gssScratch)
		if debugRecoveryCycleChecks {
			debugRecoveryCheckNodeAcyclic(p, arena, "recover-to-state-wrap", errNode)
		}
	}
	for _, ex := range trailing {
		p.pushStackNode(&fork, goal, ex, entryScratch, gssScratch)
	}
	return fork, true
}

// cAbsorbTokenIntoError ports the strategy-2 tail of ts_parser__recover:
// mark extra-shiftable tokens extra (excluded from error cost), then fold the
// token into the open error region at ERROR_STATE.
func (p *Parser) cAbsorbTokenIntoError(v *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	// Invisible tokens stay out of the visible children (the engine splices
	// invisibles at build time; C hides them at query time) but still extend
	// the error region's span.
	leafVisible := p.cSymbolVisible(tok.Symbol)
	var leaf *Node
	if leafVisible {
		leaf = newLeafNodeInArena(arena, tok.Symbol, tok.Symbol == errorSymbol || p.isNamedSymbol(tok.Symbol),
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		// C marks the enclosing ERROR node as erroneous. Keep ordinary leaves
		// clean when the region has direct internal-lexer provenance.
		clearLeafError := v.cRec.clearsOrdinaryLeafErrors() &&
			cRecoveryTokenCanClearOrdinaryLeafError(tok)
		if !clearLeafError && tok.Symbol != errorSymbol {
			leaf.setHasError(true)
		}
		if tok.Symbol == errorSymbol && !tok.lexerErrorModeLexed {
			leaf.setHasError(true)
		}
		// C: if the token shifts as extra in state 1, mark it extra so it is
		// not counted in error cost calculations.
		if idx := p.lookupActionIndex(1, tok.Symbol); idx != 0 && int(idx) < len(p.language.ParseActions) {
			if actions := p.language.ParseActions[idx].Actions; len(actions) > 0 {
				if last := actions[len(actions)-1]; last.Type == ParseActionShift && last.Extra {
					leaf.setExtra(true)
				}
			}
		}
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}

	appendLeaf := func(dst []*Node) []*Node {
		if leaf != nil {
			return append(dst, leaf)
		}
		return dst
	}

	rec := v.cRec
	if rec != nil && rec.openErr != nil {
		top := stackEntryNode(v.top())
		if top == rec.openErr {
			pre := p.cErrRegionPreAbsorb(rec.openErr)
			rec.openErr.children = appendLeaf(rec.openErr.children)
			rec.openErr.endByte = tok.EndByte
			rec.openErr.endPoint = tok.EndPoint
			nodeBumpEquivVersion(rec.openErr)
			p.cErrRegionPostAbsorb(pre, leaf)
			if debugRecoveryCycleChecks {
				debugRecoveryCheckNodeAcyclic(p, arena, "absorb-extend-top", rec.openErr)
			}
			if v.byteOffset < tok.EndByte {
				v.byteOffset = tok.EndByte
			}
			if nodeCount != nil {
				*nodeCount = *nodeCount + 1
			}
			return
		}
		// Extras were pushed above the open error region (C pops the previous
		// error_repeat plus trailing extras and re-wraps them together).
		entries := cStackEntriesTopFirst(v, gssScratch)
		above := 0
		found := false
		for i := 0; i < len(entries); i++ {
			if stackEntryNode(entries[i]) == rec.openErr {
				above = i
				found = true
				break
			}
		}
		if found {
			extras := make([]*Node, 0, above)
			for i := above - 1; i >= 0; i-- {
				n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, entries[i], materializeForRecovery, materializeForRecovery)
				if n == nil {
					found = false
					break
				}
				extras = p.cAppendVisibleSplice(extras, n)
			}
			if found && v.truncate(len(entries)-above) {
				pre := p.cErrRegionPreAbsorb(rec.openErr)
				rec.openErr.children = append(rec.openErr.children, extras...)
				rec.openErr.children = appendLeaf(rec.openErr.children)
				rec.openErr.endByte = tok.EndByte
				rec.openErr.endPoint = tok.EndPoint
				nodeBumpEquivVersion(rec.openErr)
				added := extras
				if leaf != nil {
					added = append(added, leaf)
				}
				p.cErrRegionPostAbsorb(pre, added...)
				if debugRecoveryCycleChecks {
					debugRecoveryCheckNodeAcyclic(p, arena, "absorb-fold-extras", rec.openErr)
				}
				if v.byteOffset < tok.EndByte {
					v.byteOffset = tok.EndByte
				}
				if nodeCount != nil {
					*nodeCount = *nodeCount + 1
				}
				return
			}
		}
	}

	var errChildren []*Node
	if leaf != nil {
		errChildren = []*Node{leaf}
	}
	errNode := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, errChildren, 0)
	cSetNodeSpan(errNode, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	errNode.setHasError(true)
	errNode.parseState = cErrorState
	nodeBumpEquivVersionBeforePublication(errNode)
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	p.pushStackNode(v, cErrorState, errNode, entryScratch, gssScratch)
	if rec != nil {
		rec.openErr = errNode
	}
	p.cErrRegionPrime(errNode)
	if debugRecoveryCycleChecks {
		debugRecoveryCheckNodeAcyclic(p, arena, "absorb-new-region", errNode)
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 2
	}
}

// ---------------------------------------------------------------------------
// Dispatch hooks
// ---------------------------------------------------------------------------

// cRecoverDispatchInError intercepts dispatch for a stack already in the
// error state (C: the ERROR_STATE table row). Shiftable tokens fall through
// to the normal dispatch — extras shift in ERROR_STATE without extending the
// error, and non-terminal extras (e.g. requirements' linebreak) enter their
// sub-parse via a real shift in the state-0 row. Everything else goes through
// ts_parser__recover.
func (p *Parser) cRecoverDispatchInError(stacks *[]glrStack, si int, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool, ParseStopReason) {
	s := &(*stacks)[si]
	if s.top().state != cErrorState {
		// Mid non-terminal-extra parse: the version temporarily left
		// ERROR_STATE; C dispatches it through the normal table rows.
		return cRecFallthrough, false, ParseStopNone
	}
	if tok.Symbol != 0 {
		if p.isGraphQLRecoveryTripleQuote(tok.Symbol) {
			return p.cRecover(stacks, s, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		}
		if idx := p.lookupActionIndex(cErrorState, tok.Symbol); idx != 0 && int(idx) < len(p.language.ParseActions) {
			if actions := p.language.ParseActions[idx].Actions; len(actions) > 0 &&
				actions[0].Type == ParseActionShift {
				return cRecFallthrough, false, ParseStopNone
			}
		}
		// Zero-width non-EOF tokens are skipped (C's error-mode lexer never
		// returns empty internal tokens; the Go DFA source can). Record them
		// in the open ERROR when possible; the token source owns cursor
		// progress for true zero-width tokens.
		if tok.StartByte == tok.EndByte {
			if s.cRec != nil && s.cRec.openErr != nil && arena != nil {
				p.cAbsorbTokenIntoError(s, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			} else if s.byteOffset < tok.EndByte {
				s.byteOffset = tok.EndByte
			}
			s.shifted = true
			return cRecConsumed, false, ParseStopNone
		}
	}
	return p.cRecover(stacks, s, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
}

func (p *Parser) isGraphQLRecoveryTripleQuote(sym Symbol) bool {
	return p != nil &&
		p.language != nil &&
		p.language.Name == "graphql" &&
		int(sym) < len(p.language.SymbolNames) &&
		p.language.SymbolNames[sym] == "\"\"\""
}

// cCondenseAndResume ports ts_parser__condense_stack for the gated grammar:
// remove halted versions, remove versions that clearly lose the error-cost
// competition, order survivors most-promising-first, enforce
// MAX_VERSION_COUNT, and resume the best paused version (ts_parser__handle_error)
// when no unpaused version outranks it. Merging identical stacks remains the
// job of the regular mergeStacks pass. Only runs when some stack is paused or
// in the error state, so clean parses keep today's behavior exactly.
//
// Returns the condensed slice, whether new versions need to re-dispatch
// the current token (strategy-1 forks / missing-token versions created by a
// resumed handle_error), the possibly error-mode-relexed current token
// (see cRecoverResumeLookahead) which the caller must adopt so redispatched
// versions act on the same lookahead the resumed group consumed, and any
// active budget/timeout stop reason encountered while condensing.
func (p *Parser) cCondenseAndResume(stacks []glrStack, source []byte, ts TokenSource, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, parseScratch *parserScratch, trackChildErrors *bool) ([]glrStack, bool, Token, ParseStopReason) {
	checkStop := func() ParseStopReason {
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return reason
		}
		return ParseStopNone
	}
	if reason := checkStop(); reason != ParseStopNone {
		return stacks, false, tok, reason
	}
	relevant := false
	for i := range stacks {
		if stacks[i].cPaused || stacks[i].cRec != nil || stacks[i].cRecoverMissingGroup != nil {
			relevant = true
			break
		}
	}
	if debugRecoveryCycleChecks && relevant {
		for i := range stacks {
			if reason := checkStop(); reason != ParseStopNone {
				return stacks, false, tok, reason
			}
			if stacks[i].dead {
				continue
			}
			entries := cStackEntriesTopFirst(&stacks[i], gssScratch)
			debugRecoveryCheckSpineAcyclic(p, arena, fmt.Sprintf("condense-stack-%d-byte-%d", i, stacks[i].byteOffset), entries)
		}
	}
	if !relevant {
		return stacks, false, tok, ParseStopNone
	}
	// Drop dead versions first (C removes halted versions in condense).
	// Accepted versions have left the pool in C (ts_parser__accept stashes
	// the tree and removes the version): they sit out the cost competition,
	// the ordering, and the version cap, and rejoin only for final result
	// selection.
	var acceptedStacks []glrStack
	alive := stacks[:0]
	for i := range stacks {
		if reason := checkStop(); reason != ParseStopNone {
			return stacks, false, tok, reason
		}
		if stacks[i].dead {
			continue
		}
		if stacks[i].accepted {
			acceptedStacks = append(acceptedStacks, stacks[i])
			continue
		}
		alive = append(alive, stacks[i])
	}
	stacks = alive
	// No stack payloads are inserted during the pairwise phase, so the sticky
	// construction proof cannot change until the resume phase below.
	subtreeCostRelevant := trackChildErrors == nil || *trackChildErrors
	statusProbe := 0
	for i := 1; i < len(stacks); i++ {
		if reason := checkStop(); reason != ParseStopNone {
			return stacks, false, tok, reason
		}
		statusI := p.cCondenseVersionStatus(&stacks[i], subtreeCostRelevant)
		for j := 0; j < i; j++ {
			statusProbe++
			if statusProbe&63 == 0 {
				if reason := checkStop(); reason != ParseStopNone {
					return stacks, false, tok, reason
				}
			}
			if cRecoverVersionsSameGroup(stacks[j], stacks[i]) {
				continue
			}
			if cRecoverVersionShouldStayBefore(stacks[j], stacks[i]) {
				continue
			}
			if cRecoverVersionShouldStayBefore(stacks[i], stacks[j]) {
				stacks[i], stacks[j] = stacks[j], stacks[i]
				statusI = p.cCondenseVersionStatus(&stacks[i], subtreeCostRelevant)
				continue
			}
			statusJ := p.cCondenseVersionStatus(&stacks[j], subtreeCostRelevant)
			switch p.cCompareCondenseVersions(statusJ, statusI, &stacks[j], &stacks[i]) {
			case cErrorComparisonTakeLeft:
				if p.glrTrace {
					p.traceCCondenseDrop("take-left", i, j, stacks[i], stacks[j],
						p.cVersionStatusForTrace(&stacks[i], statusI),
						p.cVersionStatusForTrace(&stacks[j], statusJ))
				}
				stacks = append(stacks[:i], stacks[i+1:]...)
				i--
				j = i
			case cErrorComparisonPreferRight:
				if p.glrTrace {
					p.traceCCondenseSwap("prefer-right", i, j, stacks[i], stacks[j],
						p.cVersionStatusForTrace(&stacks[i], statusI),
						p.cVersionStatusForTrace(&stacks[j], statusJ))
				}
				stacks[i], stacks[j] = stacks[j], stacks[i]
				statusI = p.cCondenseVersionStatus(&stacks[i], subtreeCostRelevant)
			case cErrorComparisonTakeRight:
				if p.glrTrace {
					p.traceCCondenseDrop("take-right", j, i, stacks[j], stacks[i],
						p.cVersionStatusForTrace(&stacks[j], statusJ),
						p.cVersionStatusForTrace(&stacks[i], statusI))
				}
				stacks = append(stacks[:j], stacks[j+1:]...)
				i--
				j--
				statusI = p.cCondenseVersionStatus(&stacks[i], subtreeCostRelevant)
			}
			if i < 1 {
				break
			}
		}
	}
	if len(stacks) > cRecoverMaxVersionCount {
		// C's ts_parser__condense_stack MERGES merge-equivalent versions
		// (ts_stack_merge: same head state, byte position, error cost, and
		// external scanner state) during the pairwise loop, BEFORE the
		// MAX_VERSION_COUNT truncation — the surviving histories live on as
		// extra links of the merged GSS head. So C's cap only ever counts
		// DISTINCT (state, position, cost) versions. This port keeps
		// merge-equivalent stacks as separate versions (each carrying one of
		// the histories C would fold into links), so a raw positional
		// truncation at cRecoverMaxVersionCount can kill every copy of a
		// distinct grammar interpretation while retaining redundant
		// duplicates of another. Observed on c_sharp (precise-ELS election,
		// DeclaredTypeManager.cs): six merge-equivalent missing-group stacks
		// + a 2-way `.` shift conflict forked 6->12, and the positional trim
		// kept the six duplicates of one interpretation while dropping all
		// six copies of the other — misparsing a switch-expression arm
		// (`DeclaredSymbol declaredSymbol => ErrorType.Create(...)`, line
		// 578) that the C oracle parses clean. Enforce the version window
		// the way C effectively does: bound the number of DISTINCT merge
		// keys at cRecoverMaxVersionCount (dropping all stacks of the
		// least-promising excess keys), and leave same-key duplicates — C's
		// merged links — to the engine's own boundary merge/cull population
		// discipline.
		keyRanks := p.cCondenseVersionKeyRanks
		if keyRanks == nil {
			keyRanks = make(map[cCondenseVersionKey]uint8, cRecoverMaxVersionCount)
			p.cCondenseVersionKeyRanks = keyRanks
		} else {
			clear(keyRanks)
		}
		for i := 0; i < len(stacks); i++ {
			if reason := checkStop(); reason != ParseStopNone {
				return stacks, false, tok, reason
			}
			key := p.cCondenseVersionKeyFor(&stacks[i])
			if cCondenseVersionRankFor(keyRanks, key) < cRecoverMaxVersionCount {
				continue
			}
			if p.glrTrace {
				p.traceCCondenseTrim(i, stacks[i])
			}
			stacks = append(stacks[:i], stacks[i+1:]...)
			i--
		}
	}

	// Resume the best paused version; remove the rest (C condense tail).
	//
	// C's single dispatch loop keeps every version at the same token position,
	// so at most one paused version ever reaches condense out of sync with its
	// siblings. This engine's per-token settle step can occasionally let two
	// versions arrive at condense both paused (e.g. a reduce-only version that
	// still needs to recheck a not-yet-advanced token alongside a sibling that
	// already shifted past it and paused one token later). When that
	// happens, the higher-priority (post-sort index 0) candidate can be
	// "stale": its position no longer matches the current token, so
	// guardRealTokenAttachmentGap halts it immediately. Faithfully resuming
	// only that one and dropping every other paused version (as plain
	// ts_parser__condense_stack does when it only ever sees one paused
	// version) then discards a still-viable sibling and the parse dies with
	// no recovery at all. Fall through to the next paused candidate whenever
	// a resume halts, instead of unconditionally dropping the remaining
	// paused versions after the first attempt — this only ever recovers
	// MORE input (a halt previously meant giving up outright), so it cannot
	// regress an already-successful single-candidate resume.
	needsRedispatch := false
	hasUnpaused := false
	for i := 0; i < len(stacks); i++ {
		if reason := checkStop(); reason != ParseStopNone {
			return stacks, false, tok, reason
		}
		if !stacks[i].cPaused {
			hasUnpaused = true
			continue
		}
		if !hasUnpaused {
			if p.glrTrace {
				fmt.Printf("      -> C-RESUME stack=%d state=%d byte=%d\n", i, stacks[i].top().state, stacks[i].byteOffset)
			}
			// C's pause lookahead already went through ts_parser__lex's
			// error-mode fallback; a custom source's normal-mode token must
			// be substituted the same way before handle_error consumes it.
			if replacement, replaced := p.cRecoverResumeLookahead(ts, source, &stacks[i], tok, parseScratch); replaced {
				if p.glrTrace {
					fmt.Printf("      -> C-RESUME-RELEX sym=%d [%d-%d] -> sym=%d [%d-%d]\n",
						tok.Symbol, tok.StartByte, tok.EndByte,
						replacement.Symbol, replacement.StartByte, replacement.EndByte)
				}
				tok = replacement
			}
			if reason := checkStop(); reason != ParseStopNone {
				return stacks, false, tok, reason
			}
			outcome, redispatch, reason := p.cHandleError(&stacks, i, source, tok, nodeCount, arena, entryScratch, gssScratch, tmpEntries, trackChildErrors)
			if reason != ParseStopNone {
				return stacks, false, tok, reason
			}
			if reason := checkStop(); reason != ParseStopNone {
				return stacks, false, tok, reason
			}
			if redispatch {
				needsRedispatch = true
			}
			if outcome == cRecHalted {
				stacks[i].dead = true
				// Keep hasUnpaused false: give the next still-paused
				// candidate (if any) a chance to resume instead of dropping
				// it unattempted.
				continue
			}
			hasUnpaused = true
			continue
		}
		stacks = append(stacks[:i], stacks[i+1:]...)
		i--
	}
	stacks = append(stacks, acceptedStacks...)
	return stacks, needsRedispatch, tok, ParseStopNone
}

func (p *Parser) traceCCondenseDrop(reason string, dropIndex, keepIndex int, drop, keep glrStack, dropStatus, keepStatus cErrorStatus) {
	fmt.Printf("      -> C-CONDENSE-DROP reason=%s drop=%d %s %s keep=%d %s %s\n",
		reason,
		dropIndex,
		cRecoverStackTraceKind(&drop),
		cCondenseStackTraceSummary(drop, dropStatus),
		keepIndex,
		cRecoverStackTraceKind(&keep),
		cCondenseStackTraceSummary(keep, keepStatus),
	)
}

func (p *Parser) traceCCondenseSwap(reason string, i, j int, left, right glrStack, leftStatus, rightStatus cErrorStatus) {
	fmt.Printf("      -> C-CONDENSE-SWAP reason=%s i=%d %s %s j=%d %s %s\n",
		reason,
		i,
		cRecoverStackTraceKind(&left),
		cCondenseStackTraceSummary(left, leftStatus),
		j,
		cRecoverStackTraceKind(&right),
		cCondenseStackTraceSummary(right, rightStatus),
	)
}

func (p *Parser) traceCCondenseTrim(index int, stack glrStack) {
	fmt.Printf("      -> C-CONDENSE-TRIM index=%d %s state=%d byte=%d depth=%d score=%d\n",
		index,
		cRecoverStackTraceKind(&stack),
		stack.top().state,
		stack.byteOffset,
		stack.depth(),
		stack.score,
	)
}

func cCondenseStackTraceSummary(stack glrStack, status cErrorStatus) string {
	return fmt.Sprintf("{state:%d byte:%d depth:%d score:%d cost:%d inErr:%v dyn:%d nodes:%d}",
		stack.top().state,
		stack.byteOffset,
		stack.depth(),
		stack.score,
		status.cost,
		status.isInError,
		status.dynPrec,
		status.nodeCount,
	)
}

func cRecoverVersionsSameGroup(a, b glrStack) bool {
	return a.cRec != nil &&
		b.cRec != nil &&
		a.cRec.group != nil &&
		a.cRec.group == b.cRec.group
}

// cCondenseVersionKey identifies a stack version group under C's
// ts_stack_can_merge (stack.c): active status, head state, byte position,
// and error cost. C additionally requires equal external scanner state; this
// port's lockstep token loop keeps all live stacks on the same lookahead, so
// same-byte heads share the token source's external state by construction.
// Port-conservative extras C does not key on: dynamic-precedence score
// (score breaks final-selection ties here, so unequal-score stacks are not
// interchangeable), shifted phase, and recovery-group identity (cRec.group /
// cRecoverMissingGroup pointers).
type cCondenseVersionKey struct {
	state        StateID
	byteOffset   uint32
	cost         uint32
	score        int
	shifted      bool
	paused       bool
	recGroup     *cRecGroup
	missingGroup *cRecGroup
}

// cCondenseVersionRankFor returns the same cap decision as the legacy rank
// map. It stores only ranks below the cap, because every excess rank is
// dropped and every later copy of an excess key is dropped too.
func cCondenseVersionRankFor(ranks map[cCondenseVersionKey]uint8, key cCondenseVersionKey) int {
	rank, seen := ranks[key]
	if seen {
		return int(rank)
	}
	rank = uint8(len(ranks))
	if rank < cRecoverMaxVersionCount {
		ranks[key] = rank
	}
	return int(rank)
}

func (p *Parser) cCondenseVersionKeyFor(s *glrStack) cCondenseVersionKey {
	key := cCondenseVersionKey{
		state:        s.top().state,
		byteOffset:   s.byteOffset,
		cost:         p.cStackErrorCost(s),
		score:        s.score,
		shifted:      s.shifted,
		paused:       s.cPaused,
		missingGroup: s.cRecoverMissingGroup,
	}
	if s.cRec != nil {
		key.recGroup = s.cRec.group
	}
	return key
}

func cRecoverVersionShouldStayBefore(a, b glrStack) bool {
	if a.dead || b.dead || a.accepted || b.accepted {
		return false
	}
	if a.cPaused || b.cPaused {
		return false
	}
	return a.cRec != nil && a.cRec.group != nil && b.cRec == nil && b.cRecoverMissingGroup == a.cRec.group
}

// cAcceptRootRebuild ports the root construction half of ts_parser__accept:
// C pops the whole accepted stack, finds the TOPMOST non-extra subtree, and
// rebuilds a node with that subtree's symbol whose children are every other
// popped subtree spliced around its own children — so trailing extras (a
// comment after the last rule of a stylesheet, the EOF padding) become
// children OF the root instead of siblings. Without this, the engine's root
// builder sees multiple top-level nodes on an accepted stack and wraps them
// in a synthetic ERROR root (the css "collapse" shape: stylesheet{ERROR{...}}
// where C has stylesheet{...}).
//
// The stack becomes [base, rebuiltRoot]. Stacks that already hold a single
// payload node are left untouched (the no-error fast path and
// cRecoverEOFAccept results).
func (p *Parser) cAcceptRootRebuild(s *glrStack, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch) ParseStopReason {
	if p == nil || s == nil || !s.accepted {
		return ParseStopNone
	}
	entries := cStackEntriesTopFirst(s, gssScratch)
	payloads := 0
	for i := range entries {
		if stackEntryHasNode(entries[i]) {
			payloads++
		}
	}
	if payloads <= 1 {
		return ParseStopNone
	}
	// Materialize base-most first, mirroring C's popped subtree order.
	nodes := make([]*Node, 0, payloads)
	for i := len(entries) - 1; i >= 0; i-- {
		if !stackEntryHasNode(entries[i]) {
			continue // stack base / error discontinuity
		}
		n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, entries[i], materializeForRecovery, materializeForRecovery)
		if n == nil {
			return ParseStopNone
		}
		nodes = append(nodes, n)
	}
	// C: the topmost non-extra subtree names the root.
	rootIdx := -1
	for j := len(nodes) - 1; j >= 0; j-- {
		if !nodes[j].isExtra() {
			rootIdx = j
			break
		}
	}
	if rootIdx < 0 {
		return ParseStopNone
	}
	cand := nodes[rootIdx]
	childLimit := cAcceptRootRebuildChildLimit(arena)
	initialCap := min(len(nodes)-1+len(cand.children), 1024)
	children := make([]*Node, 0, initialCap)
	for _, n := range nodes[:rootIdx] {
		var ok bool
		children, ok = p.cAppendVisibleSpliceUntil(children, n, childLimit)
		if !ok {
			return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
		}
	}
	candidateOffset := len(children)
	var ok bool
	children, ok = cAppendNodeSliceUntil(children, cand.children, childLimit)
	if !ok {
		return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
	}
	for _, n := range nodes[rootIdx+1:] {
		var ok bool
		children, ok = p.cAppendVisibleSpliceUntil(children, n, childLimit)
		if !ok {
			return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
		}
	}
	if arena != nil && arena.budgetBytes > 0 {
		used := arena.allocatedBytes - arena.budgetBaselineBytes
		if used < 0 {
			used = 0
		}
		if used+childSliceBytesForCap(len(children)) >= arena.budgetBytes {
			return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
		}
	}
	children = cloneNodeSliceInArena(arena, children)
	fieldIDs, fieldSources := cAcceptRootFieldMetadata(
		arena,
		len(children),
		candidateOffset,
		cand,
	)
	root := p.newRecoveryParentNodeInArena(arena, cand.symbol, p.isNamedSymbol(cand.symbol), children, cand.productionID)
	root.setFieldMetadata(fieldIDs, fieldSources)
	root.rawShape = captureRawShapeForNodeSlice(arena, cand.symbol, cand.productionID, children)
	root.dynamicPrecedence = nodeSliceDynamicPrecedence(children)
	first, last := nodes[0], nodes[len(nodes)-1]
	cSetNodeSpan(root, first.startByte, last.endByte, first.startPoint, last.endPoint)
	hasErr := false
	for _, c := range children {
		if c != nil && c.hasError() {
			hasErr = true
			break
		}
	}
	if hasErr || cand.hasError() {
		root.setHasError(true)
	}
	nodeBumpEquivVersionBeforePublication(root)
	if !s.truncate(1) {
		return ParseStopNone
	}
	p.pushStackNode(s, 1, root, entryScratch, gssScratch)
	if debugRecoveryCycleChecks {
		debugRecoveryCheckNodeAcyclic(p, arena, "accept-root-rebuild", root)
	}
	return ParseStopNone
}

func cAcceptRootFieldMetadata(
	arena *nodeArena,
	childCount int,
	candidateOffset int,
	candidate *Node,
) ([]FieldID, []uint8) {
	if candidate == nil || childCount <= 0 || candidateOffset < 0 {
		return nil, nil
	}
	candidateFields := candidate.fieldIDs()
	if len(candidateFields) == 0 ||
		!fieldIDSliceHasAny(candidateFields) ||
		candidateOffset+len(candidateFields) > childCount {
		return nil, nil
	}
	fieldIDs := arena.allocFieldIDSlice(childCount)
	copy(fieldIDs[candidateOffset:], candidateFields)
	candidateSources := candidate.fieldSources()
	if len(candidateSources) == 0 {
		return fieldIDs, nil
	}
	fieldSources := arena.allocFieldSourceSlice(childCount)
	copy(fieldSources[candidateOffset:], candidateSources)
	return fieldIDs, fieldSources
}

func cAcceptRootRebuildChildLimit(arena *nodeArena) int {
	if arena == nil || arena.budgetBytes <= 0 {
		return -1
	}
	used := arena.allocatedBytes - arena.budgetBaselineBytes
	if used < 0 {
		used = 0
	}
	remaining := arena.budgetBytes - used
	if remaining <= 0 {
		return 0
	}
	perChildPeakBytes := childSliceBytesForCap(1) * 2
	if perChildPeakBytes <= 0 {
		return -1
	}
	limit := remaining / perChildPeakBytes
	maxInt := int64(^uint(0) >> 1)
	if limit > maxInt {
		return -1
	}
	return int(limit)
}

func cAppendNodeSliceUntil(dst, src []*Node, limit int) ([]*Node, bool) {
	if limit >= 0 && len(dst)+len(src) > limit {
		return dst, false
	}
	return append(dst, src...), true
}

func nodeSliceDynamicPrecedence(children []*Node) int32 {
	var dyn int32
	for _, child := range children {
		if child != nil {
			dyn += child.dynamicPrecedence
		}
	}
	return dyn
}

func captureRawShapeForNodeSlice(arena *nodeArena, symbol Symbol, productionID uint16, children []*Node) rawShapeRef {
	if arena == nil || len(children) == 0 {
		return 0
	}
	entries := make([]stackEntry, 0, len(children))
	for _, child := range children {
		if child != nil {
			entries = append(entries, newStackEntryNode(child.parseState, child))
		}
	}
	return captureRawShapeForEntries(arena, symbol, productionID, entries)
}

func captureRawShapeForEntries(arena *nodeArena, symbol Symbol, productionID uint16, entries []stackEntry) rawShapeRef {
	if arena == nil || len(entries) == 0 {
		return 0
	}
	var p Parser
	// C-recovery error-path shape construction is out of scope for the
	// single-stack elision (the RCA's reader inventory already gates every
	// consumer on recovery among other things): nil never elides, preserving
	// this lane's existing unconditional-capture behavior.
	return p.captureRawShape(nil, arena, symbol, productionID, entries, 0, len(entries))
}

// cStackResultErrorCost is the result-selection cost: the error cost of the
// stack's would-be tree (requirement 4 of the spec: fold error cost into
// stackCompareForResultSelection).
func (p *Parser) cStackResultErrorCost(s *glrStack) uint32 {
	return p.cStackErrorCost(s)
}

// cTreeErrorCost computes the C error cost over a finished tree, for
// retry-selection integration (preferRetryTree).
func (p *Parser) cTreeErrorCost(t *Tree) uint32 {
	if t == nil || t.root == nil {
		return 0
	}
	return p.cNodeErrorCost(t.root)
}

func traceCRecoverToState(state StateID, depth int) {
	fmt.Printf("      -> C-RECOVER-TO-STATE state=%d depth=%d\n", state, depth)
}

// newRecoveryParentNodeInArena builds a recovery-construction parent node
// (ERROR wrappers, recover_to_state splice wrappers, EOF-accept and
// accept-rebuild roots) without corrupting the transient-parent sentinel.
//
// Recovery parents wrap children materialized straight off stack entries. In
// the fresh-parse regime those children can be TRANSIENT reduce parents
// (transientParentScratch slab nodes), whose .parent field doubles as the
// result-time materializer's {nil: unvisited, self: in-progress, other: arena
// clone} map (transient_parents.go materializeNodesUntil /
// transientReplacement). Wiring eager parent links into such a child — what
// newParentNodeInArena's populateParentNode does — corrupts that map:
// materializeNodesUntil then skips the child as "already cloned", and
// transientReplacement substitutes the child's TREE PARENT for the child,
// linking the new parent under itself. That self back-edge is the
// cyclic-transient-tree defect: every subsequent full-tree walk
// (wireParentLinksWithScratchUntil, the walkResultTree normalizer family)
// hangs or stack-overflows (go zerrors_windows.go truncations with recovery
// engaged; the trailing-EOF shape of issue #110).
//
// Transient parents only exist on fresh parses (shouldUseTransientReduceParents
// requires reuse == nil && oldTree == nil), and exactly those parses wire
// parent links from the root in finalizeResultRoot — so skipping eager wiring
// there loses nothing. Incremental parses never allocate transient parents and
// skip the finalize wiring; they keep the eager-wiring constructor.
func (p *Parser) newRecoveryParentNodeInArena(arena *nodeArena, sym Symbol, named bool, children []*Node, productionID uint16) *Node {
	if p != nil && p.reduceScratch != nil && p.reduceScratch.transientParents != nil {
		return newParentNodeInArenaNoLinksWithFieldSources(arena, sym, named, children, nil, nil, productionID, true)
	}
	return newParentNodeInArena(arena, sym, named, children, nil, productionID)
}

// relexTokenForStackLexState re-lexes the current lookahead using one GLR
// stack's own lex mode.
//
// Background (issue #454 Scala investigation). tree-sitter C lexes once per
// parse version, so two versions sitting in different states can legitimately
// receive different tokenizations of the same bytes. This engine lexes once for
// all stacks (see updateParserStateTokenSource), which is cheaper and correct
// whenever every live stack accepts the shared token. It starves a stack when
// the same bytes must lex as a different symbol in that stack's state.
//
// Scala is the witness. In `if (a) c + 2` the correct derivation reduces `(a)`
// to _if_condition and then needs `+` as the grammar's generic
// operator_identifier. The rival derivation, which treats `(a)` as a plain
// expression, needs the dedicated `+` token that exists so prefix_expression
// can spell unary plus. The shared lexer emits the dedicated `+`, the correct
// stack finds no action for it, pauses, and the condense step drops it because
// the rival is still unpaused. The rival then dead-ends and the whole file
// becomes one ERROR. The C oracle parses the same input cleanly.
//
// This is not Scala-specific. Any grammar where one byte sequence lexes as
// different symbols depending on parse state has the same exposure: `+ - ! ~`
// as unary or binary, `/` as divide or regex start, `<` as comparison or type
// bracket, a keyword literal or the grammar's generic word token. Apex's
// `Type.class` is the word-token witness: `class_literal`'s _unannotated_type
// reading and `field_access`'s primary_expression reading both stay live GLR
// forks through the trailing `.`, one needing the `class` keyword and the
// other needing plain `identifier` for the same bytes; the shared lexer
// promotes the keyword, and the field_access fork used to die here with no
// alternative. This function has two callers: the C-recovery port above
// (parser.go, gated by errorCostCompetitionEnabled -- a no-action stack there
// pauses instead of dying if the re-lex fails) and the plain multi-stack
// dispatch loop (parser.go, ungated -- a no-action stack there is killed
// instead of dying if the re-lex fails). Both callers restore the shared
// token for every sibling stack via the same stackRelexRestoreTok /
// stackRelexActive pair, so a re-lex never leaks sideways to a stack that
// does accept the original symbol.
//
// The re-lex is deliberately narrow, so it cannot disturb the lockstep token
// loop the rest of the engine relies on:
//
//   - It only runs where the stack would otherwise pause with no action, so a
//     parse in which every stack accepts the shared token pays nothing.
//   - It requires the re-lexed token to cover exactly the shared token's byte
//     span. Same span means a stack that adopts it advances to the same offset
//     as every stack that took the shared token, so the versions stay in
//     lockstep and no caller has to reason about a ragged frontier.
//   - It requires the stack's state to have a real action for the re-lexed
//     symbol, so a failed probe leaves the existing pause path untouched.
//   - It runs the internal DFA only. The external scanner is never re-entered,
//     so no scanner state is mutated or needs restoring.
func (p *Parser) relexTokenForStackLexState(source []byte, state StateID, tok Token) (Token, bool) {
	lang := p.language
	if lang == nil || len(lang.LexStates) == 0 || int(state) >= len(lang.LexModes) {
		return tok, false
	}
	// Zero-width, missing, error-run and EOF lookaheads have no alternative
	// tokenization to find; they are handled by the paths above the pause.
	if tok.Symbol == 0 || tok.Symbol == errorSymbol || tok.Missing || tok.NoLookahead {
		return tok, false
	}
	if tok.StartByte >= tok.EndByte || int(tok.StartByte) >= len(source) {
		return tok, false
	}
	ls := lang.LexModes[state].LexStateIndex()
	if ls == noLookaheadLexState || int(ls) >= len(lang.LexStates) {
		return tok, false
	}
	// GLR prunes branches at no-action points constantly during ordinary
	// parses, so this probe runs often even on grammars that never need a
	// re-lex. Reuse one parser-owned Lexer rather than constructing a fresh one
	// per call so the probe stays allocation-free on that hot path. Measured
	// against origin/main with the grammar pre-warmed, java, cpp, go and
	// javascript allocate byte-identically with the probe in place.
	probe := &p.relexProbeLexer
	*probe = Lexer{
		states:          lang.LexStates,
		asciiTable:      lang.LexAsciiTable(),
		source:          source,
		pos:             int(tok.StartByte),
		row:             tok.StartPoint.Row,
		col:             tok.StartPoint.Column,
		immediateTokens: lang.ImmediateTokens,
		zeroWidthTokens: lang.ZeroWidthTokens,
	}
	if len(p.included) != 0 && lang.ExternalScanner == nil && len(lang.ExternalSymbols) == 0 {
		probe.setIncludedRanges(p.included)
	}
	relexed, ok := probe.scan(uint32(ls), probe.pos, probe.row, probe.col)
	if !ok || relexed.Symbol == 0 || relexed.Symbol == tok.Symbol {
		return tok, false
	}
	// Exact-span requirement: this is what keeps the shared-token loop in
	// lockstep. A shorter or longer re-lex would leave this stack at a
	// different byte offset than its siblings.
	if relexed.StartByte != tok.StartByte || relexed.EndByte != tok.EndByte {
		return tok, false
	}
	if !p.stateHasActionForSymbol(state, relexed.Symbol) {
		return tok, false
	}
	return relexed, true
}

// stateHasActionForSymbol reports whether state carries at least one real parse
// action for sym. It mirrors the action-lookup guard in the dispatch loop.
func (p *Parser) stateHasActionForSymbol(state StateID, sym Symbol) bool {
	if p.language == nil {
		return false
	}
	parseActions := p.language.ParseActions
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(parseActions) {
		return false
	}
	return len(parseActions[idx].Actions) > 0
}
