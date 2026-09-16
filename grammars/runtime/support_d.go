//go:build !grammar_subset || grammar_subset_d

package grammarruntime

// RegisterDSupport registers the scanner support for d.
func RegisterDSupport() {
	RegisterExternalScanner("d", DExternalScanner{})
}
