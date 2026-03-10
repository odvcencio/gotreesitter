package grammargen

// AliasSuperGrammar returns a grammar that exercises aliases and supertypes.
//
// Supertypes:
//
//	_expression is a supertype with children: number, string, identifier, binary_expression
//
// Aliases:
//
//	In assignment, the left-hand side identifier is aliased to "variable"
//	In binary_expression, the operator string is aliased to "op"
func AliasSuperGrammar() *Grammar {
	g := NewGrammar("alias_super_test")

	// program: repeat(statement)
	g.Define("program", Repeat(Sym("_statement")))

	// _statement: assignment | expression_statement
	g.Define("_statement", Choice(
		Sym("assignment"),
		Sym("expression_statement"),
	))

	// assignment: identifier "=" _expression ";"
	// The identifier child is aliased to "variable" (named alias).
	g.Define("assignment", Seq(
		Field("left", Alias(Sym("identifier"), "variable", true)),
		Str("="),
		Field("right", Sym("_expression")),
		Str(";"),
	))

	// expression_statement: _expression ";"
	g.Define("expression_statement", Seq(
		Sym("_expression"),
		Str(";"),
	))

	// _expression: supertype for expressions
	g.Define("_expression", Choice(
		Sym("number"),
		Sym("string"),
		Sym("identifier"),
		Sym("binary_expression"),
	))

	// binary_expression: _expression op _expression
	// The "+" operator is aliased to "op" (anonymous alias).
	g.Define("binary_expression", PrecLeft(1, Seq(
		Field("left", Sym("_expression")),
		Field("operator", Alias(Str("+"), "op", false)),
		Field("right", Sym("_expression")),
	)))

	// Terminals
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))
	g.Define("number", Token(Repeat1(Pat(`[0-9]`))))
	g.Define("string", Token(Seq(Str(`"`), Repeat(Pat(`[^"\\]`)), Str(`"`))))

	// Extras: whitespace
	g.SetExtras(Pat(`\s`))

	// Supertypes
	g.SetSupertypes("_expression")

	return g
}
