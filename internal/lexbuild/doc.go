// Package lexbuild builds the lexer of a tree-sitter grammar the way the
// tree-sitter generator (release v0.27.1, crates/generate) builds it. It is a
// function-by-function port of the lexer path: the grammar front end that
// yields the lexical grammar, the NFA, the lex table construction, and the
// token conflict analysis that decides which parse states share a lex state.
//
// The package reads a grammar.json and the valid tokens of each parse state,
// and returns lex states in C's form. It stands alone: standard library and
// internal/lexexact only. Each function names the generator function it
// ports as "file.rs: fn name".
//
// # Size limits
//
// The generator has no size limits: some inputs of a few hundred bytes ask it
// for billions of automaton states, productions or state sets, and it runs
// until memory is gone. This package is written for the repository's own
// grammars and was not otherwise hardened against hostile input. What holds is
// this, and no more:
//
// 1. Caps on the shape of the input, checked first (ParseGrammar, Prepare and
// the repetition counts of patterns). Each is more than ten times the largest
// value over the 204 bundled grammars that could be measured (the grammars
// move and wat were not measured: the generator writes no reference parser for
// them). A grammar over a cap is refused with a *SizeError that names the cap.
//
// 2. Budgets on the constructions and passes behind the caps, each checked
// before the allocation or the step that would pass it, each returning a
// *SizeError from Prepare, Analyze, BuildUnmerged, BuildLexTables or
// NewFirstLastSets. Each is more than ten times the largest value measured over
// the same grammars.
//
// 3. Within the caps and budgets, the audit of the package (every place where
// the result or the working state can grow faster than the grammar) bounds
// memory by about one gibibyte and the number of simple steps by about
// 3x10^10; both are computed from the limits below, not measured at the limits.
//
// Caps on the shape of the input:
//
//	bytes of grammar.json: 8,388,608
//	rules: 16,384
//	external symbols: 2,048
//	precedence orderings: 512
//	names in one precedence ordering: 256
//	bytes in a name (rule, symbol, field, alias, precedence, external, reserved set): 1,024
//	bytes in a string literal: 2,048
//	bytes in a pattern source: 262,144
//	largest count of a repetition {n} or {n,m} in a pattern: 1,024
//	nesting depth of rules: 256
//	entries in a list of the grammar (supertypes, inline names, conflicts and each conflict, extras, reserved sets and each set, members of a rule): 16,384
//
// Budgets:
//
//	distinct precedence pairs: 2,048
//	steps of the pairing of tokens that share a parse state: 4,294,967,296
//	rule nodes times dropped rules in the removal of unreachable rules: 1,073,741,824
//	rule nodes compared while looking for equal rules: 4,194,304
//	rule nodes evaluated while expanding repeats: 524,288
//	steps of the indirect-recursion search: 32,768
//	bytes of production keys retained while flattening rules: 8,388,608
//	steps of the keyword identification: 4,194,304
//	steps of the grouping of parse states into lex states: 17,179,869,184
//	automaton states visited while closing state sets over splits: 33,554,432
//	steps of the grouping of transitions into character sets: 536,870,912
//	steps of the ordering of lex states: 33,554,432
//	steps of rendering lex states (large character sets and plans): 33,554,432
//	character ranges read into the classes of the patterns of a grammar: 16,777,216
//	expansion steps in the check of inline rules: 200,000
//	symbols copied in the check of inline rules: 16,777,216
//	automaton states in the lexical grammar: 262,144
//	character ranges summed over automaton states: 4,194,304
//	choice paths explored while flattening rules: 131,072
//	production steps retained while flattening rules: 1,048,576
//	steps in one flattened production: 4,096
//	cells of a token-by-token or rule-by-token matrix: 16,777,216
//	state sets in one token-pair conflict search: 131,072
//	automaton states retained by one token-pair conflict search: 2,097,152
//	lex states: 131,072
//	automaton states summed over lex states: 8,388,608
//	character transitions summed over lex states: 1,048,576
//	character ranges summed over lex-state transitions: 8,388,608
//	automaton states summed over the transitions of one lex state: 65,536
//	steps of the comparisons while minimizing a lex table: 134,217,728
//	lex states summed over the single-token constructions: 262,144
//
// Constructs not ported yet return a *NotPortedError.
package lexbuild
