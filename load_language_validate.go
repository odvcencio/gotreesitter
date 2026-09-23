package gotreesitter

import "fmt"

// maxSaneLanguageTableCount bounds every count field on a decoded Language
// (SymbolCount, TokenCount, StateCount, and friends). It is set far above the
// largest shipped grammar (COBOL, ~67K states as of v0.43.1) so it only ever
// rejects a corrupted or adversarial count, never a legitimate large grammar.
const maxSaneLanguageTableCount = 4_000_000

// validateDecodedLanguage performs structural sanity checks on a Language
// freshly gob-decoded from an untrusted grammar blob. LoadLanguage calls it
// before returning, so a malformed or adversarial blob fails to load with a
// clear error instead of reaching the lexer or parser later with an index
// that runs past the end of the table it indexes into -- which panics,
// because the hot parse and lex loops trust a Language's internal table
// consistency and mostly do not re-check it themselves.
//
// The checks are deliberately conservative: every shipped grammar blob must
// pass them (see TestValidateDecodedLanguageAcceptsAllShippedBlobs), so they
// verify only the index bounds that later table lookups rely on, not
// grammar-specific semantics.
func validateDecodedLanguage(lang *Language) error {
	if lang == nil {
		return fmt.Errorf("validate language: nil language")
	}
	if err := validateLanguageCountFields(lang); err != nil {
		return err
	}
	if err := validateLanguageSymbolTables(lang); err != nil {
		return err
	}
	if err := validateLanguageLexTables(lang); err != nil {
		return err
	}
	if err := validateLanguageParseTables(lang); err != nil {
		return err
	}
	return nil
}

func validateLanguageCountFields(lang *Language) error {
	counts := []struct {
		name  string
		value uint32
	}{
		{"SymbolCount", lang.SymbolCount},
		{"TokenCount", lang.TokenCount},
		{"ExternalTokenCount", lang.ExternalTokenCount},
		{"StateCount", lang.StateCount},
		{"LargeStateCount", lang.LargeStateCount},
		{"FieldCount", lang.FieldCount},
		{"ProductionIDCount", lang.ProductionIDCount},
	}
	for _, c := range counts {
		if c.value > maxSaneLanguageTableCount {
			return fmt.Errorf("validate language: %s = %d exceeds the sane limit of %d", c.name, c.value, maxSaneLanguageTableCount)
		}
	}
	if lang.SymbolCount > 0 && lang.TokenCount > lang.SymbolCount {
		return fmt.Errorf("validate language: TokenCount (%d) exceeds SymbolCount (%d)", lang.TokenCount, lang.SymbolCount)
	}
	if lang.StateCount > 0 && lang.LargeStateCount > lang.StateCount {
		return fmt.Errorf("validate language: LargeStateCount (%d) exceeds StateCount (%d)", lang.LargeStateCount, lang.StateCount)
	}
	return nil
}

func validateLanguageSymbolTables(lang *Language) error {
	symbolCount := int(lang.SymbolCount)
	if symbolCount == 0 {
		return nil
	}
	if len(lang.SymbolNames) > 0 && len(lang.SymbolNames) < symbolCount {
		return fmt.Errorf("validate language: SymbolNames has %d entries, shorter than SymbolCount %d", len(lang.SymbolNames), symbolCount)
	}
	if len(lang.SymbolMetadata) > 0 && len(lang.SymbolMetadata) < symbolCount {
		return fmt.Errorf("validate language: SymbolMetadata has %d entries, shorter than SymbolCount %d", len(lang.SymbolMetadata), symbolCount)
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) >= symbolCount {
			return fmt.Errorf("validate language: ExternalSymbols[%d] = %d is outside SymbolCount %d", i, sym, symbolCount)
		}
	}
	for i, sym := range lang.SupertypeSymbols {
		if int(sym) >= symbolCount {
			return fmt.Errorf("validate language: SupertypeSymbols[%d] = %d is outside SymbolCount %d", i, sym, symbolCount)
		}
	}
	for i, sym := range lang.ReservedWords {
		if sym != 0 && int(sym) >= symbolCount {
			return fmt.Errorf("validate language: ReservedWords[%d] = %d is outside SymbolCount %d", i, sym, symbolCount)
		}
	}
	// Alias sequences can legitimately name "virtual" alias symbols beyond
	// SymbolCount: ts2go's extractor (cmd/ts2go/extract.go) appends AliasCount
	// extra entries to SymbolNames/SymbolMetadata past the real symbol range
	// and lets AliasSequences reference that extended range. The runtime does
	// not carry AliasCount separately, so the widest available name/metadata
	// table is the real bound here, not SymbolCount.
	aliasCeiling := symbolCount
	if n := len(lang.SymbolNames); n > aliasCeiling {
		aliasCeiling = n
	}
	if n := len(lang.SymbolMetadata); n > aliasCeiling {
		aliasCeiling = n
	}
	for i, row := range lang.AliasSequences {
		for j, sym := range row {
			if sym != 0 && int(sym) >= aliasCeiling {
				return fmt.Errorf("validate language: AliasSequences[%d][%d] = %d is outside the symbol+alias range %d", i, j, sym, aliasCeiling)
			}
		}
	}
	return nil
}

