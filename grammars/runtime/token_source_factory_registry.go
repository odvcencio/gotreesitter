package grammarruntime

import "github.com/odvcencio/gotreesitter"

var tokenSourceFactories = map[string]func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource{}

func registerTokenSourceFactory(name string, factory func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource) {
	tokenSourceFactories[name] = factory
}

// TokenSourceFactory returns the registered lexer factory, or nil for the DFA lexer.
func TokenSourceFactory(name string) func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	return tokenSourceFactories[name]
}
