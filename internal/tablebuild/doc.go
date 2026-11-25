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
// with the number of inlined steps in one production. That expansion counts
// the production steps it builds and the steps of its work list. The removal
// of redundant default aliases compares each aliased step with every other
// production of its rule, so it is quadratic in the productions of one rule;
// it counts its comparisons. Each count returns a *lexbuild.SizeError before
// it passes its limit:
//
//	production steps built by inlining: 1,048,576
//	production steps held while inlining one step of one production: 1,048,576
//	steps of the inline work list: 67,108,864
//	steps compared while removing redundant default aliases: 1,073,741,824
//
// The limits were not measured over the bundled grammars. The first two equal
// lexbuild's limit on the production steps that flattening keeps. On the
// pinned grammar of the largest reference grammar (116 rules, 7 inlined) the
// values reached are 705, 12, 2,319 and 173; TestL1 prints them.
package tablebuild
