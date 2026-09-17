//go:build !grammar_subset || grammar_subset_norg

package grammarruntime

// RegisterNorgSupport registers the scanner support for norg.
func RegisterNorgSupport() {
	RegisterExternalScanner("norg", NorgExternalScanner{})
}
