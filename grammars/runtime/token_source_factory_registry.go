package grammarruntime

import (
	"github.com/odvcencio/gotreesitter"
	"sync"
)

var tokenSourceFactoriesMu sync.RWMutex
var tokenSourceFactories = map[string]func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource{}

func registerTokenSourceFactory(name string, factory func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource) {
	tokenSourceFactoriesMu.Lock()
	defer tokenSourceFactoriesMu.Unlock()
	tokenSourceFactories[name] = factory
}
func registerBuiltinTokenSourceFactory(name string, factory func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource) {
	tokenSourceFactoriesMu.Lock()
	defer tokenSourceFactoriesMu.Unlock()
	if _, exists := tokenSourceFactories[name]; !exists {
		tokenSourceFactories[name] = factory
	}
}

// TokenSourceFactory returns the registered lexer factory, or nil for the DFA lexer.
func TokenSourceFactory(name string) func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	ensureBuiltinTokenSourceFactories()
	tokenSourceFactoriesMu.RLock()
	defer tokenSourceFactoriesMu.RUnlock()
	return tokenSourceFactories[name]
}
