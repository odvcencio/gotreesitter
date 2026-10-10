package main

import (
	"fmt"
	"os"
	"reflect"

	gts "github.com/odvcencio/gotreesitter"
)

// refreshAliasMapBlob repairs metadata without replacing a shipped grammar's
// lexer, ABI, scanner states, or runtime certifications. Parse tables and symbol
// identities must match before source alias indices can be applied to the blob.
func refreshAliasMapBlob(grammar *ExtractedGrammar, path string) error {
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lang, err := gts.LoadLanguage(blob)
	if err != nil {
		return err
	}
	if err := refreshNonTerminalAliasMap(grammar, lang); err != nil {
		return err
	}
	blob, err = gts.EncodeLanguageBlobWithGenerator(lang, "ts2go/alias-map-only")
	if err != nil {
		return err
	}
	return os.WriteFile(path, blob, 0644)
}

func refreshNonTerminalAliasMap(grammar *ExtractedGrammar, lang *gts.Language) error {
	if grammar == nil || lang == nil {
		return fmt.Errorf("missing grammar or destination language")
	}
	reference := BuildLanguage(grammar)
	NewLanguageCompactor().CompactLanguage(reference)
	actual, expected := reflect.ValueOf(lang).Elem(), reflect.ValueOf(reference).Elem()
	for _, field := range []string{
		"Name", "SymbolCount", "TokenCount", "ExternalTokenCount", "StateCount", "LargeStateCount",
		"FieldCount", "ProductionIDCount", "SymbolNames", "FieldNames", "ExternalSymbols",
		"ParseTable", "SmallParseTable", "SmallParseTableMap", "ParseActions",
		"AliasSequences", "FieldMapSlices", "FieldMapEntries", "LargeStateGotos",
	} {
		if !reflect.DeepEqual(actual.FieldByName(field).Interface(), expected.FieldByName(field).Interface()) {
			return fmt.Errorf("source and blob %s differ; refusing to apply alias indices", field)
		}
	}
	if len(lang.SymbolMetadata) != len(reference.SymbolMetadata) {
		return fmt.Errorf("source and blob symbol metadata lengths differ")
	}
	for i, symbol := range lang.SymbolMetadata {
		want := reference.SymbolMetadata[i]
		if symbol.Name != want.Name || symbol.Visible != want.Visible || symbol.Named != want.Named || symbol.Supertype != want.Supertype {
			return fmt.Errorf("source and blob symbol %d metadata differ", i)
		}
	}
	lang.NonTerminalAliasMap = reference.NonTerminalAliasMap
	return nil
}
