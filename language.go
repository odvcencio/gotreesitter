// Package gotreesitter implements a pure Go tree-sitter runtime.
//
// This file defines the core data structures that mirror tree-sitter's
// TSLanguage C struct and related types. They form the foundation on
// which the lexer, parser, query engine, and syntax tree are built.
package gotreesitter

import (
	"slices"
	"sync"
)

// Symbol is a grammar symbol ID (terminal or nonterminal).
type Symbol uint16

// StateID is a parser state index. uint32 supports grammars with >65K states
// (e.g. COBOL with 67K states from 1071 rules).
type StateID uint32

// FieldID is a named field index.
type FieldID uint16

// ParseActionType identifies the kind of parse action.
type ParseActionType uint8

const (
	ParseActionShift ParseActionType = iota
	ParseActionReduce
	ParseActionAccept
	ParseActionRecover
)

const (
	// RuntimeLanguageVersion is the maximum tree-sitter language version this
	// runtime is known to support.
	RuntimeLanguageVersion uint32 = 15
	// MinCompatibleLanguageVersion is the minimum accepted language version.
	MinCompatibleLanguageVersion uint32 = 13
)

// ParseAction is a single parser action from the parse table.
type ParseAction struct {
	Type              ParseActionType
	State             StateID // target state (shift/recover)
	Symbol            Symbol  // reduced symbol (reduce)
	ChildCount        uint8   // children consumed (reduce)
	DynamicPrecedence int16   // precedence (reduce)
	ProductionID      uint16  // which production (reduce)
	Extra             bool    // is this an extra token (shift)
	ExtraChain        bool    // does this shift enter a nonterminal extra chain
	Repetition        bool    // is this a repetition (shift)
}

// ParseActionEntry is a group of actions for a (state, symbol) pair.
type ParseActionEntry struct {
	Reusable bool
	Actions  []ParseAction
}

// LexState is one state in the table-driven lexer DFA.
type LexState struct {
	AcceptToken    Symbol // 0 if this state doesn't accept
	AcceptPriority int16  // lower = higher priority (0 for ts2go blobs = longest-match)
	Skip           bool   // true if accepted chars are whitespace
	Default        int    // default next state (-1 if none)
	EOF            int    // state on EOF (-1 if none)
	Transitions    []LexTransition
}

// LexTransition maps a character range to a next state.
type LexTransition struct {
	Lo, Hi    rune // inclusive character range
	NextState int
	// Skip mirrors tree-sitter's SKIP(state): consume the matched rune
	// and continue lexing while resetting token start.
	Skip bool
}

// LexMode maps a parser state to its lexer configuration.
type LexMode struct {
	LexState                  uint16
	ExternalLexState          uint16
	ReservedWordSetID         uint16
	AfterWhitespaceLexState   uint16 // DFA start state to use after whitespace (0 = same as LexState)
	LexStateID                uint32 // widened DFA start state for grammargen tables with >64K lexer states
	AfterWhitespaceLexStateID uint32
}

// LexStateIndex returns the DFA start state for this lex mode. Older grammar
// blobs only populate the uint16 LexState field; grammargen-generated tables
// can populate LexStateID when the DFA table exceeds 64K states.
func (m LexMode) LexStateIndex() uint32 {
	if m.LexStateID != 0 {
		return m.LexStateID
	}
	if m.LexState == ^uint16(0) {
		return ^uint32(0)
	}
	return uint32(m.LexState)
}

// AfterWhitespaceLexStateIndex returns the alternate DFA start state used
// after whitespace, or zero when the primary lex state should be used.
func (m LexMode) AfterWhitespaceLexStateIndex() uint32 {
	if m.AfterWhitespaceLexStateID != 0 {
		return m.AfterWhitespaceLexStateID
	}
	if m.AfterWhitespaceLexState == ^uint16(0) {
		return ^uint32(0)
	}
	return uint32(m.AfterWhitespaceLexState)
}

func (m *LexMode) SetLexStateIndex(idx uint32) {
	if m == nil {
		return
	}
	m.LexStateID = idx
	if idx == ^uint32(0) {
		m.LexState = ^uint16(0)
		return
	}
	m.LexState = uint16(idx)
}

