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
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      if (eof) ADVANCE(1);
      if (lookahead == ' ') SKIP(0);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(2);
      END_STATE();
    case 1:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();
    case 2:
      ACCEPT_TOKEN(sym_number);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(2);
      END_STATE();
  }
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

func TestExtractEnumAnonymous(t *testing.T) {
	src := `
enum {
  anon_sym_EQ = 1,
  sym_value = 2,
};
`
	vals := extractEnum(src)
	if v, ok := vals["anon_sym_EQ"]; !ok || v != 1 {
		t.Errorf("anon_sym_EQ = %d, ok=%v, want 1", v, ok)
	}
	if v, ok := vals["sym_value"]; !ok || v != 2 {
		t.Errorf("sym_value = %d, ok=%v, want 2", v, ok)
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

func TestExtractFieldMapsModernName(t *testing.T) {
	// Modern tree-sitter generators use "ts_field_map_entries" instead of "ts_field_map".
	modernSource := strings.Replace(miniParserC, "ts_field_map[]", "ts_field_map_entries[]", 1)

	g := miniGrammar()
	g.ProductionIDCount = 2
	g.FieldCount = 2

	if err := extractFieldMaps(modernSource, g); err != nil {
		t.Fatal(err)
	}

	if len(g.FieldMapEntries) != 2 {
		t.Fatalf("len(FieldMapEntries) = %d, want 2", len(g.FieldMapEntries))
	}
	if g.FieldMapEntries[0].FieldID != 1 {
		t.Errorf("FieldMapEntries[0].FieldID = %d, want 1", g.FieldMapEntries[0].FieldID)
	}
	if g.FieldMapEntries[1].FieldID != 2 {
		t.Errorf("FieldMapEntries[1].FieldID = %d, want 2", g.FieldMapEntries[1].FieldID)
	}
}

func TestExtractFieldMapsMixedInherited(t *testing.T) {
	// Real C grammars use mixed syntax: positional field_id and child_index
	// with named .inherited. e.g. {field_name, 0, .inherited = true}
	mixedSource := strings.Replace(miniParserC,
		`{field_value, 1, true}`,
		`{field_value, 1, .inherited = true}`, 1)

	g := miniGrammar()
	g.ProductionIDCount = 2
	g.FieldCount = 2

	if err := extractFieldMaps(mixedSource, g); err != nil {
		t.Fatal(err)
	}

	if len(g.FieldMapEntries) != 2 {
		t.Fatalf("len(FieldMapEntries) = %d, want 2", len(g.FieldMapEntries))
	}
	e := g.FieldMapEntries[1]
	if e.FieldID != 2 {
		t.Errorf("FieldMapEntries[1].FieldID = %d, want 2 (field_value)", e.FieldID)
	}
	if e.ChildIndex != 1 {
		t.Errorf("FieldMapEntries[1].ChildIndex = %d, want 1", e.ChildIndex)
	}
	if !e.Inherited {
		t.Error("FieldMapEntries[1].Inherited = false, want true")
	}
}

func TestExtractGrammarInfersLegacyProductionIDCount(t *testing.T) {
	legacySource := strings.Replace(miniParserC, "#define PRODUCTION_ID_COUNT 2\n", "", 1)
	legacySource = strings.Replace(legacySource, "ts_field_map_slices[PRODUCTION_ID_COUNT]", "ts_field_map_slices[2]", 1)
	legacySource = strings.Replace(legacySource, "static const uint16_t ts_parse_table", `static TSSymbol ts_alias_sequences[2][MAX_ALIAS_SEQUENCE_LENGTH] = {
  [0] = {[0] = sym_number},
  [1] = {[1] = sym_object},
};

static const uint16_t ts_parse_table`, 1)

	g, err := ExtractGrammar(legacySource)
	if err != nil {
		t.Fatal(err)
	}

	if g.ProductionIDCount != 2 {
		t.Fatalf("ProductionIDCount = %d, want 2", g.ProductionIDCount)
	}
	if len(g.FieldMapSlices) != 2 {
		t.Fatalf("len(FieldMapSlices) = %d, want 2", len(g.FieldMapSlices))
	}
	if len(g.FieldMapEntries) != 2 {
		t.Fatalf("len(FieldMapEntries) = %d, want 2", len(g.FieldMapEntries))
	}
	if len(g.AliasSequences) != 2 {
		t.Fatalf("len(AliasSequences) = %d, want 2", len(g.AliasSequences))
	}
	if got := g.AliasSequences[0][0]; got != 3 {
		t.Errorf("AliasSequences[0][0] = %d, want 3 (sym_number)", got)
	}
	if got := g.AliasSequences[1][1]; got != 5 {
		t.Errorf("AliasSequences[1][1] = %d, want 5 (sym_object)", got)
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

func TestExtractParseActionsNamedReduce(t *testing.T) {
	// Newer tree-sitter versions emit REDUCE with named arguments:
	// REDUCE(.symbol = X, .child_count = Y, .production_id = Z)
	src := `
#define STATE_COUNT 1
#define SYMBOL_COUNT 1
#define LARGE_STATE_COUNT 0
#define TOKEN_COUNT 1
#define PRODUCTION_ID_COUNT 1

enum ts_symbol_identifiers {
  sym_class_selector = 42,
};

static const char * const ts_symbol_names[] = {
  [0] = "x",
};

static const TSSymbolMetadata ts_symbol_metadata[] = {
  [0] = { .visible = false, .named = false, },
};

static const TSParseActionEntry ts_parse_actions[] = {
  [0] = {.entry = {.count = 0, .reusable = false}},
  [1] = {.entry = {.count = 1, .reusable = true}}, REDUCE(.symbol = sym_class_selector, .child_count = 2, .production_id = 2),
  [3] = {.entry = {.count = 2, .reusable = false}}, REDUCE(.symbol = sym_class_selector, .child_count = 2), SHIFT_REPEAT(16),
};

static const TSLexMode ts_lex_modes[STATE_COUNT] = {
  [0] = {.lex_state = 0},
};

const TSLanguage *tree_sitter_named(void) {
  return 0;
}
`
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.ParseActions) != 3 {
		t.Fatalf("len(ParseActions) = %d, want 3", len(g.ParseActions))
	}

	// Group 1: named REDUCE with production_id
	ag := g.ParseActions[1]
	if len(ag.Actions) != 1 {
		t.Fatalf("group[1] has %d actions, want 1", len(ag.Actions))
	}
	if ag.Actions[0].Type != "reduce" {
		t.Errorf("Actions[0].Type = %q, want reduce", ag.Actions[0].Type)
	}
	if ag.Actions[0].Symbol != 42 {
		t.Errorf("Actions[0].Symbol = %d, want 42", ag.Actions[0].Symbol)
	}
	if ag.Actions[0].ChildCount != 2 {
		t.Errorf("Actions[0].ChildCount = %d, want 2", ag.Actions[0].ChildCount)
	}
	if ag.Actions[0].ProductionID != 2 {
		t.Errorf("Actions[0].ProductionID = %d, want 2", ag.Actions[0].ProductionID)
	}

	// Group 2: named REDUCE + SHIFT_REPEAT (multi-action with named reduce)
	ag2 := g.ParseActions[2]
	if len(ag2.Actions) != 2 {
		t.Fatalf("group[2] has %d actions, want 2", len(ag2.Actions))
	}
	if ag2.Actions[0].Type != "reduce" {
		t.Errorf("group[2].Actions[0].Type = %q, want reduce", ag2.Actions[0].Type)
	}
	if ag2.Actions[0].Symbol != 42 {
		t.Errorf("group[2].Actions[0].Symbol = %d, want 42", ag2.Actions[0].Symbol)
	}
	if ag2.Actions[1].Type != "shift" {
		t.Errorf("group[2].Actions[1].Type = %q, want shift", ag2.Actions[1].Type)
	}
	if ag2.Actions[1].State != 16 {
		t.Errorf("group[2].Actions[1].State = %d, want 16", ag2.Actions[1].State)
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

func TestGenerateEmbeddedGo(t *testing.T) {
	g, err := ExtractGrammar(miniParserC)
	if err != nil {
		t.Fatal(err)
	}

	g.ExternalSymbols = []uint16{5}
	lang := BuildLanguage(g)
	if lang == nil {
		t.Fatal("BuildLanguage returned nil")
	}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob failed: %v", err)
	}
	if len(blob) == 0 {
		t.Fatal("EncodeLanguageBlob returned empty payload")
	}

	code := GenerateEmbeddedGo(g, "testpkg", "test_lang.bin")

	// Verify the generated code contains expected strings.
	checks := []string{
		"package testpkg",
		`import "github.com/odvcencio/gotreesitter"`,
		"func TestLangLanguage()",
		"*gotreesitter.Language",
		`loadEmbeddedLanguage("test_lang.bin")`,
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

func TestExtractLexModesReservedWordSetID(t *testing.T) {
	src := `
#define LANGUAGE_VERSION 15
#define STATE_COUNT 3
#define LARGE_STATE_COUNT 1
#define SYMBOL_COUNT 3
#define ALIAS_COUNT 0
#define TOKEN_COUNT 2
#define EXTERNAL_TOKEN_COUNT 0
#define FIELD_COUNT 0
#define MAX_ALIAS_SEQUENCE_LENGTH 1
#define PRODUCTION_ID_COUNT 1

enum ts_symbol_identifiers {
  sym_id = 1,
  sym_doc = 2,
};

static const char * const ts_symbol_names[] = {
  [0] = "end",
  [1] = "id",
  [2] = "doc",
};

static const TSSymbolMetadata ts_symbol_metadata[] = {
  [0] = { .visible = false, .named = true },
  [1] = { .visible = true, .named = true },
  [2] = { .visible = true, .named = true },
};

static const TSParseActionEntry ts_parse_actions[] = {
  [0] = {.entry = {.count = 0, .reusable = false}},
};

static const uint16_t ts_parse_table[LARGE_STATE_COUNT][SYMBOL_COUNT] = {
  [0] = {
    [1] = 0,
  },
};

static const TSLexMode ts_lex_modes[STATE_COUNT] = {
  [0] = {.lex_state = 0, .reserved_word_set_id = 1},
  [1] = {.lex_state = 1, .external_lex_state = 2, .reserved_word_set_id = 3},
  [2] = {.lex_state = 5},
};

static bool ts_lex(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();
  }
}

const TSLanguage *tree_sitter_rwtest(void) {
  static const TSLanguage language = { .version = LANGUAGE_VERSION };
  return &language;
}
`
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.LexModes) != 3 {
		t.Fatalf("len(LexModes) = %d, want 3", len(g.LexModes))
	}
	if g.LexModes[0].ReservedWordSetID != 1 {
		t.Errorf("LexModes[0].ReservedWordSetID = %d, want 1", g.LexModes[0].ReservedWordSetID)
	}
	if g.LexModes[1].ReservedWordSetID != 3 {
		t.Errorf("LexModes[1].ReservedWordSetID = %d, want 3", g.LexModes[1].ReservedWordSetID)
	}
	if g.LexModes[1].ExternalLexState != 2 {
		t.Errorf("LexModes[1].ExternalLexState = %d, want 2", g.LexModes[1].ExternalLexState)
	}
	if g.LexModes[2].ReservedWordSetID != 0 {
		t.Errorf("LexModes[2].ReservedWordSetID = %d, want 0", g.LexModes[2].ReservedWordSetID)
	}
}

func TestExtractReservedWords(t *testing.T) {
	src := `
enum ts_symbol_identifiers {
  sym_if = 1,
  sym_else = 2,
  sym_while = 3,
  sym_class = 4,
};

static const TSSymbol ts_reserved_words[3][5] = {
  [1] = {sym_if, sym_else, sym_while, 0, 0},
  [2] = {sym_class, 0, 0, 0, 0},
};
`
	g := &ExtractedGrammar{enumValues: extractEnum(src)}
	if err := extractReservedWords(src, g); err != nil {
		t.Fatal(err)
	}

	if g.MaxReservedWordSetSize != 5 {
		t.Errorf("MaxReservedWordSetSize = %d, want 5", g.MaxReservedWordSetSize)
	}
	if len(g.ReservedWords) != 15 {
		t.Fatalf("len(ReservedWords) = %d, want 15 (3*5)", len(g.ReservedWords))
	}

	// Set 0 should be all zeros (not initialized).
	for i := 0; i < 5; i++ {
		if g.ReservedWords[i] != 0 {
			t.Errorf("ReservedWords[%d] = %d, want 0", i, g.ReservedWords[i])
		}
	}
	// Set 1: {sym_if=1, sym_else=2, sym_while=3, 0, 0}
	if g.ReservedWords[5] != 1 {
		t.Errorf("ReservedWords[5] = %d, want 1 (sym_if)", g.ReservedWords[5])
	}
	if g.ReservedWords[6] != 2 {
		t.Errorf("ReservedWords[6] = %d, want 2 (sym_else)", g.ReservedWords[6])
	}
	if g.ReservedWords[7] != 3 {
		t.Errorf("ReservedWords[7] = %d, want 3 (sym_while)", g.ReservedWords[7])
	}
	// Set 2: {sym_class=4, 0, 0, 0, 0}
	if g.ReservedWords[10] != 4 {
		t.Errorf("ReservedWords[10] = %d, want 4 (sym_class)", g.ReservedWords[10])
	}
}

func TestExtractReservedWordsAbsent(t *testing.T) {
	// ABI 14 grammar without reserved words — should not error.
	g := &ExtractedGrammar{enumValues: map[string]int{}}
	if err := extractReservedWords(miniParserC, g); err != nil {
		t.Fatal(err)
	}
	if g.ReservedWords != nil {
		t.Error("ReservedWords should be nil for ABI < 15")
	}
}

func TestExtractSupertypes(t *testing.T) {
	src := `
#define SUPERTYPE_COUNT 2

enum ts_symbol_identifiers {
  sym__expression = 10,
  sym__statement = 20,
  sym_binary = 11,
  sym_call = 12,
  sym_assign = 21,
};

static const TSSymbol ts_supertype_symbols[SUPERTYPE_COUNT] = {
  [0] = sym__expression,
  [1] = sym__statement,
};

static const TSMapSlice ts_supertype_map_slices[] = {
  [sym__expression] = {.index = 0, .length = 2},
  [sym__statement] = {.index = 2, .length = 1},
};

static const TSSymbol ts_supertype_map_entries[] = {
  sym_binary,
  sym_call,
  sym_assign,
};
`
	g := &ExtractedGrammar{
		SupertypeCount: 2,
		enumValues:     extractEnum(src),
	}
	if err := extractSupertypes(src, g); err != nil {
		t.Fatal(err)
	}

	// Supertype symbols.
	if len(g.SupertypeSymbols) != 2 {
		t.Fatalf("len(SupertypeSymbols) = %d, want 2", len(g.SupertypeSymbols))
	}
	if g.SupertypeSymbols[0] != 10 {
		t.Errorf("SupertypeSymbols[0] = %d, want 10 (sym__expression)", g.SupertypeSymbols[0])
	}
	if g.SupertypeSymbols[1] != 20 {
		t.Errorf("SupertypeSymbols[1] = %d, want 20 (sym__statement)", g.SupertypeSymbols[1])
	}

	// Supertype map slices — indexed by symbol ID.
	if len(g.SupertypeMapSlices) < 21 {
		t.Fatalf("len(SupertypeMapSlices) = %d, want at least 21", len(g.SupertypeMapSlices))
	}
	if g.SupertypeMapSlices[10] != [2]uint16{0, 2} {
		t.Errorf("SupertypeMapSlices[10] = %v, want [0 2]", g.SupertypeMapSlices[10])
	}
	if g.SupertypeMapSlices[20] != [2]uint16{2, 1} {
		t.Errorf("SupertypeMapSlices[20] = %v, want [2 1]", g.SupertypeMapSlices[20])
	}

	// Supertype map entries.
	if len(g.SupertypeMapEntries) != 3 {
		t.Fatalf("len(SupertypeMapEntries) = %d, want 3", len(g.SupertypeMapEntries))
	}
	if g.SupertypeMapEntries[0] != 11 {
		t.Errorf("SupertypeMapEntries[0] = %d, want 11 (sym_binary)", g.SupertypeMapEntries[0])
	}
	if g.SupertypeMapEntries[1] != 12 {
		t.Errorf("SupertypeMapEntries[1] = %d, want 12 (sym_call)", g.SupertypeMapEntries[1])
	}
	if g.SupertypeMapEntries[2] != 21 {
		t.Errorf("SupertypeMapEntries[2] = %d, want 21 (sym_assign)", g.SupertypeMapEntries[2])
	}
}

func TestExtractSupertypesAbsent(t *testing.T) {
	// Grammar without SUPERTYPE_COUNT — should not error.
	g := &ExtractedGrammar{
		SupertypeCount: 0,
		enumValues:     map[string]int{},
	}
	if err := extractSupertypes("", g); err != nil {
		t.Fatal(err)
	}
	if g.SupertypeSymbols != nil {
		t.Error("SupertypeSymbols should be nil when SupertypeCount=0")
	}
}

func TestExtractLanguageMetadata(t *testing.T) {
	src := `
const TSLanguage *tree_sitter_example(void) {
  static const TSLanguage language = {
    .version = 15,
    .metadata = {
      .major_version = 1,
      .minor_version = 23,
      .patch_version = 5,
    },
  };
  return &language;
}
`
	g := &ExtractedGrammar{}
	if err := extractLanguageMetadata(src, g); err != nil {
		t.Fatal(err)
	}
	if g.LanguageMetadataMajor != 1 {
		t.Errorf("LanguageMetadataMajor = %d, want 1", g.LanguageMetadataMajor)
	}
	if g.LanguageMetadataMinor != 23 {
		t.Errorf("LanguageMetadataMinor = %d, want 23", g.LanguageMetadataMinor)
	}
	if g.LanguageMetadataPatch != 5 {
		t.Errorf("LanguageMetadataPatch = %d, want 5", g.LanguageMetadataPatch)
	}
}

func TestExtractLanguageMetadataAbsent(t *testing.T) {
	// ABI 14 grammar without .metadata — should not error.
	g := &ExtractedGrammar{}
	if err := extractLanguageMetadata(miniParserC, g); err != nil {
		t.Fatal(err)
	}
	if g.LanguageMetadataMajor != 0 || g.LanguageMetadataMinor != 0 || g.LanguageMetadataPatch != 0 {
		t.Error("metadata should be zeroed for ABI < 15")
	}
}

func TestExtractSupertypeCount(t *testing.T) {
	src := `
#define STATE_COUNT 1
#define SYMBOL_COUNT 1
#define SUPERTYPE_COUNT 7
`
	g := &ExtractedGrammar{}
	if err := extractConstants(src, g); err != nil {
		t.Fatal(err)
	}
	if g.SupertypeCount != 7 {
		t.Errorf("SupertypeCount = %d, want 7", g.SupertypeCount)
	}
}

func TestExtractGrammarABI14NoCrash(t *testing.T) {
	// Ensure ABI 14 miniParserC still works with the new extraction pipeline.
	g, err := ExtractGrammar(miniParserC)
	if err != nil {
		t.Fatal(err)
	}
	if g.ReservedWords != nil {
		t.Error("ReservedWords should be nil for ABI 14")
	}
	if g.SupertypeSymbols != nil {
		t.Error("SupertypeSymbols should be nil for ABI 14")
	}
	if g.LanguageMetadataMajor != 0 {
		t.Error("LanguageMetadataMajor should be 0 for ABI 14")
	}
}
