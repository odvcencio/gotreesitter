package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ExtractedGrammar holds all data extracted from a tree-sitter parser.c file.
type ExtractedGrammar struct {
	Name               string
	StateCount         int
	LargeStateCount    int
	SymbolCount        int
	TokenCount         int
	FieldCount         int
	ProductionIDCount  int
	ExternalTokenCount int

	SymbolNames    []string
	SymbolMetadata []SymbolMeta
	FieldNames     []string

	ParseTable   [][]uint16
	ParseActions []ActionGroup
	LexModes     []LexModeEntry

	// Field mapping metadata for ChildByFieldName support.
	FieldMapSlices  [][2]uint16
	FieldMapEntries []FieldMapEntry

	// Small parse table (compressed sparse format for non-large states).
	SmallParseTable    []uint16
	SmallParseTableMap []uint32

	// Enum values extracted from the C source for resolving symbolic names.
	enumValues map[string]int

	// External token index -> grammar symbol ID.
	ExternalSymbols []uint16
}

// FieldMapEntry mirrors gotreesitter.FieldMapEntry for code generation.
type FieldMapEntry struct {
	FieldID    uint16
	ChildIndex uint16
	Inherited  bool
}

// SymbolMeta holds visibility and naming info for a symbol.
type SymbolMeta struct {
	Visible   bool
	Named     bool
	Supertype bool
}

// ActionGroup is a contiguous group of parse actions in the actions table.
type ActionGroup struct {
	Index    int // C array index of the header entry
	Count    int
	Reusable bool
	Actions  []ExtractedAction
}

// ExtractedAction is a single parse action extracted from parser.c.
type ExtractedAction struct {
	Type         string // "shift", "reduce", "accept", "recover"
	State        int    // for shift/recover
	Symbol       int    // for reduce
	ChildCount   int    // for reduce
	Precedence   int    // for reduce
	ProductionID int    // for reduce
	Extra        bool   // for shift (extra tokens)
	Repetition   bool   // for shift (repetition)
}

// LexModeEntry maps a parser state to its lexer configuration.
type LexModeEntry struct {
	LexState         int
	ExternalLexState int
}

// ExtractGrammar parses a tree-sitter parser.c source and extracts all
// structured data tables from it.
func ExtractGrammar(source string) (*ExtractedGrammar, error) {
	g := &ExtractedGrammar{}

	if err := extractConstants(source, g); err != nil {
		return nil, fmt.Errorf("constants: %w", err)
	}

	// Extract the C enum so we can resolve symbolic names in tables.
	g.enumValues = extractEnum(source)

	if err := extractLanguageName(source, g); err != nil {
		// Not fatal — name can be provided via flag.
		g.Name = "unknown"
	}

	if err := extractSymbolNames(source, g); err != nil {
		return nil, fmt.Errorf("symbol names: %w", err)
	}

	if err := extractSymbolMetadata(source, g); err != nil {
		return nil, fmt.Errorf("symbol metadata: %w", err)
	}

	if err := extractFieldNames(source, g); err != nil {
		// Not fatal — some grammars have no fields.
		g.FieldNames = nil
	}

	if strings.Contains(source, "ts_field_map_slices") || strings.Contains(source, "ts_field_map") {
		if err := extractFieldMaps(source, g); err != nil {
			// Some parser generators omit or rename field-map data.
		}
	}

	if err := extractParseTable(source, g); err != nil {
		return nil, fmt.Errorf("parse table: %w", err)
	}

	if err := extractSmallParseTable(source, g); err != nil {
		// Not fatal — only present when LARGE_STATE_COUNT < STATE_COUNT.
	}

	if err := extractParseActions(source, g); err != nil {
		return nil, fmt.Errorf("parse actions: %w", err)
	}

	if err := extractLexModes(source, g); err != nil {
		return nil, fmt.Errorf("lex modes: %w", err)
	}

	if g.ExternalTokenCount > 0 {
		if err := extractExternalSymbols(source, g); err != nil {
			// Not fatal: grammars without external scanners will omit this.
		}
	}

	return g, nil
}

// extractFieldMaps parses ts_field_map_slices[] and ts_field_map[] arrays.
func extractFieldMaps(source string, g *ExtractedGrammar) error {
	if g.ProductionIDCount == 0 {
		return nil
	}

	// Slice metadata.
	slices, err := extractFieldMapSlices(source, g)
	if err == nil {
		g.FieldMapSlices = slices
	}

	// Entry metadata.
	entries, err := extractFieldMapEntries(source, g)
	if err == nil {
		g.FieldMapEntries = entries
	}

	return nil
}

// extractConstants finds #define constants in the parser.c source.
func extractConstants(source string, g *ExtractedGrammar) error {
	defs := map[string]*int{
		"STATE_COUNT":          &g.StateCount,
		"LARGE_STATE_COUNT":    &g.LargeStateCount,
		"SYMBOL_COUNT":         &g.SymbolCount,
		"TOKEN_COUNT":          &g.TokenCount,
		"FIELD_COUNT":          &g.FieldCount,
		"PRODUCTION_ID_COUNT":  &g.ProductionIDCount,
		"EXTERNAL_TOKEN_COUNT": &g.ExternalTokenCount,
	}

	re := regexp.MustCompile(`#define\s+(\w+)\s+(\d+)`)
	matches := re.FindAllStringSubmatch(source, -1)

	for _, m := range matches {
		if ptr, ok := defs[m[1]]; ok {
			val, err := strconv.Atoi(m[2])
			if err != nil {
				return fmt.Errorf("parse %s: %w", m[1], err)
			}
			*ptr = val
		}
	}

	if g.StateCount == 0 {
		return fmt.Errorf("STATE_COUNT not found")
	}
	if g.SymbolCount == 0 {
		return fmt.Errorf("SYMBOL_COUNT not found")
	}

	return nil
}

