//go:build !grammar_subset || grammar_subset_elixir

package grammarruntime

// RegisterElixirSupport registers the scanner support for elixir.
func RegisterElixirSupport() {
	RegisterExternalScanner("elixir", ElixirExternalScanner{})
}
