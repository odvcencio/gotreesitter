package grammargen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// GenerateC compiles a Grammar to a standard tree-sitter parser.c string.
// The output is compatible with tree-sitter's C runtime ABI 14/15 features
// that grammargen currently emits.
//
// Uses GenerateLanguageForC rather than GenerateLanguage: EmitC's lexer
// codegen (modeStartOf, see codegen_c.go) requires each lex mode's DFA
// states to occupy a contiguous, increasing index block, which
// GenerateLanguage's cross-mode LexState minimization does not preserve.
func GenerateC(g *Grammar) (string, error) {
	lang, err := GenerateLanguageForC(g)
	if err != nil {
		return "", fmt.Errorf("generate language: %w", err)
	}
	return EmitC(g.Name, lang)
}

// EmitC emits a parser.c string from a compiled Language struct.
func EmitC(name string, lang *gotreesitter.Language) (string, error) {
	if err := validateCSupertypeSurface(lang); err != nil {
		return "", err
	}
	if err := validateCRuntimeTables(lang); err != nil {
		return "", err
	}
	parseActionOffsets, err := cParseActionOffsets(lang)
	if err != nil {
		return "", err
	}
	// Deduplicated C identifiers per symbol id (defect: two distinct
	// same-text anonymous tokens must not collide into one enumerator).
	cNames := buildCSymbolNames(lang)
	var b strings.Builder

	emitHeader(&b, name, lang)
	emitSymbolEnum(&b, lang, cNames)
	emitFieldEnum(&b, lang)
	emitSymbolNames(&b, lang, cNames)
	emitSymbolMetadata(&b, lang, cNames)
	emitSymbolMap(&b, lang, cNames)
	emitFieldNames(&b, lang)
	emitFieldMaps(&b, lang)
	emitAliasSequences(&b, lang, cNames)
	emitNonTerminalAliasMap(&b, lang, cNames)
	emitParseActions(&b, lang, cNames)
	emitParseTable(&b, lang, parseActionOffsets, cNames)
	emitSmallParseTable(&b, lang, parseActionOffsets)
	emitLexModes(&b, lang)
	emitPrimaryStateIDs(&b, lang)
	emitReservedWords(&b, lang, cNames)
	emitSupertypes(&b, lang, cNames)
	emitLexFunction(&b, "ts_lex", lang.LexStates, lang, cNames, mainLexStartStates(lang))
	if len(lang.KeywordLexStates) > 0 {
		// The keyword lexer is always invoked with state 0 (see
		// ts_parser__call_keyword_lex_fn), so state 0 is its only start.
		emitLexFunction(&b, "ts_lex_keywords", lang.KeywordLexStates, lang, cNames, map[int]bool{0: true})
	}
	if len(lang.ExternalSymbols) > 0 {
		emitExternalScanner(&b, lang, cNames)
	}
	emitLanguageExport(&b, name, lang)

	return b.String(), nil
}

// buildCSymbolNames returns a C identifier for every symbol id, disambiguating
// collisions the way upstream tree-sitter does: the first symbol to claim a
// given identifier keeps it, and each subsequent symbol that would sanitize to
// the same identifier gets a numeric suffix (name2, name3, ...), bumped past
// any suffix already claimed by a DIFFERENT base. That second half matters:
// with a naive "base + occurrence count" suffix, three same-text-derived
// tokens — a plain `"` (base anon_sym_DQUOTE), a naturally-suffixed `"2`
// token (base anon_sym_DQUOTE2, unrelated text, coincidental collision), and
// token.immediate('"') (also base anon_sym_DQUOTE, occurrence 2 so its naive
// suffix is also anon_sym_DQUOTE2) — would have two symbols both landing on
// anon_sym_DQUOTE2, and the generated parser.c would fail to compile with
// "redeclaration of enumerator". buildCSymbolNames instead tracks every name
// already claimed by any symbol and keeps incrementing the suffix counter
// until the candidate is unclaimed, in the same index order the symbols are
// walked (so output stays deterministic).
func buildCSymbolNames(lang *gotreesitter.Language) []string {
	names := make([]string, len(lang.SymbolNames))
	counts := make(map[string]int, len(lang.SymbolNames))
	used := make(map[string]bool, len(lang.SymbolNames))
	for i := range lang.SymbolNames {
		base := symbolToCName(lang.SymbolNames[i], i, lang)
		counts[base]++
		n := counts[base]
		candidate := base
		if n > 1 {
			candidate = base + strconv.Itoa(n)
		}
		for used[candidate] {
			n++
			candidate = base + strconv.Itoa(n)
		}
		counts[base] = n
		names[i] = candidate
		used[candidate] = true
	}
	return names
}

// mainLexStartStates returns the set of lex-DFA states that a parser state can
// begin lexing from (the lex_state of every TSLexerMode). Only these "start"
// states synthesize the ts_builtin_sym_end token at end-of-input; a state that
// is only ever reached mid-token must instead report "no token" at EOF so a
// partial match becomes an error rather than a spurious end.
func mainLexStartStates(lang *gotreesitter.Language) map[int]bool {
	starts := make(map[int]bool, len(lang.LexModes))
	for _, mode := range lang.LexModes {
		starts[int(mode.LexState)] = true
		if mode.AfterWhitespaceLexState != 0 {
			starts[int(mode.AfterWhitespaceLexState)] = true
		}
	}
	// State 0 is always a valid lexer entry point.
	starts[0] = true
	return starts
}

// sortedIntKeys returns the keys of m in ascending order.
func sortedIntKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// modeStartOf returns the mode-start state that owns DFA state i. Each lex
// mode's states occupy a contiguous, increasing block of indices (buildLexDFA
// appends every mode's states after the previous mode's), and modeStarts is
// the sorted list of every such block's starting index (see the call site's
// use of mainLexStartStates). The owning mode-start is therefore the greatest
// boundary that is <= i — this holds whether i is itself a mode-start state
// or an intermediate, mid-token state reached only from within its mode.
func modeStartOf(modeStarts []int, i int) int {
	idx := sort.SearchInts(modeStarts, i+1) - 1
	if idx < 0 {
		return 0
	}
	return modeStarts[idx]
}

