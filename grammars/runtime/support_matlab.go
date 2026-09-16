//go:build !grammar_subset || grammar_subset_matlab

package grammarruntime

// RegisterMatlabSupport registers the scanner support for matlab.
func RegisterMatlabSupport() {
	RegisterExternalScanner("matlab", MatlabExternalScanner{})
}
