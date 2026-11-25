package tablebuild

import "github.com/odvcencio/gotreesitter/internal/lexbuild"

// This file ports build_tables.rs: fn build_tables after
// populate_used_symbols (error_state.go ports the part before it), and fn
// mark_fragile_tokens. build_lex_table and populate_external_lex_states are
// not ported yet: the lex state and the external lex state of every parse
// state stay 0, and no external lex state is listed.

// Tables is build_tables.rs: struct Tables, without the lex tables: the
// parse table after minimize_parse_table, mark_fragile_tokens and
// ActionListPool::canonicalize.
type Tables struct {
	ParseTable *InternedTable
	// Raw is the build before ActionListPool::intern_table. Its parse table is
	// moved into ParseTable: Raw.Table holds no states.
	Raw *RawTables
	// Stats counts what each pass did.
	Stats TableStats
}

// TableStats holds counts of the passes after the search (measurements for
// the pull request; the generator logs some of them).
type TableStats struct {
	RawStates      int // states of the raw table
	RawActionLists int // distinct action lists after intern_table
	// The merge (OptMergeStates only): the groups by parse item set core, the
	// groups after the split by states_conflict, the calls of the split by
	// state_successors_differ (the last one splits nothing), the states after
	// merge_compatible_states, and the count of bMergeWork.
	CoreGroups            int
	GroupsAfterConflicts  int
	SuccessorPasses       int
	StatesAfterMerge      int
	MergeWork             int64
	UnitReductionStates   int // states that remove_unit_reductions skips over
	AfterUnitReductions   int // pool lists after remove_unit_reductions
	AfterUnusedStates     int // states after remove_unused_states
	FragileEntries        int // terminal entries that mark_fragile_tokens marks
	CanonicalActionLists  int // pool lists after canonicalize
	StatesAfterMinimizing int // states of the final table
}

// BuildTables is build_tables.rs: fn build_tables up to
// ActionListPool::canonicalize, without the lex tables. With OptMergeStates
// (the generator's default) it builds the parse table of the generator's
// parser.c; with optimizations 0 it builds the table that the generator
// writes with --disable-optimizations.
func BuildTables(g *PreparedGrammar, variableInfo []VariableInfo, optimizations OptLevel) (*Tables, error) {
	raw, err := BuildRawTables(g, variableInfo)
	if err != nil {
		return nil, err
	}
	return finishTables(g, raw, optimizations)
}

// finishTables runs the passes of build_tables.rs: fn build_tables that
// follow populate_used_symbols on a raw build. It moves raw.Table.
func finishTables(g *PreparedGrammar, raw *RawTables, optimizations OptLevel) (*Tables, error) {
	out := &Tables{Raw: raw}
	out.Stats.RawStates = len(raw.Table.States)
	err := catchGeneratorPanic(func() error {
		parseTable := internTable(raw.Table)
		out.ParseTable = parseTable
		out.Stats.RawActionLists = parseTable.ActionLists.Len()
		var minimizeErr error
		if err := guardSize(func() {
			minimizeErr = minimizeParseTable(parseTable, g.Syntax, g.Lex, g.DefaultAliases, raw.Analysis, raw.Analysis.Keywords(), optimizations, &out.Stats)
		}); err != nil {
			return err
		}
		if minimizeErr != nil {
			return minimizeErr
		}
		// build_lex_table and populate_external_lex_states: not ported yet.
		out.Stats.FragileEntries = markFragileTokens(parseTable, raw.Analysis)
		parseTable.ActionLists.canonicalize(parseTable.States)
		out.Stats.CanonicalActionLists = parseTable.ActionLists.Len()
		out.Stats.StatesAfterMinimizing = len(parseTable.States)
		if len(parseTable.States) > 65535 {
			return &StateCountError{len(parseTable.States)}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// catchGeneratorPanic runs f and turns a panic of panicLikeGenerator into
// its *GeneratorPanicError.
func catchGeneratorPanic(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if p, ok := r.(generatorPanic); ok {
				err = p.err
				return
			}
			panic(r)
		}
	}()
	return f()
}

// markFragileTokens is build_tables.rs: fn mark_fragile_tokens. A terminal
// entry is not reusable when another (or the same) valid terminal of its
// state overlaps it (token_conflicts.rs: fn does_overlap). It returns the
// number of entries it marked.
func markFragileTokens(parseTable *InternedTable, a *lexbuild.Analysis) int {
	marked := 0
	var validTerminalIndices []uint32
	for si := range parseTable.States {
		state := &parseTable.States[si]
		validTerminalIndices = validTerminalIndices[:0]
		for _, e := range state.TerminalEntries {
			if e.Symbol.IsTerminal() {
				validTerminalIndices = append(validTerminalIndices, e.Symbol.Index)
			}
		}
		for k := range state.TerminalEntries {
			e := &state.TerminalEntries[k]
			if !e.Symbol.IsTerminal() {
				continue
			}
			for _, i := range validTerminalIndices {
				if a.DoesOverlap(int(i), int(e.Symbol.Index)) {
					if e.ID.Reusable() {
						marked++
					}
					e.ID.setReusable(false)
					break
				}
			}
		}
	}
	return marked
}
