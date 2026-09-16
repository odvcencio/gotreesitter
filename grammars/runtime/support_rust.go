//go:build !grammar_subset || grammar_subset_rust

package grammarruntime

// RegisterRustSupport registers the scanner support for rust.
func RegisterRustSupport() {
	RegisterExternalScanner("rust", RustExternalScanner{})
}
