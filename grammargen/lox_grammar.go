package grammargen

// LoxGrammar returns a production-grade Lox grammar (Crafting Interpreters spec).
//
// Implements the full Lox language:
//   - Variables: var x = expr;
//   - Functions: fun name(params) { body }
//   - Classes: class Name < Super { methods }
//   - Control flow: if/else, while, for
//   - Operators: or, and, ==, !=, <, >, <=, >=, +, -, *, /, !, unary -
//   - Calls and property access: f(args), obj.prop, obj.prop = val
//   - Literals: numbers, strings, true, false, nil, this, super
//   - Print: print expr;
//   - Return: return expr;
//   - Comments: // line comments
//   - Block scoping: { statements }
func LoxGrammar() *Grammar {
	g := NewGrammar("lox")

	// ── Program ──
	g.Define("program", Repeat(Sym("_declaration")))

	// ── Declarations ──
	g.Define("_declaration", Choice(
		Sym("class_declaration"),
		Sym("function_declaration"),
		Sym("variable_declaration"),
		Sym("_statement"),
	))

	// class Name (< Super)? { methods }
	g.Define("class_declaration", Seq(
		Str("class"),
		Field("name", Sym("identifier")),
		Optional(Seq(
			Str("<"),
			Field("superclass", Sym("identifier")),
		)),
		Str("{"),
		Repeat(Sym("method_definition")),
		Str("}"),
	))

	g.Define("method_definition", Seq(
		Field("name", Sym("identifier")),
		Str("("),
		Optional(Sym("parameter_list")),
		Str(")"),
		Field("body", Sym("block")),
	))

	// fun name(params) { body }
	g.Define("function_declaration", Seq(
		Str("fun"),
		Field("name", Sym("identifier")),
		Str("("),
		Optional(Sym("parameter_list")),
		Str(")"),
		Field("body", Sym("block")),
	))

	g.Define("parameter_list", Seq(
		Sym("identifier"),
		Repeat(Seq(Str(","), Sym("identifier"))),
	))

	// var name (= expr)?;
	g.Define("variable_declaration", Seq(
		Str("var"),
		Field("name", Sym("identifier")),
		Optional(Seq(
			Str("="),
			Field("value", Sym("_expression")),
		)),
		Str(";"),
	))

	// ── Statements ──
	g.Define("_statement", Choice(
		Sym("expression_statement"),
		Sym("print_statement"),
		Sym("return_statement"),
		Sym("if_statement"),
		Sym("while_statement"),
		Sym("for_statement"),
		Sym("block"),
	))

	g.Define("expression_statement", Seq(
		Sym("_expression"),
		Str(";"),
	))

	g.Define("print_statement", Seq(
		Str("print"),
		Field("value", Sym("_expression")),
		Str(";"),
	))

	g.Define("return_statement", Seq(
		Str("return"),
		Optional(Field("value", Sym("_expression"))),
		Str(";"),
	))

	g.Define("if_statement", PrecRight(0, Seq(
		Str("if"),
		Str("("),
		Field("condition", Sym("_expression")),
		Str(")"),
		Field("consequence", Sym("_statement")),
		Optional(Seq(
			Str("else"),
			Field("alternative", Sym("_statement")),
		)),
	)))

	g.Define("while_statement", Seq(
		Str("while"),
		Str("("),
		Field("condition", Sym("_expression")),
		Str(")"),
		Field("body", Sym("_statement")),
	))

	g.Define("for_statement", Seq(
		Str("for"),
		Str("("),
		Field("initializer", Choice(
			Sym("variable_declaration"),
			Sym("expression_statement"),
			Str(";"),
		)),
		Optional(Field("condition", Sym("_expression"))),
		Str(";"),
		Optional(Field("update", Sym("_expression"))),
		Str(")"),
		Field("body", Sym("_statement")),
	))

	g.Define("block", Seq(
		Str("{"),
		Repeat(Sym("_declaration")),
		Str("}"),
	))

	// ── Expressions (precedence climbing) ──
	// Lox precedence from lowest to highest:
	//   1: assignment (=)
	//   2: or
	//   3: and
	//   4: equality (==, !=)
	//   5: comparison (<, >, <=, >=)
	//   6: term (+, -)
	//   7: factor (*, /)
	//   8: unary (!, -)
	//   9: call, property access

	g.Define("_expression", Choice(
		Sym("assignment_expression"),
		Sym("binary_expression"),
		Sym("unary_expression"),
		Sym("call_expression"),
		Sym("member_expression"),
		Sym("_primary"),
	))

	// assignment: (identifier | member) = expression
	g.Define("assignment_expression", PrecRight(1, Seq(
		Field("left", Choice(
			Sym("identifier"),
			Sym("member_expression"),
		)),
		Str("="),
		Field("right", Sym("_expression")),
	)))

	// Binary operators at various precedence levels
	g.Define("binary_expression", Choice(
		PrecLeft(2, Seq(Field("left", Sym("_expression")), Str("or"), Field("right", Sym("_expression")))),
		PrecLeft(3, Seq(Field("left", Sym("_expression")), Str("and"), Field("right", Sym("_expression")))),
		PrecLeft(4, Seq(Field("left", Sym("_expression")), Str("=="), Field("right", Sym("_expression")))),
		PrecLeft(4, Seq(Field("left", Sym("_expression")), Str("!="), Field("right", Sym("_expression")))),
		PrecLeft(5, Seq(Field("left", Sym("_expression")), Str("<"), Field("right", Sym("_expression")))),
		PrecLeft(5, Seq(Field("left", Sym("_expression")), Str(">"), Field("right", Sym("_expression")))),
		PrecLeft(5, Seq(Field("left", Sym("_expression")), Str("<="), Field("right", Sym("_expression")))),
		PrecLeft(5, Seq(Field("left", Sym("_expression")), Str(">="), Field("right", Sym("_expression")))),
		PrecLeft(6, Seq(Field("left", Sym("_expression")), Str("+"), Field("right", Sym("_expression")))),
		PrecLeft(6, Seq(Field("left", Sym("_expression")), Str("-"), Field("right", Sym("_expression")))),
		PrecLeft(7, Seq(Field("left", Sym("_expression")), Str("*"), Field("right", Sym("_expression")))),
		PrecLeft(7, Seq(Field("left", Sym("_expression")), Str("/"), Field("right", Sym("_expression")))),
	))

	// Unary: ! and - (right-associative, prec 8)
	g.Define("unary_expression", PrecRight(8, Seq(
		Field("operator", Choice(Str("!"), Str("-"))),
		Field("operand", Sym("_expression")),
	)))

	// Call: expr(args)
	g.Define("call_expression", Prec(9, Seq(
		Field("function", Sym("_expression")),
		Str("("),
		Optional(Sym("argument_list")),
		Str(")"),
	)))

	g.Define("argument_list", Seq(
		Sym("_expression"),
		Repeat(Seq(Str(","), Sym("_expression"))),
	))

	// Member access: expr.name
	g.Define("member_expression", Prec(9, Seq(
		Field("object", Sym("_expression")),
		Str("."),
		Field("property", Sym("identifier")),
	)))

	// ── Primaries ──
	g.Define("_primary", Choice(
		Sym("number"),
		Sym("string"),
		Sym("true"),
		Sym("false"),
		Sym("nil"),
		Sym("this"),
		Sym("super_expression"),
		Sym("identifier"),
		Sym("parenthesized_expression"),
	))

	g.Define("true", Str("true"))
	g.Define("false", Str("false"))
	g.Define("nil", Str("nil"))
	g.Define("this", Str("this"))

	// super.method
	g.Define("super_expression", Seq(
		Str("super"),
		Str("."),
		Field("method", Sym("identifier")),
	))

	g.Define("parenthesized_expression", Seq(
		Str("("),
		Sym("_expression"),
		Str(")"),
	))

	// ── Terminals ──
	g.Define("identifier", Token(Seq(
		Pat(`[a-zA-Z_]`),
		Repeat(Pat(`[a-zA-Z0-9_]`)),
	)))

	g.Define("number", Token(Seq(
		Repeat1(Pat(`[0-9]`)),
		Optional(Seq(
			Str("."),
			Repeat1(Pat(`[0-9]`)),
		)),
	)))

	g.Define("string", Token(Seq(
		Str("\""),
		Repeat(Pat(`[^"\\]`)),
		Str("\""),
	)))

	// Comment: // to end of line
	g.Define("comment", Token(Seq(
		Str("//"),
		Repeat(Pat(`[^\n]`)),
	)))

	// ── Word token for keyword disambiguation ──
	g.SetWord("identifier")

	// ── Extras ──
	g.SetExtras(Pat(`\s`), Sym("comment"))

	// ── Conflicts ──
	// call/member vs binary/unary are resolved by precedence.
	// assignment_expression left can be identifier or member_expression,
	// which creates ambiguity with bare identifier in _expression.
	g.SetConflicts(
		[]string{"_expression", "assignment_expression"},
	)

	// ── Embedded tests ──
	g.Test("empty", "", "(program)")
	g.Test("print", "print 42;", "")
	g.Test("var decl", "var x = 1;", "")
	g.Test("var no init", "var y;", "")
	g.Test("assignment", "x = 42;", "")
	g.Test("arithmetic", "1 + 2 * 3;", "")
	g.Test("comparison", "a < b;", "")
	g.Test("equality", "a == b;", "")
	g.Test("logical", "a or b and c;", "")
	g.Test("unary", "!true;", "")
	g.Test("unary neg", "-x;", "")
	g.Test("string", "\"hello\";", "")
	g.Test("nil", "nil;", "")
	g.Test("grouping", "(1 + 2) * 3;", "")
	g.Test("function", "fun add(a, b) { return a + b; }", "")
	g.Test("call", "add(1, 2);", "")
	g.Test("method call", "obj.method(x);", "")
	g.Test("class", "class Dog { bark() { print \"woof\"; } }", "")
	g.Test("inheritance", "class Poodle < Dog { }", "")
	g.Test("this", "this.x;", "")
	g.Test("super", "super.method;", "")
	g.Test("if", "if (x) print x;", "")
	g.Test("if else", "if (x) print x; else print y;", "")
	g.Test("while", "while (true) { print 1; }", "")
	g.Test("for", "for (var i = 0; i < 10; i = i + 1) print i;", "")
	g.Test("block", "{ var x = 1; print x; }", "")
	g.Test("nested calls", "f(g(x));", "")
	g.Test("chained member", "a.b.c;", "")
	g.Test("comment", "// this is a comment\nprint 1;", "")

	return g
}
