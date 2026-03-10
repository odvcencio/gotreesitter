package grammargen

// CalcGrammar returns a calculator grammar that exercises precedence
// and associativity. It defines:
//   - Binary operators: +, -, *, / with standard math precedence
//   - Unary prefix minus: -x (highest precedence)
//   - Parenthesized expressions: (x)
//   - Integer literals: number
func CalcGrammar() *Grammar {
	g := NewGrammar("calc")

	// program: repeat(expression)
	g.Define("program", Repeat(Sym("expression")))

	// expression: choice of binary ops, unary minus, parens, number
	g.Define("expression", Choice(
		// Binary operators with left-associativity and standard precedence.
		PrecLeft(1, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("+")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(1, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("-")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(2, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("*")),
			Field("right", Sym("expression")),
		)),
		PrecLeft(2, Seq(
			Field("left", Sym("expression")),
			Field("operator", Str("/")),
			Field("right", Sym("expression")),
		)),
		// Unary minus — higher precedence, right-associative so --x works.
		PrecRight(3, Seq(
			Field("operator", Str("-")),
			Field("operand", Sym("expression")),
		)),
		// Parenthesized expression — no precedence needed.
		Seq(Str("("), Sym("expression"), Str(")")),
		// Number literal.
		Sym("number"),
	))

	// number: integer token
	g.Define("number", Token(Repeat1(Pat(`[0-9]`))))

	// Extras: whitespace.
	g.SetExtras(Pat(`\s`))

	return g
}
