package lexexact

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// tableWith returns a header table that holds the known headers and the given
// synthetic one, for a test that calls parseWith.
func tableWith(text string, h header) map[string]header {
	t := map[string]header{}
	for k, v := range knownHeaders {
		t[k] = v
	}
	t[headerHash(text)] = h
	return t
}

// factsOf reads the facts of an entry from the macro section of a header.
func factsOf(h string) header {
	sec := macroSection(h)
	return header{
		eofDeclared: strings.Contains(sec, "bool eof = false;"),
		eofSet:      strings.Contains(sec, "eof = lexer->eof(lexer);"),
		hasMap:      strings.Contains(sec, "#define ADVANCE_MAP"),
		hasSet:      strings.Contains(sec, "bool set_contains("),
	}
}

// macroSection returns the part of a parser.h that holds the lexer macros and
// set_contains. It is a reading aid for a person who adds an entry to
// knownHeaders; production code does not use it.
func macroSection(h string) string {
	var b strings.Builder
	if a := strings.Index(h, "static inline bool set_contains"); a >= 0 {
		if e := strings.Index(h[a:], "\n}\n"); e >= 0 {
			b.WriteString(h[a : a+e+3])
			b.WriteString("\n")
		}
	}
	i := strings.Index(h, "#ifdef _MSC_VER")
	if k := strings.Index(h, "#define START_LEXER"); i < 0 || k < i {
		i = k
	}
	if j := strings.Index(h, "#define END_STATE"); i >= 0 && j > i {
		if e := strings.Index(h[j:], "\n"); e >= 0 {
			b.WriteString(h[i : j+e+1])
		}
	}
	return b.String()
}

func TestMacroSectionReadingAid(t *testing.T) {
	sec := macroSection(StandardHeader)
	for _, want := range []string{"static inline bool set_contains", "#define START_LEXER", "#define ADVANCE_MAP", "#define END_STATE"} {
		if !strings.Contains(sec, want) {
			t.Errorf("macro section lacks %q", want)
		}
	}
}

func TestStandardHeaderIsKnown(t *testing.T) {
	h, err := parseHeader(StandardHeader)
	if err != nil {
		t.Fatal(err)
	}
	if !h.eofDeclared || h.eofSet || !h.hasMap || !h.hasSet {
		t.Errorf("facts = %+v", h)
	}
	if want := factsOf(StandardHeader); h != want {
		t.Errorf("entry %+v differs from the facts read from the header: %+v", h, want)
	}
	if got := headerHash(StandardHeader)[:12]; got != "180b893c8734" {
		t.Errorf("StandardHeader hash prefix = %s, want 180b893c8734", got)
	}
	// Carriage returns do not change the identity.
	if _, err := parseHeader(strings.ReplaceAll(StandardHeader, "\n", "\r\n")); err != nil {
		t.Errorf("CRLF copy: %v", err)
	}
}

// Changing one byte of an accepted header refuses it, and the error names the hash and the list.
func TestHeaderOneByteChangeIsRefused(t *testing.T) {
	for _, i := range []int{0, len(StandardHeader) / 2, len(StandardHeader) - 1} {
		b := []byte(StandardHeader)
		b[i] ^= 1
		_, err := parseHeader(string(b))
		if err == nil || !strings.Contains(err.Error(), headerHash(string(b))) || !strings.Contains(err.Error(), "knownHeaders") {
			t.Errorf("byte %d: err = %v", i, err)
		}
	}
	if _, err := parseHeader(StandardHeader + " "); err == nil {
		t.Error("an appended space was accepted")
	}
}

// Entries that differ in what the header defines change what Parse accepts.
func TestHeaderEntryFacts(t *testing.T) {
	body := "case 0:\n if (eof) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"
	noEOFLine := strings.Replace(cfile("", body), "  eof = lexer->eof(lexer);\n", "", 1)
	step := func(text string, h header) string {
		src, err := parseWith(noEOFLine, text, tableWith(text, h))
		if err != nil {
			return "error: " + err.Error()
		}
		return stepOf(src.Main, 0, true, 0)
	}
	v := func(n string) string { return StandardHeader + "\n/* variant " + n + " */\n" }
	if got := step(v("1"), header{eofDeclared: true, eofSet: true, hasMap: true, hasSet: true}); got != "adv>1" {
		t.Errorf("eof set in the macro: %s", got)
	}
	if got := step(v("2"), header{eofDeclared: true, hasMap: true, hasSet: true}); got != "none" {
		t.Errorf("eof never set: %s", got)
	}
	if got := step(v("3"), header{hasMap: true, hasSet: true}); !strings.Contains(got, "eof") {
		t.Errorf("undeclared eof: %s", got)
	}
	mapBody := cfile("", "case 0:\n ADVANCE_MAP('a', 0);\n END_STATE();")
	if _, err := parseWith(mapBody, v("4"), tableWith(v("4"), header{eofDeclared: true, hasSet: true})); err == nil || !strings.Contains(err.Error(), "ADVANCE_MAP") {
		t.Errorf("map without the macro: %v", err)
	}
	extra := "static const TSCharacterRange s1[] = { {'a', 'z'}, };\n"
	setBody := cfile(extra, "case 0:\n if (set_contains(s1, 1, lookahead)) ADVANCE(0);\n END_STATE();")
	if _, err := parseWith(setBody, v("5"), tableWith(v("5"), header{eofDeclared: true, hasMap: true})); err == nil || !strings.Contains(err.Error(), "set_contains") {
		t.Errorf("set_contains without the helper: %v", err)
	}
	if _, err := parseInc(setBody, StandardHeader); err != nil {
		t.Errorf("set_contains with the known header: %v", err)
	}
}

