package grammargen

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// maxUint16Index is the largest value that fits in the uint16 index/ID fields
// grammargen writes into a Language: parse-action group indices (ParseTable /
// SmallParseTable values), field-map and supertype-map offsets, reserved-word
// set IDs, and external-lex-state row indices. Nonterminal GOTO state IDs can
// exceed this and spill into Language.LargeStateGotos. A naive uint16(n)
// conversion of a larger value silently wraps and produces a valid-looking but
// WRONG table — generation "succeeds" and the corruption only surfaces later
// as mysterious downstream parse failures. checkUint16Index
// turns that worst-case silent wrap into a hard generation error instead.
const maxUint16Index = math.MaxUint16 // 65535

// checkUint16Index reports an actionable error when value cannot be stored in
// the uint16 table slot named by field. grammarName identifies the offending
// grammar so a downstream author is not left chasing a corrupted table.
func checkUint16Index(grammarName, field string, value int) error {
	if value > maxUint16Index {
		return fmt.Errorf(
			"grammar %q: %s index %d exceeds the uint16 table limit of %d; "+
				"the grammar is too large to encode with this parser ABI — reduce its "+
				"rule/alias/state count or split it into smaller grammars",
			grammarName, field, value, maxUint16Index)
	}
	return nil
}

// pipeReduceLHSIsRowBoundary reports whether the _pipe_table_line_ending reduce
// action reduces to a pipe-table delimiter-row or body-data-row nonterminal — a
// position where the body-loop separator (_pipe_table_newline →
// _pipe_table_line_ending) can legitimately follow next, so the external token
// must be kept rather than Case-2-suppressed.
//
// The match is by name prefix to stay robust against the auto-numbered
// `_repeat<N>` suffix on generated repeat-aux nonterminals (the counter is
// grammar-global and shifts when other rules are added):
//
//   - `pipe_table_delimiter_*` covers the delimiter row, its cells, and their
//     repeat auxes; after the delimiter row the body loop begins.
//   - `_pipe_table_data_row*` covers the body data row (a structural copy of
//     `pipe_table_row`, see `_pipe_table` / `_pipe_table_data_row` in
//     markdown_grammar.go) and its repeat aux; after a body row the loop may
//     continue with another body row.
//
// The header row uses the bare `pipe_table_row` nonterminal (NOT
// `_pipe_table_data_row`); it deliberately falls through here so Case-2 still
// suppresses `_pipe_table_line_ending` after the header (where only the
// `_pipe_table_header_block`'s trailing `_newline`/`_line_ending` is valid).
// Cell-content nonterminals (`pipe_table_cell*`) also fall through and stay
// suppressed.
func pipeReduceLHSIsRowBoundary(ng *NormalizedGrammar, actionList []lrAction) bool {
	for _, a := range actionList {
		if a.kind != lrReduce {
			continue
		}
		if a.lhsSym < 0 || a.lhsSym >= len(ng.Symbols) {
			continue
		}
		name := ng.Symbols[a.lhsSym].Name
		if strings.HasPrefix(name, "pipe_table_delimiter_") ||
			strings.HasPrefix(name, "_pipe_table_data_row") {
			return true
		}
	}
	return false
}

// assemble populates a gotreesitter.Language from the normalized grammar,
// LR parse tables, and lex DFA states.
func assemble(
	ng *NormalizedGrammar,
	tables *LRTables,
	lexStates []gotreesitter.LexState,
	lexModeMapping []int,
	lexModeOffsets []int,
	afterWSModes []afterWSModeEntry,
) (*gotreesitter.Language, error) {
	tokenCount := ng.TokenCount()
	symbolCount := len(ng.Symbols)

	lang := &gotreesitter.Language{
		Name:                  ng.GrammarName,
		SymbolCount:           uint32(symbolCount),
		TokenCount:            uint32(tokenCount),
		ExternalTokenCount:    uint32(len(ng.ExternalSymbols)),
		StateCount:            uint32(tables.StateCount),
		InitialState:          1,
		LexStates:             lexStates,
		LanguageVersion:       14,
		GeneratedByGrammargen: true,
	}
	if len(lexModeOffsets) > 0 {
		lang.LayoutFallbackLexState = uint16(lexModeOffsets[0])
		lang.HasLayoutFallbackLexState = true
	}

	// Symbol names and metadata.
	lang.SymbolNames = make([]string, symbolCount)
	lang.SymbolMetadata = make([]gotreesitter.SymbolMetadata, symbolCount)
	for i, sym := range ng.Symbols {
		lang.SymbolNames[i] = sym.Name
		lang.SymbolMetadata[i] = gotreesitter.SymbolMetadata{
			Name:               sym.Name,
			Visible:            sym.Visible,
			Named:              sym.Named,
			Supertype:          sym.Supertype,
			GeneratedRepeatAux: sym.GeneratedRepeatAux,
		}
	}
	lang.HiddenChoicePassthroughSymbols = buildHiddenChoicePassthroughSymbols(ng, symbolCount)

	// Field names.
	lang.FieldNames = ng.FieldNames
	lang.FieldCount = uint32(len(ng.FieldNames))

	// Build pre-remap lex modes (will be remapped inside buildParseTables).
	lang.LexModes = make([]gotreesitter.LexMode, tables.StateCount)
	for i := 0; i < tables.StateCount; i++ {
		modeIdx := 0
		if i < len(lexModeMapping) {
			modeIdx = lexModeMapping[i]
		}
		offset := 0
		if modeIdx < len(lexModeOffsets) {
			offset = lexModeOffsets[modeIdx]
		}
		lang.LexModes[i].SetLexStateIndex(uint32(offset))
	}
	for _, entry := range afterWSModes {
		if entry.stateIdx < len(lang.LexModes) && entry.modeIdx < len(lexModeOffsets) {
			lang.LexModes[entry.stateIdx].SetAfterWhitespaceLexStateIndex(uint32(lexModeOffsets[entry.modeIdx]))
		}
	}

	// Compact production IDs: assign ProductionID=0 to all productions without
	// aliases, and assign shared IDs to productions with identical alias patterns.
	// This reduces AliasSequences from O(total_productions) to O(unique_alias_patterns),
	// which is critical for large grammars: Markdown has 50k+ productions but only
	// ~14 unique alias patterns. Without compaction, the parse action group map
	// grows beyond uint16 capacity (65535), corrupting the parse table.
	compactProductionIDs(ng)

	// Build parse actions array, parse table, and small parse table.
	// This remaps state IDs (adding error recovery state 0) and
	// also remaps LexModes to match the new state numbering.
	err := buildParseTables(lang, tables, ng, tokenCount)
	if err != nil {
		return nil, fmt.Errorf("build parse tables: %w", err)
	}
	lang.ConflictPolicies = buildConflictPolicies(tables, ng)

	if err := buildReservedWordTables(lang, ng); err != nil {
		return nil, err
	}

	// Build field map tables.
	if err := buildFieldMaps(lang, ng); err != nil {
		return nil, err
	}
	buildProductionSignatures(lang, ng)

	// Supertype symbols.
	if len(ng.Supertypes) > 0 {
		lang.SupertypeSymbols = make([]gotreesitter.Symbol, len(ng.Supertypes))
		for i, s := range ng.Supertypes {
			lang.SupertypeSymbols[i] = gotreesitter.Symbol(s)
		}
	}

	// External symbols.
	if len(ng.ExternalSymbols) > 0 {
		lang.ExternalSymbols = make([]gotreesitter.Symbol, len(ng.ExternalSymbols))
		for i, s := range ng.ExternalSymbols {
			lang.ExternalSymbols[i] = gotreesitter.Symbol(s)
		}
		// Build ExternalLexStates validity table.
		if err := buildExternalLexStates(lang, tables, ng); err != nil {
			return nil, err
		}
	}

	gotreesitter.RepairNoLookaheadLexModes(lang)

	// Immediate tokens — populate bitmask so the runtime lexer can reject
	// immediate token matches when whitespace was consumed before them.
	{
		hasImm := false
		for _, t := range ng.Terminals {
			if t.Immediate {
				hasImm = true
				break
			}
		}
		if hasImm {
			lang.ImmediateTokens = make([]bool, symbolCount)
			for _, t := range ng.Terminals {
				if t.Immediate && t.SymbolID < symbolCount {
					lang.ImmediateTokens[t.SymbolID] = true
				}
			}
		}
	}
	// Zero-width terminals — populate bitmask so the runtime lexer can reject
	// accidental empty accepts for terminals whose patterns require input.
	// grammargen has this information, so an all-false bitset means "no DFA
	// terminals may match empty"; nil remains reserved for older ts2go blobs.
	if len(ng.Terminals) > 0 {
		lang.ZeroWidthTokens = make([]bool, symbolCount)
		for _, t := range ng.Terminals {
			if t.SymbolID < symbolCount && terminalRuleCanMatchEmpty(t.Rule) {
				lang.ZeroWidthTokens[t.SymbolID] = true
			}
		}
	}

	// Alias sequences.
	buildAliasSequences(lang, ng)

	// Supertype map.
	if err := buildSupertypeMap(lang, ng); err != nil {
		return nil, err
	}

	certifyGeneratedCRecoveryCostCompetition(lang)

	return lang, nil
}

