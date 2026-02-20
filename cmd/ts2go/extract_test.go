package main

import (
	"strings"
	"testing"
)

// miniParserC is a minimal tree-sitter parser.c snippet for testing.
const miniParserC = `
#include <tree_sitter/parser.h>

#define LANGUAGE_VERSION 14
#define STATE_COUNT 5
#define LARGE_STATE_COUNT 2
#define SYMBOL_COUNT 6
#define ALIAS_COUNT 0
#define TOKEN_COUNT 4
#define EXTERNAL_TOKEN_COUNT 0
#define FIELD_COUNT 2
#define MAX_ALIAS_SEQUENCE_LENGTH 3
#define PRODUCTION_ID_COUNT 2

enum ts_symbol_identifiers {
  anon_sym_LBRACE = 1,
  anon_sym_RBRACE = 2,
  sym_number = 3,
  sym_document = 4,
  sym_object = 5,
};

enum ts_field_identifiers {
  field_key = 1,
  field_value = 2,
};

static const char * const ts_symbol_names[] = {
  [ts_builtin_sym_end] = "end",
  [anon_sym_LBRACE] = "{",
  [anon_sym_RBRACE] = "}",
  [sym_number] = "number",
  [sym_document] = "document",
  [sym_object] = "object",
};

static const TSSymbolMetadata ts_symbol_metadata[] = {
  [ts_builtin_sym_end] = {
    .visible = false,
    .named = true,
  },
  [anon_sym_LBRACE] = {
    .visible = true,
    .named = false,
  },
  [anon_sym_RBRACE] = {
    .visible = true,
    .named = false,
  },
  [sym_number] = {
    .visible = true,
    .named = true,
  },
  [sym_document] = {
    .visible = true,
    .named = true,
  },
  [sym_object] = {
    .visible = true,
    .named = true,
  },
};

static const char * const ts_field_names[] = {
  [0] = NULL,
  [1] = "key",
  [2] = "value",
};

static const TSFieldMapSlice ts_field_map_slices[PRODUCTION_ID_COUNT] = {
  [0] = {.index = 0, .length = 1},
  [1] = {1, 1},
};

static const TSFieldMapEntry ts_field_map[] = {
  [0] = {.field_id = field_key, .child_index = 0, .inherited = false},
  [1] = {field_value, 1, true},
};

static const uint16_t ts_parse_table[LARGE_STATE_COUNT][SYMBOL_COUNT] = {
  [0] = {
    [1] = 3,
    [3] = 4,
  },
  [1] = {
    [0] = 1,
  },
};

static const TSParseActionEntry ts_parse_actions[] = {
  [0] = {.entry = {.count = 0, .reusable = false}},
  [1] = {.entry = {.count = 1, .reusable = true}}, ACCEPT_INPUT(),
  [3] = {.entry = {.count = 1, .reusable = true}}, SHIFT(2),
  [5] = {.entry = {.count = 1, .reusable = true}}, REDUCE(4, 1, 0),
  [7] = {.entry = {.count = 1, .reusable = false}}, SHIFT_EXTRA(),
};

static const TSLexMode ts_lex_modes[STATE_COUNT] = {
  [0] = {.lex_state = 0},
  [1] = {.lex_state = 0},
  [2] = {.lex_state = 1},
  [3] = {.lex_state = 0, .external_lex_state = 1},
  [4] = {.lex_state = 2},
};

static bool ts_lex(TSLexer *lexer, TSStateId state) {
  // ... complex lex function we skip ...
  return false;
}

const TSLanguage *tree_sitter_test_lang(void) {
  static const TSLanguage language = { .version = LANGUAGE_VERSION };
  return &language;
}
`

// miniGrammar returns an ExtractedGrammar with enum values pre-populated,
// for use by tests that call individual extraction functions.
func miniGrammar() *ExtractedGrammar {
	g := &ExtractedGrammar{}
	g.enumValues = extractEnum(miniParserC)
	return g
}

