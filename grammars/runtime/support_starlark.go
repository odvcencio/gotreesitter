//go:build !grammar_subset || grammar_subset_starlark

package grammarruntime

// RegisterStarlarkSupport registers the scanner support for starlark.
func RegisterStarlarkSupport() {
	RegisterExternalScanner("starlark", StarlarkExternalScanner{})
}
