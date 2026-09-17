//go:build !grammar_subset || grammar_subset_dart

package grammarruntime

// RegisterDartSupport registers the scanner support for dart.
func RegisterDartSupport() {
	RegisterExternalScanner("dart", DartExternalScanner{})
}