func certifyGeneratedCRecoveryCostCompetition(lang *gotreesitter.Language) {
	gotreesitter.CertifyCRecoveryCostCompetition(lang)
}

func buildHiddenChoicePassthroughSymbols(ng *NormalizedGrammar, symbolCount int) []bool {
	if ng == nil || symbolCount == 0 {
		return nil
	}
	aliasReferenced := make([]bool, symbolCount)
	prodCount := make([]int, symbolCount)
	allNeutralUnary := make([]bool, symbolCount)
	for i := range allNeutralUnary {
		allNeutralUnary[i] = true
	}
	for _, prod := range ng.Productions {
		if prod.LHS >= 0 && prod.LHS < symbolCount {
			prodCount[prod.LHS]++
			if len(prod.RHS) != 1 ||
				prod.Prec != 0 || prod.DynPrec != 0 || prod.Assoc != AssocNone || prod.HasExplicitPrec ||
				len(prod.Fields) != 0 || len(prod.Aliases) != 0 {
				allNeutralUnary[prod.LHS] = false
			}
		}
		for _, alias := range prod.Aliases {
			if alias.ChildIndex < 0 || alias.ChildIndex >= len(prod.RHS) {
				continue
			}
			child := prod.RHS[alias.ChildIndex]
			if child >= 0 && child < symbolCount {
				aliasReferenced[child] = true
			}
		}
	}
	var out []bool
	for i, sym := range ng.Symbols {
		if i >= symbolCount || prodCount[i] <= 1 || !allNeutralUnary[i] || aliasReferenced[i] {
			continue
		}
		if sym.Kind != SymbolNonterminal || sym.Visible || !sym.Named || sym.Supertype || sym.GeneratedRepeatAux {
			continue
		}
		if !strings.HasPrefix(sym.Name, "_") {
			continue
		}
		if out == nil {
			out = make([]bool, symbolCount)
		}
		out[i] = true
	}
	return out
}

func buildReservedWordTables(lang *gotreesitter.Language, ng *NormalizedGrammar) error {
	if lang == nil || ng == nil || ng.WordSymbolID == 0 || len(ng.ReservedWordSets) == 0 {
		return nil
	}

	// grammar.json's first reserved set is the global set. Tree-sitter derives
	// per-state subsets by removing keywords that are explicitly valid in a
	// state; mirror that derivation here for the imported global set.
	base := make([]gotreesitter.Symbol, 0, len(ng.ReservedWordSets[0]))
	for _, symID := range ng.ReservedWordSets[0] {
		if symID > 0 {
			base = append(base, gotreesitter.Symbol(symID))
		}
	}
	if len(base) == 0 {
		return nil
	}

	serializedSets := map[string]uint16{"": 0}
	uniqueSets := [][]gotreesitter.Symbol{{}}
	wordSym := gotreesitter.Symbol(ng.WordSymbolID)

	for state := 1; state < len(lang.LexModes); state++ {
		if !stateNeedsReservedWords(lang, gotreesitter.StateID(state), wordSym, ng.KeywordSymbols) {
			continue
		}

		reserved := make([]gotreesitter.Symbol, 0, len(base))
		for _, sym := range base {
			if lookupActionIndexForLanguage(lang, gotreesitter.StateID(state), sym) == 0 {
				reserved = append(reserved, sym)
			}
		}
		if len(reserved) == 0 {
			continue
		}

		key := serializeReservedWordSet(reserved)
		setID, ok := serializedSets[key]
		if !ok {
			if err := checkUint16Index(ng.GrammarName, "reserved word set", len(uniqueSets)); err != nil {
				return err
			}
			setID = uint16(len(uniqueSets))
			serializedSets[key] = setID
			uniqueSets = append(uniqueSets, reserved)
		}
		lang.LexModes[state].ReservedWordSetID = setID
	}

	if len(uniqueSets) <= 1 {
		return nil
	}

	maxSetSize := 0
	for _, set := range uniqueSets {
		if len(set) > maxSetSize {
			maxSetSize = len(set)
		}
	}
	if maxSetSize == 0 {
		return nil
	}

	lang.ReservedWords = make([]gotreesitter.Symbol, len(uniqueSets)*maxSetSize)
	lang.MaxReservedWordSetSize = uint16(maxSetSize)
	for i, set := range uniqueSets {
		offset := i * maxSetSize
		copy(lang.ReservedWords[offset:offset+len(set)], set)
	}
	if lang.LanguageVersion < 15 {
		lang.LanguageVersion = 15
	}
	return nil
}

func stateNeedsReservedWords(lang *gotreesitter.Language, state gotreesitter.StateID, wordSym gotreesitter.Symbol, keywordSymbols []int) bool {
	if lookupActionIndexForLanguage(lang, state, wordSym) != 0 {
		return true
	}
	for _, symID := range keywordSymbols {
		if lookupActionIndexForLanguage(lang, state, gotreesitter.Symbol(symID)) != 0 {
			return true
		}
	}
	return false
}

