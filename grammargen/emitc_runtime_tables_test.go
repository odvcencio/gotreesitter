package grammargen

// C-runtime tests for the tables that tree-sitter's C runtime reads without a
// NULL check, and for the table layout that its query analysis assumes.
//
// EmitC output parsed correctly under the C runtime but failed as soon as a
// host compiled a query, which is the first thing an editor does:
//
//  1. ts_symbol_map was never emitted. ts_node_symbol and
//     ts_language_symbol_for_name read public_symbol_map unconditionally.
//  2. ts_non_terminal_alias_map was never emitted. Query analysis scans
//     alias_map unconditionally.
//  3. ts_alias_sequences was omitted for grammars without aliases. A
//     production that only carries fields still has a non-zero production id,
//     and the runtime indexes alias_sequences for it.
//  4. ts_primary_state_ids was never emitted (and was referenced without a
//     definition when a Language carried primary states). Query analysis
//     reads it at ABI 14 and later.
//  5. Field ids followed first appearance. ts_language_field_id_for_name
//     scans field names in order and stops early, so it needs sorted names,
//     and FIELD_COUNT counted the empty slot 0, which let a failed lookup
//     read past ts_field_names.
//  6. ts_small_parse_table groups mixed terminals and nonterminals that
//     shared a value. The lookahead iterator decides once per group whether
//     the value is an action or a goto, so query analysis lost the gotos and
//     aborted with "final_step_indices.size > 0".

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// emitcQueryGrammar has several sibling declarations behind a hidden choice,
// a word token, and fields whose first appearance is not in sorted order.
func emitcQueryGrammar() *Grammar {
	g := NewGrammar("emitc_query_tables")
	g.Define("source_file", Repeat(Sym("_declaration")))
	g.Define("_declaration", Choice(
		Sym("let_decl"), Sym("const_decl"), Sym("type_decl"),
		Sym("func_decl"), Sym("var_decl"), Sym("use_decl"),
	))
	g.Define("let_decl", Seq(Str("let"), Field("name", Sym("identifier")), Str("="), Field("value", Sym("number")), Str(";")))
	g.Define("const_decl", Seq(Str("const"), Field("name", Sym("identifier")), Str("="), Field("value", Sym("number")), Str(";")))
	g.Define("type_decl", Seq(Str("type"), Field("name", Sym("identifier")), Optional(Seq(Str(":"), Field("kind", Sym("identifier")))), Str(";")))
	g.Define("func_decl", Seq(Str("func"), Field("name", Sym("identifier")), Str("("), Repeat(Field("param", Sym("identifier"))), Str(")"), Str(";")))
	g.Define("var_decl", Seq(Str("var"), Field("name", Sym("identifier")), Field("mode", Sym("identifier")), Str(";")))
	g.Define("use_decl", Seq(Str("use"), Field("target", Sym("identifier")), Str(";")))
	g.Define("identifier", Pat(`[a-z_]+`))
	g.Define("number", Pat(`[0-9]+`))
	g.SetExtras(Pat(`\s`))
	g.SetWord("identifier")
	return g
}

const emitcQueryInput = "let a = 1; const b = 2; type c : d; type e; func f (g h); var i j; use k; let l = 3;\n"

const emitcQuerySource = `(let_decl name: (identifier) @let.name value: (number) @let.value)
(const_decl name: (identifier) @const.name)
(type_decl kind: (identifier) @type.kind)
(func_decl param: (identifier) @func.param)
(var_decl mode: (identifier) @var.mode)
(use_decl target: (identifier) @use.target)
"let" @keyword
`

func TestEmitCDeclaresRuntimeTables(t *testing.T) {
	lang, err := GenerateLanguageForC(emitcQueryGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	if len(lang.AliasSequences) != 0 {
		t.Fatalf("fixture must have no alias sequences, got %d rows", len(lang.AliasSequences))
	}
	code, err := EmitC("emitc_query_tables", lang)
	if err != nil {
		t.Fatalf("EmitC: %v", err)
	}
	for _, want := range []string{
		"static const TSSymbol ts_symbol_map[] = {",
		".public_symbol_map = ts_symbol_map,",
		"static const uint16_t ts_non_terminal_alias_map[] = {",
		".alias_map = ts_non_terminal_alias_map,",
		"static const TSSymbol ts_alias_sequences[PRODUCTION_ID_COUNT][MAX_ALIAS_SEQUENCE_LENGTH] = {",
		".alias_sequences = &ts_alias_sequences[0][0],",
		"static const TSStateId ts_primary_state_ids[STATE_COUNT] = {",
		".primary_state_ids = ts_primary_state_ids,",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("EmitC output lacks %q", want)
		}
	}
	names := cFieldNames(lang)
	if got := headerDefineInt(t, code, "FIELD_COUNT"); got != len(names) || got != 6 {
		t.Errorf("FIELD_COUNT = %d, want the 6 field names %v", got, names)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("C field names are not sorted: %v", names)
	}
	var order []string
	for _, line := range strings.Split(code, "\n") {
		if strings.HasPrefix(line, "  [field_") && strings.Contains(line, "] = \"") {
			order = append(order, line[strings.Index(line, "\"")+1:strings.LastIndex(line, "\"")])
		}
	}
	if strings.Join(order, " ") != strings.Join(names, " ") {
		t.Errorf("ts_field_names order = %v, want %v", order, names)
	}
	ids := cFieldIDs(lang)
	for id, name := range lang.FieldNames {
		if id == 0 {
			continue
		}
		if names[ids[id]-1] != name {
			t.Errorf("field %q maps to C id %d (%q)", name, ids[id], names[ids[id]-1])
		}
	}
}