func cParseActionOffsets(lang *gotreesitter.Language) ([]uint16, error) {
	offsets := make([]uint16, len(lang.ParseActions))
	offset := 0
	for i, entry := range lang.ParseActions {
		if offset > int(^uint16(0)) {
			return nil, fmt.Errorf("emit C: flattened parse actions exceed uint16 at group %d", i)
		}
		offsets[i] = uint16(offset)
		offset += 1 + len(entry.Actions)
	}
	return offsets, nil
}

// maxAliasSequenceLength returns the value MAX_ALIAS_SEQUENCE_LENGTH must take:
// the length of the longest production (its child count), NOT merely the length
// of the longest alias-sequence row.
//
// The tree-sitter runtime reads ts_alias_sequences as a flat
// [PRODUCTION_ID_COUNT][MAX_ALIAS_SEQUENCE_LENGTH] array and indexes it as
// alias_sequences[production_id * max_alias_sequence_length + structural_child_index]
// for EVERY production whose production_id is non-zero — including productions
// that carry a field map but no alias, whose alias-sequence rows are all zeros.
// structural_child_index runs up to child_count-1, so if the stride is only as
// wide as the longest ALIAS row, a longer production reads past its own row into
// the next productions' rows and picks up their aliases as spurious ones (this
// is why a plain binder in `Rect(w, h)` was rendered as raw_string_literal_content:
// production 61 with 6 children read alias_sequences[61*3 + 4], which is
// production 62's aliased child). Sizing the stride to the longest production
// keeps every row self-contained and zero-padded.
func maxAliasSequenceLength(lang *gotreesitter.Language) int {
	maxLen := 0
	for _, row := range lang.AliasSequences {
		if len(row) > maxLen {
			maxLen = len(row)
		}
	}
	for _, entry := range lang.ParseActions {
		for _, action := range entry.Actions {
			if action.Type == gotreesitter.ParseActionReduce && int(action.ChildCount) > maxLen {
				maxLen = int(action.ChildCount)
			}
		}
	}
	return maxLen
}

func cParseTableValue(lang *gotreesitter.Language, actionOffsets []uint16, symbol int, value uint16) uint16 {
	if symbol >= int(lang.TokenCount) || value == 0 {
		return value
	}
	if int(value) >= len(actionOffsets) {
		return value
	}
	return actionOffsets[value]
}

func validateCSupertypeSurface(lang *gotreesitter.Language) error {
	if lang == nil {
		return fmt.Errorf("emit C: nil language")
	}
	hasSymbols := len(lang.SupertypeSymbols) > 0
	hasSlices := len(lang.SupertypeMapSlices) > 0
	hasEntries := len(lang.SupertypeMapEntries) > 0
	if !hasSymbols && !hasSlices && !hasEntries {
		return nil
	}
	if lang.LanguageVersion < 15 {
		return fmt.Errorf("emit C: supertype map requires ABI 15, got %d", lang.LanguageVersion)
	}
	if !hasSymbols || !hasSlices || !hasEntries {
		return fmt.Errorf("emit C: incomplete ABI-15 supertype surface")
	}
	if len(lang.SupertypeMapSlices) < int(lang.SymbolCount) {
		return fmt.Errorf("emit C: supertype map has %d slices for %d symbols", len(lang.SupertypeMapSlices), lang.SymbolCount)
	}
	for _, supertype := range lang.SupertypeSymbols {
		if int(supertype) >= len(lang.SymbolNames) || int(supertype) >= len(lang.SymbolMetadata) {
			return fmt.Errorf("emit C: supertype symbol %d outside symbol tables", supertype)
		}
		if !lang.SymbolMetadata[supertype].Supertype {
			return fmt.Errorf("emit C: symbol %d is in the supertype table without supertype metadata", supertype)
		}
		slice := lang.SupertypeMapSlices[supertype]
		end := int(slice[0]) + int(slice[1])
		if slice[1] == 0 || end > len(lang.SupertypeMapEntries) {
			return fmt.Errorf("emit C: invalid supertype slice for symbol %d: index=%d length=%d entries=%d", supertype, slice[0], slice[1], len(lang.SupertypeMapEntries))
		}
	}
	for _, subtype := range lang.SupertypeMapEntries {
		if int(subtype) >= len(lang.SymbolNames) {
			return fmt.Errorf("emit C: subtype symbol %d outside symbol names", subtype)
		}
	}
	return nil
}

func emitHeader(b *strings.Builder, name string, lang *gotreesitter.Language) {
	fmt.Fprintf(b, "#include <tree_sitter/parser.h>\n\n")
	fmt.Fprintf(b, "#if defined(__GNUC__) || defined(__clang__)\n")
	fmt.Fprintf(b, "#pragma GCC diagnostic push\n")
	fmt.Fprintf(b, "#pragma GCC diagnostic ignored \"-Wmissing-field-initializers\"\n")
	fmt.Fprintf(b, "#endif\n\n")

	maxAliasLen := maxAliasSequenceLength(lang)

	fmt.Fprintf(b, "#define LANGUAGE_VERSION %d\n", lang.LanguageVersion)
	fmt.Fprintf(b, "#define STATE_COUNT %d\n", lang.StateCount)
	fmt.Fprintf(b, "#define LARGE_STATE_COUNT %d\n", lang.LargeStateCount)
	fmt.Fprintf(b, "#define SYMBOL_COUNT %d\n", lang.SymbolCount)
	fmt.Fprintf(b, "#define ALIAS_COUNT 0\n")
	fmt.Fprintf(b, "#define TOKEN_COUNT %d\n", lang.TokenCount)
	fmt.Fprintf(b, "#define EXTERNAL_TOKEN_COUNT %d\n", lang.ExternalTokenCount)
	fmt.Fprintf(b, "#define FIELD_COUNT %d\n", len(cFieldNames(lang)))
	if len(lang.SupertypeSymbols) > 0 {
		fmt.Fprintf(b, "#define SUPERTYPE_COUNT %d\n", len(lang.SupertypeSymbols))
	}
	if lang.MaxReservedWordSetSize > 0 {
		fmt.Fprintf(b, "#define MAX_RESERVED_WORD_SET_SIZE %d\n", lang.MaxReservedWordSetSize)
	}
	fmt.Fprintf(b, "#define MAX_ALIAS_SEQUENCE_LENGTH %d\n", maxAliasLen)
	fmt.Fprintf(b, "#define PRODUCTION_ID_COUNT %d\n\n", lang.ProductionIDCount)
}

