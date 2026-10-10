package gotreesitter_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// These receipts were generated at 952c5f79b before declaration extraction.
// Include the Go fixtures from understanding, FactProgram, and outline tests.
func TestFactProgramGoCompatibility(t *testing.T) {
	sources := []string{
		"package main\n\ntype Service struct{}\n\nfunc (s Service) Run() {\n\thelper()\n}\n\nfunc helper() {}\n",
		"package main\n\nimport alias \"example.com/library\"\n\ntype Service struct{}\n\nfunc (s Service) Run() {\n\talias.Helper()\n}\n",
		"package main\nfunc run() { helper() }\n",
	}
	for _, fixture := range outlineGoldenFixtures {
		if fixture.Language != "go" {
			continue
		}
		source, err := os.ReadFile(filepath.Join(outlineGoldenRoot, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, string(source))
	}
	entry := grammars.DetectLanguage("main.go")
	for i, source := range sources {
		// The broken outline fixture intentionally contains syntax errors.
		tree, err := gts.NewParser(entry.Language()).Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Release()
		definitions, err := gts.NewFactProgram(tree.Language(), gts.FactDefinitions, gts.WithDeclarationRules(grammars.DeclarationRules(*entry)))
		if err != nil {
			t.Fatal(err)
		}
		all, err := gts.NewFactProgram(tree.Language(), gts.FactAll, gts.WithDeclarationRules(grammars.DeclarationRules(*entry)))
		if err != nil {
			t.Fatal(err)
		}
		defs := definitions.Extract(tree).Definitions
		if !slices.Equal(defs, gts.ExtractDefinitionSpans(tree)) {
			t.Fatal("legacy definitions differ")
		}
		outline, err := gts.NewOutliner(tree.Language(), grammars.ResolveTagsQuery(*entry))
		if err != nil {
			t.Fatal(err)
		}
		symbols, report := outline.OutlineTree(tree)
		enclosing := make([]gts.DefinitionSpan, len(source))
		for offset := range len(source) {
			enclosing[offset], _ = gts.EnclosingDefinition(tree, uint32(offset))
		}
		receipt := struct {
			Source      string
			Definitions []gts.DefinitionSpan
			All         gts.FactSet
			Calls       []gts.CallRef
			Enclosing   []gts.DefinitionSpan
			Outline     []gts.OutlineSymbol
			Report      gts.OutlineReport
			Kinds       []string
		}{source, defs, all.Extract(tree), gts.ExtractCalls(tree), enclosing, symbols, report, outline.DefinitionKinds()}
		data, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", "facts", "go-compatibility-"+string(rune('0'+i))+".json")
		if os.Getenv("GTS_UPDATE_FACT_COMPAT") == "1" {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
		}
		expected, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != strings.TrimSpace(string(expected)) {
			t.Fatalf("legacy outputs changed for fixture %d (%s)", i, path)
		}
	}
}
