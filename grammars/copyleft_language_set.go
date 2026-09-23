//go:build !gotreesitter_no_copyleft

package grammars

// copyleftLanguageAllowed reports whether name may register in the default
// build. The default build ships every grammar, copyleft-licensed or not;
// see copyleft_language_set_no_copyleft.go for the gotreesitter_no_copyleft
// build, and docs/licensing.md for what "copyleft-licensed grammar" means
// here.
func copyleftLanguageAllowed(name string) bool {
	return true
}
