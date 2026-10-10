package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestRefreshNonTerminalAliasMapBlob(t *testing.T) {
	grammar, err := ExtractGrammar(miniParserC)
	if err != nil {
		t.Fatal(err)
	}
	grammar.NonTerminalAliasMap = make([][]uint16, grammar.SymbolCount)
	grammar.NonTerminalAliasMap[4] = []uint16{4, 5}
	lang := BuildLanguage(grammar)
	NewLanguageCompactor().CompactLanguage(lang)
	lang.NonTerminalAliasMap = nil
	blob, err := gts.EncodeLanguageBlobWithGenerator(lang, "test")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "grammar.bin")
	if err := os.WriteFile(path, blob, 0644); err != nil {
		t.Fatal(err)
	}
	if err := refreshAliasMapBlob(grammar, path); err != nil {
		t.Fatal(err)
	}
	patched, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := gts.LoadLanguage(patched)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.NonTerminalAliasMap[4], []gts.Symbol{4, 5}) {
		t.Fatalf("encoded alias map = %v", loaded.NonTerminalAliasMap)
	}
	if err := refreshAliasMapBlob(grammar, path); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(patched, again) {
		t.Fatalf("refresh is not idempotent: %v", err)
	}
	grammar.Name = "different"
	if err := refreshAliasMapBlob(grammar, path); err == nil {
		t.Fatal("mismatched artifact accepted")
	}
	rejected, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(patched, rejected) {
		t.Fatalf("rejected refresh changed the blob: %v", err)
	}
}

func TestRefreshNonTerminalAliasMapPreservesOtherMetadata(t *testing.T) {
	grammar, err := ExtractGrammar(miniParserC)
	if err != nil {
		t.Fatal(err)
	}
	grammar.NonTerminalAliasMap = make([][]uint16, grammar.SymbolCount)
	grammar.NonTerminalAliasMap[4] = []uint16{4, 5}
	lang := BuildLanguage(grammar)
	NewLanguageCompactor().CompactLanguage(lang)
	lang.NonTerminalAliasMap = nil
	// These can legitimately differ in a shipped blob; refreshing aliases must
	// not re-extract them or change their existing certification.
	lang.LanguageVersion = 13
	lang.CRecoveryCostCompetitionCapable = false
	lang.ExternalLexStates = [][]bool{{true}}
	lang.LexStates = nil
	before := reflect.ValueOf(lang).Elem()
	saved := make(map[string]any)
	for i := 0; i < before.NumField(); i++ {
		field := before.Type().Field(i)
		if field.IsExported() && field.Name != "NonTerminalAliasMap" {
			saved[field.Name] = before.Field(i).Interface()
		}
	}
	if err := refreshNonTerminalAliasMap(grammar, lang); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lang.NonTerminalAliasMap[4], []gts.Symbol{4, 5}) {
		t.Fatalf("alias map = %v", lang.NonTerminalAliasMap)
	}
	for field, value := range saved {
		if !reflect.DeepEqual(value, before.FieldByName(field).Interface()) {
			t.Errorf("refresh changed %s", field)
		}
	}
	for _, mismatch := range []string{"symbol", "table"} {
		t.Run(mismatch, func(t *testing.T) {
			other := BuildLanguage(grammar)
			NewLanguageCompactor().CompactLanguage(other)
			other.NonTerminalAliasMap = nil
			if mismatch == "symbol" {
				other.SymbolNames[1] = "different"
			} else {
				other.ParseTable[0][0]++
			}
			if err := refreshNonTerminalAliasMap(grammar, other); err == nil || !strings.Contains(err.Error(), "differ") {
				t.Fatalf("mismatched artifact accepted: %v", err)
			}
			if other.NonTerminalAliasMap != nil {
				t.Fatal("rejected refresh changed alias metadata")
			}
		})
	}
}
