//go:build !grammar_subset || grammar_subset_cpp

package grammarruntime

// RegisterCppSupport registers the scanner support for cpp.
func RegisterCppSupport() {
	RegisterExternalScanner("cpp", CppExternalScanner{})
}
