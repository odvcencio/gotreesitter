package grammars

import (
	"errors"

	"github.com/odvcencio/gotreesitter"
)

var errEntryWithoutLanguage = errors.New("grammars: language entry has no loader")

// NewParser returns a parser for the entry's language. The parser has no token
// source of its own, so parser.Parse always uses the DFA lexer. Call
// entry.Parse to use the registered token source (c, cpp, java, json, authzed).
func (e *LangEntry) NewParser() *gotreesitter.Parser {
	return gotreesitter.NewParser(e.Language())
}

// Parse parses src with a new parser. It uses the entry's TokenSourceFactory
// when there is one and the DFA lexer otherwise, the same choice ParseFile
// makes. Unlike gotreesitter.NewParser(lang).Parse, it picks up the hand-written
// lexers. Release the tree when done.
func (e *LangEntry) Parse(src []byte) (*gotreesitter.Tree, error) {
	if e.Language == nil {
		return nil, errEntryWithoutLanguage
	}
	lang := e.Language()
	parser := gotreesitter.NewParser(lang)
	if e.TokenSourceFactory != nil {
		return parser.ParseWithTokenSource(src, e.TokenSourceFactory(src, lang))
	}
	return parser.Parse(src)
}

// NewHighlighter returns a highlighter built from the entry's language and
// HighlightQuery. It passes the entry's TokenSourceFactory to the highlighter
// when there is one; a WithTokenSourceFactory option in opts overrides it. Take
// the entry from DetectLanguage, DetectLanguageByName or AllLanguages, so that
// HighlightQuery holds the composed query.
func (e *LangEntry) NewHighlighter(opts ...gotreesitter.HighlighterOption) (*gotreesitter.Highlighter, error) {
	if e.Language == nil {
		return nil, errEntryWithoutLanguage
	}
	lang := e.Language()
	if factory := e.TokenSourceFactory; factory != nil {
		opts = append([]gotreesitter.HighlighterOption{
			gotreesitter.WithTokenSourceFactory(func(src []byte) gotreesitter.TokenSource {
				return factory(src, lang)
			}),
		}, opts...)
	}
	return gotreesitter.NewHighlighter(lang, e.HighlightQuery, opts...)
}
