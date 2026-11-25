package tablebuild

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports the numbering of render.rs: the parts of struct Generator
// that give every symbol, alias, field, production, parse state and action
// list its number in parser.c. The C text itself is not written. Where the
// generator writes a C identifier that the compiler turns into a number, the
// port keeps the identifier, because sorting by identifier decides some
// orders (the supertype map and the non-terminal alias map), and then
// resolves it to its number. lex_layout.go ports the parts that number the
// lexer: the lex functions, the lex modes and the external scanner tables.
//
// # Order
//
// The generator keeps several hash maps here (symbol_order, symbol_ids,
// alias_ids, symbol_map, the action list offsets, the alias ids by symbol and
// the groups of a small state). Each is either read only by key, or its
// entries are sorted on a unique key before they are written; the comment
// at each place says which. No hash order reaches a number.

const (
	// smallStateThreshold is render.rs: const SMALL_STATE_THRESHOLD.
	smallStateThreshold = 64
	// ABIVersionMin and ABIVersionMax are render.rs: const ABI_VERSION_MIN
	// and ABI_VERSION_MAX (generate.rs: const LANGUAGE_VERSION).
	ABIVersionMin = 14
	ABIVersionMax = 15
	// abiVersionWithReservedWords is render.rs: const ABI_VERSION_WITH_RESERVED_WORDS.
	abiVersionWithReservedWords = 15
)

// ParseActionCountError is render.rs: RenderError::ParseTable.
type ParseActionCountError struct{ Count int }

func (e *ParseActionCountError) Error() string {
	return fmt.Sprintf("Parse table action count %d exceeds maximum value of 65535", e.Count)
}

// ABIVersionError is render.rs: RenderError::ABI.
type ABIVersionError struct{ Version int }

func (e *ABIVersionError) Error() string {
	return fmt.Sprintf("This version of Tree-sitter can only generate parsers with ABI version %d - %d, not %d", ABIVersionMin, ABIVersionMax, e.Version)
}

// IdentifierError reports two C identifiers of ts_symbol_identifiers that
// are equal: the C compiler rejects the parser.c that the generator writes.
type IdentifierError struct{ Identifier string }

func (e *IdentifierError) Error() string {
	return fmt.Sprintf("tablebuild: the C identifier %s is declared twice in ts_symbol_identifiers", e.Identifier)
}

// SymbolMetadata is one entry of ts_symbol_metadata.
type SymbolMetadata struct {
	Visible, Named, Supertype bool
}

// FieldMapEntry is one entry of ts_field_map_entries.
type FieldMapEntry struct {
	FieldID    uint16
	ChildIndex uint32
	Inherited  bool
}

// RenderedAction is one action of ts_parse_actions, with its symbol numbered.
// Kind ActionShiftExtra is SHIFT_EXTRA(); the other fields follow ParseAction.
type RenderedAction struct {
	Kind              ParseActionKind
	State             uint32
	IsRepetition      bool
	Symbol            uint16
	ChildCount        uint16
	DynamicPrecedence int32
	ProductionID      uint16
}

// ActionListEntry is one list of ts_parse_actions: its offset (the number
// that ts_parse_table and ts_small_parse_table hold), its reusable flag and
// its actions.
type ActionListEntry struct {
	Offset   uint32
	Reusable bool
	Actions  []RenderedAction
}

// Layout holds the tables of the parser.c that render.rs writes, as numbers.
// Symbol ids count the parse table's symbols (end first), then the aliases
// that match no symbol.
type Layout struct {
	ABIVersion             int
	StateCount             int
	LargeStateCount        int
	SymbolCount            int // SYMBOL_COUNT: the symbols, without the aliases
	AliasCount             int
	TokenCount             int
	ExternalTokenCount     int
	FieldCount             int
	MaxAliasSequenceLength int
	MaxReservedWordSetSize int
	ProductionIDCount      int
	SupertypeCount         int

	// SymbolIdentifiers holds the C identifier of each symbol id.
	SymbolIdentifiers []string
	SymbolNames       []string         // ts_symbol_names
	SymbolMetadata    []SymbolMetadata // ts_symbol_metadata
	SymbolMap         []uint16         // ts_symbol_map
	// FieldNames is ts_field_names: index 0 is "" (NULL).
	FieldNames      []string
	FieldMapSlices  [][2]uint16 // ts_field_map_slices: index, length per production id
	FieldMapEntries []FieldMapEntry
	// AliasSequences is ts_alias_sequences: one row of
	// MaxAliasSequenceLength symbol ids per production id, 0 for no alias.
	AliasSequences [][]uint16
	// NonTerminalAliasMap is ts_non_terminal_alias_map, flat: per symbol its
	// id, 1 plus its alias count, its public id and its aliases, then 0.
	NonTerminalAliasMap []uint16
	PrimaryStateIDs     []uint16 // ts_primary_state_ids
	SupertypeSymbols    []uint16 // ts_supertype_symbols
	// SupertypeMapSlices is ts_supertype_map_slices, indexed by symbol id.
	SupertypeMapSlices  [][2]uint16
	SupertypeMapEntries []uint16 // ts_supertype_map_entries
	// ReservedWords is ts_reserved_words: one row of MaxReservedWordSetSize
	// symbol ids per reserved word set, 0 after the last word; row 0 is the
	// empty set. ReservedWordSetIDs holds the set of each parse state.
	ReservedWords      [][]uint16
	ReservedWordSetIDs []int
	// ParseTable is ts_parse_table: one row of SymbolCount cells per large
	// state, an action list offset for a token and a state for a non-terminal.
	ParseTable         [][]uint16
	SmallParseTable    []uint16 // ts_small_parse_table
	SmallParseTableMap []uint32 // ts_small_parse_table_map
	ParseActions       []ActionListEntry

	// LexModes is ts_lex_modes: one entry per parse state.
	LexModes []LexMode
	// MainLex and KeywordLex are ts_lex and ts_lex_keywords. RenderTables
	// fills them; Render, which has no lex tables, leaves them nil.
	// KeywordLex is nil without a word token.
	MainLex, KeywordLex []LexState
	// KeywordCaptureToken is the symbol id of the word token (0 without
	// one).
	KeywordCaptureToken uint16
	// ExternalScannerSymbolMap is ts_external_scanner_symbol_map and
	// ExternalScannerStates ts_external_scanner_states (row 0 empty); both
	// are nil without external tokens.
	ExternalScannerSymbolMap []uint16
	ExternalScannerStates    [][]bool
}

