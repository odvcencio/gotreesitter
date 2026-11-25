package tablebuild

import "github.com/odvcencio/gotreesitter/internal/lexbuild"

// PreparedGrammar is prepare_grammar.rs: struct PreparedGrammar. The string
// pool of the generator has no counterpart: names are held as strings.
type PreparedGrammar struct {
	// Syntax is the syntax grammar. Its steps no longer hold the aliases that
	// became default aliases, and its productions end with the productions that
	// inlining built (the range of each variable in Syntax.VarProds is
	// unchanged), as in the generator.
	Syntax *lexbuild.SyntaxGrammar
	// Lex is the lexical grammar.
	Lex *lexbuild.LexicalGrammar
	// Inlines maps a production and a step to the productions that replace it.
	Inlines InlinedProductionMap
	// DefaultAliases holds the alias of each symbol that always appears aliased.
	DefaultAliases AliasMap
	// Lexer is the lexbuild preparation that Syntax and Lex belong to, for
	// lexbuild.Analyze and the lex tables.
	Lexer *lexbuild.Prepared
}

// Prepare is prepare_grammar.rs: fn prepare_grammar. lexbuild.Prepare runs
// every step up to and including expand_tokens; this function then runs the
// last two steps in the generator's order: extract_default_aliases, then
// process_inlines. The preparation of lexbuild is owned by the result: its
// syntax grammar is modified in place.
func Prepare(grammarJSON []byte) (*PreparedGrammar, error) {
	p, err := lexbuild.Prepare(grammarJSON)
	if err != nil {
		return nil, err
	}
	g := &PreparedGrammar{Syntax: p.Syntax, Lex: p.Lex, Lexer: p}
	if err := guardSize(func() { g.DefaultAliases = extractDefaultAliases(p.Syntax, len(p.Lex.Variables)) }); err != nil {
		return nil, err
	}
	inlines, err := processInlines(p.Syntax, p.Lex)
	if err != nil {
		return nil, err
	}
	g.Inlines = inlines
	return g, nil
}
