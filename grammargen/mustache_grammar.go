package grammargen

// MustacheGrammar returns a production-grade Mustache template grammar.
//
// Implements the required Mustache spec features:
//   - Interpolation: {{ name }}
//   - Unescaped interpolation: {{{ name }}} and {{& name }}
//   - Sections: {{# name }} ... {{/ name }}
//   - Inverted sections: {{^ name }} ... {{/ name }}
//   - Comments: {{! comment text }}
//   - Partials: {{> partial_name }}
//   - Dotted names: {{ person.name }}
//   - Implicit iterator: {{ . }}
//   - Raw text between tags
//
// The grammar treats {{ and }} as delimiters. Text outside tags is raw content.
// The DFA handles {{{ vs {{ disambiguation via maximal munch.
func MustacheGrammar() *Grammar {
	g := NewGrammar("mustache")

	// Template is a sequence of content items
	g.Define("template", Repeat(Sym("_content")))

	g.Define("_content", Choice(
		Sym("interpolation"),
		Sym("unescaped_interpolation"),
		Sym("section"),
		Sym("inverted_section"),
		Sym("comment_tag"),
		Sym("partial"),
		Sym("raw_text"),
	))

	// {{ expression }}
	g.Define("interpolation", Seq(
		Sym("_open_tag"),
		Field("name", Sym("expression")),
		Sym("_close_tag"),
	))

	// {{{ expression }}} or {{& expression }}
	g.Define("unescaped_interpolation", Choice(
		Seq(
			Sym("_open_unescaped"),
			Field("name", Sym("expression")),
			Sym("_close_unescaped"),
		),
		Seq(
			Sym("_open_tag"),
			Str("&"),
			Field("name", Sym("expression")),
			Sym("_close_tag"),
		),
	))

	// {{# name }} content {{/ name }}
	g.Define("section", Seq(
		Sym("_open_tag"),
		Str("#"),
		Field("name", Sym("expression")),
		Sym("_close_tag"),
		Repeat(Sym("_content")),
		Sym("_open_tag"),
		Str("/"),
		Field("close_name", Sym("expression")),
		Sym("_close_tag"),
	))

	// {{^ name }} content {{/ name }}
	g.Define("inverted_section", Seq(
		Sym("_open_tag"),
		Str("^"),
		Field("name", Sym("expression")),
		Sym("_close_tag"),
		Repeat(Sym("_content")),
		Sym("_open_tag"),
		Str("/"),
		Field("close_name", Sym("expression")),
		Sym("_close_tag"),
	))

	// {{! comment text }}
	g.Define("comment_tag", Seq(
		Sym("_open_tag"),
		Str("!"),
		Optional(Field("text", Sym("comment_text"))),
		Sym("_close_tag"),
	))

	g.Define("comment_text", Token(Repeat1(Pat(`[^}]`))))

	// {{> partial_name }}
	g.Define("partial", Seq(
		Sym("_open_tag"),
		Str(">"),
		Field("name", Sym("expression")),
		Sym("_close_tag"),
	))

	// Expression: dotted name or implicit iterator (.)
	g.Define("expression", Choice(
		Sym("dotted_name"),
		Sym("identifier"),
		Sym("implicit_iterator"),
	))

	g.Define("dotted_name", Seq(
		Sym("identifier"),
		Repeat1(Seq(Str("."), Sym("identifier"))),
	))

	g.Define("identifier", Token(Repeat1(Pat(`[a-zA-Z0-9_]`))))

	g.Define("implicit_iterator", Str("."))

	// Delimiters
	g.Define("_open_tag", Token(Str("{{")))
	g.Define("_close_tag", Token(Str("}}")))

	// Triple-brace delimiters for unescaped
	// Prec(1) ensures {{{ is preferred over {{ when both could match
	g.Define("_open_unescaped", Token(Prec(1, Str("{{{"))))
	g.Define("_close_unescaped", Token(Prec(1, Str("}}}"))))

	// Raw text: everything that isn't a tag opener
	// Uses Prec(1) so raw_text is preferred over identifier in text contexts
	g.Define("raw_text", Token(Prec(1, Repeat1(Choice(
		Pat(`[^{]`),
		Seq(Str("{"), Pat(`[^{]`)),
	)))))

	// Extras: whitespace inside tags
	g.SetExtras(Pat(`[ \t\r\n]`))

	// Embedded tests
	g.Test("empty", "", "(template)")
	g.Test("raw text only", "Hello, world!", "")
	g.Test("interpolation", "Hello, {{name}}!", "")
	g.Test("unescaped triple", "Hello, {{{name}}}!", "")
	g.Test("unescaped ampersand", "Hello, {{&name}}!", "")
	g.Test("section", "{{#show}}visible{{/show}}", "")
	g.Test("inverted section", "{{^show}}hidden{{/show}}", "")
	g.Test("comment", "{{! this is a comment }}", "")
	g.Test("partial", "{{>header}}", "")
	g.Test("dotted name", "{{person.name}}", "")
	g.Test("implicit iterator", "{{.}}", "")
	g.Test("mixed content", "Hello {{name}}, you have {{count}} items.", "")

	return g
}