func emitSymbolEnum(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	fmt.Fprintf(b, "enum ts_symbol_identifiers {\n")
	for i := range lang.SymbolNames {
		if i == 0 {
			continue // ts_builtin_sym_end is implicit
		}
		fmt.Fprintf(b, "  %s = %d,\n", cNames[i], i)
	}
	fmt.Fprintf(b, "};\n\n")
}

// cFieldNames returns the grammar's field names in the order the C runtime
// requires. ts_language_field_id_for_name scans field_names in order and stops
// early when a name sorts after the one it looks for, so C field ids must follow
// the sorted names, as upstream tree-sitter assigns them. grammargen numbers
// fields in order of first appearance; cFieldIDs maps its ids to the C ids.
// Index 0 of Language.FieldNames is the empty "no field" slot and is excluded.
func cFieldNames(lang *gotreesitter.Language) []string {
	var names []string
	seen := make(map[string]bool)
	for i, name := range lang.FieldNames {
		if i == 0 || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// cFieldIDs maps each Language field id to its C field id.
func cFieldIDs(lang *gotreesitter.Language) []int {
	position := make(map[string]int)
	for i, name := range cFieldNames(lang) {
		position[name] = i + 1
	}
	ids := make([]int, len(lang.FieldNames))
	for i, name := range lang.FieldNames {
		ids[i] = position[name]
	}
	return ids
}

func emitFieldEnum(b *strings.Builder, lang *gotreesitter.Language) {
	names := cFieldNames(lang)
	if len(names) == 0 {
		return
	}
	fmt.Fprintf(b, "enum ts_field_identifiers {\n")
	for i, name := range names {
		fmt.Fprintf(b, "  field_%s = %d,\n", name, i+1)
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitSymbolNames(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	fmt.Fprintf(b, "static const char * const ts_symbol_names[] = {\n")
	for i, name := range lang.SymbolNames {
		fmt.Fprintf(b, "  [%s] = %q,\n", cNames[i], name)
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitSymbolMetadata(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	fmt.Fprintf(b, "static const TSSymbolMetadata ts_symbol_metadata[] = {\n")
	for i, meta := range lang.SymbolMetadata {
		fmt.Fprintf(b, "  [%s] = {\n", cNames[i])
		fmt.Fprintf(b, "    .visible = %s,\n", boolStr(meta.Visible))
		fmt.Fprintf(b, "    .named = %s,\n", boolStr(meta.Named))
		if meta.Supertype {
			fmt.Fprintf(b, "    .supertype = true,\n")
		}
		fmt.Fprintf(b, "  },\n")
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitFieldNames(b *strings.Builder, lang *gotreesitter.Language) {
	names := cFieldNames(lang)
	if len(names) == 0 {
		return
	}
	fmt.Fprintf(b, "static const char * const ts_field_names[] = {\n")
	fmt.Fprintf(b, "  [0] = NULL,\n")
	for _, name := range names {
		fmt.Fprintf(b, "  [field_%s] = %q,\n", name, name)
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitFieldMaps(b *strings.Builder, lang *gotreesitter.Language) {
	if len(lang.FieldMapEntries) == 0 {
		return
	}

	fmt.Fprintf(b, "static const TSMapSlice ts_field_map_slices[PRODUCTION_ID_COUNT] = {\n")
	for i, slice := range lang.FieldMapSlices {
		if slice[0] != 0 || slice[1] != 0 {
			fmt.Fprintf(b, "  [%d] = {.index = %d, .length = %d},\n", i, slice[0], slice[1])
		}
	}
	fmt.Fprintf(b, "};\n\n")

	ids := cFieldIDs(lang)
	fmt.Fprintf(b, "static const TSFieldMapEntry ts_field_map_entries[] = {\n")
	for i, entry := range lang.FieldMapEntries {
		inherited := boolStr(entry.Inherited)
		fieldID := int(entry.FieldID)
		if fieldID < len(ids) {
			fieldID = ids[fieldID]
		}
		fmt.Fprintf(b, "  [%d] = {.field_id = %d, .child_index = %d, .inherited = %s},\n",
			i, fieldID, entry.ChildIndex, inherited)
	}
	fmt.Fprintf(b, "};\n\n")
}

// emitAliasSequencesEnabled reports whether the grammar's alias-sequence
// surface is emitted. emitAliasSequences (the array definition) and
// emitLanguageExport (the .alias_sequences reference) MUST share this exact
// predicate, or the reference can name an array that was never declared
// ("use of undeclared identifier ts_alias_sequences").
//
// The predicate does not depend on whether the grammar has aliases. The
// runtime reads alias_sequences without a NULL check for every production
// whose production_id is non-zero (ts_language_alias_sequence and
// ts_language_alias_at), and a production that only carries a field map
// has a non-zero production_id. A grammar with fields and no aliases
// therefore still needs the zero-filled array, as upstream tree-sitter
// emits it. The array is only skipped when no production can index it.
func emitAliasSequencesEnabled(lang *gotreesitter.Language) bool {
	return lang.ProductionIDCount > 0 && maxAliasSequenceLength(lang) > 0
}

func emitAliasSequences(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	if !emitAliasSequencesEnabled(lang) {
		return
	}

	fmt.Fprintf(b, "static const TSSymbol ts_alias_sequences[PRODUCTION_ID_COUNT][MAX_ALIAS_SEQUENCE_LENGTH] = {\n")
	rows := 0
	for i, row := range lang.AliasSequences {
		if len(row) == 0 {
			continue
		}
		hasNonZero := false
		for _, sym := range row {
			if sym != 0 {
				hasNonZero = true
				break
			}
		}
		if !hasNonZero {
			continue
		}
		fmt.Fprintf(b, "  [%d] = {\n", i)
		for j, sym := range row {
			if sym != 0 {
				fmt.Fprintf(b, "    [%d] = %s,\n", j, cNames[sym])
			}
		}
		fmt.Fprintf(b, "  },\n")
		rows++
	}
	if rows == 0 {
		// C11 has no empty initializer. Upstream writes the same placeholder.
		fmt.Fprintf(b, "  [0] = {0},\n")
	}
	fmt.Fprintf(b, "};\n\n")
}

// emitSymbolMap writes ts_symbol_map, the public symbol of each symbol. The
// runtime reads it without a NULL check in ts_node_symbol, in
// ts_language_symbol_for_name (which every query compilation calls), and in
// query analysis. A symbol maps to the first symbol with the same name and
// namedness, which is the mapping gotreesitter's own query engine uses.
func emitSymbolMap(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	fmt.Fprintf(b, "static const TSSymbol ts_symbol_map[] = {\n")
	for i := range lang.SymbolNames {
		named := i < len(lang.SymbolMetadata) && lang.SymbolMetadata[i].Named
		public := lang.PublicSymbolForNamedness(gotreesitter.Symbol(i), named)
		fmt.Fprintf(b, "  [%s] = %s,\n", cNames[i], cNames[public])
	}
	fmt.Fprintf(b, "};\n\n")
}

// emitNonTerminalAliasMap writes ts_non_terminal_alias_map. Each entry is a
// nonterminal, a count, and the symbols that nonterminal can show as, in
// increasing symbol order; a zero ends the list. Query analysis scans the list
// without a NULL check, so the terminator is emitted even when no
// nonterminal has an alias.
func emitNonTerminalAliasMap(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	fmt.Fprintf(b, "static const uint16_t ts_non_terminal_alias_map[] = {\n")
	for sym, row := range lang.NonTerminalAliasMap {
		if len(row) == 0 {
			continue
		}
		fmt.Fprintf(b, "  %s, %d,\n", cNames[sym], len(row))
		for _, alias := range row {
			fmt.Fprintf(b, "    %s,\n", cNames[alias])
		}
	}
	fmt.Fprintf(b, "  0,\n")
	fmt.Fprintf(b, "};\n\n")
}

// emitPrimaryStateIDs writes ts_primary_state_ids. Query analysis reads it
// without a NULL check at ABI 14 and later. A state with no recorded primary
// is its own primary: the analysis then treats every state as distinct, which
// costs time but never changes a result.
func emitPrimaryStateIDs(b *strings.Builder, lang *gotreesitter.Language) {
	fmt.Fprintf(b, "static const TSStateId ts_primary_state_ids[STATE_COUNT] = {\n")
	for state := 0; state < int(lang.StateCount); state++ {
		primary := state
		if state < len(lang.PrimaryStateIDs) {
			primary = int(lang.PrimaryStateIDs[state])
		}
		fmt.Fprintf(b, "  [%d] = %d,\n", state, primary)
	}
	fmt.Fprintf(b, "};\n\n")
}

// validateCRuntimeTables rejects caller-supplied alias-map and primary-state
// tables that would index outside the emitted C arrays.
func validateCRuntimeTables(lang *gotreesitter.Language) error {
	if len(lang.NonTerminalAliasMap) > len(lang.SymbolNames) {
		return fmt.Errorf("emit C: non-terminal alias map has %d rows for %d symbols", len(lang.NonTerminalAliasMap), len(lang.SymbolNames))
	}
	for sym, row := range lang.NonTerminalAliasMap {
		for _, alias := range row {
			if int(alias) >= len(lang.SymbolNames) {
				return fmt.Errorf("emit C: non-terminal alias map row %d names symbol %d outside the symbol table", sym, alias)
			}
		}
	}
	if len(lang.PrimaryStateIDs) > int(lang.StateCount) {
		return fmt.Errorf("emit C: %d primary state ids for %d states", len(lang.PrimaryStateIDs), lang.StateCount)
	}
	for state, primary := range lang.PrimaryStateIDs {
		if uint32(primary) >= lang.StateCount {
			return fmt.Errorf("emit C: state %d has primary state %d outside %d states", state, primary, lang.StateCount)
		}
	}
	return nil
}

func emitParseActions(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	fmt.Fprintf(b, "static const TSParseActionEntry ts_parse_actions[] = {\n")
	idx := 0
	for _, entry := range lang.ParseActions {
		fmt.Fprintf(b, "  [%d] = {.entry = {.count = %d, .reusable = %s}},",
			idx, len(entry.Actions), boolStr(entry.Reusable))
		idx++
		for _, action := range entry.Actions {
			switch action.Type {
			case gotreesitter.ParseActionShift:
				if action.Extra {
					fmt.Fprintf(b, " SHIFT_EXTRA(),")
				} else if action.Repetition {
					fmt.Fprintf(b, " SHIFT_REPEAT(%d),", action.State)
				} else {
					fmt.Fprintf(b, " SHIFT(%d),", action.State)
				}
			case gotreesitter.ParseActionReduce:
				fmt.Fprintf(b, " REDUCE(%s, %d, %d, %d),",
					cNames[action.Symbol], action.ChildCount, action.DynamicPrecedence, action.ProductionID)
			case gotreesitter.ParseActionAccept:
				fmt.Fprintf(b, " ACCEPT_INPUT(),")
			case gotreesitter.ParseActionRecover:
				fmt.Fprintf(b, " RECOVER(),")
			}
			idx++
		}
		fmt.Fprintf(b, "\n")
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitParseTable(b *strings.Builder, lang *gotreesitter.Language, actionOffsets []uint16, cNames []string) {
	if lang.LargeStateCount == 0 || len(lang.ParseTable) == 0 {
		return
	}
	fmt.Fprintf(b, "static const uint16_t ts_parse_table[LARGE_STATE_COUNT][SYMBOL_COUNT] = {\n")
	for i, row := range lang.ParseTable {
		if i >= int(lang.LargeStateCount) {
			break
		}
		fmt.Fprintf(b, "  [%d] = {\n", i)
		for j, val := range row {
			if val == 0 {
				continue
			}
			fmt.Fprintf(b, "    [%s] = %d,\n", cNames[j], cParseTableValue(lang, actionOffsets, j, val))
		}
		fmt.Fprintf(b, "  },\n")
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitSmallParseTable(b *strings.Builder, lang *gotreesitter.Language, actionOffsets []uint16) {
	if len(lang.SmallParseTable) == 0 {
		return
	}
	states := cSmallParseTableData(lang, actionOffsets)
	offsets := make([]uint32, len(states))
	fmt.Fprintf(b, "static const uint16_t ts_small_parse_table[] = {\n")
	offset := uint32(0)
	for i, data := range states {
		offsets[i] = offset
		for _, val := range data {
			fmt.Fprintf(b, "  /* %d */ %d,\n", offset, val)
			offset++
		}
	}
	fmt.Fprintf(b, "};\n\n")

	fmt.Fprintf(b, "static const uint32_t ts_small_parse_table_map[] = {\n")
	for i := range states {
		fmt.Fprintf(b, "  [SMALL_STATE(%d)] = %d,\n", int(lang.LargeStateCount)+i, offsets[i])
	}
	fmt.Fprintf(b, "};\n\n")
}

// cSmallParseTableData re-encodes each small parse state in the C layout:
// a group count, then per group a value, a symbol count, and the symbols.
//
// A group holds only terminals or only nonterminals. The runtime's
// ts_lookahead_iterator__next decides once per group, from the kind of its
// first symbol, whether the group value is a parse-action index or a goto
// state. The parser's own lookup reads each symbol's value directly, so a
// mixed group still parses, but query analysis then reads a nonterminal's
// goto as a terminal action and finds no start state for that nonterminal.
// Upstream tree-sitter never mixes the two kinds, and it lists terminal groups
// first.
func cSmallParseTableData(lang *gotreesitter.Language, actionOffsets []uint16) [][]uint16 {
	type groupKey struct {
		nonterminal bool
		value       uint16
	}
	states := make([][]uint16, len(lang.SmallParseTableMap))
	for state, sourceOffset := range lang.SmallParseTableMap {
		pos := int(sourceOffset)
		if pos >= len(lang.SmallParseTable) {
			continue
		}
		groupCount := int(lang.SmallParseTable[pos])
		pos++
		groups := make(map[groupKey][]uint16)
		for group := 0; group < groupCount && pos+1 < len(lang.SmallParseTable); group++ {
			value := lang.SmallParseTable[pos]
			count := int(lang.SmallParseTable[pos+1])
			pos += 2
			for i := 0; i < count && pos < len(lang.SmallParseTable); i++ {
				symbol := lang.SmallParseTable[pos]
				pos++
				mapped := cParseTableValue(lang, actionOffsets, int(symbol), value)
				key := groupKey{nonterminal: uint32(symbol) >= lang.TokenCount, value: mapped}
				groups[key] = append(groups[key], symbol)
			}
		}
		keys := make([]groupKey, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].nonterminal != keys[j].nonterminal {
				return !keys[i].nonterminal
			}
			return keys[i].value < keys[j].value
		})
		data := []uint16{uint16(len(keys))}
		for _, key := range keys {
			syms := groups[key]
			sort.Slice(syms, func(i, j int) bool { return syms[i] < syms[j] })
			data = append(data, key.value, uint16(len(syms)))
			data = append(data, syms...)
		}
		states[state] = data
	}
	return states
}

func emitLexModes(b *strings.Builder, lang *gotreesitter.Language) {
	modeType := "TSLexMode"
	if lang.LanguageVersion >= 15 {
		modeType = "TSLexerMode"
	}
	fmt.Fprintf(b, "static const %s ts_lex_modes[STATE_COUNT] = {\n", modeType)
	for i, mode := range lang.LexModes {
		parts := []string{fmt.Sprintf(".lex_state = %d", mode.LexState)}
		if mode.ExternalLexState > 0 {
			parts = append(parts, fmt.Sprintf(".external_lex_state = %d", mode.ExternalLexState))
		}
		if lang.LanguageVersion >= 15 && mode.ReservedWordSetID > 0 {
			parts = append(parts, fmt.Sprintf(".reserved_word_set_id = %d", mode.ReservedWordSetID))
		}
		fmt.Fprintf(b, "  [%d] = {%s},\n", i, strings.Join(parts, ", "))
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitReservedWords(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	if lang.MaxReservedWordSetSize == 0 || len(lang.ReservedWords) == 0 {
		return
	}

	stride := int(lang.MaxReservedWordSetSize)
	rowCount := (len(lang.ReservedWords) + stride - 1) / stride
	fmt.Fprintf(b, "static const TSSymbol ts_reserved_words[%d][MAX_RESERVED_WORD_SET_SIZE] = {\n", rowCount)
	for row := 0; row < rowCount; row++ {
		fmt.Fprintf(b, "  [%d] = {\n", row)
		start := row * stride
		end := start + stride
		if end > len(lang.ReservedWords) {
			end = len(lang.ReservedWords)
		}
		for _, sym := range lang.ReservedWords[start:end] {
			if sym == 0 {
				break
			}
			fmt.Fprintf(b, "    %s,\n", cNames[sym])
		}
		fmt.Fprintf(b, "  },\n")
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitSupertypes(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	if len(lang.SupertypeSymbols) == 0 || len(lang.SupertypeMapEntries) == 0 {
		return
	}

	fmt.Fprintf(b, "static const TSSymbol ts_supertype_symbols[SUPERTYPE_COUNT] = {\n")
	for i, sym := range lang.SupertypeSymbols {
		if int(sym) >= len(lang.SymbolNames) {
			continue
		}
		fmt.Fprintf(b, "  [%d] = %s,\n", i, cNames[sym])
	}
	fmt.Fprintf(b, "};\n\n")

	fmt.Fprintf(b, "static const TSMapSlice ts_supertype_map_slices[SYMBOL_COUNT] = {\n")
	for sym, slice := range lang.SupertypeMapSlices {
		if slice == [2]uint16{} || sym >= len(lang.SymbolNames) {
			continue
		}
		fmt.Fprintf(b, "  [%s] = {.index = %d, .length = %d},\n",
			cNames[sym], slice[0], slice[1])
	}
	fmt.Fprintf(b, "};\n\n")

	fmt.Fprintf(b, "static const TSSymbol ts_supertype_map_entries[] = {\n")
	for _, sym := range lang.SupertypeMapEntries {
		if int(sym) >= len(lang.SymbolNames) {
			continue
		}
		fmt.Fprintf(b, "  %s,\n", cNames[sym])
	}
	fmt.Fprintf(b, "};\n\n")
}

func emitLexFunction(b *strings.Builder, funcName string, states []gotreesitter.LexState, lang *gotreesitter.Language, cNames []string, startStates map[int]bool) {
	fmt.Fprintf(b, "static bool %s(TSLexer *lexer, TSStateId state) {\n", funcName)
	fmt.Fprintf(b, "  START_LEXER();\n")
	fmt.Fprintf(b, "  eof = lexer->eof(lexer);\n")
	fmt.Fprintf(b, "  switch (state) {\n")

	// Lex modes occupy contiguous, increasing blocks of DFA state indices
	// (buildLexDFA appends each mode's states after the previous mode's), and
	// startStates is exactly the set of state indices any parser state can
	// begin lexing from — i.e. every mode-start offset (see
	// mainLexStartStates). Sorting that set therefore recovers the ordered
	// mode-start boundary list without any extra plumbing: for any DFA state
	// i, the mode that owns it is the one whose boundary is the greatest
	// entry <= i. modeStartOf uses this to answer "which mode-start state
	// should SKIP re-lex from" for a state reached mid-token (see below).
	modeStarts := sortedIntKeys(startStates)

	for i, st := range states {
		fmt.Fprintf(b, "    case %d:\n", i)

		// Accept the real token this state matches, if any. ACCEPT_TOKEN sets
		// result_symbol and marks the token end at the current position, so it
		// must run before the EOF/transition logic below (a state that both
		// accepts and can continue reports the accepted token when lookahead
		// stops matching).
		if st.AcceptToken > 0 {
			fmt.Fprintf(b, "      ACCEPT_TOKEN(%s);\n", cNames[st.AcceptToken])
		} else if st.AcceptEOF {
			fmt.Fprintln(b, "      if (eof) ACCEPT_TOKEN(ts_builtin_sym_end);")
		}

		// End-of-input handling.
		//
		// grammargen's lexer DFA never populates LexState.EOF (the pure-Go
		// lexer treats EOF specially: it follows an EOF transition if present,
		// otherwise it stops without evaluating any character transition). The
		// emitted C must mirror that: at true EOF we must NOT fall through to
		// the character transitions below, because a transition range that
		// includes codepoint 0 (common for negated classes like [^"] and for
		// `.`-style tokens) would match lookahead==0 at EOF and ADVANCE in
		// place forever. So we branch on `eof` up front.
		//
		// A start state (one a parser state can begin lexing from) that has not
		// itself accepted a token synthesizes ts_builtin_sym_end at EOF, which
		// is the end-of-file token tree-sitter's runtime requires; grammargen's
		// pure-Go runtime synthesizes it in the parser instead, so the emitted
		// lexer has to produce it here. A non-start (mid-token) state instead
		// returns whatever it accepted — nothing, for a partial match, which
		// correctly becomes a lex error rather than a spurious end token.
		if st.EOF >= 0 {
			fmt.Fprintf(b, "      if (eof) ADVANCE(%d);\n", st.EOF)
		} else if startStates[i] && st.AcceptToken == 0 && !st.Skip && !st.AcceptEOF {
			fmt.Fprintf(b, "      if (eof) ACCEPT_TOKEN(ts_builtin_sym_end);\n")
			fmt.Fprintf(b, "      if (eof) END_STATE();\n")
		} else {
			fmt.Fprintf(b, "      if (eof) END_STATE();\n")
		}

		// Character transitions. A transition that consumes a skippable extra
		// (whitespace, or any other silently-dropped extra) must use SKIP so
		// the runtime advances past the character as trivia and re-lexes for
		// a real token, rather than accepting it. grammargen models a silent
		// extra two ways:
		//
		//   - An explicit skip edge (tr.Skip): addWhitespaceSkip only ever
		//     adds these on a mode's own start state and always targets that
		//     same start state (tr.NextState), so SKIP(tr.NextState) is
		//     already correct as written.
		//   - An edge into a terminal skip-accept state: this fires on
		//     whichever state currently owns the transition, which for a
		//     multi-character extra (CRLF, backslash-newline continuations)
		//     is an intermediate mid-token state, not the mode's start state.
		//     SKIP's target must still be the mode-start state — re-lexing
		//     from the intermediate state would resume from a DFA state that
		//     only recognizes the extra's continuation, not a fresh token —
		//     so this uses modeStartOf(modeStarts, i), the start state of the
		//     mode that contains state i, rather than i itself.
		for _, tr := range st.Transitions {
			cond := charCondition(tr.Lo, tr.Hi)
			switch {
			case tr.Skip:
				fmt.Fprintf(b, "      if (%s) SKIP(%d);\n", cond, tr.NextState)
			case tr.NextState >= 0 && tr.NextState < len(states) && states[tr.NextState].Skip:
				fmt.Fprintf(b, "      if (%s) SKIP(%d);\n", cond, modeStartOf(modeStarts, i))
			default:
				fmt.Fprintf(b, "      if (%s) ADVANCE(%d);\n", cond, tr.NextState)
			}
		}

		// Default transition. Same reasoning as the transition loop above:
		// a default edge into a skip-accept state must re-lex from that
		// state's owning mode-start, not from state i itself.
		if st.Default >= 0 {
			if st.Default < len(states) && states[st.Default].Skip {
				fmt.Fprintf(b, "      SKIP(%d);\n", modeStartOf(modeStarts, i))
			} else {
				fmt.Fprintf(b, "      ADVANCE(%d);\n", st.Default)
			}
		}

		fmt.Fprintf(b, "      END_STATE();\n")
	}

	fmt.Fprintf(b, "    default:\n")
	fmt.Fprintf(b, "      return false;\n")
	fmt.Fprintf(b, "  }\n")
	fmt.Fprintf(b, "}\n\n")
}

func emitExternalScanner(b *strings.Builder, lang *gotreesitter.Language, cNames []string) {
	// External scanner symbol map.
	fmt.Fprintf(b, "static const uint16_t ts_external_scanner_symbol_map[EXTERNAL_TOKEN_COUNT] = {\n")
	for i, sym := range lang.ExternalSymbols {
		fmt.Fprintf(b, "  [%d] = %s,\n", i, cNames[sym])
	}
	fmt.Fprintf(b, "};\n\n")

	// External scanner states (validity table).
	if len(lang.ExternalLexStates) > 0 {
		fmt.Fprintf(b, "static const bool ts_external_scanner_states[%d][EXTERNAL_TOKEN_COUNT] = {\n",
			len(lang.ExternalLexStates))
		for i, row := range lang.ExternalLexStates {
			fmt.Fprintf(b, "  [%d] = {", i)
			for j, valid := range row {
				if j > 0 {
					fmt.Fprintf(b, ", ")
				}
				fmt.Fprintf(b, "%s", boolStr(valid))
			}
			fmt.Fprintf(b, "},\n")
		}
		fmt.Fprintf(b, "};\n\n")
	}
}

func emitLanguageExport(b *strings.Builder, name string, lang *gotreesitter.Language) {
	funcName := "tree_sitter_" + name

	fmt.Fprintf(b, "const TSLanguage *%s(void) {\n", funcName)
	fmt.Fprintf(b, "  static const TSLanguage language = {\n")
	fmt.Fprintf(b, "    .abi_version = LANGUAGE_VERSION,\n")
	fmt.Fprintf(b, "    .symbol_count = SYMBOL_COUNT,\n")
	fmt.Fprintf(b, "    .alias_count = ALIAS_COUNT,\n")
	fmt.Fprintf(b, "    .token_count = TOKEN_COUNT,\n")
	fmt.Fprintf(b, "    .external_token_count = EXTERNAL_TOKEN_COUNT,\n")
	fmt.Fprintf(b, "    .state_count = STATE_COUNT,\n")
	fmt.Fprintf(b, "    .large_state_count = LARGE_STATE_COUNT,\n")
	fmt.Fprintf(b, "    .production_id_count = PRODUCTION_ID_COUNT,\n")
	fmt.Fprintf(b, "    .field_count = FIELD_COUNT,\n")
	fmt.Fprintf(b, "    .max_alias_sequence_length = MAX_ALIAS_SEQUENCE_LENGTH,\n")
	fmt.Fprintf(b, "    .parse_table = &ts_parse_table[0][0],\n")

	if len(lang.SmallParseTable) > 0 {
		fmt.Fprintf(b, "    .small_parse_table = ts_small_parse_table,\n")
		fmt.Fprintf(b, "    .small_parse_table_map = ts_small_parse_table_map,\n")
	}

	fmt.Fprintf(b, "    .parse_actions = ts_parse_actions,\n")
	fmt.Fprintf(b, "    .symbol_names = ts_symbol_names,\n")
	fmt.Fprintf(b, "    .symbol_metadata = ts_symbol_metadata,\n")
	fmt.Fprintf(b, "    .public_symbol_map = ts_symbol_map,\n")

	if len(lang.FieldNames) > 1 {
		fmt.Fprintf(b, "    .field_names = ts_field_names,\n")
		fmt.Fprintf(b, "    .field_map_slices = ts_field_map_slices,\n")
		fmt.Fprintf(b, "    .field_map_entries = ts_field_map_entries,\n")
	}

	if emitAliasSequencesEnabled(lang) {
		fmt.Fprintf(b, "    .alias_sequences = &ts_alias_sequences[0][0],\n")
	}
	fmt.Fprintf(b, "    .alias_map = ts_non_terminal_alias_map,\n")

	fmt.Fprintf(b, "    .lex_modes = ts_lex_modes,\n")
	fmt.Fprintf(b, "    .lex_fn = ts_lex,\n")

	if len(lang.KeywordLexStates) > 0 {
		fmt.Fprintf(b, "    .keyword_lex_fn = ts_lex_keywords,\n")
		fmt.Fprintf(b, "    .keyword_capture_token = %d,\n", lang.KeywordCaptureToken)
	}

	if len(lang.ExternalSymbols) > 0 {
		fmt.Fprintf(b, "    .external_scanner = {\n")
		if len(lang.ExternalLexStates) > 0 {
			fmt.Fprintf(b, "      .states = ts_external_scanner_states,\n")
		}
		fmt.Fprintf(b, "      .symbol_map = ts_external_scanner_symbol_map,\n")
		fmt.Fprintf(b, "    },\n")
	}

	fmt.Fprintf(b, "    .primary_state_ids = ts_primary_state_ids,\n")
	if len(lang.ReservedWords) > 0 && lang.MaxReservedWordSetSize > 0 {
		fmt.Fprintf(b, "    .reserved_words = &ts_reserved_words[0][0],\n")
		fmt.Fprintf(b, "    .max_reserved_word_set_size = %d,\n", lang.MaxReservedWordSetSize)
	}
	if lang.LanguageVersion >= 15 {
		fmt.Fprintf(b, "    .name = %q,\n", name)
		if len(lang.SupertypeSymbols) > 0 && len(lang.SupertypeMapEntries) > 0 {
			fmt.Fprintf(b, "    .supertype_count = SUPERTYPE_COUNT,\n")
			fmt.Fprintf(b, "    .supertype_symbols = ts_supertype_symbols,\n")
			fmt.Fprintf(b, "    .supertype_map_slices = ts_supertype_map_slices,\n")
			fmt.Fprintf(b, "    .supertype_map_entries = ts_supertype_map_entries,\n")
		}
		fmt.Fprintf(b, "    .metadata = {\n")
		fmt.Fprintf(b, "      .major_version = %d,\n", lang.Metadata.MajorVersion)
		fmt.Fprintf(b, "      .minor_version = %d,\n", lang.Metadata.MinorVersion)
		fmt.Fprintf(b, "      .patch_version = %d,\n", lang.Metadata.PatchVersion)
		fmt.Fprintf(b, "    },\n")
	}

	fmt.Fprintf(b, "  };\n")
	fmt.Fprintf(b, "  return &language;\n")
	fmt.Fprintf(b, "}\n\n")

	fmt.Fprintf(b, "#if defined(__GNUC__) || defined(__clang__)\n")
	fmt.Fprintf(b, "#pragma GCC diagnostic pop\n")
	fmt.Fprintf(b, "#endif\n")
}

// symbolToCName converts a symbol name to a C identifier.
func symbolToCName(name string, id int, lang *gotreesitter.Language) string {
	if id == 0 {
		return "ts_builtin_sym_end"
	}

	// Check if it's a named symbol (nonterminal or named token).
	if id < len(lang.SymbolMetadata) && lang.SymbolMetadata[id].Named {
		return "sym_" + sanitizeCIdent(name)
	}

	// Anonymous terminal.
	return "anon_sym_" + sanitizeCIdent(name)
}

// sanitizeCIdent converts a string to a valid C identifier component.
func sanitizeCIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_':
			b.WriteRune(r)
		case r == '{':
			b.WriteString("LBRACE")
		case r == '}':
			b.WriteString("RBRACE")
		case r == '[':
			b.WriteString("LBRACK")
		case r == ']':
			b.WriteString("RBRACK")
		case r == '(':
			b.WriteString("LPAREN")
		case r == ')':
			b.WriteString("RPAREN")
		case r == '<':
			b.WriteString("LT")
		case r == '>':
			b.WriteString("GT")
		case r == '+':
			b.WriteString("PLUS")
		case r == '-':
			b.WriteString("DASH")
		case r == '*':
			b.WriteString("STAR")
		case r == '/':
			b.WriteString("SLASH")
		case r == '=':
			b.WriteString("EQ")
		case r == '!':
			b.WriteString("BANG")
		case r == '&':
			b.WriteString("AMP")
		case r == '|':
			b.WriteString("PIPE")
		case r == '^':
			b.WriteString("CARET")
		case r == '~':
			b.WriteString("TILDE")
		case r == '.':
			b.WriteString("DOT")
		case r == ',':
			b.WriteString("COMMA")
		case r == ';':
			b.WriteString("SEMI")
		case r == ':':
			b.WriteString("COLON")
		case r == '"':
			b.WriteString("DQUOTE")
		case r == '\'':
			b.WriteString("SQUOTE")
		case r == '\\':
			b.WriteString("BSLASH")
		case r == '#':
			b.WriteString("POUND")
		case r == '@':
			b.WriteString("AT")
		case r == '?':
			b.WriteString("QMARK")
		case r == '%':
			b.WriteString("PERCENT")
		case r == ' ':
			b.WriteString("_")
		default:
			fmt.Fprintf(&b, "U%04X", r)
		}
	}
	result := b.String()
	if result == "" {
		return fmt.Sprintf("_sym_%d", 0)
	}
	// C identifiers can't start with a digit.
	if result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}
	return result
}

// charCondition generates a C condition for a character range.
func charCondition(lo, hi rune) string {
	if lo == hi {
		return fmt.Sprintf("lookahead == %s", charLiteral(lo))
	}
	return fmt.Sprintf("(%s <= lookahead && lookahead <= %s)", charLiteral(lo), charLiteral(hi))
}

// charLiteral formats a rune as a C character literal.
func charLiteral(r rune) string {
	switch r {
	case '\n':
		return "'\\n'"
	case '\r':
		return "'\\r'"
	case '\t':
		return "'\\t'"
	case '\\':
		return "'\\\\'"
	case '\'':
		return "'\\''"
	case 0:
		return "0"
	default:
		if r >= 0x20 && r < 0x7f {
			return fmt.Sprintf("'%c'", r)
		}
		return fmt.Sprintf("%d", r)
	}
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