func lookupActionIndexForLanguage(lang *gotreesitter.Language, state gotreesitter.StateID, sym gotreesitter.Symbol) uint16 {
	if lang == nil {
		return 0
	}
	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	if int(state) < denseLimit {
		if int(state) >= len(lang.ParseTable) {
			return 0
		}
		row := lang.ParseTable[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}
	smallIdx := int(state) - int(lang.LargeStateCount)
	if smallIdx < 0 || smallIdx >= len(lang.SmallParseTableMap) {
		return 0
	}
	table := lang.SmallParseTable
	offset := lang.SmallParseTableMap[smallIdx]
	if int(offset) >= len(table) {
		return 0
	}
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if gotreesitter.Symbol(table[pos]) == sym {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

func serializeReservedWordSet(set []gotreesitter.Symbol) string {
	buf := make([]byte, 0, len(set)*2)
	for _, sym := range set {
		buf = append(buf, byte(sym>>8), byte(sym))
	}
	return string(buf)
}

// buildParseTables constructs ParseActions, ParseTable (dense),
// SmallParseTable, and SmallParseTableMap from the LR tables.
func buildParseTables(
	lang *gotreesitter.Language,
	tables *LRTables,
	ng *NormalizedGrammar,
	tokenCount int,
) error {
	symbolCount := len(ng.Symbols)

	// Build parse action entries.
	// Index 0 is always the error/no-action entry.
	var parseActions []gotreesitter.ParseActionEntry
	parseActions = append(parseActions, gotreesitter.ParseActionEntry{}) // index 0 = error

	actionGroupMap := make(map[string]int) // serialized action → index

	// serializeActions produces a canonical key based on the *semantic content*
	// of the resulting ParseAction entries, not the raw lrAction fields.
	// This ensures that two different prodIdx values that reduce to the same
	// (LHS, childCount, dynPrec, productionID, isExtra) produce the same key
	// and therefore share a single ParseActionEntry. Without this deduplication
	// the action group count exceeds uint16 capacity for large grammars like
	// Markdown (50k+ productions → 78k+ raw groups vs the limit of 65535).
	serializeActions := func(acts []lrAction) string {
		buf := make([]byte, 0, len(acts)*9)
		for _, a := range acts {
			switch a.kind {
			case lrShift:
				// SHIFT: key = kind(1) + isExtra(1) + repeat(1) + state(2) = 5 bytes
				buf = append(buf, byte(a.kind))
				if a.isExtra {
					buf = append(buf, 1)
				} else {
					buf = append(buf, 0)
				}
				if a.repeat {
					buf = append(buf, 1)
				} else {
					buf = append(buf, 0)
				}
				buf = append(buf, byte(a.state>>8), byte(a.state))
			case lrReduce:
				// REDUCE: key = kind(1) + isExtra(1) + repeat(1) + lhs(2) + childCount(1) + dynPrec(2) + prodID(2) + extra(1) = 11 bytes
				// Use semantic content (prod fields) rather than raw prodIdx.
				prod := &ng.Productions[a.prodIdx]
				childCount := len(prod.RHS)
				buf = append(buf, byte(a.kind))
				if a.isExtra {
					buf = append(buf, 1)
				} else {
					buf = append(buf, 0)
				}
				if a.repeat {
					buf = append(buf, 1)
				} else {
					buf = append(buf, 0)
				}
				buf = append(buf, byte(prod.LHS>>8), byte(prod.LHS))
				buf = append(buf, byte(childCount))
				buf = append(buf, byte(prod.DynPrec>>8), byte(prod.DynPrec))
				buf = append(buf, byte(prod.ProductionID>>8), byte(prod.ProductionID))
				if prod.IsExtra {
					buf = append(buf, 1)
				} else {
					buf = append(buf, 0)
				}
			default:
				// ACCEPT and others: kind only
				buf = append(buf, byte(a.kind))
			}
		}
		return string(buf)
	}

	// indexErr captures the first uint16 overflow encountered while assigning
	// parse-action group indices. The closure cannot return an error, so it
	// records one here; buildParseTables checks it after the table is built.
	var indexErr error
	getOrAddActionGroup := func(acts []lrAction) uint16 {
		if len(acts) == 0 {
			return 0
		}
		key := serializeActions(acts)
		if idx, ok := actionGroupMap[key]; ok {
			return uint16(idx)
		}
		idx := len(parseActions)
		if err := checkUint16Index(ng.GrammarName, "parse action group", idx); err != nil {
			if indexErr == nil {
				indexErr = err
			}
			return 0
		}
		actionGroupMap[key] = idx

		entry := gotreesitter.ParseActionEntry{}
		for _, a := range acts {
			pa := gotreesitter.ParseAction{}
			switch a.kind {
			case lrShift:
				pa.Type = gotreesitter.ParseActionShift
				pa.State = gotreesitter.StateID(a.state)
				pa.Repetition = a.repeat
				if a.isExtra {
					if a.state == 0 {
						pa.Extra = true
					} else {
						pa.ExtraChain = true
					}
				}
			case lrReduce:
				prod := &ng.Productions[a.prodIdx]
				pa.Type = gotreesitter.ParseActionReduce
				pa.Symbol = gotreesitter.Symbol(prod.LHS)
				pa.ChildCount = uint8(len(prod.RHS))
				pa.DynamicPrecedence = int16(prod.DynPrec)
				pa.ProductionID = uint16(prod.ProductionID)
				pa.Extra = prod.IsExtra
			case lrAccept:
				pa.Type = gotreesitter.ParseActionAccept
			}
			entry.Actions = append(entry.Actions, pa)
		}
		parseActions = append(parseActions, entry)
		return uint16(idx)
	}

	// Add extra shift actions for extra symbols.
	// Extra symbols can be shifted in any state.
	var extraShiftIdx uint16
	if len(ng.ExtraSymbols) > 0 {
		extraEntry := gotreesitter.ParseActionEntry{
			Actions: []gotreesitter.ParseAction{{
				Type:  gotreesitter.ParseActionShift,
				Extra: true,
			}},
		}
		if err := checkUint16Index(ng.GrammarName, "extra shift action", len(parseActions)); err != nil {
			return err
		}
		extraShiftIdx = uint16(len(parseActions))
		parseActions = append(parseActions, extraEntry)
	}

	// Build the raw action table: [state][symbol] -> action index or GOTO state.
	// Keep this intermediate wide: terminal action indices must still fit in
	// uint16, but large generated grammars can have nonterminal GOTO targets
	// above 65535. Those spill into Language.LargeStateGotos when the final
	// runtime tables are emitted.
	rawTable := make([][]uint32, tables.StateCount)
	for state := 0; state < tables.StateCount; state++ {
		row := make([]uint32, symbolCount)
		rawTable[state] = row

		// Terminal actions.
		if acts, ok := tables.ActionTable[state]; ok {
			syms := make([]int, 0, len(acts))
			for sym := range acts {
				if sym < tokenCount {
					syms = append(syms, sym)
				}
			}
			sort.Ints(syms)
			for _, sym := range syms {
				row[sym] = uint32(getOrAddActionGroup(acts[sym]))
			}
		}

		// Extra symbols: shiftable in every state (terminal extras only).
		// Nonterminal extras are handled via LR reduce with Extra=true.
		for _, extraSym := range ng.ExtraSymbols {
			if extraSym >= tokenCount {
				continue // nonterminal extra — handled by LR items/reduce
			}
			if row[extraSym] == 0 {
				row[extraSym] = uint32(extraShiftIdx)
			}
		}

		// Nonterminal gotos: encode directly as state ID (ts2go convention).
		if gotos, ok := tables.GotoTable[state]; ok {
			syms := make([]int, 0, len(gotos))
			for sym := range gotos {
				if sym >= tokenCount && sym < symbolCount {
					syms = append(syms, sym)
				}
			}
			sort.Ints(syms)
			for _, sym := range syms {
				row[sym] = uint32(gotos[sym])
			}
		}
	}

	// A parse-action group index overflowed uint16 while filling the table
	// above (the closure could not return the error itself). Fail hard now
	// rather than shipping a silently-wrapped, corrupted table.
	if indexErr != nil {
		return indexErr
	}

	// Determine which states should be dense vs sparse.
	// Heuristic: states with many non-zero entries go dense.
	type stateInfo struct {
		idx     int
		nonZero int
	}
	var infos []stateInfo
	for i, row := range rawTable {
		nz := 0
		for _, v := range row {
			if v != 0 {
				nz++
			}
		}
		infos = append(infos, stateInfo{i, nz})
	}

	// Sort states by non-zero count descending. Dense states first.
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].nonZero > infos[j].nonZero
	})

	// Choose a cutoff: states with >= threshold non-zero entries go dense.
	// tree-sitter typically makes states with many entries dense.
	threshold := 1
	if len(infos) > 0 {
		// Use median as a rough heuristic.
		median := infos[len(infos)/2].nonZero
		threshold = median + 1
		if threshold < 2 {
			threshold = 2
		}
	}

	// We need a remapping from old state IDs to new state IDs.
	// State 0 must remain state 0 (error recovery state).
	// State with initial items should be state 1 (InitialState).
	// For simplicity, keep original ordering — dense states first.
	// The initial state (which contains the augmented start item) should be
	// early. In our construction, state 0 IS the initial state.
	// tree-sitter reserves state 0 for error recovery and uses state 1 as initial.

	// Remap: state 0 in our LR construction = initial state = should be state 1.
	// We need to insert an empty state 0 for error recovery.
	newStateCount := tables.StateCount + 1 // +1 for error recovery state 0
	stateRemap := make([]int, tables.StateCount)
	for i := range stateRemap {
		stateRemap[i] = i + 1 // shift everything up by 1
	}

	// Rebuild rawTable with remapped states.
	newRawTable := make([][]uint32, newStateCount)
	newRawTable[0] = make([]uint32, symbolCount) // state 0 = error recovery (empty)
	for oldState, newState := range stateRemap {
		row := make([]uint32, symbolCount)
		for sym, val := range rawTable[oldState] {
			if val == 0 {
				continue
			}
			// Remap shift/goto target states in action entries.
			// For terminals: the action index points to ParseActions, which
			// contain State fields that need remapping.
			// For nonterminals: the value IS a state ID that needs remapping.
			if sym >= tokenCount {
				// GOTO: value is a state ID.
				row[sym] = uint32(stateRemap[int(val)])
			} else {
				row[sym] = val
			}
		}
		newRawTable[newState] = row
	}

	// Remap state IDs in ParseActions.
	for i := range parseActions {
		for j := range parseActions[i].Actions {
			a := &parseActions[i].Actions[j]
			if a.Type == gotreesitter.ParseActionShift && (!a.Extra || a.State != 0) {
				if int(a.State) < len(stateRemap) {
					a.State = gotreesitter.StateID(stateRemap[int(a.State)])
				}
			}
		}
	}

	// Determine large (dense) vs small (sparse) states.
	largeStateCount := 0
	for state := 0; state < newStateCount; state++ {
		nz := 0
		for _, v := range newRawTable[state] {
			if v != 0 {
				nz++
			}
		}
		if nz >= threshold {
			largeStateCount++
		} else {
			break // dense states must be contiguous from 0
		}
	}
	// Ensure at least state 0 and 1 are dense.
	if largeStateCount < 2 {
		largeStateCount = 2
	}
	if largeStateCount > newStateCount {
		largeStateCount = newStateCount
	}

	recordLargeGoto := func(state int, sym int, target uint32) {
		if lang.LargeStateGotos == nil {
			lang.LargeStateGotos = make(map[uint64]gotreesitter.StateID)
		}
		key := uint64(gotreesitter.StateID(state))<<32 | uint64(gotreesitter.Symbol(sym))
		lang.LargeStateGotos[key] = gotreesitter.StateID(target)
	}

	downcastCell := func(state int, sym int, val uint32) (uint16, error) {
		if val == 0 {
			return 0, nil
		}
		if sym >= tokenCount && val > maxUint16Index {
			recordLargeGoto(state, sym, val)
			return 0, nil
		}
		if val > maxUint16Index {
			return 0, checkUint16Index(ng.GrammarName, "parse table action/goto", int(val))
		}
		return uint16(val), nil
	}

	// Build dense parse table (first largeStateCount states).
	lang.ParseTable = make([][]uint16, largeStateCount)
	for i := 0; i < largeStateCount; i++ {
		row := make([]uint16, symbolCount)
		for sym, val := range newRawTable[i] {
			downcast, err := downcastCell(i, sym, val)
			if err != nil {
				return err
			}
			row[sym] = downcast
		}
		lang.ParseTable[i] = row
	}

	// Build sparse parse table for remaining states.
	if newStateCount > largeStateCount {
		var smallTable []uint16
		var smallMap []uint32

		for state := largeStateCount; state < newStateCount; state++ {
			smallMap = append(smallMap, uint32(len(smallTable)))

			// Group non-zero entries by value.
			groups := make(map[uint16][]uint16)
			for sym, val := range newRawTable[state] {
				if val != 0 {
					downcast, err := downcastCell(state, sym, val)
					if err != nil {
						return err
					}
					if downcast == 0 {
						continue
					}
					groups[downcast] = append(groups[downcast], uint16(sym))
				}
			}

			// Write group count.
			smallTable = append(smallTable, uint16(len(groups)))

			// Sort groups for determinism.
			vals := make([]uint16, 0, len(groups))
			for v := range groups {
				vals = append(vals, v)
			}
			sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })

			for _, val := range vals {
				syms := groups[val]
				sort.Slice(syms, func(i, j int) bool { return syms[i] < syms[j] })
				smallTable = append(smallTable, val, uint16(len(syms)))
				smallTable = append(smallTable, syms...)
			}
		}

		lang.SmallParseTable = smallTable
		lang.SmallParseTableMap = smallMap
	}

	lang.ParseActions = parseActions
	lang.StateCount = uint32(newStateCount)
	lang.LargeStateCount = uint32(largeStateCount)

	// Rebuild LexModes for the remapped state count.
	newLexModes := make([]gotreesitter.LexMode, newStateCount)
	// State 0 (error recovery) gets mode 0.
	if len(lang.LexModes) > 0 {
		newLexModes[0] = lang.LexModes[0]
		for oldState, newState := range stateRemap {
			if oldState < len(lang.LexModes) {
				newLexModes[newState] = lang.LexModes[oldState]
			}
		}
	}
	lang.LexModes = newLexModes

	// Count unique production IDs for ProductionIDCount.
	maxProdID := 0
	for _, prod := range ng.Productions {
		if prod.ProductionID > maxProdID {
			maxProdID = prod.ProductionID
		}
	}
	lang.ProductionIDCount = uint32(maxProdID + 1)

	return nil
}

