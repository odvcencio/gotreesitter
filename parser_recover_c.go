package gotreesitter

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"unicode"
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
	case "cpp", "html", "julia":
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

// cRecoverResumeLookahead ports the error-mode half of ts_parser__lex for the
// pause lookahead of a custom (non-DFA) token source. In C the lookahead that
// triggers detect_error is already the product of the in-lex fallback chain:
// the paused state's own lex mode first, then the ERROR-state mode, then the
// skipped-character error subtree. A custom source that only lexes
// normal-mode tokens hands handle_error/recover a lookahead C would never see
// at this position (authzed: int_literal "1" where C's error-mode DFA lexes a
// 13-byte hidden string-content run). When the paused state's own DFA mode
// cannot lex here — C's fallback trigger — substitute the ERROR-mode token
// and schedule the source resync.
func (p *Parser) cRecoverResumeLookahead(source []byte, s *glrStack, tok Token) (Token, bool) {
	if !p.cRecoverCustomSourceEligible || p.cRecoverSharedTokenErrorModeLexed {
		return tok, false
	}
	if tok.Symbol == 0 || tok.Symbol == errorSymbol || tok.Missing || tok.NoLookahead {
		return tok, false
	}
	if s == nil || int(s.byteOffset) >= len(source) {
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
	relexed := lx.NextWithErrorRuns(uint32(errLS))
	if relexed.Symbol == tok.Symbol && relexed.StartByte == tok.StartByte && relexed.EndByte == tok.EndByte {
		return tok, false
	}
	p.cRecoverSharedTokenErrorModeLexed = true
	p.cRecoverCustomResyncActive = true
	p.cRecoverCustomResyncByte = relexed.EndByte
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
	tok := lx.NextWithErrorRuns(uint32(ls))
	// The shared token now carries the C error-mode identity; the election
	// can trust it directly.
	p.cRecoverSharedTokenErrorModeLexed = true
	p.cRecoverCustomResyncActive = true
	p.cRecoverCustomResyncByte = tok.EndByte
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
	depth    int
	state    StateID
	posBytes uint32
	posRow   uint32
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
	// groupOrder preserves the path order inside the C merged error-state
	// version. Later Go stack ordering can move members around, but C's
	// strategy-1 summary scan still walks the merged paths in record order.
	groupOrder int
	// openErr is the open error region node on the stack top (the C
	// error_repeat being accumulated). nil right after entering the error
	// state — the C "ERROR_STATE head with NULL subtree" shape, which costs an
	// extra ERROR_COST_PER_RECOVERY in ts_stack_error_cost.
	openErr *Node
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
	extraRecoveries int
}

func (r *cRecoverState) clone() *cRecoverState {
	if r == nil {
		return nil
	}
	cp := &cRecoverState{openErr: r.openErr, group: r.group, groupOrder: r.groupOrder, extraRecoveries: r.extraRecoveries}
	if len(r.summary) > 0 {
		cp.summary = append([]cStackSummaryEntry(nil), r.summary...)
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
	if p != nil && p.cNodeMemo != nil {
		if e, ok := p.cNodeMemo[n]; ok && e.hasVis && e.ver == n.equivVersion {
			return e.visCount
		}
	}
	count := 0
	if p.cSymbolVisible(n.symbol) {
		count++
	}
	for _, c := range n.children {
		count += p.cNodeVisibleSubtreeCount(c)
	}
	if p != nil && p.cNodeMemo != nil {
		e := p.cNodeMemo[n]
		if e.ver != n.equivVersion {
			e = cNodeMemoEntry{ver: n.equivVersion}
		}
		e.visCount = count
		e.hasVis = true
		p.cNodeMemo[n] = e
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

// cNodeMemoEntry caches the gated recovery's per-subtree aggregates, the
// engine analogue of C SubtreeHeapData.error_cost / node counts computed once
// per subtree in ts_subtree_summarize_children. Finished subtrees never
// mutate during a parse; the open error region is bumped (equivVersion) on
// every absorb, invalidating its entry. Without the memo every
// condense/competition step rewalks whole accumulated subtrees per token,
// which is O(n^2) on large gated files.
type cNodeMemoEntry struct {
	ver      uint32
	cost     uint32
	visCount int
	hasCost  bool
	hasVis   bool
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
	if p == nil || p.cNodeMemo == nil || p.language == nil || n == nil {
		return cErrRegionAbsorbPre{}
	}
	if n.symbol != errorSymbol || (n.isMissing() && len(n.children) == 0) {
		return cErrRegionAbsorbPre{}
	}
	// Route through the standard memoized walks so the delta base is exactly
	// the full-walk answer at the pre-absorb version (O(1) once warm).
	return cErrRegionAbsorbPre{
		node:     n,
		cost:     p.cNodeErrorCost(n),
		vis:      p.cNodeVisibleSubtreeCount(n),
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
	if !pre.valid || p == nil || p.cNodeMemo == nil {
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
		vis += p.cNodeVisibleSubtreeCount(c)
		if !(c.symbol == errorSymbol && len(c.children) == 0) {
			// C ERROR leaf children keep subtree error_cost 0 (see
			// cNodeErrorCostLang); everything else contributes its own cost.
			cost += p.cNodeErrorCost(c)
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
	p.cNodeMemo[n] = cNodeMemoEntry{
		ver:      n.equivVersion,
		cost:     cost,
		visCount: vis,
		hasCost:  true,
		hasVis:   true,
	}
	if ms := p.mergeScratch; ms != nil {
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
	if p == nil || p.cNodeMemo == nil || p.language == nil || n == nil {
		return
	}
	cost := p.cNodeErrorCost(n)
	p.cNodeVisibleSubtreeCount(n)
	if ms := p.mergeScratch; ms != nil {
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
	if p.cNodeMemo == nil {
		return cNodeErrorCostLang(p.language, n)
	}
	if e, ok := p.cNodeMemo[n]; ok && e.hasCost && e.ver == n.equivVersion {
		return e.cost
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
	e := p.cNodeMemo[n]
	if e.ver != n.equivVersion {
		e = cNodeMemoEntry{ver: n.equivVersion}
	}
	e.cost = cost
	e.hasCost = true
	p.cNodeMemo[n] = e
	return cost
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
// The on-node aggregates below (gssNode.aggGen/aggCost/aggVisGen/aggVis)
// restore C's shape: per gssNode, the cumulative aggregates of the prev-chain
// prefix root..node inclusive. gssNode prev/entry links are write-once at
// allocation except setGSSMainLink (link-0 rewrite), and node payload
// contents mutate only through nodeBumpEquivVersion call sites; both choke
// points bump gssPrefixAggGen, so an aggregate with a matching generation is
// exactly the full-walk answer. allocNode zeroes the gens on every (possibly
// slab-recycled) node and the generation counter starts at 1, so stale or
// fresh nodes can never validate.
// ---------------------------------------------------------------------------

// gssPrefixAggGen is the global invalidation generation for the GSS prefix
// aggregates stored on gssNode (aggGen/aggCost/aggVisGen/aggVis). Bumped by
// nodeBumpEquivVersion (any in-place node content mutation, tree.go) and
// setGSSMainLink link-0 rewrites (glr.go). Global rather than per-parser
// because nodeBumpEquivVersion has no parser in scope; cross-parser
// over-invalidation only costs a rebuild, never staleness. Initialized to 1
// so the zero value of gssNode.aggGen / aggVisGen (fresh or slab-cleared
// nodes) can never validate.
var gssPrefixAggGen atomic.Uint64

func init() {
	gssPrefixAggGen.Store(1)
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
		if gn.aggGen == gen && gn.aggVisGen == gen {
			cost, vis = gn.aggCost, gn.aggVis
			break
		}
		path = append(path, gn)
		gn = gn.prev
	}
	for i := len(path) - 1; i >= 0; i-- {
		gn := path[i]
		if n := stackEntryNode(gn.entry); n != nil {
			cost += p.cNodeErrorCost(n)
			vis += int32(p.cNodeVisibleSubtreeCount(n))
		}
		gn.aggGen = gen
		gn.aggVisGen = gen
		gn.aggCost = cost
		gn.aggVis = vis
	}
	p.cPrefixPath = path
	return cost, int(vis)
}

// cStackPrefixCostForMerge is the merge-scratch twin of cStackPrefixAgg. It
// fills cost only (the merge side has no memoized visible-count walk), so it
// leaves aggVisGen untouched; the cost value is identical to the parser-side
// fill, letting both sides share aggCost.
func cStackPrefixCostForMerge(scratch *glrMergeScratch, lang *Language, head *gssNode) uint32 {
	gen := gssPrefixAggGen.Load()
	var cost uint32
	path := scratch.cPrefixPath[:0]
	gn := head
	for gn != nil {
		if gn.aggGen == gen {
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
		gn.aggGen = gen
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
// so a poisoned cNodeMemo cannot mask itself) and compares it against the
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
	var cost uint32
	if len(s.entries) > 0 {
		for i := range s.entries {
			if n := stackEntryNode(s.entries[i]); n != nil {
				cost += p.cNodeErrorCost(n)
			}
		}
	} else if p != nil && p.cNodeMemo != nil && s.gss.head != nil {
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
		for i := range s.entries {
			if n := stackEntryNode(s.entries[i]); n != nil {
				count += p.cNodeVisibleSubtreeCount(n)
			}
		}
		return count
	}
	if p != nil && p.cNodeMemo != nil && s.gss.head != nil {
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
		versions[vi].cNodeBaseline = groupBaseline
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
	count := p.cStackCumulativeNodeCount(s) - s.cNodeBaseline
	if count < 0 {
		// C clamps (and writes back) when the stack popped below the baseline.
		s.cNodeBaseline = p.cStackCumulativeNodeCount(s)
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
			if stacksHeaderEquivalent(stacks[self], stacks[i]) &&
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

// cStackEntriesTopFirst materializes the stack spine top-first.
func cStackEntriesTopFirst(s *glrStack, gssScratch *gssScratch) []stackEntry {
	s.ensureGSS(gssScratch)
	depth := s.depth()
	if depth == 0 {
		return nil
	}
	entries := make([]stackEntry, 0, depth)
	for n := s.gss.head; n != nil; n = n.prev {
		entries = append(entries, n.entry)
	}
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
	summary := make([]cStackSummaryEntry, 0, 8)
	depth := 0
	// Position of the node at-or-below each entry: C node positions are the
	// cumulative input position at that stack node.
	posBytesAt := make([]uint32, len(entries))
	posRowAt := make([]uint32, len(entries))
	var pb uint32
	var pr uint32
	for i := len(entries) - 1; i >= 0; i-- {
		if stackEntryHasNode(entries[i]) {
			pb = stackEntryNodeEndByte(entries[i])
			pr = stackEntryNodeEndPoint(entries[i]).Row
		}
		posBytesAt[i] = pb
		posRowAt[i] = pr
	}
	record := func(d int, st StateID, posBytes, posRow uint32) {
		for j := len(summary) - 1; j >= 0; j-- {
			if summary[j].depth < d {
				break
			}
			if summary[j].depth == d && summary[j].state == st {
				return
			}
		}
		summary = append(summary, cStackSummaryEntry{depth: d, state: st, posBytes: posBytes, posRow: posRow})
	}
	for i := 0; i < len(entries); i++ {
		record(depth, entries[i].state, posBytesAt[i], posRowAt[i])
		if cEntryCountsTowardDepth(entries[i]) {
			depth++
			if depth > cRecoverMaxSummaryDepth {
				break
			}
		}
	}
	return summary
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
// overloading lookaheadSym == 0.
func (p *Parser) cDoAllPotentialReductions(source []byte, start glrStack, lookaheadSym Symbol, anyLookahead bool, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) ([]glrStack, bool) {
	oldDisablePostReduceForkMerge := p.disablePostReduceForkMerge
	p.disablePostReduceForkMerge = true
	defer func() {
		p.disablePostReduceForkMerge = oldDisablePostReduceForkMerge
	}()

	versions := []glrStack{start}
	canShift := false
	var reduces []ParseAction
	v := 0
	for iter := 0; ; iter++ {
		if v >= len(versions) {
			break
		}
		// Merge check against earlier versions created in this call.
		merged := false
		for j := 0; j < v; j++ {
			if p.cTryMergeReductionVersion(&versions[j], &versions[v]) {
				versions = append(versions[:v], versions[v+1:]...)
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
			actionCandidates := p.cReductionCandidatesForAction(source, versions[v], act, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
			actionReductionVersion := -1
			if len(actionCandidates) > 0 {
				versions, actionReductionVersion = p.cAppendActionReductionVersions(versions, actionCandidates, v, arena)
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
			versions[v] = versions[lastReduction]
			versions = append(versions[:lastReduction], versions[lastReduction+1:]...)
			continue
		} else if !anyLookahead {
			versions = append(versions[:v], versions[v+1:]...)
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
	return versions, canShift
}

func (p *Parser) cReductionCandidatesForAction(source []byte, start glrStack, act ParseAction, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) []glrStack {
	p.pendingForkStacks = p.pendingForkStacks[:0]
	fork := start.cloneWithScratch(gssScratch)
	fork.cRec = start.cRec.clone()
	var dummy bool
	p.applyAction(source, &fork, act, tok, &dummy, nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
	candidates := make([]glrStack, 0, 1+len(p.pendingForkStacks))
	if !fork.dead {
		candidates = append(candidates, fork)
	}
	for i := range p.pendingForkStacks {
		if !p.pendingForkStacks[i].dead {
			candidates = append(candidates, p.pendingForkStacks[i])
		}
	}
	p.pendingForkStacks = p.pendingForkStacks[:0]
	return candidates
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

func (p *Parser) cTryMergeReductionVersion(target, candidate *glrStack) bool {
	if target == nil || candidate == nil || target.dead || candidate.dead || target.accepted || candidate.accepted {
		return false
	}
	if target.entries != nil || candidate.entries != nil || target.gss.head == nil || candidate.gss.head == nil {
		return false
	}
	if !stacksHeaderEquivalent(*target, *candidate) {
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
	if targetPopTo == nil || !stacksHeaderEquivalent(*target, *candidate) {
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
func (p *Parser) cHandleError(stacks *[]glrStack, si int, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool) {
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
	p.crecoveryEnteredErrorState = true
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
	versions, _ := p.cDoAllPotentialReductions(source, s.clone(), 0, true, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	group := &cRecGroup{}

	// 2. Missing-token insertion (once across the version set, in order).
	// C keeps every version that survives do_all_potential_reductions on the
	// lookahead (the copied version plus its reduction forks).
	var missingVersions []glrStack
	if !p.isGraphQLRecoveryTripleQuote(tok.Symbol) {
		for vi := range versions {
			state := versions[vi].top().state
			tokenCount := Symbol(p.language.TokenCount)
			for ms := Symbol(1); ms < tokenCount; ms++ {
				nextState, shiftAct, ok := p.cTerminalNextState(state, ms)
				if !ok || nextState == 0 || nextState == state {
					continue
				}
				if !p.stateHasLeadingReduceAction(nextState, tok.Symbol) {
					continue
				}
				cand := versions[vi].cloneWithScratch(gssScratch)
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
				if p.rejectUndrainedPendingForkStacks(&cand) {
					continue
				}
				cand.shifted = false
				if cand.dead {
					continue
				}
				reduced, canShift := p.cDoAllPotentialReductions(source, cand, tok.Symbol, false, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
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
		v := &versions[vi]
		v.pushEntry(stackEntry{state: cErrorState}, entryScratch, gssScratch)
		v.shifted = false
	}
	p.cApplyMergedErrorGroupBaseline(versions)
	for vi := range versions {
		v := &versions[vi]
		entries := cStackEntriesTopFirst(v, gssScratch)
		if debugRecoveryCycleChecks {
			for ei := range entries {
				if entries[ei].node != nil {
					debugRecoveryCheckAcyclic(p, arena, fmt.Sprintf("handle-error-version-%d-spine-%d", vi, ei), entries[ei])
				}
			}
		}
		v.cRec = &cRecoverState{summary: p.cRecordSummary(entries), group: group, groupOrder: vi}
		v.cRecoverMissingGroup = nil
	}

	// The original stack becomes the first absorbing version.
	*s = versions[0]
	for vi := 1; vi < len(versions); vi++ {
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
		v := &(*stacks)[i]
		if v.dead || v.cRec == nil || v.cRec.group != group {
			continue
		}
		res, forked := p.cRecover(stacks, v, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
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
	return outcome, needsRedispatch
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
func (p *Parser) cRecover(stacks *[]glrStack, v *glrStack, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool) {
	rec := v.cRec
	if rec == nil {
		return cRecFallthrough, false
	}
	vIndex := -1
	for i := range *stacks {
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
			didRecover, forked = p.cRecoverStrategy1Election(stacks, g, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
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
		return cRecHalted, forked
	}

	// EOF: wrap everything and accept (ts_parser__recover recover_eof).
	if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
		p.cRecoverEOFAccept(v, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		return cRecConsumed, forked
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
	if vIndex >= 0 && p.cBetterVersionExists(*stacks, vIndex, false, newCost) {
		v.dead = true
		return cRecHalted, forked
	}

	// Wrap the lookahead into the open error region (strategy 2).
	if !p.guardRealTokenAttachmentGap(source, v, tok, "c-recover") {
		return cRecHalted, forked
	}
	p.cAbsorbTokenIntoError(v, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	v.shifted = true
	return cRecConsumed, forked
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
func (p *Parser) cRecoverStrategy1Election(stacks *[]glrStack, group *cRecGroup, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (didRecover, forked bool) {
	// C runs the summary scan for every non-error lookahead INCLUDING the EOF
	// token (parser.c ts_parser__recover: `summary && !ts_subtree_is_error`).
	// EOF election is what lets C pop the open error region to a state where
	// the end symbol is valid and finish with a named root + contained ERROR
	// instead of recover_eof's whole-file ERROR wrap. A wide symbol-0 token is
	// this engine's unlexable run (C: error-subtree lookahead) — C skips
	// strategy 1 for those.
	if tok.Symbol == errorSymbol {
		return false, false
	}
	if p.isGraphQLRecoveryTripleQuote(tok.Symbol) {
		return false, false
	}
	if tok.Symbol == 0 && tok.StartByte != tok.EndByte {
		return false, false
	}
	var members []int
	for i := range *stacks {
		if !(*stacks)[i].dead && (*stacks)[i].cRec != nil && (*stacks)[i].cRec.group == group {
			members = append(members, i)
		}
	}
	if len(members) == 0 {
		return false, false
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
		return false, false
	}
	type seenKey struct {
		depth int
		state StateID
	}
	seen := make(map[seenKey]bool, 16)
	for d := 0; d <= cRecoverMaxSummaryDepth+1; d++ {
		for _, mi := range members {
			for _, entry := range (*stacks)[mi].cRec.summary {
				if entry.depth != d {
					continue
				}
				if entry.state == cErrorState {
					continue
				}
				key := seenKey{depth: entry.depth, state: entry.state}
				if seen[key] {
					continue
				}
				seen[key] = true
				if entry.posBytes == pos {
					continue
				}
				depth := entry.depth + depthBump
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
					return false, false
				}
				if p.lookupActionIndex(entry.state, electionSym) == 0 {
					continue
				}
				if fork, ok := p.cRecoverToState(&(*stacks)[mi], depth, entry.state, arena, entryScratch, gssScratch, trackChildErrors); ok {
					fork.branchOrder = (*stacks)[mi].branchOrder
					*stacks = append(*stacks, fork)
					if nodeCount != nil {
						*nodeCount = *nodeCount + 1
					}
					if p.glrTrace {
						traceCRecoverToState(entry.state, depth)
					}
					return true, true
				}
			}
		}
	}
	return false, false
}

func cSortRecoverMembersByGroupOrder(stacks []glrStack, members []int) {
	for i := 1; i < len(members); i++ {
		cur := members[i]
		curOrder := stacks[cur].cRec.groupOrder
		j := i - 1
		for ; j >= 0; j-- {
			prev := members[j]
			if stacks[prev].cRec.groupOrder <= curOrder {
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
	nodeBumpEquivVersion(root)
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
		nodeBumpEquivVersion(errNode)
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
		leaf = newLeafNodeInArena(arena, tok.Symbol, p.isNamedSymbol(tok.Symbol),
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		leaf.setHasError(true)
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
	nodeBumpEquivVersion(errNode)
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
func (p *Parser) cRecoverDispatchInError(stacks *[]glrStack, si int, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) (cRecoverOutcome, bool) {
	s := &(*stacks)[si]
	if s.top().state != cErrorState {
		// Mid non-terminal-extra parse: the version temporarily left
		// ERROR_STATE; C dispatches it through the normal table rows.
		return cRecFallthrough, false
	}
	if tok.Symbol != 0 {
		if p.isGraphQLRecoveryTripleQuote(tok.Symbol) {
			return p.cRecover(stacks, s, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		}
		if idx := p.lookupActionIndex(cErrorState, tok.Symbol); idx != 0 && int(idx) < len(p.language.ParseActions) {
			if actions := p.language.ParseActions[idx].Actions; len(actions) > 0 &&
				actions[0].Type == ParseActionShift {
				return cRecFallthrough, false
			}
		}
		// Zero-width non-EOF tokens are skipped (C's error-mode lexer never
		// returns empty internal tokens; the Go DFA source can).
		if tok.StartByte == tok.EndByte {
			s.shifted = true
			return cRecConsumed, false
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
// resumed handle_error), and the possibly error-mode-relexed current token
// (see cRecoverResumeLookahead) which the caller must adopt so redispatched
// versions act on the same lookahead the resumed group consumed.
func (p *Parser) cCondenseAndResume(stacks []glrStack, source []byte, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) ([]glrStack, bool, Token) {
	relevant := false
	for i := range stacks {
		if stacks[i].cPaused || stacks[i].cRec != nil || stacks[i].cRecoverMissingGroup != nil {
			relevant = true
			break
		}
	}
	if debugRecoveryCycleChecks && relevant {
		for i := range stacks {
			if stacks[i].dead {
				continue
			}
			entries := cStackEntriesTopFirst(&stacks[i], gssScratch)
			debugRecoveryCheckSpineAcyclic(p, arena, fmt.Sprintf("condense-stack-%d-byte-%d", i, stacks[i].byteOffset), entries)
		}
	}
	if !relevant {
		return stacks, false, tok
	}
	// Drop dead versions first (C removes halted versions in condense).
	// Accepted versions have left the pool in C (ts_parser__accept stashes
	// the tree and removes the version): they sit out the cost competition,
	// the ordering, and the version cap, and rejoin only for final result
	// selection.
	var acceptedStacks []glrStack
	alive := stacks[:0]
	for i := range stacks {
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
	for i := 1; i < len(stacks); i++ {
		statusI := p.cVersionStatus(&stacks[i])
		for j := 0; j < i; j++ {
			if cRecoverVersionsSameGroup(stacks[j], stacks[i]) {
				continue
			}
			if cRecoverVersionShouldStayBefore(stacks[j], stacks[i]) {
				continue
			}
			if cRecoverVersionShouldStayBefore(stacks[i], stacks[j]) {
				stacks[i], stacks[j] = stacks[j], stacks[i]
				statusI = p.cVersionStatus(&stacks[i])
				continue
			}
			statusJ := p.cVersionStatus(&stacks[j])
			switch cCompareVersions(statusJ, statusI) {
			case cErrorComparisonTakeLeft:
				if p.glrTrace {
					p.traceCCondenseDrop("take-left", i, j, stacks[i], stacks[j], statusI, statusJ)
				}
				stacks = append(stacks[:i], stacks[i+1:]...)
				i--
				j = i
			case cErrorComparisonPreferRight:
				if p.glrTrace {
					p.traceCCondenseSwap("prefer-right", i, j, stacks[i], stacks[j], statusI, statusJ)
				}
				stacks[i], stacks[j] = stacks[j], stacks[i]
				statusI = p.cVersionStatus(&stacks[i])
			case cErrorComparisonTakeRight:
				if p.glrTrace {
					p.traceCCondenseDrop("take-right", j, i, stacks[j], stacks[i], statusJ, statusI)
				}
				stacks = append(stacks[:j], stacks[j+1:]...)
				i--
				j--
				statusI = p.cVersionStatus(&stacks[i])
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
		keyRanks := make(map[cCondenseVersionKey]int, len(stacks))
		for i := 0; i < len(stacks); i++ {
			key := p.cCondenseVersionKeyFor(&stacks[i])
			rank, seen := keyRanks[key]
			if !seen {
				rank = len(keyRanks)
				keyRanks[key] = rank
			}
			if rank < cRecoverMaxVersionCount {
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
			if replacement, replaced := p.cRecoverResumeLookahead(source, &stacks[i], tok); replaced {
				if p.glrTrace {
					fmt.Printf("      -> C-RESUME-RELEX sym=%d [%d-%d] -> sym=%d [%d-%d]\n",
						tok.Symbol, tok.StartByte, tok.EndByte,
						replacement.Symbol, replacement.StartByte, replacement.EndByte)
				}
				tok = replacement
			}
			outcome, redispatch := p.cHandleError(&stacks, i, source, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
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
	return stacks, needsRedispatch, tok
}

func (p *Parser) traceCCondenseDrop(reason string, dropIndex, keepIndex int, drop, keep glrStack, dropStatus, keepStatus cErrorStatus) {
	fmt.Printf("      -> C-CONDENSE-DROP reason=%s drop=%d %s %s keep=%d %s %s\n",
		reason,
		dropIndex,
		cRecoverStackTraceKind(drop),
		cCondenseStackTraceSummary(drop, dropStatus),
		keepIndex,
		cRecoverStackTraceKind(keep),
		cCondenseStackTraceSummary(keep, keepStatus),
	)
}

func (p *Parser) traceCCondenseSwap(reason string, i, j int, left, right glrStack, leftStatus, rightStatus cErrorStatus) {
	fmt.Printf("      -> C-CONDENSE-SWAP reason=%s i=%d %s %s j=%d %s %s\n",
		reason,
		i,
		cRecoverStackTraceKind(left),
		cCondenseStackTraceSummary(left, leftStatus),
		j,
		cRecoverStackTraceKind(right),
		cCondenseStackTraceSummary(right, rightStatus),
	)
}

func (p *Parser) traceCCondenseTrim(index int, stack glrStack) {
	fmt.Printf("      -> C-CONDENSE-TRIM index=%d %s state=%d byte=%d depth=%d score=%d\n",
		index,
		cRecoverStackTraceKind(stack),
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
func (p *Parser) cAcceptRootRebuild(s *glrStack, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	if p == nil || s == nil || !s.accepted {
		return
	}
	entries := cStackEntriesTopFirst(s, gssScratch)
	payloads := 0
	for i := range entries {
		if stackEntryHasNode(entries[i]) {
			payloads++
		}
	}
	if payloads <= 1 {
		return
	}
	// Materialize base-most first, mirroring C's popped subtree order.
	nodes := make([]*Node, 0, payloads)
	for i := len(entries) - 1; i >= 0; i-- {
		if !stackEntryHasNode(entries[i]) {
			continue // stack base / error discontinuity
		}
		n, _ := materializeStackEntryPayloadEntryWithParser(p, arena, entries[i], materializeForRecovery, materializeForRecovery)
		if n == nil {
			return
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
		return
	}
	cand := nodes[rootIdx]
	children := make([]*Node, 0, len(nodes)-1+len(cand.children))
	for _, n := range nodes[:rootIdx] {
		children = p.cAppendVisibleSplice(children, n)
	}
	children = append(children, cand.children...)
	for _, n := range nodes[rootIdx+1:] {
		children = p.cAppendVisibleSplice(children, n)
	}
	root := p.newRecoveryParentNodeInArena(arena, cand.symbol, p.isNamedSymbol(cand.symbol), children, 0)
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
	nodeBumpEquivVersion(root)
	if !s.truncate(1) {
		return
	}
	p.pushStackNode(s, 1, root, entryScratch, gssScratch)
	if debugRecoveryCycleChecks {
		debugRecoveryCheckNodeAcyclic(p, arena, "accept-root-rebuild", root)
	}
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
	return p.captureRawShape(arena, symbol, productionID, entries, 0, len(entries))
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
