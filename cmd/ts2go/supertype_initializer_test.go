package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestSupertypeDesignatedInitializers(t *testing.T) {
	enums := map[string]int{"sym_a": 11, "sym_b": 12, "sym_alias": 21}
	cases := []struct {
		name, body string
		want       []uint16
		bad        bool
	}{
		{"positional", "sym_a, sym_b, sym_alias,", []uint16{11, 12, 21}, false},
		{"mixed-and-gap", "[2] = sym_a, sym_b, /* gap */ [6] = sym_alias,", []uint16{0, 0, 11, 12, 0, 0, 21}, false},
		{"overwrite-and-continue", "sym_a, sym_b, [0] = sym_alias, sym_a,", []uint16{21, 11}, false},
		{"parenthesized", "[(2)] = (sym_a), +sym_b,", []uint16{0, 0, 11, 12}, false},
		{"integer-literals", "[0x2] = 0x10, 011,", []uint16{0, 0, 16, 9}, false},
		{"unknown-symbol", "[1] = missing,", nil, true},
		{"missing-equals", "[1] sym_a,", nil, true},
		{"adjacent-increment", "++sym_a,", nil, true},
		{"deep-prefix", strings.Repeat("(", 258) + "sym_a" + strings.Repeat(")", 258), nil, true},
		{"missing-comma", "sym_a sym_b", nil, true},
		{"out-of-range-symbol", "65536,", nil, true},
		{"out-of-range-index", "[131070] = sym_a,", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSupertypeEntries(tc.body, enums)
			if tc.bad {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractSupertypesDesignatedEntries(t *testing.T) {
	source := `static const TSSymbol ts_supertype_symbols[] = { sym_parent, };
static const TSMapSlice ts_supertype_map_slices[] = { [sym_parent] = {.index = 2, .length = 2}, };
static const TSSymbol ts_supertype_map_entries[] = { [2] = sym_first, sym_second, };`
	grammar := &ExtractedGrammar{SupertypeCount: 1, enumValues: map[string]int{"sym_parent": 1, "sym_first": 11, "sym_second": 12}}
	if err := extractSupertypes(source, grammar); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(grammar.SupertypeMapEntries, []uint16{0, 0, 11, 12}) {
		t.Fatalf("entries=%v", grammar.SupertypeMapEntries)
	}
	if got := grammar.SupertypeMapSlices[1]; got != ([2]uint16{2, 2}) {
		t.Fatalf("slice=%v", got)
	}
	if len(grammar.SupertypeSymbols) != 1 || grammar.SupertypeSymbols[0] != 1 {
		t.Fatalf("symbols=%v", grammar.SupertypeSymbols)
	}
}

func TestExtractGrammarRejectsMalformedSupertypeEntries(t *testing.T) {
	source := strings.Replace(miniParserC, "#define LANGUAGE_VERSION 14", "#define LANGUAGE_VERSION 15\n#define SUPERTYPE_COUNT 1", 1)
	source += `
static const TSSymbol ts_supertype_symbols[] = { 1, };
static const TSMapSlice ts_supertype_map_slices[] = { [1] = {.index = 0, .length = 1}, };
static const TSSymbol ts_supertype_map_entries[] = { missing_symbol, };`
	if _, err := ExtractGrammar(source); err == nil || !strings.Contains(err.Error(), "supertype entries") {
		t.Fatalf("expected supertype error, got %v", err)
	}
}
