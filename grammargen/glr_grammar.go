package grammargen

// GLRGrammar returns a grammar with intentional ambiguity that requires
// GLR parsing. It models a simplified C-like language where `a * b` can
// be parsed as either multiplication or a pointer declaration:
//
//	expression_statement: a * b ;  (multiplication)
//	pointer_declaration:  a * b ;  (type * name)
//
// The conflict between _expression and type_name is declared, causing the
// parser to fork stacks when it encounters the ambiguity.
func GLRGrammar() *Grammar {
	g := NewGrammar("glr_test")

	// program: repeat(_statement)
	g.Define("program", Repeat(Sym("_statement")))

	// _statement: expression_statement | pointer_declaration
	g.Define("_statement", Choice(
		Sym("expression_statement"),
		Sym("pointer_declaration"),
	))

	// expression_statement: _expression ";"
	g.Define("expression_statement", Seq(Sym("_expression"), Str(";")))

	// _expression: identifier | _expression "*" _expression
	g.Define("_expression", Choice(
		Sym("identifier"),
		PrecLeft(1, Seq(Sym("_expression"), Str("*"), Sym("_expression"))),
	))

	// pointer_declaration: type_name "*" identifier ";"
	g.Define("pointer_declaration", Seq(
		Field("type", Sym("type_name")),
		Str("*"),
		Field("name", Sym("identifier")),
		Str(";"),
	))

	// type_name: identifier
	g.Define("type_name", Sym("identifier"))

	// identifier: /[a-zA-Z_]+/
	g.Define("identifier", Token(Repeat1(Pat(`[a-zA-Z_]`))))

	// Extras: whitespace.
	g.SetExtras(Pat(`\s`))

	// Declare the conflict: _expression and type_name are ambiguous
	// when followed by "*".
	g.SetConflicts([]string{"_expression", "type_name"})

	return g
}
