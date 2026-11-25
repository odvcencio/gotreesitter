package tablebuild

import (
	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports the passes of build_tables.rs: fn build_tables that follow
// minimize_parse_table, in the generator's order:
//
//	build_tables/build_lex_table.rs: fn build_lex_table, the part that reads
//	  the parse table (buildLexTable); internal/lexbuild ports the rest
//	  (merge_token_set, check_token_conflicts, LexTableBuilder,
//	  minimize_lex_table, sort_states and the large character sets)
//	build_tables.rs: fn populate_external_lex_states
//	build_tables.rs: fn mark_fragile_tokens (build_tables.go)
//
// The generator chains the reserved words of a parse state to the keys of its
// terminal entries before it merges token sets (build_lex_table.rs). The
// chain is closed here, not in lexbuild, because the reserved words belong to
// the parse state, which lexbuild does not hold: lexbuild.BuildLexTables
// takes one token set per parse state, and this file builds each set from the
// two sources.

// buildLexTable is build_lex_table.rs: fn build_lex_table. It collects, for
// each parse state in table order, the keys of its terminal entries chained
// with its reserved words, and hands the sets to lexbuild.BuildLexTables,
// which filters them (terminals, with keywords replaced by the word token,
// and the end token), merges them with merge_token_set in the same order,
// builds the main and keyword lex tables, minimizes and sorts the main one.
// It then sets the lex state of each parse state, as the generator does in
// build_lex_table, minimize_lex_table and sort_states.
func buildLexTable(parseTable *InternedTable, g *PreparedGrammar, a *lexbuild.Analysis) (*lexbuild.LexTables, error) {
	tokens := make([]lexbuild.TokenSet, len(parseTable.States))
	for i := range parseTable.States {
		tokens[i] = lexTokens(&parseTable.States[i])
	}
	lex, err := g.Lexer.BuildLexTables(a, tokens)
	if err != nil {
		return nil, err
	}
	for i := range parseTable.States {
		parseTable.States[i].LexStateID = lex.LexState[i]
	}
	return lex, nil
}

// lexTokens is the set that build_lex_table.rs: fn build_lex_table collects
// from one parse state before its filter_map: the keys of the terminal
// entries chained with the reserved words. The generator collects into a
// TokenSet, so the order of the chain reaches no result.
func lexTokens(state *InternedState) lexbuild.TokenSet {
	var tokens lexbuild.TokenSet
	for _, e := range state.TerminalEntries {
		tokens.Insert(e.Symbol)
	}
	for _, s := range state.ReservedWords.Symbols() {
		tokens.Insert(s)
	}
	return tokens
}

// populateExternalLexStates is build_tables.rs: fn
// populate_external_lex_states. External lex state 0 is the empty set; each
// parse state gets the first equal set, or a new one at the end. A terminal
// that an external token corresponds to stands for that external token.
func populateExternalLexStates(parseTable *InternedTable, sg *lexbuild.SyntaxGrammar) {
	// external_tokens_by_corresponding_internal_token: keyed by the token
	// index alone, filled in external token order, so a later external token
	// with the same internal token wins, as in the generator's map. It is read
	// only by key.
	byInternal := map[uint32]int{}
	for i, et := range sg.ExternalTokens {
		if et.HasCorresponding {
			byInternal[et.CorrespondingInternal.Index] = i
		}
	}

	// Ensure that external lex state 0 represents the absence of any external
	// tokens.
	parseTable.ExternalLexStates = append(parseTable.ExternalLexStates, lexbuild.TokenSet{})

	// The generator looks up each set with a linear scan of
	// external_lex_states (Iterator::position). Every set after the first is
	// pushed only when no equal set exists, so the sets are distinct and the
	// first equal set is the only one: a map from the set to its position
	// gives the same id in time linear in the states. The map is read only by
	// key.
	ids := map[string]uint32{}
	for i, set := range parseTable.ExternalLexStates {
		if _, ok := ids[set.Key()]; !ok {
			ids[set.Key()] = uint32(i)
		}
	}
	for i := range parseTable.States {
		state := &parseTable.States[i]
		var externalTokens lexbuild.TokenSet
		for _, e := range state.TerminalEntries {
			token := e.Symbol
			if token.IsExternal() {
				externalTokens.Insert(token)
			} else if token.IsTerminal() {
				if index, ok := byInternal[token.Index]; ok {
					externalTokens.Insert(lexbuild.ExternalSym(index))
				}
			}
		}
		key := externalTokens.Key()
		id, ok := ids[key]
		if !ok {
			parseTable.ExternalLexStates = append(parseTable.ExternalLexStates, externalTokens)
			id = uint32(len(parseTable.ExternalLexStates) - 1)
			ids[key] = id
		}
		state.ExternalLexStateID = id
	}
}