// supertypeEntry is one entry of render.rs: Generator::supertype_map, a
// BTreeMap from the supertype's C identifier to its child types.
type supertypeEntry struct {
	identifier string
	subtypes   []ChildType
}

// generator is render.rs: struct Generator, without the C buffer.
type generator struct {
	parseTable                     *InternedTable
	lexTables                      *lexbuild.LexTables // nil: no lex functions
	largeStateCount                int
	syntaxGrammar                  *lexbuild.SyntaxGrammar
	lexicalGrammar                 *lexbuild.LexicalGrammar
	defaultAliases                 AliasMap
	symbolOrder                    map[Symbol]int
	symbolIDs                      map[Symbol]string
	aliasIDs                       map[Alias]string
	uniqueAliases                  []Alias
	symbolMap                      map[Symbol]Symbol
	reservedWordSets               []lexbuild.TokenSet
	reservedWordSetIDsByParseState []int
	fieldNames                     []string
	supertypeSymbolMap             []SupertypeSubtypes
	supertypeMap                   []supertypeEntry
	abiVersion                     int

	// enumValues is the value of each enumerator of ts_symbol_identifiers
	// (add_symbol_enum), with ts_builtin_sym_end (parser.h) as 0.
	enumValues map[string]int
	out        *Layout
}

// Render is render.rs: fn render_c_code and fn Generator::generate, for the
// numbered tables. It reads the parse table of BuildTables, the prepared
// grammar, and the supertype map of GetSupertypeSymbolMap. It has no lex
// tables, so it writes no lex functions; RenderTables writes them too.
func Render(table *InternedTable, g *PreparedGrammar, supertypeSymbolMap []SupertypeSubtypes, abiVersion int) (*Layout, error) {
	return render(table, nil, g, supertypeSymbolMap, abiVersion)
}

