package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type collapsedChildOccurrenceRow struct {
	language   string
	load       func() *gotreesitter.Language
	parent     string
	child      string
	childNamed bool
}

func TestCollapsedChildOccurrencePolicyCoversExactAndAdaptedArtifacts(t *testing.T) {
	rows := []collapsedChildOccurrenceRow{
		{language: "ruby", load: grammars.RubyLanguage, parent: "bare_string", child: "string_content", childNamed: true},
		{language: "ruby", load: grammars.RubyLanguage, parent: "bare_symbol", child: "string_content", childNamed: true},
		{language: "apex", load: grammars.ApexLanguage, parent: "after_delete", child: "after_delete"},
		{language: "apex", load: grammars.ApexLanguage, parent: "after_insert", child: "after_insert"},
		{language: "apex", load: grammars.ApexLanguage, parent: "after_undelete", child: "after_undelete"},
		{language: "apex", load: grammars.ApexLanguage, parent: "after_update", child: "after_update"},
		{language: "apex", load: grammars.ApexLanguage, parent: "before_delete", child: "before_delete"},
		{language: "apex", load: grammars.ApexLanguage, parent: "before_insert", child: "before_insert"},
		{language: "apex", load: grammars.ApexLanguage, parent: "before_update", child: "before_update"},
		{language: "apex", load: grammars.ApexLanguage, parent: "delete", child: "delete"},
		{language: "apex", load: grammars.ApexLanguage, parent: "insert", child: "insert"},
		{language: "apex", load: grammars.ApexLanguage, parent: "super", child: "super"},
		{language: "apex", load: grammars.ApexLanguage, parent: "system", child: "system"},
		{language: "apex", load: grammars.ApexLanguage, parent: "undelete", child: "undelete"},
		{language: "apex", load: grammars.ApexLanguage, parent: "upsert", child: "upsert"},
		{language: "apex", load: grammars.ApexLanguage, parent: "user", child: "user"},
		{language: "kotlin", load: grammars.KotlinLanguage, parent: "identifier", child: "simple_identifier", childNamed: true},
		{language: "hack", load: grammars.HackLanguage, parent: "true", child: "true"},
		{language: "hack", load: grammars.HackLanguage, parent: "false", child: "false"},
		{language: "hack", load: grammars.HackLanguage, parent: "null", child: "null"},
		{language: "dart", load: grammars.DartLanguage, parent: "super", child: "super"},
		{language: "dart", load: grammars.DartLanguage, parent: "this", child: "this"},
		{language: "elixir", load: grammars.ElixirLanguage, parent: "nil", child: "nil"},
		{language: "rust", load: grammars.RustLanguage, parent: "range_expression", child: ".."},
	}
	if len(rows) != 24 {
		t.Fatalf("collapsed-child occurrence rows=%d, want 24", len(rows))
	}
	for _, row := range rows {
		row := row
		t.Run(row.language+"/"+row.parent, func(t *testing.T) {
			lang := row.load()
			if lang.NativeResultCompatibility&gotreesitter.ResultCompatibilityNativeCollapsedChildren == 0 {
				t.Fatal("exact artifact omitted native collapsed-child capability")
			}
			parent, parentOK := collapsedChildStrictSubsetSymbol(lang, row.parent, true)
			child, childOK := collapsedChildStrictSubsetSymbol(lang, row.child, row.childNamed)
			if !parentOK || !childOK {
				t.Fatalf("could not resolve exact pair %s/%s", row.parent, row.child)
			}
			// A true adapted clone retains the exact built-in profile receipt and
			// symbol table; display-name-compatible caller artifacts are not covered.
			adapted := &gotreesitter.Language{
				Name:                      lang.Name,
				TokenCount:                lang.TokenCount,
				NativeResultCompatibility: lang.NativeResultCompatibility,
				SymbolNames:               lang.SymbolNames,
				SymbolMetadata:            lang.SymbolMetadata,
			}
			for _, artifact := range []struct {
				name string
				lang *gotreesitter.Language
			}{
				{name: "exact", lang: lang},
				{name: "adapted", lang: adapted},
			} {
				parser := gotreesitter.NewParser(artifact.lang)
				if !gotreesitter.ParserRetainsCollapsedChildOccurrenceForTest(parser, parent, child) {
					t.Fatalf("%s occurrence policy rejected registered pair", artifact.name)
				}
				if gotreesitter.ParserRetainsCollapsedChildOccurrenceForTest(parser, parent, 0) ||
					gotreesitter.ParserRetainsCollapsedChildOccurrenceForTest(parser, 0, child) {
					t.Fatalf("%s occurrence policy accepted a parent/raw-child mismatch", artifact.name)
				}
			}

		})
	}
}

func collapsedChildStrictSubsetSymbol(lang *gotreesitter.Language, name string, named bool) (gotreesitter.Symbol, bool) {
	for index, symbolName := range lang.SymbolNames {
		if symbolName == name && index < len(lang.SymbolMetadata) && lang.SymbolMetadata[index].Named == named {
			return gotreesitter.Symbol(index), true
		}
	}
	return 0, false
}
