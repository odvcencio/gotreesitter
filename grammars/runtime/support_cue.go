//go:build !grammar_subset || grammar_subset_cue

package grammarruntime

// RegisterCueSupport registers the scanner support for cue.
func RegisterCueSupport() {
	RegisterExternalScanner("cue", CueExternalScanner{})
}
