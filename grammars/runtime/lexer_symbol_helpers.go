package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This file carries lexer helpers that more than one grammar shares. It has no
// grammar_subset build tag, so every single-language subset build keeps them.
// A helper that lives in a language-gated file breaks each subset build that
// selects a different language, so put shared helpers here instead.

// firstNonZeroSymbol returns the first symbol in symbols that is not zero, or
// zero when every symbol is zero. Lexers use it to pick the first symbol id
// that the loaded grammar actually defines.
func firstNonZeroSymbol(symbols ...gotreesitter.Symbol) gotreesitter.Symbol {
	for _, sym := range symbols {
		if sym != 0 {
			return sym
		}
	}
	return 0
}