// render is Render with the lex tables of a build (nil: none). A panic of
// panicLikeGenerator becomes its *GeneratorPanicError.
func render(table *InternedTable, lexTables *lexbuild.LexTables, g *PreparedGrammar, supertypeSymbolMap []SupertypeSubtypes, abiVersion int) (*Layout, error) {
	if abiVersion < ABIVersionMin || abiVersion > ABIVersionMax {
		return nil, &ABIVersionError{abiVersion}
	}
	gen := &generator{
		parseTable:         table,
		lexTables:          lexTables,
		syntaxGrammar:      g.Syntax,
		lexicalGrammar:     g.Lex,
		defaultAliases:     g.DefaultAliases,
		supertypeSymbolMap: supertypeSymbolMap,
		abiVersion:         abiVersion,
		out:                &Layout{ABIVersion: abiVersion},
	}
	var out *Layout
	err := catchGeneratorPanic(func() error {
		var err error
		out, err = gen.generate()
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// generate is render.rs: fn Generator::generate, the calls that number.
func (g *generator) generate() (*Layout, error) {
	g.init()
	g.addStats()
	if err := g.addSymbolEnum(); err != nil {
		return nil, err
	}
	g.addSymbolNamesList()
	g.addUniqueSymbolMap()
	g.addSymbolMetadataList()

	if len(g.fieldNames) > 0 {
		g.addFieldNameNamesList()
		g.addFieldSequences()
	}

	if len(g.parseTable.ProductionInfos) > 0 {
		g.addAliasSequences()
	}

	g.addNonTerminalAliasMap()
	g.addPrimaryStateIDList()

	if g.abiVersion >= abiVersionWithReservedWords && len(g.supertypeMap) > 0 {
		g.addSupertypeMap()
	}

	if err := g.addLexFunctions(); err != nil {
		return nil, err
	}
	g.addLexModes()

	if g.abiVersion >= abiVersionWithReservedWords && len(g.reservedWordSets) > 1 {
		g.addReservedWordSets()
	}
	g.out.ReservedWordSetIDs = g.reservedWordSetIDsByParseState

	if err := g.addParseTable(); err != nil {
		return nil, err
	}

	if len(g.syntaxGrammar.ExternalTokens) > 0 {
		g.addExternalScannerSymbolMap()
		g.addExternalScannerStatesList()
	}
	g.addKeywordCaptureToken()
	return g.out, nil
}

// init is render.rs: fn Generator::init, without the large character sets.
func (g *generator) init() {
	table := g.parseTable
	symbolIdentifiers := map[string]bool{}
	g.symbolIDs = map[Symbol]string{}
	for _, s := range table.Symbols {
		g.assignSymbolID(s, symbolIdentifiers)
	}
	g.symbolIDs[lexbuild.EndOfNonTerminalExtraSym()] = g.symbolIDs[lexbuild.EndSym()]

	g.symbolMap = map[Symbol]Symbol{}
	for _, symbol := range table.Symbols {
		mapping := symbol

		// There can be multiple symbols in the grammar that have the same name
		// and kind, due to simple aliases. When that happens, ensure that they
		// map to the same public-facing symbol. If one of the symbols is not
		// aliased, choose that one to be the public-facing symbol. Otherwise,
		// pick the symbol with the lowest numeric value.
		if alias, ok := g.defaultAliases.get(symbol); ok {
			kind := aliasKind(alias)
			for _, other := range table.Symbols {
				if otherAlias, ok := g.defaultAliases.get(other); ok {
					if lexbuild.SymbolLess(other, mapping) && aliasEq(otherAlias, alias) {
						mapping = other
					}
				} else if name, k := g.metadataForSymbol(other); name == alias.Value && k == kind {
					mapping = other
					break
				}
			}
		} else if symbol.IsTerminal() {
			// Two anonymous tokens with different flags but the same string
			// value should be represented with the same symbol in the public
			// API. Examples:
			// * "<" and token(prec(1, "<"))
			// * "(" and token.immediate("(")
			name, kind := g.metadataForSymbol(symbol)
			for _, other := range table.Symbols {
				if otherName, otherKind := g.metadataForSymbol(other); otherName == name && otherKind == kind {
					if mapped, ok := g.symbolMap[other]; ok && mapped == symbol {
						break
					}
					mapping = other
					break
				}
			}
		}
		g.symbolMap[symbol] = mapping
	}

	g.aliasIDs = map[Alias]string{}
	for k := range table.ProductionInfos {
		info := &table.ProductionInfos[k]
		// Build a list of all field names. The generator iterates the field
		// map in the order of its keys' string ids; the insertion keeps the
		// list sorted by name whatever that order is.
		for _, fieldName := range info.FieldNames() {
			i := sort.SearchStrings(g.fieldNames, fieldName)
			if i == len(g.fieldNames) || g.fieldNames[i] != fieldName {
				g.fieldNames = append(g.fieldNames, "")
				copy(g.fieldNames[i+1:], g.fieldNames[i:])
				g.fieldNames[i] = fieldName
			}
		}

		// Generate a mapping from aliases to C identifiers.
		for _, alias := range info.AliasSequence {
			if !alias.Set {
				continue
			}
			var aliasID string
			if existing := g.symbolsForAlias(alias); len(existing) > 0 {
				// Some aliases match an existing symbol in the grammar.
				aliasID = g.symbolIDs[g.symbolMap[existing[0]]]
			} else {
				// Other aliases don't match any existing symbol, and need their
				// own identifiers.
				i := sort.Search(len(g.uniqueAliases), func(k int) bool { return !aliasLessByName(g.uniqueAliases[k], alias) })
				if i == len(g.uniqueAliases) || !aliasEq(g.uniqueAliases[i], alias) {
					g.uniqueAliases = append(g.uniqueAliases, Alias{})
					copy(g.uniqueAliases[i+1:], g.uniqueAliases[i:])
					g.uniqueAliases[i] = alias
				}
				if alias.Named {
					aliasID = "alias_sym_" + sanitizeIdentifier(alias.Value)
				} else {
					aliasID = "anon_alias_sym_" + sanitizeIdentifier(alias.Value)
				}
			}
			if _, ok := g.aliasIDs[alias]; !ok {
				g.aliasIDs[alias] = aliasID
			}
		}
	}

	// Assign an id to each unique reserved word set.
	g.reservedWordSets = append(g.reservedWordSets, lexbuild.TokenSet{})
	for i := range table.States {
		state := &table.States[i]
		id := -1
		for k := range g.reservedWordSets {
			if g.reservedWordSets[k].Equal(&state.ReservedWords) {
				id = k
				break
			}
		}
		if id < 0 {
			g.reservedWordSets = append(g.reservedWordSets, state.ReservedWords.Clone())
			id = len(g.reservedWordSets) - 1
		}
		g.reservedWordSetIDsByParseState = append(g.reservedWordSetIDsByParseState, id)
	}

	if g.abiVersion >= abiVersionWithReservedWords {
		// supertype_symbol_map is a BTreeMap keyed by symbol: its entries
		// come in symbol order, and the first one for an identifier stays.
		for _, e := range g.supertypeSymbolMap {
			supertype, ok := g.symbolIDs[e.Supertype]
			if !ok {
				continue
			}
			i := sort.Search(len(g.supertypeMap), func(k int) bool { return g.supertypeMap[k].identifier >= supertype })
			if i < len(g.supertypeMap) && g.supertypeMap[i].identifier == supertype {
				continue
			}
			g.supertypeMap = append(g.supertypeMap, supertypeEntry{})
			copy(g.supertypeMap[i+1:], g.supertypeMap[i:])
			g.supertypeMap[i] = supertypeEntry{identifier: supertype, subtypes: append([]ChildType(nil), e.Subtypes...)}
		}
		g.supertypeSymbolMap = nil
	}

	// Determine which states should use the "small state" representation,
	// and which should use the normal array representation.
	threshold := min(smallStateThreshold, len(table.Symbols)/2)
	g.largeStateCount = 0
	for i := range table.States {
		s := &table.States[i]
		if !(i <= 1 || len(s.TerminalEntries)+len(s.NonterminalEntries) > threshold) {
			break
		}
		g.largeStateCount++
	}
}

// aliasKind is rules.rs: fn Alias::kind.
func aliasKind(a Alias) lexbuild.VariableType {
	if a.Named {
		return lexbuild.VarNamed
	}
	return lexbuild.VarAnonymous
}

// aliasLessByName is the order of render.rs: Generator::unique_aliases: by
// value, then unnamed before named.
func aliasLessByName(a, b Alias) bool {
	if a.Value != b.Value {
		return a.Value < b.Value
	}
	return !a.Named && b.Named
}

// addStats is render.rs: fn Generator::add_stats: the counts.
func (g *generator) addStats() {
	table := g.parseTable
	tokenCount := 0
	for _, s := range table.Symbols {
		switch {
		case s.IsTerminal() || s.Kind == lexbuild.End:
			tokenCount++
		case s.IsExternal():
			if !g.syntaxGrammar.ExternalTokens[s.Index].HasCorresponding {
				tokenCount++
			}
		}
	}
	maxReserved := 0
	for k := range g.reservedWordSets {
		maxReserved = max(maxReserved, g.reservedWordSets[k].Len())
	}
	g.out.StateCount = len(table.States)
	g.out.LargeStateCount = g.largeStateCount
	g.out.SymbolCount = len(table.Symbols)
	g.out.AliasCount = len(g.uniqueAliases)
	g.out.TokenCount = tokenCount
	g.out.ExternalTokenCount = len(g.syntaxGrammar.ExternalTokens)
	g.out.FieldCount = len(g.fieldNames)
	g.out.MaxAliasSequenceLength = table.MaxAliasedProductionLength
	g.out.MaxReservedWordSetSize = maxReserved
	g.out.ProductionIDCount = len(table.ProductionInfos)
	g.out.SupertypeCount = len(g.supertypeMap)
}

// addSymbolEnum is render.rs: fn Generator::add_symbol_enum: the end token
// is 0, the other symbols follow in the order of the parse table's symbols,
// then the unique aliases.
func (g *generator) addSymbolEnum() error {
	g.symbolOrder = map[Symbol]int{lexbuild.EndSym(): 0}
	g.enumValues = map[string]int{"ts_builtin_sym_end": 0}
	ids := []string{"ts_builtin_sym_end"}
	i := 1
	declare := func(identifier string) error {
		if _, dup := g.enumValues[identifier]; dup {
			return &IdentifierError{identifier}
		}
		g.enumValues[identifier] = i
		ids = append(ids, identifier)
		i++
		return nil
	}
	for _, symbol := range g.parseTable.Symbols {
		if symbol != lexbuild.EndSym() {
			g.symbolOrder[symbol] = i
			if err := declare(g.symbolIDs[symbol]); err != nil {
				return err
			}
		}
	}
	for _, alias := range g.uniqueAliases {
		if err := declare(g.aliasIDs[alias]); err != nil {
			return err
		}
	}
	g.out.SymbolIdentifiers = ids
	return nil
}

// value is the number that the C compiler gives an identifier of
// ts_symbol_identifiers.
func (g *generator) value(identifier string) uint16 {
	v, ok := g.enumValues[identifier]
	if !ok {
		panicLikeGenerator("the C identifier %s is not declared", identifier)
	}
	return uint16(v)
}

// symbolValue is the number of a symbol: the value of its identifier.
func (g *generator) symbolValue(s Symbol) uint16 {
	id, ok := g.symbolIDs[s]
	if !ok {
		panicLikeGenerator("symbol %v has no identifier", s)
	}
	return g.value(id)
}

// addSymbolNamesList is render.rs: fn Generator::add_symbol_names_list.
func (g *generator) addSymbolNamesList() {
	for _, symbol := range g.parseTable.Symbols {
		name, _ := g.metadataForSymbol(symbol)
		if alias, ok := g.defaultAliases.get(symbol); ok {
			name = alias.Value
		}
		g.out.SymbolNames = append(g.out.SymbolNames, name)
	}
	for _, alias := range g.uniqueAliases {
		g.out.SymbolNames = append(g.out.SymbolNames, alias.Value)
	}
}

// addUniqueSymbolMap is render.rs: fn Generator::add_unique_symbol_map.
func (g *generator) addUniqueSymbolMap() {
	for _, symbol := range g.parseTable.Symbols {
		g.out.SymbolMap = append(g.out.SymbolMap, g.symbolValue(g.symbolMap[symbol]))
	}
	for _, alias := range g.uniqueAliases {
		g.out.SymbolMap = append(g.out.SymbolMap, g.value(g.aliasIDs[alias]))
	}
}

// addFieldNameNamesList is render.rs: fn Generator::add_field_name_names_list
// (and add_field_name_enum: the field ids are 1 plus the index in the list).
func (g *generator) addFieldNameNamesList() {
	g.out.FieldNames = append([]string{""}, g.fieldNames...)
}

// fieldValue is the number of a field: render.rs: fn add_field_name_enum.
func (g *generator) fieldValue(name string) uint16 {
	i := sort.SearchStrings(g.fieldNames, name)
	if i == len(g.fieldNames) || g.fieldNames[i] != name {
		panicLikeGenerator("field %q has no id", name)
	}
	return uint16(i + 1)
}

// addSymbolMetadataList is render.rs: fn Generator::add_symbol_metadata_list.
func (g *generator) addSymbolMetadataList() {
	for _, symbol := range g.parseTable.Symbols {
		var m SymbolMetadata
		if alias, ok := g.defaultAliases.get(symbol); ok {
			m = SymbolMetadata{Visible: true, Named: alias.Named}
		} else {
			switch _, kind := g.metadataForSymbol(symbol); kind {
			case lexbuild.VarNamed:
				m = SymbolMetadata{Visible: true, Named: true}
			case lexbuild.VarAnonymous:
				m = SymbolMetadata{Visible: true}
			case lexbuild.VarHidden:
				m = SymbolMetadata{Named: true, Supertype: containsSymbol(g.syntaxGrammar.SupertypeSymbols, symbol)}
			case lexbuild.VarAuxiliary:
			}
		}
		g.out.SymbolMetadata = append(g.out.SymbolMetadata, m)
	}
	for _, alias := range g.uniqueAliases {
		g.out.SymbolMetadata = append(g.out.SymbolMetadata, SymbolMetadata{Visible: true, Named: alias.Named})
	}
}

// addAliasSequences is render.rs: fn Generator::add_alias_sequences.
func (g *generator) addAliasSequences() {
	width := g.parseTable.MaxAliasedProductionLength
	for i := range g.parseTable.ProductionInfos {
		row := make([]uint16, width)
		for j, alias := range g.parseTable.ProductionInfos[i].AliasSequence {
			if alias.Set {
				row[j] = g.value(g.aliasIDs[alias])
			}
		}
		g.out.AliasSequences = append(g.out.AliasSequences, row)
	}
}

// addNonTerminalAliasMap is render.rs: fn Generator::add_non_terminal_alias_map.
// The generator collects the aliases of each symbol in a hash map, then
// sorts its entries by symbol (keys are unique); each symbol's alias
// identifiers are kept sorted as strings, so a symbol's aliases follow the
// order of their C identifiers, not of their numbers.
func (g *generator) addNonTerminalAliasMap() {
	sg := g.syntaxGrammar
	aliasIDsBySymbol := map[Symbol][]string{}
	for i := range sg.Variables {
		vp := sg.VarProds[i]
		for prodID := vp[0]; prodID < vp[1]; prodID++ {
			for _, step := range sg.Productions[prodID].Steps {
				if !step.Alias.Set || !step.Symbol.IsNonTerminal() {
					continue
				}
				if d, ok := g.defaultAliases.get(step.Symbol); ok && aliasEq(d, step.Alias) {
					continue
				}
				if _, ok := g.symbolIDs[step.Symbol]; !ok {
					continue
				}
				aliasID, ok := g.aliasIDs[step.Alias]
				if !ok {
					continue
				}
				ids := aliasIDsBySymbol[step.Symbol]
				k := sort.SearchStrings(ids, aliasID)
				if k == len(ids) || ids[k] != aliasID {
					ids = append(ids, "")
					copy(ids[k+1:], ids[k:])
					ids[k] = aliasID
				}
				aliasIDsBySymbol[step.Symbol] = ids
			}
		}
	}
	symbols := make([]Symbol, 0, len(aliasIDsBySymbol))
	for s := range aliasIDsBySymbol {
		symbols = append(symbols, s)
	}
	sort.Slice(symbols, func(i, j int) bool { return lexbuild.SymbolLess(symbols[i], symbols[j]) })
	var out []uint16
	for _, symbol := range symbols {
		aliasIDs := aliasIDsBySymbol[symbol]
		out = append(out, g.symbolValue(symbol), uint16(1+len(aliasIDs)), g.symbolValue(g.symbolMap[symbol]))
		for _, id := range aliasIDs {
			out = append(out, g.value(id))
		}
	}
	g.out.NonTerminalAliasMap = append(out, 0)
}

// addPrimaryStateIDList is render.rs: fn Generator::add_primary_state_id_list:
// the primary state of a state is the first state with the same core id.
func (g *generator) addPrimaryStateIDList() {
	firstStateForEachCoreID := map[uint32]int{}
	for idx := range g.parseTable.States {
		coreID := g.parseTable.States[idx].CoreID
		primary, ok := firstStateForEachCoreID[coreID]
		if !ok {
			primary = idx
			firstStateForEachCoreID[coreID] = idx
		}
		g.out.PrimaryStateIDs = append(g.out.PrimaryStateIDs, uint16(primary))
	}
}

// flatFieldEntry is one (field name, location) pair of a flat field map.
type flatFieldEntry struct {
	name     string
	location FieldLocation
}

// flatFieldMap is one row of add_field_sequences: its index in
// ts_field_map_entries and its entries.
type flatFieldMap struct {
	index   int
	entries []flatFieldEntry
}

// addFieldSequences is render.rs: fn Generator::add_field_sequences.
func (g *generator) addFieldSequences() {
	var flatFieldMaps []flatFieldMap
	nextFlatFieldMapIndex := 0
	getFieldMapID(nil, &flatFieldMaps, &nextFlatFieldMapIndex)

	slices := make([][2]uint16, len(g.parseTable.ProductionInfos))
	for i := range g.parseTable.ProductionInfos {
		info := &g.parseTable.ProductionInfos[i]
		if len(info.FieldMap) == 0 {
			continue
		}
		// The generator pushes the fields in the order of their string ids,
		// then sorts stably by name: the names come in name order, and the
		// locations of one name in the order they were pushed.
		var flat []flatFieldEntry
		for _, name := range info.FieldNames() {
			for _, location := range info.FieldMap[name] {
				flat = append(flat, flatFieldEntry{name, location})
			}
		}
		slices[i] = [2]uint16{uint16(getFieldMapID(flat, &flatFieldMaps, &nextFlatFieldMapIndex)), uint16(len(flat))}
	}
	g.out.FieldMapSlices = slices
	for _, row := range flatFieldMaps[1:] {
		for _, e := range row.entries {
			g.out.FieldMapEntries = append(g.out.FieldMapEntries, FieldMapEntry{
				FieldID:    g.fieldValue(e.name),
				ChildIndex: e.location.Index,
				Inherited:  e.location.Inherited,
			})
		}
	}
}

// addSupertypeMap is render.rs: fn Generator::add_supertype_map. The
// supertypes are ordered by their C identifiers, and each supertype's
// subtypes are a sorted set of C identifiers.
func (g *generator) addSupertypeMap() {
	maxSymbol := 0
	rowID := 0
	type row struct {
		supertype uint16
		subtypes  []string
	}
	var rows []row
	for _, e := range g.supertypeMap {
		set := map[string]bool{}
		for _, s := range e.subtypes {
			if !s.Aliased {
				if id, ok := g.symbolIDs[s.Symbol]; ok {
					set[id] = true
				}
				continue
			}
			if id, ok := g.aliasIDs[s.Alias]; ok {
				set[id] = true
				continue
			}
			for _, symbol := range g.symbolsForAlias(s.Alias) {
				if id, ok := g.symbolIDs[symbol]; ok {
					set[id] = true
				}
			}
		}
		subtypes := make([]string, 0, len(set))
		for id := range set {
			subtypes = append(subtypes, id)
		}
		sort.Strings(subtypes)
		supertype := g.value(e.identifier)
		g.out.SupertypeSymbols = append(g.out.SupertypeSymbols, supertype)
		rows = append(rows, row{supertype, subtypes})
		maxSymbol = max(maxSymbol, int(supertype))
	}
	g.out.SupertypeMapSlices = make([][2]uint16, maxSymbol+1)
	for _, r := range rows {
		g.out.SupertypeMapSlices[r.supertype] = [2]uint16{uint16(rowID), uint16(len(r.subtypes))}
		rowID += len(r.subtypes)
		for _, id := range r.subtypes {
			g.out.SupertypeMapEntries = append(g.out.SupertypeMapEntries, g.value(id))
		}
	}
}

// addReservedWordSets is render.rs: fn Generator::add_reserved_word_sets.
func (g *generator) addReservedWordSets() {
	width := g.out.MaxReservedWordSetSize
	g.out.ReservedWords = make([][]uint16, len(g.reservedWordSets))
	for id := range g.reservedWordSets {
		row := make([]uint16, width)
		for k, token := range g.reservedWordSets[id].Symbols() {
			row[k] = g.symbolValue(token)
		}
		g.out.ReservedWords[id] = row
	}
}

// smallGroupKey is the key of render.rs: add_parse_table's symbols_by_value:
// a value and the kind of its symbols (SymbolType::Terminal for an action
// list offset, SymbolType::NonTerminal for a state).
type smallGroupKey struct {
	value uint32
	kind  lexbuild.SymbolType
}

// addParseTable is render.rs: fn Generator::add_parse_table. Offsets are
// given to action lists in order of first use: the states in order, and in
// each state its terminal entries in symbol id order.
func (g *generator) addParseTable() error {
	table := g.parseTable
	parseTableEntries := map[ActionListID]uint32{}
	var order []ActionListID
	nextParseActionListIndex := uint32(0)
	getID := func(id ActionListID) uint32 {
		return getParseActionListID(id, &table.ActionLists, parseTableEntries, &order, &nextParseActionListIndex)
	}

	// Parse action list zero is for the default value, when a symbol is not
	// valid. canonicalize guarantees pool index 0 is the empty list.
	getID(newActionListID(0, false))

	// The generator sorts each state's entries by symbol_order (unique keys:
	// no two entries have one symbol; the end of non-terminal extra token
	// has no order and sorts first) and the non-terminal entries by symbol.
	var terminalEntries []TerminalActionList
	sortTerminals := func(state *InternedState) {
		terminalEntries = append(terminalEntries[:0], state.TerminalEntries...)
		sort.Slice(terminalEntries, func(a, b int) bool {
			oa, okA := g.symbolOrder[terminalEntries[a].Symbol]
			ob, okB := g.symbolOrder[terminalEntries[b].Symbol]
			if okA != okB {
				return !okA
			}
			return oa < ob
		})
	}

	for i := 0; i < g.largeStateCount; i++ {
		state := &table.States[i]
		row := make([]uint16, len(table.Symbols))
		sortTerminals(state)
		for _, e := range state.NonterminalEntries {
			value := uint16(i)
			if !e.Action.ShiftExtra {
				value = uint16(e.Action.State)
			}
			row[g.symbolValue(e.Symbol)] = value
		}
		for _, e := range terminalEntries {
			row[g.symbolValue(e.Symbol)] = uint16(getID(e.ID))
		}
		g.out.ParseTable = append(g.out.ParseTable, row)
	}

	if g.largeStateCount < len(table.States) {
		nextTableIndex := 0
		var smallStateIndices []uint32
		for si := g.largeStateCount; si < len(table.States); si++ {
			state := &table.States[si]
			smallStateIndices = append(smallStateIndices, uint32(nextTableIndex))
			symbolsByValue := map[smallGroupKey][]Symbol{}
			var keys []smallGroupKey
			add := func(k smallGroupKey, s Symbol) {
				if _, ok := symbolsByValue[k]; !ok {
					keys = append(keys, k)
				}
				symbolsByValue[k] = append(symbolsByValue[k], s)
			}

			sortTerminals(state)
			// In a given parse state, many lookahead symbols have the same
			// actions. So in the "small state" representation, group symbols
			// by their action in order to avoid repeating the action.
			for _, e := range terminalEntries {
				add(smallGroupKey{getID(e.ID), lexbuild.Terminal}, e.Symbol)
			}
			for _, e := range state.NonterminalEntries {
				stateID := uint32(g.largeStateCount + len(smallStateIndices) - 1)
				if !e.Action.ShiftExtra {
					stateID = e.Action.State
				}
				add(smallGroupKey{stateID, lexbuild.NonTerminal}, e.Symbol)
			}

			// The generator drains the hash map and sorts the groups by
			// (symbol count, kind, value, first symbol); (kind, value) is
			// unique, so the hash order reaches no result.
			sort.Slice(keys, func(a, b int) bool {
				ka, kb := keys[a], keys[b]
				la, lb := len(symbolsByValue[ka]), len(symbolsByValue[kb])
				if la != lb {
					return la < lb
				}
				if ka.kind != kb.kind {
					return ka.kind < kb.kind
				}
				return ka.value < kb.value
			})

			g.out.SmallParseTable = append(g.out.SmallParseTable, uint16(len(keys)))
			nextTableIndex++
			for _, k := range keys {
				symbols := symbolsByValue[k]
				nextTableIndex += 2 + len(symbols)
				g.out.SmallParseTable = append(g.out.SmallParseTable, uint16(k.value), uint16(len(symbols)))
				sort.Slice(symbols, func(a, b int) bool { return lexbuild.SymbolLess(symbols[a], symbols[b]) })
				for _, s := range symbols {
					g.out.SmallParseTable = append(g.out.SmallParseTable, g.symbolValue(s))
				}
			}
		}
		g.out.SmallParseTableMap = smallStateIndices
	}

	if nextParseActionListIndex >= 65535 {
		return &ParseActionCountError{int(nextParseActionListIndex)}
	}

	// The offsets are listed in increasing order (sort_by_key on unique
	// offsets): the order in which getParseActionListID gave them.
	for _, id := range order {
		g.addParseActionList(id, parseTableEntries[id])
	}
	return nil
}

// addParseActionList is render.rs: fn Generator::add_parse_action_list, for
// one list.
func (g *generator) addParseActionList(id ActionListID, offset uint32) {
	actions := g.parseTable.ActionLists.Get(id)
	entry := ActionListEntry{Offset: offset, Reusable: id.Reusable(), Actions: make([]RenderedAction, len(actions))}
	for k, a := range actions {
		r := RenderedAction{Kind: a.Kind}
		switch a.Kind {
		case ActionShift:
			r.State, r.IsRepetition = a.State, a.IsRepetition
		case ActionReduce:
			r.Symbol = g.symbolValue(a.Symbol)
			r.ChildCount, r.DynamicPrecedence, r.ProductionID = a.ChildCount, a.DynamicPrecedence, a.ProductionID
		}
		entry.Actions[k] = r
	}
	g.out.ParseActions = append(g.out.ParseActions, entry)
}

// getParseActionListID is render.rs: fn Generator::get_parse_action_list_id:
// the offset of a list (and reusable flag) in ts_parse_actions, given at its
// first use; each list takes one header slot and one slot per action. The
// offsets map is read only by key; order records the first uses.
func getParseActionListID(id ActionListID, pool *ActionListPool, offsets map[ActionListID]uint32, order *[]ActionListID, next *uint32) uint32 {
	if index, ok := offsets[id]; ok {
		return index
	}
	result := *next
	offsets[id] = result
	*order = append(*order, id)
	*next += 1 + uint32(len(pool.Get(id)))
	return result
}

// getFieldMapID is render.rs: fn Generator::get_field_map_id: the index of
// the first equal row, or of a new row placed after the last one.
func getFieldMapID(flat []flatFieldEntry, flatFieldMaps *[]flatFieldMap, next *int) int {
	for _, m := range *flatFieldMaps {
		if flatFieldEntriesEqual(m.entries, flat) {
			return m.index
		}
	}
	result := *next
	*next += len(flat)
	*flatFieldMaps = append(*flatFieldMaps, flatFieldMap{result, flat})
	return result
}

func flatFieldEntriesEqual(a, b []flatFieldEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assignSymbolID is render.rs: fn Generator::assign_symbol_id: the C
// identifier of a symbol, made unique with a number suffix.
func (g *generator) assignSymbolID(symbol Symbol, usedIdentifiers map[string]bool) {
	var id string
	if symbol == lexbuild.EndSym() {
		id = "ts_builtin_sym_end"
	} else {
		name, kind := g.metadataForSymbol(symbol)
		switch kind {
		case lexbuild.VarAuxiliary:
			id = "aux_sym_" + sanitizeIdentifier(name)
		case lexbuild.VarAnonymous:
			id = "anon_sym_" + sanitizeIdentifier(name)
		default:
			id = "sym_" + sanitizeIdentifier(name)
		}
		suffixNumber, suffix := 1, ""
		for usedIdentifiers[id] {
			id = id[:len(id)-len(suffix)]
			suffixNumber++
			suffix = strconv.Itoa(suffixNumber)
			id += suffix
		}
	}
	usedIdentifiers[id] = true
	g.symbolIDs[symbol] = id
}

// metadataForSymbol is render.rs: fn Generator::metadata_for_symbol.
func (g *generator) metadataForSymbol(symbol Symbol) (string, lexbuild.VariableType) {
	switch symbol.Kind {
	case lexbuild.End, lexbuild.EndOfNonTerminalExtra:
		return "end", lexbuild.VarHidden
	case lexbuild.NonTerminal:
		v := g.syntaxGrammar.Variables[symbol.Index]
		return v.Ident, v.Kind
	case lexbuild.Terminal:
		v := g.lexicalGrammar.Variables[symbol.Index]
		return v.Ident, v.Kind
	}
	t := g.syntaxGrammar.ExternalTokens[symbol.Index]
	return t.Ident, t.Kind
}

// symbolsForAlias is render.rs: fn Generator::symbols_for_alias: the symbols
// whose default alias is the alias, or that have no default alias and the
// alias's name and kind.
func (g *generator) symbolsForAlias(alias Alias) []Symbol {
	var out []Symbol
	for _, symbol := range g.parseTable.Symbols {
		if d, ok := g.defaultAliases.get(symbol); ok {
			if aliasEq(d, alias) {
				out = append(out, symbol)
			}
			continue
		}
		if name, kind := g.metadataForSymbol(symbol); name == alias.Value && kind == aliasKind(alias) {
			out = append(out, symbol)
		}
	}
	return out
}

// sanitizeIdentifier is render.rs: fn Generator::sanitize_identifier.
func sanitizeIdentifier(name string) string {
	var b strings.Builder
	for _, c := range name {
		if c < utf8.RuneSelf && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			b.WriteRune(c)
			continue
		}
		var replacement string
		switch {
		case c == ' ' && len(name) == 1:
			replacement = "SPACE"
		case c == ' ':
			continue
		case c >= 0x80 && c <= 0xFFFF && c != 0xFEFF:
			fmt.Fprintf(&b, "u%04x", c)
			continue
		case c >= 0x10000:
			fmt.Fprintf(&b, "U%08x", c)
			continue
		default:
			replacement = specialCharNames[c]
		}
		if s := b.String(); s != "" && !strings.HasSuffix(s, "_") {
			b.WriteByte('_')
		}
		b.WriteString(replacement)
	}
	return b.String()
}

// specialCharNames holds the replacements of render.rs: fn sanitize_identifier.
var specialCharNames = map[rune]string{
	'~': "TILDE", '`': "BQUOTE", '!': "BANG", '@': "AT", '#': "POUND", '$': "DOLLAR", '%': "PERCENT",
	'^': "CARET", '&': "AMP", '*': "STAR", '(': "LPAREN", ')': "RPAREN", '-': "DASH", '+': "PLUS",
	'=': "EQ", '{': "LBRACE", '}': "RBRACE", '[': "LBRACK", ']': "RBRACK", '\\': "BSLASH", '|': "PIPE",
	':': "COLON", ';': "SEMI", '"': "DQUOTE", '\'': "SQUOTE", '<': "LT", '>': "GT", ',': "COMMA",
	'.': "DOT", '?': "QMARK", '/': "SLASH", '\n': "LF", '\r': "CR", '\t': "TAB", 0: "NULL",
	1: "SOH", 2: "STX", 3: "ETX", 4: "EOT", 5: "ENQ", 6: "ACK", 7: "BEL", 8: "BS", 0x0b: "VTAB",
	0x0c: "FF", 0x0e: "SO", 0x0f: "SI", 0x10: "DLE", 0x11: "DC1", 0x12: "DC2", 0x13: "DC3",
	0x14: "DC4", 0x15: "NAK", 0x16: "SYN", 0x17: "ETB", 0x18: "CAN", 0x19: "EM", 0x1a: "SUB",
	0x1b: "ESC", 0x1c: "FS", 0x1d: "GS", 0x1e: "RS", 0x1f: "US", 0x7f: "DEL", 0xFEFF: "BOM",
}
