package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// extractCharacterSetsLegacy is the reader that split entries on ',' and '}'
// and skipped entries it could not read. It stays here so a test can compare
// its result with the current reader on real parser.c files.
func extractCharacterSetsLegacy(source string) (map[string][]lexRange, error) {
	re := regexp.MustCompile(`(?m)(?:static\s+)?(?:const\s+)?TSCharacterRange\s+(\w+)\s*\[[^\]]*\]\s*=\s*\{`)
	matches := re.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	sets := make(map[string][]lexRange, len(matches))
	entryRe := regexp.MustCompile(`\{\s*([^,}]+)\s*,\s*([^}]+)\s*\}`)
	for _, m := range matches {
		name := m[1]
		body, err := findArrayBody(source, name)
		if err != nil {
			continue
		}

		entries := entryRe.FindAllStringSubmatch(body, -1)
		if len(entries) == 0 {
			continue
		}

		ranges := make([]lexRange, 0, len(entries))
		for _, e := range entries {
			lo, ok := parseCLookaheadLiteral(strings.TrimSpace(e[1]))
			if !ok {
				continue
			}
			hi, ok := parseCLookaheadLiteral(strings.TrimSpace(e[2]))
			if !ok {
				continue
			}
			if lo > hi {
				lo, hi = hi, lo
			}
			if hi < 0 || lo > maxLookaheadRune {
				continue
			}
			if lo < 0 {
				lo = 0
			}
			if hi > maxLookaheadRune {
				hi = maxLookaheadRune
			}
			ranges = append(ranges, lexRange{lo: lo, hi: hi})
		}
		sets[name] = normalizeRanges(ranges)
	}

	return sets, nil
}

const badSetHead = "static const TSCharacterRange bad_set[] = {"

func TestExtractCharacterSetsReadsCommaAndBrace(t *testing.T) {
	src := `static TSCharacterRange demo_character_set_1[] = {
  {',', '/'}, {'}', 0x10ffff}, {'\'', '\''}, {'\\', '\\'},
};
`
	sets, err := extractCharacterSets(src)
	if err != nil {
		t.Fatalf("extractCharacterSets: %v", err)
	}
	want := []lexRange{{lo: '\'', hi: '\''}, {lo: ',', hi: '/'}, {lo: '\\', hi: '\\'}, {lo: '}', hi: 0x10ffff}}
	if got := sets["demo_character_set_1"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges = %v, want %v", got, want)
	}
}