// extractEnum parses the C enum block(s) and returns a map of name->value.
// Tree-sitter parser.c files contain enums like:
//
//	enum ts_symbol_identifiers {
//	  anon_sym_LBRACE = 1,
//	  sym_number = 3,
//	};
//
// and also:
//
//	enum ts_field_identifiers {
//	  field_key = 1,
//	};
//
// We also add the well-known ts_builtin_sym_end = 0.
func extractEnum(source string) map[string]int {
	vals := map[string]int{
		"ts_builtin_sym_end":   0,
		"ts_builtin_sym_error": 65535,
	}

	// Find all enum blocks.
	enumRe := regexp.MustCompile(`(?s)enum\s+\w+\s*\{([^}]*)\}`)
	enumMatches := enumRe.FindAllStringSubmatch(source, -1)

	entryRe := regexp.MustCompile(`(\w+)\s*=\s*(\d+)`)

	for _, em := range enumMatches {
		entries := entryRe.FindAllStringSubmatch(em[1], -1)
		for _, e := range entries {
			val, err := strconv.Atoi(e[2])
			if err == nil {
				vals[e[1]] = val
			}
		}
	}

	return vals
}

// resolveSymbol resolves a symbolic name or numeric literal to an integer
// using the extracted enum values.
func (g *ExtractedGrammar) resolveSymbol(s string) (int, bool) {
	// Try numeric first.
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	// Try enum lookup.
	if g.enumValues != nil {
		if v, ok := g.enumValues[s]; ok {
			return v, true
		}
	}
	return 0, false
}

// extractLanguageName tries to find the language name from the
// tree_sitter_LANG function at the bottom of parser.c.
func extractLanguageName(source string, g *ExtractedGrammar) error {
	re := regexp.MustCompile(`(?m)^(?:const\s+)?(?:TS_PUBLIC\s+)?(?:const\s+)?TSLanguage\s+\*tree_sitter_(\w+)\s*\(`)
	m := re.FindStringSubmatch(source)
	if m == nil {
		return fmt.Errorf("language name function not found")
	}
	g.Name = m[1]
	return nil
}

// extractSymbolNames parses the ts_symbol_names[] array.
func extractSymbolNames(source string, g *ExtractedGrammar) error {
	body, err := findArrayBody(source, "ts_symbol_names")
	if err != nil {
		return err
	}

	names, err := parseIndexedStringArray(body, g.SymbolCount, g.enumValues)
	if err != nil {
		return err
	}
	g.SymbolNames = names
	return nil
}

// extractSymbolMetadata parses the ts_symbol_metadata[] array.
func extractSymbolMetadata(source string, g *ExtractedGrammar) error {
	body, err := findArrayBody(source, "ts_symbol_metadata")
	if err != nil {
		return err
	}

	meta := make([]SymbolMeta, g.SymbolCount)

	// Split the body into individual entries by matching each [...] = { ... }
	// block. The metadata can span multiple lines, so we use a multiline
	// approach: find each entry start, then scan for the closing brace.
	type metaEntry struct {
		idx    int
		fields string
	}

	var entries []metaEntry
	idxRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*\{`)
	locs := idxRe.FindAllStringSubmatchIndex(body, -1)

	for _, loc := range locs {
		name := body[loc[2]:loc[3]]
		braceStart := loc[1] - 1 // position of '{'

		// Find matching closing brace.
		depth := 0
		end := braceStart
		for i := braceStart; i < len(body); i++ {
			if body[i] == '{' {
				depth++
			} else if body[i] == '}' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}

		idx := 0
		if n, err := strconv.Atoi(name); err == nil {
			idx = n
		} else if g.enumValues != nil {
			if v, ok := g.enumValues[name]; ok {
				idx = v
			}
		}

		entries = append(entries, metaEntry{
			idx:    idx,
			fields: body[braceStart+1 : end],
		})
	}

	for _, e := range entries {
		if e.idx >= g.SymbolCount {
			continue
		}
		meta[e.idx] = SymbolMeta{
			Visible:   strings.Contains(e.fields, ".visible = true"),
			Named:     strings.Contains(e.fields, ".named = true"),
			Supertype: strings.Contains(e.fields, ".supertype = true"),
		}
	}

	g.SymbolMetadata = meta
	return nil
}

// extractFieldNames parses the ts_field_names[] array.
func extractFieldNames(source string, g *ExtractedGrammar) error {
	if g.FieldCount == 0 {
		return nil
	}

	body, err := findArrayBody(source, "ts_field_names")
	if err != nil {
		return err
	}

	// Field names array is indexed. Index 0 is always NULL.
	names := make([]string, g.FieldCount+1) // +1 because field IDs are 1-based

	// Match entries with numeric or symbolic indices.
	re := regexp.MustCompile(`\[(\w+)\]\s*=\s*(?:NULL|"([^"]*)")`)
	matches := re.FindAllStringSubmatch(body, -1)

	for _, m := range matches {
		idx := 0
		if n, err := strconv.Atoi(m[1]); err == nil {
			idx = n
		} else if g.enumValues != nil {
			if v, ok := g.enumValues[m[1]]; ok {
				idx = v
			}
		}
		if idx >= len(names) {
			continue
		}
		names[idx] = m[2] // empty string if NULL
	}

	// If no indexed entries, try sequential parsing.
	if len(matches) == 0 {
		names = parseSequentialFieldNames(body, g.FieldCount+1)
	}

	g.FieldNames = names
	return nil
}