// TestEmitCSmallParseGroupsHoldOneSymbolKind checks the invariant directly
// and checks that the fixture really has a terminal action index equal to a
// goto state in one state, so the runtime test below covers the regrouping.
func TestEmitCSmallParseGroupsHoldOneSymbolKind(t *testing.T) {
	lang, err := GenerateLanguageForC(emitcQueryGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	offsets, err := cParseActionOffsets(lang)
	if err != nil {
		t.Fatalf("cParseActionOffsets: %v", err)
	}
	collision := false
	for _, sourceOffset := range lang.SmallParseTableMap {
		kinds := make(map[uint16]map[bool]bool)
		pos := int(sourceOffset)
		groupCount := int(lang.SmallParseTable[pos])
		pos++
		for group := 0; group < groupCount; group++ {
			value, count := lang.SmallParseTable[pos], int(lang.SmallParseTable[pos+1])
			pos += 2
			for i := 0; i < count; i++ {
				symbol := lang.SmallParseTable[pos]
				pos++
				mapped := cParseTableValue(lang, offsets, int(symbol), value)
				if kinds[mapped] == nil {
					kinds[mapped] = make(map[bool]bool)
				}
				kinds[mapped][uint32(symbol) >= lang.TokenCount] = true
			}
		}
		for _, k := range kinds {
			collision = collision || len(k) == 2
		}
	}
	if !collision {
		t.Fatal("fixture no longer has a terminal action equal to a goto state; choose a grammar that does")
	}
	for state, data := range cSmallParseTableData(lang, offsets) {
		pos := 1
		sawNonterminal := false
		for group := 0; group < int(data[0]); group++ {
			count := int(data[pos+1])
			symbols := data[pos+2 : pos+2+count]
			pos += 2 + count
			nonterminal := uint32(symbols[0]) >= lang.TokenCount
			for _, symbol := range symbols {
				if (uint32(symbol) >= lang.TokenCount) != nonterminal {
					t.Fatalf("small state %d group %d mixes terminals and nonterminals: %v", state, group, symbols)
				}
			}
			if sawNonterminal && !nonterminal {
				t.Fatalf("small state %d lists a terminal group after a goto group", state)
			}
			sawNonterminal = sawNonterminal || nonterminal
		}
	}
}

const emitcQueryMainC = `#include <stdio.h>
#include <stdlib.h>
#include <tree_sitter/api.h>

const TSLanguage *tree_sitter_emitc_query_tables(void);

static char *slurp(const char *path, uint32_t *length) {
  FILE *f = fopen(path, "rb");
  if (!f) exit(2);
  fseek(f, 0, SEEK_END); long size = ftell(f); fseek(f, 0, SEEK_SET);
  char *buffer = malloc(size + 1);
  *length = (uint32_t)fread(buffer, 1, size, f);
  fclose(f);
  return buffer;
}

// walk prints named nodes by public symbol, which reads public_symbol_map.
static void walk(const TSLanguage *language, TSNode node) {
  if (!ts_node_is_named(node)) return;
  printf("(%s", ts_language_symbol_name(language, ts_node_symbol(node)));
  for (uint32_t i = 0; i < ts_node_named_child_count(node); i++) {
    putchar(' ');
    walk(language, ts_node_named_child(node, i));
  }
  putchar(')');
}

int main(int argc, char **argv) {
  const TSLanguage *language = tree_sitter_emitc_query_tables();
  uint32_t input_length, query_length;
  char *input = slurp(argv[1], &input_length);
  char *source = slurp(argv[2], &query_length);
  TSParser *parser = ts_parser_new();
  if (!ts_parser_set_language(parser, language)) return 3;
  TSTree *tree = ts_parser_parse_string(parser, NULL, input, input_length);
  TSNode root = ts_tree_root_node(tree);
  walk(language, root);
  putchar('\n');
  uint32_t error_offset;
  TSQueryError error;
  TSQuery *query = ts_query_new(language, source, query_length, &error_offset, &error);
  if (!query) {
    printf("query error %d at %u\n", error, error_offset);
    return 4;
  }
  TSQueryCursor *cursor = ts_query_cursor_new();
  ts_query_cursor_exec(cursor, query, root);
  TSQueryMatch match;
  while (ts_query_cursor_next_match(cursor, &match)) {
    for (uint16_t i = 0; i < match.capture_count; i++) {
      uint32_t name_length;
      const char *name = ts_query_capture_name_for_id(query, match.captures[i].index, &name_length);
      printf("%u %.*s %u %u\n", match.pattern_index, (int)name_length, name,
             ts_node_start_byte(match.captures[i].node), ts_node_end_byte(match.captures[i].node));
    }
  }
  ts_query_cursor_delete(cursor);
  ts_query_delete(query);
  ts_tree_delete(tree);
  ts_parser_delete(parser);
  return 0;
}
`

// TestEmitCQueriesMatchGoOnCRuntime compiles a field-constrained query with
// the C runtime against EmitC output and compares its captures, and the tree
// read through public symbols, with gotreesitter's own query engine.
func TestEmitCQueriesMatchGoOnCRuntime(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler is unavailable")
	}
	lang, err := GenerateLanguageForC(emitcQueryGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	code, err := EmitC("emitc_query_tables", lang)
	if err != nil {
		t.Fatalf("EmitC: %v", err)
	}

	runtimeDir := lockedTreeSitterRuntimeDir(t)
	tmp, parserPath := writeLockedRuntimeProbeFiles(t, runtimeDir, code)
	mainPath := filepath.Join(tmp, "main.c")
	inputPath := filepath.Join(tmp, "input")
	queryPath := filepath.Join(tmp, "query.scm")
	mustWrite(t, mainPath, []byte(emitcQueryMainC))
	mustWrite(t, inputPath, []byte(emitcQueryInput))
	mustWrite(t, queryPath, []byte(emitcQuerySource))
	artifact := filepath.Join(tmp, "query-probe")
	args := []string{
		"-std=c11", "-O0", "-D_DEFAULT_SOURCE",
		"-I" + filepath.Join(tmp, "include"),
		"-I" + filepath.Join(runtimeDir, "include"),
		"-I" + filepath.Join(runtimeDir, "src"),
		parserPath, filepath.Join(runtimeDir, "src", "lib.c"), mainPath,
		"-pthread", "-o", artifact,
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile query probe: %v\n%s", err, out)
	}
	out, err := exec.Command(artifact, inputPath, queryPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run query probe: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")

	tree, err := gotreesitter.NewParser(lang).Parse([]byte(emitcQueryInput))
	if err != nil {
		t.Fatalf("Go parse: %v", err)
	}
	if got, want := lines[0], tree.RootNode().SExpr(lang); got != want {
		t.Fatalf("C tree by public symbol differs from Go\n C: %s\nGo: %s", got, want)
	}
	query, err := gotreesitter.NewQuery(emitcQuerySource, lang)
	if err != nil {
		t.Fatalf("Go query: %v", err)
	}
	var want []string
	for _, match := range query.Execute(tree) {
		for _, capture := range match.Captures {
			start, end := capture.ByteRange()
			want = append(want, fmt.Sprintf("%d %s %d %d", match.PatternIndex, capture.Name, start, end))
		}
	}
	got := append([]string(nil), lines[1:]...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("C and Go captures differ\n C:\n%s\nGo:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(want) != 12 {
		t.Fatalf("want 12 captures, one per field or keyword in the input, got %d:\n%s", len(want), strings.Join(want, "\n"))
	}
}

func TestEmitCPrimaryStateIDs(t *testing.T) {
	lang, err := GenerateLanguageForC(emitcQueryGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	lang.PrimaryStateIDs = make([]gotreesitter.StateID, lang.StateCount)
	for state := range lang.PrimaryStateIDs {
		lang.PrimaryStateIDs[state] = gotreesitter.StateID(state)
	}
	last := int(lang.StateCount) - 1
	lang.PrimaryStateIDs[last] = 1
	code, err := EmitC("emitc_query_tables", lang)
	if err != nil {
		t.Fatalf("EmitC: %v", err)
	}
	if !strings.Contains(code, "  ["+strconv.Itoa(last)+"] = 1,\n") {
		t.Errorf("ts_primary_state_ids does not carry the Language's primary for state %d", last)
	}
	lang.PrimaryStateIDs[last] = gotreesitter.StateID(lang.StateCount)
	if _, err := EmitC("emitc_query_tables", lang); err == nil || !strings.Contains(err.Error(), "primary state") {
		t.Errorf("EmitC accepted a primary state outside the table: %v", err)
	}
}

func TestEmitCRejectsAliasMapSymbolsOutsideTable(t *testing.T) {
	lang, err := GenerateLanguageForC(emitcQueryGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	lang.NonTerminalAliasMap = make([][]gotreesitter.Symbol, len(lang.SymbolNames))
	lang.NonTerminalAliasMap[len(lang.SymbolNames)-1] = []gotreesitter.Symbol{gotreesitter.Symbol(len(lang.SymbolNames))}
	if _, err := EmitC("emitc_query_tables", lang); err == nil || !strings.Contains(err.Error(), "alias map") {
		t.Errorf("EmitC accepted an alias map symbol outside the table: %v", err)
	}
}
