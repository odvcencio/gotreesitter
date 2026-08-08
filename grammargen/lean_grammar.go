package grammargen

// LeanGrammarReferenceVersion is the Lean release used to define this grammar.
const LeanGrammarReferenceVersion = "4.32.2"

// LeanGrammar returns an editor-oriented grammar for Lean 4 source files.
//
// Lean permits imported modules to extend its parser. A fixed grammar cannot
// reproduce those runtime extensions. This grammar gives stable nodes to core
// declarations. It preserves custom syntax as line-scoped command nodes.
func LeanGrammar() *Grammar {
	g := NewGrammar("lean")

	// A physical newline bounds the generic fallback. Groups can still contain
	// newlines, and later lines remain available as independent command nodes.
	g.Define("source_file", Seq(
		Repeat(Choice(
			Sym("_newline"),
			Seq(Sym("_command"), Sym("_newline")),
		)),
		Optional(Sym("_command")),
	))
	g.Define("_newline", Token(Pat(`\r?\n`)))

	g.Define("_command", Prec(0, Choice(
		Sym("_core_command"),
		Sym("_modified_command"),
		Sym("modifier_command"),
		Sym("_attributed_command"),
		Sym("_attributed_modifier_command"),
		Sym("attribute_command"),
		Sym("javascript_assignment"),
		Sym("custom_command"),
	)))
	g.Define("_core_command", Choice(
		Sym("module_declaration"),
		Sym("import_declaration"),
		Sym("namespace_declaration"),
		Sym("section_declaration"),
		Sym("mutual_declaration"),
		Sym("definition_declaration"),
		Sym("theorem_declaration"),
		Sym("axiom_declaration"),
		Sym("instance_declaration"),
		Sym("structure_declaration"),
		Sym("inductive_declaration"),
		Sym("example_declaration"),
		Sym("deriving_declaration"),
		Sym("syntax_declaration"),
		Sym("directive"),
		Sym("environment_command"),
		Sym("end_command"),
	))
	g.Define("_modified_command", Seq(
		Repeat1(Sym("_declaration_modifier")),
		Sym("_core_command"),
	))
	g.Define("modifier_command", Repeat1(Sym("_declaration_modifier")))
	g.Define("_attributes", Repeat1(Sym("attribute")))
	g.Define("_attributed_command", Seq(
		Sym("_attributes"),
		Repeat(Sym("_declaration_modifier")),
		Choice(
			Sym("_core_command"),
			Sym("custom_command"),
		),
	))
	g.Define("_attributed_modifier_command", Seq(
		Sym("_attributes"),
		Repeat1(Sym("_declaration_modifier")),
	))
	g.Define("attribute_command", Sym("_attributes"))

	g.Define("_declaration_modifier", Choice(
		Str("private"),
		Str("public"),
		Str("protected"),
		Str("noncomputable"),
		Str("partial"),
		Str("unsafe"),
		Str("meta"),
		Str("nonrec"),
		Str("local"),
		Str("scoped"),
	))

	g.Define("module_declaration", Seq(
		Field("kind", Choice(Str("module"), Str("prelude"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("import_declaration", Seq(
		Str("import"),
		Repeat1(Field("module", Sym("identifier"))),
	))

	g.Define("namespace_declaration", Seq(
		Str("namespace"),
		Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("section_declaration", Seq(
		Str("section"),
		Optional(Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier")))),
	))

	g.Define("mutual_declaration", Seq(
		Str("mutual"),
		Repeat(Sym("_command_atom")),
	))

	g.Define("definition_declaration", Seq(
		Field("kind", Choice(Str("def"), Str("abbrev"), Str("opaque"))),
		Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("theorem_declaration", Seq(
		Field("kind", Choice(Str("theorem"), Str("lemma"))),
		Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("axiom_declaration", Seq(
		Field("kind", Choice(Str("axiom"), Str("constant"))),
		Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("instance_declaration", Seq(
		Str("instance"),
		Repeat(Sym("_command_atom")),
	))

	g.Define("structure_declaration", Seq(
		Field("kind", Choice(Str("structure"), Str("class"))),
		Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("inductive_declaration", Seq(
		Field("kind", Choice(Str("inductive"), Str("coinductive"))),
		Field("name", Choice(Sym("identifier"), Sym("antiquoted_identifier"))),
		Repeat(Sym("_command_atom")),
	))

	g.Define("example_declaration", Seq(
		Str("example"),
		Repeat(Sym("_command_atom")),
	))

	g.Define("deriving_declaration", Seq(
		Str("deriving"),
		Repeat(Sym("_command_atom")),
	))

	g.Define("syntax_declaration", Seq(
		Field("kind", Choice(
			Str("syntax"),
			Str("macro"),
			Str("macro_rules"),
			Str("notation"),
			Str("infix"),
			Str("infixl"),
			Str("infixr"),
			Str("prefix"),
			Str("postfix"),
			Str("elab"),
			Str("elab_rules"),
		)),
		Repeat(Sym("_command_atom")),
	))

	g.Define("directive", Seq(
		Field("name", Sym("directive_name")),
		Repeat(Sym("_command_atom")),
	))

	g.Define("environment_command", Seq(
		Field("kind", Choice(
			Str("open"),
			Str("export"),
			Str("variable"),
			Str("variables"),
			Str("include"),
			Str("omit"),
			Str("universe"),
			Str("universes"),
			Str("set_option"),
			Str("attribute"),
			Str("initialize"),
			Str("builtin_initialize"),
			Str("with_weak_namespace"),
			Str("with_exporting"),
			Str("unlock_limits"),
		)),
		Repeat(Sym("_command_atom")),
	))

	g.Define("end_command", Seq(
		Str("end"),
		Repeat(Sym("_command_atom")),
	))

	// The fallback is intentionally line-scoped. It parses one normal atom and
	// then collapses extension-specific syntax into one tail token.
	g.Define("custom_command", Seq(
		Sym("_custom_command_head"),
		Optional(Sym("_custom_command_tail")),
	))
	g.Define("_custom_command_tail", Token(Pat(`[^\r\n]+`)))
	g.Define("_custom_command_head", Choice(
		Sym("parenthesized"),
		Sym("bracketed"),
		Sym("braced"),
		Sym("anonymous_constructor"),
		Sym("string_literal"),
		Sym("character_literal"),
		Sym("numeric_literal"),
		Sym("identifier"),
		Sym("operator"),
		Sym("_closing_delimiter"),
		Sym("unknown_token"),
	))
	// Lean permits multiline strings. Its standard library assigns embedded
	// widget code to a javascript field with this form.
	g.Define("javascript_assignment", Seq(
		Sym("_javascript_assignment_start"),
		Sym("string_literal"),
		Repeat(Sym("_command_atom")),
	))
	g.Define("_javascript_assignment_start", Token(Pat(`javascript[ \t]*:=[ \t]*`)))

	g.Define("_command_atom", Choice(
		Sym("attribute"),
		Sym("parenthesized"),
		Sym("bracketed"),
		Sym("braced"),
		Sym("anonymous_constructor"),
		Sym("string_literal"),
		Sym("character_literal"),
		Sym("numeric_literal"),
		Sym("identifier"),
		Sym("operator"),
		Sym("_closing_delimiter"),
		Sym("unknown_token"),
	))

	g.Define("_group_atom", Choice(
		Sym("_newline"),
		Sym("attribute"),
		Sym("parenthesized"),
		Sym("bracketed"),
		Sym("braced"),
		Sym("anonymous_constructor"),
		Sym("string_literal"),
		Sym("character_literal"),
		Sym("numeric_literal"),
		Sym("identifier"),
		Sym("operator"),
		Sym("unknown_token"),
	))
	g.Define("_closing_delimiter", Choice(
		Str(")"),
		Str("]"),
		Str("}"),
		Str("⟩"),
	))

	g.Define("attribute", Seq(
		Str("@["),
		Repeat(Sym("_group_atom")),
		Str("]"),
	))

	g.Define("parenthesized", Seq(
		Str("("),
		Repeat(Sym("_group_atom")),
		Str(")"),
	))

	g.Define("bracketed", Seq(
		Str("["),
		Repeat(Sym("_group_atom")),
		Str("]"),
	))

	g.Define("braced", Seq(
		Str("{"),
		Repeat(Sym("_group_atom")),
		Str("}"),
	))

	g.Define("anonymous_constructor", Seq(
		Str("⟨"),
		Repeat(Sym("_group_atom")),
		Str("⟩"),
	))

	identifierPart := `(?:[\p{L}\p{Nl}_][\p{L}\p{Nl}\p{Mn}\p{Mc}\p{Nd}\p{No}\p{Pc}!?']*|«[^»]+»)`
	g.Define("identifier", Token(Pat(identifierPart+`(?:\.`+identifierPart+`)*`)))
	g.Define("antiquoted_identifier", Seq(Str("$"), Sym("identifier")))
	g.Define("directive_name", Token(Pat(`#[\p{L}_][\p{L}\p{Nl}\p{Mn}\p{Mc}\p{Nd}\p{No}\p{Pc}!?']*`)))

	g.Define("numeric_literal", Token(Pat(
		`(?:0[xX][0-9A-Fa-f_]+|0[oO][0-7_]+|0[bB][01_]+|[0-9][0-9_]*(?:\.[0-9_]+)?(?:[eE][+\-]?[0-9_]+)?)`,
	)))

	g.Define("string_literal", Token(Seq(
		Optional(Pat(`[\p{L}_][\p{L}\p{Nd}_]*!`)),
		Str("\""),
		Repeat(Choice(
			Pat(`[^"\\]`),
			Seq(Str("\\"), Choice(Pat(`.`), Pat(`[\r\n]`))),
		)),
		Str("\""),
	)))

	g.Define("character_literal", Token(Seq(
		Str("'"),
		Choice(
			Pat(`[^'\\\r\n]`),
			Seq(Str("\\"), Pat(`.`)),
		),
		Str("'"),
	)))

	// Keep quotes out of operators so token antiquotations such as &"term"
	// split into an operator and a string literal.
	operatorChar := Choice(
		Pat(`[!#$%&*+,\-./:;<=>?@\\^|~]`),
		Pat(`[\p{Sm}\p{So}\p{Sk}\p{Sc}\p{Pc}\p{Pd}]`),
		Str("`"),
		Str("·"),
		Str("…"),
	)
	g.Define("operator", Token(Repeat1(operatorChar)))
	g.Define("unknown_token", Token(Prec(-1, Pat(`[^ \t\r\n]`))))

	g.Define("line_comment", Token(Seq(Str("--"), Pat(`[^\r\n]*`))))
	g.SetExternals(
		Sym("block_comment"),
		Sym("doc_comment"),
		Sym("module_doc_comment"),
	)
	g.SetExtras(
		Pat(`[ \t\r]`),
		Sym("line_comment"),
		Sym("block_comment"),
		Sym("doc_comment"),
		Sym("module_doc_comment"),
	)
	g.SetWord("identifier")
	g.EnableLRSplitting = true

	g.Test("definition and theorem", `def double (n : Nat) : Nat := n + n
theorem double_zero : double 0 = 0 := by rfl
`, "")
	g.Test("namespace and structure", `namespace Demo
structure Point where
  x : Nat
  y : Nat
end Demo
`, "")
	g.Test("unicode and notation", `infixl:65 " ⊕ " => Nat.add
def combine (α : Type) (x y : α) := ⟨x, y⟩
`, "")
	g.Test("custom command fallback", `register_option trace.demo : Bool := {
  defValue := false
}
`, "")
	return g
}