func (m *LexMode) SetAfterWhitespaceLexStateIndex(idx uint32) {
	if m == nil {
		return
	}
	m.AfterWhitespaceLexStateID = idx
	if idx == ^uint32(0) {
		m.AfterWhitespaceLexState = ^uint16(0)
		return
	}
	m.AfterWhitespaceLexState = uint16(idx)
}

// LanguageMetadata holds the grammar's semantic version (ABI 15+).
type LanguageMetadata struct {
	MajorVersion uint8
	MinorVersion uint8
	PatchVersion uint8
}

// SymbolMetadata holds display information about a symbol.
type SymbolMetadata struct {
	Name      string
	Visible   bool
	Named     bool
	Supertype bool
}

// FieldMapEntry maps a child index to a field name.
type FieldMapEntry struct {
	FieldID    FieldID
	ChildIndex uint8
	Inherited  bool
}

// ExternalScanner is the interface for language-specific external scanners.
// Languages like Python and JavaScript need these for indent tracking,
// template literals, regex vs division, etc.
//
// The value returned by Create must be accepted by Destroy/Serialize/
// Deserialize/Scan for that scanner implementation. Most scanners use a
// concrete payload pointer type and will panic on mismatched payload types.
type ExternalScanner interface {
	Create() any
	Destroy(payload any)
	Serialize(payload any, buf []byte) int
	Deserialize(payload any, buf []byte)
	Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool
}

// IncrementalReuseExternalScanner is implemented by external scanners that can
// safely participate in DFA subtree reuse during incremental parses. Scanners
// with serialized mutable state, such as Python's indentation stack, should
// leave this unimplemented so edited incremental parses fall back to the
// conservative full-reparse path.
type IncrementalReuseExternalScanner interface {
	ExternalScanner
	SupportsIncrementalReuse() bool
}

// ReduceChainTerminalAction describes the action class expected after a
// generated reduce-chain hint finishes applying deterministic reductions.
type ReduceChainTerminalAction uint8

const (
	ReduceChainTerminalNoAction ReduceChainTerminalAction = iota
	ReduceChainTerminalSingleReduce
	ReduceChainTerminalSingleShift
	ReduceChainTerminalSingleAccept
	ReduceChainTerminalSingleOther
	ReduceChainTerminalMulti
)

// ReduceChainHint describes a terminal-verified parser hot path for a
// deterministic reduce chain. The runtime still applies normal reduce
// semantics and stops before the terminal action; this metadata only lets it
// avoid repeated generic action dispatch for approved state/lookahead pairs.
type ReduceChainHint struct {
	StartState     StateID
	Lookahead      Symbol
	TerminalStates []StateID
	TerminalAction ReduceChainTerminalAction
	MaxSteps       uint16
}

