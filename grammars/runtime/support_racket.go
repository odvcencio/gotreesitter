//go:build !grammar_subset || grammar_subset_racket

package grammarruntime

// RegisterRacketSupport registers the scanner support for racket.
func RegisterRacketSupport() {
	RegisterExternalScanner("racket", RacketExternalScanner{})
}
