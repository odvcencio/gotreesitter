// Package gotreesitter implements a pure Go tree-sitter runtime.
//
// This file defines the core data structures that mirror tree-sitter's
// TSLanguage C struct and related types. They form the foundation on
// which the lexer, parser, query engine, and syntax tree are built.
package gotreesitter

// Symbol is a grammar symbol ID (terminal or nonterminal).
type Symbol uint16

// StateID is a parser state index.
type StateID uint16

// FieldID is a named field index.
type FieldID uint16

// ParseActionType identifies the kind of parse action.
type ParseActionType uint8

const (
	ParseActionShift  ParseActionType = iota
	ParseActionReduce
	ParseActionAccept
	ParseActionRecover
)

// ParseAction is a single parser action from the parse table.
type ParseAction struct {
	Type              ParseActionType
	State             StateID  // target state (shift/recover)
	Symbol            Symbol   // reduced symbol (reduce)
	ChildCount        uint8    // children consumed (reduce)
	DynamicPrecedence int16    // precedence (reduce)
	ProductionID      uint16   // which production (reduce)
	Extra             bool     // is this an extra token (shift)
	Repetition        bool     // is this a repetition (shift)
}

// ParseActionEntry is a group of actions for a (state, symbol) pair.
type ParseActionEntry struct {
	Reusable bool
	Actions  []ParseAction
}

// LexState is one state in the table-driven lexer DFA.
type LexState struct {
	AcceptToken Symbol // 0 if this state doesn't accept
	Skip        bool   // true if accepted chars are whitespace
	Transitions []LexTransition
	Default     int // default next state (-1 if none)
	EOF         int // state on EOF (-1 if none)
}

// LexTransition maps a character range to a next state.
type LexTransition struct {
	Lo, Hi    rune // inclusive character range
	NextState int
}

// LexMode maps a parser state to its lexer configuration.
type LexMode struct {
	LexState         uint16
	ExternalLexState uint16
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
// The Scan method accepts an interface{} for the lexer parameter because
// the concrete Lexer type is defined in a later task. It will be replaced
// with *Lexer once that type exists.
type ExternalScanner interface {
	Create() interface{}
	Destroy(payload interface{})
	Serialize(payload interface{}, buf []byte) int
	Deserialize(payload interface{}, buf []byte)
	Scan(payload interface{}, lexer interface{}, validSymbols []bool) bool
}

// Language holds all data needed to parse a specific language.
// It mirrors tree-sitter's TSLanguage C struct, translated into
// idiomatic Go types with slice-based tables instead of raw pointers.
type Language struct {
	Name string

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
	ParseTable         [][]uint16         // dense: [state][symbol] -> action index
	SmallParseTable    []uint16           // compressed sparse table
	SmallParseTableMap []uint32           // state -> offset into SmallParseTable
	ParseActions       []ParseActionEntry

	// Lex tables
	LexModes            []LexMode
	LexStates           []LexState // main lexer DFA
	KeywordLexStates    []LexState // keyword lexer DFA (optional)
	KeywordCaptureToken Symbol

	// Field mapping
	FieldMapSlices  [][2]uint16   // [production_id] -> (index, length)
	FieldMapEntries []FieldMapEntry

	// Alias sequences
	AliasSequences [][]Symbol // [production_id][child_index] -> alias symbol

	// Primary state IDs (for table dedup)
	PrimaryStateIDs []StateID

	// External scanner (nil if not needed)
	ExternalScanner ExternalScanner

	// InitialState is the parser's start state. In tree-sitter grammars
	// this is always 1 (state 0 is reserved for error recovery). For
	// hand-built grammars it defaults to 0.
	InitialState StateID
}