// buildProductionSignatures records compact RHS shape metadata for generated grammars.
func buildProductionSignatures(lang *gotreesitter.Language, ng *NormalizedGrammar) {
	if lang == nil || ng == nil || len(ng.Productions) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(ng.Productions))
	out := make([]gotreesitter.ProductionSignature, 0, len(ng.Productions))
	for _, prod := range ng.Productions {
		key := productionSignatureKey(prod)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rhs := make([]gotreesitter.Symbol, len(prod.RHS))
		for i, sym := range prod.RHS {
			rhs[i] = gotreesitter.Symbol(sym)
		}
		out = append(out, gotreesitter.ProductionSignature{
			LHS:          gotreesitter.Symbol(prod.LHS),
			ProductionID: uint16(prod.ProductionID),
			RHS:          rhs,
		})
	}
	if len(out) > 0 {
		lang.ProductionSignatures = out
	}
}

func productionSignatureKey(prod Production) string {
	buf := make([]byte, 0, 4+len(prod.RHS)*2)
	buf = append(buf, byte(prod.LHS>>8), byte(prod.LHS))
	buf = append(buf, byte(prod.ProductionID>>8), byte(prod.ProductionID))
	for _, sym := range prod.RHS {
		buf = append(buf, byte(sym>>8), byte(sym))
	}
	return string(buf)
}

