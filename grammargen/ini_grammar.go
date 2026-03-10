package grammargen

// INIGrammar returns a production-grade INI file grammar.
//
// Parses the superset of major INI dialects (Windows API, Python configparser,
// Git config, PHP parse_ini_file):
//
//   - Sections: [name] and [section "subsection"] (Git-style)
//   - Key-value pairs: key = value, key : value, key=value
//   - Comments: ; and # (full-line only)
//   - Quoted string values: "..." with \" and \\ escapes
//   - Global pairs: key=value before any [section]
//   - Empty values: key= (value is optional)
//
// INI is line-oriented: newlines are significant (not extras).
// Only horizontal whitespace (spaces, tabs) is treated as extras.
func INIGrammar() *Grammar {
	g := NewGrammar("ini")

	// document: repeat(item)
	g.Define("document", Repeat(Sym("_item")))

	g.Define("_item", Choice(
		Sym("section"),
		Sym("pair"),
		Sym("comment"),
		Sym("_newline"),
	))

	// Newline as explicit token (line boundaries matter in INI)
	g.Define("_newline", Token(Repeat1(Pat(`[\r\n]`))))

	// section: header followed by pairs/comments until next section
	g.Define("section", Seq(
		Sym("section_header"),
		Repeat(Sym("_section_body")),
	))

	g.Define("_section_body", Choice(
		Sym("pair"),
		Sym("comment"),
		Sym("_newline"),
	))

	// section_header: "[" name ('"' subsection '"')? "]"
	g.Define("section_header", Seq(
		Str("["),
		Field("name", Sym("section_name")),
		Optional(Seq(
			Str("\""),
			Field("subsection", Sym("subsection_name")),
			Str("\""),
		)),
		Str("]"),
	))

	// section_name: alphanumeric, dots, dashes, slashes, spaces
	g.Define("section_name", Token(Repeat1(Pat(`[a-zA-Z0-9_.\-/ ]`))))

	// subsection_name: anything except newline and unescaped quote
	g.Define("subsection_name", Token(Repeat1(Choice(
		Pat(`[^\"\\\r\n]`),
		Seq(Str("\\"), Pat(`.`)),
	))))

	// pair: key delimiter value?
	g.Define("pair", Seq(
		Field("key", Sym("key")),
		Sym("_delimiter"),
		Optional(Field("value", Sym("_value"))),
	))

	g.Define("_delimiter", Choice(Str("="), Str(":")))

	// key: identifier-like, supports dots and dashes
	g.Define("key", Token(Seq(
		Pat(`[a-zA-Z_]`),
		Repeat(Pat(`[a-zA-Z0-9_.\-]`)),
	)))

	// value: quoted string or bare value (rest of line)
	g.Define("_value", Choice(
		Sym("quoted_string"),
		Sym("bare_value"),
	))

	// quoted_string: "..." with escape sequences
	g.Define("quoted_string", Seq(
		Str("\""),
		Repeat(Choice(
			Sym("_qstring_content"),
			Sym("escape_sequence"),
		)),
		Str("\""),
	))

	g.Define("_qstring_content", ImmToken(Prec(1, Pat(`[^\"\\\r\n]+`))))

	g.Define("escape_sequence", ImmToken(Seq(
		Str("\\"),
		Pat(`[\"\\nrtbfv0]`),
	)))

	// bare_value: rest of line (everything up to newline, not starting with quote).
	// Prec(1) gives bare_value higher lexer priority than key, so when both
	// patterns match the same input (after a delimiter), bare_value wins.
	g.Define("bare_value", Token(Prec(1, Seq(
		Pat(`[^\r\n"]`),
		Repeat(Pat(`[^\r\n]`)),
	))))

	// comment: (; or #) followed by rest of line
	g.Define("comment", Token(Seq(
		Choice(Str(";"), Str("#")),
		Repeat(Pat(`[^\r\n]`)),
	)))

	// Extras: horizontal whitespace ONLY (INI is line-oriented)
	g.SetExtras(Pat(`[ \t]`))

	// Embedded tests
	g.Test("empty", "", "(document)")
	g.Test("simple pair", "key=value", "(document (pair (key) (bare_value)))")
	g.Test("colon delimiter", "key:value", "(document (pair (key) (bare_value)))")
	g.Test("section with pairs", "[server]\nhost=localhost\nport=8080",
		"(document (section (section_header (section_name)) (pair (key) (bare_value)) (pair (key) (bare_value))))")
	g.Test("semicolon comment", "; this is a comment", "(document (comment))")
	g.Test("hash comment", "# hash comment", "(document (comment))")
	g.Test("subsection", "[remote \"origin\"]\nurl=git@github.com:user/repo.git",
		"(document (section (section_header (section_name) (subsection_name)) (pair (key) (bare_value))))")
	g.Test("quoted value", "key=\"hello world\"", "(document (pair (key) (quoted_string)))")
	g.Test("empty value", "key=", "(document (pair (key)))")
	g.Test("global pair", "key=value\n[section]\nother=val",
		"(document (pair (key) (bare_value)) (section (section_header (section_name)) (pair (key) (bare_value))))")

	return g
}