// A header with a carriage return that is not part of a line end is refused,
// also when its other bytes are those of a known header.
func TestHeaderLoneCarriageReturnIsRefused(t *testing.T) {
	i := strings.Index(StandardHeader, "return result;")
	h := StandardHeader[:i+len("return")] + "\r" + StandardHeader[i+len("return"):]
	if _, err := parseHeader(h); err == nil || !strings.Contains(err.Error(), "carriage return") {
		t.Errorf("err = %v", err)
	}
}

// The first reproducer about set_contains: a renamed helper next to a non-inline set_contains.
func TestSetContainsOtherImplementationIsRefused(t *testing.T) {
	other := strings.Replace(StandardHeader, "static inline bool set_contains(const TSCharacterRange *ranges, uint32_t len, int32_t lookahead) {",
		"static bool set_contains(TSCharacterRange *ranges, uint32_t len, int32_t lookahead) { return true; }\nstatic inline bool set_contains_old(TSCharacterRange *ranges, uint32_t len, int32_t lookahead) {", 1)
	if other == StandardHeader {
		t.Fatal("the replacement did not apply")
	}
	extra := "static const TSCharacterRange s1[] = { {'a', 'z'}, };\n"
	c := cfile(extra, "case 0:\n if ((!eof && set_contains(s1, 1, lookahead))) ADVANCE(0);\n END_STATE();")
	if _, err := parseInc(c, other); err == nil || !strings.Contains(err.Error(), "known headers") {
		t.Errorf("err = %v", err)
	}
}