func validateLanguageLexTables(lang *Language) error {
	stateCount := int(lang.StateCount)
	if stateCount > 0 && len(lang.LexModes) > 0 && stateCount > len(lang.LexModes) {
		return fmt.Errorf("validate language: StateCount (%d) exceeds LexModes length (%d)", stateCount, len(lang.LexModes))
	}

	numLexStates := len(lang.LexStates)
	for i, mode := range lang.LexModes {
		if numLexStates > 0 {
			if idx := mode.LexStateIndex(); idx != ^uint32(0) && int(idx) >= numLexStates {
				return fmt.Errorf("validate language: LexModes[%d] references lex state %d, outside LexStates (len %d)", i, idx, numLexStates)
			}
			// AfterWhitespaceLexState==0 means "same as LexState", not a
			// literal index into LexStates -- see LexMode's doc comment.
			if idx := mode.AfterWhitespaceLexStateIndex(); idx != ^uint32(0) && idx != 0 && int(idx) >= numLexStates {
				return fmt.Errorf("validate language: LexModes[%d] references after-whitespace lex state %d, outside LexStates (len %d)", i, idx, numLexStates)
			}
		}
		if len(lang.ExternalLexStates) > 0 && int(mode.ExternalLexState) >= len(lang.ExternalLexStates) {
			return fmt.Errorf("validate language: LexModes[%d] references ExternalLexStates row %d, outside table (len %d)", i, mode.ExternalLexState, len(lang.ExternalLexStates))
		}
	}

	symbolCount := int(lang.SymbolCount)
	for i, ls := range lang.LexStates {
		if symbolCount > 0 && ls.AcceptToken != 0 && int(ls.AcceptToken) >= symbolCount {
			return fmt.Errorf("validate language: LexStates[%d].AcceptToken %d is outside SymbolCount %d", i, ls.AcceptToken, symbolCount)
		}
		if ls.Default >= 0 && ls.Default >= numLexStates {
			return fmt.Errorf("validate language: LexStates[%d].Default %d is outside LexStates (len %d)", i, ls.Default, numLexStates)
		}
		if ls.EOF >= 0 && ls.EOF >= numLexStates {
			return fmt.Errorf("validate language: LexStates[%d].EOF %d is outside LexStates (len %d)", i, ls.EOF, numLexStates)
		}
		for j, tr := range ls.Transitions {
			if tr.NextState >= 0 && tr.NextState >= numLexStates {
				return fmt.Errorf("validate language: LexStates[%d].Transitions[%d].NextState %d is outside LexStates (len %d)", i, j, tr.NextState, numLexStates)
			}
		}
	}
	return nil
}