// parseSequentialFieldNames handles field name arrays without explicit indices.
func parseSequentialFieldNames(body string, count int) []string {
	names := make([]string, count)
	re := regexp.MustCompile(`(?:NULL|"([^"]*)")`)
	matches := re.FindAllStringSubmatch(body, -1)
	for i, m := range matches {
		if i >= count {
			break
		}
		names[i] = m[1]
	}
	return names
}

// extractParseTable parses the ts_parse_table[][] 2D array.
// This is the dense table for large states (0..LARGE_STATE_COUNT-1).
//
// In real parser.c files, the entries use symbolic enum names and macros:
//
//	[0] = {
//	  [ts_builtin_sym_end] = ACTIONS(1),
//	  [anon_sym_LBRACE] = ACTIONS(7),
//	  [sym_number] = ACTIONS(13),
//	};
//
// ACTIONS(N) and STATE(N) are macros that expand to numbers.
func extractParseTable(source string, g *ExtractedGrammar) error {
	if g.LargeStateCount == 0 {
		g.ParseTable = nil
		return nil
	}

	body, err := findArrayBody(source, "ts_parse_table")
	if err != nil {
		return err
	}

	table := make([][]uint16, g.LargeStateCount)
	for i := range table {
		table[i] = make([]uint16, g.SymbolCount)
	}

	// Find each state block: [N] = { ... } or [STATE(N)] = { ... }
	// We need to handle nested braces properly since each state contains
	// inner assignments.
	stateStartRe := regexp.MustCompile(`\[(?:STATE\()?(\w+)\)?\]\s*=\s*\{`)
	locs := stateStartRe.FindAllStringSubmatchIndex(body, -1)

	// Parse inner assignments: [sym_name] = ACTIONS(N) or [sym_name] = N
	// The values can be ACTIONS(N), STATE(N), or plain numbers.
	innerRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*(?:ACTIONS|STATE)\((\d+)\)`)
	innerPlainRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*(\d+)`)

	for _, loc := range locs {
		name := body[loc[2]:loc[3]]
		stateIdx := 0
		if n, err := strconv.Atoi(name); err == nil {
			stateIdx = n
		} else if g.enumValues != nil {
			if v, ok := g.enumValues[name]; ok {
				stateIdx = v
			}
		}

		if stateIdx >= g.LargeStateCount {
			continue
		}

		// Find matching closing brace for this state.
		braceStart := loc[1] - 1
		depth := 0
		braceEnd := braceStart
		for i := braceStart; i < len(body); i++ {
			if body[i] == '{' {
				depth++
			} else if body[i] == '}' {
				depth--
				if depth == 0 {
					braceEnd = i
					break
				}
			}
		}

		stateBody := body[braceStart+1 : braceEnd]

		// Try ACTIONS(N)/STATE(N) form first.
		innerMatches := innerRe.FindAllStringSubmatch(stateBody, -1)
		if len(innerMatches) == 0 {
			// Fall back to plain numeric form.
			innerMatches = innerPlainRe.FindAllStringSubmatch(stateBody, -1)
		}

		for _, im := range innerMatches {
			symIdx, ok := g.resolveSymbol(im[1])
			if !ok {
				continue
			}
			val, err := strconv.Atoi(im[2])
			if err != nil {
				continue
			}
			if symIdx < g.SymbolCount {
				table[stateIdx][symIdx] = uint16(val)
			}
		}
	}

	g.ParseTable = table
	return nil
}

// extractSmallParseTable parses the ts_small_parse_table[] and
// ts_small_parse_table_map[] arrays (compressed sparse table).
//
// The C source uses macros and symbolic names:
//
//	[0] = 33,
//	  ACTIONS(3), 1,
//	    sym_comment,
//	  ACTIONS(23), 1,
//	    anon_sym_LBRACK,
//	  STATE(542), 1,
//	    sym__expression,
//
// After macro expansion (ACTIONS(N)=N, STATE(N)=N), this becomes a flat
// array of uint16 values: [group_count, section_value, sym_count, sym1, ...]
// We need to resolve all symbolic names to their enum integer values.
func extractSmallParseTable(source string, g *ExtractedGrammar) error {
	// We need to find ts_small_parse_table but NOT ts_small_parse_table_map.
	body, err := findExactArrayBody(source, "ts_small_parse_table")
	if err != nil {
		return err
	}

	g.SmallParseTable = parseSmallParseTableValues(body, g.enumValues)

	// Map from state to offset.
	mapBody, err := findArrayBody(source, "ts_small_parse_table_map")
	if err != nil {
		return err
	}

	g.SmallParseTableMap = parseSmallParseTableMap(mapBody, g.LargeStateCount)
	return nil
}

