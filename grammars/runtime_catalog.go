package grammars

import (
	"github.com/odvcencio/gotreesitter"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

func init() {
	grammarruntime.RegisterCatalog(func(name string) ([]byte, func(), error) {
		blob, err := readGrammarBlob(name)
		return blob.data, blob.release, err
	}, func(name string) string {
		if entry := DetectLanguageByName(name); entry != nil {
			return entry.Name
		}
		return ""
	})
}

func loadEmbeddedLanguage(name string) *gotreesitter.Language  { return grammarruntime.Language(name) }
func loadPreferredLanguage(name string) *gotreesitter.Language { return grammarruntime.Language(name) }
func defaultTokenSourceFactory(name string) func([]byte, *gotreesitter.Language) gotreesitter.TokenSource {
	return grammarruntime.TokenSourceFactory(name)
}
