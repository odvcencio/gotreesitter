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
// The port refuses an automaton of more than 3,000,000 states (the generator
// has no bound: b{0,4294967295} runs until it is killed). It also refuses a
// lex table construction that passes a size limit (a *SizeError, returned by
// BuildUnmerged and BuildLexTables): more than 131,072 lex states, more than
// 33,554,432 automaton states summed over lex states, or more than 4,194,304
// character transitions summed over lex states. The pattern [ab]*a[ab]{24}
// has 29 automaton states and 33,554,432 reachable lex states; the generator
// builds them until memory is gone. The 206 bundled grammars need at most
// 11,785 lex states in one construction. Constructs not ported yet return
// a *NotPortedError.
package lexbuild
