// Package ref compares a table build with the output of the unmodified
// tree-sitter generator. It writes canonical record lines per layer, hashes
// each line, and names the first record where two record sets differ.
//
// A record line is a list of fields joined by a tab: the layer, the kind of
// record, then the record's key and contents. Names are quoted with
// strconv.Quote, so no field holds a tab or a newline. The section of a line
// is its layer and kind ("L4 state"). Within a section the order of the lines
// is the order of the generator's numbering; sections are compared in sorted
// order.
//
// Layers (the oracle of the faithful-tables design):
//
//	L0 input           grammar.json: each rule in file order, each member of
//	                   the other top-level lists ("$schema" is not read)
//	L1 prepared        symbol names and metadata as a set, field names, alias
//	                   sequences and field maps as sets, the non-terminal alias
//	                   map, supertypes, node-types.json entries
//	L2 numbering       symbols in id order, MAX_ALIAS_SEQUENCE_LENGTH, the
//	                   alias sequence and field map of each production id, the
//	                   terminal cells of the error state
//	L3 tables          parser-noopt.c: counts, every cell of every state, the
//	                   primary state ids
//	L4 tables          parser.c: the same, and ts_symbol_map
//	L5 lex             per parse state the lex state, external lex state,
//	                   reserved word set and the reusable flag of each terminal
//	                   cell; every state of the main and keyword lexers with
//	                   its ranges in first-match order merged; reserved words;
//	                   external tokens and their states
//	L6 language        one digest per exported field of the Language
//
// A digest is the first 8 bytes of the SHA-256 of a record line, in hex. A
// reference that is stored as digests can name the first differing record and
// print the port's line; a reference with full lines also prints its own line
// and the first differing field.
package ref
