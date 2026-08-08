// Package lean provides the native gotreesitter grammar for Lean 4.
// Import this package to register .lean files with the grammar registry.
package lean

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ReferenceVersion is the Lean release used to define this grammar.
const ReferenceVersion = "4.32.2"

//go:embed lean.bin
var languageBlob []byte

var (
	languageOnce sync.Once
	language     *gotreesitter.Language
	languageErr  error
)

func init() {
	grammars.RegisterExternalScanner("lean", ExternalScanner{})
	grammars.RegisterExtension(grammars.ExtensionEntry{
		Name:             "lean",
		Extensions:       []string{".lean"},
		Aliases:          []string{"lean4"},
		GenerateLanguage: loadLanguage,
		GrammarSource:    grammars.GrammarSourceGrammargenBlob,
		HighlightQuery:   HighlightQuery,
		TagsQuery:        TagsQuery,
	})
}

// Language returns the packaged Lean language.
func Language() *gotreesitter.Language {
	lang, err := loadLanguage()
	if err != nil {
		panic(fmt.Sprintf("gotreesitter: load Lean grammar: %v", err))
	}
	return lang
}

func loadLanguage() (*gotreesitter.Language, error) {
	languageOnce.Do(func() {
		language, languageErr = grammars.LoadLanguage("lean", languageBlob)
	})
	return language, languageErr
}
