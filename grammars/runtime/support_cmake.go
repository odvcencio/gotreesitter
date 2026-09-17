//go:build !grammar_subset || grammar_subset_cmake

package grammarruntime

// RegisterCmakeSupport registers the scanner support for cmake.
func RegisterCmakeSupport() {
	RegisterExternalScanner("cmake", CmakeExternalScanner{})
}
