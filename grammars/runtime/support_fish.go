//go:build !grammar_subset || grammar_subset_fish

package grammarruntime

// RegisterFishSupport registers the scanner support for fish.
func RegisterFishSupport() {
	RegisterExternalScanner("fish", FishExternalScanner{})
}
