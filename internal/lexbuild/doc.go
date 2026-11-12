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
// # Differences from the generator
//
// The generator has no size limits: some inputs of a few hundred bytes ask it
// for billions of automaton states, productions or state sets, and it runs
// until memory is gone. The port counts what it builds in every place where the
// result or the working state can grow faster than the grammar, and refuses the
// grammar with a *SizeError before it builds the item that passes a limit.
// Prepare, Analyze, BuildUnmerged and BuildLexTables return the error. The
// limits (see budget.go; a test lowers them) are each more than ten times the
// largest value measured over the 204 bundled grammars that could be measured
// (the grammars move and wat were not measured: the generator writes no
// reference parser for them). They are:
//
//	automaton states in the lexical grammar         3,000,000
//	character ranges read into one pattern's classes 4,194,304
//	character ranges summed over automaton states   4,194,304
//	choice paths explored while flattening rules      131,072
//	production steps retained while flattening      1,048,576
//	steps in one flattened production                   4,096
//	cells of a token-by-token or rule-by-token matrix 16,777,216
//	expansion steps / symbols copied, inline check    200,000 / 16,777,216
//	state sets in one token-pair conflict search      131,072
//	automaton states retained by that search        2,097,152
//	lex states in one construction                    131,072
//	automaton states summed over lex states         8,388,608
//	character transitions summed over lex states    1,048,576
//	character ranges summed over lex transitions    8,388,608
//	automaton states over the transitions of a state   65,536
//	state comparisons while minimizing             67,108,864
//	lex states over the single-token constructions    262,144
//
// For example, [ab]*a[ab]{24} has 29 automaton states and 33,554,432 reachable
// lex states. Constructs not ported yet return a *NotPortedError.
package lexbuild
