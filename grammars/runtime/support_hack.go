//go:build !grammar_subset || grammar_subset_hack

package grammarruntime

// RegisterHackSupport registers the scanner support for hack.
func RegisterHackSupport() {
	RegisterExternalScanner("hack", HackExternalScanner{})
}
