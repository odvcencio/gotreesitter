package grammars

import (
	"strings"

	"github.com/odvcencio/mane/gotreesitter"
)

// LangEntry holds a registered language with its grammar, extensions, and highlight query.
type LangEntry struct {
	Name               string
	Extensions         []string                      // e.g. [".go", ".mod"]
	Shebangs           []string                      // e.g. ["#!/usr/bin/env python"]
	Language           func() *gotreesitter.Language // lazy loader
	HighlightQuery     string
	TokenSourceFactory func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource // nil = use DFA
}

var registry []LangEntry

// Register adds a language to the registry.
func Register(entry LangEntry) {
	if entry.TokenSourceFactory == nil {
		entry.TokenSourceFactory = defaultTokenSourceFactory(entry.Name)
	}
	registry = append(registry, entry)
}

// DetectLanguage returns the LangEntry for a filename, or nil if unknown.
// Matches by extension first, then shebang.
func DetectLanguage(filename string) *LangEntry {
	// match by extension
	for i := range registry {
		for _, ext := range registry[i].Extensions {
			if strings.HasSuffix(filename, ext) {
				return &registry[i]
			}
		}
	}
	return nil
}

// DetectLanguageByShebang checks the first line of content for shebang matches.
func DetectLanguageByShebang(firstLine string) *LangEntry {
	for i := range registry {
		for _, shebang := range registry[i].Shebangs {
			if strings.HasPrefix(firstLine, shebang) {
				return &registry[i]
			}
		}
	}
	return nil
}

// AllLanguages returns all registered languages.
func AllLanguages() []LangEntry {
	return registry
}
