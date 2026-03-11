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

// StateID is a parser state index.
type StateID uint16

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
	Transitions    []LexTransition
	Default        int // default next state (-1 if none)
	EOF            int // state on EOF (-1 if none)
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
	LexState          uint16
	ExternalLexState  uint16
	ReservedWordSetID uint16
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

// Language holds all data needed to parse a specific language.
// It mirrors tree-sitter's TSLanguage C struct, translated into
// idiomatic Go types with slice-based tables instead of raw pointers.
type Language struct {
	Name string

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
	symbolNameMap      map[string]Symbol
	tokenSymbolNameMap map[string][]Symbol
	publicSymbolMap    []Symbol // internal symbol → canonical public symbol
	fieldNameMap       map[string]FieldID

	symbolMapOnce sync.Once
	fieldMapOnce  sync.Once
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

// TokenSymbolsByName returns all terminal token symbols whose display name
// matches name. The returned symbols are in grammar order.
func (l *Language) TokenSymbolsByName(name string) []Symbol {
	if name == "_" {
		return []Symbol{0}
	}
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

func (l *Language) buildSymbolMaps() {
	l.symbolMapOnce.Do(func() {
		l.symbolNameMap = make(map[string]Symbol, len(l.SymbolNames))
		l.tokenSymbolNameMap = make(map[string][]Symbol)
		l.publicSymbolMap = make([]Symbol, len(l.SymbolNames))

		tokenCount := int(l.TokenCount)
		if tokenCount > len(l.SymbolNames) {
			tokenCount = len(l.SymbolNames)
		}

		for i, sn := range l.SymbolNames {
			sym := Symbol(i)
			if sn == "" {
				l.publicSymbolMap[i] = sym
				continue
			}
			// Keep the first match so duplicate names remain deterministic.
			if _, exists := l.symbolNameMap[sn]; !exists {
				l.symbolNameMap[sn] = sym
			}
			// Map each symbol to the canonical (first) symbol with its name.
			l.publicSymbolMap[i] = l.symbolNameMap[sn]
			if i < tokenCount {
				l.tokenSymbolNameMap[sn] = append(l.tokenSymbolNameMap[sn], sym)
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
