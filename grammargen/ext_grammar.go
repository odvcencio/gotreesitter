package grammargen

// ExtScannerGrammar returns a grammar with external scanner tokens.
// It models a simple block-structured language where INDENT and DEDENT
// tokens are produced by an external scanner (like Python).
//
//	program: repeat(statement)
//	statement: simple_statement | block
//	simple_statement: identifier ";"
//	block: identifier ":" NEWLINE INDENT repeat(statement) DEDENT
//
// External tokens: INDENT, DEDENT, NEWLINE
func ExtScannerGrammar() *Grammar {
	g := NewGrammar("ext_test")

	// External tokens declared — the external scanner produces these.
	g.SetExternals(
		Sym("indent"),
		Sym("dedent"),
		Sym("newline"),
	)

	// program: repeat(statement)
	g.Define("program", Repeat(Sym("_statement")))

	// _statement: simple_statement | block
	g.Define("_statement", Choice(
		Sym("simple_statement"),
		Sym("block"),
	))

	// simple_statement: identifier ";"
	g.Define("simple_statement", Seq(
		Field("name", Sym("identifier")),
		Str(";"),
	))

	// block: identifier ":" NEWLINE INDENT repeat(statement) DEDENT
	g.Define("block", Seq(
		Field("name", Sym("identifier")),
		Str(":"),
		Sym("newline"),
		Sym("indent"),
		Field("body", Repeat1(Sym("_statement"))),
		Sym("dedent"),
	))

	// identifier: pattern
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))

	// Extras: whitespace (spaces/tabs only, NOT newlines — scanner handles those).
	g.SetExtras(Pat(`[ \t]`))

	return g
}
