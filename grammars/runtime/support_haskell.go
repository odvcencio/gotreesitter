//go:build !grammar_subset || grammar_subset_haskell

package grammarruntime

// RegisterHaskellSupport registers the scanner support for haskell.
func RegisterHaskellSupport() {
	RegisterExternalScanner("haskell", HaskellExternalScanner{})
}