// parseSmallParseTableValues extracts all uint16 values from the small parse
// table body, resolving ACTIONS(N), STATE(N) macros and symbolic enum names.
// The result is a flat uint16 array matching the compiled C format.
func parseSmallParseTableValues(body string, enums map[string]int) []uint16 {
	// Match tokens: numbers, ACTIONS(N), STATE(N), or symbolic names.
	// We need to extract them in order, skipping array indices like [0] =.
	tokenRe := regexp.MustCompile(
		`\[(\d+)\]\s*=` + // array index assignment (capture group 1)
			`|` + `(?:ACTIONS|STATE)\((\d+)\)` + // macro with number (capture group 2)
			`|` + `\b([a-zA-Z_]\w*)\b` + // symbolic name (capture group 3)
			`|` + `\b(\d+)\b`, // plain number (capture group 4)
	)

	// Keywords to skip (not enum values)
	skipWords := map[string]bool{
		"ACTIONS": true, "STATE": true, "SMALL_STATE": true,
		"static": true, "const": true, "uint16_t": true,
	}

	matches := tokenRe.FindAllStringSubmatch(body, -1)
	var result []uint16
	for _, m := range matches {
		if m[1] != "" {
			// Array index like [0] = ... — skip entirely.
			// The index is NOT part of the flat data.
			continue
		}
		if m[2] != "" {
			// ACTIONS(N) or STATE(N) — the macro is identity, so just use N.
			n, err := strconv.Atoi(m[2])
			if err == nil {
				result = append(result, uint16(n))
			}
			continue
		}
		if m[3] != "" {
			// Symbolic name — resolve via enum lookup.
			name := m[3]
			if skipWords[name] {
				continue
			}
			if enums != nil {
				if v, ok := enums[name]; ok {
					result = append(result, uint16(v))
				}
				// If not found in enums, skip (it's a C keyword or type name)
			}
			continue
		}
		if m[4] != "" {
			// Plain number.
			n, err := strconv.Atoi(m[4])
			if err == nil {
				result = append(result, uint16(n))
			}
		}
	}
	return result
}

// extractFieldMapSlices parses ts_field_map_slices[].
func extractFieldMapSlices(source string, g *ExtractedGrammar) ([][2]uint16, error) {
	body, err := findArrayBody(source, "ts_field_map_slices")
	if err != nil {
		return nil, err
	}

	slices := make([][2]uint16, g.ProductionIDCount)

	// Try indexed form: [N] = {start, length}
	indexedRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*\{([^}]*)\}`)
	matches := indexedRe.FindAllStringSubmatch(body, -1)

	if len(matches) > 0 {
		for _, m := range matches {
			idx, ok := parseFieldMapIndex(g, m[1])
			if !ok || idx >= len(slices) {
				continue
			}
			values := parseFieldIntPairs(m[2])
			if len(values) >= 2 {
				slices[idx] = [2]uint16{values[0], values[1]}
			}
		}
		return slices, nil
	}

	// Fallback: parse sequential {{a,b},{c,d},...} entries.
	pairRe := regexp.MustCompile(`\{([^{}]+)\}`)
	pairs := pairRe.FindAllStringSubmatch(body, -1)
	for i := 0; i < len(slices) && i < len(pairs); i++ {
		values := parseFieldIntPairs(pairs[i][1])
		if len(values) >= 2 {
			slices[i] = [2]uint16{values[0], values[1]}
		}
	}

	return slices, nil
}

// extractFieldMapEntries parses ts_field_map[].
func extractFieldMapEntries(source string, g *ExtractedGrammar) ([]FieldMapEntry, error) {
	body, err := findArrayBody(source, "ts_field_map")
	if err != nil {
		return nil, err
	}

	// Map entries by their array index first.
	type idxEntry struct {
		idx   int
		entry FieldMapEntry
	}

	var parsed []idxEntry
	maxIdx := -1

	// Explicit indexed entries: [N] = {...}
	indexedRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*\{([^}]*)\}`)
	indexedMatches := indexedRe.FindAllStringSubmatch(body, -1)

	for _, m := range indexedMatches {
		idx, ok := parseFieldMapIndex(g, m[1])
		if !ok {
			continue
		}
		entry, ok := parseFieldMapEntry(g, m[2])
		if !ok {
			continue
		}
		parsed = append(parsed, idxEntry{idx: idx, entry: entry})
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	// Sequential fallback if no indexed initializer entries.
	if len(parsed) == 0 {
		entryRe := regexp.MustCompile(`\{([^{}]+)\}`)
		for i, m := range entryRe.FindAllStringSubmatch(body, -1) {
			entry, ok := parseFieldMapEntry(g, m[1])
			if !ok {
				continue
			}
			parsed = append(parsed, idxEntry{idx: i, entry: entry})
			if i > maxIdx {
				maxIdx = i
			}
		}
	}

	if maxIdx < 0 {
		return nil, nil
	}

	entries := make([]FieldMapEntry, maxIdx+1)
	for _, it := range parsed {
		if it.idx >= 0 && it.idx < len(entries) {
			entries[it.idx] = it.entry
		}
	}
	return entries, nil
}

func parseFieldMapIndex(g *ExtractedGrammar, token string) (int, bool) {
	// Numeric index.
	if v, err := strconv.ParseInt(token, 0, 32); err == nil {
		return int(v), true
	}
	// Symbolic index.
	if g != nil && g.enumValues != nil {
		if v, ok := g.enumValues[token]; ok {
			return v, true
		}
	}
	return 0, false
}

