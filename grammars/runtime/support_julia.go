//go:build !grammar_subset || grammar_subset_julia

package grammarruntime

// RegisterJuliaSupport registers the scanner support for julia.
func RegisterJuliaSupport() {
	RegisterExternalScanner("julia", JuliaExternalScanner{})
}
