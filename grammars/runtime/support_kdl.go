//go:build !grammar_subset || grammar_subset_kdl

package grammarruntime

// RegisterKdlSupport registers the scanner support for kdl.
func RegisterKdlSupport() {
	RegisterExternalScanner("kdl", KdlExternalScanner{})
}
