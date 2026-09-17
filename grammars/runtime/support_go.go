//go:build !grammar_subset || grammar_subset_go

package grammarruntime

// RegisterGoSupport registers the scanner support for go.
func RegisterGoSupport() {
	RegisterExternalScanner("go", GoExternalScanner{})
}