// Language holds all data needed to parse a specific language.
// It mirrors tree-sitter's TSLanguage C struct, translated into
// idiomatic Go types with slice-based tables instead of raw pointers.
type Language struct {
	Name string
	// GeneratedByGrammargen is true for languages assembled by grammargen at
	// runtime rather than decoded from a checked-in ts2go blob.
	GeneratedByGrammargen bool

	// LanguageVersion is the tree-sitter language ABI version.
	// A value of 0 means "unknown/unspecified" and is treated as compatible.
	LanguageVersion uint32

	// Counts
	SymbolCount        uint32
	TokenCount         uint32
	ExternalTokenCount uint32
	StateCount         uint32
	LargeStateCount    uint32
	FieldCount         uint32
	ProductionIDCount  uint32

	// Symbol metadata
	SymbolNames    []string
	SymbolMetadata []SymbolMetadata
	FieldNames     []string // index 0 is ""

	// Parse tables
	ParseTable         [][]uint16 // dense: [state][symbol] -> action index
	SmallParseTable    []uint16   // compressed sparse table
	SmallParseTableMap []uint32   // state -> offset into SmallParseTable
	ParseActions       []ParseActionEntry

	// ReduceChainHints are optional generated hot-path hints for deterministic
	// reduce runs. They are only consumed when reduce-chain hints are enabled.
	ReduceChainHints []ReduceChainHint

	// Lex tables
	LexModes            []LexMode
	LexStates           []LexState // main lexer DFA
	KeywordLexStates    []LexState // keyword lexer DFA (optional)
	KeywordCaptureToken Symbol
	// LayoutFallbackLexState is an optional broad DFA start state used only in
	// layout-entry parser states. It lets the runtime avoid skipping over
	// zero-width external layout markers before the layout scanner fires.
	LayoutFallbackLexState    uint16
	HasLayoutFallbackLexState bool

	// Field mapping
	FieldMapSlices  [][2]uint16 // [production_id] -> (index, length)
	FieldMapEntries []FieldMapEntry

	// Alias sequences
	AliasSequences [][]Symbol // [production_id][child_index] -> alias symbol

	// Primary state IDs (for table dedup)
	PrimaryStateIDs []StateID

	// ABI 15: Reserved words — flat array indexed by
	// (reserved_word_set_id * MaxReservedWordSetSize + i), terminated by 0.
	ReservedWords          []Symbol
	MaxReservedWordSetSize uint16

	// ABI 15: Supertype hierarchy
	SupertypeSymbols    []Symbol
	SupertypeMapSlices  [][2]uint16 // [supertype_symbol] -> (index, length)
	SupertypeMapEntries []Symbol

	// ABI 15: Grammar semantic version
	Metadata LanguageMetadata

	// External scanner (nil if not needed)
	ExternalScanner ExternalScanner
	ExternalSymbols []Symbol // external token index -> symbol
	// ImmediateTokens is a bitmask of symbol IDs that are token.immediate() tokens.
	// When the lexer matches one of these after consuming whitespace, the match
	// should be rejected — immediate tokens must match at the original position.
	// nil means no immediate tokens (common for ts2go grammars).
	ImmediateTokens []bool
	// ZeroWidthTokens is a bitmask of symbol IDs whose DFA terminal pattern can
	// intentionally match empty input. nil means this information is unavailable,
	// which preserves historical lexer behavior for ts2go blobs.
	ZeroWidthTokens []bool

	// ExternalLexStates maps external lex state IDs (from LexMode.ExternalLexState)
	// to a boolean slice indicating which external tokens are valid. Row 0 is
	// always all-false (no external tokens valid). When non-nil, this table is
	// used instead of parse-action-table probing to compute validSymbols for the
	// external scanner, matching C tree-sitter's ts_external_scanner_states.
	ExternalLexStates [][]bool

	// InitialState is the parser's start state. In tree-sitter grammars
	// this is always 1 (state 0 is reserved for error recovery). For
	// hand-built grammars it defaults to 0.
	InitialState StateID

	// Lazily-built lookup maps for O(1) name resolution.
	symbolNameMap            map[string]Symbol
	symbolNameNamedMap       map[symbolNameNamedKey]Symbol
	visibleSymbolNameMap     map[symbolNameNamedKey]Symbol
	tokenSymbolNameMap       map[string][]Symbol
	publicSymbolMap          []Symbol // internal symbol → canonical public symbol
	publicNamedSymbolMap     []Symbol // internal symbol -> canonical public named symbol
	publicAnonymousSymbolMap []Symbol // internal symbol -> canonical public anonymous symbol
	fieldNameMap             map[string]FieldID

	symbolMapOnce sync.Once
	fieldMapOnce  sync.Once

	// ASCII fast-path for the lexer DFA. lexAsciiTable[stateID][byte] encodes
	// the result of the transition scan for ASCII input (bytes 0x00–0x7F).
	// Bit 31 set = skip transition; bits 30–0 = next state (0x7FFF_FFFF = no match).
	lexAsciiTable        [][128]int32
	lexAsciiOnce         sync.Once
	keywordLexAsciiTable [][128]int32
	keywordLexAsciiOnce  sync.Once
	lexModeStarts        []lexModeStart
	lexModeStartOnce     sync.Once
	keywordPrefilter     keywordLexPrefilter
	keywordPrefilterOnce sync.Once
}

type symbolNameNamedKey struct {
	name  string
	named bool
}

type keywordLexPrefilter struct {
	allowAll bool
	hasAny   bool
	first    [4]uint64
	lengths  [4]uint64
}