// TestExtractCharacterSetsReadsGeneratorShapes covers each accepted
// declaration shape, separator, trailing comma and bound spelling.
func TestExtractCharacterSetsReadsGeneratorShapes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []lexRange
	}{
		{"static_const", "static const TSCharacterRange s[] = {\n  {0, 0x02},\n};\n", []lexRange{{0, 0x02}}},
		{"static", "static TSCharacterRange s[] = {\n  {0, 0x02},\n};\n", []lexRange{{0, 0x02}}},
		{"const", "const TSCharacterRange s[] = {\n  {0, 0x02},\n};\n", []lexRange{{0, 0x02}}},
		{"eight_entries_then_line_break", "static TSCharacterRange s[] = {\n  {0x01, 0x01}, {0x03, 0x03}, {0x05, 0x05}, {0x07, 0x07}, {0x09, 0x09}, {0x0b, 0x0b}, {0x0d, 0x0d}, {0x0f, 0x0f},\n  {0x11, 0x11},\n};\n",
			[]lexRange{{1, 1}, {3, 3}, {5, 5}, {7, 7}, {9, 9}, {11, 11}, {13, 13}, {15, 15}, {17, 17}}},
		{"two_digit_hex_and_zero", "static TSCharacterRange s[] = {\n  {0, 0x00}, {0x10, 0x10},\n};\n", []lexRange{{0, 0}, {16, 16}}},
		{"escapes", "static TSCharacterRange s[] = {\n  {'\\t', '\\t'}, {'\\n', '\\r'}, {'\\f', '\\f'}, {'\"', '\"'}, {' ', ' '}, {'{', '{'},\n};\n",
			[]lexRange{{9, 13}, {' ', ' '}, {'"', '"'}, {'{', '{'}}},
		{"hex_and_decimal", "static TSCharacterRange s[] = {\n  {0, 0x1f}, {0x10ffff, 0x10ffff},\n};\n", []lexRange{{0, 0x1f}, {0x10ffff, 0x10ffff}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sets, err := extractCharacterSets(tc.src)
			if err != nil {
				t.Fatalf("extractCharacterSets: %v", err)
			}
			got := sets["s"]
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ranges = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestExtractCharacterSetsIgnoresOtherTypeNames checks that a longer
// identifier that ends in the type name is not taken for the type.
func TestExtractCharacterSetsIgnoresOtherTypeNames(t *testing.T) {
	src := "typedef int OtherTSCharacterRange;\nOtherTSCharacterRange xs[] = {0, 0x02};\n"
	sets, err := extractCharacterSets(src)
	if err != nil || len(sets) != 0 {
		t.Fatalf("sets = %v, err = %v; want no sets and no error", sets, err)
	}
}

// TestExtractCharacterSetsFailsClosed lists input that the reader refuses.
// Each error names the array and the byte offset of the text in `at`.
func TestExtractCharacterSetsFailsClosed(t *testing.T) {
	entry := func(a string) string { return "\n  {" + a + ", 0x02},\n};\n" }
	cases := []struct {
		name string
		tail string // text after the opening brace
		at   string // text at which the error offset must point
	}{
		// Separators and layout.
		{"missing_comma", "\n  {0, 0x02}{0x07, 0x08},\n};\n", "{0x07"},
		{"doubled_comma", "\n  {0, 0x02},, {0x07, 0x08},\n};\n", ",, {"},
		{"leading_comma", "\n  ,{0, 0x02},\n};\n", "\n  ,"},
		{"no_trailing_comma", "\n  {0, 0x02}\n};\n", "\n};"},
		{"missing_semicolon", "\n  {0, 0x02},\n}\n", ",\n}\n"},
		{"trailing_junk_after_close", "\n  {0, 0x02},\n}junk\n", ",\n}junk"},
		{"text_after_semicolon", "\n  {0, 0x02},\n}; x\n", ",\n}; x"},
		{"no_newline_after_close", "\n  {0, 0x02},\n};", ",\n};"},
		{"one_digit_hex", "\n  {1, 0xf},\n};\n", "1, 0xf"},
		{"one_digit_hex_value", entry("0xf"), "0xf"},
		{"nonzero_decimal", "\n  {0, 7},\n};\n", "7}"},
		{"nine_entries_on_a_line", "\n  {0, 0x02}, {0x03, 0x04}, {0x05, 0x06}, {0x07, 0x08}, {0x09, 0x0a}, {0x0b, 0x0c}, {0x0d, 0x0e}, {0x0f, 0x10}, {0x11, 0x12},\n};\n", ", {0x11"},
		{"line_break_after_seven_entries", "\n  {0, 0x02}, {0x03, 0x04}, {0x05, 0x06}, {0x07, 0x08}, {0x09, 0x0a}, {0x0b, 0x0c}, {0x0d, 0x0e},\n  {0x0f, 0x10},\n};\n", ",\n  {0x0f"},
		{"inner_trailing_comma", "\n  {0, 0x02,},\n};\n", ",}"},
		{"extra_scalar_braces", "\n  {{1}, {2}},\n};\n", "{1}"},
		{"two_trailing_commas", "\n  {0, 0x02},,\n};\n", ",,"},
		{"empty_array", "\n};\n", "\n};"},
		{"array_not_closed", "\n  {0, 0x02},\n", ",\n"},
		{"stray_text_after_entry", "\n  {0, 0x02} junk,\n};\n", " junk"},
		{"tab_indent", "\n\t{0, 0x02},\n};\n", "\n\t"},
		// Comments, strings and line splices.
		{"block_comment_before_entry", "\n  /* c */ {0, 0x02},\n};\n", "\n  /*"},
		{"block_comment_between_entries", "\n  {0, 0x02}, /* c */ {0x07, 0x08},\n};\n", ", /*"},
		{"string_bound", entry(`"a"`), `"a"`},
		// Character literals outside the accepted spellings.
		{"multibyte_character", entry("'é'"), "'é'"},
		{"universal_name_big_u", entry(`'\U000000ea'`), `'\U000000ea'`},
		{"universal_name_u", entry(`'\u00e9'`), `'\u00e9'`},
		{"surrogate_universal_name", entry(`'\uD800'`), `'\uD800'`},
		{"null_universal_name", entry(`'\u0000'`), `'\u0000'`},
		{"ascii_universal_name", entry(`'\u0061'`), `'\u0061'`},
		{"octal_escape_377", entry(`'\377'`), `'\377'`},
		{"octal_escape_101", entry(`'\101'`), `'\101'`},
		{"hex_escape_80", entry(`'\x80'`), `'\x80'`},
		{"hex_escape_41", entry(`'\x41'`), `'\x41'`},
		{"hex_escape_100", entry(`'\x100'`), `'\x100'`},
		{"hex_escape_nine_digits", entry(`'\x000000041'`), `'\x000000041'`},
		{"escape_a", entry(`'\a'`), `'\a'`},
		{"escape_question", entry(`'\?'`), `'\?'`},
		{"escape_double_quote", entry(`'\"'`), `'\"'`},
		{"escape_zero", entry(`'\0'`), `'\0'`},
		{"empty_character", entry("''"), "''"},
		{"two_characters", entry("'ab'"), "'ab'"},
		// Integer literals outside the accepted spellings.
		{"suffix_uul", entry("1uul"), "1uul"},
		{"suffix_lL", entry("1lL"), "1lL"},
		{"suffix_u", entry("1u"), "1u,"},
		{"suffix_hex_u", entry("0x41U"), "U,"},
		{"upper_case_hex_prefix", entry("0X41"), "X41"},
		{"upper_case_hex_digits", entry("0xAB"), "0xAB"},
		{"hex_without_digits", entry("0x"), "0x,"},
		{"hex_seven_digits", entry("0x0000041"), "0x0000041"},
		{"octal_integer", entry("010"), "10,"},
		{"negative_integer", entry("-1"), "-1"},
		{"identifier_bound", entry("FOO"), "FOO"},
		{"above_last_code_point_hex", entry("0x110000"), "0x110000"},
		{"above_last_code_point_decimal", entry("1114112"), "1114112"},
		// Ranges.
		{"start_above_end", "\n  {'z', 'a'},\n};\n", "{'z'"},
		{"missing_second_value", "\n  {'a'},\n};\n", "},"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := badSetHead + tc.tail
			sets, err := extractCharacterSets(src)
			if err == nil {
				t.Fatalf("no error; sets = %v", sets)
			}
			msg := err.Error()
			if !strings.Contains(msg, `"bad_set"`) {
				t.Errorf("error %q does not name the array", msg)
			}
			want := fmt.Sprintf("byte offset %d:", strings.Index(src, tc.at))
			if !strings.Contains(msg, want) {
				t.Errorf("error %q does not contain %q", msg, want)
			}
		})
	}
}

// TestExtractCharacterSetsRefusesUnknownTypeUse checks that the whole source
// is refused when the type name occurs outside a recognized declaration.
func TestExtractCharacterSetsRefusesUnknownTypeUse(t *testing.T) {
	good := "static const TSCharacterRange good[] = {\n  {0, 0x02},\n};\n"
	cases := []struct {
		name, src, at string
	}{
		{"in_block_comment", "/* TSCharacterRange fake[] = {{BAD, 9}}; */\n" + good, "TSCharacterRange fake"},
		{"in_line_comment", "// TSCharacterRange fake\n" + good, "TSCharacterRange fake"},
		{"in_string", "const char *s = \"TSCharacterRange fake[]={{1,2}}\";\n" + good, "TSCharacterRange fake"},
		{"comment_between_type_and_name", "static const TSCharacterRange /* c */ real[] = {\n  {0, 0x02},\n};\n", "TSCharacterRange /*"},
		{"vertical_tab_before_name", "static const TSCharacterRange\vreal[] = {\n  {0, 0x02},\n};\n", "TSCharacterRange\v"},
		{"sized_array", "static const TSCharacterRange real[3] = {\n  {0, 0x02},\n};\n", "TSCharacterRange real"},
		{"no_storage_prefix", "TSCharacterRange real[] = {\n  {0, 0x02},\n};\n", "TSCharacterRange real"},
		{"indented_declaration", "  static const TSCharacterRange real[] = {\n  {0, 0x02},\n};\n", "TSCharacterRange real"},
		{"two_spaces_before_name", "static const TSCharacterRange  real[] = {\n  {0, 0x02},\n};\n", "TSCharacterRange  real"},
		{"declaration_in_block_comment_after_real_one", good + "/*\nstatic const TSCharacterRange good[] = {\n  {0x62, 0x62},\n};\n*/\n", "TSCharacterRange good[] = {\n  {0x62"},
		{"declaration_in_line_comment", good + "// static const TSCharacterRange other[] = {\n", "TSCharacterRange other"},
		{"pointer_use", good + "static void f(const TSCharacterRange *r);\n", "TSCharacterRange *r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sets, err := extractCharacterSets(tc.src)
			if err == nil {
				t.Fatalf("no error; sets = %v", sets)
			}
			want := fmt.Sprintf("byte offset %d: TSCharacterRange", strings.Index(tc.src, tc.at))
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		})
	}
}

// TestExtractGrammarPassesCharacterSetErrorUp puts an unreadable array into a
// complete minimal parser.c and checks that the import fails with an error
// from the range reader that names the array.
func TestExtractGrammarPassesCharacterSetErrorUp(t *testing.T) {
	if _, err := ExtractGrammar(miniParserC); err != nil {
		t.Fatalf("minimal parser.c does not import without the bad array: %v", err)
	}
	src := miniParserC + "\n" + badSetHead + "\n  {FOO, 'z'},\n};\n"
	_, err := ExtractGrammar(src)
	if err == nil {
		t.Fatal("ExtractGrammar accepted an unreadable character range array")
	}
	for _, want := range []string{"lex states", `"bad_set"`, "byte offset"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// pinnedParserSources returns the parser.c path of every grammar that the
// lock file pins and that exists under $TS2GO_PARSER_REPOS, and the number of
// lock entries. It skips the test when that variable is unset.
func pinnedParserSources(t *testing.T) (map[string]string, int) {
	t.Helper()
	root := os.Getenv("TS2GO_PARSER_REPOS")
	if root == "" {
		t.Skip("TS2GO_PARSER_REPOS is not set; set it to a folder of <name>-<commit prefix> checkouts to run this test")
	}
	lock, err := os.Open(filepath.Join("..", "..", "grammars", "languages.lock"))
	if err != nil {
		t.Skipf("languages.lock not readable: %v", err)
	}
	defer lock.Close()
	out := map[string]string{}
	total := 0
	sc := bufio.NewScanner(lock)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || strings.HasPrefix(f[0], "#") || len(f[2]) < 12 {
			continue
		}
		total++
		sub := "src"
		if len(f) >= 4 && !strings.HasPrefix(f[3], ".") {
			sub = f[3]
		}
		p := filepath.Join(root, f[0]+"-"+f[2][:12], sub, "parser.c")
		if _, err := os.Stat(p); err == nil {
			out[f[0]] = p
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading languages.lock: %v", err)
	}
	if len(out) == 0 {
		t.Skipf("no pinned parser.c found under %s", root)
	}
	return out, total
}

func sortedKeys(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func TestExtractCharacterSetsPinnedCorpus(t *testing.T) {
	files, total := pinnedParserSources(t)
	read, entries, refused, arrays := 0, 0, 0, 0
	names := sortedKeys(files)
	for _, n := range names {
		data, err := os.ReadFile(files[n])
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		as, err := readCharacterRangeArrays(string(data))
		if err != nil {
			t.Errorf("%s: %v", n, err)
			refused++
			continue
		}
		read++
		arrays += len(as)
		for _, a := range as {
			entries += len(a.ranges)
		}
	}
	t.Logf("CORPUS pinned=%d present=%d read=%d arrays=%d entries=%d refused=%d", total, len(names), read, arrays, entries, refused)
}

func TestExtractCharacterSetsLegacyDifferential(t *testing.T) {
	files, _ := pinnedParserSources(t)
	for _, n := range sortedKeys(files) {
		data, err := os.ReadFile(files[n])
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		src := string(data)
		oldSets, _ := extractCharacterSetsLegacy(src)
		newSets, err := extractCharacterSets(src)
		if err != nil {
			t.Logf("DIFF %s: refused: %v", n, err)
			continue
		}
		var arrays []string
		for name, ns := range newSets {
			if !reflect.DeepEqual(oldSets[name], ns) {
				arrays = append(arrays, name)
			}
		}
		for name := range oldSets {
			if _, ok := newSets[name]; !ok {
				arrays = append(arrays, name)
			}
		}
		sort.Strings(arrays)
		if len(arrays) > 0 {
			t.Logf("DIFF %s: %d arrays differ: %v", n, len(arrays), arrays)
		}
	}
}

// TestExtractCharacterSetsRefusesBeforeReading lists input that is refused
// by a whole-file check. The error gives a byte offset and no array name.
func TestExtractCharacterSetsRefusesBeforeReading(t *testing.T) {
	good := "static const TSCharacterRange good[] = {\n  {0, 0x02},\n};\n"
	cases := []struct {
		name, src, at string
	}{
		{"line_comment_with_continuation", badSetHead + " // ignored \\\n  {0, 0x02},\n  {0x07, 0x08}};\n", "\\\n"},
		{"split_comment_opener", good + "/\\\n* c */\n", "\\\n*"},
		{"line_spliced_type_name", "static const TSCharacte\\\nrRange s[] = {\n  {0, 0},\n};\n", "\\\nrRange"},
		{"backslash_newline_between_entries", badSetHead + "\n  {0, 0x02},\\\n  {0x07, 0x08},\n};\n", "\\\n"},
		{"nul_byte", good + "\x00", "\x00"},
		{"lone_carriage_return", good + "int x;\rint y;\n", "\r"},
		{"carriage_return_in_literal", badSetHead + "\n  {'\r', 0x02},\n};\n", "\r'"},
		{"trigraph_equals", good + "??=define X 1\n", "??="},
		{"trigraph_paren", good + "??(\n", "??("},
		{"trigraph_minus", good + "??-\n", "??-"},
		{"digit_separator_before_comment_apostrophe", "int n = 1'000; /* '\nstatic const TSCharacterRange fake[] = {\n  {0x62, 0x62},\n};\n*/\n", "'000"},
		{"prefixed_character_literal", "int c = L'x';\n", "'x"},
		{"underscore_before_apostrophe", "int c = a_'x';\n", "'x"},
		{"comment_not_closed", good + "/* open\n", "/* open"},
		{"string_not_closed", good + "const char *s = \"open;\n", "\"open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sets, err := extractCharacterSets(tc.src)
			if err == nil {
				t.Fatalf("no error; sets = %v", sets)
			}
			want := fmt.Sprintf("byte offset %d:", strings.Index(tc.src, tc.at))
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
		})
	}
}

// TestExtractCharacterSetsReadsAroundComments checks that a real array is
// read when a comment further on holds text that is not the type name.
func TestExtractCharacterSetsReadsAroundComments(t *testing.T) {
	src := "/* note */ // note\nstatic const TSCharacterRange s[] = {\n  {',', ','},\n};\nconst char *p = \"x\"; char c = '\\'';\n"
	sets, err := extractCharacterSets(src)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sets["s"], []lexRange{{',', ','}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges = %v, want %v", got, want)
	}
}

// TestExtractCharacterSetsRefusesDuplicateName checks that two arrays with
// one name refuse the source and that the error gives the offset of the
// second declaration.
func TestExtractCharacterSetsRefusesDuplicateName(t *testing.T) {
	one := "static const TSCharacterRange dup[] = {\n  {0, 0x02},\n};\n"
	src := one + "int x;\n" + one
	sets, err := extractCharacterSets(src)
	if err == nil {
		t.Fatalf("no error; sets = %v", sets)
	}
	want := fmt.Sprintf("byte offset %d: character range array \"dup\"", strings.LastIndex(src, "static const"))
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
}