func parseFieldMapEntry(g *ExtractedGrammar, body string) (FieldMapEntry, bool) {
	entry := FieldMapEntry{}
	found := false

	// Named fields: .field_id = X, .child_index = N, .inherited = bool
	if fieldID, ok := extractFieldMapTokenField(g, body, `\.field_id\s*=\s*([^,}]+)`); ok {
		entry.FieldID = fieldID
		found = true
	}
	if childIndex, ok := extractFieldMapNumericField(body, `\.child_index\s*=\s*([+-]?(?:0x[0-9a-fA-F]+|\d+))`); ok {
		entry.ChildIndex = childIndex
		found = true
	}
	if inherited, ok := extractFieldMapBoolField(body, `\.inherited\s*=\s*(true|false)`); ok {
		entry.Inherited = inherited
		found = true
	}

	// Some generator versions use .field = ... and .index = ...
	if entry.FieldID == 0 {
		if fieldID, ok := extractFieldMapTokenField(g, body, `\.field\s*=\s*([^,}]+)`); ok {
			entry.FieldID = fieldID
			found = true
		}
	}
	if entry.ChildIndex == 0 {
		if childIndex, ok := extractFieldMapNumericField(body, `\.index\s*=\s*([+-]?(?:0x[0-9a-fA-F]+|\d+))`); ok {
			entry.ChildIndex = childIndex
			found = true
		}
	}

	// Fallback for positional entries: field_id, child_index, inherited
	if !found {
		tokens := parseIdentifierLikeTokens(body)
		if len(tokens) == 0 {
			return entry, false
		}

		if len(tokens) >= 1 {
			if v, ok := parseFieldMapToken(g, tokens[0]); ok {
				entry.FieldID = v
				found = true
			}
		}
		if len(tokens) >= 2 {
			if v, ok := parseFieldMapUnsignedInt(tokens[1]); ok {
				entry.ChildIndex = v
				found = true
			}
		}
		if len(tokens) >= 3 {
			if v, ok := parseFieldMapBool(tokens[2]); ok {
				entry.Inherited = v
				found = true
			}
		}
	}

	if !found {
		return entry, false
	}
	return entry, true
}

func extractFieldMapTokenField(g *ExtractedGrammar, body, pattern string) (uint16, bool) {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(body)
	if m == nil || len(m) < 2 {
		return 0, false
	}
	val, ok := parseFieldMapToken(g, strings.TrimSpace(m[1]))
	return val, ok
}

func extractFieldMapNumericField(body, pattern string) (uint16, bool) {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(body)
	if m == nil || len(m) < 2 {
		return 0, false
	}
	return parseFieldMapUnsignedInt(m[1])
}

func extractFieldMapBoolField(body, pattern string) (bool, bool) {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(body)
	if m == nil || len(m) < 2 {
		return false, false
	}
	return parseFieldMapBool(m[1])
}

func parseFieldMapToken(g *ExtractedGrammar, token string) (uint16, bool) {
	if v, ok := parseFieldMapUnsignedInt(token); ok {
		return v, true
	}
	if token == "NULL" || token == "0" {
		return 0, true
	}
	if g != nil && g.enumValues != nil {
		if v, ok := g.enumValues[token]; ok {
			return uint16(v), true
		}
	}
	return 0, false
}

func parseFieldMapUnsignedInt(raw string) (uint16, bool) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(token, 0, 0)
	if err != nil {
		return 0, false
	}
	if v < 0 {
		return 0, false
	}
	if v > int64(^uint16(0)) {
		return 0, false
	}
	return uint16(v), true
}

