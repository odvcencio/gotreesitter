package tablebuild

import "github.com/odvcencio/gotreesitter/internal/lexbuild"

// This file ports build_tables.rs: fn build_tables up to populate_used_symbols,
// fn populate_error_state and fn populate_used_symbols.

// RawTables is what build_tables.rs: fn build_tables holds after
// populate_used_symbols: the parse table before the action lists are
// interned and before minimize_parse_table, the parse state info, and the
// lexical analysis (token conflicts, coincident tokens and keywords) that
// populate_error_state read.
type RawTables struct {
	Table       *ParseTable
	StateInfo   *ParseStateInfo
	Analysis    *lexbuild.Analysis
	Diagnostics Diagnostics
	// ClosureEntries is the number of entries that the closures inserted
	// (a work count for the measurements of the pull request).
	ClosureEntries int64
}

// BuildRawTables is build_tables.rs: fn build_tables from its start through
// populate_used_symbols: ItemKeyMap::new, ParseItemSetBuilder::new,
// build_parse_table, then (through lexbuild.Analyze) get_following_tokens,
// TokenConflictMap::new, CoincidentTokenIndex::new and identify_keywords,
// then populate_error_state and populate_used_symbols. variableInfo is the
// result of GetVariableInfo for the same grammar.
func BuildRawTables(g *PreparedGrammar, variableInfo []VariableInfo) (*RawTables, error) {
	return buildRawTables(g, variableInfo, ascendingVariables)
}

func buildRawTables(g *PreparedGrammar, variableInfo []VariableInfo, order closureOrder) (*RawTables, error) {
	sg, lg := g.Syntax, g.Lex
	var keyMap *ItemKeyMap
	if err := guardSize(func() { keyMap = NewItemKeyMap(sg) }); err != nil {
		return nil, err
	}
	var builder *ParseItemSetBuilder
	var berr error
	if err := guardSize(func() { builder, berr = newParseItemSetBuilder(sg, lg, &g.Inlines, keyMap, order) }); err != nil {
		return nil, err
	}
	if berr != nil {
		return nil, berr
	}
	table, info, diags, err := buildParseTable(sg, lg, builder, variableInfo)
	if err != nil {
		return nil, err
	}

	// CoincidentTokenIndex::new reads the terminal entries of every state of
	// the table as built (before the error state is filled in).
	coincident := make([]lexbuild.TokenSet, len(table.States))
	for i := range table.States {
		for _, e := range table.States[i].TerminalEntries {
			coincident[i].Insert(e.Symbol)
		}
	}
	analysis, err := g.Lexer.Analyze(coincident)
	if err != nil {
		return nil, err
	}
	populateErrorState(table, sg, lg, analysis)
	populateUsedSymbols(table, sg, lg)
	return &RawTables{Table: table, StateInfo: info, Analysis: analysis, Diagnostics: diags, ClosureEntries: builder.work}, nil
}

// populateErrorState is build_tables.rs: fn populate_error_state. It gives the
// error state (state 0) a Recover entry for every token that error recovery
// may lex there.
func populateErrorState(table *ParseTable, sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, a *lexbuild.Analysis) {
	state := &table.States[0]
	n := len(lg.Variables)
	keywords := a.Keywords()

	// First identify the conflict-free tokens: tokens that do not overlap with
	// any other token in any way, besides matching exactly the same string.
	var conflictFreeTokens lexbuild.TokenSet
	for i := 0; i < n; i++ {
		conflictsWithOtherTokens := false
		for j := 0; j < n; j++ {
			if j != i && !a.Coincident(lexbuild.TerminalSym(i), lexbuild.TerminalSym(j)) && a.DoesMatchShorterOrLonger(i, j) {
				conflictsWithOtherTokens = true
				break
			}
		}
		if !conflictsWithOtherTokens {
			conflictFreeTokens.Insert(lexbuild.TerminalSym(i))
		}
	}
	conflictFree := conflictFreeTokens.Symbols()

	recoverEntry := func() ParseTableEntry {
		return ParseTableEntry{Actions: []ParseAction{{Kind: ActionRecover}}}
	}

	// Exclude from the error-recovery state any token that conflicts with one
	// of the conflict-free tokens identified above.
	for i := 0; i < n; i++ {
		symbol := lexbuild.TerminalSym(i)
		if !conflictFreeTokens.Contains(symbol) && !keywords.Contains(symbol) && !(sg.HasWord && sg.WordToken == symbol) {
			excluded := false
			for _, t := range conflictFree {
				if !a.Coincident(symbol, t) && a.DoesConflict(int(symbol.Index), int(t.Index)) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}
		state.terminalEntryOrInsert(symbol, recoverEntry())
	}

	for i, externalToken := range sg.ExternalTokens {
		if !externalToken.HasCorresponding {
			state.terminalEntryOrInsert(lexbuild.ExternalSym(i), recoverEntry())
		}
	}

	state.insertTerminal(lexbuild.EndSym(), recoverEntry())
}

// populateUsedSymbols is build_tables.rs: fn populate_used_symbols. It lists
// the symbols that some state uses: the end token, the used terminals in
// index order (the word token moved to slot 1), the used externals, then the
// used non-terminals.
func populateUsedSymbols(table *ParseTable, sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar) {
	terminalUsages := make([]bool, len(lg.Variables))
	nonTerminalUsages := make([]bool, len(sg.Variables))
	externalUsages := make([]bool, len(sg.ExternalTokens))
	for i := range table.States {
		state := &table.States[i]
		for _, e := range state.TerminalEntries {
			switch e.Symbol.Kind {
			case lexbuild.Terminal:
				terminalUsages[e.Symbol.Index] = true
			case lexbuild.External:
				externalUsages[e.Symbol.Index] = true
			}
		}
		for _, e := range state.NonterminalEntries {
			nonTerminalUsages[e.Symbol.Index] = true
		}
	}
	table.Symbols = append(table.Symbols, lexbuild.EndSym())
	for i, used := range terminalUsages {
		if !used {
			continue
		}
		// Assign the grammar's word token a low numerical index, so that a
		// subtree can hold it without heap allocations and can be reassigned
		// to it in place (tree-sitter issue 258).
		if sg.HasWord && sg.WordToken == lexbuild.TerminalSym(i) {
			table.Symbols = append(table.Symbols, Symbol{})
			copy(table.Symbols[2:], table.Symbols[1:])
			table.Symbols[1] = lexbuild.TerminalSym(i)
		} else {
			table.Symbols = append(table.Symbols, lexbuild.TerminalSym(i))
		}
	}
	for i, used := range externalUsages {
		if used {
			table.Symbols = append(table.Symbols, lexbuild.ExternalSym(i))
		}
	}
	for i, used := range nonTerminalUsages {
		if used {
			table.Symbols = append(table.Symbols, lexbuild.NonTerminalSym(i))
		}
	}
}