type lexModeStart struct {
	lexState                uint32
	afterWhitespaceLexState uint32
}

const lexAsciiNoMatch = int32(0x7FFF_FFFF)
const lexAsciiSkipBit = int32(-1 << 31) // bit 31

// LexAsciiTable returns the pre-built ASCII fast-path transition table for the
// main lexer DFA. The table is built once per Language. Entry format:
//
//	bit 31 set  → skip transition (consume and reset token start)
//	bits 0-30   → next state ID (lexAsciiNoMatch if no transition)
func (l *Language) LexAsciiTable() [][128]int32 {
	if l == nil {
		return nil
	}
	l.lexAsciiOnce.Do(func() {
		l.lexAsciiTable = buildLexAsciiTable(l.LexStates)
	})
	return l.lexAsciiTable
}

// KeywordLexAsciiTable returns the ASCII fast-path table for the keyword lexer DFA.
func (l *Language) KeywordLexAsciiTable() [][128]int32 {
	if l == nil || len(l.KeywordLexStates) == 0 {
		return nil
	}
	l.keywordLexAsciiOnce.Do(func() {
		l.keywordLexAsciiTable = buildLexAsciiTable(l.KeywordLexStates)
	})
	return l.keywordLexAsciiTable
}

func (l *Language) keywordLexCouldMatch(source []byte, start, end int) bool {
	if l == nil || len(l.KeywordLexStates) == 0 {
		return true
	}
	l.keywordPrefilterOnce.Do(func() {
		l.keywordPrefilter = l.buildKeywordLexPrefilter()
	})
	filter := l.keywordPrefilter
	if filter.allowAll {
		return true
	}
	if !filter.hasAny || start < 0 || end <= start || end > len(source) {
		return false
	}
	n := end - start
	if n >= 256 {
		return false
	}
	if filter.lengths[n/64]&(uint64(1)<<uint(n%64)) == 0 {
		return false
	}
	first := source[start]
	return filter.first[first/64]&(uint64(1)<<uint(first%64)) != 0
}

func (l *Language) buildKeywordLexPrefilter() keywordLexPrefilter {
	if l == nil || len(l.KeywordLexStates) == 0 {
		return keywordLexPrefilter{allowAll: true}
	}
	var filter keywordLexPrefilter
	for _, state := range l.KeywordLexStates {
		sym := state.AcceptToken
		if sym == 0 {
			continue
		}
		idx := int(sym)
		if idx < 0 || idx >= len(l.SymbolNames) {
			return keywordLexPrefilter{allowAll: true}
		}
		name := l.SymbolNames[idx]
		if name == "" || len(name) >= 256 {
			return keywordLexPrefilter{allowAll: true}
		}
		if !isPlainKeywordSymbolName(name) {
			return keywordLexPrefilter{allowAll: true}
		}
		filter.hasAny = true
		filter.first[name[0]/64] |= uint64(1) << uint(name[0]%64)
		filter.lengths[len(name)/64] |= uint64(1) << uint(len(name)%64)
	}
	return filter
}