func parseFieldMapBool(raw string) (bool, bool) {
	switch strings.TrimSpace(raw) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

func parseFieldIntPairs(raw string) []uint16 {
	re := regexp.MustCompile(`([+-]?(?:0x[0-9a-fA-F]+|\d+))`)
	matches := re.FindAllString(raw, -1)
	vals := make([]uint16, 0, len(matches))
	for _, tok := range matches {
		if v, ok := parseFieldMapUnsignedInt(tok); ok {
			vals = append(vals, v)
		}
	}
	return vals
}

func parseIdentifierLikeTokens(raw string) []string {
	re := regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*|[+-]?(?:0x[0-9a-fA-F]+|\d+)|true|false)\b`)
	matches := re.FindAllString(raw, -1)
	return matches
}

// parseSmallParseTableMap extracts the small parse table map which maps
// (state - LARGE_STATE_COUNT) to offsets in the small parse table.
// The C format is: [SMALL_STATE(N)] = offset
// where SMALL_STATE(N) = N - LARGE_STATE_COUNT.
func parseSmallParseTableMap(body string, largeStateCount int) []uint32 {
	// Match entries: [SMALL_STATE(N)] = M or [N] = M
	entryRe := regexp.MustCompile(`\[(?:SMALL_STATE\()?(\d+)\)?\]\s*=\s*(\d+)`)
	matches := entryRe.FindAllStringSubmatch(body, -1)

	// Find the max index to size the array.
	maxIdx := 0
	type mapEntry struct {
		idx int
		val uint32
	}
	var entries []mapEntry
	for _, m := range matches {
		rawIdx, err1 := strconv.Atoi(m[1])
		val, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		// SMALL_STATE(N) = N - LARGE_STATE_COUNT, so the array index is
		// (N - LARGE_STATE_COUNT) if using SMALL_STATE, or N if raw.
		// Since the regex captures the inner number, for SMALL_STATE(29)
		// we get 29. The compiled index would be 29-29=0.
		// We need to figure out if it's SMALL_STATE or raw.
		idx := rawIdx
		if rawIdx >= largeStateCount {
			// It's SMALL_STATE(N) which evaluates to N - LARGE_STATE_COUNT
			idx = rawIdx - largeStateCount
		}
		entries = append(entries, mapEntry{idx: idx, val: uint32(val)})
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	result := make([]uint32, maxIdx+1)
	for _, e := range entries {
		if e.idx < len(result) {
			result[e.idx] = e.val
		}
	}
	return result
}

// extractParseActions parses the ts_parse_actions[] array.
//
// Real parser.c format uses 4-arg REDUCE:
//
//	[0] = {.entry = {.count = 0, .reusable = false}},
//	[1] = {.entry = {.count = 1, .reusable = false}}, RECOVER(),
//	[3] = {.entry = {.count = 1, .reusable = true}}, SHIFT_EXTRA(),
//	[5] = {.entry = {.count = 1, .reusable = true}}, REDUCE(sym_document, 0, 0, 0),
//	[7] = {.entry = {.count = 1, .reusable = true}}, SHIFT(16),
//	[19] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_document_repeat1, 2, 0, 0), SHIFT_REPEAT(16),
func extractParseActions(source string, g *ExtractedGrammar) error {
	body, err := findArrayBody(source, "ts_parse_actions")
	if err != nil {
		return err
	}

	// Split into lines and parse sequentially.
	lines := strings.Split(body, "\n")

	var groups []ActionGroup
	var currentGroup *ActionGroup

	// Match header with optional C array index: [N] = {.entry = ...}
	headerRe := regexp.MustCompile(`(?:\[(\d+)\]\s*=\s*)?\{\.entry\s*=\s*\{\.count\s*=\s*(\d+),\s*\.reusable\s*=\s*(true|false)\s*\}\}`)
	shiftRe := regexp.MustCompile(`(?:^|,\s*)\bSHIFT\((\d+)\)`)
	shiftExtraRe := regexp.MustCompile(`\bSHIFT_EXTRA\(\)`)
	shiftRepeatRe := regexp.MustCompile(`\bSHIFT_REPEAT\((\d+)\)`)
	// REDUCE can have 3 or 4 args: REDUCE(sym, count, prec) or REDUCE(sym, count, prec, prod_id)
	// The precedence can be negative (e.g., -1 for type aliases in Go grammar).
	reduceRe := regexp.MustCompile(`\bREDUCE\((\w+),\s*(\d+),\s*(-?\d+)(?:,\s*(\d+))?\)`)
	acceptRe := regexp.MustCompile(`\bACCEPT_INPUT\(\)`)
	recoverRe := regexp.MustCompile(`\bRECOVER\(\)`)

	// Track the next expected C index for groups without explicit indices.
	nextCIndex := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for header entry.
		if hm := headerRe.FindStringSubmatch(line); hm != nil {
			// Flush previous group.
			if currentGroup != nil {
				groups = append(groups, *currentGroup)
			}
			// Capture the C array index if present.
			cIndex := nextCIndex
			if hm[1] != "" {
				cIndex, _ = strconv.Atoi(hm[1])
			}
			count, _ := strconv.Atoi(hm[2])
			currentGroup = &ActionGroup{
				Index:    cIndex,
				Count:    count,
				Reusable: hm[3] == "true",
			}
			// Next C index = current + 1 (header) + count (actions)
			nextCIndex = cIndex + 1 + count
		}

		if currentGroup == nil {
			continue
		}

		// Collect all action matches with their positions so we can
		// add them in source order.
		var matches []actionMatch

		// SHIFT_EXTRA
		for _, loc := range shiftExtraRe.FindAllStringIndex(line, -1) {
			matches = append(matches, actionMatch{
				pos:    loc[0],
				action: ExtractedAction{Type: "shift", Extra: true},
			})
		}

		// SHIFT_REPEAT
		for _, idx := range shiftRepeatRe.FindAllStringSubmatchIndex(line, -1) {
			state, _ := strconv.Atoi(line[idx[2]:idx[3]])
			matches = append(matches, actionMatch{
				pos:    idx[0],
				action: ExtractedAction{Type: "shift", State: state, Repetition: true},
			})
		}

		// SHIFT (plain — not SHIFT_EXTRA or SHIFT_REPEAT)
		for _, idx := range shiftRe.FindAllStringSubmatchIndex(line, -1) {
			pos := idx[0]
			// Check this is not part of SHIFT_EXTRA or SHIFT_REPEAT by
			// looking at what follows SHIFT in the source.
			shiftStart := strings.Index(line[pos:], "SHIFT")
			if shiftStart >= 0 {
				afterShift := pos + shiftStart + 5 // len("SHIFT")
				if afterShift < len(line) && line[afterShift] == '_' {
					continue // it's SHIFT_EXTRA or SHIFT_REPEAT
				}
			}
			state, _ := strconv.Atoi(line[idx[2]:idx[3]])
			matches = append(matches, actionMatch{
				pos:    pos,
				action: ExtractedAction{Type: "shift", State: state},
			})
		}

		// REDUCE
		for _, idx := range reduceRe.FindAllStringSubmatchIndex(line, -1) {
			symStr := line[idx[2]:idx[3]]
			sym, _ := g.resolveSymbol(symStr)
			childCount, _ := strconv.Atoi(line[idx[4]:idx[5]])
			prec, _ := strconv.Atoi(line[idx[6]:idx[7]])
			prodID := 0
			if idx[8] >= 0 && idx[9] >= 0 {
				prodID, _ = strconv.Atoi(line[idx[8]:idx[9]])
			}
			matches = append(matches, actionMatch{
				pos: idx[0],
				action: ExtractedAction{
					Type:         "reduce",
					Symbol:       sym,
					ChildCount:   childCount,
					Precedence:   prec,
					ProductionID: prodID,
				},
			})
		}

		// ACCEPT_INPUT
		for _, loc := range acceptRe.FindAllStringIndex(line, -1) {
			matches = append(matches, actionMatch{
				pos:    loc[0],
				action: ExtractedAction{Type: "accept"},
			})
		}

		// RECOVER
		for _, loc := range recoverRe.FindAllStringIndex(line, -1) {
			matches = append(matches, actionMatch{
				pos:    loc[0],
				action: ExtractedAction{Type: "recover"},
			})
		}

		// Sort by position and append.
		sortActionMatches(matches)
		for _, m := range matches {
			currentGroup.Actions = append(currentGroup.Actions, m.action)
		}
	}

	// Flush last group.
	if currentGroup != nil {
		groups = append(groups, *currentGroup)
	}

	g.ParseActions = groups
	return nil
}

// extractLexModes parses the ts_lex_modes[] array.
func extractLexModes(source string, g *ExtractedGrammar) error {
	body, err := findArrayBody(source, "ts_lex_modes")
	if err != nil {
		return err
	}

	modes := make([]LexModeEntry, g.StateCount)
	re := regexp.MustCompile(`\[(\d+)\]\s*=\s*\{([^}]*)\}`)
	matches := re.FindAllStringSubmatch(body, -1)

	lexStateRe := regexp.MustCompile(`\.lex_state\s*=\s*(\d+)`)
	extLexRe := regexp.MustCompile(`\.external_lex_state\s*=\s*(\d+)`)

	for _, m := range matches {
		idx, err := strconv.Atoi(m[1])
		if err != nil || idx >= g.StateCount {
			continue
		}
		fields := m[2]
		entry := LexModeEntry{}
		if lm := lexStateRe.FindStringSubmatch(fields); lm != nil {
			entry.LexState, _ = strconv.Atoi(lm[1])
		}
		if em := extLexRe.FindStringSubmatch(fields); em != nil {
			entry.ExternalLexState, _ = strconv.Atoi(em[1])
		}
		modes[idx] = entry
	}

	g.LexModes = modes
	return nil
}

// --- Helper functions ---

// actionMatch holds a parsed action and its position in the line
// for position-ordered insertion.
type actionMatch struct {
	pos    int
	action ExtractedAction
}

// sortActionMatches sorts action matches by their position in the source line.
func sortActionMatches(matches []actionMatch) {
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].pos < matches[j].pos
	})
}

// findArrayBody finds a C array declaration and returns the body between
// the outermost braces. It handles nested braces correctly.
func findArrayBody(source, name string) (string, error) {
	// Find the array declaration: "name[...] = {" or "name[...][...] = {"
	pattern := regexp.MustCompile(`(?m)` + regexp.QuoteMeta(name) + `\s*\[[^\]]*\](?:\s*\[[^\]]*\])?\s*=\s*\{`)
	loc := pattern.FindStringIndex(source)
	if loc == nil {
		return "", fmt.Errorf("array %q not found", name)
	}

	// Find the opening brace.
	start := strings.LastIndex(source[loc[0]:loc[1]], "{")
	if start == -1 {
		return "", fmt.Errorf("opening brace not found for %q", name)
	}
	start += loc[0]

	end, err := findMatchingBrace(source, start)
	if err != nil {
		return "", fmt.Errorf("unmatched brace for %q: %w", name, err)
	}
	return source[start+1 : end], nil
}

// findExactArrayBody is like findArrayBody but ensures the name is not a
// prefix of a longer name (e.g., "ts_small_parse_table" vs
// "ts_small_parse_table_map").
func findExactArrayBody(source, name string) (string, error) {
	// Find the array declaration ensuring the name is followed by [ not _
	pattern := regexp.MustCompile(`(?m)` + regexp.QuoteMeta(name) + `\[`)
	locs := pattern.FindAllStringIndex(source, -1)

	for _, loc := range locs {
		// Check that the character before the match is not alphanumeric or underscore
		// (to avoid matching ts_small_parse_table_map when looking for ts_small_parse_table).
		// Actually we need to verify the full context. Let's check that at loc[1]-1
		// we have '[' and the name before it matches exactly.
		matchStr := source[loc[0]:loc[1]]
		if matchStr != name+"[" {
			continue
		}

		// Now find the "= {" after this point.
		rest := source[loc[0]:]
		bracePattern := regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `\s*\[[^\]]*\](?:\s*\[[^\]]*\])?\s*=\s*\{`)
		bm := bracePattern.FindStringIndex(rest)
		if bm == nil {
			continue
		}

		// Find opening brace.
		segment := rest[bm[0]:bm[1]]
		bracePos := strings.LastIndex(segment, "{")
		if bracePos == -1 {
			continue
		}
		start := loc[0] + bm[0] + bracePos

		end, err := findMatchingBrace(source, start)
		if err == nil {
			return source[start+1 : end], nil
		}
	}

	return "", fmt.Errorf("array %q not found (exact)", name)
}

// findMatchingBrace returns the index of the closing brace that matches the
// opening brace at start. Braces inside C strings, chars, and comments are ignored.
func findMatchingBrace(source string, start int) (int, error) {
	if start < 0 || start >= len(source) || source[start] != '{' {
		return 0, fmt.Errorf("invalid opening brace index")
	}

	depth := 0
	inString := false
	inChar := false
	inLineComment := false
	inBlockComment := false
	escaped := false

	for i := start; i < len(source); i++ {
		c := source[i]

		if inLineComment {
			if c == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if c == '*' && i+1 < len(source) && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '\'' {
				inChar = false
			}
			continue
		}

		if c == '/' && i+1 < len(source) {
			next := source[i+1]
			if next == '/' {
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		switch c {
		case '"':
			inString = true
		case '\'':
			inChar = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
			if depth < 0 {
				return 0, fmt.Errorf("negative brace depth")
			}
		}
	}

	return 0, fmt.Errorf("no matching closing brace")
}

// parseIndexedStringArray parses a C array of the form:
//
//	[ts_builtin_sym_end] = "end",
//	[anon_sym_LBRACE] = "{",
//
// or sequential: "foo", "bar", ...
func parseIndexedStringArray(body string, count int, enums map[string]int) ([]string, error) {
	result := make([]string, count)

	// Try indexed form first: [name] = "string"
	indexedRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*"((?:[^"\\]|\\.)*)"`)
	matches := indexedRe.FindAllStringSubmatch(body, -1)

	if len(matches) > 0 {
		for i, m := range matches {
			idx := i
			if n, err := strconv.Atoi(m[1]); err == nil {
				idx = n
			} else if enums != nil {
				if v, ok := enums[m[1]]; ok {
					idx = v
				}
			}
			if idx < count {
				result[idx] = unescapeCString(m[2])
			}
		}
		return result, nil
	}

	// Fall back to sequential form: "string1", "string2"
	seqRe := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	seqMatches := seqRe.FindAllStringSubmatch(body, -1)
	for i, m := range seqMatches {
		if i >= count {
			break
		}
		result[i] = unescapeCString(m[1])
	}

	return result, nil
}

