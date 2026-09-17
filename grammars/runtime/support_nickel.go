//go:build !grammar_subset || grammar_subset_nickel

package grammarruntime

// RegisterNickelSupport registers the scanner support for nickel.
func RegisterNickelSupport() {
	RegisterExternalScanner("nickel", NickelExternalScanner{})
}
