// Package tablebuild builds the parse tables of a tree-sitter grammar the way
// the tree-sitter generator (release v0.27.1, crates/generate) builds them. It
// is a function-by-function port. Each Go function sits beside a comment that
// names the generator function it ports as "file.rs: fn name", in the order of
// the Rust file, so that the two can be read side by side. The tree-sitter
// generator is MIT-licensed; LICENSE in this folder holds its notice.
//
// The package reads what internal/lexbuild prepares. lexbuild ports the
// generator's front end up to flattening and token expansion; this package
// ports the rest of the prepared grammar:
//
//	prepare_grammar/extract_default_aliases.rs: fn extract_default_aliases (aliases.go)
//	prepare_grammar/process_inlines.rs: fn process_inlines (inlines.go)
//	node_types.rs: fn get_variable_info, fn get_supertype_symbol_map (varinfo.go)
//	prepare_grammar.rs: fn prepare_grammar, the last two steps (prepare.go)
//
// and the canonical LR(1) search that builds the raw parse table:
//
//	build_tables/item.rs: item keys, parse items, lookahead sets, item sets (item.go)
//	build_tables/item_set_builder.rs: fn new, transitive_closure, add_item (closure.go)
//	build_tables/build_parse_table.rs: fn build, add_parse_state, add_actions (parse_table.go)
//	build_tables/build_parse_table.rs: fn handle_conflict, compare_precedence,
//	  get_auxiliary_node_info (conflict.go)
//	build_tables/build_parse_table.rs: fn get_production_id (prodinfo.go)
//	build_tables.rs: fn build_tables up to populate_used_symbols, fn
//	  populate_error_state, fn populate_used_symbols (error_state.go)
//
// The parse table that BuildRawTables returns is the generator's table
// before ActionListPool::intern_table and minimize_parse_table: states in
// the order the search creates them, production infos in the order the
// search first reduces them, and the used symbols.
//
// The passes after the search, without the merge of compatible states and
// without the lex tables:
//
//	tables.rs: struct ActionListId, struct ActionListPool (intern_table,
//	  canonicalize), ParseState::referenced_states,
//	  update_nonterminal_references, remap_terminal_references (action_lists.go)
//	build_tables/minimize_parse_table.rs: fn minimize_parse_table,
//	  remove_unit_reductions (unit_reductions.go), remove_unused_states,
//	  reorder_states_by_descending_size (reorder.go)
//	build_tables.rs: fn build_tables after populate_used_symbols, fn
//	  mark_fragile_tokens (build_tables.go)
//	core::slice::sort::unstable (Rust standard library): the sort of
//	  reorder_states_by_descending_size (width_order.go)
//
// and the numbering of render.rs (layout.go), with a conversion of its result
// into a *gotreesitter.Language (language.go). BuildTables with OptLevel 0
// and Render give the table that the generator writes with
// --disable-optimizations.
//
// The subpackage ref reads the output of the unmodified generator (parser.c,
// parser-noopt.c and the node types file) as canonical records per layer, and
// names the first record where a build differs from it.
//
// # Order
//
// Where the generator iterates a hash map or orders by an interned string id,
// the port iterates in a defined order and the comment at that place says why
// the order cannot change a result. One case reaches a value: the generator
// orders the child types of a variable (node_types.rs: enum ChildType, derived
// Ord) by the intern id of an alias name, that is by the position where the
// grammar reader first met the string. The port orders aliases by name. The
// lists hold the same members; every reader of them (node types, the
// supertype map of render.rs, the production infos of build_parse_table.rs)
// uses them as sets.
//
// # Size limits
//
// The passes of this package are polynomial in the size of the prepared
// grammar, except the expansion of inline rules, which can grow exponentially
// with the number of inlined steps in one production, and the canonical
// LR(1) search, whose state count can grow exponentially with the grammar.
// The inline expansion counts the production steps it builds and the steps
// of its work list. The removal of redundant default aliases compares each
// aliased step with every other production of its rule, so it is quadratic
// in the productions of one rule; it counts its comparisons. The search
// counts the states it creates, the table entries it writes, the items its
// closures insert and the distinct lookahead sets it interns. Each count
// returns a *lexbuild.SizeError before it passes its limit:
//
//	production steps built by inlining: 1,048,576
//	production steps held while inlining one step of one production: 1,048,576
//	steps of the inline work list: 67,108,864
//	steps compared while removing redundant default aliases: 1,073,741,824
//	parse states before merging: 524,288
//	entries of the parse table before merging: 8,388,608
//	item insertions by the closures of the parse states: 268,435,456
//	distinct lookahead sets: 262,144
//
// The limits were not measured over the bundled grammars. The first two equal
// lexbuild's limit on the production steps that flattening keeps. On the
// pinned grammar of the largest reference grammar (116 rules, 7 inlined) the
// values reached are 705, 12, 2,319 and 173 (TestL1 prints them), and the
// search reaches 7,906 states, 160,191 entries, 363,376 closure insertions
// and 309 lookahead sets (TestL2 prints them); each limit of the search is
// more than 50 times that grammar's value. At the limits the entries alone
// take about 600 MB (about 72 bytes each); this is an estimate from the
// type sizes, not a measurement.
package tablebuild