// buildFieldMaps constructs FieldMapSlices and FieldMapEntries.
func buildFieldMaps(lang *gotreesitter.Language, ng *NormalizedGrammar) error {
	if len(ng.FieldNames) <= 1 {
		return nil // no fields
	}

	maxProdID := 0
	for _, prod := range ng.Productions {
		if prod.ProductionID > maxProdID {
			maxProdID = prod.ProductionID
		}
	}

	lang.FieldMapSlices = make([][2]uint16, maxProdID+1)
	var entries []gotreesitter.FieldMapEntry

	for _, prod := range ng.Productions {
		if len(prod.Fields) == 0 {
			continue
		}
		start := len(entries)
		for _, fa := range prod.Fields {
			fid, ok := ng.fieldID(fa.FieldName)
			if !ok {
				continue
			}
			entries = append(entries, gotreesitter.FieldMapEntry{
				FieldID:    gotreesitter.FieldID(fid),
				ChildIndex: uint8(fa.ChildIndex),
			})
		}
		count := len(entries) - start
		if count > 0 {
			if err := checkUint16Index(ng.GrammarName, "field map entry", start); err != nil {
				return err
			}
			if err := checkUint16Index(ng.GrammarName, "field map count", count); err != nil {
				return err
			}
			lang.FieldMapSlices[prod.ProductionID] = [2]uint16{uint16(start), uint16(count)}
		}
	}

	lang.FieldMapEntries = entries
	return nil
}