func validateLanguageParseTables(lang *Language) error {
	symbolCount := int(lang.SymbolCount)
	tokenCount := int(lang.TokenCount)
	stateCount := int(lang.StateCount)
	numActions := len(lang.ParseActions)

	validateCell := func(sym int, val uint16) error {
		if val == 0 || symbolCount == 0 {
			return nil
		}
		if sym < 0 || sym >= symbolCount {
			return fmt.Errorf("parse table symbol %d is outside SymbolCount %d", sym, symbolCount)
		}
		if sym < tokenCount {
			if int(val) >= numActions {
				return fmt.Errorf("parse table terminal action index %d is outside ParseActions (len %d)", val, numActions)
			}
			return nil
		}
		if stateCount > 0 && int(val) >= stateCount {
			return fmt.Errorf("parse table goto state %d is outside StateCount %d", val, stateCount)
		}
		return nil
	}

	for si, row := range lang.ParseTable {
		for sym, val := range row {
			if err := validateCell(sym, val); err != nil {
				return fmt.Errorf("validate language: %w (ParseTable state %d)", err, si)
			}
		}
	}

	if len(lang.SmallParseTableMap) > 0 {
		table := lang.SmallParseTable
		if len(table) == 0 {
			return fmt.Errorf("validate language: SmallParseTableMap has %d entries but SmallParseTable is empty", len(lang.SmallParseTableMap))
		}
		for si, offset := range lang.SmallParseTableMap {
			pos := int(offset)
			if pos < 0 || pos >= len(table) {
				return fmt.Errorf("validate language: SmallParseTableMap[%d] offset %d is outside SmallParseTable (len %d)", si, pos, len(table))
			}
			groupCount := int(table[pos])
			pos++
			for i := 0; i < groupCount; i++ {
				if pos+1 >= len(table) {
					return fmt.Errorf("validate language: SmallParseTable group header at offset %d is truncated", pos)
				}
				val := table[pos]
				n := int(table[pos+1])
				pos += 2
				if n < 0 || pos+n > len(table) {
					return fmt.Errorf("validate language: SmallParseTable group symbols at offset %d are truncated", pos)
				}
				for j := 0; j < n; j++ {
					if err := validateCell(int(table[pos+j]), val); err != nil {
						return fmt.Errorf("validate language: %w (SmallParseTableMap[%d])", err, si)
					}
				}
				pos += n
			}
		}
	}

	if stateCount > 0 {
		for key, target := range lang.LargeStateGotos {
			state := int(key >> 32)
			sym := int(uint32(key))
			if state < 0 || state >= stateCount {
				return fmt.Errorf("validate language: LargeStateGotos source state %d is outside StateCount %d", state, stateCount)
			}
			if symbolCount > 0 && (sym < 0 || sym >= symbolCount) {
				return fmt.Errorf("validate language: LargeStateGotos symbol %d is outside SymbolCount %d", sym, symbolCount)
			}
			if target != 0 && int(target) >= stateCount {
				return fmt.Errorf("validate language: LargeStateGotos target state %d is outside StateCount %d", target, stateCount)
			}
		}
	}

	symMetaLen := len(lang.SymbolMetadata)
	symNamesLen := len(lang.SymbolNames)
	for ai, entry := range lang.ParseActions {
		for _, action := range entry.Actions {
			switch action.Type {
			case ParseActionShift, ParseActionRecover:
				if stateCount > 0 && int(action.State) >= stateCount {
					return fmt.Errorf("validate language: ParseActions[%d] shift/recover target state %d is outside StateCount %d", ai, action.State, stateCount)
				}
			case ParseActionReduce:
				if symbolCount > 0 && int(action.Symbol) >= symbolCount {
					return fmt.Errorf("validate language: ParseActions[%d] reduce symbol %d is outside SymbolCount %d", ai, action.Symbol, symbolCount)
				}
				if symMetaLen > 0 && int(action.Symbol) >= symMetaLen {
					return fmt.Errorf("validate language: ParseActions[%d] reduce symbol %d is outside SymbolMetadata (len %d)", ai, action.Symbol, symMetaLen)
				}
				if symNamesLen > 0 && int(action.Symbol) >= symNamesLen {
					return fmt.Errorf("validate language: ParseActions[%d] reduce symbol %d is outside SymbolNames (len %d)", ai, action.Symbol, symNamesLen)
				}
			case ParseActionAccept:
			default:
				return fmt.Errorf("validate language: ParseActions[%d] has an unsupported action type %d", ai, action.Type)
			}
		}
	}

	return nil
}
