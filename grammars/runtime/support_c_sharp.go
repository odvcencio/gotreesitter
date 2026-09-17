//go:build !grammar_subset || grammar_subset_c_sharp

package grammarruntime

// RegisterC_sharpSupport registers the scanner support for c_sharp.
func RegisterC_sharpSupport() {
	RegisterExternalScanner("c_sharp", CSharpExternalScanner{})
}
