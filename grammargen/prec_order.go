package grammargen

// precOrderTable stores the symbol-level precedence ordering from the
// grammar's precedences table. It enables conflict resolution to compare
// a reduce production's LHS rule against a named precedence from a
// competing shift action, using the ordering defined in the grammar's
// precedences array.
//
// In tree-sitter grammars, the precedences array contains two types:
//   - STRING entries: define named precedence values (e.g. "logical_and")
//   - SYMBOL entries: define rule-level precedence (e.g. update_expression)
//
// When both a SYMBOL entry and a STRING entry appear in the ordering,
// earlier entries have higher precedence. This is used by tree-sitter C
// to resolve shift/reduce conflicts where the reduce LHS is a SYMBOL
// entry and the shift token's precedence is a STRING entry.
//
// Example from JavaScript:
//
//	precedences: [member, call, update_expression, unary_void, ..., logical_and, ...]
//
// This means update_expression (position 3) outranks logical_and (position 15),
// so in `++x && y`, the reduce to update_expression wins over shift `&&`.
type precOrderTable struct {
	// symbolPositions maps SYMBOL rule names to their global position.
	// Higher value = higher precedence.
	symbolPositions map[string]int

	// symbolLevels maps SYMBOL rule names to their precedences level index.
	// Two symbols in the same level are directly comparable; symbols in
	// different levels are not (they serve different ordering purposes).
	symbolLevels map[string]int

	// namedPrecPositions maps STRING-only numeric prec values (from
	// buildNamedPrecMap) to their global position. This reverse mapping
	// enables conflict resolution to compare shift prec values (which
	// use STRING-only numbering) against SYMBOL positions.
	namedPrecPositions map[int]int
}

// applyAliasRenamesToPrecedences returns a copy of the given Precedences
// levels with every SYMBOL entry's Name rewritten via the rename map.
// Called after promoteDefaultAliases so the precedence table built from
// the result lines up with the symbols' post-promotion identity.
//
// When renames is empty/nil the input is returned as-is (no allocation).
func applyAliasRenamesToPrecedences(levels [][]PrecEntry, renames map[string]string) [][]PrecEntry {
	if len(renames) == 0 || len(levels) == 0 {
		return levels
	}
	out := make([][]PrecEntry, len(levels))
	for i, level := range levels {
		if len(level) == 0 {
			out[i] = level
			continue
		}
		copied := make([]PrecEntry, len(level))
		for j, e := range level {
			copied[j] = e
			if e.IsSymbol {
				if newName, ok := renames[e.Name]; ok {
					copied[j].Name = newName
				}
			}
		}
		out[i] = copied
	}
	return out
}

// buildPrecOrderTable builds a precedence ordering table from the Grammar's
// Precedences field and a namedPrecs map (STRING-only prec values as
// assigned by buildNamedPrecMap). All entries (STRING and SYMBOL) across
// all levels participate in a single global ordering.
func buildPrecOrderTable(levels [][]PrecEntry, namedPrecs map[string]int) *precOrderTable {
	if len(levels) == 0 {
		return nil
	}

	// Count total entries for value assignment.
	total := 0
	for _, level := range levels {
		total += len(level)
	}
	if total == 0 {
		return nil
	}

	symbolPositions := make(map[string]int)
	symbolLevels := make(map[string]int)
	namedPrecPositions := make(map[int]int)

	idx := 0
	for levelIdx, level := range levels {
		for _, e := range level {
			val := total - 1 - idx // higher value = higher precedence
			idx++
			if e.IsSymbol {
				if existing, ok := symbolPositions[e.Name]; !ok || val > existing {
					symbolPositions[e.Name] = val
					symbolLevels[e.Name] = levelIdx
				}
			} else {
				// Look up the STRING entry's numeric prec value.
				if npVal, ok := namedPrecs[e.Name]; ok {
					if existing, ok := namedPrecPositions[npVal]; !ok || val > existing {
						namedPrecPositions[npVal] = val
					}
				}
			}
		}
	}

	if len(symbolPositions) == 0 {
		return nil
	}

	return &precOrderTable{
		symbolPositions:    symbolPositions,
		symbolLevels:       symbolLevels,
		namedPrecPositions: namedPrecPositions,
	}
}

// buildNamedPrecMapFromLevels builds the STRING-only named prec map from
// parsed PrecEntry levels. This mirrors buildNamedPrecMap but works with
// the Grammar IR's PrecEntry format instead of raw JSON.
func buildNamedPrecMapFromLevels(levels [][]PrecEntry) map[string]int {
	type precEntry struct {
		name      string
		globalIdx int
	}
	var all []precEntry
	for _, level := range levels {
		for _, e := range level {
			if !e.IsSymbol && e.Name != "" {
				all = append(all, precEntry{name: e.Name, globalIdx: len(all)})
			}
		}
	}
	m := make(map[string]int, len(all))
	total := len(all)
	for _, e := range all {
		val := total - 1 - e.globalIdx
		if existing, ok := m[e.name]; !ok || val > existing {
			m[e.name] = val
		}
	}
	return m
}

// resolveSymbolVsSymbol checks whether SYMBOL entry symbolA outranks
// SYMBOL entry symbolB according to the precedences table. Only compares
// symbols that appear in the same precedence level — symbols in different
// levels serve different ordering purposes and are not directly comparable.
// Returns:
//
//	 1 if symbolA has higher precedence (earlier in the same level)
//	-1 if symbolB has higher precedence
//	 0 if not comparable (different levels, not in table, or same position)
func (t *precOrderTable) resolveSymbolVsSymbol(symbolA, symbolB string) int {
	if t == nil {
		return 0
	}

	posA, okA := t.symbolPositions[symbolA]
	if !okA {
		return 0
	}

	posB, okB := t.symbolPositions[symbolB]
	if !okB {
		return 0
	}

	// Only compare symbols in the same precedence level.
	levelA, _ := t.symbolLevels[symbolA]
	levelB, _ := t.symbolLevels[symbolB]
	if levelA != levelB {
		return 0
	}

	if posA > posB {
		return 1
	}
	if posA < posB {
		return -1
	}
	return 0
}

// resolveSymbolVsNamedPrec checks whether a SYMBOL entry (symbolName)
// outranks a named precedence value (namedPrec) according to the
// precedences table. Returns:
//
//	 1 if the symbol has higher precedence
//	-1 if the named prec has higher precedence
//	 0 if not comparable (one or both not in the table)
func (t *precOrderTable) resolveSymbolVsNamedPrec(symbolName string, namedPrec int) int {
	if t == nil {
		return 0
	}

	symPos, symOK := t.symbolPositions[symbolName]
	if !symOK {
		return 0
	}

	precPos, precOK := t.namedPrecPositions[namedPrec]
	if !precOK {
		return 0
	}

	if symPos > precPos {
		return 1
	}
	if symPos < precPos {
		return -1
	}
	return 0
}
