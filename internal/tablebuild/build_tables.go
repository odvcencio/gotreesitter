package tablebuild

import (
	"time"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file ports build_tables.rs: fn build_tables after
// populate_used_symbols (error_state.go ports the part before it), and fn
// mark_fragile_tokens. lex.go ports build_lex_table and
// populate_external_lex_states.

// Tables is build_tables.rs: struct Tables: the parse table after
// minimize_parse_table, build_lex_table, populate_external_lex_states,
// mark_fragile_tokens and ActionListPool::canonicalize, and the lex tables.
type Tables struct {
	ParseTable *InternedTable
	// LexTables holds the main and keyword lex tables (Tables::main_lex_table
	// and keyword_lex_table; Keyword is nil without a word token) with the
	// large character sets that render.rs reads, and the lex state of each
	// parse state.
	LexTables *lexbuild.LexTables
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
	// The lex stage: the distinct token sets of the parse states (keywords
	// replaced by the word token), the groups that merge_token_set leaves,
	// the states of the main and keyword lex tables, the external lex
	// states, the count of bFragileWork, and the time of build_lex_table,
	// populate_external_lex_states and mark_fragile_tokens.
	LexTokenSets      int
	LexGroups         int
	MainLexStates     int
	KeywordLexStates  int
	ExternalLexStates int
	FragileWork       int64
	LexTime           time.Duration
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
		lexStart := time.Now()
		lex, err := buildLexTable(parseTable, g, raw.Analysis)
		if err != nil {
			return err
		}
		out.LexTables = lex
		populateExternalLexStates(parseTable, g.Syntax)
		if err := guardSize(func() {
			out.Stats.FragileEntries, out.Stats.FragileWork = markFragileTokens(parseTable, raw.Analysis)
		}); err != nil {
			return err
		}
		out.Stats.LexTime = time.Since(lexStart)
		out.Stats.LexTokenSets, out.Stats.LexGroups = lex.TokenSets, lex.Groups
		out.Stats.MainLexStates = lex.Main.NumStates()
		if lex.Keyword != nil {
			out.Stats.KeywordLexStates = lex.Keyword.NumStates()
		}
		out.Stats.ExternalLexStates = len(parseTable.ExternalLexStates)
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
// state overlaps it (token_conflicts.rs: fn does_overlap). lexbuild's
// Analysis.Fragile is the generator's loop over one state: for each valid
// terminal, the valid terminals in turn until one overlaps it. The loop is
// quadratic in the valid terminals of a state; it charges that square to
// bFragileWork before each state. It returns the number of entries it
// marked and the count of bFragileWork.
func markFragileTokens(parseTable *InternedTable, a *lexbuild.Analysis) (int, int64) {
	marked := 0
	var work int64
	for si := range parseTable.States {
		state := &parseTable.States[si]
		var valid lexbuild.TokenSet
		n := 0
		for _, e := range state.TerminalEntries {
			if e.Symbol.IsTerminal() {
				valid.Insert(e.Symbol)
				n++
			}
		}
		work += int64(n) * int64(n)
		bFragileWork.checkWork(work)
		fragile := a.Fragile(&valid)
		for k := range state.TerminalEntries {
			e := &state.TerminalEntries[k]
			if e.Symbol.IsTerminal() && fragile.Contains(e.Symbol) {
				if e.ID.Reusable() {
					marked++
				}
				e.ID.setReusable(false)
			}
		}
	}
	return marked, work
}
