package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractJsonGrammar tests extraction against the real tree-sitter-json
// parser.c. The file is expected at testdata/json_parser.c. If the file
// doesn't exist, the test is skipped.
func TestExtractJsonGrammar(t *testing.T) {
	path := filepath.Join("testdata", "json_parser.c")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Skip("testdata/json_parser.c not found; skipping real grammar test")
	}

	g, err := ExtractGrammar(string(source))
	if err != nil {
		t.Fatal(err)
	}

	// Verify constants.
	if g.Name != "json" {
		t.Errorf("Name = %q, want %q", g.Name, "json")
	}
	if g.StateCount != 32 {
		t.Errorf("StateCount = %d, want 32", g.StateCount)
	}
	if g.LargeStateCount != 7 {
		t.Errorf("LargeStateCount = %d, want 7", g.LargeStateCount)
	}
	if g.SymbolCount != 25 {
		t.Errorf("SymbolCount = %d, want 25", g.SymbolCount)
	}
	if g.TokenCount != 15 {
		t.Errorf("TokenCount = %d, want 15", g.TokenCount)
	}
	if g.FieldCount != 2 {
		t.Errorf("FieldCount = %d, want 2", g.FieldCount)
	}
	if g.ProductionIDCount != 2 {
		t.Errorf("ProductionIDCount = %d, want 2", g.ProductionIDCount)
	}

	// Verify symbol names.
	if len(g.SymbolNames) != 25 {
		t.Fatalf("len(SymbolNames) = %d, want 25", len(g.SymbolNames))
	}
	if g.SymbolNames[0] != "end" {
		t.Errorf("SymbolNames[0] = %q, want %q", g.SymbolNames[0], "end")
	}
	if g.SymbolNames[1] != "{" {
		t.Errorf("SymbolNames[1] = %q, want %q", g.SymbolNames[1], "{")
	}
	if g.SymbolNames[10] != "number" {
		t.Errorf("SymbolNames[10] = %q, want %q", g.SymbolNames[10], "number")
	}

	// Verify field names.
	if len(g.FieldNames) != 3 {
		t.Fatalf("len(FieldNames) = %d, want 3", len(g.FieldNames))
	}
	if g.FieldNames[1] != "key" {
		t.Errorf("FieldNames[1] = %q, want %q", g.FieldNames[1], "key")
	}
	if g.FieldNames[2] != "value" {
		t.Errorf("FieldNames[2] = %q, want %q", g.FieldNames[2], "value")
	}

	// Verify field maps.
	if len(g.FieldMapSlices) == 0 {
		t.Error("FieldMapSlices is empty")
	}
	if len(g.FieldMapEntries) == 0 {
		t.Error("FieldMapEntries is empty")
	}

	// Verify parse table.
	if len(g.ParseTable) != 7 {
		t.Errorf("len(ParseTable) = %d, want 7", len(g.ParseTable))
	}

	// Verify lex modes.
	if len(g.LexModes) != 32 {
		t.Errorf("len(LexModes) = %d, want 32", len(g.LexModes))
	}
	// States 17-19 should have lex_state=1.
	if g.LexModes[17].LexState != 1 {
		t.Errorf("LexModes[17].LexState = %d, want 1", g.LexModes[17].LexState)
	}

	// Verify parse actions were extracted.
	if len(g.ParseActions) < 10 {
		t.Errorf("len(ParseActions) = %d, want >= 10", len(g.ParseActions))
	}

	// Verify small parse table was extracted.
	if len(g.SmallParseTable) == 0 {
		t.Error("SmallParseTable is empty")
	}
	if len(g.SmallParseTableMap) == 0 {
		t.Error("SmallParseTableMap is empty")
	}

	// Verify symbol metadata.
	if len(g.SymbolMetadata) != 25 {
		t.Fatalf("len(SymbolMetadata) = %d, want 25", len(g.SymbolMetadata))
	}
	// sym__value (index 16) has supertype=true.
	if !g.SymbolMetadata[16].Supertype {
		t.Error("SymbolMetadata[16] (_value) should be supertype")
	}

	// Generate Go code and verify it compiles (basic syntax check).
	code := GenerateGo(g, "grammars")
	if !strings.Contains(code, "func JsonLanguage()") {
		t.Error("generated code missing JsonLanguage function")
	}
	if !strings.Contains(code, "SymbolCount:        25") {
		t.Error("generated code missing correct SymbolCount")
	}
	if !strings.Contains(code, "StateCount:         32") {
		t.Error("generated code missing correct StateCount")
	}
}