// buildExternalLexStates builds the ExternalLexStates validity table and sets
// ExternalLexState on each LexMode entry. Each unique set of valid external
// tokens gets its own row. Row 0 is always all-false.
func buildExternalLexStates(lang *gotreesitter.Language, tables *LRTables, ng *NormalizedGrammar) error {
	extCount := len(ng.ExternalSymbols)
	if extCount == 0 {
		return nil
	}

	// Build external symbol set for quick lookup.
	extSymSet := make(map[int]int, extCount) // symbol ID → external token index
	for i, symID := range ng.ExternalSymbols {
		extSymSet[symID] = i
	}

	// Build set of external symbols that are also extras. These terminal
	// extras (e.g. HTML's comment token from the external scanner) are
	// valid in every parser state. The LR action table doesn't contain
	// entries for them because their shift-extra actions are added later
	// in the assembly step, so we must mark them explicitly here.
	extraExtSet := make(map[int]bool, len(ng.ExtraSymbols))
	for _, extraSym := range ng.ExtraSymbols {
		if extIdx, isExt := extSymSet[extraSym]; isExt {
			extraExtSet[extIdx] = true
		}
	}
	tokenCount := ng.TokenCount()

	// Build counterpart map: external symbol ID -> non-external terminals
	// with the same surface token name. Used to detect LALR merging artifacts
	// where expression contexts (needing external scanner) and type contexts
	// (needing regular terminal) get conflated into the same LR state.
	extCp := make(map[int][]int) // external symID -> counterpart symIDs
	for extSym := range extSymSet {
		extName := ng.Symbols[extSym].Name
		if extName == "" {
			continue
		}
		for sym := 1; sym < tokenCount; sym++ {
			if _, isExt := extSymSet[sym]; isExt {
				continue
			}
			tn := ng.Symbols[sym].Name
			if tn == extName || tn == "\\"+extName {
				extCp[extSym] = append(extCp[extSym], sym)
			}
		}
	}

	// Find _pipe_table_line_ending, _line_ending, and content-token IDs.
	// LALR merging can put _pipe_table_line_ending in states where only
	// _line_ending should be valid (e.g., after the header row). We suppress
	// _pipe_table_line_ending from any state where:
	// 1. _line_ending has the SAME reduce-only action (degenerate merge), OR
	// 2. _pipe_table_line_ending has a SHIFT action alongside content tokens
	//    (_word/_whitespace) that also SHIFT (cell-content merge artifact).
	pipeTableLineEndingSymID := -1
	lineEndingSymID := -1
	pipeTableContentSymIDs := make([]int, 0, 4) // _word, _whitespace
	// Markdown-specific: track `_close_block` and `block_continuation`. The
	// bundled markdown scanner emits `_close_block` eagerly whenever it is in
	// valid_symbols (markdown_scanner.go, near `mdScan`:
	// `if isValid(mdTokCloseBlock) { lexer.SetResultSymbol(mdSymCloseBlock) }`).
	// In the fenced_code_block-body state, LALR puts `_close_block` into the
	// `_newline`-reduce lookahead, so the scanner fires it prematurely — the
	// body becomes a `paragraph` and the closing ``` a fresh fence wrapper. We
	// suppress `_close_block` in exactly that state; the discriminator is a
	// `block_continuation` SHIFT (only the fence/container-body state can
	// consume a continuation marker). See the suppression site below.
	closeBlockSymID := -1
	blockContinuationSymID := -1
	for sym := 0; sym < tokenCount; sym++ {
		if _, isExt := extSymSet[sym]; isExt {
			switch ng.Symbols[sym].Name {
			case "_pipe_table_line_ending":
				pipeTableLineEndingSymID = sym
			case "_line_ending":
				lineEndingSymID = sym
			case "_close_block":
				closeBlockSymID = sym
			case "block_continuation":
				blockContinuationSymID = sym
			}
			continue
		}
		switch ng.Symbols[sym].Name {
		case "_word", "_whitespace":
			pipeTableContentSymIDs = append(pipeTableContentSymIDs, sym)
		}
	}

	// Row 0: all-false (no external tokens valid).
	rows := [][]bool{make([]bool, extCount)}
	rowMap := make(map[string]int) // serialized row → row index
	rowMap[serializeBoolRow(rows[0])] = 0
	followTokens := buildFollowTokensFunc(tables, tokenCount)

	// For each parser state (after remapping), compute which external tokens
	// are valid based on the action table.
	stateCount := int(lang.StateCount)
	for state := 0; state < stateCount; state++ {
		row := make([]bool, extCount)
		anyValid := false

		// External extras are valid in every state.
		for extIdx := range extraExtSet {
			row[extIdx] = true
			anyValid = true
		}

		// Check which external symbols have actions in this state.
		// State 0 is the error recovery state (added by buildParseTables).
		// States 1..N map to LR states 0..N-1.
		lrState := state - 1
		if lrState >= 0 {
			if len(ng.ExternalReduceFollowLookaheads) > 0 && followTokens != nil {
				for _, symID := range followTokens(lrState) {
					extIdx, isExt := extSymSet[symID]
					if !isExt || symID < 0 || symID >= len(ng.Symbols) {
						continue
					}
					if !ng.ExternalReduceFollowLookaheads[ng.Symbols[symID].Name] {
						continue
					}
					row[extIdx] = true
					anyValid = true
				}
			}
			if acts, ok := tables.ActionTable[lrState]; ok {
				for symID, extIdx := range extSymSet {
					actionList, ok := acts[symID]
					if !ok || len(actionList) == 0 {
						continue
					}
					// Suppress hidden external symbols when a non-external
					// counterpart has the exact same action list. This helps
					// with merge artifacts like automatic semicolons, but
					// visible aliased externals (for example TypeScript's
					// ternary "?") still need the external scanner path even
					// when the immediate parser action matches a plain token.
					// Otherwise the lexer can commit to the plain token too
					// early and lose the follow-up reduction chain.
					suppressed := false
					if !ng.Symbols[symID].Visible {
						if cpSyms, hasCp := extCp[symID]; hasCp {
							for _, cpSym := range cpSyms {
								cpActs, cpOk := acts[cpSym]
								if cpOk && len(cpActs) > 0 && actListsEqual(actionList, cpActs) {
									suppressed = true
									break
								}
							}
						}
					}
					if !suppressed && shouldSuppressEquivalentExternalReduceLookahead(ng, symID, actionList, acts, extSymSet) {
						if actionsAreReduceOnly(actionList) &&
							hasEquivalentNonExternalReduceAction(acts, actionList, extSymSet, tokenCount) {
							suppressed = true
						}
					}
					// Suppress _pipe_table_line_ending in two LALR merge artifact cases:
					// 1. SHIFT-alongside-content: _pipe_table_line_ending appears as a SHIFT
					//    in the same state where content tokens (_word, _whitespace) also SHIFT.
					//    This means LALR merged cell-content states, and _pipe_table_line_ending
					//    must not be offered (it would be consumed as cell content).
					// 2. Same-action-as-_line_ending: _pipe_table_line_ending has the exact
					//    same action as _line_ending. LALR merged the _newline context (uses
					//    _line_ending) with a _pipe_table_newline context. In _newline context
					//    only _line_ending is correct; offering _pipe_table_line_ending here
					//    causes the scanner to consume the row-end newline as a table token,
					//    leaving the parser unable to recognise the continuation rows.
					if !suppressed && symID == pipeTableLineEndingSymID {
						if len(pipeTableContentSymIDs) > 0 && !actionsAreReduceOnly(actionList) {
							// Case 1: SHIFT artifact in cell-content state.
							for _, cSym := range pipeTableContentSymIDs {
								if cActs, ok := acts[cSym]; ok && len(cActs) > 0 && !actionsAreReduceOnly(cActs) {
									suppressed = true
									break
								}
							}
						}
						if !suppressed && lineEndingSymID >= 0 && actionsAreReduceOnly(actionList) {
							// Case 2: Same reduce action as _line_ending — suppress the
							// more specific token so the parser uses _line_ending instead.
							// EXCEPTION: keep _pipe_table_line_ending when the reduce is a
							// delimiter-row or body-data-row boundary, where the body-loop
							// separator (_pipe_table_newline) can legitimately follow next.
							// See pipeReduceLHSIsRowBoundary.
							if leActs, ok := acts[lineEndingSymID]; ok && len(leActs) > 0 &&
								actListsEqual(actionList, leActs) &&
								!pipeReduceLHSIsRowBoundary(ng, actionList) {
								suppressed = true
							}
						}
					}
					// Suppress `_close_block` in the fenced_code_block-body state.
					// After the `_link_reference_definition_newline` grammar split,
					// link-reference-definition boundaries no longer reduce the
					// shared `_newline` with `_close_block` in lookahead, so the
					// only reduce-only `_newline` state that also offers a
					// `block_continuation` SHIFT is the fence/container body. The
					// bundled scanner's eager `_close_block` (see the collection
					// comment above) would otherwise fire there and truncate the
					// fence body into a paragraph. A `block_continuation` SHIFT is
					// the discriminator: genuine block boundaries (e.g. between
					// consecutive link reference definitions) reduce on
					// `block_continuation` instead and must keep `_close_block`.
					if !suppressed && symID == closeBlockSymID &&
						blockContinuationSymID >= 0 && actionsAreReduceOnly(actionList) {
						// A reduce-only `_close_block` state that reduces
						// `_blank_line_newline` AND offers a `block_continuation`
						// SHIFT is the fenced_code_block / container body: the
						// parser can consume a continuation marker to keep reading
						// the body, and the bundled scanner's eager `_close_block`
						// would otherwise truncate it. Genuine block boundaries
						// keep `_close_block`. The `_blank_line_newline` reduce is
						// the load-bearing discriminator: the html_block type 6/7
						// termination also reduces a blank line with a
						// `block_continuation` shift, but via the de-merged
						// `_html_block_blank_line_newline` (see markdown_grammar.go)
						// — it MUST keep `_close_block` so the html block closes at
						// its blank line, so we require the `_blank_line_newline`
						// LHS specifically rather than any blank-line reduce.
						reducesBlankLineNewline := false
						for _, a := range actionList {
							if a.lhsSym >= 0 && a.lhsSym < len(ng.Symbols) &&
								ng.Symbols[a.lhsSym].Name == "_blank_line_newline" {
								reducesBlankLineNewline = true
								break
							}
						}
						if reducesBlankLineNewline {
							if bcActs, ok := acts[blockContinuationSymID]; ok {
								for _, a := range bcActs {
									if a.kind == lrShift {
										suppressed = true
										break
									}
								}
							}
						}
					}
					if !suppressed {
						row[extIdx] = true
						anyValid = true
					}
				}
			}
		}

		if !anyValid {
			// Map to row 0 (all-false).
			if state < len(lang.LexModes) {
				lang.LexModes[state].ExternalLexState = 0
			}
			continue
		}

		key := serializeBoolRow(row)
		rowIdx, exists := rowMap[key]
		if !exists {
			if err := checkUint16Index(ng.GrammarName, "external lex state row", len(rows)); err != nil {
				return err
			}
			rowIdx = len(rows)
			rowMap[key] = rowIdx
			rows = append(rows, row)
		}

		if state < len(lang.LexModes) {
			lang.LexModes[state].ExternalLexState = uint16(rowIdx)
		}
	}

	lang.ExternalLexStates = rows
	return nil
}

