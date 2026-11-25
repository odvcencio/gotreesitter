package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// miniStrictParserC is miniParserC with the arrays its counts require: the
// small parse table of states 2 to 4, ts_primary_state_ids and ts_symbol_map.
var miniStrictParserC = strings.Replace(miniParserC, "static const TSParseActionEntry ts_parse_actions[] = {", `static const uint16_t ts_small_parse_table[] = {
  [0] = 1,
    ACTIONS(5), 1,
      anon_sym_RBRACE,
  [4] = 1,
    ACTIONS(7), 1,
      anon_sym_LBRACE,
  [8] = 1,
    STATE(1), 1,
      sym_object,
};

static const uint32_t ts_small_parse_table_map[] = {
  [SMALL_STATE(2)] = 0,
  [SMALL_STATE(3)] = 4,
  [SMALL_STATE(4)] = 8,
};

static const TSStateId ts_primary_state_ids[STATE_COUNT] = {
  [0] = 0,
  [1] = 1,
  [2] = 2,
  [3] = 3,
  [4] = 2,
};

static const TSSymbol ts_symbol_map[] = {
  [ts_builtin_sym_end] = ts_builtin_sym_end,
  [anon_sym_LBRACE] = anon_sym_LBRACE,
  [anon_sym_RBRACE] = anon_sym_RBRACE,
  [sym_number] = sym_number,
  [sym_document] = sym_document,
  [sym_object] = sym_object,
};

static const TSParseActionEntry ts_parse_actions[] = {`, 1)

func TestExtractGrammarStrictReadsPrimaryStatesAndSymbolMap(t *testing.T) {
	strict, err := ExtractGrammarStrict(miniStrictParserC)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{0, 1, 2, 3, 2}; !reflect.DeepEqual(strict.PrimaryStateIDs, want) {
		t.Errorf("PrimaryStateIDs = %v, want %v", strict.PrimaryStateIDs, want)
	}
	if want := []int{0, 1, 2, 3, 4, 5}; !reflect.DeepEqual(strict.SymbolMap, want) {
		t.Errorf("SymbolMap = %v, want %v", strict.SymbolMap, want)
	}
	// The strict reader reads the same tables as the default one.
	lenient, err := ExtractGrammar(miniStrictParserC)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(BuildLanguage(strict), BuildLanguage(lenient)) {
		t.Error("strict and default extraction build different languages")
	}
}

// TestExtractGrammarStrictFailsOnGaps: each edit makes the reader skip data.
// The default reader still succeeds (its behaviour is unchanged); the strict
// reader fails and says what it skipped.
func TestExtractGrammarStrictFailsOnGaps(t *testing.T) {
	cases := []struct {
		name, old, new, want string
	}{
		{"missing primary state ids", "ts_primary_state_ids[STATE_COUNT]", "ts_other_ids[STATE_COUNT]", "primary state ids"},
		{"missing symbol map", "ts_symbol_map[]", "ts_other_map[]", "symbol map"},
		{"short primary state ids", "  [4] = 2,\n};", "};", "no entry for index 4"},
		{"unresolved reduce symbol", "REDUCE(4, 1, 0)", "REDUCE(sym_missing, 1, 0)", "unresolved symbol sym_missing"},
		{"unknown action macro", "SHIFT_EXTRA(),", "SHIFT_ODD(),", "header count 1, 0 actions read"},
		{"unknown small table name", "      sym_object,\n", "      sym_missing,\n", "unread sym_missing"},
		{"missing lex mode", "  [4] = {.lex_state = 2},\n", "", "no entry for state 4"},
		{"missing small parse table", "ts_small_parse_table[] =", "ts_small_parse_other[] =", "small parse table"},
		{"missing define", "#define ALIAS_COUNT 0\n", "", "#define ALIAS_COUNT missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(miniStrictParserC, c.old) {
				t.Fatalf("fixture has no %q", c.old)
			}
			src := strings.Replace(miniStrictParserC, c.old, c.new, 1)
			if _, err := ExtractGrammar(src); err != nil {
				t.Fatalf("default reader: %v", err)
			}
			_, err := ExtractGrammarStrict(src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("strict reader: err %v, want it to name %q", err, c.want)
			}
		})
	}
}

func TestExtractGrammarStrictEndOfNonTerminalExtraLexMode(t *testing.T) {
	src := strings.Replace(miniStrictParserC, "[4] = {.lex_state = 2},", "[4] = {(TSStateId)(-1),},", 1)
	lenient, err := ExtractGrammar(src)
	if err != nil {
		t.Fatal(err)
	}
	strict, err := ExtractGrammarStrict(src)
	if err != nil {
		t.Fatal(err)
	}
	if lenient.LexModes[4].LexState != 0 || strict.LexModes[4].LexState != -1 {
		t.Fatalf("lex state of state 4: default %d (want 0, unchanged), strict %d (want -1)", lenient.LexModes[4].LexState, strict.LexModes[4].LexState)
	}
	if got := BuildLanguage(strict).LexModes[4].LexStateIndex(); got != ^uint32(0) {
		t.Fatalf("strict language: lex state index %d, want none", got)
	}
}

func TestRunTablebuildRecords(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "parser.c"), filepath.Join(dir, "records.txt")
	if err := os.WriteFile(in, []byte(miniStrictParserC), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunTablebuildRecords(in, out, "mini", "L4"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"L4\tcounts\tstates=5\tlarge=2\tsymbols=6\ttokens=4\texternals=0\tfields=2\tproductions=2\n",
		"L4\tstate\t0\t1=A3[S2]\t3=A4[]\n",
		"L4\tstate\t2\t2=A5[R4/1/0/0]\n",
		"L4\tstate\t4\t5=G1\n",
		"L4\tprimary-states\t0 1 2 3 2\n",
		"L4\tsymbol-map\t0 1 2 3 4 5\n",
		"L2\tproduction\t1\taliases=\tfields=2:1i\n",
		"L5\tstate\t2\tlex=1\texternal=0\treserved=0\t2=r\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("records lack %q", want)
		}
	}
	if t.Failed() {
		t.Logf("records:\n%s", text)
	}
	if err := RunTablebuildRecords(in, out, "mini", "L3"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(out)
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if !strings.HasPrefix(l, "L3\t") {
			t.Fatalf("L3 records hold %q", l)
		}
	}
}
