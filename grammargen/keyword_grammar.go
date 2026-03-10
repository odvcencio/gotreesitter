package grammargen

// KeywordGrammar returns a simplified language grammar that exercises
// keyword extraction and the word token mechanism. Keywords "var" and
// "return" match the identifier pattern but are promoted to their own
// symbols by the keyword DFA.
func KeywordGrammar() *Grammar {
	g := NewGrammar("keyword_test")

	// program: repeat(_statement)
	g.Define("program", Repeat(Sym("_statement")))

	// _statement: var_declaration | return_statement | expression_statement
	g.Define("_statement", Choice(
		Sym("var_declaration"),
		Sym("return_statement"),
		Sym("expression_statement"),
	))

	// var_declaration: "var" identifier "=" _expression ";"
	g.Define("var_declaration", Seq(
		Str("var"),
		Field("name", Sym("identifier")),
		Str("="),
		Field("value", Sym("_expression")),
		Str(";"),
	))

	// return_statement: "return" _expression ";"
	g.Define("return_statement", Seq(
		Str("return"),
		Field("value", Sym("_expression")),
		Str(";"),
	))

	// expression_statement: _expression ";"
	g.Define("expression_statement", Seq(Sym("_expression"), Str(";")))

	// _expression: identifier | number | _expression "+" _expression
	g.Define("_expression", Choice(
		Sym("identifier"),
		Sym("number"),
		PrecLeft(1, Seq(Sym("_expression"), Str("+"), Sym("_expression"))),
	))

	// identifier: word token pattern
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))

	// number: integer token
	g.Define("number", Token(Repeat1(Pat(`[0-9]`))))

	// Set word token: "identifier" is the word pattern.
	// Keywords "var" and "return" match this pattern and will be
	// promoted from identifier to their keyword symbols.
	g.SetWord("identifier")

	// Extras: whitespace.
	g.SetExtras(Pat(`\s`))

	return g
}
