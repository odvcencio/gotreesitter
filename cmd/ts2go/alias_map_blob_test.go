package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestUpdateNonTerminalAliasMapBlob(t *testing.T) {
	source := `
#define STATE_COUNT 1
#define SYMBOL_COUNT 3
#define ALIAS_COUNT 0
static const char * const ts_symbol_names[] = {
 [0] = "end", [1] = "_wrapper", [2] = "expr",
};
static const uint16_t ts_non_terminal_alias_map[] = { 1, 2, 1, 2, 0 };
`
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserves-tables", true: "rejects-symbol-mismatch"}[mismatch], func(t *testing.T) {
			lang := &gotreesitter.Language{Name: "fixture", SymbolCount: 3, SymbolNames: []string{"end", "_wrapper", "expr"}, ParseTable: [][]uint16{{1, 2, 3}}, PrimaryStateIDs: []gotreesitter.StateID{1}, FullParseGSSConvergenceEnabled: true}
			if mismatch {
				lang.SymbolNames[1] = "different"
			}
			before, err := EncodeLanguageBlob(lang)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "fixture.bin")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			err = updateNonTerminalAliasMapBlob(source, path)
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if mismatch {
				if err == nil {
					t.Fatal("accepted mismatched symbols")
				}
				if !bytes.Equal(before, after) {
					t.Fatal("modified rejected blob")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := gotreesitter.LoadLanguage(after)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.NonTerminalAliasMap, [][]gotreesitter.Symbol{nil, {1, 2}, nil}) {
				t.Fatalf("alias map: %v", got.NonTerminalAliasMap)
			}
			got.NonTerminalAliasMap = nil
			preserved, err := EncodeLanguageBlob(got)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, preserved) {
				t.Fatal("changed fields outside alias map")
			}
		})
	}
}
