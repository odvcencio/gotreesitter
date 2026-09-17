//go:build !grammar_subset || grammar_subset_svelte

package grammarruntime

// RegisterSvelteSupport registers the scanner support for svelte.
func RegisterSvelteSupport() {
	RegisterExternalScanner("svelte", SvelteExternalScanner{})
}