func TestExtractConstants(t *testing.T) {
	g := miniGrammar()
	if err := extractConstants(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	if g.StateCount != 5 {
		t.Errorf("StateCount = %d, want 5", g.StateCount)
	}
	if g.LargeStateCount != 2 {
		t.Errorf("LargeStateCount = %d, want 2", g.LargeStateCount)
	}
	if g.SymbolCount != 6 {
		t.Errorf("SymbolCount = %d, want 6", g.SymbolCount)
	}
	if g.TokenCount != 4 {
		t.Errorf("TokenCount = %d, want 4", g.TokenCount)
	}
	if g.FieldCount != 2 {
		t.Errorf("FieldCount = %d, want 2", g.FieldCount)
	}
	if g.ProductionIDCount != 2 {
		t.Errorf("ProductionIDCount = %d, want 2", g.ProductionIDCount)
	}
	if g.ExternalTokenCount != 0 {
		t.Errorf("ExternalTokenCount = %d, want 0", g.ExternalTokenCount)
	}
}

func TestExtractEnum(t *testing.T) {
	vals := extractEnum(miniParserC)

	// Built-in.
	if v, ok := vals["ts_builtin_sym_end"]; !ok || v != 0 {
		t.Errorf("ts_builtin_sym_end = %d, ok=%v, want 0", v, ok)
	}

	// From the enum block.
	if v, ok := vals["anon_sym_LBRACE"]; !ok || v != 1 {
		t.Errorf("anon_sym_LBRACE = %d, ok=%v, want 1", v, ok)
	}
	if v, ok := vals["sym_number"]; !ok || v != 3 {
		t.Errorf("sym_number = %d, ok=%v, want 3", v, ok)
	}
	if v, ok := vals["sym_object"]; !ok || v != 5 {
		t.Errorf("sym_object = %d, ok=%v, want 5", v, ok)
	}
}

func TestExtractLanguageName(t *testing.T) {
	g := miniGrammar()
	if err := extractLanguageName(miniParserC, g); err != nil {
		t.Fatal(err)
	}
	if g.Name != "test_lang" {
		t.Errorf("Name = %q, want %q", g.Name, "test_lang")
	}
}

func TestExtractSymbolNames(t *testing.T) {
	g := miniGrammar()
	g.SymbolCount = 6
	if err := extractSymbolNames(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	want := []string{"end", "{", "}", "number", "document", "object"}
	if len(g.SymbolNames) != len(want) {
		t.Fatalf("len(SymbolNames) = %d, want %d", len(g.SymbolNames), len(want))
	}
	for i, w := range want {
		if g.SymbolNames[i] != w {
			t.Errorf("SymbolNames[%d] = %q, want %q", i, g.SymbolNames[i], w)
		}
	}
}

func TestExtractSymbolMetadata(t *testing.T) {
	g := miniGrammar()
	g.SymbolCount = 6
	if err := extractSymbolMetadata(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	if len(g.SymbolMetadata) != 6 {
		t.Fatalf("len(SymbolMetadata) = %d, want 6", len(g.SymbolMetadata))
	}

	// Index 0: end — visible=false, named=true
	if g.SymbolMetadata[0].Visible != false {
		t.Error("meta[0].Visible should be false")
	}
	if g.SymbolMetadata[0].Named != true {
		t.Error("meta[0].Named should be true")
	}

	// Index 1: { — visible=true, named=false
	if g.SymbolMetadata[1].Visible != true {
		t.Error("meta[1].Visible should be true")
	}
	if g.SymbolMetadata[1].Named != false {
		t.Error("meta[1].Named should be false")
	}

	// Index 3: number — visible=true, named=true
	if g.SymbolMetadata[3].Visible != true {
		t.Error("meta[3].Visible should be true")
	}
	if g.SymbolMetadata[3].Named != true {
		t.Error("meta[3].Named should be true")
	}
}

func TestExtractFieldNames(t *testing.T) {
	g := miniGrammar()
	g.FieldCount = 2
	if err := extractFieldNames(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	// Index 0 is empty (NULL), 1="key", 2="value"
	if len(g.FieldNames) != 3 {
		t.Fatalf("len(FieldNames) = %d, want 3", len(g.FieldNames))
	}
	if g.FieldNames[0] != "" {
		t.Errorf("FieldNames[0] = %q, want empty", g.FieldNames[0])
	}
	if g.FieldNames[1] != "key" {
		t.Errorf("FieldNames[1] = %q, want %q", g.FieldNames[1], "key")
	}
	if g.FieldNames[2] != "value" {
		t.Errorf("FieldNames[2] = %q, want %q", g.FieldNames[2], "value")
	}
}

func TestExtractFieldMaps(t *testing.T) {
	g := miniGrammar()
	g.ProductionIDCount = 2
	g.FieldCount = 2

	if err := extractFieldMaps(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	if len(g.FieldMapSlices) != 2 {
		t.Fatalf("len(FieldMapSlices) = %d, want 2", len(g.FieldMapSlices))
	}
	if g.FieldMapSlices[0] != [2]uint16{0, 1} {
		t.Errorf("FieldMapSlices[0] = %v, want [0 1]", g.FieldMapSlices[0])
	}
	if g.FieldMapSlices[1] != [2]uint16{1, 1} {
		t.Errorf("FieldMapSlices[1] = %v, want [1 1]", g.FieldMapSlices[1])
	}

	if len(g.FieldMapEntries) != 2 {
		t.Fatalf("len(FieldMapEntries) = %d, want 2", len(g.FieldMapEntries))
	}

	if g.FieldMapEntries[0].FieldID != 1 {
		t.Errorf("FieldMapEntries[0].FieldID = %d, want 1", g.FieldMapEntries[0].FieldID)
	}
	if g.FieldMapEntries[0].ChildIndex != 0 {
		t.Errorf("FieldMapEntries[0].ChildIndex = %d, want 0", g.FieldMapEntries[0].ChildIndex)
	}
	if g.FieldMapEntries[0].Inherited {
		t.Error("FieldMapEntries[0].Inherited = true, want false")
	}

	if g.FieldMapEntries[1].FieldID != 2 {
		t.Errorf("FieldMapEntries[1].FieldID = %d, want 2", g.FieldMapEntries[1].FieldID)
	}
	if g.FieldMapEntries[1].ChildIndex != 1 {
		t.Errorf("FieldMapEntries[1].ChildIndex = %d, want 1", g.FieldMapEntries[1].ChildIndex)
	}
	if !g.FieldMapEntries[1].Inherited {
		t.Error("FieldMapEntries[1].Inherited = false, want true")
	}
}

func TestExtractParseTable(t *testing.T) {
	g := miniGrammar()
	g.LargeStateCount = 2
	g.SymbolCount = 6
	if err := extractParseTable(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	if len(g.ParseTable) != 2 {
		t.Fatalf("len(ParseTable) = %d, want 2", len(g.ParseTable))
	}

	// State 0: [1]=3, [3]=4
	if g.ParseTable[0][1] != 3 {
		t.Errorf("ParseTable[0][1] = %d, want 3", g.ParseTable[0][1])
	}
	if g.ParseTable[0][3] != 4 {
		t.Errorf("ParseTable[0][3] = %d, want 4", g.ParseTable[0][3])
	}

	// State 1: [0]=1
	if g.ParseTable[1][0] != 1 {
		t.Errorf("ParseTable[1][0] = %d, want 1", g.ParseTable[1][0])
	}
}

func TestExtractParseActions(t *testing.T) {
	g := miniGrammar()
	if err := extractParseActions(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	if len(g.ParseActions) < 4 {
		t.Fatalf("len(ParseActions) = %d, want at least 4", len(g.ParseActions))
	}

	// Group 0: count=0, reusable=false, no actions (error recovery sentinel)
	if g.ParseActions[0].Count != 0 {
		t.Errorf("group[0].Count = %d, want 0", g.ParseActions[0].Count)
	}
	if g.ParseActions[0].Reusable != false {
		t.Error("group[0].Reusable should be false")
	}

	// Group 1: count=1, reusable=true, ACCEPT_INPUT()
	if g.ParseActions[1].Count != 1 {
		t.Errorf("group[1].Count = %d, want 1", g.ParseActions[1].Count)
	}
	if g.ParseActions[1].Reusable != true {
		t.Error("group[1].Reusable should be true")
	}
	if len(g.ParseActions[1].Actions) != 1 {
		t.Fatalf("group[1] has %d actions, want 1", len(g.ParseActions[1].Actions))
	}
	if g.ParseActions[1].Actions[0].Type != "accept" {
		t.Errorf("group[1].Actions[0].Type = %q, want %q", g.ParseActions[1].Actions[0].Type, "accept")
	}

	// Group 2: count=1, reusable=true, SHIFT(2)
	if len(g.ParseActions[2].Actions) != 1 {
		t.Fatalf("group[2] has %d actions, want 1", len(g.ParseActions[2].Actions))
	}
	if g.ParseActions[2].Actions[0].Type != "shift" {
		t.Errorf("group[2].Actions[0].Type = %q, want %q", g.ParseActions[2].Actions[0].Type, "shift")
	}
	if g.ParseActions[2].Actions[0].State != 2 {
		t.Errorf("group[2].Actions[0].State = %d, want 2", g.ParseActions[2].Actions[0].State)
	}

	// Group 3: count=1, reusable=true, REDUCE(4, 1, 0)
	if len(g.ParseActions[3].Actions) != 1 {
		t.Fatalf("group[3] has %d actions, want 1", len(g.ParseActions[3].Actions))
	}
	if g.ParseActions[3].Actions[0].Type != "reduce" {
		t.Errorf("group[3].Actions[0].Type = %q, want %q", g.ParseActions[3].Actions[0].Type, "reduce")
	}
	if g.ParseActions[3].Actions[0].Symbol != 4 {
		t.Errorf("group[3].Actions[0].Symbol = %d, want 4", g.ParseActions[3].Actions[0].Symbol)
	}
	if g.ParseActions[3].Actions[0].ChildCount != 1 {
		t.Errorf("group[3].Actions[0].ChildCount = %d, want 1", g.ParseActions[3].Actions[0].ChildCount)
	}

	// Group 4: count=1, reusable=false, SHIFT_EXTRA()
	if len(g.ParseActions[4].Actions) != 1 {
		t.Fatalf("group[4] has %d actions, want 1", len(g.ParseActions[4].Actions))
	}
	if g.ParseActions[4].Actions[0].Type != "shift" {
		t.Errorf("group[4].Actions[0].Type = %q, want %q", g.ParseActions[4].Actions[0].Type, "shift")
	}
	if g.ParseActions[4].Actions[0].Extra != true {
		t.Error("group[4].Actions[0].Extra should be true")
	}
}

func TestExtractParseActionsMultiAction(t *testing.T) {
	// Test a line with multiple actions (header + REDUCE + SHIFT_REPEAT).
	src := `
#define STATE_COUNT 1
#define SYMBOL_COUNT 1
#define LARGE_STATE_COUNT 0
#define TOKEN_COUNT 1
#define PRODUCTION_ID_COUNT 1

enum ts_symbol_identifiers {
  aux_sym_document_repeat1 = 22,
};

static const char * const ts_symbol_names[] = {
  [0] = "x",
};

static const TSSymbolMetadata ts_symbol_metadata[] = {
  [0] = { .visible = false, .named = false, },
};

static const TSParseActionEntry ts_parse_actions[] = {
  [0] = {.entry = {.count = 2, .reusable = true}}, REDUCE(aux_sym_document_repeat1, 2, 0, 0), SHIFT_REPEAT(16),
};

static const TSLexMode ts_lex_modes[STATE_COUNT] = {
  [0] = {.lex_state = 0},
};

const TSLanguage *tree_sitter_multi(void) {
  return 0;
}
`
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.ParseActions) != 1 {
		t.Fatalf("len(ParseActions) = %d, want 1", len(g.ParseActions))
	}

	ag := g.ParseActions[0]
	if ag.Count != 2 {
		t.Errorf("Count = %d, want 2", ag.Count)
	}
	if len(ag.Actions) != 2 {
		t.Fatalf("len(Actions) = %d, want 2", len(ag.Actions))
	}
	if ag.Actions[0].Type != "reduce" {
		t.Errorf("Actions[0].Type = %q, want reduce", ag.Actions[0].Type)
	}
	if ag.Actions[0].Symbol != 22 {
		t.Errorf("Actions[0].Symbol = %d, want 22", ag.Actions[0].Symbol)
	}
	if ag.Actions[1].Type != "shift" {
		t.Errorf("Actions[1].Type = %q, want shift", ag.Actions[1].Type)
	}
	if ag.Actions[1].State != 16 {
		t.Errorf("Actions[1].State = %d, want 16", ag.Actions[1].State)
	}
	if ag.Actions[1].Repetition != true {
		t.Error("Actions[1].Repetition should be true")
	}
}

func TestExtractLexModes(t *testing.T) {
	g := miniGrammar()
	g.StateCount = 5
	if err := extractLexModes(miniParserC, g); err != nil {
		t.Fatal(err)
	}

	if len(g.LexModes) != 5 {
		t.Fatalf("len(LexModes) = %d, want 5", len(g.LexModes))
	}

	if g.LexModes[0].LexState != 0 {
		t.Errorf("LexModes[0].LexState = %d, want 0", g.LexModes[0].LexState)
	}
	if g.LexModes[2].LexState != 1 {
		t.Errorf("LexModes[2].LexState = %d, want 1", g.LexModes[2].LexState)
	}
	if g.LexModes[3].ExternalLexState != 1 {
		t.Errorf("LexModes[3].ExternalLexState = %d, want 1", g.LexModes[3].ExternalLexState)
	}
	if g.LexModes[4].LexState != 2 {
		t.Errorf("LexModes[4].LexState = %d, want 2", g.LexModes[4].LexState)
	}
}

func TestExtractExternalSymbols(t *testing.T) {
	src := `
enum ts_symbol_identifiers {
  sym_a = 1,
  sym_ext = 2,
};
enum ts_external_token_identifiers {
  ext_tok_one = 0,
  ext_tok_two = 1,
};
static const TSSymbol ts_external_scanner_symbol_map[2] = {
  [ext_tok_one] = sym_ext,
  [ext_tok_two] = sym_a,
};
`
	g := &ExtractedGrammar{
		ExternalTokenCount: 2,
		enumValues:         extractEnum(src),
	}
	if err := extractExternalSymbols(src, g); err != nil {
		t.Fatal(err)
	}
	if len(g.ExternalSymbols) != 2 {
		t.Fatalf("len(ExternalSymbols) = %d, want 2", len(g.ExternalSymbols))
	}
	if g.ExternalSymbols[0] != 2 {
		t.Errorf("ExternalSymbols[0] = %d, want 2", g.ExternalSymbols[0])
	}
	if g.ExternalSymbols[1] != 1 {
		t.Errorf("ExternalSymbols[1] = %d, want 1", g.ExternalSymbols[1])
	}
}

func TestExtractGrammarFull(t *testing.T) {
	g, err := ExtractGrammar(miniParserC)
	if err != nil {
		t.Fatal(err)
	}

	if g.Name != "test_lang" {
		t.Errorf("Name = %q, want %q", g.Name, "test_lang")
	}
	if g.StateCount != 5 {
		t.Errorf("StateCount = %d, want 5", g.StateCount)
	}
	if g.SymbolCount != 6 {
		t.Errorf("SymbolCount = %d, want 6", g.SymbolCount)
	}
	if len(g.SymbolNames) != 6 {
		t.Errorf("len(SymbolNames) = %d, want 6", len(g.SymbolNames))
	}
	if len(g.FieldMapSlices) != 2 {
		t.Errorf("len(FieldMapSlices) = %d, want 2", len(g.FieldMapSlices))
	}
	if len(g.FieldMapEntries) != 2 {
		t.Errorf("len(FieldMapEntries) = %d, want 2", len(g.FieldMapEntries))
	}
	if len(g.ParseActions) < 4 {
		t.Errorf("len(ParseActions) = %d, want >= 4", len(g.ParseActions))
	}
}

func TestGenerateGo(t *testing.T) {
	g, err := ExtractGrammar(miniParserC)
	if err != nil {
		t.Fatal(err)
	}

	g.ExternalSymbols = []uint16{5}
	code := GenerateGo(g, "testpkg")

	// Verify the generated code contains expected strings.
	checks := []string{
		"package testpkg",
		`import "github.com/odvcencio/mane/gotreesitter"`,
		"func TestLangLanguage()",
		"*gotreesitter.Language",
		`Name: "test_lang"`,
		"SymbolCount:        6",
		"StateCount:         5",
		"ParseActionShift",
		"ParseActionAccept",
		"ParseActionReduce",
		"FieldMapSlices",
		"FieldMapEntries",
		"ExternalSymbols",
		`"number"`,
		`"end"`,
	}

	for _, check := range checks {
		if !strings.Contains(code, check) {
			t.Errorf("generated code missing %q", check)
		}
	}

	// Verify it doesn't contain obvious errors.
	if strings.Contains(code, "INVALID") {
		t.Error("generated code contains INVALID")
	}
}

func TestFindArrayBody(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		array   string
		want    string
		wantErr bool
	}{
		{
			name:   "simple array",
			source: `static int ts_foo[] = { 1, 2, 3 };`,
			array:  "ts_foo",
			want:   " 1, 2, 3 ",
		},
		{
			name:   "nested braces",
			source: `static int ts_bar[] = { {1, 2}, {3, 4} };`,
			array:  "ts_bar",
			want:   " {1, 2}, {3, 4} ",
		},
		{
			name:    "not found",
			source:  `static int ts_baz[] = { 1 };`,
			array:   "ts_missing",
			wantErr: true,
		},
		{
			name:   "2D array",
			source: `static int ts_table[2][3] = { {1, 2, 3}, {4, 5, 6} };`,
			array:  "ts_table",
			want:   " {1, 2, 3}, {4, 5, 6} ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findArrayBody(tt.source, tt.array)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindExactArrayBody(t *testing.T) {
	source := `
static int ts_small_parse_table[] = { 1, 2, 3 };
static int ts_small_parse_table_map[] = { 10, 20, 30 };
`
	// findArrayBody for "ts_small_parse_table" should find the first one.
	body, err := findExactArrayBody(source, "ts_small_parse_table")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "1, 2, 3") {
		t.Errorf("expected body to contain '1, 2, 3', got %q", body)
	}
	if strings.Contains(body, "10, 20, 30") {
		t.Error("body should not contain the map values")
	}

	// Also find the map.
	mapBody, err := findArrayBody(source, "ts_small_parse_table_map")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mapBody, "10, 20, 30") {
		t.Errorf("expected map body to contain '10, 20, 30', got %q", mapBody)
	}
}

func TestUnescapeCString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello`, "hello"},
		{`hello\"world`, `hello"world`},
		{`line\nbreak`, "line\nbreak"},
		{`tab\there`, "tab\there"},
		{`back\\slash`, `back\slash`},
	}

	for _, tt := range tests {
		got := unescapeCString(tt.input)
		if got != tt.want {
			t.Errorf("unescapeCString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseUint16List(t *testing.T) {
	vals := parseUint16List("1, 2, 3, 100, 65535")
	want := []uint16{1, 2, 3, 100, 65535}
	if len(vals) != len(want) {
		t.Fatalf("len = %d, want %d", len(vals), len(want))
	}
	for i, v := range vals {
		if v != want[i] {
			t.Errorf("[%d] = %d, want %d", i, v, want[i])
		}
	}
}

func TestResolveSymbol(t *testing.T) {
	g := miniGrammar()

	// Numeric.
	v, ok := g.resolveSymbol("42")
	if !ok || v != 42 {
		t.Errorf("resolveSymbol(42) = %d, %v", v, ok)
	}

	// Enum name.
	v, ok = g.resolveSymbol("sym_number")
	if !ok || v != 3 {
		t.Errorf("resolveSymbol(sym_number) = %d, %v", v, ok)
	}

	// Unknown.
	_, ok = g.resolveSymbol("totally_unknown")
	if ok {
		t.Error("resolveSymbol(totally_unknown) should return false")
	}
}