func isPlainKeywordSymbolName(name string) bool {
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func (l *Language) LexModeStarts() []lexModeStart {
	if l == nil || len(l.LexModes) == 0 {
		return nil
	}
	l.lexModeStartOnce.Do(func() {
		starts := make([]lexModeStart, len(l.LexModes))
		for i, mode := range l.LexModes {
			starts[i] = lexModeStart{
				lexState:                mode.LexStateIndex(),
				afterWhitespaceLexState: mode.AfterWhitespaceLexStateIndex(),
			}
		}
		l.lexModeStarts = starts
	})
	return l.lexModeStarts
}

func buildLexAsciiTable(states []LexState) [][128]int32 {
	tbl := make([][128]int32, len(states))
	for si := range states {
		row := &tbl[si]
		for c := 0; c < 128; c++ {
			row[c] = lexAsciiNoMatch
		}
		remaining := 128
		for ti := range states[si].Transitions {
			if remaining == 0 {
				break
			}
			tr := &states[si].Transitions[ti]
			if tr.Hi < 0 || tr.Lo >= 128 {
				continue
			}
			lo := tr.Lo
			if lo < 0 {
				lo = 0
			}
			hi := tr.Hi
			if hi > 127 {
				hi = 127
			}
			v := int32(tr.NextState)
			if tr.Skip {
				v |= lexAsciiSkipBit
			}
			for c := int(lo); c <= int(hi); c++ {
				if row[c] == lexAsciiNoMatch {
					row[c] = v
					remaining--
				}
			}
		}
	}
	return tbl
}

// Version returns the tree-sitter language ABI version.
func (l *Language) Version() uint32 {
	if l == nil {
		return 0
	}
	return l.LanguageVersion
}

// CompatibleWithRuntime reports whether this language can be parsed by the
// current runtime version. Unspecified versions (0) are treated as compatible.
func (l *Language) CompatibleWithRuntime() bool {
	v := l.Version()
	if v == 0 {
		return true
	}
	return v >= MinCompatibleLanguageVersion && v <= RuntimeLanguageVersion
}

// SymbolByName returns the symbol ID for a given name, or (0, false) if not found.
// The "_" wildcard returns (0, true) as a special case.
// Builds an internal map on first call for O(1) subsequent lookups.
func (l *Language) SymbolByName(name string) (Symbol, bool) {
	if name == "_" {
		return 0, true
	}
	l.buildSymbolMaps()
	sym, ok := l.symbolNameMap[name]
	return sym, ok
}

func (l *Language) symbolByNameAndNamed(name string, named bool) (Symbol, bool) {
	if name == "_" {
		return 0, true
	}
	l.buildSymbolMaps()
	sym, ok := l.symbolNameNamedMap[symbolNameNamedKey{name: name, named: named}]
	return sym, ok
}

func (l *Language) visibleSymbolByNameAndNamed(name string, named bool) (Symbol, bool) {
	if name == "_" {
		return 0, true
	}
	l.buildSymbolMaps()
	sym, ok := l.visibleSymbolNameMap[symbolNameNamedKey{name: name, named: named}]
	return sym, ok
}

func (l *Language) symbolByNamePreferNamed(name string) (Symbol, bool) {
	if sym, ok := l.symbolByNameAndNamed(name, true); ok {
		return sym, true
	}
	return l.SymbolByName(name)
}

// TokenSymbolsByName returns all terminal token symbols whose display name
// matches name. The returned symbols are in grammar order.
func (l *Language) TokenSymbolsByName(name string) []Symbol {
	l.buildSymbolMaps()
	return l.tokenSymbolNameMap[name]
}

// PublicSymbol maps an internal symbol to its canonical public form.
// Multiple internal symbols may share the same visible name (e.g.
// HTML's _start_tag_name and _end_tag_name both display as "tag_name").
// PublicSymbol returns the first symbol with that name, matching what
// SymbolByName returns. This ensures query patterns compiled with
// SymbolByName match nodes regardless of which alias produced them.
func (l *Language) PublicSymbol(sym Symbol) Symbol {
	if l == nil {
		return sym
	}
	l.buildSymbolMaps()
	if int(sym) < len(l.publicSymbolMap) {
		return l.publicSymbolMap[sym]
	}
	return sym
}

// PublicSymbolForNamedness maps an internal symbol to the canonical public
// symbol with the same display name and requested namedness. This lets query
// matching distinguish named nodes from anonymous tokens that share text.
func (l *Language) PublicSymbolForNamedness(sym Symbol, named bool) Symbol {
	if l == nil {
		return sym
	}
	l.buildSymbolMaps()
	idx := int(sym)
	if idx < 0 || idx >= len(l.SymbolNames) {
		return sym
	}
	if named {
		if idx < len(l.publicNamedSymbolMap) {
			return l.publicNamedSymbolMap[idx]
		}
		return sym
	}
	if idx < len(l.publicAnonymousSymbolMap) {
		return l.publicAnonymousSymbolMap[idx]
	}
	return sym
}

func (l *Language) buildSymbolMaps() {
	l.symbolMapOnce.Do(func() {
		l.symbolNameMap = make(map[string]Symbol, len(l.SymbolNames))
		l.symbolNameNamedMap = make(map[symbolNameNamedKey]Symbol, len(l.SymbolNames))
		l.visibleSymbolNameMap = make(map[symbolNameNamedKey]Symbol, len(l.SymbolNames))
		l.tokenSymbolNameMap = make(map[string][]Symbol)
		l.publicSymbolMap = make([]Symbol, len(l.SymbolNames))
		l.publicNamedSymbolMap = make([]Symbol, len(l.SymbolNames))
		l.publicAnonymousSymbolMap = make([]Symbol, len(l.SymbolNames))

		tokenCount := int(l.TokenCount)
		if tokenCount > len(l.SymbolNames) {
			tokenCount = len(l.SymbolNames)
		}

		for i, sn := range l.SymbolNames {
			sym := Symbol(i)
			if sn == "" {
				l.publicSymbolMap[i] = sym
				l.publicNamedSymbolMap[i] = sym
				l.publicAnonymousSymbolMap[i] = sym
				continue
			}
			// Keep the first match so duplicate names remain deterministic.
			if _, exists := l.symbolNameMap[sn]; !exists {
				l.symbolNameMap[sn] = sym
			}
			named := false
			if i < len(l.SymbolMetadata) {
				named = l.SymbolMetadata[i].Named
			}
			key := symbolNameNamedKey{name: sn, named: named}
			if _, exists := l.symbolNameNamedMap[key]; !exists {
				l.symbolNameNamedMap[key] = sym
			}
			if i < len(l.SymbolMetadata) && l.SymbolMetadata[i].Visible {
				if _, exists := l.visibleSymbolNameMap[key]; !exists {
					l.visibleSymbolNameMap[key] = sym
				}
			}
			if i < tokenCount {
				l.tokenSymbolNameMap[sn] = append(l.tokenSymbolNameMap[sn], sym)
			}
		}
		for i, sn := range l.SymbolNames {
			sym := Symbol(i)
			if sn == "" {
				l.publicSymbolMap[i] = sym
				l.publicNamedSymbolMap[i] = sym
				l.publicAnonymousSymbolMap[i] = sym
				continue
			}
			l.publicSymbolMap[i] = l.symbolNameMap[sn]
			if namedSym, exists := l.symbolNameNamedMap[symbolNameNamedKey{name: sn, named: true}]; exists {
				l.publicNamedSymbolMap[i] = namedSym
			} else {
				l.publicNamedSymbolMap[i] = l.symbolNameMap[sn]
			}
			if anonSym, exists := l.symbolNameNamedMap[symbolNameNamedKey{name: sn, named: false}]; exists {
				l.publicAnonymousSymbolMap[i] = anonSym
			} else {
				l.publicAnonymousSymbolMap[i] = l.symbolNameMap[sn]
			}
		}
	})
}

// IsSupertype reports whether sym is a supertype symbol.
func (l *Language) IsSupertype(sym Symbol) bool {
	if l == nil {
		return false
	}
	return slices.Contains(l.SupertypeSymbols, sym)
}

// SupertypeChildren returns the subtype symbols for a given supertype.
// Returns nil if sym is not a supertype or has no entries.
func (l *Language) SupertypeChildren(sym Symbol) []Symbol {
	if l == nil {
		return nil
	}
	idx := int(sym)
	if idx >= len(l.SupertypeMapSlices) {
		return nil
	}
	slice := l.SupertypeMapSlices[idx]
	start, length := int(slice[0]), int(slice[1])
	if length == 0 {
		return nil
	}
	if start+length > len(l.SupertypeMapEntries) {
		return nil
	}
	return l.SupertypeMapEntries[start : start+length]
}

// FieldByName returns the field ID for a given name, or (0, false) if not found.
// Builds an internal map on first call for O(1) subsequent lookups.
func (l *Language) FieldByName(name string) (FieldID, bool) {
	l.fieldMapOnce.Do(func() {
		l.fieldNameMap = make(map[string]FieldID, len(l.FieldNames))
		for i, fn := range l.FieldNames {
			if fn != "" {
				l.fieldNameMap[fn] = FieldID(i)
			}
		}
	})
	fid, ok := l.fieldNameMap[name]
	return fid, ok
}
