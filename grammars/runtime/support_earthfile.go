//go:build !grammar_subset || grammar_subset_earthfile

package grammarruntime

// RegisterEarthfileSupport registers the scanner support for earthfile.
func RegisterEarthfileSupport() {
	RegisterExternalScanner("earthfile", EarthfileExternalScanner{})
}