// unescapeCString handles basic C string escape sequences.
func unescapeCString(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00BACKSLASH\x00")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, "\x00BACKSLASH\x00", `\`)
	return s
}

// parseUint16List parses a comma-separated list of uint16 values from
// the body of a C array declaration.
func parseUint16List(body string) []uint16 {
	re := regexp.MustCompile(`\d+`)
	matches := re.FindAllString(body, -1)
	result := make([]uint16, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m)
		if err != nil {
			continue
		}
		result = append(result, uint16(n))
	}
	return result
}

// parseUint32List parses a comma-separated list of uint32 values from
// the body of a C array declaration.
func parseUint32List(body string) []uint32 {
	re := regexp.MustCompile(`\d+`)
	matches := re.FindAllString(body, -1)
	result := make([]uint32, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m)
		if err != nil {
			continue
		}
		result = append(result, uint32(n))
	}
	return result
}

// extractExternalSymbols parses ts_external_scanner_symbol_map[].
func extractExternalSymbols(source string, g *ExtractedGrammar) error {
	body, err := findArrayBody(source, "ts_external_scanner_symbol_map")
	if err != nil {
		return err
	}
	g.ExternalSymbols = parseIndexedSymbolArray(body, g.ExternalTokenCount, g.enumValues)
	return nil
}

