//go:build !grammar_subset || grammar_subset_typescript

package grammarruntime

// RegisterTypescriptSupport registers the scanner support for typescript.
func RegisterTypescriptSupport() {
	RegisterExternalScanner("typescript", TypeScriptExternalScanner{})
}