func serializeBoolRow(row []bool) string {
	buf := make([]byte, len(row))
	for i, v := range row {
		if v {
			buf[i] = 1
		}
	}
	return string(buf)
}

// actionsAreReduceOnly returns true if all actions in the list are reduce
// actions (no shifts, no accepts).
func actionsAreReduceOnly(acts []lrAction) bool {
	if len(acts) == 0 {
		return false
	}
	for _, a := range acts {
		if a.kind != lrReduce {
			return false
		}
	}
	return true
}

func hasEquivalentNonExternalReduceAction(
	acts map[int][]lrAction,
	actionList []lrAction,
	extSymSet map[int]int,
	tokenCount int,
) bool {
	for sym := 1; sym < tokenCount; sym++ {
		if _, isExt := extSymSet[sym]; isExt {
			continue
		}
		cpActs, ok := acts[sym]
		if !ok || len(cpActs) == 0 || !actionsAreReduceOnly(cpActs) {
			continue
		}
		if actListsEqual(actionList, cpActs) {
			return true
		}
	}
	return false
}

func shouldSuppressEquivalentExternalReduceLookahead(
	ng *NormalizedGrammar,
	symID int,
	actionList []lrAction,
	acts map[int][]lrAction,
	extSymSet map[int]int,
) bool {
	if ng == nil || !ng.SuppressEquivalentExternalReduceLookaheads ||
		symID < 0 || symID >= len(ng.Symbols) || !ng.Symbols[symID].Visible {
		return false
	}
	switch ng.Symbols[symID].Name {
	case "extglob_pattern", "regex", "variable_name":
		return true
	case "]":
		// Bash's scanner uses this delimiter external to decide when a
		// zero-width _concat is not valid. Dropping it turns a close bracket
		// into another word-like concatenation segment.
		return false
	case "}":
		// The analogous "}" token has an extra whitespace-sensitive concat rule
		// in Bash's scanner. Keep it for normal brace contexts and for
		// simple_expansion reductions that rely on it to avoid string-content
		// bleed, but suppress duplicate string-reduction exposure when the same
		// state also exposes "]". Number reductions need the same treatment:
		// exposing "}" after a numeric command argument makes Bash's scanner
		// emit whitespace _concat before the next flag.
		return hasExternalActionNamed(ng, acts, extSymSet, "]") &&
			(reduceActionListHasLHSName(ng, actionList, "string") ||
				reduceActionListHasLHSName(ng, actionList, "number"))
	default:
		return false
	}
}

func reduceActionListHasLHSName(ng *NormalizedGrammar, actions []lrAction, name string) bool {
	if ng == nil || name == "" {
		return false
	}
	for _, action := range actions {
		if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
			continue
		}
		lhs := ng.Productions[action.prodIdx].LHS
		if lhs >= 0 && lhs < len(ng.Symbols) && ng.Symbols[lhs].Name == name {
			return true
		}
	}
	return false
}

func hasExternalActionNamed(ng *NormalizedGrammar, acts map[int][]lrAction, extSymSet map[int]int, name string) bool {
	if ng == nil || len(acts) == 0 || len(extSymSet) == 0 {
		return false
	}
	for symID := range extSymSet {
		if symID < 0 || symID >= len(ng.Symbols) || ng.Symbols[symID].Name != name {
			continue
		}
		return len(acts[symID]) > 0
	}
	return false
}

// actListsEqual checks if two LR action lists are structurally identical.
func actListsEqual(a, b []lrAction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].state != b[i].state || a[i].prodIdx != b[i].prodIdx {
			return false
		}
	}
	return true
}

// findProductionAlternativeCounterparts finds non-external terminal symbols
// that appear as positional alternatives to extSym in grammar productions.
// Two terminals are considered positional alternatives when they appear at the
// same position in productions that share the same LHS and are otherwise
// identical (same prefix/suffix). For example, if the grammar has:
//
//	expression_statement → [expression, _automatic_semicolon]
//	expression_statement → [expression, ;]
//
// then ";" is a positional alternative to "_automatic_semicolon" at position 1
// of the expression_statement production. This detects inline CHOICE patterns
// like _semicolon: $ => choice($._automatic_semicolon, ";") that were
// flattened during normalization.
func findProductionAlternativeCounterparts(ng *NormalizedGrammar, extSym int, extSymSet map[int]int, tokenCount int) []int {
	// Build a map of (LHS, position, prefix-suffix hash) → productions
	// containing extSym at that position. Then find productions with the
	// same key but a different terminal at that position.
	type prodKey struct {
		lhs int
		pos int
		sig string // serialized RHS with the position blanked out
	}

	// Find all positions where extSym appears in productions.
	extPositions := make(map[prodKey]bool)
	for _, prod := range ng.Productions {
		for i, sym := range prod.RHS {
			if sym == extSym {
				// Build signature: LHS + RHS with position i blanked.
				sig := prodSignature(prod.RHS, i)
				key := prodKey{lhs: prod.LHS, pos: i, sig: sig}
				extPositions[key] = true
			}
		}
	}
	if len(extPositions) == 0 {
		return nil
	}

	// Find non-external terminals at the same position in matching
	// productions.
	seen := make(map[int]bool)
	var result []int
	for _, prod := range ng.Productions {
		for i, sym := range prod.RHS {
			if sym == extSym || sym >= tokenCount {
				continue
			}
			if _, isExt := extSymSet[sym]; isExt {
				continue
			}
			sig := prodSignature(prod.RHS, i)
			key := prodKey{lhs: prod.LHS, pos: i, sig: sig}
			if extPositions[key] && !seen[sym] {
				seen[sym] = true
				result = append(result, sym)
			}
		}
	}
	return result
}

// prodSignature returns a string identifying the RHS shape with position idx
// blanked out to -1, allowing matching of productions that differ only at
// one symbol position.
func prodSignature(rhs []int, blankIdx int) string {
	buf := make([]byte, 0, len(rhs)*4)
	for i, sym := range rhs {
		if i == blankIdx {
			buf = append(buf, 0xFF, 0xFF, 0xFF, 0xFF)
		} else {
			buf = append(buf, byte(sym>>24), byte(sym>>16), byte(sym>>8), byte(sym))
		}
	}
	return string(buf)
}

// fieldID looks up a field name in the normalized grammar.
func (ng *NormalizedGrammar) fieldID(name string) (int, bool) {
	for i, fn := range ng.FieldNames {
		if fn == name {
			return i, true
		}
	}
	return 0, false
}

