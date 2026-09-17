//go:build !grammar_subset || grammar_subset_vue

package grammarruntime

// RegisterVueSupport registers the scanner support for vue.
func RegisterVueSupport() {
	RegisterExternalScanner("vue", VueExternalScanner{})
}