// Reproducers: each text below is an accepted header with one change that makes
// a lexer macro or set_contains mean something else (or be inactive). The
// change alters the bytes, so the header is not in the list and is refused.
func TestChangedHeadersAreRefused(t *testing.T) {
	adv := "#define ADVANCE(state_value) \\\n  {                          \\\n    state = state_value;     \\\n    goto next_state;         \\\n  }\n"
	if !strings.Contains(StandardHeader, adv) {
		t.Fatal("the ADVANCE text of StandardHeader changed")
	}
	set := StandardHeader[strings.Index(StandardHeader, "static inline bool set_contains"):]
	set = set[:strings.Index(set, "\n}\n")+3]
	noAdv := strings.Replace(StandardHeader, adv, "static inline void ADVANCE(int value) { (void)value; }\n", 1)
	other := "#define ADVANCE(state_value) { state = 2; goto next_state; }\n"
	lastEndif := strings.LastIndex(noAdv, "#endif")
	cases := map[string]string{
		"definition under #if 0, then an active one": strings.Replace(StandardHeader, adv, "#if 0\n"+adv+"#endif\n"+other, 1),
		"definition twice":                           strings.Replace(StandardHeader, adv, adv+other, 1),
		"definition under #ifdef":                    strings.Replace(StandardHeader, adv, "#ifdef OTHER\n"+adv+"#endif\n", 1),
		"#undef after the definition":                strings.Replace(StandardHeader, adv, adv+"#undef ADVANCE\n", 1),
		"#undef with a line comment":                 noAdv + "#undef ADVANCE // use the function\n",
		"#if 0 definition after the include guard":   strings.Replace(StandardHeader, adv, "", 1) + "\nstatic inline void ADVANCE(int value) { (void)value; }\n\n#if 0\n" + adv + "#endif\n",
		"definition in the include guard's #else":    noAdv[:lastEndif] + "#else\n" + other + noAdv[lastEndif:],
		"definition in the include guard's #elif":    noAdv[:lastEndif] + "#elif 1\n" + other + noAdv[lastEndif:],
		"line splice in #undef and #define":          noAdv + "#un\\\ndef ADVANCE\n#def\\\nine ADVANCE(state_value) { state = 2; goto next_state; }\n",
		"strings that hold comment delimiters": strings.Replace(StandardHeader, adv, "static inline void ADVANCE(int value) { (void)value; }\n"+
			"static const char *a = \"/*\";\n#if 0\nstatic const char *b = \"*/\";\n"+adv+"static const char *c = \"/*\";\n#endif\nstatic const char *d = \"*/\";\n", 1),
		"macro parameter list ignored":             strings.Replace(StandardHeader, adv, "enum { state_value = 2 };\n#define ADVANCE(ignored) { state = state_value; goto next_state; }\n", 1),
		"include of another file":                  StandardHeader + "#include \"override.h\"\n",
		"push_macro and pop_macro":                 strings.Replace(StandardHeader, adv, "#pragma push_macro(\"ADVANCE\")\n"+adv, 1) + "#pragma pop_macro(\"ADVANCE\")\nstatic inline void ADVANCE(int value) { (void)value; }\n",
		"true redefined":                           StandardHeader + "#undef true\n#define true false\n",
		"set_contains as a macro":                  StandardHeader + "#define set_contains(ranges, len, lookahead) false\n",
		"set_contains macro under #if 1":           StandardHeader + "#if 1\n#define set_contains(ranges, len, lookahead) false\n#endif\n",
		"set_contains defined, undefined, defined": StandardHeader + "#define set_contains(ranges, len, lookahead) true\n#undef set_contains\n#define set_contains(ranges, len, lookahead) false\n",
		"set_contains is another function":         strings.Replace(StandardHeader, set, "static inline int set_contains(TSCharacterRange *r,uint32_t n,int32_t c){return 0;}\n// "+strings.ReplaceAll(set, "\n", " ")+"\n", 1),
		"set_contains text held in a string":       strings.Replace(StandardHeader, set, "static inline int set_contains(TSCharacterRange *r,uint32_t n,int32_t c){return 0;}\nstatic const char *verification = \""+strings.ReplaceAll(strings.ReplaceAll(set, "\n", " "), "\"", "")+"\";\n", 1),
		"set_contains removed":                     strings.Replace(StandardHeader, set, "", 1),
		"START_LEXER without the eof declaration":  strings.Replace(StandardHeader, "  bool eof = false;             \\\n", "", 1),
	}
	for name, h := range cases {
		if h == StandardHeader {
			t.Errorf("%s: the change did not apply", name)
			continue
		}
		_, err := parseHeader(h)
		if err == nil || !strings.Contains(err.Error(), "not in the list of known headers") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// Reproducers for parser.c: each text is placed before the lex function of a
// small parser.c and must be refused by the directive rule.
func TestChangedParserCIsRefused(t *testing.T) {
	body := "case 0:\n if (lookahead == 'a') ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"
	good := cfile("", body)
	if _, err := parseInc(good, StandardHeader); err != nil {
		t.Fatalf("control: %v", err)
	}
	// Directives that the generator writes, and strings that hold comment
	// delimiters or '#', are accepted.
	okTop := "#include \"tree_sitter/parser.h\"\n#ifdef _MSC_VER\n#pragma optimize(\"\", off)\n#elif defined(__clang__)\n#pragma clang optimize off\n#elif defined(__GNUC__)\n#pragma GCC optimize (\"O0\", \"jump-tables\")\n#endif\n#if defined(__GNUC__) || defined(__clang__)\n#pragma GCC diagnostic ignored \"-Wmissing-field-initializers\"\n#endif\n#define LANGUAGE_VERSION 14\n/* generated */\nstatic const char * const ts_symbol_names[] = {\"/*\", \"*/\", \"#define ADVANCE\"};\n"
	if _, err := parseInc(okTop+good, StandardHeader); err != nil {
		t.Errorf("generator directives and delimiter strings: %v", err)
	}
	cases := map[string]string{
		"ADVANCE redefined":                "#define ADVANCE(state_value) { state = 2; goto next_state; }\n",
		"ADVANCE undefined":                "#undef ADVANCE\n",
		"undef with a line comment":        "#undef ADVANCE // use the function\n",
		"line splice in undef and define":  "#un\\\ndef ADVANCE\n#def\\\nine ADVANCE(state_value) { state = 2; goto next_state; }\n",
		"digraph define":                   "%:define ADVANCE(state_value) { state = 2; goto next_state; }\n",
		"true redefined":                   "#undef true\n#define true false\n",
		"set_contains as a macro":          "#define set_contains(ranges, len, lookahead) false\n",
		"include of another file":          "#include \"override.h\"\n",
		"push_macro":                       "#pragma push_macro(\"ADVANCE\")\n",
		"pop_macro":                        "#pragma pop_macro(\"ADVANCE\")\n",
		"eof redefined":                    "#define eof 0\n",
		"lookahead redefined":              "#define lookahead 0\n",
		"state redefined":                  "#define state 0\n",
		"result redefined":                 "#define result 1\n",
		"lexer redefined":                  "#define lexer other\n",
		"#if 0 hides a definition":         "#if 0\n#define ADVANCE(state_value) { state = state_value; goto next_state; }\n#endif\n",
		"#else branch":                     "#ifdef _WIN32\n#else\n#endif\n#else\n",
		"text inside a conditional":        "#ifdef _WIN32\nstatic int hidden;\n#endif\n",
		"unknown count macro":              "#define ADVANCE_COUNT 3\n",
		"directive after a string with /*": "static const char *a = \"/*\";\n#undef ADVANCE\nstatic const char *b = \"*/\";\n",
		"directive after a comment":        "/* x */ #undef ADVANCE\n",
		"directive that carries a comment": "#endif /* x */\n",
		"unterminated conditional":         "#ifdef _WIN32\n",
	}
	for name, top := range cases {
		_, err := parseInc(top+good, StandardHeader)
		if err == nil || !strings.HasPrefix(err.Error(), "parser.c:") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// A directive inside a lex function is refused too (the function reader stops at it).
	inside := strings.Replace(good, "\n case 1:", "\n#undef ADVANCE\n case 1:", 1)
	if _, err := parseInc(inside, StandardHeader); err == nil {
		t.Errorf("directive inside the function: err = %v", err)
	}
}

// The reading of parser.c: constructions where a comment, a string, a line
// ending, a trigraph, an item outside the generator's kinds, or deep nesting
// would make a regex or a partial scan read something other than C does. Each
// must be refused, with the reason in the error.
func TestParserCConstructionsAreRefused(t *testing.T) {
	lex := func(body string) string {
		return "static bool ts_lex(TSLexer *lexer, TSStateId state) {\n  START_LEXER();\n  eof = lexer->eof(lexer);\n  switch (state) {\n" + body + "\n    default:\n      return false;\n  }\n}\n"
	}
	real := lex("    case 0:\n      ACCEPT_TOKEN(sym_a);\n      if (eof) ADVANCE(1);\n      END_STATE();\n    case 1:\n      END_STATE();")
	fake := strings.ReplaceAll(lex("    case 0:\n      ACCEPT_TOKEN(sym_b);\n      END_STATE();"), "\n", " ")
	enum := "enum ts_symbol_identifiers { sym_a = 1, sym_b = 2, };\n"
	aux := "if (aux(lookahead)) ADVANCE(1);"
	deep := func(n int) string {
		return "if (" + strings.Repeat("(", n) + "lookahead == 'a'" + strings.Repeat(")", n) + ") ADVANCE(1);"
	}
	cases := []struct{ name, c, want string }{
		{"string holds an enum", "static const char *n = \"enum ts_symbol_identifiers { sym_a = 2, sym_b = 1, }\";\n" + enum + real, "not one the generator writes"},
		{"commented-out set next to a real set of another shape", "// static const TSCharacterRange s1[] = {{'a','z'}};\nstatic TSCharacterRange const s1[] = {{'0','9'}};\n" + enum + lex("case 0:\n if (set_contains(s1, 1, lookahead)) ADVANCE(0);\n END_STATE();"), "not one the generator writes"},
		{"second ts_lex", enum + real + real, "ts_lex"},
		{"second symbol enum", enum + enum + real, "defined twice"},
		{"lone CR before a directive", enum + "static int pad;\r#undef ADVANCE\r#define ADVANCE(state_value) { state = 2; goto next_state; }\n" + real, "carriage return"},
		{"NUL before a directive", enum + "\x00#undef ADVANCE\n\x00#define ADVANCE(state_value) { state = 2; goto next_state; }\n" + real, "NUL"},
		{"trigraph directive", enum + "??=undef ADVANCE\n??=define ADVANCE(state_value) { state = 2; goto next_state; }\n" + real, "trigraph"},
		{"lone CR ends a comment", enum + strings.Replace(real, "case 0:", "case 0:\n // x\r ACCEPT_TOKEN(sym_b);", 1), "carriage return"},
		{"trigraph folds a step into a comment", enum + strings.Replace(real, "case 0:", "case 0:\n // x ??/\n if (eof) ADVANCE(1);", 1), "trigraph"},
		{"backslash-newline in a comment", enum + strings.Replace(real, "case 0:", "case 0:\n // x \\\n ACCEPT_TOKEN(sym_b);", 1), "backslash-newline"},
		{"helper prototype, redefinition after the lexer", "static inline bool aux(int32_t c);\n" + enum + lex("case 0:\n "+aux+"\n END_STATE();\n case 1:\n END_STATE();") + "#undef true\n#define true 0\nstatic inline bool aux(int32_t c) { return c == 'a' || true; }\n", "not one the generator writes"},
		{"helper defined after the lexer that uses it", enum + lex("case 0:\n "+aux+"\n END_STATE();\n case 1:\n END_STATE();") + "static inline bool aux(int32_t c) { return c == 'a'; }\n", "not defined before"},
		{"set defined after the lexer that uses it", enum + lex("case 0:\n if (set_contains(s1, 1, lookahead)) ADVANCE(0);\n END_STATE();") + "static const TSCharacterRange s1[] = {{'a','z'}};\n", "not defined before"},
		{"tentative set definition", "static const TSCharacterRange s1[1];\n" + enum + real, "not one the generator writes"},
		{"typedef", "typedef int myint;\n" + enum + real, "not one the generator writes"},
		{"set with a declared size", "static const TSCharacterRange s1[1] = {{'a','z'}};\n" + enum + real, "not one the generator writes"},
		{"set defined twice", "static const TSCharacterRange s1[] = {{'a','z'}};\nstatic const TSCharacterRange s1[] = {{'0','9'}};\n" + enum + real, "defined twice"},
		{"nesting of parentheses in a condition", enum + lex("case 0:\n "+deep(300)+"\n END_STATE();\n case 1:\n END_STATE();"), "deeper than 256"},
		{"deep braces in a table", "static const TSSymbol ts_symbol_map[] = " + strings.Repeat("{", 300) + strings.Repeat("}", 300) + ";\n" + enum + real, "nesting deeper"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseInc(c.c, StandardHeader)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
	// Text inside a comment is not code: the real definitions are the ones read.
	for name, c := range map[string]string{
		"line comment":  enum + "// " + fake + "\n" + real,
		"block comment": enum + "/* " + fake + " */\n" + real,
	} {
		src, err := parseInc(c, StandardHeader)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if a, _ := src.Main.Accept(0); a != 1 || stepOf(src.Main, 0, true, 0) != "adv>1" {
			t.Errorf("%s: read the commented-out function: accept %d, step %s", name, a, stepOf(src.Main, 0, true, 0))
		}
	}
	helper := "// static inline bool aux(int32_t c) { return true; }\nstatic inline bool aux(int32_t c) /* x */ { return c == 'b'; }\n"
	src, err := parseInc(helper+enum+lex("case 0:\n "+aux+"\n END_STATE();\n case 1:\n END_STATE();"), StandardHeader)
	if err != nil {
		t.Fatalf("helper next to a commented-out helper: %v", err)
	}
	if got := stepOf(src.Main, 0, false, 'b'); got != "adv>1" {
		t.Errorf("real helper: step on 'b' = %s", got)
	}
	if got := stepOf(src.Main, 0, false, 'a'); got != "none" {
		t.Errorf("commented-out helper was read: step on 'a' = %s", got)
	}
	src, err = parseInc(enum+real+"// "+strings.ReplaceAll(fake, "ts_lex(", "ts_lex_keywords(")+"\n", StandardHeader)
	if err != nil || src.Keyword != nil {
		t.Errorf("commented-out keyword lexer: err = %v, keyword = %v", err, src != nil && src.Keyword != nil)
	}
	// Nesting just inside the limit is read.
	ok := enum + lex("case 0:\n "+deep(100)+"\n END_STATE();\n case 1:\n END_STATE();")
	if _, err := parseInc(ok, StandardHeader); err != nil {
		t.Errorf("nesting 100: %v", err)
	}
	// The control reads as expected.
	src, err = parseInc(enum+real, StandardHeader)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := src.Main.Accept(0); a != 1 || stepOf(src.Main, 0, true, 0) != "adv>1" {
		t.Errorf("control: accept %d, step %s", a, stepOf(src.Main, 0, true, 0))
	}
}

const goodTail = `static const TSLexMode ts_lex_modes[STATE_COUNT] = { [0] = {.lex_state = 0} };
static const uint16_t ts_parse_table[LARGE_STATE_COUNT][SYMBOL_COUNT] = { [0] = {[ts_builtin_sym_end] = ACTIONS(1)} };
void *tree_sitter_demo_external_scanner_create(void);
void tree_sitter_demo_external_scanner_destroy(void *);
bool tree_sitter_demo_external_scanner_scan(void *, TSLexer *, const bool *);
unsigned tree_sitter_demo_external_scanner_serialize(void *, char *);
void tree_sitter_demo_external_scanner_deserialize(void *, const char *, unsigned);
#ifdef __cplusplus
extern "C" {
#endif
#ifdef TREE_SITTER_HIDE_SYMBOLS
#define TS_PUBLIC
#elif defined(_WIN32)
#define TS_PUBLIC __declspec(dllexport)
#else
#define TS_PUBLIC __attribute__((visibility("default")))
#endif
TS_PUBLIC const TSLanguage *tree_sitter_demo(void) {
  static const TSLanguage language = { .version = LANGUAGE_VERSION, 1.0, u8"x" };
  return &language;
}
#ifdef __cplusplus
}
#endif
`

// The rule covers the whole file: the items that follow the lex functions are
// matched whole as well.
func TestWholeFileRule(t *testing.T) {
	enum := "enum ts_symbol_identifiers { sym_a = 1, sym_b = 2, };\n"
	lex := func(body string) string {
		return "static bool ts_lex(TSLexer *lexer, TSStateId state) {\n  START_LEXER();\n  eof = lexer->eof(lexer);\n  switch (state) {\n" + body + "\n    default:\n      return false;\n  }\n}\n"
	}
	real := lex("    case 0:\n      ACCEPT_TOKEN(sym_a);\n      END_STATE();")
	kw := strings.Replace(real, "ts_lex(", "ts_lex_keywords(", 1)
	head := "#define STATE_COUNT 2\n#define LARGE_STATE_COUNT 1\n#define SYMBOL_COUNT 3\n#define LANGUAGE_VERSION 14\n"
	if _, err := parseInc(head+enum+real+goodTail, StandardHeader); err != nil {
		t.Fatalf("a file with the generator's tail: %v", err)
	}
	src, err := parseInc(head+enum+real+kw+goodTail, StandardHeader)
	if err != nil || src.Keyword == nil {
		t.Fatalf("keyword lexer before the tail: err = %v", err)
	}
	cases := []struct{ name, c, want string }{
		{"macro renames a second keyword lexer", enum + real + "#define KW ts_lex_keywords\nstatic bool KW(TSLexer *lexer, TSStateId state) { START_LEXER(); eof = lexer->eof(lexer); switch (state) { case 0: ACCEPT_TOKEN(7); END_STATE(); default: return false; } }\n", "directive"},
		{"count macro overrides an enumerator", "enum { STATE_COUNT = 1 };\n#define STATE_COUNT 2\n" + enum + lex("case 0: ACCEPT_TOKEN(STATE_COUNT); END_STATE();"), "defined"},
		{"count macro defined before an enumerator of that name", "#define STATE_COUNT 2\nenum { STATE_COUNT = 1 };\n" + enum + real, "defined"},
		{"count macro defined twice", "#define STATE_COUNT 2\n#define STATE_COUNT 3\n" + enum + real, "defined"},
		{"count macro with a non-decimal value", "#define STATE_COUNT 0x2\n" + enum + real, "directive"},
		{"count macro with a signed value", "#define STATE_COUNT -2\n" + enum + real, "directive"},
		{"constructor writes to a set", "static TSCharacterRange s1[] = {{0, 0}};\n" + enum + lex("case 0:\n if (eof && set_contains(s1, 1, lookahead)) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();") + "__attribute__((constructor))\nstatic void initialize(void) { s1[0].start = 1; s1[0].end = 1; }\n", "not one the generator writes"},
		{"function after the tail", enum + real + goodTail + "static void f(void) {}\n", "not one the generator writes"},
		{"typedef in the tail", enum + real + goodTail + "typedef int myint;\n", "not one the generator writes"},
		{"language function with another body", enum + real + "TS_PUBLIC const TSLanguage *tree_sitter_demo(void) { static const TSLanguage language = {1}; s1[0].start = 1; return &language; }\n", "not one the generator writes"},
		{"language function initializer closed early", enum + real + "const TSLanguage *tree_sitter_demo(void) { static const TSLanguage language = {1}; ; f(); { } ; return &language; }\n", "not one the generator writes"},
		{"second keyword lexer", enum + real + kw + kw, "defined twice"},
		{"second language function", enum + real + strings.Repeat(goodTail[strings.Index(goodTail, "TS_PUBLIC const"):strings.Index(goodTail, "#ifdef __cplusplus\n}")], 2), "defined twice"},
		{"extern C outside the cplusplus conditional", enum + real + "extern \"C\" {\n", "extern"},
		{"unclosed conditional at the end", enum + real + "#ifdef _WIN32\n", "without its end"},
		{"directive of another shape in the tail", enum + real + "#define TS_PUBLIC 1\n", "directive"},
		{"include in the tail", enum + real + "#include \"other.h\"\n", "directive"},
		{"set named like an enumerator", enum + "static const TSCharacterRange sym_a[] = {{'a','z'}};\n" + real, "defined twice"},
		{"enumerator named true", "enum { true = 0 };\n" + enum + real, "name that the lexer macros"},
		{"helper named like a lex function", enum + "static inline bool ts_lex(int32_t c) { return c == 'a'; }\n" + real, "name that the lexer macros"},
		{"table defined twice", "static const TSSymbol ts_symbol_map[] = {1};\nstatic const TSSymbol ts_symbol_map[] = {2};\n" + enum + real, "defined twice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse("#include \"tree_sitter/parser.h\"\n"+c.c, StandardHeader)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

// Lexical details: an octal escape ends after three digits, and a backslash
// followed by white space and a line end splices the line.
func TestLexicalDetails(t *testing.T) {
	enum := "enum ts_symbol_identifiers { sym_a = 1, };\n"
	lex := func(body string) string {
		return "static bool ts_lex(TSLexer *lexer, TSStateId state) {\n  START_LEXER();\n  eof = lexer->eof(lexer);\n  switch (state) {\n" + body + "\n    default:\n      return false;\n  }\n}\n"
	}
	if _, err := parseInc(enum+lex("case 0:\n if (eof && lookahead == '\\0000') ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"), StandardHeader); err == nil || !strings.Contains(err.Error(), "octal") {
		t.Errorf("four-digit octal escape: err = %v", err)
	}
	if _, err := parseInc(enum+lex("case 0:\n if (lookahead == '\\060') ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"), StandardHeader); err != nil {
		t.Errorf("three-digit octal escape: %v", err)
	}
	for name, ws := range map[string]string{"space": " ", "tab": "\t", "vertical tab": "\v", "form feed": "\f", "several": " \t \f", "none": ""} {
		c := enum + lex("case 0:\n// x \\"+ws+"\nif (eof) ADVANCE(1);\nEND_STATE();\ncase 1: END_STATE();")
		if _, err := parseInc(c, StandardHeader); err == nil || !strings.Contains(err.Error(), "backslash-newline") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// a backslash followed by other text is not a splice
	if _, err := parseInc(enum+lex("case 0:\n if (lookahead == '\\n') ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"), StandardHeader); err != nil {
		t.Errorf("escaped newline in a character constant: %v", err)
	}
}

// A chain of 100,000 helpers is refused with an error; the validation does not
// recurse over the chain.
func TestHelperChainOf100000(t *testing.T) {
	const n = 100000
	var b strings.Builder
	b.WriteString("enum ts_symbol_identifiers { sym_a = 1, };\nstatic inline bool h0(int32_t c) { return c == 'a'; }\n")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "static inline bool h%d(int32_t c) { return h%d(c); }\n", i, i-1)
	}
	fmt.Fprintf(&b, "static bool ts_lex(TSLexer *lexer, TSStateId state) {\n START_LEXER();\n eof = lexer->eof(lexer);\n switch (state) {\n case 0:\n if (h%d(lookahead)) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();\n default:\n return false;\n }\n}\n", n-1)
	if _, err := parseInc(b.String(), StandardHeader); err == nil || !strings.Contains(err.Error(), "nest deeper") {
		t.Errorf("err = %v", err)
	}
}

// A run of || or && terms at one level is a flat list: its length is not nesting.
func TestFlatChains(t *testing.T) {
	enum := "enum ts_symbol_identifiers { sym_a = 1, };\n"
	lex := func(cond string) string {
		return "static bool ts_lex(TSLexer *lexer, TSStateId state) {\n START_LEXER();\n eof = lexer->eof(lexer);\n switch (state) {\n case 0:\n if (" + cond + ") ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();\n default:\n return false;\n }\n}\n"
	}
	for _, n := range []int{400, 20000} {
		or := "lookahead == 'a'" + strings.Repeat(" || lookahead == 'b'", n)
		src, err := parseInc(enum+lex(or), StandardHeader)
		if err != nil {
			t.Fatalf("%d terms of ||: %v", n, err)
		}
		if got := stepOf(src.Main, 0, false, 'b'); got != "adv>1" {
			t.Errorf("||: step on 'b' = %s", got)
		}
		if got := stepOf(src.Main, 0, false, 'c'); got != "none" {
			t.Errorf("||: step on 'c' = %s", got)
		}
		and := "lookahead != 'a'" + strings.Repeat(" && lookahead != 'b'", n)
		src, err = parseInc(enum+lex(and), StandardHeader)
		if err != nil {
			t.Fatalf("%d terms of &&: %v", n, err)
		}
		if got := stepOf(src.Main, 0, false, 'c'); got != "adv>1" {
			t.Errorf("&&: step on 'c' = %s", got)
		}
		if got := stepOf(src.Main, 0, false, 'b'); got != "none" {
			t.Errorf("&&: step on 'b' = %s", got)
		}
	}
	// Nesting by parentheses or ternaries still counts.
	tern := strings.Repeat("lookahead == 'a' ? ", 300) + "1" + strings.Repeat(" : 0", 300)
	if _, err := parseInc(enum+lex(tern), StandardHeader); err == nil || !strings.Contains(err.Error(), "deeper than 256") {
		t.Errorf("300 nested ternaries: err = %v", err)
	}
	if _, err := parseInc(enum+lex(strings.Repeat("!", 300)+"(lookahead == 'a')"), StandardHeader); err == nil || !strings.Contains(err.Error(), "deeper than 256") {
		t.Errorf("300 negations: err = %v", err)
	}
}

// The review's reproducers about names that the header defines, an ignored
// table payload, a helper that shadows a set, the cost of nested helper calls,
// and conditionals.
func TestHeaderMacroAndPayloadReproducers(t *testing.T) {
	lex := func(body string) string {
		return "static bool ts_lex(TSLexer *lexer, TSStateId state) {\n  START_LEXER();\n  eof = lexer->eof(lexer);\n  switch (state) {\n" + body + "\n    default:\n      return false;\n  }\n}\n"
	}
	inc := "#include \"tree_sitter/parser.h\"\n"
	cases := []struct{ name, c, want string }{
		{"enumerator named like a header macro, declared before the include",
			"enum { TREE_SITTER_SERIALIZATION_BUFFER_SIZE = 1 };\n" + inc + lex("case 0:\n ACCEPT_TOKEN(TREE_SITTER_SERIALIZATION_BUFFER_SIZE);\n END_STATE();"), "before the include"},
		{"enumerator named like a header macro, after the include",
			inc + "enum { TREE_SITTER_SERIALIZATION_BUFFER_SIZE = 1 };\n" + lex("case 0:\n ACCEPT_TOKEN(TREE_SITTER_SERIALIZATION_BUFFER_SIZE);\n END_STATE();"), "name that the lexer macros"},
		{"enumerator named like a limit macro", inc + "enum { INT32_MAX = 1 };\n" + lex("case 0:\n END_STATE();"), "name that the lexer macros"},
		{"include inside a conditional", "#ifdef _WIN32\n" + inc + "#endif\n" + lex("case 0:\n END_STATE();"), "inside a conditional"},
		{"table with a second declarator and pragma operators",
			"static const char *ts_symbol_names[] = {\n _Pragma(\"push_macro(\\\"true\\\")\") 0\n}, *true = {0};\n" + inc + "static const char *ts_field_names[] = {\n _Pragma(\"pop_macro(\\\"true\\\")\") 0\n};\n" + lex("case 0:\n if (eof && true) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"), "pragma operator"},
		{"table with a second declarator", inc + "static const char *ts_symbol_names[] = { 0 }, *x = {0};\n" + lex("case 0:\n END_STATE();"), "not one the generator writes"},
		{"pragma operator in a table", inc + "static const char *ts_symbol_names[] = { _Pragma(\"once\") 0 };\n" + lex("case 0:\n END_STATE();"), "pragma operator"},
		{"pragma operator in the language initializer", inc + lex("case 0:\n END_STATE();") + "const TSLanguage *tree_sitter_demo(void) { static const TSLanguage language = { _Pragma(\"once\") 1 }; return &language; }\n", "pragma operator"},
		{"helper parameter named like a set", inc + "static const TSCharacterRange c[] = {{0, 0}};\nstatic inline bool h(int32_t c) { return set_contains(c, 1, c); }\n" + lex("case 0:\n if (eof && h(lookahead)) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();"), "parameter name"},
		{"#else twice", inc + "#ifdef _WIN32\n#else\n#else\n#endif\n" + lex("case 0:\n END_STATE();"), "after #else"},
		{"#elif after #else", inc + "#ifdef _WIN32\n#else\n#elif defined(_WIN32)\n#endif\n" + lex("case 0:\n END_STATE();"), "after #else"},
		{"count macro with a leading zero", inc + "#define STATE_COUNT 010\n" + lex("case 0:\n END_STATE();"), "directive"},
		{"count macro zero is a decimal literal", inc + "#define STATE_COUNT 0\n" + lex("case 0:\n END_STATE();"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.c, StandardHeader)
			if c.want == "" {
				if err != nil {
					t.Errorf("err = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

// Sixteen levels of helpers with ten calls each would take 10^16 calls without
// a cache; with one evaluation per (helper, argument) it is instant.
func TestHelperFanOutIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("#include \"tree_sitter/parser.h\"\nstatic inline bool h0(int32_t c) { return true; }\n")
	for i := 1; i <= 16; i++ {
		fmt.Fprintf(&b, "static inline bool h%d(int32_t c) { return %s; }\n", i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("h%d(c) && ", i-1), 10), " && "))
	}
	b.WriteString("static bool ts_lex(TSLexer *lexer, TSStateId state) {\n START_LEXER();\n eof = lexer->eof(lexer);\n switch (state) {\n case 0:\n if (eof && h16(lookahead)) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();\n default:\n return false;\n }\n}\n")
	start := time.Now()
	src, err := Parse(b.String(), StandardHeader)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := BuildUnchecked(src, b.String())
	if err != nil {
		t.Fatal(err)
	}
	if got := tab.Main.OnEOF[0]; got != 4 {
		t.Errorf("OnEOF[0] = %d, want a step to state 1 (4)", got)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v", d)
	}
}

// A header that is the standard one with a "// @generated" line and an empty
// line added at the top has the same lexer macros and the same set_contains,
// and has its own entry in the list.
func TestGeneratedCommentHeaderEntry(t *testing.T) {
	text := "// @generated\n\n" + StandardHeader
	if got := headerHash(text); got != "f1d32ece598a2dc68abc4f735f84c243484e1126ee9b23288522c23b07b73651" {
		t.Fatalf("hash = %s", got)
	}
	h, err := parseHeader(text)
	if err != nil {
		t.Fatal(err)
	}
	if want := factsOf(text); h != want {
		t.Errorf("entry %+v differs from the facts read from the header: %+v", h, want)
	}
	if macroSection(text) != macroSection(StandardHeader) {
		t.Error("the macro section differs from the standard header's")
	}
}
