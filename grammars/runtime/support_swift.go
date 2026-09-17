//go:build !grammar_subset || grammar_subset_swift

package grammarruntime

// RegisterSwiftSupport registers the scanner support for swift.
func RegisterSwiftSupport() {
	RegisterExternalScanner("swift", SwiftExternalScanner{})
}
