package tablebuild

import (
	"fmt"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports the parse table types of tables.rs and the search of
// build_tables/build_parse_table.rs: fn ParseTableBuilder::new, build,
// add_parse_state and add_actions, and fn build_parse_table. conflict.go
// ports handle_conflict, compare_precedence and get_auxiliary_node_info;
// prodinfo.go ports get_production_id.

// ParseActionKind is the variant of tables.rs: enum ParseAction, in the
// order of the enum (the derived Ord compares the variant first).
type ParseActionKind uint8

const (
	ActionAccept ParseActionKind = iota
	ActionShift
	ActionShiftExtra
	ActionRecover
	ActionReduce
)

// ParseAction is tables.rs: enum ParseAction. Fields that a variant does not
// have are zero, so == is the derived equality.
type ParseAction struct {
	Kind ParseActionKind
	// State and IsRepetition belong to Shift.
	State        uint32
	IsRepetition bool
	// Symbol, ChildCount, DynamicPrecedence and ProductionID belong to Reduce.
	Symbol            Symbol
	ChildCount        uint16
	DynamicPrecedence int32
	ProductionID      uint16
}

// GotoAction is tables.rs: enum GotoAction: Goto(State), or ShiftExtra.
type GotoAction struct {
	ShiftExtra bool
	State      uint32
}

// ParseTableEntry is tables.rs: struct ParseTableEntry.
type ParseTableEntry struct {
	Actions  []ParseAction
	Reusable bool
}

// newParseTableEntry is tables.rs: fn ParseTableEntry::new.
func newParseTableEntry() ParseTableEntry { return ParseTableEntry{Reusable: true} }

// TerminalEntry is one entry of ParseState.TerminalEntries.
type TerminalEntry struct {
	Symbol Symbol
	Entry  ParseTableEntry
}

// NonterminalEntry is one entry of ParseState.NonterminalEntries.
type NonterminalEntry struct {
	Symbol Symbol
	Action GotoAction
}

// ParseState is tables.rs: struct ParseState, for a table whose entries are
// ParseTableEntry values. The two entry maps are IndexMaps in the generator:
// they keep insertion order, and inserting an existing key replaces its value
// in place. The slices keep that order.
type ParseState struct {
	ID                 uint32
	TerminalEntries    []TerminalEntry
	NonterminalEntries []NonterminalEntry
	ReservedWords      lexbuild.TokenSet
	LexStateID         uint32
	ExternalLexStateID uint32
	CoreID             uint32
	HasEOFGatedReduce  bool
}

// terminalIndex returns the position of the symbol's terminal entry, or -1.
func (s *ParseState) terminalIndex(sym Symbol) int {
	for i := range s.TerminalEntries {
		if s.TerminalEntries[i].Symbol == sym {
			return i
		}
	}
	return -1
}

// terminalEntryOrInsert is IndexMap::entry(sym).or_insert(value): the
// position of the symbol's entry, appended with value if absent.
func (s *ParseState) terminalEntryOrInsert(sym Symbol, value ParseTableEntry) int {
	if i := s.terminalIndex(sym); i >= 0 {
		return i
	}
	s.TerminalEntries = append(s.TerminalEntries, TerminalEntry{sym, value})
	return len(s.TerminalEntries) - 1
}

// insertTerminal is IndexMap::insert: replace the value in place, or append.
func (s *ParseState) insertTerminal(sym Symbol, value ParseTableEntry) {
	if i := s.terminalIndex(sym); i >= 0 {
		s.TerminalEntries[i].Entry = value
		return
	}
	s.TerminalEntries = append(s.TerminalEntries, TerminalEntry{sym, value})
}

// insertNonterminal is IndexMap::insert on the non-terminal entries.
func (s *ParseState) insertNonterminal(sym Symbol, action GotoAction) {
	for i := range s.NonterminalEntries {
		if s.NonterminalEntries[i].Symbol == sym {
			s.NonterminalEntries[i].Action = action
			return
		}
	}
	s.NonterminalEntries = append(s.NonterminalEntries, NonterminalEntry{sym, action})
}

// isEndOfNonTerminalExtra is tables.rs: fn ParseState::is_end_of_non_terminal_extra.
func (s *ParseState) isEndOfNonTerminalExtra() bool {
	return s.terminalIndex(lexbuild.EndOfNonTerminalExtraSym()) >= 0
}

// FieldLocation is tables.rs: struct FieldLocation.
type FieldLocation struct {
	Index     uint32
	Inherited bool
}

// ProductionInfo is tables.rs: struct ProductionInfo. AliasSequence holds an
// Alias with Set false where the generator holds None. FieldMap is a BTreeMap
// keyed by the field's string id in the generator; only its keys and values
// are read here (FieldNames lists the keys by name).
type ProductionInfo struct {
	AliasSequence []Alias
	FieldMap      map[string][]FieldLocation
}

// FieldNames returns the keys of FieldMap in byte order.
func (p *ProductionInfo) FieldNames() []string {
	out := make([]string, 0, len(p.FieldMap))
	for k := range p.FieldMap {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// equal is the derived PartialEq of ProductionInfo.
func (p *ProductionInfo) equal(o *ProductionInfo) bool {
	if len(p.AliasSequence) != len(o.AliasSequence) || len(p.FieldMap) != len(o.FieldMap) {
		return false
	}
	for i := range p.AliasSequence {
		if !aliasEq(p.AliasSequence[i], o.AliasSequence[i]) {
			return false
		}
	}
	for k, v := range p.FieldMap {
		w, ok := o.FieldMap[k]
		if !ok || len(v) != len(w) {
			return false
		}
		for i := range v {
			if v[i] != w[i] {
				return false
			}
		}
	}
	return true
}

// ParseTable is tables.rs: struct ParseTable, with ParseTableEntry values
// (the table before ActionListPool::intern_table).
type ParseTable struct {
	States                     []ParseState
	Symbols                    []Symbol
	ProductionInfos            []ProductionInfo
	MaxAliasedProductionLength int
	ExternalLexStates          []lexbuild.TokenSet
}

// ParseStateInfo is build_parse_table.rs: struct ParseStateInfo.
type ParseStateInfo struct {
	PrecedingSymbolsByID [][]Symbol
	// ItemSets holds the kernel of each state, by state id (the keys of
	// item_sets_by_ids in insertion order).
	ItemSets   []ParseItemSet
	lookaheads *lookaheadSetPool
}

// Lookaheads returns the lookahead tokens of an entry of one of the item sets.
func (p *ParseStateInfo) Lookaheads(e *ParseItemSetEntry) *lexbuild.TokenSet {
	return p.lookaheads.get(e.lookaheads)
}

// auxiliarySymbolInfo is build_parse_table.rs: struct AuxiliarySymbolInfo.
type auxiliarySymbolInfo struct {
	auxiliarySymbol Symbol
	parentSymbols   []Symbol
}

func (a *auxiliarySymbolInfo) equal(o *auxiliarySymbolInfo) bool {
	if a.auxiliarySymbol != o.auxiliarySymbol || len(a.parentSymbols) != len(o.parentSymbols) {
		return false
	}
	for i := range a.parentSymbols {
		if a.parentSymbols[i] != o.parentSymbols[i] {
			return false
		}
	}
	return true
}

// reductionInfo is build_parse_table.rs: struct ReductionInfo.
type reductionInfo struct {
	precedence    lexbuild.Precedence
	symbols       []Symbol
	hasLeftAssoc  bool
	hasRightAssoc bool
	hasNonAssoc   bool
}

// parseStateQueueEntry is build_parse_table.rs: struct ParseStateQueueEntry.
type parseStateQueueEntry struct {
	stateID                   uint32
	precedingAuxiliarySymbols []auxiliarySymbolInfo
}

// ParseTableBuilderError is build_parse_table.rs: enum ParseTableBuilderError.
// The variants are *ConflictError, *AmbiguousExtraError,
// *ImproperNonTerminalExtraError and *StateCountError.
type ParseTableBuilderError interface {
	error
	parseTableBuilderError()
}

// ImproperNonTerminalExtraError is ParseTableBuilderError::ImproperNonTerminalExtra.
type ImproperNonTerminalExtraError struct{ Name string }

func (e *ImproperNonTerminalExtraError) Error() string {
	return fmt.Sprintf("The non-terminal rule `%s` is used in a non-terminal `extra` rule, which is not allowed.", e.Name)
}
func (*ImproperNonTerminalExtraError) parseTableBuilderError() {}

// StateCountError is ParseTableBuilderError::StateCount.
type StateCountError struct{ Count int }

func (e *StateCountError) Error() string {
	return fmt.Sprintf("State count `%d` exceeds the max value 65535.", e.Count)
}
func (*StateCountError) parseTableBuilderError() {}

// GeneratorPanicError reports an input on which the generator panics (an
// index out of range or an unwrap of None). The port stops with this error
// where the generator would stop with a panic.
type GeneratorPanicError struct{ What string }

func (e *GeneratorPanicError) Error() string {
	return "tablebuild: the generator panics here: " + e.What
}

type generatorPanic struct{ err *GeneratorPanicError }

func panicLikeGenerator(format string, args ...any) {
	panic(generatorPanic{&GeneratorPanicError{fmt.Sprintf(format, args...)}})
}

// Diagnostics holds the warnings that the generator collects while it builds
// the parse table (lib.rs: enum Diagnostic, the variants this stage makes).
type Diagnostics struct {
	// UnnecessaryConflicts lists the declared conflicts that no state needed,
	// each as its symbol names, sorted.
	UnnecessaryConflicts [][]string
}

// parseTableBuilder is build_parse_table.rs: struct ParseTableBuilder.
type parseTableBuilder struct {
	itemSetBuilder *ParseItemSetBuilder
	sg             *lexbuild.SyntaxGrammar
	lg             *lexbuild.LexicalGrammar
	variableInfo   []VariableInfo
	coreIDsByCore  map[string]uint32
	// stateIDsByItemSet and kernels together are state_ids_by_item_set (an
	// IndexMap from kernel to state id; the id is the insertion index).
	stateIDsByItemSet         map[string]uint32
	kernels                   []ParseItemSet
	precedingSymbolsByID      [][]Symbol
	productionInfoIDsByProdID []int32
	parseStateQueue           []parseStateQueueEntry
	queueHead                 int
	nonTerminalExtraStates    []symbolState
	// actualConflicts is actual_conflicts, a hash set of symbol lists; it is
	// read only through membership and, at the end, sorted names.
	actualConflicts map[string][]Symbol
	parseTable      ParseTable
	diagnostics     Diagnostics

	// pos maps a terminal entry symbol of the state that add_actions is
	// filling to its position in TerminalEntries (Go shape: a dense index
	// instead of the IndexMap's hash lookup).
	pos  entryPositions
	work int64 // terminal and non-terminal entries built (bTableEntries)
}

type symbolState struct {
	symbol Symbol
	state  uint32
}

// entryPositions is a dense map from a token symbol to an entry position.
type entryPositions struct {
	terminals, externals []int32
	end, eonte           int32
}

func newEntryPositions(nTerminals, nExternals int) entryPositions {
	p := entryPositions{terminals: make([]int32, nTerminals), externals: make([]int32, nExternals), end: -1, eonte: -1}
	for i := range p.terminals {
		p.terminals[i] = -1
	}
	for i := range p.externals {
		p.externals[i] = -1
	}
	return p
}

func (p *entryPositions) slot(sym Symbol) *int32 {
	switch sym.Kind {
	case lexbuild.Terminal:
		return &p.terminals[sym.Index]
	case lexbuild.External:
		return &p.externals[sym.Index]
	case lexbuild.End:
		return &p.end
	case lexbuild.EndOfNonTerminalExtra:
		return &p.eonte
	}
	panicLikeGenerator("a non-terminal symbol in a token set")
	return nil
}

// load indexes the terminal entries of a state; reset clears them again.
func (p *entryPositions) load(s *ParseState) {
	for i := range s.TerminalEntries {
		*p.slot(s.TerminalEntries[i].Symbol) = int32(i)
	}
}

func (p *entryPositions) reset(s *ParseState) {
	for i := range s.TerminalEntries {
		*p.slot(s.TerminalEntries[i].Symbol) = -1
	}
}

// entry is IndexMap::entry(sym).or_insert(value) on the indexed state.
func (b *parseTableBuilder) entry(s *ParseState, sym Symbol, value ParseTableEntry) *ParseTableEntry {
	slot := b.pos.slot(sym)
	if *slot < 0 {
		*slot = int32(len(s.TerminalEntries))
		s.TerminalEntries = append(s.TerminalEntries, TerminalEntry{sym, value})
		b.work++
		bTableEntries.checkWork(b.work)
	}
	return &s.TerminalEntries[*slot].Entry
}

// lookup is IndexMap::get on the indexed state.
func (b *parseTableBuilder) lookup(s *ParseState, sym Symbol) *ParseTableEntry {
	if i := *b.pos.slot(sym); i >= 0 {
		return &s.TerminalEntries[i].Entry
	}
	return nil
}

func conflictKey(syms []Symbol) string {
	var sb strings.Builder
	for _, s := range syms {
		fmt.Fprintf(&sb, "%d:%d,", s.Kind, s.Index)
	}
	return sb.String()
}

// newParseTableBuilder is build_parse_table.rs: fn ParseTableBuilder::new.
func newParseTableBuilder(sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, itemSetBuilder *ParseItemSetBuilder, variableInfo []VariableInfo) *parseTableBuilder {
	b := &parseTableBuilder{
		itemSetBuilder:            itemSetBuilder,
		sg:                        sg,
		lg:                        lg,
		variableInfo:              variableInfo,
		coreIDsByCore:             map[string]uint32{},
		stateIDsByItemSet:         map[string]uint32{},
		productionInfoIDsByProdID: make([]int32, len(sg.Productions)),
		actualConflicts:           map[string][]Symbol{},
		parseTable:                ParseTable{MaxAliasedProductionLength: 1},
		pos:                       newEntryPositions(len(lg.Variables), len(sg.ExternalTokens)),
	}
	for i := range b.productionInfoIDsByProdID {
		b.productionInfoIDsByProdID[i] = -1
	}
	for _, c := range sg.ExpectedConflicts {
		b.actualConflicts[conflictKey(c)] = c
	}
	return b
}

// build is build_parse_table.rs: fn ParseTableBuilder::build.
func (b *parseTableBuilder) build() (*ParseTable, *ParseStateInfo, error) {
	sg := b.sg
	// Ensure that the empty alias sequence has index 0.
	b.parseTable.ProductionInfos = append(b.parseTable.ProductionInfos, ProductionInfo{})

	// Add the error state at index 0.
	b.addParseState(nil, nil, ParseItemSet{})

	// Add the starting state at index 1.
	endLookaheads := b.itemSetBuilder.lookaheads.singleton(lexbuild.EndSym())
	b.addParseState(nil, nil, ParseItemSet{Entries: []ParseItemSetEntry{{
		Item:       startItem(b.itemSetBuilder.keyMap),
		lookaheads: endLookaheads,
	}}})

	// Compute the possible item sets for non-terminal extras: a BTreeMap from
	// the first symbol of a production to an item set.
	setsByFirst := map[Symbol]*ParseItemSet{}
	var firsts []Symbol
	for _, extra := range sg.ExtraSymbols {
		if !extra.IsNonTerminal() {
			continue
		}
		vp := sg.VarProds[extra.Index]
		for prodID := uint32(vp[0]); prodID < uint32(vp[1]); prodID++ {
			first, ok := production(sg, prodID).firstSymbol()
			if !ok {
				panicLikeGenerator("a production of the non-terminal extra %q is empty (first_symbol().unwrap())", sg.Variables[extra.Index].Ident)
			}
			set, ok := setsByFirst[first]
			if !ok {
				set = &ParseItemSet{}
				setsByFirst[first] = set
				firsts = append(firsts, first)
			}
			entry := set.insert(ParseItem{
				VariableIndex: extra.Index,
				ProdID:        prodID,
				StepIndex:     1,
				keys:          b.itemSetBuilder.keyMap.keysFor(prodID),
			})
			entry.lookaheads = b.itemSetBuilder.lookaheads.insert(entry.lookaheads, lexbuild.EndOfNonTerminalExtraSym())
		}
	}
	sort.Slice(firsts, func(i, j int) bool { return lexbuild.SymbolLess(firsts[i], firsts[j]) })

	// Add a state for each starting terminal of a non-terminal extra rule.
	for _, terminal := range firsts {
		if terminal.IsNonTerminal() {
			return nil, nil, &ImproperNonTerminalExtraError{b.symbolName(terminal)}
		}
		// Add the parse state, and then push the terminal and the state id
		// into the list of non-terminal extra states.
		stateID := b.addParseState(nil, nil, *setsByFirst[terminal])
		b.nonTerminalExtraStates = append(b.nonTerminalExtraStates, symbolState{terminal, stateID})
	}

	for b.queueHead < len(b.parseStateQueue) {
		entry := b.parseStateQueue[b.queueHead]
		b.parseStateQueue[b.queueHead] = parseStateQueueEntry{}
		b.queueHead++
		// The dedup-map key is each state's kernel (the GOTO result, before
		// the closure). Two states are identical if their kernels match.
		kernel := &b.kernels[entry.stateID]
		itemSet := b.itemSetBuilder.transitiveClosure(kernel)
		preceding := append([]Symbol(nil), b.precedingSymbolsByID[entry.stateID]...)
		if err := b.addActions(preceding, entry.precedingAuxiliarySymbols, entry.stateID, &itemSet); err != nil {
			return nil, nil, err
		}
	}

	if len(b.actualConflicts) > 0 {
		var conflicts [][]string
		for _, c := range b.actualConflicts {
			names := make([]string, len(c))
			for i, s := range c {
				names[i] = b.symbolName(s)
			}
			conflicts = append(conflicts, names)
		}
		sort.Slice(conflicts, func(i, j int) bool { return lessStrings(conflicts[i], conflicts[j]) })
		b.diagnostics.UnnecessaryConflicts = conflicts
	}

	return &b.parseTable, &ParseStateInfo{
		PrecedingSymbolsByID: b.precedingSymbolsByID,
		ItemSets:             b.kernels,
		lookaheads:           b.itemSetBuilder.lookaheads,
	}, nil
}

// lessStrings is the derived Ord of Vec<String>.
func lessStrings(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// addParseState is build_parse_table.rs: fn ParseTableBuilder::add_parse_state.
func (b *parseTableBuilder) addParseState(precedingSymbols []Symbol, precedingAuxiliarySymbols []auxiliarySymbolInfo, itemSet ParseItemSet) uint32 {
	key := itemSet.key()
	// If an equivalent item set has already been processed, return the
	// existing parse state index.
	if id, ok := b.stateIDsByItemSet[key]; ok {
		return id
	}
	// Otherwise, insert a new parse state and add it to the queue of parse
	// states to populate.
	core := itemSet.core()
	coreKey := core.key()
	coreID, ok := b.coreIDsByCore[coreKey]
	if !ok {
		coreID = uint32(len(b.coreIDsByCore))
		b.coreIDsByCore[coreKey] = coreID
	}

	stateID := uint32(len(b.parseTable.States))
	bParseStates.check(int(stateID) + 1)
	b.precedingSymbolsByID = append(b.precedingSymbolsByID, append([]Symbol(nil), precedingSymbols...))
	b.parseTable.States = append(b.parseTable.States, ParseState{ID: stateID, CoreID: coreID})
	b.parseStateQueue = append(b.parseStateQueue, parseStateQueueEntry{
		stateID:                   stateID,
		precedingAuxiliarySymbols: append([]auxiliarySymbolInfo(nil), precedingAuxiliarySymbols...),
	})
	b.stateIDsByItemSet[key] = stateID
	b.kernels = append(b.kernels, itemSet)
	return stateID
}

// successorSets is a BTreeMap from a symbol to the item set reached over it.
type successorSets struct {
	sets    map[Symbol]*ParseItemSet
	symbols []Symbol
}

func (s *successorSets) entry(sym Symbol) *ParseItemSet {
	if s.sets == nil {
		s.sets = map[Symbol]*ParseItemSet{}
	}
	set, ok := s.sets[sym]
	if !ok {
		set = &ParseItemSet{}
		s.sets[sym] = set
		s.symbols = append(s.symbols, sym)
	}
	return set
}

// sorted lists the symbols in the order of the BTreeMap.
func (s *successorSets) sorted() []Symbol {
	sort.Slice(s.symbols, func(i, j int) bool { return lexbuild.SymbolLess(s.symbols[i], s.symbols[j]) })
	return s.symbols
}

// addActions is build_parse_table.rs: fn ParseTableBuilder::add_actions.
func (b *parseTableBuilder) addActions(precedingSymbols []Symbol, precedingAuxiliarySymbols []auxiliarySymbolInfo, stateID uint32, itemSet *ParseItemSet) error {
	sg := b.sg
	pool := b.itemSetBuilder.lookaheads
	var terminalSuccessors, nonTerminalSuccessors successorSets
	var lookaheadsWithConflicts lexbuild.TokenSet
	reductionInfos := map[Symbol]*reductionInfo{}

	// get_auxiliary_node_info scans every entry of the item set, and the same
	// auxiliary symbol typically appears in many entries: memoize per symbol.
	auxNodeInfo := map[Symbol]auxiliarySymbolInfo{}

	state := &b.parseTable.States[stateID]
	b.pos.load(state)
	defer func() { b.pos.reset(&b.parseTable.States[stateID]) }()

	// Each item in the item set contributes either a Shift action or a Reduce
	// action in this state.
	for k := range itemSet.Entries {
		ent := &itemSet.Entries[k]
		item := &ent.Item
		// If the item is unfinished, then this state has a transition for the
		// item's next symbol. Advance the item to its next step and insert the
		// result into the successor item set.
		if nextSymbol, ok := item.symbol(sg); ok {
			successor := item.successor()
			var successorSet *ParseItemSet
			if nextSymbol.IsNonTerminal() {
				variable := sg.Variables[nextSymbol.Index]

				// Keep track of where auxiliary non-terminals (repeat symbols)
				// are used within visible symbols. This information may be
				// needed later for conflict resolution.
				if variable.Kind == lexbuild.VarAuxiliary {
					info, ok := auxNodeInfo[nextSymbol]
					if !ok {
						info = b.getAuxiliaryNodeInfo(itemSet, nextSymbol)
						auxNodeInfo[nextSymbol] = info
					}
					precedingAuxiliarySymbols = append(precedingAuxiliarySymbols, info)
				}

				// For most parse items, the symbols of the preceding children
				// do not matter: they have no effect on the Reduce action at the
				// end of the item. But they do matter for children that are
				// hidden and have fields, because those fields are inherited by
				// the parent node.
				if isHidden(variable.Kind) && len(b.variableInfo[nextSymbol.Index].Fields) > 0 {
					successor.HasPrecedingInheritedFields = true
				}
				successorSet = nonTerminalSuccessors.entry(nextSymbol)
			} else {
				successorSet = terminalSuccessors.entry(nextSymbol)
			}
			successorEntry := successorSet.insert(successor)
			successorEntry.lookaheads = pool.union(successorEntry.lookaheads, ent.lookaheads)
			successorEntry.FollowingReservedWordSet = max(successorEntry.FollowingReservedWordSet, ent.FollowingReservedWordSet)
			continue
		}

		// If the item is finished, add a Reduce action to this state based on
		// this item.
		symbol := lexbuild.NonTerminalSym(int(item.VariableIndex))
		var action ParseAction
		if item.isAugmented() {
			action = ParseAction{Kind: ActionAccept}
		} else {
			// The generator narrows these values to u16 (a debug assertion
			// checks the range; the release binary truncates).
			productionID := b.getProductionID(item)
			action = ParseAction{
				Kind:              ActionReduce,
				Symbol:            symbol,
				ChildCount:        uint16(item.StepIndex),
				DynamicPrecedence: item.production(sg).dynamicPrecedence,
				ProductionID:      uint16(productionID),
			}
		}

		precedence := item.precedence(sg)
		associativity := item.associativity(sg)
		requiresEOF := item.production(sg).requiresEOF
		if requiresEOF {
			state.HasEOFGatedReduce = true
		}
		for _, lookahead := range pool.get(ent.lookaheads).Symbols() {
			if requiresEOF && lookahead != lexbuild.EndSym() {
				continue
			}
			tableEntry := b.entry(state, lookahead, newParseTableEntry())
			ri := reductionInfos[lookahead]
			if ri == nil {
				ri = &reductionInfo{}
				reductionInfos[lookahead] = ri
			}

			// While inserting Reduce actions, eagerly resolve conflicts related
			// to precedence: avoid inserting lower-precedence reductions, and
			// clear the action list when inserting higher-precedence ones.
			if len(tableEntry.Actions) == 0 {
				tableEntry.Actions = append(tableEntry.Actions, action)
			} else {
				switch comparePrecedence(sg, precedence, []Symbol{symbol}, ri.precedence, ri.symbols) {
				case 1:
					tableEntry.Actions = []ParseAction{action}
					lookaheadsWithConflicts.Remove(lookahead)
					*ri = reductionInfo{}
				case 0:
					// Two items that reduce identically build the same tree, so
					// there is nothing for the user to resolve. Precedence is
					// still compared first, because an item that only repeats an
					// existing action can still outrank it and clear the entry.
					if !containsAction(tableEntry.Actions, action) {
						tableEntry.Actions = append(tableEntry.Actions, action)
						lookaheadsWithConflicts.Insert(lookahead)
					}
				default:
					continue
				}
			}

			ri.precedence = precedence
			insertSortedSymbol(&ri.symbols, symbol)
			switch associativity {
			case lexbuild.AssocLeft:
				ri.hasLeftAssoc = true
			case lexbuild.AssocRight:
				ri.hasRightAssoc = true
			default:
				ri.hasNonAssoc = true
			}
		}
	}

	precedingAuxiliarySymbols = dedupAux(precedingAuxiliarySymbols)

	// Having computed the successor item sets for each symbol, add a new
	// parse state for each of these item sets, and add a corresponding Shift
	// action to this state.
	for _, symbol := range terminalSuccessors.sorted() {
		precedingSymbols = append(precedingSymbols, symbol)
		nextStateID := b.addParseState(precedingSymbols, precedingAuxiliarySymbols, *terminalSuccessors.sets[symbol])
		precedingSymbols = precedingSymbols[:len(precedingSymbols)-1]
		state = &b.parseTable.States[stateID]

		if e := b.lookup(state, symbol); e != nil && len(e.Actions) > 0 {
			lookaheadsWithConflicts.Insert(symbol)
		}
		e := b.entry(state, symbol, newParseTableEntry())
		e.Actions = append(e.Actions, ParseAction{Kind: ActionShift, State: nextStateID})
	}

	for _, symbol := range nonTerminalSuccessors.sorted() {
		precedingSymbols = append(precedingSymbols, symbol)
		nextStateID := b.addParseState(precedingSymbols, precedingAuxiliarySymbols, *nonTerminalSuccessors.sets[symbol])
		precedingSymbols = precedingSymbols[:len(precedingSymbols)-1]
		state = &b.parseTable.States[stateID]
		state.insertNonterminal(symbol, GotoAction{State: nextStateID})
		b.work++
		bTableEntries.checkWork(b.work)
	}

	// For any symbol with multiple actions, perform conflict resolution. This
	// either chooses one action over the others using precedence or
	// associativity, keeps several actions if the grammar declares the
	// conflict, or fails.
	for _, symbol := range lookaheadsWithConflicts.Symbols() {
		if err := b.handleConflict(itemSet, stateID, precedingSymbols, precedingAuxiliarySymbols, symbol, reductionInfos[symbol]); err != nil {
			return err
		}
	}

	// Add actions for the grammar's extra symbols.
	state = &b.parseTable.States[stateID]
	if state.isEndOfNonTerminalExtra() {
		// If this state represents the end of a non-terminal extra rule, then
		// make sure that it has no other successor states. Non-terminal extra
		// rules must have unambiguous endings.
		if len(state.TerminalEntries) > 1 {
			// The generator collects the parent variables in a hash set; the
			// order of the names in the error is hash order there. The port
			// sorts them.
			seen := map[uint32]bool{}
			var names []string
			for k := range itemSet.Entries {
				it := &itemSet.Entries[k].Item
				if !it.isAugmented() && it.StepIndex > 0 && !seen[it.VariableIndex] {
					seen[it.VariableIndex] = true
					names = append(names, sg.Variables[it.VariableIndex].Ident)
				}
			}
			sort.Strings(names)
			return &AmbiguousExtraError{ParentSymbols: names}
		}
	} else {
		// Add actions for the start tokens of each non-terminal extra rule.
		for _, ts := range b.nonTerminalExtraStates {
			b.entry(state, ts.symbol, ParseTableEntry{
				Reusable: true,
				Actions:  []ParseAction{{Kind: ActionShift, State: ts.state}},
			})
		}

		// Add ShiftExtra actions for the terminal extra tokens. These actions
		// are added to every state except those at the ends of non-terminal
		// extras.
		for _, extra := range sg.ExtraSymbols {
			if extra.IsNonTerminal() {
				state.insertNonterminal(extra, GotoAction{ShiftExtra: true})
			} else {
				b.entry(state, extra, ParseTableEntry{
					Reusable: true,
					Actions:  []ParseAction{{Kind: ActionShiftExtra}},
				})
			}
		}
	}

	if sg.HasWord {
		keywordCaptureToken := sg.WordToken
		found := false
		var reservedWordSetID uint32
		for k := range itemSet.Entries {
			e := &itemSet.Entries[k]
			var id uint32
			if nextStep, ok := e.Item.step(sg); ok {
				if nextStep.Symbol != keywordCaptureToken {
					continue
				}
				id = uint32(nextStep.Reserved)
			} else if pool.get(e.lookaheads).Contains(keywordCaptureToken) {
				id = e.FollowingReservedWordSet
			} else {
				continue
			}
			if !found || id > reservedWordSetID {
				reservedWordSetID, found = id, true
			}
		}
		if found {
			if int(reservedWordSetID) >= len(sg.ReservedWordSets) {
				panicLikeGenerator("reserved word set %d of %d", reservedWordSetID, len(sg.ReservedWordSets))
			}
			state.ReservedWords = sg.ReservedWordSets[reservedWordSetID].Clone()
		}
	}

	return nil
}

func isHidden(k lexbuild.VariableType) bool {
	return k == lexbuild.VarHidden || k == lexbuild.VarAuxiliary
}

func isVisible(k lexbuild.VariableType) bool {
	return k == lexbuild.VarNamed || k == lexbuild.VarAnonymous
}

func containsAction(list []ParseAction, a ParseAction) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

// insertSortedSymbol is Vec::binary_search then insert on a sorted list.
func insertSortedSymbol(list *[]Symbol, s Symbol) {
	l := *list
	i := sort.Search(len(l), func(k int) bool { return !lexbuild.SymbolLess(l[k], s) })
	if i < len(l) && l[i] == s {
		return
	}
	l = append(l, Symbol{})
	copy(l[i+1:], l[i:])
	l[i] = s
	*list = l
}

// dedupAux is Vec::dedup: drop consecutive equal entries.
func dedupAux(list []auxiliarySymbolInfo) []auxiliarySymbolInfo {
	if len(list) < 2 {
		return list
	}
	out := list[:1]
	for i := 1; i < len(list); i++ {
		if !list[i].equal(&out[len(out)-1]) {
			out = append(out, list[i])
		}
	}
	return out
}

// symbolName is build_parse_table.rs: fn ParseTableBuilder::symbol_name.
func (b *parseTableBuilder) symbolName(s Symbol) string {
	switch s.Kind {
	case lexbuild.End, lexbuild.EndOfNonTerminalExtra:
		return "EOF"
	case lexbuild.External:
		return b.sg.ExternalTokens[s.Index].Ident
	case lexbuild.NonTerminal:
		if int(s.Index) >= len(b.sg.Variables) {
			panicLikeGenerator("symbol_name of non-terminal %d of %d", s.Index, len(b.sg.Variables))
		}
		return b.sg.Variables[s.Index].Ident
	}
	v := b.lg.Variables[s.Index]
	if v.Kind == lexbuild.VarNamed {
		return v.Ident
	}
	return "'" + v.Ident + "'"
}

// buildParseTable is build_parse_table.rs: fn build_parse_table.
func buildParseTable(sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, itemSetBuilder *ParseItemSetBuilder, variableInfo []VariableInfo) (table *ParseTable, info *ParseStateInfo, diags Diagnostics, err error) {
	b := newParseTableBuilder(sg, lg, itemSetBuilder, variableInfo)
	defer func() {
		if r := recover(); r != nil {
			if p, ok := r.(generatorPanic); ok {
				err = p.err
				return
			}
			panic(r)
		}
	}()
	var gerr error
	if serr := guardSize(func() { table, info, gerr = b.build() }); serr != nil {
		return nil, nil, Diagnostics{}, serr
	}
	if gerr != nil {
		return nil, nil, Diagnostics{}, gerr
	}
	return table, info, b.diagnostics, nil
}