// compactProductionIDs reassigns ProductionID on every production so that only
// distinct alias patterns get distinct IDs, with ID=0 meaning "no aliases".
// This is a prerequisite for keeping the parse action group count within
// uint16 range: without compaction a grammar like Markdown (50k+ productions,
// ~14 unique alias patterns) would generate 50k+ unique action group keys.
func compactProductionIDs(ng *NormalizedGrammar) {
	// Fingerprint an alias + field list into a canonical string key.
	// Productions must share the same ProductionID only when they have
	// identical alias sequences AND identical field assignments. Two
	// productions that differ in either aliases or fields must receive
	// distinct IDs; otherwise buildAliasSequences or buildFieldMaps will
	// write incorrect data to the shared slot.
	productionFingerprint := func(prod *Production) string {
		if len(prod.Aliases) == 0 && len(prod.Fields) == 0 {
			return "" // ID=0 bucket: no aliases, no named fields
		}
		var buf []byte
		// Aliases section: sorted by child index.
		sortedAliases := make([]AliasInfo, len(prod.Aliases))
		copy(sortedAliases, prod.Aliases)
		for i := 1; i < len(sortedAliases); i++ {
			for j := i; j > 0 && sortedAliases[j].ChildIndex < sortedAliases[j-1].ChildIndex; j-- {
				sortedAliases[j], sortedAliases[j-1] = sortedAliases[j-1], sortedAliases[j]
			}
		}
		buf = append(buf, 'A') // section marker
		for _, ai := range sortedAliases {
			buf = append(buf, byte(ai.ChildIndex>>8), byte(ai.ChildIndex))
			if ai.Named {
				buf = append(buf, 1)
			} else {
				buf = append(buf, 0)
			}
			buf = append(buf, []byte(ai.Name)...)
			buf = append(buf, 0) // null separator
		}
		// Fields section: sorted by child index.
		sortedFields := make([]FieldAssign, len(prod.Fields))
		copy(sortedFields, prod.Fields)
		for i := 1; i < len(sortedFields); i++ {
			for j := i; j > 0 && sortedFields[j].ChildIndex < sortedFields[j-1].ChildIndex; j-- {
				sortedFields[j], sortedFields[j-1] = sortedFields[j-1], sortedFields[j]
			}
		}
		if len(sortedFields) > 0 {
			buf = append(buf, 'F') // section marker
			for _, fa := range sortedFields {
				buf = append(buf, byte(fa.ChildIndex>>8), byte(fa.ChildIndex))
				buf = append(buf, []byte(fa.FieldName)...)
				buf = append(buf, 0) // null separator
			}
		}
		return string(buf)
	}

	fingerprintToID := make(map[string]int)
	nextID := 1 // 0 is reserved for "no aliases, no fields"

	for i := range ng.Productions {
		fp := productionFingerprint(&ng.Productions[i])
		if fp == "" {
			ng.Productions[i].ProductionID = 0
			continue
		}
		if id, ok := fingerprintToID[fp]; ok {
			ng.Productions[i].ProductionID = id
		} else {
			fingerprintToID[fp] = nextID
			ng.Productions[i].ProductionID = nextID
			nextID++
		}
	}
}

// buildAliasSequences constructs the AliasSequences table from production alias info.
// AliasSequences[productionID][childIndex] = alias symbol (0 if no alias).
func buildAliasSequences(lang *gotreesitter.Language, ng *NormalizedGrammar) {
	// Check if any production has aliases.
	hasAliases := false
	for _, prod := range ng.Productions {
		if len(prod.Aliases) > 0 {
			hasAliases = true
			break
		}
	}
	if !hasAliases {
		return
	}

	// Build a map from (alias name, named) → symbol ID. Create new symbols if needed.
	// An alias with Named=false (e.g. keyword "subgraph") must not reuse a Named=true
	// symbol (rule "subgraph") — they need separate symbol IDs, just like tree-sitter.
	type aliasKey struct {
		name  string
		named bool
	}
	aliasSymMap := make(map[aliasKey]gotreesitter.Symbol)
	for _, prod := range ng.Productions {
		for _, ai := range prod.Aliases {
			ak := aliasKey{ai.Name, ai.Named}
			if _, ok := aliasSymMap[ak]; ok {
				continue
			}
			// Check if the alias name matches an existing symbol with the same Named status.
			found := false
			for i, sn := range lang.SymbolNames {
				if sn == ai.Name && lang.SymbolMetadata[i].Named == ai.Named {
					aliasSymMap[ak] = gotreesitter.Symbol(i)
					found = true
					break
				}
			}
			if !found {
				// Create a new alias symbol at the end of the symbol table.
				newID := gotreesitter.Symbol(len(lang.SymbolNames))
				lang.SymbolNames = append(lang.SymbolNames, ai.Name)
				lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{
					Name:    ai.Name,
					Visible: true,
					Named:   ai.Named,
				})
				lang.SymbolCount = uint32(len(lang.SymbolNames))
				aliasSymMap[ak] = newID
			}
		}
	}

	// Build the AliasSequences table.
	maxProdID := 0
	for _, prod := range ng.Productions {
		if prod.ProductionID > maxProdID {
			maxProdID = prod.ProductionID
		}
	}

	lang.AliasSequences = make([][]gotreesitter.Symbol, maxProdID+1)
	for _, prod := range ng.Productions {
		if len(prod.Aliases) == 0 {
			continue
		}
		// Create a row sized to the production's RHS length.
		row := make([]gotreesitter.Symbol, len(prod.RHS))
		for _, ai := range prod.Aliases {
			if ai.ChildIndex < len(row) {
				row[ai.ChildIndex] = aliasSymMap[aliasKey{ai.Name, ai.Named}]
			}
		}
		lang.AliasSequences[prod.ProductionID] = row
	}
}

// buildSupertypeMap builds SupertypeMapSlices and SupertypeMapEntries from
// the grammar's supertype declarations. A supertype's children are the symbols
// that appear in its rule's Choice alternatives.
func buildSupertypeMap(lang *gotreesitter.Language, ng *NormalizedGrammar) error {
	if len(ng.Supertypes) == 0 {
		return nil
	}

	// Collect children for each supertype: the direct LHS symbols of productions
	// where the supertype is the LHS and the RHS is a single nonterminal.
	supertypeChildren := make(map[int][]gotreesitter.Symbol)
	for _, prod := range ng.Productions {
		for _, stID := range ng.Supertypes {
			if prod.LHS == stID && len(prod.RHS) == 1 {
				childSym := gotreesitter.Symbol(prod.RHS[0])
				supertypeChildren[stID] = append(supertypeChildren[stID], childSym)
			}
		}
	}

	// Build the flat entries table and slices.
	var entries []gotreesitter.Symbol
	symbolCount := int(lang.SymbolCount)
	lang.SupertypeMapSlices = make([][2]uint16, symbolCount)

	for _, stID := range ng.Supertypes {
		children := supertypeChildren[stID]
		if len(children) == 0 || stID >= symbolCount {
			continue
		}
		if err := checkUint16Index(ng.GrammarName, "supertype map entry", len(entries)); err != nil {
			return err
		}
		if err := checkUint16Index(ng.GrammarName, "supertype map count", len(children)); err != nil {
			return err
		}
		start := uint16(len(entries))
		entries = append(entries, children...)
		lang.SupertypeMapSlices[stID] = [2]uint16{start, uint16(len(children))}
	}

	lang.SupertypeMapEntries = entries
	return nil
}
