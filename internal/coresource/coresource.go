// Package coresource lets library code mark a token source as one whose lexing
// the C-faithful core can reproduce with a certified scan provider. The root
// package sets Mint in an init; grammars/runtime calls it. The marked value is
// an unexported type of the root package, so only library code can make it.
package coresource

// Mint returns fallback marked for the core scan provider, bound to lang (a
// *gotreesitter.Language) and to the bytes src. fallback is a
// gotreesitter.TokenSource. The result is a gotreesitter.TokenSource. A nil
// Mint (the root package is not linked) returns fallback unchanged.
var Mint func(fallback, lang any, src []byte) any

// Mark returns fallback marked as described at Mint, or fallback itself when
// Mint is not set.
func Mark(fallback, lang any, src []byte) any {
	if Mint == nil {
		return fallback
	}
	return Mint(fallback, lang, src)
}