// parseIndexedSymbolArray parses a C symbol array in either indexed form:
//
//	[ext_tok_foo] = sym_bar,
//
// or sequential form:
//
//	sym_bar, sym_baz,
func parseIndexedSymbolArray(body string, count int, enums map[string]int) []uint16 {
	result := make([]uint16, count)
	if count == 0 {
		return result
	}

	// Indexed form.
	indexedRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*(\w+)`)
	indexed := indexedRe.FindAllStringSubmatch(body, -1)
	if len(indexed) > 0 {
		for i, m := range indexed {
			idx := i
			if v, ok := resolveIndexedName(m[1], enums); ok {
				idx = v
			}
			sym, ok := resolveIndexedName(m[2], enums)
			if !ok || idx < 0 || idx >= count {
				continue
			}
			result[idx] = uint16(sym)
		}
		return result
	}

	// Sequential form.
	tokenRe := regexp.MustCompile(`\b([A-Za-z_]\w*|\d+)\b`)
	toks := tokenRe.FindAllStringSubmatch(body, -1)
	out := 0
	for _, t := range toks {
		if out >= count {
			break
		}
		sym, ok := resolveIndexedName(t[1], enums)
		if !ok {
			continue
		}
		result[out] = uint16(sym)
		out++
	}
	return result
}

func resolveIndexedName(name string, enums map[string]int) (int, bool) {
	if n, err := strconv.Atoi(name); err == nil {
		return n, true
	}
	if enums != nil {
		if v, ok := enums[name]; ok {
			return v, true
		}
	}
	return 0, false
}
